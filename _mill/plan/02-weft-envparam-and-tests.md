# Batch: weft-envparam-and-tests

```yaml
task: "Optimise and slim the test suite"
batch: "weft-envparam-and-tests"
number: 2
cards: 4
verify: go test ./internal/weft/... && go test -tags integration ./internal/weft/...
depends-on: [1]
```

## Batch Scope

Refactor the weft package's in-process sync functions to take an explicit `SyncOptions` (env→option, overview "Decision: layered env→option"), map env at the `cli.go` edge, then migrate every weft git/subprocess test onto `lyxtest` fixtures, gate them behind `//go:build integration`, parallelise them, and table-drive the families that share setup. Production change and test migration are in one batch so the package's `verify` stays green at every step (changing `Push`/`Commit`/`Pull` signatures requires the tests to be updated in lockstep). `config_test.go` is pure-unit and stays untagged. Keep the env-var contract intact at the spawn boundary (`spawn_*.go` unchanged).

## Cards

### Card 5: env→option in weft/sync.go

- **Context:**
  - `internal/git`
  - `internal/lock`
- **Edits:**
  - `internal/weft/sync.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `type SyncOptions struct { SkipGit, SkipPush bool }`. Change `Commit(weftPath string, pathspec []string)` → `Commit(weftPath string, pathspec []string, opts SyncOptions)`, `Push(weftPath string)` → `Push(weftPath string, opts SyncOptions)`, `Pull(weftPath string)` → `Pull(weftPath string, opts SyncOptions)`. Remove the three `os.Getenv("WEFT_SKIP_GIT")`/`WEFT_SKIP_PUSH` reads (lines ~34, ~83, ~120) and branch on `opts.SkipGit`/`opts.SkipPush` instead, preserving identical semantics: `Commit` no-ops on `SkipGit`; `Push` no-ops on `SkipGit || SkipPush`; `Pull` no-ops on `SkipGit`. Remove the now-unused `os` import only if nothing else needs it (the lock-dir mkdir uses `os.MkdirAll`, so `os` likely stays). Update the package/function doc comments to describe the option rather than the env vars.
- **Commit:** `refactor(weft): thread SyncOptions through Commit/Push/Pull`

### Card 6: map env→SyncOptions at the cli.go edge

- **Context:**
  - `internal/weft/sync.go`
  - `internal/weft/spawn_windows.go`
  - `internal/weft/spawn_other.go`
- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** At the **cwd-resolved** call sites (the `commit`, `push`, `pull`, `sync` cases, ~lines 106/113/117/123/129), read `os.Getenv("WEFT_SKIP_GIT")=="1"` and `os.Getenv("WEFT_SKIP_PUSH")=="1"` into a `SyncOptions` and pass it to `Commit`/`Push`/`Pull`. Add a small local helper (e.g. `func envSyncOptions() SyncOptions`) to avoid repeating the reads. At the **detached `--weft-path` branch** (line ~66, `Push(*weftPathFlag)`), pass a zero-value `SyncOptions{}` — do NOT add an env read there: by the time the detached child runs, `spawnPush` has already decided (via its env check) that pushing should proceed, so the child must push unconditionally. Leave `spawnPush` and `spawn_*.go` unchanged. Add the `os` import if not present.
- **Commit:** `refactor(weft): map WEFT_SKIP_* env to SyncOptions at cli edge`

### Card 7: migrate + tag + parallelise sync_test.go and status_test.go

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/weft/sync.go`
  - `internal/weft/status.go`
- **Edits:**
  - `internal/weft/sync_test.go`
  - `internal/weft/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `//go:build integration` (blank line, then `package weft`) to both files. Replace the local `newTestWeftRepo`/`addWeftRemote` fixtures with `lyxtest.CopyWeft(t)` (and `lyxtest.MustRun` for any verification git calls); delete the local helper funcs from `sync_test.go`. Replace every `t.Setenv("WEFT_SKIP_GIT"/"WEFT_SKIP_PUSH", "1")` with the explicit `SyncOptions{...}` argument to `Commit`/`Push`/`Pull` (covers `TestCommit_SkipGit`, `TestPush_SkipGit`, `TestPush_SkipPush`; the non-skip tests pass `SyncOptions{}`). Add `t.Parallel()` to each test (now legal — no `t.Setenv`). Table-drive the families that share setup: `Status_Junction*` (missing / plain-dir / valid-symlink / valid-junction — keep the symlink/junction skips via `SKIP_SYMLINK_TEST`/`SKIP_MKLINK_TEST`), `Commit_*`, and `Push_*`, building one `CopyWeft` base per case. Preserve every distinct behavioural assertion (the equivalence guardrail must show the post subtest set is a superset).
- **Commit:** `test(weft): migrate sync/status tests to lyxtest, tag+parallelise`

### Card 8: migrate + tag cli_test.go and weft_integration_test.go

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/weft/cli.go`
  - `internal/weft/sync.go`
- **Edits:**
  - `internal/weft/cli_test.go`
  - `internal/weft/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `//go:build integration` to both files (`weft_integration_test.go` currently has no tag — add it). In `cli_test.go`: re-express `TestRunCLI_StatusWithMinimalFixture`'s inline 2-repo fixture using a `lyxtest` paired/weft fixture (a host + `<base>-weft` sibling pair); it uses `t.Chdir`, so keep it **serial** (no `t.Parallel`). The env-free router tests (`TestRunCLI_UnknownSubcommand`, `TestRunCLI_WeftPathPushOnly`) may stay as-is but tag the file. In `weft_integration_test.go`: replace `newTestWeftRepo`+`addWeftRemote` with `lyxtest.CopyWeft(t)`, add `t.Parallel()` to the four tests (`TestPushIntegration_*`, `TestPullIntegration_FastForward`, `TestSyncIntegration_EventuallyPushed`), and pass `SyncOptions{}` to any `Push`/`Pull`/`Commit` calls. Add a test asserting the cli edge maps env→option (set `WEFT_SKIP_PUSH` via `t.Setenv` in a dedicated serial test that drives the cwd `push` path and asserts no push occurred) so the env-contract is still covered.
- **Commit:** `test(weft): migrate cli/integration tests to lyxtest, tag`

## Batch Tests

`verify` runs both tiers for the package: untagged `go test ./internal/weft/...` (must compile + pass offline — only `config_test.go` runs) and `go test -tags integration ./internal/weft/...` (the full migrated git suite). Equivalence guardrail: before card 7, capture `go test -tags integration -list '.*' ./internal/weft/...` and the `=== RUN` subtest paths to `.scratch/baseline-weft.txt`; after card 8, diff and confirm the post set is a superset (record any intentional table-driven folds). Also run `go test -race -tags integration -count=2 ./internal/weft/...` once to catch shared-state leaks from the new `t.Parallel()`. Scratch files are not committed.
