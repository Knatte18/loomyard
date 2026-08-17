MILL_REVIEW_BEGIN
# Review: planparser owns the plan-directory path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [BLOCKING:consistency] Q&A still says `cli.go:194` stays uncovered
**Section:** Q&A log (line 356) vs §anchor-always / §Technical context / Testing (84, 248, 324-331, 358)
**Issue:** The r1 re-decision adds a `"backend"`-anchored `PersistentPreRunE` case that covers `cli.go:194` itself, but the earlier Q&A row still answers "state plainly that `cli.go:194`'s own line stays uncovered until T7" — a plan writer reading only the Q&A implements the superseded decision.
**Fix:** Rewrite that Q&A answer to the re-decided outcome (three coverage levels, `cli.go:194` covered by the new pre-run case, no T7 dependency).

### [BLOCKING:scope] Stale-comment inventory misses `websterengine/runlevel.go:100`
**Section:** Scope, "Update three stale comments the code change falsifies"
**Issue:** `internal/websterengine/runlevel.go:100` reads "PlanDir, WebsterDir, ReportsDir, PromptsDir, ScratchDir, WorktreeRoot are lyxcwd- resolved paths" — the identical falsehood the in-scope `cli.go:57-58` fix targets, and `RunDeps.PlanDir` is fed verbatim from `c.planDir` (`internal/webstercli/run.go:66`). The stale-comment set was assembled by reading, with no enumeration method stated, so nothing bounds it at three.
**Fix:** State the enumeration method used for stale comments (e.g. a grep for "lyxcwd-resolved"/"resolved" near plan-path fields) and take its full result into scope, `runlevel.go:100` included.

### [NIT:consistency] `cli_test.go` anchor flip is tautological too, unlike the cmd/lyx rows
**Section:** §anchor-always "three levels" / Testing → `internal/webstercli`
**Issue:** `newTestCLI` (`cli_test.go:169-177`) both computes `planDir` and seeds into `c.planDir` (`:201`, `:252`), so a `WorktreePath()` slip stays self-consistent and passes at any `AnchorRel` — the same weakening the discussion carefully annotates for `cmd/lyx/constructoranchoring_test.go`, unrecorded here.
**Fix:** Say what the `cli_test.go` flip does prove (nested-anchor path handling) and that only the pre-run case and the `loomengine.PlanSpec` case prove anchoring.

## Verdict

REQUEST_CHANGES
One superseded Q&A answer and an unbounded stale-comment inventory; everything else verified sound.
MILL_REVIEW_END
