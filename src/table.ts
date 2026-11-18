import type { AccessRow } from './types.js';

/**
 * ls -l's visual language, not its semantics: r = can read, w = can write, third place always
 * '-'. It shows what the credential can do RIGHT NOW, so anything not effective is '---'.
 */
export function accessBits(row: AccessRow): 'rw-' | 'r--' | '---' {
  if (row.grant.effective !== true) return '---';
  return row.grant.access === 'read' ? 'r--' : 'rw-';
}

function relative(iso: string, now: number): string {
  const s = Math.max(0, (now - Date.parse(iso)) / 1000);
  if (s < 60) return '<1m ago';
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86_400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86_400)}d ago`;
}

export function lastUsedCell(row: AccessRow, now: number): string {
  if (row.grant.effective === null) return '?';
  if (row.grant.effective === false) return row.grant.status === 'active' ? 'inactive' : row.grant.status;
  return row.usage.lastUsedAt ? relative(row.usage.lastUsedAt, now) : '-';
}

const MIN_CLIENT = 6;

export function renderAccessTable(rows: AccessRow[], now: number, width: number): string {
  const cells = rows.map((r) => [accessBits(r), r.principal.kind, r.client.label, r.grant.scope.kind, lastUsedCell(r, now)]);
  const header = ['ACCESS', 'PRINCIPAL', 'CLIENT', 'SCOPE', 'LAST USED'];
  const all = [header, ...cells];
  const widths = header.map((_, i) => Math.max(...all.map((c) => c[i]!.length)));
  // Only CLIENT gives way on a narrow terminal: it is the one free-text column.
  const others = widths.reduce((sum, w, i) => (i === 2 ? sum : sum + w), 0) + 2 * (header.length - 1);
  widths[2] = Math.max(MIN_CLIENT, Math.min(widths[2]!, width - others));
  const fit = (s: string, w: number) => (s.length > w ? `${s.slice(0, w - 1)}…` : s);
  return all
    .map((c) => c.map((cell, i) => (i === c.length - 1 ? cell : fit(cell, widths[i]!).padEnd(widths[i]!))).join('  '))
    .join('\n');
}
