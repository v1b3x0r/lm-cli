import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { withAccountLock } from './account-store.js';
import { accessToken, HTTP_TIMEOUT_MS, type Endpoints } from './auth.js';
import { CliError } from './output.js';

export const ACCOUNT_MCP = 'https://lme.viibe.to/mcp';

/**
 * Call one tool on the account rail (LME's OAuth /mcp) and return its structuredContent.
 *
 * The bearer is attached here and only for `base` — the fetch refuses any other destination,
 * as the Go transport did. A 401 refreshes the token once and retries that one request.
 */
export async function callAccountTool<T>(
  home: string, name: string, args: Record<string, unknown>,
  opts: { base?: string; endpoints?: Partial<Endpoints>; timeoutMs?: number } = {},
): Promise<T> {
  const base = opts.base ?? ACCOUNT_MCP;
  return withAccountLock(home, async () => {
    let bearer = await accessToken(home, opts.endpoints);
    let signedOut = false;

    const accountFetch = async (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      const url = input instanceof Request ? input.url : String(input);
      if (url !== base) throw new CliError('refused_destination', 'account transport refuses non-account destination');
      const send = (token: string) => {
        const headers = new Headers(init?.headers);
        headers.set('Authorization', `Bearer ${token}`);
        // A stalled server must not hold the account lock: each request gets the deadline.
        const deadline = AbortSignal.timeout(opts.timeoutMs ?? HTTP_TIMEOUT_MS);
        const signal = init?.signal ? AbortSignal.any([init.signal, deadline]) : deadline;
        return fetch(url, { ...init, headers, redirect: 'manual', signal });
      };
      let res = await send(bearer);
      if (res.status === 401) {
        bearer = await accessToken(home, opts.endpoints, true);
        res = await send(bearer);
        if (res.status === 401) signedOut = true;
      }
      return res;
    };

    const client = new Client({ name: 'lm', version: '0.2.0' });
    try {
      try {
        await client.connect(new StreamableHTTPClientTransport(new URL(base), { fetch: accountFetch }));
      } catch (e) {
        if (e instanceof CliError) throw e;
        if (signedOut) throw new CliError('not_signed_in', 'account session expired; run lm login');
        throw new CliError('server_unreachable', 'cannot reach Living Memory; try again later');
      }
      // The tool must exist AND declare every argument we send: an MCP server strips unknown
      // arguments silently, so an older client_mint would drop access:'read' and mint a WRITE door.
      const { tools } = await client.listTools();
      const tool = tools.find((t) => t.name === name);
      const props = (tool?.inputSchema.properties ?? {}) as Record<string, { enum?: unknown[] }>;
      const understood = Object.entries(args).every(([k, v]) =>
        Object.hasOwn(props, k) && (!props[k]?.enum || props[k]!.enum!.includes(v)));
      if (!tool || !understood)
        throw new CliError('server_unsupported', 'this Living Memory server does not support that command yet');
      const result = await client.callTool({ name, arguments: args });
      // A refusal's text is a server response body, and the spec forbids echoing those: no
      // blocklist can prove it holds no URL or credential. The code says what happened; the
      // message says where to look.
      if (result.isError)
        throw new CliError('server_refused',
          'Living Memory refused that request (for example: the label is taken or not allowed, or the door limit is reached). Check with lm doors list.');
      return result.structuredContent as T;
    } catch (e) {
      if (e instanceof CliError) throw e;
      if (signedOut) throw new CliError('not_signed_in', 'account session expired; run lm login');
      throw new CliError('server_unreachable', 'cannot reach Living Memory; try again later');
    } finally {
      await client.close().catch(() => {});
    }
  });
}
