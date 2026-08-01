# Plan: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests

```yaml
task: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests
slug: test-tier-substrate-audit
approved: true
started: 20260801-113818
parent: main
root: ""
verify: go vet ./...
```

## Batch Index

```yaml
batches:
  - number: 1
    name: tierpurity-guard-generalization
    file: 01-tierpurity-guard-generalization.md
    depends-on: []
    verify: go test ./cmd/lyx/... -run TestTierPurity -count=1 && go test ./cmd/lyx/... -run TestIsTierTagged -count=1 && go test ./cmd/lyx/... -run TestFindLongLiteralSleep -count=1
  - number: 2
    name: scoutengine-scout-tag
    file: 02-scoutengine-scout-tag.md
    depends-on: [1]
    verify: go build -tags scout ./cmd/lyx/... ./internal/scoutengine/... && go vet -tags scout ./cmd/lyx/... ./internal/scoutengine/... && go test ./cmd/lyx/... ./internal/scoutengine/... -count=1 && go test -tags integration ./cmd/lyx/... ./internal/scoutengine/... -count=1
  - number: 3
    name: substrate-rule-docs-and-sweep
    file: 03-substrate-rule-docs-and-sweep.md
    depends-on: [1, 2]
    verify: go vet -tags integration ./internal/gitrepo/... ./internal/websterengine/... ./internal/webstercli/...
```

## Shared Decisions

### Decision: `scout` tag scoped to real-external-binary tests only, no CI

Only the 4 existing `internal/scoutengine/*_integration_test.go` files (retagged `integration` → `scout`, filenames unchanged) plus `TestEnsureSupervised_StaleSocketCleanupAllowsRebind` and `TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr` (split out of `supervised_test.go` into a new `//go:build scout`-tagged file, dropping their runtime `exec.LookPath("gopls")`/`t.Skip` gates in favor of the build-tag-only gate their `toolchain_integration_test.go` sibling already uses) move behind the new tag. The untagged decision-logic tests in scoutengine (`daemonstate_test.go`, the rest of `supervised_test.go`, `refs_test.go`, `ensureserver_test.go`, `definition_test.go`, `lspclient_test.go`) are untouched — they test retry/lock/state-file logic with an already-bounded 300ms timeout and a generic held subprocess, never a real LSP binary. `go test -tags scout ./...` is documented as manual-only, no CI wiring (no CI exists anywhere in this repo).

**Applies to:** batch `scoutengine-scout-tag`, batch `substrate-rule-docs-and-sweep`.

### Decision: filenames are never changed by the retag

`ensureserver_integration_test.go`, `refs_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go` keep their existing `_integration_test.go` filename suffix even after their `//go:build` line changes to `scout` — only the build tag and the tag-describing prose in each file's own header comment change. Every filename cross-reference between these four files (e.g. `refs_integration_test.go` mentioned inside `ensureserver_integration_test.go`'s header) is left exactly as-is; only mentions of the tag *category itself* (`//go:build integration`-tagged, `-tags integration`, "any other integration test") change to `scout`.

**Applies to:** batch `scoutengine-scout-tag`.

### Decision: the full `-tags integration` repo-wide sweep found zero mis-tiering

This plan's authoring pass (Opus, five parallel research audits covering all 89 files carrying `//go:build integration` as of this plan's writing — reproducible via `grep -rl "^//go:build integration" --include="*_test.go" .` from the repo root) confirmed every file is tagged for a genuine substrate reason (real `git` subprocess spawn, real tmux session, real cross-compilation, real external-binary spawn, or real filesystem junction/symlink creation). Zero files were found mis-tiered (hermetic-but-tagged, or slow-for-no-substrate-reason). One doc-staleness nit was found (`internal/gitrepo/testmain_test.go`'s header comment) and is fixed in batch `substrate-rule-docs-and-sweep`. Because the sweep found no code-level mis-tiering, no batch in this plan re-tags or fixture-refactors any file outside `internal/scoutengine` — the full sweep's "fix everything found" mandate is satisfied by fixing the one doc nit; there was nothing else to fix.

**Applies to:** batch `substrate-rule-docs-and-sweep`.

### Decision: `-tags scout` test execution is a manual, gopls-gated verification step, never a machine-enforced `verify:` gate

`gopls` is not installed in this task's implementation environment (confirmed: `gopls` is absent from `$PATH` in the worktree container). The two subtests moved into the new scout-tagged file drop their runtime skip-gate (per the first Decision above), so running them under `-tags scout` without `gopls` present would hard-fail, not skip. No batch's `verify:` command in this plan executes `go test -tags scout`; batch `scoutengine-scout-tag`'s `verify:` instead confirms the tag compiles (`go build -tags scout`/`go vet -tags scout`) and confirms the four retagged files no longer execute under either `go test ./...` or `go test -tags integration ./...`. Running `go test -tags scout ./internal/scoutengine/... -count=1` to confirm the moved/retagged tests still pass is a manual step for an operator on a machine with `gopls` on `$PATH` — stated as such in that batch's `## Batch Tests` section.

**Applies to:** batch `scoutengine-scout-tag`.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/sandbox_coverage_test.go`
- `cmd/lyx/tierpurity_test.go`
- `cmd/lyx/tiersleep_test.go`
- `docs/benchmarks/running-tests.md`
- `internal/gitrepo/testmain_test.go`
- `internal/scoutengine/ensureserver_integration_test.go`
- `internal/scoutengine/refs_integration_test.go`
- `internal/scoutengine/supervised_integration_test.go`
- `internal/scoutengine/supervised_scout_test.go`
- `internal/scoutengine/supervised_test.go`
- `internal/scoutengine/toolchain_integration_test.go`
