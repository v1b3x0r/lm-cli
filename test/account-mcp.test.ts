import { describe, expect, it } from 'vitest';
import { ACCOUNT_MCP } from '../src/account-mcp.js';
import { DEFAULT_ENDPOINTS } from '../src/auth.js';

describe('account rail destination', () => {
  it('calls the CLI door, where the client tools are offered', () => {
    expect(ACCOUNT_MCP).toBe('https://lme.viibe.to/cli/mcp');
  });

  it('still asks for tokens for the /mcp resource, which /cli/mcp accepts', () => {
    expect(DEFAULT_ENDPOINTS.resource).toBe('https://lme.viibe.to/mcp');
  });
});
