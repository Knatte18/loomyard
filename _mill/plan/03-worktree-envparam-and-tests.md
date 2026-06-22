# Batch: worktree-envparam-and-tests

```yaml
task: "Optimise and slim the test suite"
batch: "worktree-envparam-and-tests"
number: 3
cards: 4
verify: go test ./internal/worktree/... && go test -tags integration ./internal/worktree/...
depends-on: [1]
```

## Batch Scope

Thread an explicit `AddOptions` through `Add` → `pushWeftBranch` (env→option), map env at the `worktree/cli.go` edge, then migrate every worktree git/junction test onto `lyxtest`, gate behind `//go:build integration`, parallelise, and table-drive the `TestAdd` precondition + `TestRemove` dirty-gate families. Production change and test migration ship together so `verify` stays green (the `Add` signature change must land with its test call-site updates). The duplicated helper files (`testhelpers_test.go`, `helpers_test.go`) are drained into `lyxtest` and deleted. `config_test.go`, `links_test.go`, `prune_test.go` are pure-unit and stay untagged. Junction/`mklink` production code is untouched (out of scope).

## Cards

### Card 9: env→option in add.go, weft.go, cli.go

- **Context:**
  - `internal/git`
  - `internal/paths/paths.go`
  - `internal/weft/spawn_windows.go`
- **Edits:**
  - `internal/worktree/add.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/cli.go`
  - `internal/worktree/add_test.go`
  - `internal/worktree/remove_test.go`
  - `internal/worktree/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `type AddOptions struct { SkipGit, SkipPush bool }`. Change `func (w *Worktree) Add(l *paths.Layout, slug string)` → `Add(l *paths.Layout, slug string, opts AddOptions)` (`add.go:59`); thread `opts` into the `pushWeftBranch(l, slug, branch)` call at `add.go:167`. Change `pushWeftBranch(l *paths.Layout, slug, branch string)` → `pushWeftBranch(l *paths.Layout, slug, branch string, opts AddOptions)` (`weft.go:207`); remove its `os.Getenv` read (line ~208) and branch on `opts.SkipGit || opts.SkipPush` instead, preserving identical semantics (no-op return nil when either is set). Do NOT alter the unconditional host `git push -u origin branch` at `add.go:156` — it is a real local push to the bare remote and was never env-gated. At the sole production caller `w.Add(l, slug)` in `cli.go:90`, read `os.Getenv("WEFT_SKIP_GIT")`/`WEFT_SKIP_PUSH` into an `AddOptions` and pass it; add the `os` import. Update doc comments to describe the option rather than the env vars. Update the existing test call-sites in add_test.go/remove_test.go/weft_test.go to pass `AddOptions{SkipPush:true}` directly (remove `t.Setenv` for those calls); the full lyxtest migration of these files happens in card 10.
- **Commit:** `refactor(worktree): thread AddOptions through Add and pushWeftBranch`

### Card 10: migrate + tag + parallelise add_test.go and remove_test.go

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/worktree/add.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/weft.go`
- **Edits:**
  - `internal/worktree/add_test.go`
  - `internal/worktree/remove_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `//go:build integration` (blank line, then `package worktree`) to both files. Replace the `newTestRepo`+`addRemote`+`newWeftRepo` setup with `lyxtest.CopyPaired(t)` (use its `Layout`/paths). Replace every `t.Setenv("WEFT_SKIP_PUSH","1")` with the explicit `AddOptions{SkipPush: true}` passed to `w.Add(l, slug, opts)`. Add `t.Parallel()` to each test that no longer uses `t.Setenv`/`t.Chdir`; `TestRemoveSubpathJunction` (in `remove_test.go`) uses `t.Chdir` and stays serial. Table-drive the `TestAdd` precondition subtests (DirtySource / BranchExists / TargetDirExists / NoRemote / NoWeftRepo) and the `TestRemove` dirty-gate variants (host/weft × with/without force) as cases that build one `CopyPaired` base then apply a per-case delta (dirty a file, pre-create a dir, pre-create a branch). Keep `TestAddRollback` and every distinct assertion. Use `lyxtest.MustRun` for any per-case git mutation.
- **Commit:** `test(worktree): migrate add/remove tests to lyxtest, tag+parallelise`

### Card 11: migrate + tag + parallelise weft/portals/launchers/list/junction tests

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/portals.go`
  - `internal/worktree/launchers.go`
  - `internal/worktree/list.go`
  - `internal/worktree/junction_windows.go`
- **Edits:**
  - `internal/worktree/weft_test.go`
  - `internal/worktree/portals_test.go`
  - `internal/worktree/launchers_test.go`
  - `internal/worktree/list_test.go`
  - `internal/worktree/junction_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add `//go:build integration` to all five files. Replace local fixtures: `weft_test.go` → `lyxtest.CopyPaired(t)` and pass `AddOptions{SkipPush: true}` to `w.Add`; `portals_test.go`/`launchers_test.go`/`list_test.go` → `lyxtest.CopyHostHub(t)` (they need only a hub; `list_test.go` adds extra worktrees via `lyxtest.MustRun`). `junction_test.go` (`TestCreateJunction`) keeps calling the unexported `createJunction` (white-box) but uses `t.TempDir()` dirs — it spawns `mklink` on Windows so it must be tagged; add `t.Parallel()`. Add `t.Parallel()` to all migrated tests (none use `t.Setenv`/`t.Chdir` after migration). Preserve every distinct assertion.
- **Commit:** `test(worktree): migrate weft/portals/launchers/list/junction tests, tag+parallelise`

### Card 12: migrate cli_test.go, delete drained helper files, confirm untagged split

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/worktree/cli.go`
  - `internal/worktree/config_test.go`
  - `internal/worktree/links_test.go`
  - `internal/worktree/prune_test.go`
- **Edits:**
  - `internal/worktree/cli_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/worktree/testhelpers_test.go`
  - `internal/worktree/helpers_test.go`
- **Requirements:** Add `//go:build integration` to `cli_test.go`. Re-express `setupCLIRepo` on top of `lyxtest.CopyHostHub(t)` (it writes `_lyx/worktree.yaml` and `t.Chdir(hub)`); these CLI router tests keep `t.Chdir` and stay **serial** (no `t.Parallel`). Pass `AddOptions{SkipPush: true}` wherever the remove-flow drives `w.Add`. Delete `testhelpers_test.go` (white-box helpers) and `helpers_test.go` (black-box duplicates) now that `mustRun`/`newTestRepo`/`addRemote`/`newWeftRepo`/`addWeftRemote` live in `lyxtest` — confirm no remaining reference to those local helpers compiles (grep the package). Confirm `config_test.go`, `links_test.go`, `prune_test.go` remain **untagged** and reference no deleted helper (they use only `t.TempDir()` + the unexported funcs under test); if any references a deleted helper, repoint it at `lyxtest`. This is the "drain confirmation" — no spawning `func Test...` may remain in an untagged file.
- **Commit:** `test(worktree): migrate cli tests, delete drained helpers`

## Batch Tests

`verify` runs both tiers: untagged `go test ./internal/worktree/...` (must pass offline — only `config_test.go`/`links_test.go`/`prune_test.go` run, zero git spawns) and `go test -tags integration ./internal/worktree/...` (full migrated suite). Equivalence guardrail: capture `-list` + `=== RUN` baselines to `.scratch/baseline-worktree.txt` before card 10; diff after card 12 and confirm superset (record table-driven folds in the PR). Run `go test -race -tags integration -count=2 ./internal/worktree/...` once to catch parallel shared-state leaks. The offline guarantee (no subprocess in the untagged build) is the load-bearing check for card 12's drain. Scratch files are not committed.
