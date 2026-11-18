import { createServer, type Server } from 'node:http';
import { existsSync, mkdirSync, mkdtempSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { accessToken, login, logout, type Endpoints } from '../src/auth.js';
import { readAccount, writeAccount } from '../src/account-store.js';
import { CliError } from '../src/output.js';

let server: Server;
let base: string;
let home: string;
let registrations = 0;
let tokenBody: Record<string, unknown>;
let lastTokenForm: URLSearchParams | undefined;

function endpoints(): Partial<Endpoints> {
  return {
    issuer: base,
    register: `${base}/v1/oauth2/register`,
    token: `${base}/v1/oauth2/token`,
    authorize: `${base}/authorize`,
  };
}

/** Simulates the browser: read the authorize link, then hit the loopback callback. */
function approve(overrides: Record<string, string> = {}) {
  return (link: string) => {
    const u = new URL(link);
    const cb = new URL(u.searchParams.get('redirect_uri')!);
    cb.searchParams.set('code', 'the-code');
    cb.searchParams.set('state', u.searchParams.get('state')!);
    cb.searchParams.set('iss', base);
    for (const [k, v] of Object.entries(overrides)) cb.searchParams.set(k, v);
    void fetch(cb).catch(() => {});
  };
}

beforeEach(async () => {
  home = mkdtempSync(join(tmpdir(), 'lm-auth-'));
  registrations = 0;
  lastTokenForm = undefined;
  tokenBody = { access_token: 'at-1', refresh_token: 'rt-1', token_type: 'Bearer', expires_in: 3600 };
  server = createServer((req, res) => {
    let raw = '';
    req.on('data', (c) => (raw += c));
    req.on('end', () => {
      res.setHeader('Content-Type', 'application/json');
      if (req.url === '/v1/oauth2/register') {
        registrations += 1;
        const body = JSON.parse(raw) as { redirect_uris: string[] };
        return res.end(JSON.stringify({ client_id: 'cid-1', token_endpoint_auth_method: 'none', redirect_uris: body.redirect_uris }));
      }
      if (req.url === '/v1/oauth2/token') {
        lastTokenForm = new URLSearchParams(raw);
        return res.end(JSON.stringify(tokenBody));
      }
      res.statusCode = 404;
      res.end('{}');
    });
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  const addr = server.address() as { port: number };
  base = `http://127.0.0.1:${addr.port}`;
  vi.spyOn(process.stderr, 'write').mockImplementation(() => true);
});

afterEach(async () => {
  vi.restoreAllMocks();
  await new Promise((r) => server.close(r));
});

describe('lm login', () => {
  it('registers, exchanges the code with PKCE, and saves a 0600 account', async () => {
    await login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 });
    const a = readAccount(home)!;
    expect(a.clientId).toBe('cid-1');
    expect(a.accessToken).toBe('at-1');
    expect(a.refreshToken).toBe('rt-1');
    expect(a.redirectUri).toMatch(/^http:\/\/127\.0\.0\.1:\d+\/callback$/);
    expect(statSync(join(home, 'account.json')).mode & 0o777).toBe(0o600);
    expect(lastTokenForm!.get('grant_type')).toBe('authorization_code');
    expect(lastTokenForm!.get('code')).toBe('the-code');
    expect(lastTokenForm!.get('code_verifier')).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(registrations).toBe(1);
  });

  it('reuses the saved clientId and redirect port', async () => {
    await login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 });
    const first = readAccount(home)!;
    await login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 });
    const second = readAccount(home)!;
    expect(registrations).toBe(1);
    expect(second.clientId).toBe(first.clientId);
    expect(second.redirectUri).toBe(first.redirectUri);
  });

  it('rejects a callback whose state does not match, and keeps waiting', async () => {
    const p = login({ home, endpoints: endpoints(), openUrl: approve({ state: 'forged' }), timeoutMs: 300 });
    await expect(p).rejects.toMatchObject({ code: 'login_timeout' });
    expect(lastTokenForm).toBeUndefined();
  });

  it('times out and releases the lock', async () => {
    await expect(login({ home, endpoints: endpoints(), openUrl: () => {}, timeoutMs: 50 }))
      .rejects.toMatchObject({ code: 'login_timeout' });
    expect(existsSync(join(home, 'account.json.lock'))).toBe(false);
  });

  it('reports a held lock by its path', async () => {
    mkdirSync(home, { recursive: true });
    mkdirSync(join(home, 'account.json.lock'));
    const err = await login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 }).catch((e) => e);
    expect(err).toBeInstanceOf(CliError);
    expect(err.code).toBe('account_locked');
    expect(err.message).toContain('account.json.lock');
    expect(err.message).toContain('only when no account command is running');
  });

  it('rejects a token containing CR or LF', async () => {
    tokenBody = { ...tokenBody, access_token: 'at\r\nX-Evil: 1' };
    await expect(login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 }))
      .rejects.toMatchObject({ code: 'oauth_invalid' });
    expect(readAccount(home)?.accessToken).toBeUndefined();
  });
});

describe('lm logout', () => {
  it('removes the account', async () => {
    await login({ home, endpoints: endpoints(), openUrl: approve(), timeoutMs: 5000 });
    logout(home);
    expect(existsSync(join(home, 'account.json'))).toBe(false);
    expect(readAccount(home)).toBeNull();
  });
});

describe('accessToken', () => {
  it('refreshes when fewer than 30 seconds remain', async () => {
    mkdirSync(home, { recursive: true, mode: 0o700 });
    writeAccount(home, {
      clientId: 'cid-1', redirectUri: 'http://127.0.0.1:1/callback',
      accessToken: 'old', refreshToken: 'rt-old', expiresAt: new Date(Date.now() + 10_000).toISOString(),
    });
    tokenBody = { access_token: 'fresh', token_type: 'bearer', expires_in: 600 };
    expect(await accessToken(home, endpoints())).toBe('fresh');
    expect(lastTokenForm!.get('grant_type')).toBe('refresh_token');
    expect(lastTokenForm!.get('refresh_token')).toBe('rt-old');
    const saved = readAccount(home)!;
    expect(saved.accessToken).toBe('fresh');
    expect(saved.refreshToken).toBe('rt-old');
  });

  it('is not_signed_in without an account', async () => {
    await expect(accessToken(home, endpoints())).rejects.toMatchObject({ code: 'not_signed_in' });
  });
});

describe('browserCommand', () => {
  it('never routes the authorize link through a shell on Windows', async () => {
    const { browserCommand } = await import('../src/auth.js');
    expect(browserCommand('win32', 'https://x/a?b=1&c=2')).toBeNull();
    expect(browserCommand('darwin', 'https://x/a?b=1&c=2')).toEqual(['open', ['https://x/a?b=1&c=2']]);
    expect(browserCommand('linux', 'https://x/a?b=1&c=2')).toEqual(['xdg-open', ['https://x/a?b=1&c=2']]);
  });
});
