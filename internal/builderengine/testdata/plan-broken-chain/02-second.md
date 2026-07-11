---
verify: deferred
chain-end: 2
---

# 02 — second: declares a self-deferred chain-end

## Intent

Declares chain-end: 2 — itself — which also carries verify: deferred, so the
"target is not itself deferred" rule is tripped.

## Scope

- 02-second.md

## Cards

### Card 1 — placeholder

**What:** Nothing — this fixture exists only to trip chain-end-dangling.
**Where:** 02-second.md
