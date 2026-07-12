---
format: 2
approved: true
---

# Plan: broken chain fixture

Two batches whose chain-end: values are both invalid, tripping validation check 4
(chain-end-dangling) twice: batch 01's target does not exist, and batch 02's target is
itself verify: deferred.

## Batch Index

- 01 — first (1 card) — declares a dangling chain-end with no batch 03
- 02 — second (1 card) — declares a self-deferred chain-end target
