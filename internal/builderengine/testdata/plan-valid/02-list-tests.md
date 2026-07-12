# 02 — list-tests: cover --json in tests and help pins

## Intent

Tests prove the --json path end-to-end; help-tree pins reflect the new flag.
Stand-alone: assumes batch 01 is committed.

## Scope

- 02-list-tests.md
- 03-refactor-a.md
- 03-refactor-a-renamed.md

## Rename mechanic

Run `git mv 03-refactor-a.md 03-refactor-a-renamed.md` first, before any other change to
the moved file; then make only the surgical edits Card 02.2's What: describes — never
write the relocated file from scratch and delete the original.

## Cards

### Card 02.1 — list --json tests

**What:** Add table-driven tests asserting one `output.Ok` envelope per row for
`list --json`, and that the table path is unchanged without the flag.
**Context:** none
**Edits:**
- `02-list-tests.md`
**Creates:** none
**Deletes:** none
**Moves:** none

### Card 02.2 — help-tree pin + fixture relocation

**What:** Update the pinned help-tree set with the new --json flag help text, and
relocate the refactor-a fixture ahead of its rename in batch 03 (git mv first, per the
Rename mechanic above).
**Context:** none
**Edits:**
- `02-list-tests.md`
**Creates:** none
**Deletes:** none
**Moves:**
- `03-refactor-a.md` -> `03-refactor-a-renamed.md`

## verify:

go test ./internal/boardcli/... ./cmd/lyx/...
