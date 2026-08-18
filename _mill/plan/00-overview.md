# Plan: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
slug: "orchestrator-preflight"
approved: true
started: "20260818-060343"
parent: "standalone-producers"
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: buildinfo-and-mode-mapping
    file: 01-buildinfo-and-mode-mapping.md
    depends-on: []
    verify: go test ./internal/buildinfo/... ./internal/stencilstore/... ./internal/lyxcwd/...
  - number: 2
    name: standalonestate-leaf
    file: 02-standalonestate-leaf.md
    depends-on: []
    verify: go test ./internal/standalonestate/... ./internal/lyxcwd/... && go test -tags integration ./internal/standalonestate/...
  - number: 3
    name: preflight-lift
    file: 03-preflight-lift.md
    depends-on: []
    verify: go test ./internal/preflight/... ./internal/loomengine/... ./internal/lyxcwd/... && go test -tags integration ./internal/preflight/... ./internal/loomengine/...
  - number: 4
    name: cli-gate-and-ldflags
    file: 04-cli-gate-and-ldflags.md
    depends-on: [1, 3]
    verify: go test ./cmd/lyx/... ./tools/deploy/... ./internal/lyxcwd/... && go test -tags integration ./cmd/lyx/...
  - number: 5
    name: docs-and-invariants
    file: 05-docs-and-invariants.md
    depends-on: [1, 2, 3, 4]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: fabric-vocabulary-is-the-build-breaker

- **Decision:** No production `.go` file created or edited by this task, and no `.md` file placed under `internal/`, may contain the substrings `weft` or `warp` in any case, in identifiers, string literals, or comments.
  None of `internal/preflight`, `internal/buildinfo`, `internal/standalonestate`, `internal/loomengine`, `cmd/lyx`, or `internal/stencilstore` is in the Fabric Vocabulary Invariant's owner set.
  Describe the check as "Fabric is wired here" or "the worktree pair", never "the weft sibling".
  Call `fabricengine.Ready(l)`; never name `WeftWorktree`.
  `*_test.go` files are excluded from the rule by the enforcement test, but keep them clean anyway.
- **Rationale:** `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` is a case-insensitive substring walk, so the token inside a camelCase identifier trips it. `internal/loomengine/preflight.go`'s existing "worktree pair cleanliness" wording is the in-repo precedent.
- **Applies to:** all batches

### Decision: report-not-error contract is preserved verbatim

- **Decision:** Every lifted check keeps the report-not-error contract: `(Report{OK:true}, nil)` when everything passes, `(Report{OK:false, Failures}, nil)` for a determined negative verdict, and `(Report{}, err)` only when the answer could not be determined.
  A `fabricengine.PrimeName` failure stays a `CheckGeometry` failure, never an escalated error.
- **Rationale:** `internal/loomengine/preflight_integration_test.go`'s 13 test functions are the compatibility contract; the lift must be behaviour-preserving.
- **Applies to:** all batches

### Decision: type aliases, never duplicate types

- **Decision:** `internal/preflight` owns `CheckID`, `Failure`, `Report` and the five tier-1/tier-2 check-ID constants. `internal/loomengine/report.go` re-exposes them as Go **type aliases** (`type Report = preflight.Report`) and **const aliases** (`const CheckGeometry = preflight.CheckGeometry`), never as new named types.
- **Rationale:** aliases make `loomengine.Report` and `preflight.Report` the identical type, so `internal/loomengine/preflight_integration_test.go` compiles with zero edits — which is the actual proof the lift changed no behaviour.
  Any required edit to that file signals the aliases were implemented as duplicate types.
- **Applies to:** batch 3

### Decision: test tier and hermetic-git discipline

- **Decision:** Any new test file that spawns git or builds a `hubforge`/`gitkit` fixture carries `//go:build integration` on its first non-empty line.
  Any new test **package** that spawns git carries a `TestMain` calling `gitkit.HermeticGitEnv()`.
  Untagged test files spawn nothing.
- **Rationale:** Test Tier Purity Invariant (`cmd/lyx/tierpurity_test.go`) and Hermetic Git Test Environment Invariant (`cmd/lyx/hermeticenv_test.go`) both fail the build otherwise. `cmd/lyx` and `internal/loomengine` already have their `TestMain`; `internal/preflight` is a new git-spawning package and needs one.
- **Applies to:** batches 2, 3, 4

### Decision: leaf enforcement tests are copies of the tokenvocab idiom

- **Decision:** `internal/buildinfo/leaf_enforcement_test.go` and `internal/standalonestate/leaf_enforcement_test.go` copy the structure of `internal/tokenvocab/leaf_enforcement_test.go` — `go/parser` with `parser.ImportsOnly`, stdlib detected as "no `.` in the first path segment", an `allowedImports` allowlist map — with the allowlist set to an **empty** map for both packages.
- **Rationale:** an unenforced leaf claim rots on the first convenience import, and T7/T8 import both packages from CLI packages specifically to avoid cycles.
- **Applies to:** batches 1, 2

### Decision: no export_test.go anywhere in this task

- **Decision:** This task adds no `export_test.go` file. `internal/preflight`'s seams are exported, so its external test package reaches them directly; `internal/standalonestate`'s tests are in-package and reach `derive` directly.
  `internal/loomengine/export_test.go` already exists and is left unedited.
- **Rationale:** a shim over an already-exported symbol is dead code.
- **Applies to:** batches 2, 3

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/stencilseed.go`
- `cmd/lyx/stencilseed_integration_test.go`
- `docs/overview.md`
- `docs/shared-libs/README.md`
- `internal/buildinfo/buildinfo.go`
- `internal/buildinfo/buildinfo_test.go`
- `internal/buildinfo/doc.go`
- `internal/buildinfo/leaf_enforcement_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/report.go`
- `internal/preflight/doc.go`
- `internal/preflight/predicates.go`
- `internal/preflight/preflight.go`
- `internal/preflight/preflight_integration_test.go`
- `internal/preflight/report.go`
- `internal/preflight/report_test.go`
- `internal/preflight/testmain_test.go`
- `internal/standalonestate/doc.go`
- `internal/standalonestate/leaf_enforcement_test.go`
- `internal/standalonestate/standalonestate.go`
- `internal/standalonestate/standalonestate_test.go`
- `internal/standalonestate/symlink_integration_test.go`
- `internal/stencilstore/modefor_test.go`
- `internal/stencilstore/stencilstore.go`
- `tools/deploy/main.go`
- `tools/deploy/main_test.go`
