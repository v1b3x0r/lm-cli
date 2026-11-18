import { createServer, type Server } from 'node:http';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { main } from '../src/cli.js';
import { doctorDoor } from '../src/commands/doctor.js';

const TOKEN = 'lmc_00112233445566778899aabbccddeeff';
let server: Server;
let base: string;
let tools: string[];
let out: string[];
let err: string[];

beforeEach(async () => {
  tools = ['memory_search', 'memory_state', 'world_list'];
  server = createServer(async (req, res) => {
    if (req.url !== `/t/${TOKEN}/mcp`) { res.statusCode = 404; return void res.end('{"error":"unknown token"}'); }
    let raw = '';
    for await (const c of req) raw += c;
    const mcp = new McpServer({ name: 'door', version: '0' });
    for (const t of tools) mcp.registerTool(t, { description: t }, () => ({ content: [{ type: 'text', text: '' }] }));
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined, enableJsonResponse: true });
    res.on('close', () => { void transport.close(); void mcp.close(); });
    await mcp.connect(transport);
    await transport.handleRequest(req, res, raw ? JSON.parse(raw) : undefined);
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  base = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  out = []; err = [];
  vi.spyOn(process.stdout, 'write').mockImplementation((s) => (out.push(String(s)), true));
  vi.spyOn(process.stderr, 'write').mockImplementation((s) => (err.push(String(s)), true));
});

afterEach(async () => {
  vi.restoreAllMocks();
  server.closeAllConnections();
  await new Promise((r) => server.close(r));
});

describe('lm doctor door', () => {
  it('reports a read door as read-only', async () => {
    expect(await doctorDoor(`${base}/t/${TOKEN}/mcp`)).toEqual({
      reachable: true, readOnly: true, tools: ['memory_search', 'memory_state', 'world_list'],
    });
  });

  it('reports a write door as not read-only', async () => {
    tools = ['memory_add', 'memory_search', 'memory_state', 'handoff_post', 'world_list'];
    expect((await doctorDoor(`${base}/t/${TOKEN}/mcp`)).readOnly).toBe(false);
  });

  it('is unreachable, exit 1, for a revoked or unknown door', async () => {
    expect((await doctorDoor(`${base}/t/lmc_ffffffffffffffffffffffffffffffff/mcp`)).reachable).toBe(false);
    expect(await main(['doctor', 'door', `${base}/t/lmc_ffffffffffffffffffffffffffffffff/mcp`, '--json'])).toBe(1);
  });

  it('refuses anything that is not a door URL', async () => {
    expect(await main(['doctor', 'door', 'https://evil.example/collect', '--json'])).toBe(2);
  });

  it('never prints the url or the token', async () => {
    for (const json of [[], ['--json']]) {
      await main(['doctor', 'door', `${base}/t/${TOKEN}/mcp`, ...json]);
      await main(['doctor', 'door', `${base}/t/lmc_ffffffffffffffffffffffffffffffff/mcp`, ...json]);
    }
    const all = out.join('') + err.join('');
    expect(all).toContain('memory_search');
    expect(all).not.toContain(TOKEN);
    expect(all).not.toContain('lmc_ffff');
    expect(all).not.toContain(base);
  });
});
