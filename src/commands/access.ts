import { callAccountTool } from '../account-mcp.js';
import type { Context } from '../context.js';
import type { Result } from '../output.js';
import { renderAccessTable } from '../table.js';
import type { AccessRow } from '../types.js';

/** Who can reach this World's memory. JSON is the server's array untouched; the table is for people. */
export async function accessList(ctx: Context): Promise<Result> {
  const { access } = await callAccountTool<{ access: AccessRow[] }>(ctx.home, 'access_list', {}, ctx);
  const human = access.length
    ? renderAccessTable(access, Date.now(), process.stdout.columns || 80)
    : 'Nothing can reach this World.';
  return { ok: true, data: access, human };
}
