/**
 * Every failure lm reports. `message` is shown to people and agents, so it must never
 * carry a URL, a token or a server response body — callers pass fixed text only.
 */
export class CliError extends Error {
  constructor(readonly code: string, message: string, readonly exit: 1 | 2 = 1) {
    super(message);
  }
}

export type Result = { ok: true; data: unknown; human: string };

/** Write a result in the requested mode and return the process exit code. */
export function emit(json: boolean, result: Result | CliError): number {
  if (result instanceof CliError) {
    if (json) process.stdout.write(`${JSON.stringify({ ok: false, error: { code: result.code, message: result.message } })}\n`);
    else process.stderr.write(`lm: ${result.message}\n`);
    return result.exit;
  }
  process.stdout.write(json ? `${JSON.stringify({ ok: true, data: result.data })}\n` : `${result.human}\n`);
  return 0;
}
