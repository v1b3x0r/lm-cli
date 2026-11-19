<h1 align="center">lm</h1>

<p align="center"><b>Give an AI a key to your memory, and take it back with one command.</b></p>

`lm` is the terminal companion for [Living Memory](https://living-memory.app). Sign in,
create **Capsules**, open a **read-only door** into one Capsule for any MCP client (LM Studio, a
local agent, a friend's tool), see who holds a door, and close it in one command.

```text
$ npx @nature-labs/lm@alpha doors open w_0123456789abcdef0123456789abcdef lmstudio
https://lme.viibe.to/t/…/mcp
This link is a key: whoever holds it can read this Capsule's memory until it expires or you run "lm doors revoke <capsule-id> <door-id>".
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
  `memory_state`) and nothing else. It cannot write, forget, or post.
- **Works with any MCP client.** A door is a plain Streamable HTTP link. Your model and system
  prompt stay exactly as they are.
- **One Capsule at a time.** A door reaches the Capsule you opened it on and no other.
- **See who holds a door.** `lm doors list <capsule-id>` shows each door, who holds it, and when it was last used.
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
| `lm capsules list` | List your Capsules (find a Capsule ID here). |
| `lm capsules create <name> <note>` | Create a Capsule with a first note. |
| `lm doors list <capsule-id>` | List the doors into a Capsule: door, holder, label, access, status, last used, expires. |
| `lm doors open <capsule-id> <label>` | Open a read-only door into a Capsule. Its link is printed **once**. |
| `lm doors revoke <capsule-id> <door-id>` | Close a door. |
| `lm doctor door <url>` | Check a door answers and is read-only. Never prints the link back. |
| `lm version` | Print the version. |

Add `--json` to any command for machine-readable output.

### Examples

- **Let LM Studio read one Capsule.** Open a door, then paste the link into LM Studio as an
  MCP server (Streamable HTTP):

  ```bash
  lm capsules list
  lm doors open w_0123456789abcdef0123456789abcdef lmstudio
  ```

  > [!WARNING]
  > The link is a key. Anyone holding it can read that Capsule until it expires or you revoke
  > it. Don't paste it in chats, screenshots, or issues.

- **Check a link someone gave you** (is it alive? can it write?):

  ```bash
  lm doctor door "https://lme.viibe.to/t/…/mcp"
  # Door: reachable
  # Read-only: yes (no tool can write, delete or post)
  # Tools: memory_search, memory_state
  ```

- **See who holds a door, then close it:**

  ```bash
  lm doors list w_0123456789abcdef0123456789abcdef
  lm doors revoke w_0123456789abcdef0123456789abcdef <door-id>
  # Revoked <door-id>. That link no longer opens this Capsule.
  ```

**What `lm doors list` does not show.** Pakkana's own doors and a Pakkana visitor QR are managed
in Pakkana, not here. A visitor QR is a separate read link that `lm` does not control, so
`lm doors revoke` will not close it (known exposure KSE-1).

### JSON output

With `--json`, every command prints exactly one line:

```json
{"ok":true,"data":{...}}
{"ok":false,"error":{"code":"not_signed_in","message":"not signed in; run lm login"}}
```

Exit codes: `0` success, `1` failure, `2` usage error.

```bash
# IDs of every door that is still live on a Capsule
lm doors list w_0123456789abcdef0123456789abcdef --json | jq -r '.data.doors[] | select(.status == "live") | .door_id'
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
