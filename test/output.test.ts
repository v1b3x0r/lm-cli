import { afterEach, describe, expect, it, vi } from 'vitest';
import { CliError, emit } from '../src/output.js';
import { main } from '../src/cli.js';

function capture() {
  const out: string[] = [];
  const err: string[] = [];
  vi.spyOn(process.stdout, 'write').mockImplementation((s) => (out.push(String(s)), true));
  vi.spyOn(process.stderr, 'write').mockImplementation((s) => (err.push(String(s)), true));
  return { out: () => out.join(''), err: () => err.join('') };
}

afterEach(() => vi.restoreAllMocks());

describe('emit', () => {
  it('writes the json ok envelope and returns 0', () => {
    const io = capture();
    const code = emit(true, { ok: true, data: { a: 1 }, human: 'ignored' });
    expect(code).toBe(0);
    expect(JSON.parse(io.out())).toEqual({ ok: true, data: { a: 1 } });
  });

  it('writes the json error envelope and returns 1', () => {
    const io = capture();
    const code = emit(true, new CliError('not_found', 'no such door'));
    expect(code).toBe(1);
    expect(JSON.parse(io.out())).toEqual({ ok: false, error: { code: 'not_found', message: 'no such door' } });
  });

  it('writes human text to stdout and errors to stderr', () => {
    const io = capture();
    expect(emit(false, { ok: true, data: null, human: 'hello' })).toBe(0);
    expect(emit(false, new CliError('usage', 'bad', 2))).toBe(2);
    expect(io.out()).toBe('hello\n');
    expect(io.err()).toBe('lm: bad\n');
  });
});

describe('main', () => {
  it('exits 2 with usage on an unknown command', async () => {
    const io = capture();
    expect(await main(['frobnicate'])).toBe(2);
    expect(io.err()).toMatch(/usage: lm /i);
  });

  it('prints the package version', async () => {
    const io = capture();
    expect(await main(['version'])).toBe(0);
    expect(io.out().trim()).toBe('0.2.0-alpha.0');
  });
});
