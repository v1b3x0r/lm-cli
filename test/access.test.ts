import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeAccount } from '../src/account-store.js';
import { main } from '../src/cli.js';
import { FakeLme, ok } from './fake-lme.js';

const ROWS = [{
  id: 'acc_0011223344556677',
  principal: { kind: 'human', evidence: 'verified' },
  client: { label: 'owner', rail: 'oauth' },
  grant: { scope: { kind: 'world', id: 'w_abc' }, access: 'owner', status: 'active', effective: true,
    createdAt: null, expiresAt: null, revocable: false, note: null },
  usage: { lastUsedAt: '2026-11-18T11:00:00.000Z' },
}];

let lme: FakeLme;
let home: string;
let out: string[];

beforeEach(async () => {
  home = mkdtempSync(join(tmpdir(), 'lm-access-'));
  lme = new FakeLme();
  await lme.start();
  lme.tools = { access_list: () => ok({ access: ROWS }) };
  writeAccount(home, { clientId: 'c', redirectUri: 'http://127.0.0.1:1/callback', accessToken: 'at-good',
    refreshToken: 'rt', expiresAt: new Date(Date.now() + 3_600_000).toISOString() });
  out = [];
  vi.spyOn(process.stdout, 'write').mockImplementation((s) => (out.push(String(s)), true));
  vi.spyOn(process.stderr, 'write').mockImplementation(() => true);
});

afterEach(async () => {
  vi.restoreAllMocks();
  await lme.stop();
});

describe('lm access list', () => {
  it('passes the server array through unchanged inside the json envelope', async () => {
    const code = await main(['access', 'list', '--json'], { home, base: lme.mcpUrl, endpoints: lme.endpoints });
    expect(code).toBe(0);
    expect(JSON.parse(out.join(''))).toEqual({ ok: true, data: ROWS });
  });

  it('renders the table by default', async () => {
    await main(['access', 'list'], { home, base: lme.mcpUrl, endpoints: lme.endpoints });
    expect(out.join('')).toMatch(/^ACCESS  PRINCIPAL  CLIENT  SCOPE  LAST USED\nrw-     human      owner   world  /);
  });
});
