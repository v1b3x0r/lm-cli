# RC5 candidate: owner acceptance

## Try this workspace candidate

The private development launcher is `bin/lm-rc5`; it uses
`.local/rc5-workshop/` as LM_HOME and the installed candidate tarballs. It does
not use an existing account/Room configuration. From the CLI repository:

```sh
./bin/lm-rc5 setup local
./bin/lm-rc5 create workshop --local --provider openrouter
./bin/lm-rc5 doctor local:workshop --probe
printf '%s\n' "Workshop นี้ใช้ OpenRouter และเก็บความจำบนเครื่อง" | ./bin/lm-rc5 remember local:workshop
printf '%s\n' "Workshop ใช้อะไร" | ./bin/lm-rc5 recall local:workshop
```

Creation accepts hidden key input, then a live model choice. Enter keys in the
terminal prompt, not in chat.

## Authenticated acceptance — 2026-10-02

The owner completed hidden key entry, the live catalog picker and doctor probe
with OpenRouter `perplexity/pplx-embed-v1-4b` (2560 dimensions). The configured
software then stored a Thai workshop fact and recalled it from a new process
using a differently worded Thai question. Raw handoff/resume returned the exact
note across processes. Keys stayed in private lm.config; no key was read into
chat or committed.

This validates local file persistence with external semantic computation. An
on-device semantic provider (Ollama/LM Studio) was unavailable and remains
unverified; offline lexical and provider-failure fixtures pass.

Public npm packages, CLI release, site deployment and GitBook sync remain
pending. Release canonical MCP, compatibility MCP, CLI RC5, then installer/site
and docs. Merging source alone does not publish the runtime or release archives.
