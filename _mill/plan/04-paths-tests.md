# Batch: paths-tests

```yaml
task: "Optimise and slim the test suite"
batch: "paths-tests"
number: 4
cards: 1
verify: go test ./internal/paths/... && go test -tags integration ./internal/paths/...
depends-on: [1]
```

## Batch Scope

Migrate the paths package's git-spawning tests onto `lyxtest`, gate them behind `//go:build integration`, and parallelise them. paths has **no env seam** (it never reads `WEFT_SKIP_*`) so there is no production change here — it depends only on the `lyxtest` foundation (batch 1), not on the env→option batches. The pure-computation `weft_test.go` (literal `Layout` geometry) and the static guard tests (`codeguide_guard_test.go`, `enforcement_test.go`) do no I/O and stay untagged. The duplicated `helpers_test.go` is drained into `lyxtest` and deleted. One card covers it.

## Cards

### Card 13: migrate + tag + parallelise paths_test.go and worktreelist_test.go; drain helpers

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/paths/paths.go`
  - `internal/paths/worktreelist.go`
  - `internal/paths/weft_test.go`
  - `internal/paths/codeguide_guard_test.go`
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/paths/paths_test.go`
  - `internal/paths/worktreelist_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/paths/helpers_test.go`
- **Requirements:** Add `//go:build integration` (blank line, then `package paths_test`) to `paths_test.go` and `worktreelist_test.go` — every case calls `newTestRepo` and/or `paths.Resolve` (which spawns `git rev-parse --show-toplevel`, `paths.go:61`). Replace the local `newTestRepo`/`mustRun` (in `helpers_test.go`) with `lyxtest.CopyHostHub(t)` / `lyxtest.MustRun`; for the `Resolve` tests use the copy's `Hub` path. `worktreelist_test.go`'s `TestList` adds extra worktrees via `lyxtest.MustRun`, and its `BareRepoRejection` case needs a bare repo — build it with `lyxtest.MustRun(t, t.TempDir(), "git","init","--bare")` (or expose a tiny `lyxtest` bare helper if cleaner). Add `t.Parallel()` to all migrated tests/subtests (paths tests use no `t.Setenv`/`t.Chdir` — `paths.Resolve(cwd)` takes cwd explicitly). Delete `helpers_test.go` after migration; confirm `weft_test.go`, `codeguide_guard_test.go`, `enforcement_test.go` stay **untagged** and reference no deleted helper (they do no git/IO). Preserve every distinct assertion, including `TestMirroredMethods`'s subtree and `TestResolve_NotAGitRepo`.
- **Commit:** `test(paths): migrate resolve/list tests to lyxtest, tag+parallelise`

## Batch Tests

`verify` runs both tiers: untagged `go test ./internal/paths/...` (must pass offline — `weft_test.go` + guard tests run, zero git spawns) and `go test -tags integration ./internal/paths/...` (Resolve/list suite). Equivalence guardrail: capture `-list` + `=== RUN` baselines to `.scratch/baseline-paths.txt` before editing; diff after and confirm superset. Run `go test -race -tags integration -count=2 ./internal/paths/...` once for parallel-safety. The offline check is load-bearing: after the drain, no spawning `func Test...` may remain in an untagged paths file. Scratch files are not committed.
