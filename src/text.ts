/**
 * Make a server-supplied string safe to print in a terminal: control characters (C0, DEL, C1)
 * and bidi overrides become '?', so a label cannot clear the screen, set the window title,
 * start a new fake row, or reorder what the owner reads.
 */
export function safeText(s: unknown): string {
  return String(s ?? '').replace(/[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/g, '?');
}

/** An enum value the server is expected to send, or '?' for anything else. */
export function known<T extends string>(value: unknown, allowed: readonly T[]): T | '?' {
  return allowed.includes(value as T) ? (value as T) : '?';
}
