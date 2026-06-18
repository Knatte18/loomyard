# Review: Weft repo — companion-repo overlay for lyx

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-18
```

## Findings

### [GAP] Rename touchpoints table omits multiple files
**Section:** Technical context → Rename touchpoints
**Issue:** Grep for `Container|MainWorktree|HubName` finds production-touching files not in the table: `internal/ide/spawn_test.go` (8 sites), `internal/ide/menu_test.go` (10 sites), `internal/worktree/launchers_test.go` (1 comment), and `docs/shared-libs/paths.md` (full Layout-type doc with ~14 references). The paths.md doc is the authoritative shared-libs reference and must rename in lock-step.
**Fix:** Add these four files to the rename table; explicitly call out `docs/shared-libs/paths.md` as the canonical Layout doc to update.

### [GAP] Config migration touchpoints table omits fixture-bearing tests and docs
**Section:** Technical context → Config path migration touchpoints
**Issue:** Files that seed `_lyx/board.yaml` or `_lyx/worktree.yaml` and are NOT listed: `internal/board/config_test.go` (≥4 sites), `internal/board/cli_test.go`, `internal/board/boardtest/bench_test.go`, `internal/board/boardtest/concurrency_test.go`, `internal/worktree/config_test.go`, `cmd/lyx/main_test.go` (lines 43, 73). Docs and comments also reference the old path: `docs/shared-libs/config.md` (l.31, 51), `docs/shared-libs/README.md` (l.19), `docs/modules/board.md` (l.153, 227, 234, 262, 299), `docs/benchmarks/board-performance.md` (l.29, 65, 66), `cmd/lyx/main.go` (l.13), `internal/board/init.go` (l.3, 22).
**Fix:** Expand the migration table to enumerate every fixture/comment/doc site; a "hard cut" implies all of these break unless updated together.

### [GAP] Where the canonical weft architecture lands is unspecified
**Section:** Scope / Decisions → Weft model
**Issue:** Scope says "document the weft model as the new architecture" across `overview.md`, `roadmap.md`, `worktree.md`, `board.md`, `CONSTRAINTS.md`, but does not say where the *canonical* description of the weft model lives (a new `docs/architecture/weft.md`? a new section in `overview.md`? distributed paragraphs?). A plan writer will have to invent the structure.
**Fix:** Name the target file and section for the canonical weft architecture write-up; specify what each other doc gets (full description vs. pointer).

### [NOTE] CONSTRAINTS.md method list update not spelled out
**Section:** Technical context → Rename touchpoints (CONSTRAINTS.md row)
**Issue:** Row says only "Layout method list (currently lists `HubName()`)". The line also implicitly anchors the `internal/paths.Getwd()` / `Resolve()` invariant — only the method name `HubName()→PrimeName()` needs swapping; "Container/MainWorktree" do not appear in CONSTRAINTS.md (verified). Make this explicit so reviewers don't hunt for non-existent field references.
**Fix:** State that the CONSTRAINTS.md change is a single token swap (`HubName()` → `PrimeName()`).

### [NOTE] Portal geometry methods retained in task 005
**Section:** Decisions → Portals deprecated
**Issue:** Discussion says portals "removed in task 006" but does not explicitly state that `PortalsDir()`, `PortalLink()`, `PortalTarget()` remain in `internal/paths/paths.go` and `docs/shared-libs/paths.md` for this task. Implementer might assume "deprecated" means remove the methods now.
**Fix:** Add one line: "Portal geometry methods on Layout stay in task 005; removal is part of task 006."

## Verdict

GAPS_FOUND
Touchpoint tables under-enumerate; weft-doc landing location and CONSTRAINTS scope need pinning before planning.