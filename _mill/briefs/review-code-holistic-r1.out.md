MILL_REVIEW_BEGIN
# Review: the standalone CLI path — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-18
```

## Findings

### [NIT:consistency] Stale HubPresent-trigger wording left in two doc comments
**Location:** `internal/preflight/predicates.go:61-63` (HubPresent doc) and `internal/webstercli/cli_integration_test.go:25-26`
**Issue:** `HubPresent`'s own doc comment still claims it is "the mode-selection trigger a standalone-capable CLI's pre-run consults", and the integration test's comment says cwd resolution failure means "preflight.HubPresent folds it into standalone mode" — both are now false since all three CLIs consult `ResolveMode`. Both are plan-mandated leave-as-is spots (card 1 explicitly requires `HubPresent`'s doc comment unchanged; card 12 explicitly forbids touching the test's existing doc comments), so this is a plan artifact, not an implementer slip, but it leaves two self-contradicting comments in the shipped tree next to `doc.go`'s freshly corrected "three functions" section.
**Fix:** Non-blocking; a future doc pass could soften `HubPresent`'s doc comment to say it "was" or "remains only" the stencil-seed gate without re-deriving mode-selection language, but this doesn't warrant its own commit here.

## Verdict

APPROVE
Implementation matches the plan's cards, shared decisions, and cross-batch contracts with no scope gaps or duplication found.
MILL_REVIEW_END
