# Plan: Move config templates home by removing the lyxtest->configreg edge

```yaml
task: "Move config templates home by removing the lyxtest->configreg edge"
slug: config-test-cleanup
approved: true
started: "20260625-063846"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: lyxtest-leaf-seed
    file: 01-lyxtest-leaf-seed.md
    depends-on: []
    verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...
  - number: 2
    name: templates-home
    file: 02-templates-home.md
    depends-on: [1]
    verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...
  - number: 3
    name: lyxtest-tidy
    file: 03-lyxtest-tidy.md
    depends-on: [2]
    verify: go vet -tags integration ./... && go test -tags integration ./...
```

## Shared Decisions

### Decision: batch ordering is forced by the cycle

- **Decision:** `lyxtest` is made a leaf (batch 1) **before** `configreg` is reverted to import the
  feature packages (batch 2). Reverting `configreg → feature` while `lyxtest` still imports
  `configreg` would re-close the exact test-build cycle this task removes
  (`feature internal test → lyxtest → configreg → feature`). After batch 1, `lyxtest` no longer
  reaches `configreg`, so batch 2's `configreg → feature` edge is harmless.
- **Rationale:** Each batch must leave `go vet -tags integration ./...` green. Doing the revert
  first would make batch 2 fail the cycle gate. Batch 1 keeps `configtmpl` intact (so
  `configreg → configtmpl` still resolves templates), and only batch 2 deletes it.
- **Applies to:** all batches

### Decision: lyxtest must stay a stdlib + internal/paths leaf

- **Decision:** `internal/lyxtest` imports only the standard library and `internal/paths`. It must
  not import `internal/configreg` or any feature package (`board`/`worktree`/`weft`). Tests that
  need real config seed it themselves via `lyxtest.SeedConfig`, which takes a configreg-free
  `map[string]string` (module name → YAML content). The `configreg.Modules()` → map conversion
  happens at the test site, in a package that may legally import `configreg`.
- **Rationale:** Feature packages have internal (`package <pkg>`) tests that import `lyxtest`; any
  `lyxtest → configreg → feature` edge closes a test-build cycle (the `6d24098` trap). A
  configreg-typed parameter would re-introduce the import for the type alone.
- **Applies to:** all batches

### Decision: all `_lyx`/config paths go through internal/paths helpers

- **Decision:** Use `paths.LyxDirName`, `paths.ConfigDir(base)`, `paths.ConfigFile(base, module)`
  for every `_lyx`/config path, including in test and fixture code. Never build literals like
  `filepath.Join(base, "_lyx", "config")` or `"weft.yaml"`.
- **Rationale:** CONSTRAINTS.md "`_lyx` and config-file paths" rule — routing through helpers keeps
  layout migrations tracking automatically. Exceptions: `internal/paths/*_test.go` and `_lyx` used
  as pure link-target geometry / string assertions.
- **Applies to:** all batches

### Decision: verify is the Go integration gate

- **Decision:** Each batch's `verify:` runs `go vet -tags integration ./...` (the import-cycle
  gate; a re-closed cycle is build-fatal here) and `go test -tags integration ./...` (full suite,
  26 packages). Batches 1 and 2 also run `go build ./...` first because they change production code.
- **Rationale:** `lyxtest` is imported by nearly every integration test package, so the affected
  surface is the whole suite; there is no narrower scoping that still catches the cycle. The full
  suite is the justified scope (see each batch's `## Batch Tests`).
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `internal/board/template.go`
- `internal/board/template.yaml`
- `internal/configcli/configcli_integration_test.go`
- `internal/configreg/configreg.go`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/lyxtest/lyxtest.go`
- `internal/lyxtest/lyxtest_test.go`
- `internal/weft/template.go`
- `internal/weft/template.yaml`
- `internal/weft/weft_integration_test.go`
- `internal/worktree/add_test.go`
- `internal/worktree/cli_test.go`
- `internal/worktree/remove_test.go`
- `internal/worktree/template.go`
- `internal/worktree/template.yaml`
- `internal/worktree/weft_test.go`
