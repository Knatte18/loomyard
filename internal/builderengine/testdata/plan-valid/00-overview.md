---
format: 2
approved: true
---

# Plan: add --json to lyx board list

Add a --json output mode to lyx board list, emitting one JSON object per row via the
internal/output envelope, with tests and help text updated. A trailing deferred-verify
chain (batches 03-04) and one oversized batch (05) extend the worked example with
plan-format v2 validation coverage.

## Batch Index

- 01 — json-flag (2 cards) — add the --json flag and envelope emission to boardcli list
- 02 — list-tests (2 cards) — cover --json in boardcli list tests and update help-tree pins
- 03 — refactor-a (1 card) — start splitting the row-envelope mapper out of boardcli list
- 04 — refactor-b (1 card) — finish the mapper extraction and run the chain's real verify
- 05 — oversized (1 card) — rewrite boardengine's row pipeline in one atomic pass
