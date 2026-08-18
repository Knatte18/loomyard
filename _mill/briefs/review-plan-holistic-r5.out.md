MILL_REVIEW_BEGIN
# Review: the standalone CLI path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5 / Claude Sonnet 4.5-class)
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] "byte-identical, four deviations" decision omits webstercli's envelope
**Location:** overview.md's "hub mode stays byte-identical, with exactly four named deviations" Shared Decision (Applies to: all batches), vs batch 3 (webstercli-repoint).
**Issue:** Deviation (3) states three additive envelope fields (`mode`, `stateDir`, `stencilsDir`) "appear in both modes' run-verb success envelopes," with no CLI scoping, under a decision that applies to all batches. Verified against source: `websterCLI` (cli.go) carries no `mode`/`stateDir`/`stencilsDir` fields today, `run.go`'s `output.Ok` map has no such keys, and batch 3's cards 9-12 explicitly declare "no verb, no engine wiring, no flag, and no other behaviour in the package changes" — webster's `run` envelope never gains these fields. Cards 15/22 (batches 4/5) do add them for burler/perch. As written, the decision's own claimed-exhaustive deviation list is false for `webstercli`'s run-verb envelope, defeating the decision's stated purpose ("lets a reviewer treat any other observed change as a regression without re-deriving intent").
**Fix:** Either scope deviation (3)'s wording explicitly to burler's and perch's run-verb envelopes (excluding webstercli, which already shipped without them), or add a card to batch 3 giving `webstercli` the same three envelope fields for consistency across the three CLIs this task explicitly unifies on one mode-trigger rule.

## Verdict
REQUEST_CHANGES
One Shared Decision's claimed-exhaustive deviation list is contradicted by batch 3's own declared narrow scope; everything else checked out clean.
MILL_REVIEW_END
