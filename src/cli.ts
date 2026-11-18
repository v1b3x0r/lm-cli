#!/usr/bin/env node
import { readFileSync, realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { CliError, emit } from './output.js';

const USAGE = `usage: lm <command> [--json]

  lm version                      Print the lm version`;

function version(): string {
  const pkg = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { version: string };
  return pkg.version;
}

export async function main(argv: string[]): Promise<number> {
  const json = argv.includes('--json');
  const args = argv.filter((a) => a !== '--json');
  const [command] = args;
  if (command === 'version') return emit(json, { ok: true, data: { version: version() }, human: version() });
  return emit(json, new CliError('usage', USAGE, 2));
}

// Run only when executed as the `lm` binary, not when imported by tests.
const invoked = process.argv[1] ? realpathSync(process.argv[1]) : '';
if (invoked === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).then((code) => { process.exitCode = code; });
}
