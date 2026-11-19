# lm — Capsule-first CLI acceptance contract

Date: 2026-11-19 · Status: APPROVED by founder 2026-11-19 ~04:22 ICT (D1–D9, J1–J6); E2 + deltas + J7 approved ~04:33 ICT.
Plan: `../plans/2026-11-19-capsule-first-cli.md` (awaiting review); no code
Supersedes the scope (not the history) of `2026-11-18-lm-v1-read-door-design.md`.
Sources: handoffs `h_mwmbcgou` (Capsule-first goal), `h_mwmitilv` (recursive Capsule
canon), founder messages 2026-11-19; code read of lme-remote `330c72f`, pakkana
`1587258`, pakkana-web `9f03e1c`, lm-cli-ts `4c220c1`; prod census 2026-11-19.
Ledger: `living-memory/advisor-inbox.md` A31–A38.

DONE-WHEN (proposed): on production, a freshly signed-up account that has no World
and has never used Pakkana runs `npx @nature-labs/lm`, creates Capsules A and B,
opens a read-only door on A, and an unmodified external MCP client (LM Studio or
Claude Desktop) answers a question from A's memory through that door while a
marker phrase stored only in B never appears. `lm doors list A` shows the door
with a real `lastUsedAt`; `lm doors revoke` withdraws it; the next call from the
client is refused; `lm doors list A` shows it revoked. A Pakkana visitor QR for
an existing Capsule still opens afterwards. Journeys J1–J7 below all pass.

---

## A. Existing implementation (proven)

| Area | LME (lme-remote) | Pakkana (pakkana-api / web) | lm-cli-ts |
|---|---|---|---|
| Capsule identity | Mints the room: `POST /ons/new` → `roomId = w_ + sha256(ns)[:32]`, write URL `/t/ons_…`, read URL `/t/ro_…` | `places.id` = that `roomId`; `places.url` keeps the write URL; the `ro_` URL is discarded | — |
| Ownership / membership | None for Capsules (an ONS room has no `owner`). Owns World ownership (OAuth subjects). Account Free Room = `freeRoomToken` pointer, not ownership | `places.createdBy` (provenance), `members`, `member_capabilities` (read/write/invite/manage), `room_memberships` (controller/contributor/reader). Flag `PAKKANA_ROOM_MEMBERSHIP=1` on prod, set outside `wrangler.jsonc` | — |
| Access grants | Token rows: `ons_` (write, held by Pakkana), `ro_` (read, unheld), `lmc_` client doors (World only, `client_mint`, readOnly since #60). Read-only = write tools never registered | `doors` (read/write, 90 d, ≤32 live/place, list/revoke bound to issuing device); visitor link = read door, 10 y, sealed URL, served at `/c/<t>` (web) and `/door/<t>/mcp` (raw MCP) | `doors list/open/revoke` → World-wide `client_mint`; `access list` → `access_list` |
| Capsule tools | `CapsuleBridge`: one Pakkana device per OAuth subject, stored on the LME box; `/rooms/identity` links it to the subject. `capsule_list` shows only bridge-created Capsules | `/places`, `/places/create`, `/rooms/*`, `/places/door*` behind bridge token + device token | none |
| Free account (no World) | Gets Free Room + Capsule tools; does NOT get `client_*` / `access_list` (`app.ts:1083`) | — | unusable (World-only commands) |
| Lifetime / capacity | Enforced on token rows (`expiresAt`, `paidUntil`); capacity = constant 1 MiB for every ONS room | `lifetime_purchases` (billing), calls LME `/space/lifetime`; `room_status_cache` | — |
| Conversation / assets / memory | Memory only | Conversation: `capsule_questions`, `agent_turns`, `conversation_context`. Assets: `event_media` in R2 (`pakkana-evidence`). A guest question becomes memory only when a member answers it (`inbox.mjs:101`) | — |

Prod census 2026-11-19: 100 ONS rooms (96 with an `ro_`), 16 OAuth subjects, 10 Free
Rooms, 9 bridge-created Capsules (one subject), 1 live `lmc_` door.

## B. Authority boundary

| Data | Source of truth today | Who authorizes / enforces today | Direction (canon) |
|---|---|---|---|
| Capsule identity | LME (mints `roomId`) | — | LM ✓ already |
| Name | Pakkana | Pakkana | LM later |
| Ownership, membership | Pakkana | Pakkana | LM later (Phase 2) |
| Memory | LME | LME, by credential only (whoever holds `ons_` can write all) | LM ✓ |
| Access grants | **Split**: LME token rows + Pakkana `doors` | **Split**: LME for its tokens, Pakkana for its doors, both riding one `ons_` | LM (Phase 1 for CLI grants, Phase 2 for the rest) |
| Lifetime, capacity | LME (enforcement) + Pakkana (purchase records) | LME | LM ✓ already |
| QR, guest sessions, media, wallet, checkout | Pakkana | Pakkana | Pakkana ✓ (client product state) |

Mismatches found:

1. **Revocation is not symmetric.** LME cannot revoke a Pakkana door, and Pakkana cannot
   revoke an LME token. The only LM-side lever over Pakkana access is the `ons_` master
   credential, which ends everything (QR included).
2. **Read-only means two different things.** LME: write tools are not registered. Pakkana:
   a name allowlist (`doors.mjs:20`) in front of the write-capable `ons_` credential.
3. **The visitor QR token is a raw read grant.** `/c/<t>` and `/door/<t>/mcp` take the same
   token, so whoever extracts it from a QR can attach any MCP client and read raw memory
   and original photos. The canon says a guest should not hold an LM grant (A38; see KSE-1).
4. **Unheld read grants.** Each ONS room has an `ro_` read token that Pakkana discards.
   Those for Pakkana rooms are live credentials with no known holder.
5. **Deploy drift.** Pakkana membership flags exist only on the deployed version, not in
   `wrangler.jsonc`.

## C. Compatibility with the Capsule relationship and worker canon

Nothing in the proposed primitive conflicts, provided the decisions in D hold. Every
grant, capability, lifetime and capacity is already per Capsule, and no parent concept
exists to leak through. Constraints to carry (not to fix now):

- **Identity coupling.** Today `capsule_id == roomId`. That holds while the room never
  moves. Clients must treat the id as opaque so a later re-host (for example a larger
  Capsule) keeps its id.
- **Capacity is per plan, not per Capsule.** "Child larger than parent" needs a
  per-Capsule capacity field (same shape as `paidUntil`). Future.
- **Pakkana is custodian and worker at once.** It holds the only write credential, so a
  future non-Pakkana worker has no LM grant of its own. Phase 2 gives workers scoped LM
  grants.
- **No consent model for traces.** Conversation and assets are already keyed by place, so
  a future export is possible. Export and training rights do not exist yet and must be
  separate grants when they do.

## D. Decisions

Locked by the founder (2026-11-19):

- D1 The Capsule ID is opaque and stable; never parsed, never mapped to a World.
- D2 Capsules are addressed by ID; names are labels, not identity.
- D3 One LM door grants exactly one Capsule. No "include children" mode, ever (A33).
- D4 `capsule_list` returns every Capsule the account can access, not only bridge-created ones.
- D5 The World-wide door leaves the CLI journey. The backend `client_mint` stays for its
  existing users (Codex, doo-dd, pakkana-web) and lm-cli #7 is not merged as the user path.
- D6 The CLI door is raw read. Guest QR / ask-only is Pakkana's concern and is not moved.

**Locked after the audit (founder, 2026-11-19):**

- **D7 — LOCKED 2026-11-19: L.** LME issues and enforces the Capsule-scoped read grant
  directly. v1 is text-only; media/photo access is separate later work and must not
  lean on Pakkana doors.

  | | L: LME-native grant on the Capsule's room | P: Pakkana door via the bridge |
  |---|---|---|
  | Who enforces read-only | LME, write tools never registered | Pakkana allowlist in front of `ons_` |
  | Who revokes | LME (the canon's authority) | Pakkana |
  | Visible in "who holds keys" | Yes, as an LM grant | Only to the issuing device |
  | Door host | `lme.viibe.to` | `pakkana-api.viibe.to`: the CLI would become a Pakkana client |
  | Photos | **No** (assets live in Pakkana R2) | Yes (`evidence_*`) |
  | New backend | 3 LME tools + one token-row kind | 3 LME tools that forward to existing routes |

  L matches "LM is the authority of memory access grants" and the "CLI is a client of LM,
  not of Pakkana" framing, without migrating ownership. Issuing still asks Pakkana whether
  the caller may (the existing membership authority). The cost is text-only reads in v1.
- **D8 — LOCKED 2026-11-19: a + i.** Only members with the invite capability
  (owner/controller) may open a door; readers and contributors see the Capsule but
  cannot. Mirrors Pakkana's `doors.mint`; capability, never `createdBy`, decides. The
  capability is checked at issuance only, and a grant lives at most 90 days.
  Accepted v1 risk: a grant can outlive its issuer's Pakkana membership by up to 90 days
  (LME is not told of a removal). Bounded because only owner/controller can issue.
  Phase 2 closes it (Pakkana notifies LME, or membership moves to LM). Rejected: b (any
  reader; makes the stale-grant case common), c (`createdBy`; contradicts the
  2026-11-01 "createdBy is provenance" decision); ii webhook and iii per-use check
  (deferred to Phase 2 / would make LM reads depend on Pakkana uptime).
- **D9 — LOCKED 2026-11-19: a + a1.** `lm doors list <capsule>` shows every LM grant on
  that Capsule's room, with the holder shown as `you (label)`, `Pakkana (custodian,
  write)` or `unclaimed read link`. Pakkana's own doors and QR fold under the custodian
  row and are not listed one by one. The custodian row can never be revoked from the
  CLI, because that would end the Capsule for Pakkana, QR included. Unclaimed `ro_` rows
  are shown only, never revoked, in v1.
  Founder's rationale: each system shows what it governs. Pakkana users (most people)
  see their own encapsulated circle in Pakkana; LM sees every grant it manages itself and
  does not reach into Pakkana's delegations. That is the natural boundary, not a gap.

**Locked at plan review (founder, 2026-11-19 ~04:33 ICT):**

- **E2 — Co-member revoke.** Any member who CURRENTLY holds the invite capability on a
  Capsule (Pakkana `canShare === 1`, checked at call time) may revoke any LM grant on
  that Capsule, including another member's. The custodian and unclaimed rows stay
  unrevocable. Emergency path when nobody with the capability is reachable: operator
  break-glass (plan Task 5), which removes grant rows only. Ending the room is never used.
  Consequence to know: Pakkana gives every member who joins a *legacy* (non-managed)
  Capsule the full capability set (`grantFreeParticipant` → read/write/invite/manage =
  1). On legacy Capsules, every member may therefore open doors and revoke any LM door.
  On managed Capsules only the controller may, because `controller` cannot be invited
  (Pakkana invitable roles: `reader`, `contributor`). This matches Pakkana's own door
  rule. It is not a new grant of power.
- **Spec deltas (approved):**
  1. The CLI uses positional arguments (`lm capsules create <name> <note>`, `lm doors
     open <capsule-id> <label>`). The parser has no value flags.
  2. `role` gains `member` for legacy device-joined Capsules, which have no managed
     role.
  3. A Capsule holds at most 32 live LM grants, mirroring Pakkana's door limit.
  4. Reads through a grant extend the room's idle horizon, like every admitted op
     today (`shouldRefreshOnsExpiry`). An AI reading a Capsule keeps it alive while
     its grant lives.

Deferred, with no migration cost if D1–D9 hold: nesting, worker registry/assignment,
per-Capsule capacity, ownership and membership in LM, Pakkana grants moving into LM,
QR ask-only split, `ro_` cleanup, trace export, consent separation for training, photos
through LM doors.

## E. Proposed backend contract (LME, Phase 1)

These tools sit in the Capsule tool set, which both the World mount and the Free Room
mount get. They are offered on `/cli/mcp` and withheld from agent callers, as
`client_*` is today.

- `capsule_list {}` → `{ capsules: [{ capsule_id, name, role, can_open_door, expires_at }] }`.
  It links the identity first, then shows every non-World place the subject can read.
  `role` is one of `owner | controller | contributor | reader | member`.
- `capsule_create {name, content}`: existing tool, unchanged.
- `capsule_door_open {capsule_id, label}` → `{ door_id, label, access: 'read', url,
  expires_at, capsule_id }`. The URL is returned once and never stored readable.
  Requires `can_open_door`. LME resolves `capsule_id` to the room from its own records:
  it minted that id. It never takes a credential from Pakkana.
- `capsule_door_list {capsule_id}` → rows per D9 with `status` (`live | expired |
  revoked`), `last_used_at` (hour granular), `created_at`, `expires_at`. Revoked and
  expired rows are kept for 30 days (same as `access_list`).
  Authorization policy (checked per call, for exactly that `capsule_id`):

  | Caller | Sees |
  |---|---|
  | No membership, unknown id, World id, or Capsule swept on LME | Refusal (generic message), identical in every case |
  | Member without the invite capability (`reader`, `contributor`, managed `member`) | Refusal (generic message). The door list is an administrative view, and these callers can open no door |
  | Member with the invite capability (owner, controller, legacy member) | Every live LM grant on that Capsule's room, plus that Capsule's revoked/expired history (30 days) |

  What a permitted caller sees of another member's grant: `holder: 'another member'`,
  label, status, dates and `last_used_at`. Never the other member's subject, email or
  token, and never their grants on other Capsules.
  Cross-Capsule isolation: rows are selected by the grant's own `capsuleId` AND the
  room tuple of `capsule_id`. History is selected by `capsuleId`. A door id from another
  Capsule is never listed, and is refused by revoke.
- `capsule_door_revoke {capsule_id, door_id}`. Allowed when the grant belongs to
  `capsule_id` and either the caller opened it, or the caller currently holds the
  invite capability on that Capsule (E2). Never allowed for the custodian or unclaimed
  rows.

Each refusal returns the same "Capsule unavailable for this account" message, so the
response never reveals whether a Capsule exists.

Grant semantics (shape decided in the plan, behaviour fixed here):
- It opens exactly that room. A request is served only while BOTH are true: the
  grant's own expiry (90 days from issue, fixed at issue) has not passed, AND the room
  is live (not expired, swept or purged). Room activity and paid Lifetime extend the
  room, never the grant.
  Constraint found in code: `touchOnsActivity` (`ons.ts:288-306`) writes the room
  horizon onto every `plan:'ons'` row of the room. A grant shaped like today's `ro_`
  row would have its 90-day cap overwritten by the next room activity. The plan must
  keep the grant's own expiry out of that propagation and check room liveness at use.
- `last_used_at` is updated only by an authorized MCP request: the grant passed every
  check above and is not revoked. A request refused for an unknown, expired, revoked
  or room-ended grant never updates it, and neither does a GET of the entrance page.
  A tool-level error after authorization (bad arguments) still counts, because the
  holder proved possession of a live grant. That is what this field is for. Today's
  rule is close: `app.ts:1951` touches only after token authorization, but also
  counts GET.
- Write tools, `capsule_*`, `client_*`, `room_*`, `handoff_post` and `world_raise` are
  never registered on it.
- Every HTTP route that accepts a `/t/` bearer either treats it as read or refuses it.
  This is the #60 lesson: `/world/raise` had to be closed separately.
- Revocation removes the row. The next request fails with no cache window.

## F. CLI surface (lm-cli-ts)

```
lm login | logout | whoami
lm capsules list                       # id, name, role, door?, expires
lm capsules create <name> <note>
lm doors open <capsule-id> <label>     # prints the URL once
lm doors list <capsule-id>
lm doors revoke <capsule-id> <door-id>
lm doctor door <url>
```
`lm doors` with no Capsule ID is an error that points to `lm capsules list`. `lm access
list` (World-wide) is no longer part of the user journey (D5).

## G. Journeys and acceptance checks

Test accounts: **N** is a new Stytch account (no World, no Pakkana history). **M** is a
second new account. **F** is the founder (paid World, ~30 Pakkana Capsules).

**J1 — Fresh free account, first Capsule.** N: `login` → `whoami` shows no World →
`capsules create A "<marker-A>"` → `capsules list` shows A with role `owner`,
`can_open_door: true` → `doors open <A-id> lmstudio` prints a URL → the client calls
`tools/list` (read tools only) → `memory_search` finds `<marker-A>`.
✓ No World is required. ✓ The URL host is `lme.viibe.to`. ✓ No write tool is listed.
✓ N signs in to Pakkana web with the same Stytch account and sees A. This proves the
LME subject and the Pakkana account subject are the same `sub`; it is unverified today.

*Ownership bootstrap: where `owner` / `can_open_door` come from.* LME stores no
Capsule owner (Phase 1 does not change that). Pakkana stays the membership authority.
The chain, all existing code:
1. N's first OAuth request on `/cli/mcp` has no World. `serveAccountFreeRoom` mounts
   the Free Room and passes `capsules = CapsuleBridge.forSubject(sub)` (`app.ts:954-956`).
2. The first `capsule_create` finds no bridge state for (issuer, sub) and calls
   Pakkana `POST /device/register`. The new device token D_N is stored on the LME box
   (`capsules.ts:206-209`). D_N is a capability holder bound to N's subject. It is not
   a Pakkana login.
3. `POST /places/create` as D_N: Pakkana mints the room through LME `/ons/new`, stores
   `places(createdBy = D_N)` and `grantFreeParticipant(D_N, A)`, which gives
   `member_capabilities = read 1, write 1, invite 1, manage 1` (`places.mjs:150-155,
   341`). With `manage_members`, it also makes a `controller` membership.
4. Phase 1 change: `capsule_list` first calls `/rooms/identity` (link D_N ↔ `sub`), so
   Pakkana's `syncAccount` gives the same capabilities to every device of N's account
   (web and app included), and the reverse.
5. `can_open_door` = Pakkana `canInvite` for D_N, read from `/places` at issuance (D8).
   `role: owner` is the display of Pakkana's `role = 'created'`. It is never used for
   authorization (2026-11-01: createdBy is provenance).
So for a fresh account, "owner" means "created this Capsule through the bridge and
holds invite capability in Pakkana". No LME-side owner record is created.

**J2 — Multiple Capsules, isolation.** N creates B with `<marker-B>` and opens a door on A.
✓ `memory_search "<marker-B>"` through A's door returns nothing from B. ✓ `doors list A`
does not show B's grants and vice versa. ✓ `doors open <B-id>` from M is refused with the
generic message. ✓ `doors revoke A <B's door-id>` is refused.

**J3 — Paid World account.** F: `capsules list` shows F's Pakkana Capsules (created in the
app/web), not only bridge ones, and not F's World. F opens a door on one Pakkana Capsule
and reads it. ✓ The World-wide door is not offered. ✓ The Capsule's visitor QR `/c/…`
still opens and answers afterwards.

**J4 — Invited member.** F invites M to Capsule C as `reader` (existing room
invitations). M: `capsules list` shows C with `role: reader`, `can_open_door: false`.
✓ `doors open C` is refused. ✓ After F revokes M's membership, C disappears from M's list.

**J5 — Revoke and multiple doors.** N opens two doors on A (`d1`, `d2`) and uses both
once. ✓ `doors list A` shows both with `last_used_at` and the custodian row.
`doors revoke A d1` → ✓ the next call on `d1` is refused, ✓ `d2` still reads, ✓ `d1`
shows `revoked`. ✓ `lm doctor door <d1-url>` exits 1.

**J6 — Capsule ends after a door was issued.** This is separate from the D8 stale-
membership risk: here the Capsule itself ends, not the issuer's membership. It runs in
the lme-remote suite with an injected clock and a fixture room, because on prod a room
takes 21 idle days to expire. N opens door `d` on A, and `d` reads.
✓ Room A passes its horizon while `d`'s own 90-day expiry has not → `d` is refused.
✓ A is swept (its rows are removed) → `d` is refused, and `doors list A` answers
"Capsule unavailable".
✓ Activity on A and a paid Lifetime both extend the room. Then, at day 90 from issue,
`d` is refused although the room is live.
✓ Refused requests in these cases leave `last_used_at` unchanged.
Note: no user-initiated "delete Capsule" exists today, in Pakkana or LME (`purge.ts`
covers Worlds only). When one is added, it must end every LM grant on that room in
the same step.

**J7 — Co-member revoke (E2) and list policy.** On prod. Use a *legacy* Capsule L (F
creates it with `lm capsules create`, so it is not managed) and a *managed* Capsule C
(F creates it in Pakkana with membership enabled). M joins L through Pakkana's invite
link and has `role: member`, DOOR yes. F invites M to C as `reader`.
✓ M opens `dm` on L, and `dm` reads. F runs `doors list L` and sees `dm` as
`another member` with M's label and no subject or email. F revokes `dm`, and the next
call on `dm` is refused immediately.
✓ F opens `df` on C. As reader, M gets the generic refusal for both `doors list C` and
`doors revoke C <df-id>`, and `df` still reads.
✓ M runs `doors list C` and `doors revoke L <df-id>`, both refused. A door id is never
honoured outside its own Capsule.
✓ After every revoke above, L's and C's Pakkana custodian still serves: each Capsule's
existing `/c/…` visitor page still answers in a browser, and the Pakkana app still
opens both Capsules. No visitor link is rotated.

Backend checks, run against the deployed build:
- The read grant cannot call any write tool by name, even one it does not list.
- Every bearer-accepting HTTP route: `/world/raise` is refused; `/space/status` is read
  only; `/space/lifetime` needs the seller key.
- A Free Room account and a World account behave the same through J1, J2 and J5.

## H. Phases

- **Phase 1 (this contract):** D7-L tools in LME, the bridge `capsule_list` fix, and the
  lm-cli-ts surface. Approve the spec, then write the plan.
- **Phase 2 — LM as the single authority of grants:** Pakkana's worker uses scoped LM
  grants instead of the master `ons_`. Pakkana doors register as LM grants or retire.
  The visitor QR becomes ask-only through the worker (stop serving `/door/<t>/mcp` for
  visitor links). Unheld `ro_` tokens are cleaned up. Membership moves to LM. Each step
  needs its own plan and approval.
- **Phase 3 — canon features:** nesting, per-Capsule capacity, worker registration vs
  assignment, trace export with separate export and training consent, photos through
  LM grants.

## I0. Known security exposure KSE-1 — Guest QR is a raw MCP read grant

Status: KNOWN, ACCEPTED FOR NOW, not fixed in Phase 1 (advisor-inbox A38). Found by code
reading; not exploited or tested on prod.

**What.** The visitor link token behind every guest QR (`/c/<t>` on pakkana-web) also
works at `pakkana-api.viibe.to/door/<t>/mcp` as a raw read door (`doors.mjs` scope
`read`). Anyone who sees the QR, a photo of it, or the URL can attach any MCP client
and, without going through Pakkana's agent:
- enumerate the Capsule's memory with `memory_state` / `memory_search`, including
  owner-answered inbox items and every future addition;
- list media with `evidence_search` and fetch 10-minute links to ORIGINAL photos and
  videos with `evidence_present`. Whether originals keep EXIF/GPS metadata is
  unverified.

The token lives 10 years unless the owner rotates it. It cannot write.

**Impact.** The guest experience is meant to be ask-only through Pakkana's Worker, which
can decline, summarise and cite. Raw access skips that layer: full enumeration instead
of answers to questions, plus original media. It is also likely an availability
exposure, from code reading: the door proxies through the room's `ons_`, and LME counts
the 300 ops/day budget per room (`ons.ts` `onsRoomKey`). A scraper could exhaust the
owner's daily budget.

**Why not Phase 1.** It does not touch LM grants. And the obvious fix breaks something
in use: LME `capsule_share` and `room_read_link` deliberately hand out and probe this
same `/door/<t>/mcp` (`capsules.ts:121-131, 449-453`). "Share with an AI" and "QR for
guests" share one token today, so they must be split, not switched off.

**Mitigation path (Phase 2, each needs its own approval):**
- M1. Split the token: the guest QR gets an ask-only scope, accepted by `/capsule/<t>*`
  only. An AI share link becomes a separate read grant (an LM grant, per D7) shown to
  the owner as such.
- M2. Stop serving `/door/<t>/mcp` for visitor-link doors once M1 has shipped.
  Printed QR codes keep working, because `/c/<t>` stays.
- M3. Per-door rate limit on the Pakkana proxy, so one holder cannot spend the room's
  daily budget.
- M4. Check and, if needed, strip location metadata from originals served to visitor
  scopes.
- Interim, already possible: the owner can rotate the visitor link
  (`visitorLink(..., {rotate:true})`). It ends the old token, and every printed QR with
  it.

## I. Risks to watch in Phase 1

- **Proxy token:** the LM grant never touches Pakkana or `ons_`, so it adds no proxy
  path.
- **The bridge state file** (one Pakkana device token per subject, on the LME box) becomes
  load-bearing for `capsule_list`. If it is damaged, the result must be a refusal, never
  a silent empty list. It already refuses today (`capsules.ts:205`).
- **The `ons/new` IP quota** (5 per day per address) limits test runs of J1/J2 from one
  network.
- **Pakkana flags outside the repo:** a plain `wrangler deploy` could turn membership off
  and break J3/J4. Move the flags into `wrangler.jsonc` before the next Pakkana deploy.
