import { mkdtempSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeAccount } from '../src/account-store.js';
import { main } from '../src/cli.js';
import { z } from 'zod';
import { FakeLme, ok } from './fake-lme.js';

const CAP = 'w_0123456789abcdef0123456789abcdef';
const DID = 'acc_0123456789abcdef';
const DOOR_URL = 'https://lme.example/t/lmc_0123456789abcdef0123456789abcdef/mcp';
const DOORS = [
  { door_id: DID, holder: 'you', label: 'lmstudio', access: 'read', status: 'live', revocable: true,
    created_at: '2026-11-18T00:00:00.000Z', expires_at: '2027-02-16T00:00:00.000Z', last_used_at: '2026-11-19T09:00:00.000Z' },
  { door_id: 'pakkana', holder: 'Pakkana (custodian)', label: null, access: 'write', status: 'live', revocable: false,
    created_at: null, expires_at: null, last_used_at: null },
  { door_id: 'u_1', holder: 'unclaimed read link', label: null, access: 'read', status: 'live', revocable: true,
    created_at: '2026-11-01T00:00:00.000Z', expires_at: '2027-01-30T00:00:00.000Z', last_used_at: null },
];
let lme: FakeLme;
let home: string;
let out: string[];
let err: string[];

function signIn(accessToken: string, expiresInMs = 3_600_000) {
  writeAccount(home, {
    clientId: 'cid', redirectUri: 'http://127.0.0.1:1/callback', accessToken, refreshToken: 'rt',
    expiresAt: new Date(Date.now() + expiresInMs).toISOString(),
  });
}

async function lm(...argv: string[]) {
  out = []; err = [];
  const code = await main(argv, { home, base: lme.mcpUrl, endpoints: lme.endpoints });
  return { code, out: out.join(''), err: err.join('') };
}

beforeEach(async () => {
  home = mkdtempSync(join(tmpdir(), 'lm-doors-'));
  lme = new FakeLme();
  await lme.start();
  lme.tools = {
    world_list: () => ok({ worlds: [{ id: 'w_abc', title: 'Founder', isDefault: true }] }),
    // LME's door tools return their payload unwrapped (no { result } envelope).
    capsule_door_open: (a) => ok({ door_id: DID, label: a.label, access: 'read', url: DOOR_URL, expires_at: '2027-02-16T00:00:00.000Z', capsule_id: a.capsule_id }),
    capsule_door_list: (a) => ok({ capsule_id: a.capsule_id, doors: DOORS }),
    capsule_door_revoke: (a) => ok({ revoked: true, door_id: a.door_id }),
  };
  lme.schemas = {
    capsule_door_open: { capsule_id: z.string(), label: z.string() },
    capsule_door_list: { capsule_id: z.string() },
    capsule_door_revoke: { capsule_id: z.string(), door_id: z.string() },
  };
  signIn('at-good');
  vi.spyOn(process.stdout, 'write').mockImplementation((s) => (out.push(String(s)), true));
  vi.spyOn(process.stderr, 'write').mockImplementation((s) => (err.push(String(s)), true));
});

afterEach(async () => {
  vi.restoreAllMocks();
  await lme.stop();
});

describe('lm whoami', () => {
  it('shows the default World', async () => {
    const r = await lm('whoami', '--json');
    expect(r.code).toBe(0);
    expect(JSON.parse(r.out).data).toEqual({ world: { id: 'w_abc', title: 'Founder' } });
  });

  it('never puts the bearer or a door url in an error', async () => {
    lme.tools.world_list = () => ({ content: [{ type: 'text', text: `refused ${DOOR_URL} for at-good` }], isError: true });
    for (const argv of [['whoami', '--json'], ['whoami']]) {
      const r = await lm(...argv);
      expect(r.code).toBe(1);
      expect(r.out + r.err).not.toContain(DOOR_URL);
      expect(r.out + r.err).not.toContain('lmc_');
      expect(r.out + r.err).not.toContain('at-good');
    }
  });

  it('reports a server refusal on world_list with fixed text, never the server body', async () => {
    lme.tools.world_list = () => ({ content: [{ type: 'text', text: 'Refused LME.VIIBE.TO/t/LMC_0123456789ABCDEF/mcp sk-live-9f8e7d6c5b4a' }], isError: true });
    const r = await lm('whoami', '--json');
    const e = JSON.parse(r.out).error;
    expect(e.code).toBe('server_refused');
    expect(e.message).toBe('Living Memory refused that request (for example: the label is taken or not allowed, or the door limit is reached).');
    expect(r.out + r.err).not.toMatch(/LMC_|sk-live|VIIBE|Refused/);
  });
});

describe('lm doors', () => {
  it('without a Capsule ID is a usage error that names lm capsules list', async () => {
    for (const argv of [['doors'], ['doors', 'list']]) {
      const r = await lm(...argv);
      expect(r.code).toBe(2);
      expect(r.err).toContain('lm capsules list');
    }
    expect(lme.calls).toEqual([]);
  });

  it('open without a label is a usage error and calls nothing', async () => {
    const r = await lm('doors', 'open', CAP);
    expect(r.code).toBe(2);
    expect(lme.calls).toEqual([]);
  });

  it('open sends capsule_id and label', async () => {
    await lm('doors', 'open', CAP, 'lmstudio');
    expect(lme.calls).toContainEqual({ name: 'capsule_door_open', args: { capsule_id: CAP, label: 'lmstudio' } });
  });

  it('open prints the url exactly once, then the key copy', async () => {
    const r = await lm('doors', 'open', CAP, 'lmstudio');
    expect(r.code).toBe(0);
    expect(r.out.split(DOOR_URL).length - 1).toBe(1);
    expect(r.err).not.toContain(DOOR_URL);
    expect(r.out).toContain('This link is a key: whoever holds it can read this Capsule\'s memory until it expires or you run "lm doors revoke <capsule-id> <door-id>".');
    const j = await lm('doors', 'open', CAP, 'lmstudio', '--json');
    expect(j.out.split(DOOR_URL).length - 1).toBe(1);
    expect(JSON.parse(j.out).data).toEqual({ door_id: DID, label: 'lmstudio', access: 'read', url: DOOR_URL, expires_at: '2027-02-16T00:00:00.000Z', capsule_id: CAP });
  });

  it('list shows the columns, the custodian and unclaimed rows, with - for empties', async () => {
    const r = await lm('doors', 'list', CAP);
    expect(r.code).toBe(0);
    const lines = r.out.trim().split('\n');
    expect(lines[0]).toMatch(/^DOOR\s+HOLDER\s+LABEL\s+ACCESS\s+STATUS\s+LAST USED\s+EXPIRES$/);
    expect(r.out).toMatch(/acc_0123456789abcdef\s+you\s+lmstudio\s+read\s+live\s+2026-11-19\s+2027-02-16/);
    expect(r.out).toMatch(/pakkana\s+Pakkana \(custodian\)\s+-\s+write\s+live\s+-\s+-/);
    expect(r.out).toMatch(/u_1\s+unclaimed read link\s+-\s+read\s+live\s+-\s+2027-01-30/);
    expect(lme.calls).toContainEqual({ name: 'capsule_door_list', args: { capsule_id: CAP } });
  });

  it('revoke success copy echoes only the validated door id', async () => {
    const r = await lm('doors', 'revoke', CAP, DID);
    expect(r.code).toBe(0);
    expect(r.out).toContain(`Revoked ${DID}. That link no longer opens this Capsule.`);
  });

  it('bad ids are usage errors, never echoed, and never reach the server', async () => {
    const cases = [
      ['doors', 'list', 'nope'], ['doors', 'list', DOOR_URL], ['doors', 'open', DOOR_URL, 'x'],
      ['doors', 'revoke', 'nope', DID], ['doors', 'revoke', DOOR_URL, DID],
      ['doors', 'revoke', CAP, 'nope'], ['doors', 'revoke', CAP, DOOR_URL],
    ];
    for (const argv of cases) for (const json of [[], ['--json']]) {
      const r = await lm(...argv, ...json);
      expect(r.code, argv.join(' ')).toBe(2);
      expect(r.out + r.err).not.toContain('lmc_');
      expect(r.out + r.err).not.toContain('nope');
      expect(r.err + r.out).toMatch(/lm capsules list|lm doors list <capsule-id>/);
    }
    expect(lme.calls).toEqual([]);
  });

  it('open prints no URL unless the door is read with an https url', async () => {
    for (const bad of [{ access: 'write', url: DOOR_URL }, { access: 'read', url: 'http://evil.example/x' }, { access: 'read', url: 42 }]) {
      lme.tools.capsule_door_open = (a) => ({ content: [{ type: 'text', text: 'x' }], structuredContent: { door_id: DID, label: a.label, expires_at: '2027-01-01', capsule_id: a.capsule_id, ...bad } });
      const r = await lm('doors', 'open', CAP, 'x');
      expect(r.code).toBe(1);
      expect(r.out + r.err).not.toContain('evil.example');
      expect(r.out + r.err).not.toContain('lmc_');
    }
  });

  it('revoke needs revoked true and the same door id back', async () => {
    for (const bad of [{ revoked: false, door_id: DID }, { revoked: true, door_id: 'acc_ffffffffffffffff' }, { revoked: true, door_id: DOOR_URL }]) {
      lme.tools.capsule_door_revoke = () => ok(bad);
      const r = await lm('doors', 'revoke', CAP, DID);
      expect(r.code).toBe(1);
      expect(r.out).not.toContain('Revoked');
      expect(r.out + r.err).not.toContain('lmc_');
    }
  });

  it('shows LME\'s refusal text', async () => {
    lme.tools.capsule_door_list = () => ({ content: [{ type: 'text', text: 'Capsule unavailable for this account.' }], isError: true });
    const r = await lm('doors', 'list', CAP);
    expect(r.code).toBe(1);
    expect(r.err).toContain('Capsule unavailable for this account.');
  });

  it('strips control characters from server text', async () => {
    lme.tools.capsule_door_list = (a) => ok({ capsule_id: a.capsule_id, doors: [{ ...DOORS[0], label: 'x\r\ny\x1b]0;pwned\x07', holder: 'a\tb' }] });
    const r = await lm('doors', 'list', CAP);
    expect(r.out).not.toMatch(/[\u0000-\u0009\u000b-\u001f\u007f-\u009f]/);
    expect(r.out.trim().split('\n')).toHaveLength(2);
  });

  it('refreshes once on 401, then retries', async () => {
    signIn('at-stale');
    const r = await lm('doors', 'list', CAP, '--json');
    expect(r.code).toBe(0);
    expect(lme.refreshes).toBe(1);
  });

  it('is not_signed_in when refresh cannot help', async () => {
    signIn('at-stale');
    lme.refreshTo = null;
    const r = await lm('doors', 'list', CAP, '--json');
    expect(JSON.parse(r.out).error.code).toBe('not_signed_in');
  });

  it('maps a missing tool to server_unsupported (an older server has none of the door tools)', async () => {
    for (const t of ['capsule_door_open', 'capsule_door_list', 'capsule_door_revoke']) delete lme.tools[t];
    const r = await lm('doors', 'open', CAP, 'x', '--json');
    expect(r.code).toBe(1);
    expect(JSON.parse(r.out).error.code).toBe('server_unsupported');
  });

  // The lapsed-World mount lists and revokes doors but has no capsule_door_open and no world_list.
  it.each([
    [['doors', 'open', CAP, 'x']],
    [['whoami']],
  ])('says the World lapsed, not "unsupported", for lm %s on a lapsed mount', async (argv) => {
    delete lme.tools.capsule_door_open;
    delete lme.tools.world_list;
    const r = await lm(...argv, '--json');
    expect(r.code).toBe(1);
    const err = JSON.parse(r.out).error;
    expect(err.code).toBe('world_lapsed');
    expect(err.message).toMatch(/lapsed/);
    expect(err.message).toMatch(/list and revoke/);
    expect(err.message).not.toMatch(/does not support/);
  });
});

describe('the World door is gone', () => {
  it('lm access list is a usage error', async () => {
    const r = await lm('access', 'list');
    expect(r.code).toBe(2);
  });

  it('--read is no longer a flag', async () => {
    expect((await lm('doors', 'open', CAP, 'x', '--read')).code).toBe(2);
  });

  it('src calls none of the World door tools', () => {
    const files = readdirSync('src', { recursive: true, encoding: 'utf8' }).filter((f) => f.endsWith('.ts'));
    for (const f of files) {
      const text = readFileSync(join('src', f), 'utf8');
      for (const name of ['client_mint', 'client_list', 'client_revoke', 'access_list']) expect(text, `${f} ${name}`).not.toContain(name);
    }
  });
});

describe('a Living Memory server that never answers', () => {
  it('gives up, reports server_unreachable, and releases the lock', async () => {
    lme.hang = true;
    out = []; err = [];
    const code = await main(['doors', 'list', CAP, '--json'], { home, base: lme.mcpUrl, endpoints: lme.endpoints, timeoutMs: 300 });
    expect(code).toBe(1);
    expect(JSON.parse(out.join('')).error.code).toBe('server_unreachable');
    lme.hang = false;
    expect((await lm('doors', 'list', CAP)).code).toBe(0);
  });
});
