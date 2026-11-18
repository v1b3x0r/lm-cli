#!/usr/bin/env node
import { readFileSync, realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { login, logout } from './auth.js';
import { accessList } from './commands/access.js';
import { doorsList, doorsOpen, doorsRevoke, whoami } from './commands/doors.js';
import type { Context } from './context.js';
import { configHome } from './home.js';
import { CliError, emit, type Result } from './output.js';

const USAGE = `usage: lm <command> [--json]

  lm login | lm logout            Sign in to Living Memory, or forget this computer's sign-in
  lm whoami                       Show your default World
  lm doors list                   List the doors into your World
  lm doors open <label> --read    Open a read-only door (prints its link once)
  lm doors revoke <label>         Close a door
  lm access list                  Who can reach your World's memory right now
  lm version                      Print the lm version`;

function version(): string {
  const pkg = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { version: string };
  return pkg.version;
}

const usage = () => new CliError('usage', USAGE, 2);

async function run(args: string[], flags: Set<string>, ctx: Context): Promise<Result> {
  const [command, sub, arg, ...rest] = args;
  if (rest.length) throw usage();
  switch (command) {
    case 'version':
      return { ok: true, data: { version: version() }, human: version() };
    case 'login':
      if (sub) throw usage();
      await login({ home: ctx.home, endpoints: ctx.endpoints });
      return { ok: true, data: { signedIn: true }, human: 'Signed in. Next: lm whoami' };
    case 'logout':
      if (sub) throw usage();
      logout(ctx.home);
      return { ok: true, data: { signedIn: false }, human: 'Signed out on this computer.' };
    case 'whoami':
      if (sub) throw usage();
      return whoami(ctx);
    case 'doors':
      if (sub === 'list' && !arg) return doorsList(ctx);
      if (sub === 'open' && arg) return doorsOpen(ctx, arg, flags.has('--read'));
      if (sub === 'revoke' && arg) return doorsRevoke(ctx, arg);
      throw usage();
    case 'access':
      if (sub === 'list' && !arg) return accessList(ctx);
      throw usage();
    default:
      throw usage();
  }
}

export async function main(argv: string[], ctx: Partial<Context> = {}): Promise<number> {
  const flags = new Set(argv.filter((a) => a.startsWith('--')));
  const json = flags.has('--json');
  const known = new Set(['--json', '--read']);
  const args = argv.filter((a) => !a.startsWith('--'));
  try {
    if ([...flags].some((f) => !known.has(f))) throw usage();
    return emit(json, await run(args, flags, { home: configHome(), ...ctx }));
  } catch (e) {
    return emit(json, e instanceof CliError ? e : new CliError('internal', 'unexpected failure; nothing was printed about your credentials'));
  }
}

// Run only when executed as the `lm` binary, not when imported by tests.
const invoked = process.argv[1] ? realpathSync(process.argv[1]) : '';
if (invoked === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).then((code) => { process.exitCode = code; });
}
