MILL_REVIEW_BEGIN
# Review: Extend worktree module: portals and launchers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-14
```

## Findings

### [BLOCKING] ide/cli_test.go never tests actual dispatch paths

**Location:** `internal/ide/cli_test.go:1-110`
**Issue:** All four tests (`TestRunCLISpawnDispatch`, `TestRunCLIUnknownSubcommand`, `TestRunCLIMissingSlug`, `TestRunCLINoArgs`) fail at `paths.Resolve` because they chdir into a non-git tmpDir; they never reach spawn dispatch, missing-slug handling, or the unknown-subcommand branch. The plan requires "spawn dispatch with a stubbed codeLauncher; unknown-subcommand and missing-slug error envelopes; usage on no args."
**Fix:** Either chdir into a real git repo (use `t.TempDir` + `git init` as the worktree package does) or construct a `paths.Layout` directly and call `Spawn`/`Menu` through a separate test that bypasses `RunCLI`; also add a test that drives `RunCLI` from inside a git repo with the launcher stub.

### [BLOCKING] menu_test.go missing plan-required coverage

**Location:** `internal/ide/menu_test.go:1-52`
**Issue:** Only `TestMenuHardErrorOnMissingBoard` is implemented. The plan mandates: "discovery excludes main and requires `_mhgo/`; titles come from the board facade; a numeric selection maps to the correct worktree and calls `Spawn`; the zero-worktree path prints its message." Three of four required scenarios are absent.
**Fix:** Add table-driven tests with a git-init'd container: verify that non-main worktrees without `_mhgo` are excluded, that numeric selection invokes `Spawn` (via `codeLauncher` stub), and that the zero-worktree message is printed.

### [NIT] menu.go silently swallows LoadConfig error via dead comment block

**Location:** `internal/ide/menu.go:35-41`
**Issue:** `cfg, err := board.LoadConfig(...)` assigns `err` but the `if err != nil { ... }` block only contains a comment; `err` is effectively discarded, which is misleading and the blank `cfg` will then reach `board.New` before `HealthCheck` catches it.
**Fix:** Replace with `cfg, _ := board.LoadConfig(l.Cwd, "board")` to make the intentional discard explicit, matching the plan's `cfg, _ :=` notation.

### [NIT] worktree/add.go rollback branch is dead code

**Location:** `internal/worktree/add.go:103-108`
**Issue:** The `if rollbackAddError == nil { ... } else { ... }` block returns identical values in both branches (`AddResult{}, err`), making the conditional meaningless.
**Fix:** Replace with a single `return AddResult{}, err` after the `w.rollbackAdd(...)` call.

## Verdict

REQUEST_CHANGES
Two test-coverage gaps prevent verifying plan-required behaviors for `ide/cli.go` and `ide/menu.go`.
MILL_REVIEW_END
