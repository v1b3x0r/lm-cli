# Your first Room

After installation, open a new Terminal tab. Run `lm version` to check it is ready.

## 1. Create once

```sh
lm create my-room --room
```

`my-room` is the local name for this Room. If you already created it, skip this
step. Use `lm list` to find Room names saved on this machine.

The source build shows Room ID, Open/Guide, MCP, and creation-time expiry.
Open and Guide are the same browser entrance. These links carry the door's
access: keep them private unless you intend to share that access. The currently
published 0.1.0-rc.1 has the earlier, smaller output; see the README for building
this checkout.

```sh
lm inspect my-room
```

Inspect reads live identity, state and tools. The local name and canonical ID
are separate; neither is the `ons_…`/`ro_…` credential in the browser link.
Older grants initially show an unknown ID; inspect fills it when supported.

## 2. Leave work for the next agent

```sh
printf '%s' 'The draft is ready. Next: review the introduction.' | lm handoff my-room
```

## 3. Return in another process

Close this Terminal tab and open a new one (using the same `lm` binary):

```sh
lm resume my-room
```

The returned checkpoint should say the draft is ready and the next step is to
review the introduction. Keep using this Room; do not create another to resume.

## 4. Remember a fact when needed

```sh
printf '%s' 'Our project color is amber.' | lm remember my-room
```

The Room confirms it stored the memory.

## 5. Recall the fact

Close this Terminal tab and open a new one:

```sh
printf '%s' 'What is our project color?' | lm recall my-room
```

Look for the saved fact about amber in the returned memories. Use the same Room
name next time; do not create another Room to resume.

Free Rooms have usage and inactivity limits. This CLI currently works with Room
credentials saved on your machine. `lm list` shows local snapshots without a
network lookup. Creation-time expiry is not current server expiry. If inspect
fails, local identity remains visible, the result is partial, and the command
exits nonzero; a network error does not mean the Room disappeared.

## If a command is not found

After automatic installation, open a new Terminal tab. If you downloaded the
binary manually, use `./lm` while inside its extracted folder, or use the
[installer](install.md) to set up the `lm` command.
