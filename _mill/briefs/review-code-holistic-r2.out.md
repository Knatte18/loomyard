MILL_REVIEW_BEGIN
# Review: Reed and Fabric as standalone modules: public API design — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-06
```

## Findings

### [NIT:consistency] Two short parenthetical asides still carry a mid-line semicolon
**Location:** `manifest/designs/reed-fabric-standalone-api.md:495,497`
**Issue:** `(`StripEnvKeys`'s shape is fixed; its exact name is not).` and `(the pair-kernel/hub-layout boundary is fixed; naming beyond that is not).` each join two independent clauses with a semicolon that has trailing text on the same line, which `tools/mdreflow`'s own `semicolon_split` behavior would break onto two lines. This is the same convention gap a prior round judged non-blocking (then at different line numbers, since intervening batch-2 content shifted the file); no new evidence here changes that call, so it stays a NIT.
**Fix:** Either split each parenthetical across two lines at the semicolon, or leave as-is by reviewer discretion, matching the prior round's disposition.

## Verdict

APPROVE
All cards across both batches are realized, cross-batch references and shared decisions hold, and every checked claim is verbatim-accurate against the cited source.
MILL_REVIEW_END
