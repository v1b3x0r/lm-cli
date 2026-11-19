# Capsule-first CLI (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fresh free account can create Capsules, list them, open an LME-issued read-only door scoped to one Capsule, let an external AI read it, and revoke it, all through `lm`.

**Architecture:** LME issues a new kind of token row, a *capsule read grant*. It shares the Capsule room's brain tuple, is read-only by tool registration, and carries its own 90-day cap (`grant.expiresAt`). The room horizon (`expiresAt`) keeps propagating to it, so it also dies with the room. Pakkana stays the membership authority: before minting, LME asks Pakkana through the existing `CapsuleBridge` whether the caller holds the invite capability. `lm-cli-ts` gets Capsule-scoped commands and drops the World-wide door commands.

**Tech Stack:** TypeScript, Node, vitest, `@modelcontextprotocol/sdk` (both repos). No Pakkana code change.

**Spec:** `lm-cli-ts/docs/superpowers/specs/2026-11-19-lm-capsule-first-cli-contract.md`, approved 2026-11-19: D1–D9, J1–J7, E2 and the 4 spec deltas.

**Execution:** subagent-driven development (founder, 2026-11-19).

**Status:** SEALED 2026-11-19. Execution starts only on the founder's go.

**Repos / workspaces:**
- LME: new worktree `~/w/viibe/living-memory/lme-remote-capsule-grants` on branch `feat/capsule-read-grants`, from `origin/main` 330c72f. Never touch the main `lme-remote` checkout.
- CLI: worktree `~/w/viibe/living-memory/lm-cli-ts`, new branch `feat/capsule-first` from `feat/cli-door` (4c220c1), so lm-cli #7 lands with this work.

## Global Constraints

- Commit, push, merge, deploy and npm publish each need the founder's explicit word. The "Commit" steps below run only once the founder has authorized execution with commits.
- The Capsule ID is opaque: never parse `w_`, never derive a room or World from it except through `findCapsuleRoom` (D1).
- One grant opens exactly one Capsule room. There is no "include children" mode (D3).
- Only a member whose Pakkana entry has `canShare === 1` (the invite capability) may open a door (D8). `createdBy` / `role` is never used for authorization.
- Grant lifetime: 90 days from issue, fixed (`CAPSULE_GRANT_DAYS = 90`). A request is served only while BOTH the grant cap has not passed AND the room is live.
- `last_used_at` moves only on an authorized MCP POST. Never on refusals, never on GET.
- Every refusal returns exactly `Capsule unavailable for this account.` (no existence leak).
- The custodian row (`ons_`) and unclaimed `ro_` rows are listed and never revocable from any user surface (D9).
- v1 is text-only: a grant exposes exactly `memory_search` and `memory_state` (D7).
- Read-only is enforced by not registering tools. No allowlist proxy.
- English in code, copy and docs. Thai only in conversation.

## Review Focus

1. **Rollback with live grants:** pre-change code reads a grant row as a plain `plan:'ons'` read door, with no 90-day cap and a wider tool set. Every rollback must remove grant rows first. Covered by Task 5 (`removeCapsuleGrants`) and the Rollback section.
2. **Grant cap reached while the room is live:** must delete only the grant row. It must never call `destroyTrial` or touch sibling rows or the brain. Test in Task 2.
3. **Room destroyed or swept while a grant exists:** the grant must go with it, and `capsule_door_list` must answer the generic refusal. Test in Task 1 (sweep) and Task 4 (list after sweep).
4. **Bridge state file damaged or the Pakkana API down during `capsule_door_open`:** must refuse, never mint without a capability answer. Test in Task 4.
5. **A label or door id pasted as the URL itself:** the CLI must never echo it back (existing `doorsRevoke` rule). Test in Task 8.

---

### Task 1: Grant row model and room resolution (LME)

**Files:**
- Create: `src/capsule-grants.ts`
- Modify: `src/identity.ts` (add `grant?: CapsuleGrant` to `TokenEntry`, with a doc comment)
- Test: `test/capsule-grants.test.ts`

**Interfaces:**
- Produces:
  - `CAPSULE_GRANT_DAYS = 90`
  - `MAX_GRANTS_PER_CAPSULE = 32` (mirrors Pakkana's door limit)
  - `GRANT_REFUSAL = 'Capsule unavailable for this account.'`
  - `class CapsuleGrantRefused extends Error`, whose message is always `GRANT_REFUSAL`
  - `interface CapsuleGrant { kind: 'capsule-read'; capsuleId: string; owner: string; label: string; createdAt: string; expiresAt: string }`
  - `findCapsuleRoom(tokens: TokenMap, capsuleId: string, now: number): { token: string; entry: TokenEntry } | null`: the custodian row, i.e. `plan==='ons'`, not `readOnly`, no `grant`, `worldIdFor(ns)===capsuleId`, not expired.
  - `mintCapsuleGrant(o: { tokens: TokenMap; tokenMapPath: string; capsuleId: string; owner: string; label: string; now: number }): { token: string; doorId: string; entry: TokenEntry }`
  - `removeCapsuleGrants(tokens: TokenMap, f: { capsuleId?: string; doorId?: string; all?: true }): Array<[string, TokenEntry]>`: a pure function that removes only rows carrying `grant`.
  - `doorId = accessRowId(token)` (existing `acc_…` convention)

- [ ] **Step 1: Write failing tests** in `test/capsule-grants.test.ts`, using a tokens map with room A (`ons_a`, `ro_a`) and room B (`ons_b`):
  - `findCapsuleRoom` returns A's custodian for `worldIdFor('ons_a')`. It returns null for an unknown id, for B's id with A's rows deleted, for an expired room, and for an id that only matches a `ro_` or grant row.
  - `mintCapsuleGrant`: the token matches `/^lmr_[a-f0-9]{32}$/`. The entry has `plan:'ons'`, `readOnly:true`, `ns/rcUserId/brainPath` equal to A's, `expiresAt` equal to A's current horizon, `grant.expiresAt === new Date(now + 90*86400000).toISOString()`, and `grant.owner`/`label`/`capsuleId` set. The map is persisted (re-read from `tokenMapPath`).
  - Mint refuses with `CapsuleGrantRefused` for an unknown or expired room, a duplicate label for the same owner+capsule, and the 33rd live grant on one Capsule.
  - `touchOnsActivity` on A's custodian moves the grant's `expiresAt` to the new horizon and leaves `grant.expiresAt` unchanged.
  - Sweeping A (the existing `sweepExpired` path with the clock past A's horizon) removes the grant row too.
  - `removeCapsuleGrants`: `{doorId}` removes one grant. `{capsuleId}` removes all of A's grants and none of B's. `{all:true}` removes every grant. None of these ever removes `ons_*` or `ro_*`.
- [ ] **Step 2: Run** `npx vitest run test/capsule-grants.test.ts`. Expected: FAIL (module not found).
- [ ] **Step 3: Implement** `src/capsule-grants.ts` and the `TokenEntry.grant` field. Mint reuses `persistTokenMap` with the same rollback-on-persist-failure pattern as `mintClient` (`clients.ts:226-246`).
- [ ] **Step 4: Run** `npx vitest run test/capsule-grants.test.ts test/ons.test.ts test/identity.test.ts`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(grants): capsule read grant rows with their own 90-day cap`

### Task 2: Serving a grant: cap, tool set, usage, sibling routes (LME)

**Files:**
- Modify: `src/authorize.ts` (new `TokenVerdict` reason `'grant-expired'`, checked after the room-expiry gate)
- Modify: `src/app.ts`
  - token route: handle `'grant-expired'`, move `usage.touch` after the GET entrance branch, and give grants `mintedClient` treatment with no `worlds`
  - `/world/raise`: refuse grant rows like `plan==='client' && readOnly`
- Test: `test/capsule-grants-route.test.ts`

**Interfaces:**
- Consumes: Task 1 (`mintCapsuleGrant`, `CapsuleGrant`)
- Produces: `TokenVerdict['reason']` includes `'grant-expired'`. The token route answers `410 {"error":"door expired"}` for it.

- [ ] **Step 1: Write failing tests** (`makeHandler`, mock embed, rooms A and B with seeded memories `<marker-A>` / `<marker-B>`):
  - `tools/list` through A's grant equals exactly `['memory_search','memory_state']`, sorted.
  - `tools/call memory_add` / `handoff_post` / `world_raise` / `capsule_create` by name → `isError` (not registered).
  - `memory_search '<marker-B>'` through A's grant returns no B content. `'<marker-A>'` returns A's memory.
  - Clock past `grant.expiresAt` with the room live → 410 `door expired`. The grant row is gone. `ons_a` still serves `memory_search` and the brain file still exists. `destroyTrial` is not invoked (spy, or assert that the brain file and siblings are intact). An access-history record has `status:'expired'`.
  - Clock past the room horizon with the grant cap live → refused (the existing room-expiry path). The grant is gone with the room.
  - Deleted (revoked) grant → 404 `unknown token`.
  - `usage.lastUsedAt(usageKeyForToken(grant))` stays null after a GET of `/t/<grant>/mcp` and after every refused request above. It is set after one successful POST.
  - `POST /world/raise` with a grant bearer → 403, and the body is never read.
  - `GET /space/status` with a grant bearer → 401 (regex accepts only `ons|ro`; pin that).
  - `POST /space/lifetime` with a grant bearer and no seller key → 403 or 404 (seller gate first; pin the current code).
- [ ] **Step 2: Run** `npx vitest run test/capsule-grants-route.test.ts`. Expected: FAIL.
- [ ] **Step 3: Implement.** The `'grant-expired'` branch deletes only that row via `removeCapsuleGrants(tokens, { doorId })`, persists the map, calls `appendHistory` (`status:'expired'`, `capsuleId`), `clearOnsOps(token)` without a room key, and answers 410. The `never` switch forces this branch to exist. Grants mount with `mintedClient: true` and `worlds: undefined`.
- [ ] **Step 4: Run** `npm test`. Expected: the full suite passes (baseline 1098, plus the new tests).
- [ ] **Step 5: Commit** `feat(grants): serve capsule grants read-only, capped, and refused on sibling routes`

### Task 3: Bridge lists every accessible Capsule, with capability (LME)

**Files:**
- Modify: `src/capsules.ts`
  - `CapsuleAccess`: change the `list()` result and add `doorCapability`
  - `forSubject`: `list` links the identity first when memberships are enabled
- Modify: `src/capsule-tools.ts`: update the `capsule_list` description (it no longer says "created through this plugin")
- Test: `test/capsules.test.ts` (new `describe('capsule_list (Capsule-first)')`)

**Interfaces:**
- Produces:
  - `CapsuleAccess.list(): Promise<{ capsules: Array<{ capsule_id: string; name: string; role: 'owner'|'controller'|'contributor'|'reader'|'member'; can_open_door: boolean; expires_at: string }> }>`
    - `role` = `membership_role` if present, else `'owner'` when Pakkana `role==='created'`, else `'member'`.
    - `can_open_door` = Pakkana `canShare === 1`.
  - `CapsuleAccess.doorCapability(capsuleId: string): Promise<{ capsule_id: string; name: string; can_open_door: boolean }>`
    - Throws `CapsuleError(GRANT_REFUSAL)` when the Capsule is not in `/places` for the linked device, or is `kind:'world'`.
- Behaviour when memberships are disabled (`enabled === false`): `list` and `doorCapability` keep today's rule (bridge-created only), and `can_open_door` still comes from `canShare`.

- [ ] **Step 1: Write failing tests** with the existing `fixture()`, extended so that `/places` can return `canShare`, `role` and `membership_role`, and `/rooms/identity` answers `{linked:true}`. Use a bridge config with `memberships: true`, `membershipToken`, `identityProject`, and `membershipIssuer: 'issuer'`.
  - A Capsule present in `/places` but never created by the bridge (an app-created Capsule) appears in `list`.
  - `kind:'world'` never appears.
  - `/rooms/identity` is called before `/places` on `list`.
  - Role and `can_open_door` mapping: created + canShare 1 → `owner` / true; `membership_role:'reader'` + canShare 0 → `reader` / false.
  - `doorCapability`: an unknown id, a World id, and a damaged state file are all refused with exactly `GRANT_REFUSAL`. Pakkana answering 500 → refused (no mint can follow).
  - With memberships disabled: the existing bridge-created-only tests still pass unchanged.
- [ ] **Step 2: Run** `npx vitest run test/capsules.test.ts`. Expected: the new tests FAIL and the old ones PASS.
- [ ] **Step 3: Implement.** Delete the `allowed.has(p.id)` filter only on the memberships-enabled path.
- [ ] **Step 4: Run** `npx vitest run test/capsules.test.ts test/room-tools*.test.ts`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(capsules): list every Capsule the account can reach, with door capability`

### Task 4: Capsule door tools on both mounts (LME)

**Files:**
- Create: `src/capsule-door-tools.ts`
- Modify: `src/access-history.ts`
  - `HistoryRecord.capsuleId?: string` (optional, backward compatible)
  - new `readCapsuleHistory(path: string, capsuleId: string, now: number): HistoryRecord[]` (30-day window, filtered by `capsuleId` only)
- Modify: `src/tools.ts` (option `capsuleDoors?: CapsuleDoorAdmin`; register when present and `!opts.readOnly && !opts.mintedClient`)
- Modify: `src/app.ts`: build `CapsuleDoorAdmin` in the OAuth World mount (next to `clientAdmin`, `app.ts:1038-1084`) and in `serveAccountFreeRoom` (thread a new parameter through `dispatchQueuedMount`). Both use the same withholding rule as `clientAdmin`: absent on `/mcp` when `hideClientTools`, and absent for `actorType === 'agent'`.
- Test: `test/capsule-door-tools.test.ts`

**Interfaces:**
- Consumes: Task 1 (`mintCapsuleGrant`, `removeCapsuleGrants`, `findCapsuleRoom`) and Task 3 (`doorCapability`)
- Produces:
  - `interface CapsuleDoorAdmin { sub: string; capsules: CapsuleAccess; tokens: TokenMap; tokenMapPath: string; historyPath: string; usage?: UsageLog; publicBase: string; now?: () => number }`
  - `registerCapsuleDoorTools(server: McpServer, admin: CapsuleDoorAdmin): void`
  - `capsule_door_open {capsule_id, label}` → `{ door_id, label, access:'read', url, expires_at, capsule_id }`
  - `capsule_door_list {capsule_id}` → `{ capsule_id, doors: CapsuleDoorRow[] }`
  - `capsule_door_revoke {capsule_id, door_id}` → `{ revoked: true, door_id }`
  - `CapsuleDoorRow = { door_id; holder: 'you'|'another member'|'Pakkana (custodian)'|'unclaimed read link'; label: string|null; access: 'read'|'write'; status: 'live'|'expired'|'revoked'; revocable: boolean; created_at: string|null; expires_at: string|null; last_used_at: string|null }`
  - The custodian row's label is exactly `Pakkana doors and visitor QR are managed in Pakkana`.

- [ ] **Step 1: Write failing tests.** Use the OAuth fake from `test/clients-rail.test.ts` and the bridge fixture from Task 3, with LME rooms A and B whose ids match the Pakkana place ids.
  - **Free account** (no World) on `/cli/mcp`: the three tools are listed. On `/mcp` with `hideClientTools`: absent. As an agent: absent.
  - **World account**: also listed on `/cli/mcp`.
  - `open` on A as an owner returns a URL `https://<publicBase>/t/lmr_…/mcp`. Calling `open` again with the same label is refused.
  - `open` refusals, each with exactly `GRANT_REFUSAL` and no token row created:
    - a reader (`canShare 0`)
    - an id absent from `/places`
    - an id present in `/places` with no LME room
    - when Pakkana returns 500
  - `list` A: the custodian row has `revocable:false`, the `ro_` row is `unclaimed read link` with `revocable:false`, the caller's grant is `you`, and another subject's grant is `another member` with its label shown and its owner never shown. After a revoke, the row comes back as `status:'revoked'` from history for 30 days.
  - **List policy** (spec E table), each refusal exactly `GRANT_REFUSAL`:
    - a non-member
    - an unknown id
    - a World id
    - a reader (`canShare 0`)
    - a contributor (`canShare 0`)

    With `canShare 1`, the caller sees all of A's rows and A's history only. Seeded history for B (`capsuleId: B`) never appears in A's list. Grants on B never appear in A's list, even when both are owned by the caller.
  - `list` after A is swept → refused.
  - `revoke` own → `{revoked:true}`. The next request on that token → 404.
  - `revoke` refusals, each with `GRANT_REFUSAL` and the rows still present: the custodian's door_id, the `ro_` door_id, another member's door_id, and B's door_id passed with A's capsule_id.
  - **Isolation (J2):** a grant opened on A, used through MCP, never returns `<marker-B>`.
- [ ] **Step 2: Run** `npx vitest run test/capsule-door-tools.test.ts`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Order inside `open` and `list`: `doorCapability` first (Pakkana; it requires `can_open_door`), then `findCapsuleRoom` (LME), then `mintCapsuleGrant` (open only). Any failure → `GRANT_REFUSAL`.
  - In this task, `revoke` succeeds only when `grant.owner === admin.sub`. Task 6 widens it.
  - `last_used_at` comes from `usage.lastUsedAt(usageKeyForToken(token))`.
- [ ] **Step 4: Run** `npm run typecheck && npm test`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(grants): capsule_door_open/list/revoke for World and Free Room accounts`

### Task 5: Break-glass and rollback tool (LME)

This answers the founder's question about emergency revocation (see the section below).

**Files:**
- Create: `src/capsule-grants-admin.ts` (operator CLI, compiled to `dist/capsule-grants-admin.js`)
- Test: `test/capsule-grants-admin.test.ts`

**Interfaces:**
- Consumes: Task 1 (`removeCapsuleGrants`)
- Produces: `runGrantsAdmin(argv: string[], io: { tokensPath: string; historyPath: string; out: (s: string) => void; now: () => number }): number` (exit code)
  - `list [--capsule <id>]`: prints door_id, capsuleId, label, created, cap. Never prints tokens.
  - `revoke (--door <id> | --capsule <id> | --all) [--apply]`: a dry run unless `--apply`. `--apply` also requires `--service-stopped`, an explicit acknowledgement. The live server keeps the token map in memory and would overwrite an edit made while it runs.
  - `verify-none`: re-reads the token file from disk. Exit 0 and prints `capsule-read grants: 0` only when no row carries `grant`. Otherwise exit 1 and prints the count. An unreadable or unparsable file is exit 1, never 0. This is the fail-closed gate for Rollback.
  - Writes access-history `status:'revoked'` for each removed row.

- [ ] **Step 1: Write failing tests:**
  - A dry run changes nothing.
  - `--apply` without `--service-stopped` → exit 2, nothing changed.
  - `--apply --service-stopped --door X` removes exactly X.
  - `--capsule A` removes only A's grants.
  - `--all` removes every grant and no `ons_` / `ro_` / `lmc_` row.
  - `verify-none` exit codes:
    - exit 0 after `--all --apply`
    - exit 1 with one grant present
    - exit 1 on a missing file
    - exit 1 on a truncated or invalid JSON file
  - Output never contains `lmr_`, `ons_` or `ro_`.
- [ ] **Step 2: Run** the test. Expected: FAIL.
- [ ] **Step 3: Implement.** Use the existing `persistTokenMap` for the atomic write.
- [ ] **Step 4: Run** `npm run build && npx vitest run test/capsule-grants-admin.test.ts`. Expected: PASS, and `dist/capsule-grants-admin.js` exists.
- [ ] **Step 5: Commit** `feat(grants): operator break-glass and rollback tool for capsule grants`

### Task 6: Co-member revoke, E2 (LME). APPROVED 2026-11-19

**Files:** Modify `src/capsule-door-tools.ts`. Test: `test/capsule-door-tools.test.ts`.

- [ ] **Step 1: Write failing tests:**
  - A subject whose `doorCapability(A).can_open_door` is true at call time can revoke another member's grant on A. The history row records `owner` = the original owner and `capsuleId` = A.
  - That subject's capability turns false (fixture flips `canShare` to 0) → revoke is refused.
  - A reader cannot.
  - Another member's grant on B, passed with A's `capsule_id`, is refused.
  - The custodian and unclaimed rows stay refused.
  - `revocable` in `list` is true for every grant row when the viewer can open doors, and false for the custodian and unclaimed rows.
- [ ] **Step 2–4:** Implement and run. Revoke allows `grant.capsuleId === capsule_id && (grant.owner === sub || (await doorCapability(capsule_id)).can_open_door)`.
- [ ] **Step 5: Commit** `feat(grants): Capsule controllers can revoke any LM grant on that Capsule`

### Task 7: `lm capsules list|create` (CLI)

**Files:**
- Create: `src/commands/capsules.ts`
- Modify: `src/cli.ts`: dispatch, USAGE, and a parser that accepts up to two positional args after `<command> <sub>`
- Test: `test/capsules.test.ts` (uses `test/fake-lme.ts`)

**Interfaces:**
- Consumes: LME `capsule_list` (Task 3 shape) and `capsule_create` (existing)
- Produces:
  - `capsulesList(ctx: Context): Promise<Result>` with columns `ID  NAME  ROLE  DOOR  EXPIRES`. DOOR shows `yes`/`no` from `can_open_door`.
  - `capsulesCreate(ctx: Context, name: string, note: string): Promise<Result>`, which calls `capsule_create {name, content: note, request_id: randomUUID()}` and prints the new ID.

**Spec delta (needs approval):** the parser has no value flags, so the surface is positional:
- `lm capsules create <name> <note>`
- `lm doors open <capsule-id> <label>`
- `lm doors revoke <capsule-id> <door-id>`

Spec F used `--note` and `--label`.

- [ ] **Step 1: Write failing tests:**
  - The list renders rows and `--json` returns `data.capsules` unchanged.
  - An empty list prints `No Capsules yet. Create one: lm capsules create <name> <note>`.
  - `create` passes name and note, and two calls use different `request_id`s.
  - An LME `isError` surfaces as a `CliError` with LME's text.
  - `whoami` on a Free Room account prints the room (no `no_world` error).
- [ ] **Step 2: Run** `npx vitest run test/capsules.test.ts`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(cli): lm capsules list and create`

### Task 8: Capsule-scoped `lm doors`; drop the World door; new DONE-WHEN (CLI)

**Files:**
- Modify: `src/commands/doors.ts`: replace `doorsList/doorsOpen/doorsRevoke` with Capsule-scoped versions; keep `whoami`
- Modify: `src/cli.ts`: remove `access list` and the `--read` flag (D5, D6), update USAGE
- Delete: `src/commands/access.ts`, `test/access.test.ts`
- Modify: `test/doors.test.ts`, `CLAUDE.md` (DONE-WHEN), `README.md` (commands + KSE-1 note)

**Interfaces:**
- Consumes: Task 4 tools
- Produces:
  - `doorsOpen(ctx, capsuleId: string, label: string)`
  - `doorsList(ctx, capsuleId: string)` with columns `DOOR  HOLDER  LABEL  ACCESS  STATUS  LAST USED  EXPIRES`
  - `doorsRevoke(ctx, capsuleId: string, doorId: string)`

- [ ] **Step 1: Write failing tests:**
  - `lm doors` / `lm doors list` without a Capsule ID → usage error that names `lm capsules list`.
  - `open` prints the URL exactly once, with the copy `This link is a key: whoever holds it can read this Capsule's memory until it expires or you run "lm doors revoke <capsule-id> <door-id>".`
  - `list` shows the custodian and unclaimed rows.
  - `revoke` success copy is `Revoked <door-id>. That link no longer opens this Capsule.` and never echoes an argument that looks like a URL (`/^https?:/`), printing `Revoked.` instead.
  - `lm access list` → usage error.
  - No code path calls `client_mint`, `client_list`, `client_revoke` or `access_list` (grep assertion in the test).
- [ ] **Step 2: Run** `npm test`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Replace DONE-WHEN in `CLAUDE.md` with the spec's DONE-WHEN, verbatim.
  - README: Capsule commands, plus a "What `lm doors list` does not show" paragraph: Pakkana doors and visitor QR, managed in Pakkana (D9), with a KSE-1 pointer.
- [ ] **Step 4: Run** `npm run build && npm test && npm run typecheck`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(cli): Capsule-scoped doors; retire the World-wide door from the CLI journey`

### Task 9: Production acceptance J1–J7

This is a runbook, not code. It runs only after the founder approves the LME deploy.

- [ ] **Preflight** (Claude runs these, read-only):
  1. `npx wrangler versions view <current>` on pakkana-api shows `PAKKANA_ROOM_MEMBERSHIP ("1")` and `PAKKANA_IDENTITY_PROJECT`. If either is missing, stop: J1, J3 and J4 depend on them.
  2. LME suite green on the branch head.
  3. lm-cli-ts suite green.
- [ ] **Deploy LME** via the `lme-remote-deploy` skill. The founder runs ssh. Record the backup dir name and the boot line.
  - As part of the deploy, copy the WHOLE new `dist/` (plus the `node_modules` it resolves `zod` from) to `/opt/lme-remote/rollback-verify/`, so the fail-closed check survives a dist restore (Rollback step 5). Do not hand-pick "the file and its imports": its import closure is 8 modules plus `zod` (final review, 2026-11-20).
  - The operator tool must run as the service user with the service's env, never as bare root (it would leave the token map root-owned and the service unable to boot, or read `/root/...`). Form: `systemd-run --uid=lme -p EnvironmentFile=<the unit's EnvironmentFile> --wait --pipe node <dist>/capsule-grants-admin.js <cmd>`, or pass `--tokens <path>` explicitly. Every command prints `tokens: <path>` first — check it is the service's file.
  - Run `verify-none` once from `/opt/lme-remote/rollback-verify/` that way to prove the copy runs: exit 0, `capsule-read grants: 0` before any door exists.
  - This release's Rollback (section below) REPLACES the `lme-remote-deploy` skill's standard rollback one-liner, which restarts the service immediately.
- [ ] **Accounts:** the founder signs up N and M (fresh Stytch accounts via email aliases, never used in Pakkana) plus their own account F. The CLI runs from the local build: `alias lmx='node ~/w/viibe/living-memory/lm-cli-ts/dist/cli.js'`.
- [ ] **J1** as N:
  - `lmx login` → `lmx whoami` shows the Free Room.
  - `lmx capsules create A "<marker-A> …"`.
  - `lmx capsules list` shows A with role owner and DOOR yes.
  - `lmx doors open <A> lmstudio`, then attach the URL to Claude Desktop or LM Studio, unmodified, and ask for `<marker-A>`.
  - N signs in to pakkana web and sees A.
- [ ] **J2** as N: create B with `<marker-B>`, ask through A's door for `<marker-B>` (not found), then `lmx doors open <B-id> x` as M (refused, generic).
- [ ] **J3** as F:
  - `lmx capsules list` shows app-created Capsules, without the World.
  - Open a door on a test Capsule and read it.
  - Then open that Capsule's existing `/c/…` link in a browser: it still answers. **Do not rotate any visitor link.**
- [ ] **J4:** F invites M as reader (existing room invitation flow) → M's list shows `reader`, DOOR no, and `doors open` is refused. F revokes M → the Capsule disappears from M's list.
- [ ] **J5** as N: open `d1` and `d2` on A, use each once, `doors list A` (both have last used, plus the custodian row), revoke `d1` → the client is refused on `d1` and `d2` still reads → `lmx doctor door <d1-url>` exits 1.
- [ ] **J6:** already proven in the suite (Task 2). On prod, only check that `doors list` shows `expires_at` ≤ 90 days from issue.
- [ ] **J7 (E2):** run exactly as the spec's J7.
  - Setup: legacy Capsule L, created by F with `lmx capsules create`. M joins through the Pakkana invite link, giving `role: member` and DOOR yes. Managed Capsule C, where M is `reader`.
  - Checks:
    1. M opens `dm` on L, which reads → F lists L and sees `another member` → F revokes `dm` → the next call on `dm` is refused immediately.
    2. F opens `df` on C → M gets the generic refusal for both list C and revoke `df` → `df` still reads.
    3. Cross-Capsule revoke attempts are refused.
    4. After each revoke: L's and C's `/c/…` visitor pages still answer in a browser, and the Pakkana app still opens both.
  - Never rotate a visitor link.
- [ ] **Evidence:** record command output with every URL redacted, and post a World handoff note. Then the founder decides on merge (lme-remote PR, lm-cli #7 + this branch) and, separately, on the npm publish.

## Emergency revocation (founder's question 2)

The situation: the opener has lost their membership, lost their account, or is unreachable, and their LM door must stop now. Pakkana's custodian and every guest QR must keep working.

| Means | Who | Stops the door | Touches Pakkana or QR? | Phase 1 |
|---|---|---|---|---|
| The 90-day cap | Nobody, automatic | ≤ 90 days | No | ✅ built in (Task 1) |
| Opener runs `lm doors revoke` | The opener | Immediately | No | ✅ Task 4 |
| **E1. Operator break-glass**: stop the service, run `node dist/capsule-grants-admin.js revoke --door <id> --apply --service-stopped` as the service user with its env (see Task 9 Deploy), start the service | Founder (ssh) | Immediately; about 10 s of LME downtime | No: removes only `grant` rows (tested) | ✅ Task 5 |
| **E2. Any current invite-capable member** of the Capsule can revoke any LM grant on it | Owner/controller (and every legacy member) via `lm` | Immediately, no downtime | No | ✅ Task 6 (approved) |
| Ending the room (`ons_`) | — | Yes, but it destroys the Capsule for Pakkana and kills the QR | **Yes** | ❌ never used for this |

To find the door id when the opener is gone: the founder runs `node dist/capsule-grants-admin.js list --capsule <id>`. The output never prints a token.

**Decision E2: approved 2026-11-19** (spec section D). On legacy Capsules every joined member holds the invite capability, so every member may revoke any LM door there. That is Pakkana's existing rule, not a new power.

## Rollback

Fail-closed: the old version is never started while any capsule-read grant row may remain. Pre-change code would serve a surviving grant row as an uncapped read door with `world_list` and the handoff tools.

1. Stop the service. Confirm `systemctl is-active lme-remote` prints `inactive`. If not, **STOP**.
2. With the CURRENT (new) dist, run `node dist/capsule-grants-admin.js revoke --all --apply --service-stopped`. If it exits non-zero, **STOP** and leave the service down. Starting the new version again is a safe fallback; starting the old one is not.
3. Run `node dist/capsule-grants-admin.js verify-none`. It must exit 0 and print `capsule-read grants: 0`. On any other result, **STOP** with the service down. Do not restore the old dist and do not start anything old. Escalate to the founder.
4. Only after step 3 passes: restore the previous dist from the deploy backup (the `lme-remote-deploy` skill procedure).
5. Before starting, run step 3's check once more against the token file. The verifier is in the new dist, so run it from the saved copy at `/opt/lme-remote/rollback-verify/capsule-grants-admin.js`, copied there at deploy time (Task 9, Deploy). Then start the service.
6. History records `revoked` for every removed row.
3. Data left behind is harmless to old code: the `HistoryRecord.capsuleId` field is ignored, and the bridge state files are unchanged in format. One visible effect: old code's `readHistory` does not filter on `capsuleId`, so revoked/expired Capsule grant rows show in the World `access_list` for up to 30 days. Cosmetic, accepted.
   (All operator-tool commands in steps 2, 3 and 5 run as the service user with the service's env — see Task 9 Deploy.)
4. CLI: nothing is published, so rollback means not publishing. lm-cli #7 stays open.
5. Pakkana: no change in Phase 1, so no rollback.

## Tracked exposure KSE-1 (not Phase 1 work)

The status stays as the spec states: known, accepted, tracked as advisor-inbox A38. Phase 1 adds no path to visitor tokens and changes none.

Cautions while Phase 1 is in use, for Capsules that already have a public QR:
- `lm doors list` will not show the QR. It folds under the custodian row, whose label says Pakkana manages it. Revoking every LM door does **not** close the QR's raw `/door/<t>/mcp` access.
- Do not use a Capsule with a public QR for J1, J2 or J5 markers. Use fresh test Capsules.
- J3 must not rotate a visitor link. Rotation kills printed QRs.
- If someone needs the QR closed now, the only lever is the owner rotating it in Pakkana (accepting that printed codes die), until M1/M2 ship.
- Watch: the room's 300 ops/day budget errors in `journalctl -u lme-remote` for a Capsule nobody is using are a sign of QR scraping.

## Self-review notes

- Spec coverage:
  - D1 → Global Constraints
  - D2 → Task 7/8 (ID arguments)
  - D3 → Task 1 (`findCapsuleRoom` + one tuple)
  - D4 → Task 3
  - D5 → Task 8
  - D6/D7 → Tasks 1–2
  - D8 → Task 3 `can_open_door` + Task 4
  - D9 → Task 4 rows
  - Ownership bootstrap → Task 3 (link first) + J1 web check
  - Door lifetime → Tasks 1–2 + J6
  - `last_used_at` → Task 2
  - KSE-1 → section above
  - Emergency → Task 5/6
- Spec deltas, APPROVED 2026-11-19 and recorded identically in spec section D:
  1. Positional CLI arguments instead of `--note` / `--label`.
  2. `role` gains `'member'` for legacy device-joined Capsules, which have no managed role.
  3. A 32-grant limit per Capsule, mirroring Pakkana.
  4. Grant reads extend the room's idle horizon, like every admitted op today (`shouldRefreshOnsExpiry`): an AI reading a Capsule keeps it alive while its grant lives.
