# Review: lyx loom step + external supervisor skill

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [NIT:design] Skill loop's 30-step cap has no stated derivation
**Section:** Decisions — `skill-loop-has-a-hard-iteration-cap`
**Issue:** The cap is justified as a backstop against an unattended spend loop, but the number 30 itself isn't derived from anything checkable (e.g. producer-row count × a typical per-row bounce budget) — a plan writer could pick 50 or 100 with equal justification.
**Suggested fix:** Either derive it from the recipe's row count and typical `MaxBounces`, or note explicitly that the value is an arbitrary, adjustable safety margin rather than a computed bound.

## Verdict

APPROVE
Every structural, envelope, and constraint claim verified against actual source; scope, decisions, and testing strategy are thorough and free of BLOCKING gaps.
