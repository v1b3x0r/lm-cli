import { describe, expect, it } from 'vitest';
import { accessBits, lastUsedCell, renderAccessTable } from '../src/table.js';
import type { AccessRow } from '../src/types.js';

const NOW = Date.parse('2026-11-18T12:00:00.000Z');
const ago = (ms: number) => new Date(NOW - ms).toISOString();

function row(p: {
  kind: AccessRow['principal']['kind']; label: string; access: AccessRow['grant']['access'];
  status?: AccessRow['grant']['status']; effective?: boolean | null; lastUsedAt?: string | null;
}): AccessRow {
  return {
    id: `acc_${p.label}`,
    principal: { kind: p.kind, evidence: 'verified' },
    client: { label: p.label, rail: 'token' },
    grant: {
      scope: { kind: 'world', id: 'w_abc' }, access: p.access, status: p.status ?? 'active',
      effective: p.effective === undefined ? true : p.effective,
      createdAt: null, expiresAt: null, revocable: true, note: null,
    },
    usage: { lastUsedAt: p.lastUsedAt ?? null },
  };
}

// The four rows of the spec's mock, in its order.
const SPEC_ROWS = [
  row({ kind: 'human', label: 'owner', access: 'owner', lastUsedAt: ago(2 * 60_000) }),
  row({ kind: 'holder', label: 'lmstudio-proof', access: 'read', lastUsedAt: ago(60 * 60_000) }),
  row({ kind: 'service', label: 'pakkana', access: 'write', lastUsedAt: ago(5 * 60_000) }),
  row({ kind: 'holder', label: 'old-agent', access: 'read', status: 'revoked', effective: false }),
];

describe('accessBits', () => {
  it('shows the effective capability only', () => {
    expect(accessBits(row({ kind: 'human', label: 'o', access: 'owner' }))).toBe('rw-');
    expect(accessBits(row({ kind: 'holder', label: 'w', access: 'write' }))).toBe('rw-');
    expect(accessBits(row({ kind: 'holder', label: 'r', access: 'read' }))).toBe('r--');
    expect(accessBits(row({ kind: 'holder', label: 'x', access: 'write', effective: false }))).toBe('---');
    expect(accessBits(row({ kind: 'holder', label: 'n', access: 'write', effective: null }))).toBe('---');
  });
});

describe('lastUsedCell', () => {
  it('uses the status word when not effective, inactive for a lapsed active grant', () => {
    expect(lastUsedCell(row({ kind: 'holder', label: 'a', access: 'read', status: 'revoked', effective: false }), NOW)).toBe('revoked');
    expect(lastUsedCell(row({ kind: 'holder', label: 'a', access: 'read', status: 'expired', effective: false }), NOW)).toBe('expired');
    expect(lastUsedCell(row({ kind: 'agent', label: 'a', access: 'write', status: 'suspended', effective: false }), NOW)).toBe('suspended');
    expect(lastUsedCell(row({ kind: 'holder', label: 'a', access: 'read', status: 'active', effective: false }), NOW)).toBe('inactive');
  });

  it('shows ? when the server could not evaluate', () => {
    expect(lastUsedCell(row({ kind: 'agent', label: 'a', access: 'write', effective: null, lastUsedAt: ago(1000) }), NOW)).toBe('?');
  });

  it('shows a relative time, or - when never used', () => {
    const at = (ms: number) => lastUsedCell(row({ kind: 'holder', label: 'a', access: 'read', lastUsedAt: ago(ms) }), NOW);
    expect(at(30_000)).toBe('<1m ago');
    expect(at(2 * 60_000)).toBe('2m ago');
    expect(at(60 * 60_000)).toBe('1h ago');
    expect(at(3 * 86_400_000)).toBe('3d ago');
    expect(lastUsedCell(row({ kind: 'holder', label: 'a', access: 'read' }), NOW)).toBe('-');
  });
});

describe('renderAccessTable', () => {
  it('renders the spec example rows exactly', () => {
    expect(renderAccessTable(SPEC_ROWS, NOW, 120)).toBe([
      'ACCESS  PRINCIPAL  CLIENT          SCOPE  LAST USED',
      'rw-     human      owner           world  2m ago',
      'r--     holder     lmstudio-proof  world  1h ago',
      'rw-     service    pakkana         world  5m ago',
      '---     holder     old-agent       world  revoked',
    ].join('\n'));
  });

  it('truncates the client column so no row exceeds the width', () => {
    const long = row({ kind: 'holder', label: 'a-very-long-client-label-that-would-wrap', access: 'read', lastUsedAt: ago(60_000) });
    const text = renderAccessTable([long], NOW, 50);
    for (const line of text.split('\n')) expect(line.length).toBeLessThanOrEqual(50);
    expect(text).toContain('…');
  });
});
