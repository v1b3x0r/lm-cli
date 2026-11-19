# lm-cli

Project notes for agents working in this repo. Users: see README.md.

DONE-WHEN: on production, a freshly signed-up account that has no World
and has never used Pakkana runs `npx @nature-labs/lm`, creates Capsules A and B,
opens a read-only door on A, and an unmodified external MCP client (LM Studio or
Claude Desktop) answers a question from A's memory through that door while a
marker phrase stored only in B never appears. `lm doors list A` shows the door
with a real `lastUsedAt`; `lm doors revoke` withdraws it; the next call from the
client is refused; `lm doors list A` shows it revoked. A Pakkana visitor QR for
an existing Capsule still opens afterwards. Journeys J1–J7 in `docs/superpowers/specs/2026-11-19-lm-capsule-first-cli-contract.md` (section G) all pass.
