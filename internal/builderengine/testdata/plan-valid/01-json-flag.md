# 01 — json-flag: add the --json flag and envelope emission

## Intent

`lyx board list --json` emits one `output.Ok` envelope per row instead of the table.
Stand-alone: after this batch the flag works end-to-end; tests land in batch 02.

## Scope

- 01-json-flag.md

## Cards

### Card 1 — flag + row struct

**What:** Add a `--json` bool flag to the list command; define `RowJSON` with the
existing table's columns as fields.
**Where:** 01-json-flag.md
**verify:** go build ./...

### Card 2 — emission path

**What:** When `--json` is set, marshal each row through `output.Ok` instead of the
table writer; keep the table path unchanged.
**Where:** 01-json-flag.md

## verify:

go test ./internal/boardcli/... ./internal/boardengine/...
