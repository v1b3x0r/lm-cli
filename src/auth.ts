import { createHash, randomBytes, timingSafeEqual } from 'node:crypto';
import { createServer, type Server } from 'node:http';
import { spawn } from 'node:child_process';
import { type Account, lockAccount, readAccount, removeAccount, withAccountLock, writeAccount } from './account-store.js';
import { CliError } from './output.js';

// A straight port of the Go rc5 `lm login`: same issuer, same public client, same PKCE loopback.
const ISSUER = 'https://exciting-kayak-4871.customers.stytch.com';
export interface Endpoints { issuer: string; register: string; token: string; authorize: string; resource: string }
export const DEFAULT_ENDPOINTS: Endpoints = {
  issuer: ISSUER,
  register: `${ISSUER}/v1/oauth2/register`,
  token: `${ISSUER}/v1/oauth2/token`,
  authorize: 'https://viibe.to/living-memory/oauth/authorize',
  resource: 'https://lme.viibe.to/mcp',
};
const SCOPE = 'openid offline_access';
const LOGIN_TIMEOUT_MS = 5 * 60_000;
const MAX_RESPONSE_BYTES = 1 << 20;
const REFRESH_MARGIN_MS = 30_000;

const resolve = (e?: Partial<Endpoints>): Endpoints => ({ ...DEFAULT_ENDPOINTS, ...e });
const b64url = (b: Buffer) => b.toString('base64url');

/** POST to an OAuth endpoint. Never follows redirects (codes and refresh tokens must not travel). */
async function oauthPost<T>(url: string, contentType: string, body: string): Promise<T> {
  let res: Response;
  try { res = await fetch(url, { method: 'POST', headers: { 'Content-Type': contentType }, body, redirect: 'manual' }); }
  catch { throw new CliError('oauth_unreachable', 'OAuth request failed; no automatic retry'); }
  if (res.status < 200 || res.status >= 300)
    throw new CliError('oauth_refused', `OAuth endpoint answered HTTP ${res.status}; run lm login if authorization expired`);
  const text = await res.text();
  if (text.length > MAX_RESPONSE_BYTES) throw new CliError('oauth_invalid', 'invalid OAuth response');
  try { return JSON.parse(text) as T; } catch { throw new CliError('oauth_invalid', 'invalid OAuth response'); }
}

async function exchange(a: Account, form: Record<string, string>, ep: Endpoints): Promise<void> {
  const t = await oauthPost<{ access_token?: unknown; refresh_token?: unknown; token_type?: unknown; expires_in?: unknown }>(
    ep.token, 'application/x-www-form-urlencoded', new URLSearchParams({ ...form, client_id: a.clientId }).toString());
  const valid = typeof t.access_token === 'string' && t.access_token !== '' && !/[\r\n]/.test(t.access_token)
    && typeof t.token_type === 'string' && t.token_type.toLowerCase() === 'bearer'
    && typeof t.expires_in === 'number' && t.expires_in > 0 && t.expires_in <= 31_536_000;
  if (!valid) throw new CliError('oauth_invalid', 'invalid OAuth token response');
  a.accessToken = t.access_token as string;
  a.expiresAt = new Date(Date.now() + (t.expires_in as number) * 1000).toISOString();
  if (typeof t.refresh_token === 'string' && t.refresh_token) a.refreshToken = t.refresh_token;
}

function savedBind(a: Account): string {
  let u: URL;
  try { u = new URL(a.redirectUri); } catch { throw new CliError('account_invalid', 'invalid saved OAuth callback'); }
  if (u.protocol !== 'http:' || u.hostname !== '127.0.0.1' || !u.port || u.pathname !== '/callback'
    || u.search || u.hash || u.username || u.password)
    throw new CliError('account_invalid', 'invalid saved OAuth callback');
  return u.port;
}

function listen(port: number): Promise<Server> {
  return new Promise((ok, fail) => {
    const s = createServer();
    s.once('error', () => fail(new CliError('loopback_busy', 'cannot bind OAuth loopback callback; close the process using its port')));
    s.listen(port, '127.0.0.1', () => ok(s));
  });
}

const sameText = (a: string, b: string) =>
  a.length === b.length && timingSafeEqual(Buffer.from(a), Buffer.from(b));

/**
 * Best-effort browser open; the URL is always printed as well. Windows is deliberately skipped
 * (Go rc5 parity): routing the link through `cmd /c start` would let cmd.exe split it at every
 * `&` between query parameters and run the pieces as commands.
 */
export function browserCommand(platform: NodeJS.Platform, url: string): [string, string[]] | null {
  if (platform === 'darwin') return ['open', [url]];
  if (platform === 'linux') return ['xdg-open', [url]];
  return null;
}

function defaultOpen(url: string): void {
  const cmd = browserCommand(process.platform, url);
  if (!cmd) return;
  try { spawn(cmd[0], cmd[1], { detached: true, stdio: 'ignore' }).on('error', () => {}).unref(); } catch { /* URL is printed */ }
}

export async function login(opts: {
  home: string; endpoints?: Partial<Endpoints>; openUrl?: (u: string) => void; timeoutMs?: number;
}): Promise<void> {
  const ep = resolve(opts.endpoints);
  await withAccountLock(opts.home, async () => {
    const old = readAccount(opts.home);
    const server = await listen(old ? Number(savedBind(old)) : 0);
    const host = `127.0.0.1:${(server.address() as { port: number }).port}`;
    let timer: NodeJS.Timeout | undefined;
    try {
      const a: Account = old
        ? { clientId: old.clientId, redirectUri: old.redirectUri, expiresAt: new Date(0).toISOString() }
        : { clientId: '', redirectUri: `http://${host}/callback`, expiresAt: new Date(0).toISOString() };
      if (!a.clientId) {
        const r = await oauthPost<{ client_id?: unknown; token_endpoint_auth_method?: unknown; redirect_uris?: unknown }>(
          ep.register, 'application/json', JSON.stringify({
            client_name: 'Living Memory CLI', redirect_uris: [a.redirectUri],
            grant_types: ['authorization_code', 'refresh_token'], response_types: ['code'],
            token_endpoint_auth_method: 'none',
          }));
        if (typeof r.client_id !== 'string' || !r.client_id || r.token_endpoint_auth_method !== 'none'
          || !Array.isArray(r.redirect_uris) || r.redirect_uris.length !== 1 || r.redirect_uris[0] !== a.redirectUri)
          throw new CliError('oauth_invalid', 'invalid public client registration');
        a.clientId = r.client_id;
        writeAccount(opts.home, a);
      }

      const state = b64url(randomBytes(32));
      const verifier = b64url(randomBytes(32));
      const challenge = b64url(createHash('sha256').update(verifier).digest());

      const code = new Promise<string>((ok, fail) => {
        let received = false;
        server.on('request', (req, res) => {
          res.setHeader('Cache-Control', 'no-store');
          res.setHeader('Referrer-Policy', 'no-referrer');
          res.setHeader('Content-Type', 'text/plain; charset=utf-8');
          // The loopback lives for one login. A kept-alive socket would outlive it and be reused
          // against the NEXT login's listener on the same saved port, failing with ECONNRESET.
          res.setHeader('Connection', 'close');
          const u = new URL(req.url ?? '/', `http://${host}`);
          const q = u.searchParams;
          // Anything that is not OUR callback is ignored, not fatal: a stray or forged hit must
          // not end the login the person is still completing in the browser.
          if (req.method !== 'GET' || req.headers.host !== host || u.pathname !== '/callback'
            || q.getAll('state').length !== 1 || !sameText(q.get('state') ?? '', state)
            || q.getAll('iss').length !== 1 || q.get('iss') !== ep.issuer) {
            res.statusCode = 400;
            return res.end('Invalid callback.\n');
          }
          if (received) { res.statusCode = 409; return res.end('Callback already received.\n'); }
          received = true;
          res.end('Returned to lm. You can close this tab.\n');
          if (q.get('error')) return fail(new CliError('login_denied', 'authorization denied; run lm login to try again'));
          const c = q.getAll('code');
          if (c.length !== 1 || !c[0]) return fail(new CliError('login_failed', 'authorization code missing'));
          ok(c[0]);
        });
      });
      const timeout = new Promise<never>((_, fail) => {
        timer = setTimeout(() => fail(new CliError('login_timeout', 'login timed out; run lm login again')),
          opts.timeoutMs ?? LOGIN_TIMEOUT_MS);
      });

      const link = `${ep.authorize}?${new URLSearchParams({
        client_id: a.clientId, redirect_uri: a.redirectUri, response_type: 'code', scope: SCOPE, state,
        code_challenge: challenge, code_challenge_method: 'S256', resource: ep.resource,
      })}`;
      process.stderr.write(`Open this URL on this computer and approve within 5 minutes:\n${link}\n`);
      (opts.openUrl ?? defaultOpen)(link);

      const received = await Promise.race([code, timeout]);
      await exchange(a, {
        grant_type: 'authorization_code', code: received, redirect_uri: a.redirectUri,
        code_verifier: verifier, resource: ep.resource,
      }, ep);
      writeAccount(opts.home, a);
    } finally {
      clearTimeout(timer);
      server.closeAllConnections();
      server.close();
    }
  });
}

export function logout(home: string): void {
  const release = lockAccount(home);
  try { removeAccount(home); } finally { release(); }
}

function signedIn(a: Account | null): a is Account {
  return !!a && (!!a.refreshToken || (!!a.accessToken && Date.parse(a.expiresAt) - Date.now() >= REFRESH_MARGIN_MS));
}

/**
 * A bearer for the account rail, refreshed when it has under 30 s left (or when `force`d after
 * a 401). Callers hold the account lock (see withAccountLock) so a rotating refresh token is
 * never used twice.
 */
export async function accessToken(home: string, endpoints?: Partial<Endpoints>, force = false): Promise<string> {
  const a = readAccount(home);
  if (!signedIn(a)) throw new CliError('not_signed_in', 'not signed in; run lm login');
  if (!force && a.accessToken && Date.parse(a.expiresAt) - Date.now() >= REFRESH_MARGIN_MS) return a.accessToken;
  if (!a.refreshToken) throw new CliError('not_signed_in', 'account session expired; run lm login');
  try {
    await exchange(a, { grant_type: 'refresh_token', refresh_token: a.refreshToken }, resolve(endpoints));
  } catch {
    throw new CliError('not_signed_in', 'account session expired; run lm login');
  }
  writeAccount(home, a);
  return a.accessToken!;
}
