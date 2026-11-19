import { randomUUID } from 'node:crypto';
import { callAccountTool } from '../account-mcp.js';
import type { Context } from '../context.js';
import type { Result } from '../output.js';
import { safeText } from '../text.js';

interface CapsuleRow {
  capsule_id: string;
  name: string;
  role: 'owner' | 'controller' | 'contributor' | 'reader' | 'member';
  can_open_door: boolean;
  expires_at: string | null;
}

// LME's capsule tools wrap their payload as structuredContent: { result }.
const call = <T>(ctx: Context, name: string, args: Record<string, unknown>) =>
  callAccountTool<{ result: T }>(ctx.home, name, args, { ...ctx, surfaceRefusal: true }).then((r) => r.result);

function columns(rows: string[][]): string {
  const widths = rows[0]!.map((_, i) => Math.max(...rows.map((r) => r[i]!.length)));
  return rows.map((r) => r.map((c, i) => (i === r.length - 1 ? c : c.padEnd(widths[i]!))).join('  ')).join('\n');
}

export async function capsulesList(ctx: Context): Promise<Result> {
  const { capsules } = await call<{ capsules: CapsuleRow[] }>(ctx, 'capsule_list', {});
  const human = capsules.length
    ? columns([['ID', 'NAME', 'ROLE', 'DOOR', 'EXPIRES'], ...capsules.map((c) => [
      safeText(c.capsule_id), safeText(c.name), safeText(c.role), c.can_open_door ? 'yes' : 'no',
      safeText(c.expires_at ? c.expires_at.slice(0, 10) : '-')])])
    : 'No Capsules yet. Create one: lm capsules create <name> <note>';
  return { ok: true, data: { capsules }, human };
}

export async function capsulesCreate(ctx: Context, name: string, note: string): Promise<Result> {
  const r = await call<{ capsule_id: string; name: string }>(ctx, 'capsule_create', { name, content: note, request_id: randomUUID() });
  return { ok: true, data: { capsule_id: r.capsule_id, name: r.name }, human: `Created "${safeText(r.name)}"  ${safeText(r.capsule_id)}` };
}
