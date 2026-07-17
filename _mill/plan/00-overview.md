# Plan: loom: Preflight phase (precondition validation)

```yaml
task: 'loom: Preflight phase (precondition validation)'
slug: loom-preflight
approved: false
started: 20260717-160904
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: preflight-prereqs
    file: 01-preflight-prereqs.md
    depends-on: []
    verify: go test ./internal/state/ ./internal/hubgeometry/ && go test -tags integration -run TestHostClean ./internal/warpengine/
  - number: 2
    name: loomengine-preflight
    file: 02-loomengine-preflight.md
    depends-on: [1]
    verify: go test -tags integration ./internal/loomengine/ && go test -run 'TestTierPurity|TestHermeticGitEnv' ./cmd/lyx/
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [2]
    verify: null
```

## Shared Decisions

### Decision: engine-only, no cobra module

- **Decision:** This task adds no `lyx loom` cobra subtree. All new code lands in
  `internal/loomengine` (a new package) plus small helpers in existing packages
  (`internal/state`, `internal/hubgeometry`, `internal/warpengine`). No registration in
  `cmd/lyx/main.go`, no `loomcli`, no `Command()`/`RunCLI`.
- **Rationale:** `discussion.md` → package-placement-engine-only. Preflight is a pure
  precondition validator, testable in isolation; the `lyx loom` CLI lands later with the
  phase-machine skeleton. This deliberately avoids the CLI/Cobra + Sandbox-coverage
  scaffolding (no `drift_test`/`helptree_test`/`registration_test`/`longlist_test`/sandbox
  scenario changes are needed or made).
- **Applies to:** all batches

### Decision: Preflight owns Getwd+Resolve (reconciling the discussion signature)

- **Decision:** The public entry is `func Preflight() (Report, error)` (no argument). It calls
  `hubgeometry.Getwd()` + `hubgeometry.Resolve(cwd)` itself and owns check 1 end-to-end,
  delegating checks 1b–4 to an unexported `checkResolved(l *hubgeometry.Layout) (Report, error)`
  that takes an already-resolved `Layout` for isolation testing.
- **Rationale:** `discussion.md`'s the-four-checks check 1 explicitly does "Getwd() then
  Resolve(cwd)" and lists "Not a git repo → geometry" as a Preflight test scenario — both
  require Preflight to perform the resolution. The discussion's illustrative
  `Preflight(l *hubgeometry.Layout)` signature cannot satisfy that (a caller with no repo has no
  `Layout` to pass). Where the discussion's behavioural prose and its illustrative signature
  conflict, the prose wins; the `checkResolved(l)` helper preserves the "testable in complete
  isolation" intent by accepting an injected `Layout`. `hubgeometry.ErrNotAGitRepo` maps to a
  determined `geometry` Report failure (short-circuit); a non-sentinel git-spawn/exec error maps
  to the `error` return per result-error-contract.
- **Applies to:** loomengine-preflight

### Decision: error-vs-Report contract

- **Decision:** `error != nil` = "couldn't determine" (git spawn failure, non-`IsNotExist` I/O,
  `lock.AcquireReadLock` failure, `state.ErrRead`). `error == nil` with `Report{OK:false,
  Failures}` = determined unmet preconditions. `Report{OK:true}` = fit to run. `OK == (len(Failures)
  == 0)`.
- **Rationale:** `discussion.md` → result-error-contract, strict-read-mechanism. Cleanly
  separates infra failure (escalate) from a determined "not ready" verdict (list reasons).
- **Applies to:** loomengine-preflight

### Decision: Go test tiering (Tier 1 untagged vs integration-tagged)

- **Decision:** Pure, spawn-free tests are untagged (Tier 1): `state.ReadJSONStrict`,
  `hubgeometry.LoomStatusFile`/`LoomStatusLock`, `loomengine` status-type + coherence validator.
  Any test that spawns git (fixtures, `HostClean`, the `Preflight` end-to-end integration tests)
  is `//go:build integration`-tagged and lives in a package with a `TestMain` calling
  `lyxtest.HermeticGitEnv()`.
- **Rationale:** `discussion.md` → Constraints (Test Tier Purity Invariant, Hermetic Git Test
  Environment Invariant). Untagged tests must not spawn; git-spawning packages need the hermetic
  `TestMain`.
- **Applies to:** all batches

### Decision: CheckID vocabulary

- **Decision:** `type CheckID string` with the closed set: `geometry`, `worktree-root`,
  `worktree-clean`, `weft-pairing`, `weft-sync`, `junction`, `seed-missing`, `seed-unreadable`,
  `seed-incoherent`, `half-finished`.
- **Rationale:** `discussion.md` → report-shape. Machine-consumable classification for the future
  phase machine + human `Reason` string per failure.
- **Applies to:** loomengine-preflight

## All Files Touched

- `docs/modules/loom.md`
- `docs/reference/status-schema.md`
- `docs/roadmap.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/loomstatus_test.go`
- `internal/loomengine/coherence.go`
- `internal/loomengine/coherence_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/loomengine/report.go`
- `internal/loomengine/status.go`
- `internal/loomengine/testmain_test.go`
- `internal/state/state.go`
- `internal/state/strict_test.go`
- `internal/warpengine/hostclean.go`
- `internal/warpengine/hostclean_test.go`
