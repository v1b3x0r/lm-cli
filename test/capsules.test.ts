import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod';
import { writeAccount } from '../src/account-store.js';
import { main } from '../src/cli.js';
import { FakeLme, ok } from './fake-lme.js';

const ID_A = 'w_' + 'a'.repeat(32);
const ID_B = 'w_' + 'b'.repeat(32);
const ROWS = [
  { capsule_id: ID_A, name: 'Trip', role: 'owner', can_open_door: true, expires_at: '2026-12-10T00:00:00.000Z' },
  { capsule_id: ID_B, name: 'Shared', role: 'reader', can_open_door: false, expires_at: '2026-12-11T00:00:00.000Z' },
];
let lme: FakeLme;
let home: string;
let out: string[];
let err: string[];

async function lm(...argv: string[]) {
  out = []; err = [];
  const code = await main(argv, { home, base: lme.mcpUrl, endpoints: lme.endpoints });
  return { code, out: out.join(''), err: err.join('') };
}

beforeEach(async () => {
  home = mkdtempSync(join(tmpdir(), 'lm-capsules-'));
  lme = new FakeLme();
  await lme.start();
  lme.schemas = {
    capsule_create: { name: z.string(), content: z.string(), request_id: z.string() },
    capsule_list: {},
    world_list: {},
  };
  lme.tools = {
    capsule_list: () => ok({ result: { capsules: ROWS } }),
    capsule_create: (a) => ok({ result: { capsule_id: ID_A, name: a.name, stored: true } }),
    world_list: () => ok({ worlds: [] }),
  };
  writeAccount(home, {
    clientId: 'cid', redirectUri: 'http://127.0.0.1:1/callback', accessToken: 'at-good', refreshToken: 'rt',
    expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
  });
  vi.spyOn(process.stdout, 'write').mockImplementation((s) => (out.push(String(s)), true));
  vi.spyOn(process.stderr, 'write').mockImplementation((s) => (err.push(String(s)), true));
});

afterEach(async () => {
  vi.restoreAllMocks();
  await lme.stop();
});

describe('lm capsules list', () => {
  it('renders one row per Capsule', async () => {
    const r = await lm('capsules', 'list');
    expect(r.code).toBe(0);
    const lines = r.out.trimEnd().split('\n');
    expect(lines[0]!.split(/\s+/)).toEqual(['ID', 'NAME', 'ROLE', 'DOOR', 'EXPIRES']);
    expect(lines[1]).toMatch(new RegExp(`^${ID_A}\\s+Trip\\s+owner\\s+yes\\s+2026-12-10$`));
    expect(lines[2]).toMatch(new RegExp(`^${ID_B}\\s+Shared\\s+reader\\s+no\\s+2026-12-11$`));
  });

  it('--json returns the capsules unchanged', async () => {
    const r = await lm('capsules', 'list', '--json');
    expect(JSON.parse(r.out).data).toEqual({ capsules: ROWS });
  });

  it('says how to create the first one when empty', async () => {
    lme.tools.capsule_list = () => ok({ result: { capsules: [] } });
    const r = await lm('capsules', 'list');
    expect(r.out.trim()).toBe('No Capsules yet. Create one: lm capsules create <name> <note>');
  });
});

describe('lm capsules create', () => {
  it('sends name and note and prints the new ID', async () => {
    const r = await lm('capsules', 'create', 'Trip', 'Packing list');
    expect(r.code).toBe(0);
    expect(r.out).toContain(ID_A);
    const call = lme.calls.find((c) => c.name === 'capsule_create')!;
    expect(call.args).toMatchObject({ name: 'Trip', content: 'Packing list' });
  });

  it('uses a fresh request_id per invocation', async () => {
    await lm('capsules', 'create', 'Trip', 'one');
    await lm('capsules', 'create', 'Trip', 'one');
    const ids = lme.calls.filter((c) => c.name === 'capsule_create').map((c) => c.args.request_id);
    expect(ids).toHaveLength(2);
    expect(ids[0]).toBeTruthy();
    expect(ids[0]).not.toBe(ids[1]);
  });

  it('surfaces an LME refusal with its own text', async () => {
    lme.tools.capsule_create = () => ({ isError: true, content: [{ type: 'text', text: 'Free Room limit reached.' }] });
    const r = await lm('capsules', 'create', 'Trip', 'note');
    expect(r.code).toBe(1);
    expect(r.err).toContain('Free Room limit reached.');
  });

  it('strips terminal escapes from refusal text and caps its length', async () => {
    lme.tools.capsule_create = () => ({ isError: true, content: [{ type: 'text', text: `bad\u001b[2J\u0007${'x'.repeat(500)}` }] });
    const r = await lm('capsules', 'create', 'Trip', 'note');
    expect(r.err).not.toContain('\u001b');
    expect(r.err).not.toContain('\u0007');
    expect(r.err.length).toBeLessThan(320);
  });

  it('needs exactly a name and a note', async () => {
    expect((await lm('capsules', 'create', 'Trip')).code).toBe(2);
    expect((await lm('capsules', 'create')).code).toBe(2);
  });
});

describe('lm whoami on a Free Room account', () => {
  it('prints the room LME lists as the default World', async () => {
    lme.tools.world_list = () => ok({ worlds: [{ id: ID_A, title: 'Free Room', isDefault: true }] });
    const r = await lm('whoami');
    expect(r.code).toBe(0);
    expect(r.out).toContain('Free Room');
    expect(r.out).toContain(ID_A);
    expect(r.err).toBe('');
  });
});
