# lm

Living Memory from your terminal: sign in, open read-only doors into your World, and see who can reach it.

```sh
npx @nature-labs/lm login
```

Requires Node >= 20.12. Alpha: `0.2.x` is published under the npm `alpha` tag.

DONE-WHEN: on production, `npx @nature-labs/lm` signs in, opens a read-only door
into a test World, LM Studio running its own unchanged model and system prompt
answers a question from `memory_search` through that door, `lm doors revoke`
withdraws it, and the next LM Studio call is refused. Throughout, `lm access list`
shows that door with a real `lastUsedAt` before the revoke and `status: revoked`
after it. The proof runs on the founder's own World.

## Commands

| Command | What it does |
| --- | --- |
| `lm login` / `lm logout` | Sign in to Living Memory in your browser, or forget this computer's sign-in |
| `lm whoami` | Show your default World |
| `lm doors list` | List the doors into your World |
| `lm doors open <label> --read` | Open a read-only door. Its link is printed once; treat it as a key |
| `lm doors revoke <label>` | Close a door. The next call through it is refused |
| `lm access list` | Who can reach your World's memory right now, `ls -l` style |
| `lm doctor door <url>` | Check a door is reachable and read-only, without printing it back |
| `lm version` | Print the version |

Every command accepts `--json` and then prints one line: `{"ok":true,"data":…}` or
`{"ok":false,"error":{"code":…,"message":…}}`. Exit codes: `0` ok, `1` failure, `2` usage.

## A door for a local model

```sh
lm doors open lmstudio --read
```

Paste the printed link into your MCP client (for example LM Studio) as a Streamable HTTP
server. It can search and read your memory and nothing else. `lm access list` shows when
it was last used; `lm doors revoke lmstudio` takes it back.

## Where the sign-in lives

`account.json` (mode 0600) in `~/Library/Application Support/lm-cli` on macOS,
`$XDG_CONFIG_HOME/lm-cli` (or `~/.config/lm-cli`) on Linux, `%AppData%\lm-cli` on Windows.
`LM_HOME` overrides it. A sign-in from the Go `lm` 0.1.0-rc.5 keeps working.

## Earlier versions

The Go `lm` 0.1.0-rc.5 (local memory, Rooms) stays available at
https://github.com/v1b3x0r/lm-cli/releases.
