---
verify: deferred
chain-end: 4
---

# 03 — refactor-a: start splitting the row-envelope mapper

## Intent

First half of an atomic-refactor chain: extracts the row-envelope mapper into its own
file. This intermediate state does not compile cleanly on its own, so verify is
deferred to batch 04, the chain end.

## Scope

- 03-refactor-a.md

## Cards

### Card 1 — extract mapper skeleton

**What:** Move the row-to-envelope mapping function into its own file, leaving call
sites pointing at the old location temporarily.
**Where:** 03-refactor-a.md
