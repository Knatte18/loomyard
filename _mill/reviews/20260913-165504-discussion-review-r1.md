# Review: Deploy cited spec/design docs to target repos like stencils

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:design] "Embed site covers both" glosses over a hard Go constraint
**Section:** Technical context, "The embed constraint is the one real structural obstacle."
**Issue:** The two options given are "the design doc's bytes reach the embed site by a different route" or "the embed site sits somewhere that covers both." The second option is misleading as stated: `//go:embed` patterns cannot contain `..` and cannot reach outside the embedding file's own directory subtree, so a single embed site can only "cover both" `contracts/specs/` and `manifest/designs/` if it lives at the repo root — an unusual placement this repo doesn't otherwise use. The two docs have no common ancestor below repo root, so this isn't a free choice between two equally-viable routes.
**Suggested fix:** Note explicitly that the two docs most likely need two separate embed sites (one under `contracts/specs/`, one under or beside `manifest/designs/`) each feeding the same logical `Registry`, since `Registry` only requires `Names()`/`Default(name)` and doesn't care how many `//go:embed` directives back it — this is a low-risk, mechanical resolution, not a blocker, but stating it saves the plan writer a detour into "can I put one embed file at repo root" before arriving at the same place.

## Verdict

APPROVE
Exceptionally well-grounded discussion — every spot-checked citation (embed constraints, `Reconcile` signature, geometry mirrors, the rubric marker-value-not-template trap, `rubric_test.go`'s pinned phrase, `planparser.ValidationError.Detail`, CONSTRAINTS.md invariant text) verified verbatim against source; the one NIT is a precision point on an already-correctly-deferred plan-level detail.
