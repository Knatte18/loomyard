Confirmed: `update` import is test-only. The production importers of `internal/git` are: `weft`, `worktree` (deleted), `gitclone` (folded), `paths`, `board`, plus `git` itself. The discussion's importer list "weft, new warp, tests" omits the two production importers **`paths`** and **`board`** that survive this task.

MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\warp-module\_mill\discussion.md
date: 2026-06-25
```

## Findings

### [GAP] gitexec-rename importer list is incomplete
**Section:** Scope (In) / Decision `gitexec-rename` / Technical context
**Issue:** The rename lists importers to update as "`weft`, new `warp`, tests", but `internal/git` is also imported by two surviving production packages — `internal/paths/paths.go` + `internal/paths/worktreelist.go` and `internal/board/git.go` + `internal/board/sync.go` — plus test-only importers in `internal/update`, `internal/initcli`, `cmd/lyx`. Omitting `paths`/`board` leaves the build broken.
**Fix:** State that all `internal/git` importers are swept (explicitly naming `paths` and `board` as production importers, not just `weft`/`warp`/tests), since `paths` itself sits below `warp`.

### [NOTE] Launcher-menu checkout shortcut mechanism unspecified
**Section:** Scope (In) — "`lyx warp checkout` launcher-menu shortcut"; Decision references Q4
**Issue:** The "per-worktree launcher menu" is ambiguous: the per-worktree launchers (`launchers.go`, moved verbatim) emit `ide.cmd`/`ide-menu.cmd` calling `lyx ide spawn`/`lyx ide menu`; the interactive picker lives in `internal/ide/menu.go`, not warp. It is unclear whether the shortcut edits launcher generation or the `ide` menu (a different module).
**Fix:** Name the integration point (new launcher `.cmd` vs an `ide menu` entry) and, if it touches `internal/ide`, list that module in scope.

### [NOTE] post-checkout hook script source and Windows execution unspecified
**Section:** Decision `drift-detection-three-points-incl-hook`; Technical context (hook validation item)
**Issue:** The hook install mechanics (common `.git/hooks`, idempotent, chain existing) are decided, but the hook *script's* origin (embedded asset? generated?) and how it runs on Windows (git-for-Windows runs hooks under bash) are unstated; no hook infrastructure exists in the tree today.
**Fix:** Specify the hook body source and confirm the Windows execution path (shell), or explicitly defer that detail to the plan.

### [NOTE] Host-pollution guard targets `_codeguide` with no warp-wired junction
**Section:** Scope (In) — host-pollution guard; Decision `host-pollution-guard`
**Issue:** The guard detects/flags `_lyx`/`_codeguide` host-index pollution, but `paths.HostJunctions(slug)` returns only the `_lyx` entry and `_codeguide` junction activation is explicitly deferred — so warp wires no `_codeguide` junction this task. What "restore junction/exclude" means for a `_codeguide` path warp never created is underspecified.
**Fix:** Clarify whether `_codeguide` detection is report-only (no restore) this task, since warp creates no `_codeguide` junction yet.

## Verdict
GAPS_FOUND
One build-breaking importer omission must be resolved before plan writing.
MILL_REVIEW_END
