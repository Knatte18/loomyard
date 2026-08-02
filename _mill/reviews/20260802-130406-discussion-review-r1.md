MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] Card path is absolute, not worktree-relative
**Section:** card-source-path-in-planparser; Technical context (line 85–86); Testing (planparser)
**Issue:** `planparser.Plan.Dir` is `hubgeometry.PlanDir(l.WorktreeRoot)` — absolute in production, a `t.TempDir()` in tests (plan.go:14–17) — so a Dir-derived card path is absolute, yet the design and the test assertion both require the worktree-relative `_lyx/plan/NN-<slug>.md` the `_lyx`-junction / weft-reference audit depends on.
**Fix:** State that the new field is a freshly constructed relative token (via `hubgeometry.LyxDirName` + `plan/` + filename), independent of the absolute `Dir`, so the pointer and the test both see `_lyx/plan/NN-<slug>.md`.

### [GAP] Recovery strand's cwd-is-worktree assumption unconfirmed
**Section:** full-cold-recovery-prompt; Technical context (recoverbatch, line 86)
**Issue:** Card-by-pointer plus the cold template make the recovery strand — a genuinely separate process — newly depend on its process cwd being the worktree root for `_lyx/plan/…`, `_pattern/PATTERN.md`, and `CONSTRAINTS.md` to resolve; today it gets everything inlined, and `recoverbatch.go`'s `shuttleengine.Spec` (lines 226–235) sets no explicit working dir.
**Fix:** Confirm (and note where) the recovery strand is started with cwd = worktree root, or the pointer-only recovery prompt dangles and starts the strand truly blind.

### [NOTE] noSharedDecisions removal coupled to a vetoable fold-in
**Section:** card-content-by-file-pointer vs integration-fork-drops-shared-decisions
**Issue:** The decision states `noSharedDecisions` "becomes dead … and is removed" unconditionally, but `RenderIntegrationPrompt` still uses it (render.go:269); if the user vetoes the integration fold-in the const stays alive, contradicting "nothing else depends on it."
**Fix:** Make `noSharedDecisions` removal explicitly contingent on the integration fold-in surviving veto.

## Verdict

GAPS_FOUND
Path-relativity and recovery-strand cwd must be pinned before plan writing.
MILL_REVIEW_END
