import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeAccount } from '../src/account-store.js';
import { main } from '../src/cli.js';
import { z } from 'zod';
import { FakeLme, ok } from './fake-lme.js';

const DOOR_URL = 'https://lme.example/t/lmc_0123456789abcdef0123456789abcdef/mcp';
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
    client_mint: (a) => ok({ label: a.label, url: DOOR_URL, expiresAt: '2027-02-16T00:00:00.000Z', message: `minted ${DOOR_URL}` }),
    client_list: () => ok({ clients: [
      { label: 'codex', createdAt: '2026-11-01T00:00:00.000Z', expiresAt: '2027-01-30T00:00:00.000Z', access: 'write' },
      { label: 'lmstudio-proof', createdAt: '2026-11-18T00:00:00.000Z', expiresAt: '2027-02-16T00:00:00.000Z', access: 'read' },
    ] }),
    client_revoke: (a) => ok({ revoked: a.label === 'lmstudio-proof' ? 1 : 0 }),
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
});

describe('lm doors', () => {
  it('open sends access read', async () => {
    await lm('doors', 'open', 'lmstudio-proof', '--read');
    expect(lme.calls).toContainEqual({ name: 'client_mint', args: { label: 'lmstudio-proof', access: 'read' } });
  });

  it('open without --read exits 2 and calls nothing', async () => {
    const r = await lm('doors', 'open', 'x');
    expect(r.code).toBe(2);
    expect(r.err).toContain('v1 opens read-only doors only: add --read');
    expect(lme.calls).toEqual([]);
  });

  it('open prints the url exactly once, then the warning', async () => {
    const r = await lm('doors', 'open', 'lmstudio-proof', '--read');
    expect(r.code).toBe(0);
    expect(r.out.split(DOOR_URL).length - 1).toBe(1);
    expect(r.err).not.toContain(DOOR_URL);
    expect(r.out).toContain('This link is a key: whoever holds it can read everything in this World until it expires or you run "lm doors revoke lmstudio-proof".');
    const j = await lm('doors', 'open', 'lmstudio-proof', '--read', '--json');
    expect(JSON.parse(j.out).data).toEqual({ label: 'lmstudio-proof', url: DOOR_URL, expiresAt: '2027-02-16T00:00:00.000Z' });
  });

  it('list maps label, access, created, expires', async () => {
    const r = await lm('doors', 'list');
    expect(r.code).toBe(0);
    expect(r.out).toMatch(/LABEL\s+ACCESS\s+CREATED\s+EXPIRES/);
    expect(r.out).toMatch(/lmstudio-proof\s+read\s+2026-11-18\s+2027-02-16/);
    const j = await lm('doors', 'list', '--json');
    expect(JSON.parse(j.out).data.doors[1]).toEqual({
      label: 'lmstudio-proof', access: 'read', createdAt: '2026-11-18T00:00:00.000Z', expiresAt: '2027-02-16T00:00:00.000Z',
    });
  });

  it('revoke of an unknown label is not_found', async () => {
    const r = await lm('doors', 'revoke', 'nope', '--json');
    expect(r.code).toBe(1);
    expect(JSON.parse(r.out).error.code).toBe('not_found');
    expect((await lm('doors', 'revoke', 'lmstudio-proof')).code).toBe(0);
  });

  it('refreshes once on 401, then retries', async () => {
    signIn('at-stale');
    const r = await lm('doors', 'list', '--json');
    expect(r.code).toBe(0);
    expect(lme.refreshes).toBe(1);
  });

  it('is not_signed_in when refresh cannot help', async () => {
    signIn('at-stale');
    lme.refreshTo = null;
    const r = await lm('doors', 'list', '--json');
    expect(JSON.parse(r.out).error.code).toBe('not_signed_in');
  });

  it('maps a missing tool to server_unsupported', async () => {
    delete lme.tools.client_mint;
    const r = await lm('doors', 'open', 'x', '--read', '--json');
    expect(r.code).toBe(1);
    expect(JSON.parse(r.out).error.code).toBe('server_unsupported');
  });

  it('never puts the bearer or a door url in an error', async () => {
    lme.tools.client_mint = () => ({ content: [{ type: 'text', text: `refused ${DOOR_URL} for at-good` }], isError: true });
    const r = await lm('doors', 'open', 'x', '--read', '--json');
    const e = JSON.parse(r.out).error;
    expect(e.code).toBe('server_refused');
    expect(JSON.stringify(e)).not.toContain(DOOR_URL);
    expect(JSON.stringify(e)).not.toContain('lmc_');
    expect(r.out + r.err).not.toContain('at-good');
  });
});

describe('server refusals', () => {
  it('are reported with fixed text, never the server body', async () => {
    lme.tools.client_mint = () => ({ content: [{ type: 'text', text: 'Refused LME.VIIBE.TO/t/LMC_0123456789ABCDEF/mcp sk-live-9f8e7d6c5b4a' }], isError: true });
    const r = await lm('doors', 'open', 'x', '--read', '--json');
    const e = JSON.parse(r.out).error;
    expect(e.code).toBe('server_refused');
    expect(e.message).not.toMatch(/LMC_|sk-live|VIIBE|Refused/);
    expect(e.message).toContain('lm doors list');
  });
});

describe('server text in human output', () => {
  it('strips control characters from labels and titles', async () => {
    lme.tools.world_list = () => ok({ worlds: [{ id: 'w_abc', title: 'Fo\x1b]0;pwned\x07under', isDefault: true }] });
    lme.tools.client_list = () => ok({ clients: [{ label: 'x\r\ny', createdAt: null, expiresAt: null, access: 'read' }] });
    const who = await lm('whoami');
    const list = await lm('doors', 'list');
    expect(who.out + list.out).not.toMatch(/[\u0000-\u0009\u000b-\u001f\u007f-\u009f]/);
    expect(list.out.trim().split('\n')).toHaveLength(2);
  });
});

describe('a server that predates read doors', () => {
  it('refuses to open a door when client_mint cannot take access', async () => {
    lme.schemas.client_mint = { label: z.string() };
    const r = await lm('doors', 'open', 'x', '--read', '--json');
    expect(r.code).toBe(1);
    expect(JSON.parse(r.out).error.code).toBe('server_unsupported');
    expect(lme.calls).toEqual([]);
  });
});

describe('revoke of something that is not a door label', () => {
  it('never echoes what was typed', async () => {
    for (const json of [['--json'], []]) {
      const r = await lm('doors', 'revoke', DOOR_URL, ...json);
      expect(r.code).toBe(1);
      expect(r.out + r.err).not.toContain('lmc_');
    }
  });
});

describe('a Living Memory server that never answers', () => {
  it('gives up, reports server_unreachable, and releases the lock', async () => {
    lme.hang = true;
    out = []; err = [];
    const code = await main(['doors', 'list', '--json'], { home, base: lme.mcpUrl, endpoints: lme.endpoints, timeoutMs: 300 });
    expect(code).toBe(1);
    expect(JSON.parse(out.join('')).error.code).toBe('server_unreachable');
    lme.hang = false;
    expect((await lm('doors', 'list')).code).toBe(0);
  });
});
