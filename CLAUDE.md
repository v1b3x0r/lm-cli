# lm-cli

Project notes for agents working in this repo. Users: see README.md.

DONE-WHEN: on production, `npx @nature-labs/lm` signs in, opens a read-only door
into a test World, LM Studio running its own unchanged model and system prompt
answers a question from `memory_search` through that door, `lm doors revoke`
withdraws it, and the next LM Studio call is refused. Throughout, `lm access list`
shows that door with a real `lastUsedAt` before the revoke and `status: revoked`
after it. The proof runs on the founder's own World.
