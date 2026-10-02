# Local memory (RC5 candidate)

This source targets `0.1.0-rc.5` and `@nature-labs/living-memory-mcp@0.1.3`.
Publication is pending; the public installer still serves RC4.

## Start

Install Node >=20.12 and npm if they are missing, then:

```sh
lm setup local
lm config keys
lm create notes --local --provider openrouter
lm doctor local:notes
printf '%s' 'เชียงใหม่ is my workshop base' | lm remember local:notes
printf '%s' 'เชียงใหม่' | lm recall local:notes
lm list --json
lm inspect local:notes --json
```

Setup installs an exact local MCP package version with npm lifecycle scripts off.
Later operations use the installed file, not an unpinned npx fetch. Room/World
commands do not require Node. Each Local has a stable local ID and its own store
under LM_HOME/locals/<name>. Interactive creation offers OpenRouter first and a live model picker.
Use `--lexical` explicitly for Unicode hash retrieval without a semantic model;
non-interactive creation without provider options retains that labeled mode. Facts are durable files; their vectors are representations.

## Private key and live model selection

`lm config keys` reads the key with terminal echo disabled and writes
`LM_HOME/lm.config` privately (0600). Ctrl-C cancels input and restores terminal
echo. The file accepts `VARIABLE=value` lines and never executes their contents.
Existing environment variables override saved values. `--key-env VARIABLE`
selects another variable; the command takes the value from private stdin, not a
command argument. Never paste a key into a shell command or share lm.config.

```sh
lm config keys
lm models --provider openrouter
lm create notes --local --provider openrouter
lm doctor local:notes --probe
```

The create command fetches live embedding model IDs and pricing, then accepts a
number or exact ID. It does not choose a hardcoded model. For an agent:

```sh
lm models --provider openrouter --json
lm create notes --local --provider openrouter --model CHOSEN_MODEL_ID --json
```

Only OpenRouter's dedicated embedding catalog is used, not the chat-model list.
Prices reflect the returned catalog, not a guaranteed future rate. Model vector
dimensions remain unknown until an explicit probe or the first stored fact.

## Semantic embedding

Create a separate Local; a populated store's embedding identity cannot change.

```sh
lm create semantic --local --provider ollama --model embeddinggemma
lm doctor local:semantic --probe
# LM Studio uses the embedding model ID loaded in that server:
lm create studio --local --provider lmstudio --model YOUR_EMBEDDING_MODEL
lm create remote-compute --local --provider openrouter \
  --model YOUR_EMBEDDING_MODEL --key-env OPENROUTER_API_KEY
lm create custom --local --provider openai-compatible \
  --base-url https://example.com/v1 --model YOUR_EMBEDDING_MODEL --key-env MY_EMBED_KEY
```

Models must actually be available at the provider. Configure keys privately in
the environment of the CLI or agent process; --key-env takes the variable's name,
not its value. No key is persisted in Local config, mcp output or inspection.
Storage remains Local when remote computation is configured, but memory/query
text is sent to that provider. Inspect shows this boundary. There is no hosted
Door or upload of the entire store. There is no chat-model call.

`lm config local:notes --json` reads saved configuration. Provider/model options
can update a new store, but cannot silently replace an established embedding
identity. Neither equal dimensions nor a successful HTTP response proves a new
model is compatible with old vectors.

## Diagnostics and continuation

Doctor checks the runtime and store without calling a provider by default.
Semantic mode remains unprobed until you use --probe, which sends only a generic
string and may incur provider charges. A probe does not send existing memories.
Machine output has readiness fields and nonzero failure exits; it never
substitutes lexical results after a failed semantic request.

```sh
printf '%s' 'Next: review the introduction.' | lm handoff local:notes
lm resume local:notes
lm mcp local:notes
```

The last command emits client configuration using an absolute lm executable and
LM_HOME. It runs `lm serve local:notes`. Keep the selected key environment
available to that client; config does not embed secrets. Handoffs are raw,
unembedded and expire after 24 hours by default; MCP accepts up to 72 hours.
Expired notes are removed on the next handoff operation.

Explicit local: selectors do not consult account inventory. A bare name is
accepted only when unambiguous; use local:/room:/world: for collisions. list
combines locally saved Local/Room rows with authenticated World inventory.
Existing export/import commands transfer remote grants, not Local memories.
There is no automatic Local-to-Room/World migration or whole-store transfer.

## Recovery and development

Stores use 0700 directories and 0600 files. A corrupt primary is not replaced
with empty memory; restore a verified backup explicitly. Another process may
hold the store lock: first reconcile state, then retry after it finishes.
After a crash, inspect brain.json.lock/owner.json, confirm no live process is
using it, and explicitly remove the stale lock. Do not delete a live lock.
Legacy direct-MCP stores require confirmed provenance and explicit adoption;
see the canonical MCP README. They are not imported into a new CLI Local.

For an unreleased source build, build lme-mcp and set LM_LOCAL_SERVER to its
absolute dist/server.js path. This is an explicit development override of the
installed runtime; normal installations should leave it unset.
