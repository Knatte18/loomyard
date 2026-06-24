The templates match the discussion's described conversions. The discussion notes weft's pathspec becomes a plain literal `"_lyx"` (no env var) — consistent with the current `# pathspec: _lyx` comment.

I have enough to evaluate. Let me consider the main-worktree weft.yaml question more carefully. The discussion says init now materializes weft.yaml at the host baseDir (`WorktreeRoot/RelPath/_lyx/config/weft.yaml`), and weft.LoadConfig reads from `WeftWorktree()/RelPath/_lyx/config/weft.yaml`. At a **task worktree** these are the same physical file via junction. But at the **main worktree**, `lyx init` is run, and `WeftWorktree()` = `WeftRepoRoot()` = `<Hub>/<prime>-weft`. Whether the main worktree's `_lyx` is a junction into the weft is the load-bearing question for whether strict `Load` of weft will succeed after `init`/`update`.

This is a real consideration, but the discussion explicitly scopes the junction relationship under the "config-path resolution" decision and states weft's baseDir split is unchanged (r1 Q&A). The existing `lyx config weft` command already relies on this same junction unification today (configcli edits at host baseDir; weft reads at weft baseDir). So this is pre-existing behavior, not introduced by this task. Not a gap.

The discussion is thorough, all decisions have rationale + rejected alternatives, scope in/out is explicit, testing is comprehensive and TDD-oriented, failure modes (migration, empty values, error-ignoring callsite) are addressed, and both r1 gaps are resolved in the Q&A. I found only minor citation imprecisions worth a NOTE.

MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\yamlengine\_mill\discussion.md
date: 2026-06-24
```

## Findings

### [NOTE] weft.LoadConfig baseDir citation is imprecise
**Section:** Technical context (weft wrappers) / Q&A r1
**Issue:** The text says weft keeps "`weftBaseDir` from `WeftWorktree()` (weft/cli.go:98)", but weft/cli.go:95 actually builds `filepath.Join(l.WeftWorktree(), l.RelPath)` and line 98 is only the `LoadConfig` call site.
**Fix:** Restate as `filepath.Join(WeftWorktree(), RelPath)` so the plan writer copies the correct (RelPath-mirrored) baseDir, not bare `WeftWorktree()`.

### [NOTE] --board-path child also drops applySkipEnv-relevant defaults
**Section:** Decision: remove DefaultConfig, no helper
**Issue:** cli.go:71-72 currently does `cfg = DefaultConfig(); cfg.Path = ...`; switching to `Config{Path: *boardPathFlag}` is correct since `Sync` only reads `Path` and `applySkipEnv` (cli.go:86) still runs, but the discussion does not explicitly note that `SkipGit/SkipPush` stay zero-valued (unchanged) under the new form.
**Fix:** One line confirming the detached child's behavior is unchanged (DefaultConfig never set the Skip fields either) to preempt a plan-writer reintroducing a helper.

## Verdict

APPROVE
Scope, decisions, failure modes, and testing are complete; only minor citation NOTEs remain.
MILL_REVIEW_END