MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [NOTE] Never-force-add enforcement left as "consider"
**Section:** Constraints — "Never force-add (NEW invariant)"
**Issue:** The grep enforcement test (no `-f` in git-add argv under gitrepo/fabricengine) is worded "consider," leaving machine-check vs review-obligation undecided.
**Fix:** State outright whether the new invariant is machine-checked or review-only; the structural removal of the `hasPathspecMagic` branch (gitrepo.go:171-192, verified) already makes force-add impossible, so a test is optional belt-and-suspenders — say so explicitly.

## Verdict

APPROVE
Scope, decisions, constraints, and testing are settled; all load-bearing source claims verify.
MILL_REVIEW_END
