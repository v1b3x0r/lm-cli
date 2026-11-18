import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { CliError, type Result } from '../output.js';
import { safeText } from '../text.js';

/** A tool that writes, deletes or posts. A door without any of them is read-only. */
const WRITE_TOOLS = ['memory_add', 'memory_forget', 'handoff_post'];

/**
 * Only an LME door shape is accepted, so `doctor` cannot be aimed at an arbitrary host.
 * Loopback http is allowed for local testing only.
 */
function doorUrl(raw: string): URL {
  let u: URL;
  try { u = new URL(raw); } catch { throw new CliError('usage', 'that is not a door link (expected https://…/t/<key>/mcp)', 2); }
  const doorPath = /^\/t\/[A-Za-z0-9_-]+\/mcp$/.test(u.pathname);
  const allowed = u.protocol === 'https:' || (u.protocol === 'http:' && u.hostname === '127.0.0.1');
  if (!doorPath || !allowed || u.username || u.password || u.search || u.hash)
    throw new CliError('usage', 'that is not a door link (expected https://…/t/<key>/mcp)', 2);
  return u;
}

export async function doctorDoor(raw: string): Promise<{ reachable: boolean; readOnly: boolean; tools: string[] }> {
  const url = doorUrl(raw);
  const client = new Client({ name: 'lm-doctor', version: '0.2.0' });
  try {
    await client.connect(new StreamableHTTPClientTransport(url, {
      fetch: (input, init) => fetch(input, { ...init, redirect: 'manual' }),
    }));
    const tools = (await client.listTools()).tools.map((t) => t.name).sort();
    return { reachable: true, readOnly: !tools.some((t) => WRITE_TOOLS.includes(t)), tools };
  } catch {
    return { reachable: false, readOnly: false, tools: [] };
  } finally {
    await client.close().catch(() => {});
  }
}

/** Never prints the door back: the link is a key, and terminals get recorded and shared. */
export async function doctor(raw: string): Promise<Result> {
  const r = await doctorDoor(raw);
  if (!r.reachable)
    throw new CliError('door_unreachable', 'that door did not answer: it may be revoked, expired or mistyped');
  const human = [
    'Door: reachable',
    `Read-only: ${r.readOnly ? 'yes (no tool can write, delete or post)' : 'NO — this door can change memory'}`,
    `Tools: ${r.tools.map(safeText).join(', ')}`,
  ].join('\n');
  return { ok: true, data: r, human };
}
