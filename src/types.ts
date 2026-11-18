/**
 * One row of LME's `access_list`, mirrored field for field from lme-remote src/access.ts.
 * The CLI never imports server code; if the server shape changes, this must change with it.
 */
export interface AccessRow {
  id: string;
  principal: { kind: 'human' | 'agent' | 'holder' | 'service'; name?: string; evidence: 'verified' | 'declared' | 'system' };
  client: { label: string; labelEvidence?: 'declared'; rail: 'oauth' | 'token' | 'agentid' };
  grant: {
    scope: { kind: 'world'; id: string };
    access: 'read' | 'write' | 'owner';
    status: 'active' | 'revoked' | 'expired' | 'suspended';
    effective: boolean | null;
    createdAt: string | null;
    expiresAt: string | null;
    revocable: boolean;
    note: string | null;
  };
  usage: { lastUsedAt: string | null };
}
