This confirms a real concern: `internal/ide/menu.go:39` calls `board.LoadConfig` and **ignores the error** (`cfg, _ :=`), relying today on `DefaultConfig()` to populate `cfg` even when load partially fails. Under strict Load, a missing-key or absent-file error returns a zero `Config{}`, so `b.HealthCheck()` runs against an empty `cfg.Path`. The discussion lists board/worktree/weft `LoadConfig` callers but does not call out this error-ignoring consumer.

I have enough to write the review.

MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [GAP] error-ignoring LoadConfig consumer not addressed
**Section:** Technical context → Typed wrappers / strict Load
**Issue:** `internal/ide/menu.go:39` does `cfg, _ := board.LoadConfig(...)` and relies on today's `DefaultConfig()`-populated `cfg` even on error; under strict Load it gets a zero `Config{}` (empty `cfg.Path`) before `HealthCheck()`.
**Fix:** Name this consumer; specify it must now handle the error (or accept that strict Load means empty cfg) so the menu does not silently HealthCheck an empty board path.

### [GAP] weft Load signature vs host-baseDir junction unstated
**Section:** Decisions → config-path resolution / strict Load
**Issue:** `weft.LoadConfig` is called with `weftBaseDir` from `WeftWorktree()` (weft/cli.go:98), while `update`/`init` resolve weft at the host baseDir via junction; the discussion asserts these are one physical file but does not state weft's `LoadConfig(baseDir, module, template)` call site / signature stays host-vs-weft as-is.
**Fix:** State explicitly that `weft.LoadConfig` keeps reading via `WeftWorktree()` (unchanged baseDir) and only its internal `config.Load` call gains the template arg.

### [NOTE] empty-default key vs strict missing-key diff
**Section:** Decisions → strict Load + env-marker grammar
**Issue:** `worktree.yaml` resolves `branch_prefix` to `""` via `${env:LYX_BRANCH_PREFIX:-}`; strict Load diffs key-paths (presence), not values, so an empty-valued key must count as present — worth stating to avoid a planner treating empty as missing.
**Fix:** Add one line: the key-path-set diff is presence-based; a key with an empty resolved value is present.

### [NOTE] template func shape left as plan detail but registry type couples it
**Section:** Technical context → registry / ConfigTemplate
**Issue:** Registry today is `{Name, Template func() string}` (configcli.go:23); discussion leaves "func() string vs bytes" to mill-plan but `templateFor`/`editOne` consume `func() string`, so the choice ripples into `lyx config`.
**Fix:** Note that whichever shape is chosen, `configcli`'s `templateFor`/`editOne`/`config.Edit(...,template,...)` consumers must be updated in lockstep.

## Verdict
GAPS_FOUND
Two consumer/signature gaps (ide menu error-handling; weft baseDir) need resolution before planning.
MILL_REVIEW_END