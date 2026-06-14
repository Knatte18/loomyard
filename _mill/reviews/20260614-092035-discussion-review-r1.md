The porcelain parser identifies main (first block) and exposes `.Path` and `.Main`. The hub name can be derived from the main worktree's path basename at add time. The discussion's launcher menu logic is feasible.

I have enough to render the review. Let me compile findings.

Key issues found:
1. **Board-absence contradiction**: discussion says "board absent → hard error for the menu," but the board facade read methods (`ListTasksBrief`/`GetTask`) return `(nil, nil)` when board dir doesn't exist (board.go:191-193, 176-178). And `LoadConfig` errors only when `_mhgo/` is missing, not when the `_board` dir is missing. So an empty/absent board produces empty titles, not an error. The "hard error" path is ambiguous/unverified.

2. **relpath / LoadConfig interaction**: `RunCLI` calls `LoadConfig(cwd, "worktree")` which via `FindBaseDir` strictly requires `_mhgo/` at cwd (no walk up). So `worktree add` only works when invoked from the exact dir containing `_mhgo/`. The discussion's cwd-≠-gitroot fix derives `relpath = rel(gitroot, cwd)` — but if `_mhgo` is at the relpath subdir (not gitroot), then `LoadConfig(cwd)` works only when cwd == that subdir. The discussion doesn't reconcile that the existing `RunCLI` config-resolution already pins cwd to the `_mhgo`-bearing dir, which is the same dir where relpath should be computed. Worth a NOTE — the fix must update RunCLI/the call site, not just add.go/remove.go internals, since `container = parent(gitroot)` must replace `filepath.Dir(cwd)` but config resolution stays cwd-based.

3. **FindRoot path normalization**: `rev-parse --show-toplevel` returns forward-slash, possibly symlink-resolved paths on Windows; `filepath.Dir`/`rel` mixing with `cwd` (backslash) could mismatch. Launcher uses backslash paths for `mklink`. NOTE.

4. **secondarySideBar key uncertainty**: discussion itself flags "verify the exact key" — that's an acknowledged TBD but acceptable as it's implementation-time.

Let me reconsider #1 severity — it directly affects the menu's documented failure-mode behavior, so it's a GAP.

MILL_REVIEW_BEGIN
# Review: mhgo-portals-launchers

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-14
```

## Findings

### [GAP] "Board absent → hard error" contradicts facade behavior
**Section:** Decisions → board-is-sole-tasks-reader / ide-menu
**Issue:** The decision asserts an absent/unreadable board is a hard error for `ide menu`, but the facade read methods silently return empty: `ListTasksBrief`/`GetTask` short-circuit to `(nil, nil)` when the board dir is missing (`internal/board/board.go:191-193, 176-178`), and `LoadConfig` only errors when `_mhgo/` is absent — not when the `_board` dir is missing — so a missing board yields blank titles, not an error.
**Fix:** Decide and state the actual contract: either the menu treats empty/missing board as a real error (requires an explicit existence check, since the facade won't raise), or downgrade to "titles blank when board absent" and drop the hard-error language.

### [GAP] cwd-≠-gitroot fix must touch the config/call site, not only add.go/remove.go
**Section:** Decisions → cwd-not-gitroot
**Issue:** The fix names only `add.go:63`/`remove.go:36` (`container := filepath.Dir(sourceDir)`), but `RunCLI` resolves config via `LoadConfig(cwd,"worktree")` whose `FindBaseDir` strictly requires `_mhgo/` at cwd with no parent walk (`internal/config/config.go:31-40`) and passes `cwd` as `sourceDir` to Add/Remove (`internal/worktree/cli.go:49,72,107`). So `container` must become `parent(FindRoot(cwd))` while config stays cwd-based; the discussion conflates the two and doesn't say the call site keeps passing cwd for config but derives container/relpath from gitroot.
**Fix:** State that the call site continues resolving config from cwd (the `_mhgo`-bearing dir at relpath) and that `container`/`relpath` are derived from `git.FindRoot(cwd)`, so the strict cwd config-resolution invariant is preserved.

### [NOTE] FindRoot path form may mismatch cwd for path joins
**Section:** Decisions → launcher-content-relative-paths / Technical context
**Issue:** `git.FindRoot` returns `rev-parse --show-toplevel` output (forward slashes, possibly symlink-resolved on Windows), while `cwd` from `os.Getwd()` uses backslashes; `filepath.Rel(gitroot, cwd)` and the backslash `mklink`/`%~dp0` paths could mismatch if forms differ.
**Fix:** Note that gitroot must be normalized via `filepath.Clean`/`FromSlash` before computing `relpath` and building launcher/junction paths.

### [NOTE] hub-name derivation for ide-menu.cmd left implicit
**Section:** Decisions → launcher-content-relative-paths
**Issue:** `ide-menu.cmd` bakes `<hubname>` = main worktree dir name, but the discussion doesn't say how `add` obtains it; the porcelain parser marks the first block `Main=true` with `.Path` (`internal/worktree/list.go:65,76`), so hubname = basename of that path — but this dependency is unstated.
**Fix:** State that hubname is `filepath.Base` of the `Main` worktree entry from `git worktree list --porcelain`.

## Verdict

GAPS_FOUND
Two decisions contradict verified facade and config behavior and must be reconciled before planning.
MILL_REVIEW_END