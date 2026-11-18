import { lstatSync, mkdirSync, readFileSync, renameSync, rmdirSync, unlinkSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';
import { CliError } from './output.js';

/** Same keys as the Go rc5 account.json, so an existing sign-in keeps working. */
export interface Account {
  clientId: string;
  redirectUri: string;
  accessToken?: string;
  refreshToken?: string;
  expiresAt: string;
}

const MAX_ACCOUNT_BYTES = 1 << 20;

/** The config home must be a real directory only its owner can reach (Go storePath parity). */
export function ensureHome(home: string): void {
  try { mkdirSync(home, { recursive: true, mode: 0o700 }); }
  catch { throw new CliError('home_unavailable', 'cannot create private credential directory'); }
  const st = lstatSync(home);
  if (!st.isDirectory() || (st.mode & 0o077) !== 0)
    throw new CliError('home_unsafe', 'credential directory must be a real directory accessible only to its owner (0700)');
}

const accountPath = (home: string) => join(home, 'account.json');

/** null when there is no usable account: missing, not a regular file, readable by others, or malformed. */
export function readAccount(home: string): Account | null {
  try {
    const st = lstatSync(accountPath(home));
    if (!st.isFile() || (st.mode & 0o077) !== 0 || st.size > MAX_ACCOUNT_BYTES) return null;
    const a = JSON.parse(readFileSync(accountPath(home), 'utf8')) as Partial<Account>;
    if (typeof a.clientId !== 'string' || !a.clientId || typeof a.redirectUri !== 'string') return null;
    return a as Account;
  } catch {
    return null;
  }
}

/** Atomic replace (temp file in the same directory, then rename), mode 0600. */
export function writeAccount(home: string, a: Account): void {
  ensureHome(home);
  const temp = join(home, `.account-${randomBytes(6).toString('hex')}`);
  try {
    writeFileSync(temp, `${JSON.stringify(a)}\n`, { mode: 0o600, flag: 'wx' });
    renameSync(temp, accountPath(home));
  } catch {
    try { unlinkSync(temp); } catch { /* already gone */ }
    throw new CliError('account_write_failed', 'cannot persist private account');
  }
}

export function removeAccount(home: string): void {
  try { unlinkSync(accountPath(home)); }
  catch (e) { if ((e as NodeJS.ErrnoException).code !== 'ENOENT') throw new CliError('account_write_failed', 'cannot remove private account'); }
}

/**
 * One account command at a time: a refresh token may rotate, and two commands refreshing
 * at once would leave one holding a dead token. Same directory lock and wording as Go.
 */
export function lockAccount(home: string): () => void {
  ensureHome(home);
  const lock = `${accountPath(home)}.lock`;
  try { mkdirSync(lock, { mode: 0o700 }); }
  catch {
    throw new CliError('account_locked',
      `account is locked; finish the other account command first. After a crashed command, remove ${lock} only when no account command is running`);
  }
  // Ctrl+C during the 5-minute login wait is the normal way out; it must not leave the lock.
  const onSignal = (signal: NodeJS.Signals) => { release(); process.exit(signal === 'SIGINT' ? 130 : 143); };
  const release = () => {
    process.off('SIGINT', onSignal);
    process.off('SIGTERM', onSignal);
    try { rmdirSync(lock); } catch { /* already released */ }
  };
  process.once('SIGINT', onSignal);
  process.once('SIGTERM', onSignal);
  return release;
}

export async function withAccountLock<T>(home: string, fn: () => Promise<T>): Promise<T> {
  const release = lockAccount(home);
  try { return await fn(); } finally { release(); }
}
