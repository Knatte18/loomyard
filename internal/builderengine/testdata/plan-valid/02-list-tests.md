# 02 — list-tests: cover --json in tests and help pins

## Intent

Tests prove the --json path end-to-end; help-tree pins reflect the new flag.
Stand-alone: assumes batch 01 is committed.

## Scope

- 02-list-tests.md

## Cards

### Card 1 — list --json tests

**What:** Add table-driven tests asserting one `output.Ok` envelope per row for
`list --json`, and that the table path is unchanged without the flag.
**Where:** 02-list-tests.md

### Card 2 — help-tree pin

**What:** Update the pinned help-tree set with the new --json flag help text.
**Where:** 02-list-tests.md

## verify:

go test ./internal/boardcli/... ./cmd/lyx/...
