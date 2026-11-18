import { callAccountTool } from '../account-mcp.js';
import type { Context } from '../context.js';
import { CliError, type Result } from '../output.js';
import { safeText } from '../text.js';

interface World { id: string; title: string | null; isDefault: boolean }
interface ClientRow { label: string; createdAt: string | null; expiresAt: string | null; access?: 'read' | 'write' }

const day = (iso: string | null) => (iso ? iso.slice(0, 10) : '-');

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

export async function doorsList(ctx: Context): Promise<Result> {
  const { clients } = await callAccountTool<{ clients: ClientRow[] }>(ctx.home, 'client_list', {}, ctx);
  const doors = clients.map((c) => ({ label: c.label, access: c.access ?? 'write', createdAt: c.createdAt, expiresAt: c.expiresAt }));
  const human = doors.length
    ? columns([['LABEL', 'ACCESS', 'CREATED', 'EXPIRES'], ...doors.map((d) => [safeText(d.label), safeText(d.access), safeText(day(d.createdAt)), safeText(day(d.expiresAt))])])
    : 'No doors are open.';
  return { ok: true, data: { doors }, human };
}

export async function doorsOpen(ctx: Context, label: string, read: boolean): Promise<Result> {
  if (!read) throw new CliError('usage', 'v1 opens read-only doors only: add --read', 2);
  const m = await callAccountTool<{ label: string; url: string; expiresAt: string | null }>(
    ctx.home, 'client_mint', { label, access: 'read' }, ctx);
  // The ONE place a door URL is ever printed. Never echo it anywhere else.
  const human = `${safeText(m.url)}\nThis link is a key: whoever holds it can read everything in this World until it expires or you run "lm doors revoke ${safeText(m.label)}".`;
  return { ok: true, data: { label: m.label, url: m.url, expiresAt: m.expiresAt }, human };
}

export async function doorsRevoke(ctx: Context, label: string): Promise<Result> {
  const { revoked } = await callAccountTool<{ revoked: number }>(ctx.home, 'client_revoke', { label }, ctx);
  if (!revoked) throw new CliError('not_found', `no door named "${label}"`);
  return { ok: true, data: { label, revoked }, human: `Revoked "${safeText(label)}". That link no longer opens this World.` };
}
