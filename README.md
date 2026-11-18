<h1 align="center">lm</h1>

<p align="center"><b>Give an AI a key to your memory, and take it back with one command.</b></p>

`lm` is the terminal companion for [Living Memory](https://living-memory.app). Sign in,
open a **read-only door** into your World for any MCP client (LM Studio, a local agent, a
friend's tool), see **who can reach your memory right now**, and close any door in one command.

```text
$ npx @nature-labs/lm@alpha doors open lmstudio --read
https://lme.viibe.to/t/…/mcp
This link is a key: whoever holds it can read everything in this World until it expires or you run "lm doors revoke lmstudio".

$ npx @nature-labs/lm@alpha access list
ACCESS  PRINCIPAL  CLIENT          SCOPE  LAST USED
rw-     human      owner           world  2m ago
r--     holder     lmstudio        world  1h ago
rw-     service    pakkana-web     world  5m ago
---     holder     old-agent       world  revoked
```

## Table of Contents

- [Features](#features)
- [Usage](#usage)
  - [Commands](#commands)
  - [Examples](#examples)
  - [JSON output](#json-output)
- [Where your sign-in lives](#where-your-sign-in-lives)
- [Set up locally](#set-up-locally)
- [Earlier versions](#earlier-versions)
- [License](#license)

## Features

- **Read-only by design.** A door can search and read your memory (`memory_search`,
  `memory_state`, `world_list`) and nothing else. It cannot write, forget, or post.
- **Works with any MCP client.** A door is a plain Streamable HTTP link. Your model and system
  prompt stay exactly as they are.
- **See who has access.** `lm access list` shows every person, agent, link and service that
  can reach your World, `ls -l` style, with when each was last used.
- **Take it back instantly.** `lm doors revoke` closes a door; the very next call through it is refused.
- **Safe to script.** Every command speaks one-line JSON with `--json` and stable exit codes.

## Usage

You need Node.js 20.12 or newer. No install step: run it with `npx`.

```bash
npx @nature-labs/lm@alpha login
```

Your browser opens Living Memory's sign-in page. Approve within 5 minutes and you are back in
the terminal, signed in. Then:

```bash
npx @nature-labs/lm@alpha whoami
```

> [!NOTE]
> `lm` is in alpha and published under the npm `alpha` tag, so the `@alpha` suffix is
> required for now. Prefer a short command? `npm install -g @nature-labs/lm@alpha`, then just `lm`.

### Commands

<!-- prettier-ignore -->
| COMMAND | DESCRIPTION |
| --- | --- |
| `lm login` | Sign in with your browser. On Windows the link is printed for you to open. |
| `lm logout` | Forget this computer's sign-in. |
| `lm whoami` | Show your default World. |
| `lm doors list` | List the doors into your World: label, access, created, expires. |
| `lm doors open <label> --read` | Open a read-only door. Its link is printed **once**. |
| `lm doors revoke <label>` | Close a door. |
| `lm access list` | Who can reach your World's memory right now. |
| `lm doctor door <url>` | Check a door answers and is read-only. Never prints the link back. |
| `lm version` | Print the version. |

Add `--json` to any command for machine-readable output.

### Examples

- **Let LM Studio read your memory.** Open a door, then paste the link into LM Studio as an
  MCP server (Streamable HTTP):

  ```bash
  lm doors open lmstudio --read
  ```

  > [!WARNING]
  > The link is a key. Anyone holding it can read your World until it expires or you revoke
  > it. Don't paste it in chats, screenshots, or issues.

- **Check a link someone gave you** (is it alive? can it write?):

  ```bash
  lm doctor door "https://lme.viibe.to/t/…/mcp"
  # Door: reachable
  # Read-only: yes (no tool can write, delete or post)
  # Tools: memory_search, memory_state, world_list
  ```

- **See who's been in, then close a door:**

  ```bash
  lm access list
  lm doors revoke lmstudio
  # Revoked "lmstudio". That link no longer opens this World.
  ```

How to read `lm access list`:

| Column | Meaning |
| --- | --- |
| `ACCESS` | What it can do **right now**: `rw-` read and write, `r--` read only, `---` nothing. |
| `PRINCIPAL` | `human` (you), `agent` (a verified agent), `holder` (whoever holds a link), `service` (a Living Memory app). |
| `CLIENT` | The label it was given. |
| `SCOPE` | What it can reach. Today always `world`. |
| `LAST USED` | Roughly when it was last used (hour-granular, best effort), or why it can't be: `revoked`, `expired`, `suspended`. |

### JSON output

With `--json`, every command prints exactly one line:

```json
{"ok":true,"data":{...}}
{"ok":false,"error":{"code":"not_signed_in","message":"not signed in; run lm login"}}
```

Exit codes: `0` success, `1` failure, `2` usage error.

```bash
# Labels of every door that can still be used
lm access list --json | jq -r '.data[] | select(.grant.effective) | .client.label'
```

## Where your sign-in lives

`lm` keeps one file, `account.json`, readable only by you (mode `0600`) in:

| System | Folder |
| --- | --- |
| macOS | `~/Library/Application Support/lm-cli` |
| Linux | `$XDG_CONFIG_HOME/lm-cli` (or `~/.config/lm-cli`) |
| Windows | `%AppData%\lm-cli` |

Set `LM_HOME` to use another folder. A sign-in from the earlier Go `lm` (0.1.0-rc.5) keeps working.
`lm` never prints your tokens, and error messages never contain links or server responses.

## Set up locally

```bash
git clone https://github.com/v1b3x0r/lm-cli.git
cd lm-cli
npm ci
npm test          # run the test suite
npm run build     # compile to dist/
node dist/cli.js version
```

## Earlier versions

The Go `lm` 0.1.0-rc.5 (local memory, Rooms) and its guides remain available from the
[v0.1.0-rc.5 release](https://github.com/v1b3x0r/lm-cli/releases/tag/v0.1.0-rc.5).

## License

[Apache License 2.0](LICENSE)
