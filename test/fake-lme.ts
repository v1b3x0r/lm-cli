import { createServer, type IncomingMessage, type Server } from 'node:http';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { z } from 'zod';

type ToolResult = { content: Array<{ type: 'text'; text: string }>; structuredContent?: Record<string, unknown>; isError?: boolean };
export type Handler = (args: Record<string, unknown>) => ToolResult;

/**
 * A stand-in for LME's OAuth /mcp rail: Bearer-checked, stateless Streamable HTTP, plus a
 * token endpoint so refresh can be exercised against the same process.
 */
export class FakeLme {
  server!: Server;
  base = '';
  validBearer = 'at-good';
  refreshTo: string | null = 'at-good';
  calls: Array<{ name: string; args: Record<string, unknown> }> = [];
  refreshes = 0;
  tools: Record<string, Handler> = {};

  get mcpUrl() { return `${this.base}/mcp`; }
  get endpoints() { return { issuer: this.base, token: `${this.base}/token` }; }

  async start(): Promise<void> {
    this.server = createServer((req, res) => void this.handle(req, res));
    await new Promise<void>((r) => this.server.listen(0, '127.0.0.1', r));
    this.base = `http://127.0.0.1:${(this.server.address() as { port: number }).port}`;
  }

  async stop(): Promise<void> {
    this.server.closeAllConnections();
    await new Promise((r) => this.server.close(r));
  }

  private async body(req: IncomingMessage): Promise<string> {
    let raw = '';
    for await (const c of req) raw += c;
    return raw;
  }

  private async handle(req: IncomingMessage, res: import('node:http').ServerResponse): Promise<void> {
    if (req.url === '/token') {
      await this.body(req);
      this.refreshes += 1;
      res.setHeader('Content-Type', 'application/json');
      if (!this.refreshTo) { res.statusCode = 400; return void res.end('{"error":"invalid_grant"}'); }
      return void res.end(JSON.stringify({ access_token: this.refreshTo, token_type: 'Bearer', expires_in: 3600 }));
    }
    if (req.headers.authorization !== `Bearer ${this.validBearer}`) {
      res.statusCode = 401;
      return void res.end('{"error":"invalid_token"}');
    }
    const raw = await this.body(req);
    const mcp = new McpServer({ name: 'fake-lme', version: '0' });
    for (const [name, fn] of Object.entries(this.tools)) {
      mcp.registerTool(name, { description: name, inputSchema: { label: z.string().optional(), access: z.string().optional() } },
        (args: Record<string, unknown>) => { this.calls.push({ name, args }); return fn(args); });
    }
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined, enableJsonResponse: true });
    res.on('close', () => { void transport.close(); void mcp.close(); });
    await mcp.connect(transport);
    await transport.handleRequest(req, res, raw ? JSON.parse(raw) : undefined);
  }
}

export const ok = (structuredContent: Record<string, unknown>, text = 'ok'): ToolResult =>
  ({ content: [{ type: 'text', text }], structuredContent });
