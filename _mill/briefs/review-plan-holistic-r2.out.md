MILL_REVIEW_BEGIN
# Review: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [NIT:scope] Card 9 Context omits manifest/roadmap.md despite citing its line 98
**Location:** batch 01-raddle-fold-and-link-guard, Card 9 **Issue:** Requirements instructs writing "`manifest/roadmap.md:98`'s `scout-redesign.md` reference named as a live example this task leaves standing" into `CONSTRAINTS.md`, but Card 9's `Context:` list is only `internal/lyxcwd/docslink_test.go`, `manifest/designs/finalize.md`, `README.md`, `CLAUDE.md` — `manifest/roadmap.md` is absent. **Fix:** Add `manifest/roadmap.md` to Card 9's `Context:` so the implementer can confirm the line-98 citation against the live file rather than copying it unverified (this exact task's own discussion notes a prior stale line-number citation for the "Weft Git Invariant," so re-verifying citations before they land is warranted).

## Verdict

REQUEST_CHANGES — one minor Context-completeness gap; all cross-checked facts, anchors, line citations, and DAG/scope decisions verified accurate against source.
MILL_REVIEW_END
