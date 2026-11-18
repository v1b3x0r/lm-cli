import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { withAccountLock } from './account-store.js';
import { accessToken, type Endpoints } from './auth.js';
import { CliError } from './output.js';

export const ACCOUNT_MCP = 'https://lme.viibe.to/mcp';

/** Remove anything that could be a door URL or credential from text a server sent us. */
export function redact(text: string, secrets: string[] = []): string {
  for (const s of secrets) if (s) text = text.split(s).join('<credential>');
  return text
    .replace(/https?:\/\/\S+/g, '<url>')
    .replace(/\b(?:lm[a-z]?|ro|ons)_[A-Za-z0-9]{8,}\b/g, '<credential>')
    .slice(0, 300);
}

/**
 * Call one tool on the account rail (LME's OAuth /mcp) and return its structuredContent.
 *
 * The bearer is attached here and only for `base` — the fetch refuses any other destination,
 * as the Go transport did. A 401 refreshes the token once and retries that one request.
 */
export async function callAccountTool<T>(
  home: string, name: string, args: Record<string, unknown>,
  opts: { base?: string; endpoints?: Partial<Endpoints> } = {},
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
        return fetch(url, { ...init, headers, redirect: 'manual' });
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
      const { tools } = await client.listTools();
      if (!tools.some((t) => t.name === name))
        throw new CliError('server_unsupported', 'this Living Memory server does not support that command yet');
      const result = await client.callTool({ name, arguments: args });
      if (result.isError) {
        const text = (result.content as Array<{ type: string; text?: string }>).find((c) => c.type === 'text')?.text ?? 'refused';
        throw new CliError('server_refused', redact(text, [bearer]));
      }
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
