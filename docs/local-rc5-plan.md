# Local activation: MCP 0.1.3 + CLI 0.1.0-rc.5

Prepared: 2026-10-02, Asia/Bangkok.
Status: approved October 2; implementation and release candidates in progress, not published.

Owner steering at 21:42: OpenRouter is the first workshop path. Provide private
`lm.config` credential entry and fetch a live embedding-model picker; do not ask
for the owner key or hardcode credentials/model choices. Authenticated provider
acceptance is through the user-configured CLI flow.

## Outcome

A person with another Mac can install `lm`, create a Local Memory Capsule,
remember something, close the process, recall it in another process, and connect
an agent to that same store. `lm` presents Local, Room and World together while
showing their actual storage, computation and access boundaries.

Local means the canonical store is on the device. Embedding may run on the
device or at a remote provider. Local does not imply that computation is offline.
Remember/recall do not require a chat model. Do not add an unused LLM setting or
claim chat, extraction, consolidation or crystallization is implemented.

## Release and naming

- CLI target: `0.1.0-rc.5`; keep executable `lm`.
- Local MCP target: `0.1.3`.
- Proposed canonical npm name: `@nature-labs/living-memory-mcp@0.1.3`.
- Compatibility name: `@nature-labs/lme-mcp@0.1.3`, delegating to the canonical
  implementation at an exact version. Existing registrations retain their name,
  four original tools, environment configuration and snapshot path.
- Maintain one implementation, not two diverging servers. Both package entry
  points must pass the same packed-artifact tests.
- Naming is a proposal for owner review before publication. Registry returned
  404 for the proposed name on October 2; this is not proof of publish permission.
  If naming is deferred, release the implementation under the existing name and
  pin CLI to that name. Naming must not block local correctness work.
- Do not deprecate the old name, change existing client registrations, publish
  npm packages, create a public release, deploy the installer, or merge GitBook
  changes without the applicable owner authorization.

## Evidence reused

On October 2 the published `@nature-labs/lme-mcp@0.1.2` was downloaded from npm
and installed into an isolated temporary directory with lifecycle scripts off.

- Registry latest: 0.1.2, last modified August 15. README and handshake: 0.1.1.
- Published smoke passed: add in process A, close A, recall in process B, state,
  and forget. This used the mock embedder, not a real semantic provider.
- Existing CLI tests passed: `go test ./internal/cli`.
- CLI uses HTTP MCP; the local package uses stdio. Wiring is missing.
- Local tools: memory_add, memory_search, memory_state, memory_forget. No handoff.
- A controlled loopback embedding fixture reproduced silent empty retrieval
  after changing from two-dimensional to three-dimensional embeddings.
- The same fixture reproduced a lost write: retrieve loads a snapshot, waits
  for embedding, another instance adds a memory, then retrieve saves its stale
  snapshot and removes the newly added memory. Search is a write operation.
- Reproduction script: `/private/tmp/lm-local-audit-20261002/boundaries.mjs`.
  Temporary evidence is not a durable test; port these cases into regression tests.

## 1. Make the local runtime safe

Work in `living-memory-engine/lme-mcp`; reuse the engine and embedding adapter.
Do not upgrade the frozen engine tarball as an incidental dependency change.

1. Serialize complete load/compute/save operations, including search, state and
   forget as appropriate. Share the protection across CLI and MCP processes.
   Locking only `save()` does not prevent stale snapshot overwrites.
2. Use an exclusive per-store operation lock with bounded, actionable failure.
   Handle cleanup in `finally`. Never automatically steal a lock based only on
   age. Report stale locks through diagnostics; recovery must be explicit.
3. Bind the store to embedding identity: mode, endpoint identity, model and
   observed dimensions. Validate finite nonempty vectors and consistent lengths.
   Same dimensions alone do not prove compatibility. Reject incompatible use
   before writing; do not auto-migrate or auto-reembed.
4. Treat pre-0.1.3 stores as legacy/unverified. State can remain readable;
   semantic add/search need an explicit adoption path with a backup and owner
   acknowledgement of the original provider/model. Do not guess old provenance.
5. Newly created directories/files must be private (0700/0600). Check unsafe
   paths and avoid silently treating corrupt stores as new empty stores.
   Preserve backup recovery with an explicit diagnostic.
6. Keep original tool names and human text; add structured results for machine
   consumers and runtime metadata sufficient for inspect/doctor. Align package,
   handshake and README versions. Never expose API keys in metadata or errors.
7. Support explicit offline lexical/hash mode, clearly labeled non-semantic.
   Do not silently switch from a failed semantic provider to mock retrieval.
   Check Unicode token handling so Thai workshop examples are meaningful.

Verification: regressions for dimension/model mismatch, interleaved search/add,
same-process concurrent calls, lock contention/cleanup, corrupted files and
privacy permissions; original smoke and embed-failure tests; typecheck/build.
Tests use disposable stores and a loopback fixture, without paid API calls.

## 2. Connect Local to the existing CLI

Work in `lm-cli`. Preserve HTTP Room/World paths and private grant export/import.

1. Add a stdio MCP client behind the existing discovery/call contract. Support
   initialize, initialized notification, tools/list and tools/call on one session;
   bound messages and timeouts, separate stdout/stderr, and close child processes.
   Execute argv directly, never through a shell.
2. Use a verified, exact-version local package installation. Steady-state commands
   must not invoke an unpinned `npx -y` that fetches new code on each call.
3. Add `lm create <name> --local`. Store local configuration separately from
   remote door credentials; each Local has its own store and stable local ID.
   Creating another place never copies the contents of an existing place.
4. Support `local:<name>` selectors alongside room:/world:. Bare-name collisions
   require an explicit selector. An explicit Local selector must work without
   consulting account inventory, OAuth or hosted services.
5. Route remember/recall/state through actual discovered tools. Include Local
   rows in list and give inspect truthful local storage/provider/network details.
   List should not probe models or send local memory to a provider.
6. Existing `export`/`import` remain remote-grant operations; reject Local there
   with an explanation. Do not accidentally turn grant export into store export.

Verification: fake stdio server protocol/error tests; real packed-package
CLI-to-MCP add/recall across processes; mixed inventory and selector collisions;
regression checks for existing Room/World commands. No production probe needed.

## 3. Make setup usable by people and agents

1. Provide `lm setup local` to install/check the pinned runtime and explain Node
   requirements. Keep Room/World commands usable without Node. Local creation
   cannot be labeled ready if its runtime is missing.
2. Provide small per-Local embedding configuration controls. First provider
   presets: Ollama, LM Studio, OpenRouter and generic OpenAI-compatible. Reuse
   the shared HTTP contract; specialize only verified provider differences.
3. Require an explicit embedding model for semantic mode, with discovery where
   available. Use an environment-variable reference for a remote API key;
   never store a key in a public command example or print it in inspect/JSON.
   Add hidden `lm config keys` input and private LM_HOME/lm.config (0600), with
   environment precedence and a live OpenRouter embedding-model picker.
4. Inspect shows configuration, stable ID, store path, embedding mode/provider/
   model/dimensions or unknown, runtime readiness, no hosted Door, persistence
   and network boundary. Do not say "no network" for a LAN/remote provider.
5. `lm doctor local:<name>` checks runtime, private storage, lock state, configured
   key availability and known embedding identity. Explicit `--probe` may contact
   the provider with a generic diagnostic string, never stored memory. Explain
   that the probe can incur provider charges; missing config is not readiness.
6. Human errors name the next corrective command. `--json` uses stable typed
   fields, unknown/null values and meaningful exit codes. Sanitize terminal text.

Verification: one fresh temporary installation on macOS; create/doctor/remember/
new-process recall; Thai and English examples; unavailable runtime/provider,
missing key and incompatible model cases. Verify real on-device semantic recall
only if a suitable local provider is available. Otherwise report that gap rather
than equating mock/fixture success with semantic-provider acceptance.

## 4. Finish the continuation path

Add local handoff_post/handoff_read, with list if required for choosing an older
note, so CLI handoff/resume work consistently on Local as well as hosted places.
Notes are raw, ephemeral, unembedded and separate from durable memories.
Use explicit creation/expiry timestamps, default 24 hours and maximum 72 hours;
expired notes are unavailable. Apply the same private-storage/concurrency rules.
Do not make a handoff upload data or create a Door.

Verification: post in process A, resume in B; exact text readback, deterministic
expiry, concurrent notes and discovered capabilities. Existing hosted behavior
continues unchanged.

## 5. Package, document and release

1. Write CLI/MCP quickstarts from the commands actually implemented. Explain
   storage versus computation; semantic versus lexical recall; legacy adoption;
   runtime prerequisites; network exposure; and Local/Room/World boundaries.
2. Update GitBook `content/local.md`, `content/cli.md`, choose-your-path and
   reference pages in the same implementation pass. Work from the current synced
   content. Keep Thai translation deferred. Use one authoring path (GitHub PR),
   not a simultaneous GitBook change request and repository import.
3. Build MCP packages and test installed tarballs, including compatibility entry
   point and two-process persistence. Check dependency versions and package
   allowlists; exclude configs, stores and secrets.
4. Build CLI RC5 for existing macOS/Linux arm64/amd64 targets with checksums.
   Execute the macOS artifact; distinguish cross-compilation from native testing.
   Do not expand this release to Windows or notarization incidentally.
5. Prepare installer/site RC5 updates only after the matching release artifacts
   exist. Show the Local quickstart and runtime requirements. Review the final
   diffs and required checks before owner-approved publication.
6. Publish canonical MCP then compatibility MCP, then CLI prerelease, then update
   installer/site and merge docs. Attach release/PR receipts. Download the public
   artifacts back and run one focused fresh-install smoke. Verify docs sync/live
   commands; do not redo unchanged production/payment flows.

## Completion criteria

- A fresh Mac has a clear path to a ready Local, with semantic provider or an
  explicitly chosen and honestly labeled lexical mode.
- A memory survives processes; two callers cannot silently lose each other's
  writes; changing embedding identity cannot silently hide existing memories.
- list/inspect/doctor and --json expose truthful Local/Room/World boundaries.
- Agent MCP and CLI can access the same Local store safely; handoff/resume work.
- Existing Room/World commands and old package registrations retain their contract.
- Release artifacts, installer, README and English GitBook describe the same
  version and behavior; test/release limitations are recorded.

## Deliberately outside this release

Automatic migration, full-store transfer, selective memory-transfer UI/commands,
chat/LLM inference, consolidation changes, hosted Door/tunnel creation from Local,
co-op/human invites, billing changes, affiliate/workshop distribution, Windows,
Thai docs translation and rewriting the core engine.

Current frontier: implementation and authenticated OpenRouter acceptance pass.
Prepare reviewed source PRs; publication follows the dependency order above.
