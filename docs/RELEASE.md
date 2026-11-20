# Releasing `@nature-labs/lm`

Agents prepare; the founder publishes. npm 2FA is a passkey, so `npm publish` and `npm login` are the
founder's to run (an agent-side publish fails with EOTP).

## What is frozen per version

Only the tarball: `README.md`, `package.json` and `dist/`. Changing any of it means a new version.
Everything the README links to (docs, `/harbor`, `llms.txt`, GitHub settings) can change any day,
so anything likely to change belongs there, not in the README.

## Before publishing

1. Gates at the release commit, from a clean checkout of `main`:

   ```bash
   npm ci && npm run typecheck && npm test && npm run build
   npm pack --dry-run   # expect dist/, README.md, package.json, LICENSE only
   ```

2. Every URL in `README.md` and `package.json` answers. In particular the cover image
   `https://living-memory.app/living-memory-genesis.jpg` must return 200 (it is served by
   lm-launcher, and npm renders the README from the tarball, so a 404 ships a broken cover).
3. The mutable signs the README points to are current: `docs.living-memory.app/cli` teaches this
   TypeScript `lm` (Go 0.1.0-rc.5 marked legacy), `living-memory.app/llms.txt` mentions Capsule doors
   and `npx @nature-labs/lm@alpha`, and the Harbor Capsule's notes match what works today.
4. `npm view @nature-labs/lm` — know whether this is the first publish and which version/tags exist.

## Publish

Do not push a `v*` tag: `.github/workflows/publish.yml` publishes on tags but the repo has no
`NPM_TOKEN`, so the run would fail. Until Trusted Publishing is set up on npmjs.com for this package,
the founder publishes by hand from the release commit, in a real terminal (iTerm, Terminal.app):

```bash
npm publish --access public --tag alpha
```

Not through Claude Code's `!`: it has no TTY, and npm only offers the passkey/OTP step on a TTY
(`npm/lib/utils/auth.js`, `otplease`), so it fails with EOTP even with `--auth-type=web`.
`npm login` through `!` does work. After publishing, the registry can take a few minutes to serve the
new version.

`--tag alpha` matters: without it the alpha becomes `latest`.

## After publishing

1. `npx @nature-labs/lm@alpha version` from a clean directory prints the new version.
2. The npm page shows the cover, the links, and the repository/homepage/issues sidebar.
3. Update the Harbor Capsule note that says `lm` is not yet on npm (append a dated note; notes are
   append-only).
4. GitHub: a release for the version, topics without `go`, homepage, social preview.
5. Run the DONE-WHEN journey from `CLAUDE.md` with `npx`, not a local build.
