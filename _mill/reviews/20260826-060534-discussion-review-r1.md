# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

(no findings)

## Verdict

APPROVE
Every load-bearing claim (entries_bouncer.go's WorktreeRoot resolution, hubgeom's BurlerGeometry/WebsterGeometry precedent, burlerengine.Profile.validate, Bouncer.Call's four-mode branching and judged/settle/seedCall logic, round.go/archive.go helpers, entries_burler.go's relative-passthrough comment, the current Plan-Bouncer/Plan-Revalidate recipe comments, the doc.go stale .json-vs-.md spelling, and all five cited CONSTRAINTS.md invariants) checked out exactly against source; scope covers both hubgeom.BurlerGeometry callers with nothing missed, and all eight decisions carry rationale and rejected alternatives.
