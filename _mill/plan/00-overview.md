# Plan: Speed up internal/warp integration tests

```yaml
task: "Speed up internal/warp integration tests"
slug: "warp-test-speedup"
approved: false
started: "20260626-065126"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: fixture-hook-strip
    file: 01-fixture-hook-strip.md
    depends-on: []
    verify: go test ./internal/lyxtest/ && go test -tags integration -run TestList ./internal/warp/
  - number: 2
    name: add-weftwiring
    file: 02-add-weftwiring.md
    depends-on: [1]
    verify: go test -tags integration -run TestAdd ./internal/warp/ && go test -tags integration -run TestWeft ./internal/warp/ && go test -tags integration -run TestWire ./internal/warp/
  - number: 3
    name: drift-status
    file: 03-drift-status.md
    depends-on: [1]
    verify: go test -tags integration -run TestPairInSync ./internal/warp/ && go test -tags integration -run TestStatus ./internal/warp/
  - number: 4
    name: prune-cleanup
    file: 04-prune-cleanup.md
    depends-on: [1]
    verify: go test -tags integration -run TestPrune ./internal/warp/ && go test -tags integration -run TestCleanup ./internal/warp/
  - number: 5
    name: remove-hook-misc
    file: 05-remove-hook-misc.md
    depends-on: [1]
    verify: go test -tags integration -run TestRemove ./internal/warp/ && go test -tags integration -run TestInstallPostCheckoutHook ./internal/warp/ && go test -tags integration -run TestList ./internal/warp/ && go test -tags integration -run TestRunDispatchesToWarp ./internal/warp/ && go test -tags integration -run TestWriteLaunchers ./internal/warp/ && go test -tags integration -run TestCreatePortal ./internal/warp/
```

## Shared Decisions

### Decision: pipe-free-verify

- **Decision:** Every `verify:` uses `go test -tags integration -run <PREFIX> ./internal/warp/`, chaining multiple prefixes with `&&`. Never use `|` regex alternation in `-run`.
- **Rationale:** Verified during planning that this environment's shell re-splits a single-quoted piped `-run 'TestA|TestB'` value on `|` (it tries to execute `TestB...` as a command). Plain `-run <prefix>` and `&&` chaining are safe. A bare prefix like `TestAdd` matches every `TestAdd*` function (incl. subtests); `TestWeft` + `TestWire` together cover weftwiring.
- **Applies to:** all batches.

### Decision: preserve-coverage

- **Decision:** Consolidation folds keep **every** assertion from the source tests (ported verbatim into the surviving sibling). Deletions remove only the exact tests named in the discussion delete table; the KEEP list is never touched (`TestWeftSpawnPushesWeftBranch`, `TestWeftRollbackOnPostHostCreateFailure`, `TestCleanup_LiveBranchNeverDeleted_NonEmptyBranchPrefix` (merged not deleted), `TestWeftPrechecksRejectExistingWeftWorktree`, all `Reconcile_*`/`Checkout_*`).
- **Rationale:** The speedup comes from fewer fixture builds, not less coverage. A dropped assertion is a plan defect.
- **Applies to:** all batches.

### Decision: parallel-safety

- **Decision:** Top-level test functions keep `t.Parallel()`. Within a test that runs multiple operations sequentially against one shared fixture (groups F, G, I) or shares one fixture across read-only subtests (group M), the shared steps/subtests must NOT call `t.Parallel()` relative to each other — they run in order on the same fixture. The group-A `TestAdd` table stays `t.Parallel()` at the top level; the per-case `extraAssert` runs inside the existing per-row body.
- **Rationale:** Git serializes on its own index/ref locks; concurrent operations on one shared repo race. Sequential execution on a shared fixture is the safe way to reuse a build.
- **Applies to:** batches 2, 4, 5.

## All Files Touched

- `internal/lyxtest/lyxtest.go`
- `internal/warp/add_test.go`
- `internal/warp/cleanup_test.go`
- `internal/warp/drift_test.go`
- `internal/warp/hook_test.go`
- `internal/warp/launchers_test.go`
- `internal/warp/list_test.go`
- `internal/warp/portals_test.go`
- `internal/warp/prune_test.go`
- `internal/warp/remove_test.go`
- `internal/warp/status_test.go`
- `internal/warp/warp_test.go`
- `internal/warp/weftwiring_test.go`
