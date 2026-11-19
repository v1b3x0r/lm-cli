import { callAccountTool } from '../account-mcp.js';
import type { Context } from '../context.js';
import { CliError, type Result } from '../output.js';
import { safeText } from '../text.js';

interface World { id: string; title: string | null; isDefault: boolean }
interface DoorRow {
  door_id: string; holder: string; label: string | null; access: 'read' | 'write';
  status: 'live' | 'expired' | 'revoked'; revocable: boolean;
  created_at: string | null; expires_at: string | null; last_used_at: string | null;
}

const day = (iso: string | null) => (iso ? iso.slice(0, 10) : '-');

// Unlike the capsule tools, LME's door tools return their payload unwrapped. Every refusal is
// LME's own fixed text, so callers opt in to showing it.
const call = <T>(ctx: Context, name: string, args: Record<string, unknown>) =>
  callAccountTool<T>(ctx.home, name, args, { ...ctx, surfaceRefusal: true });

function columns(rows: string[][]): string {
  const widths = rows[0]!.map((_, i) => Math.max(...rows.map((r) => r[i]!.length)));
  return rows.map((r) => r.map((c, i) => (i === r.length - 1 ? c : c.padEnd(widths[i]!))).join('  ')).join('\n');
}

export async function whoami(ctx: Context): Promise<Result> {
  const { worlds } = await callAccountTool<{ worlds: World[] }>(ctx.home, 'world_list', {}, ctx);
  const w = worlds.find((x) => x.isDefault) ?? worlds[0];
  if (!w) throw new CliError('no_world', 'this account has no World yet');
  return { ok: true, data: { world: { id: w.id, title: w.title } }, human: `${w.title === null ? '(untitled)' : safeText(w.title)}  ${safeText(w.id)}` };
}

export async function doorsList(ctx: Context, capsuleId: string): Promise<Result> {
  const r = await call<{ capsule_id: string; doors: DoorRow[] }>(ctx, 'capsule_door_list', { capsule_id: capsuleId });
  const human = r.doors.length
    ? columns([['DOOR', 'HOLDER', 'LABEL', 'ACCESS', 'STATUS', 'LAST USED', 'EXPIRES'], ...r.doors.map((d) => [
      safeText(d.door_id), safeText(d.holder), safeText(d.label ?? '-'), safeText(d.access), safeText(d.status),
      safeText(day(d.last_used_at)), safeText(day(d.expires_at))])])
    : 'No doors on this Capsule.';
  return { ok: true, data: r, human };
}

export async function doorsOpen(ctx: Context, capsuleId: string, label: string): Promise<Result> {
  const r = await call<{ door_id: string; label: string; access: 'read'; url: string; expires_at: string; capsule_id: string }>(
    ctx, 'capsule_door_open', { capsule_id: capsuleId, label });
  // The ONE place a door URL is ever printed. Never echo it anywhere else.
  const human = `${safeText(r.url)}\nThis link is a key: whoever holds it can read this Capsule's memory until it expires or you run "lm doors revoke <capsule-id> <door-id>".`;
  return { ok: true, data: r, human };
}

export async function doorsRevoke(ctx: Context, capsuleId: string, doorId: string): Promise<Result> {
  const r = await call<{ revoked: true; door_id: string }>(ctx, 'capsule_door_revoke', { capsule_id: capsuleId, door_id: doorId });
  // Never echo an argument that looks like a link: people paste the door URL itself here.
  if (/^https?:/i.test(capsuleId) || /^https?:/i.test(doorId)) return { ok: true, data: { revoked: true }, human: 'Revoked.' };
  return { ok: true, data: r, human: `Revoked ${safeText(r.door_id)}. That link no longer opens this Capsule.` };
}
