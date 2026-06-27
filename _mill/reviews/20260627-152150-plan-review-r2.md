MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-27
```

## Findings

### [NIT] "verbatim" body copy ignores arg-index shift
**Location:** Batch 4 Card 12 (board), also Batch 5 Card 16 (warp)
**Issue:** Handlers read the subcommand at `rest[0]` and payload/slug at `rest[1]`/`fs.Arg(0)`; under `WrapRun(fn(out, args))` cobra strips the subcommand so the payload is `args[0]` — copying the case body "verbatim" leaves a dangling `rest`.
**Fix:** Note in the cards that JSON-payload/slug extraction must rebind to `args[0]` (and `fs.Arg(0)` → positional `args`).

### [NIT] --json leaf test may pick a flagless leaf
**Location:** Batch 6 Card 22
**Issue:** Card asserts "a leaf has populated flags," but most leaves (e.g. `board upsert`) have no local flags, making the assertion vacuous or failing.
**Fix:** Pin the leaf to one with a local flag (e.g. `lyx update --help --json` for `--apply`, or `warp remove`).

### [NIT] muxpoc table-test shares now-divergent assertions
**Location:** Batch 5 Card 17
**Issue:** `TestRunCLIErrors` asserts exit 1 + empty stdout for all three cases; post-cobra all three diverge (no-arg → exit 0 non-empty; unknown subcommand/flag → exit 1 non-empty), so the shared `out.Len()!=0` guard must be dismantled, not just re-pointed.
**Fix:** Card should state the table must be split into per-case expectations.

## Verdict

APPROVE
Plan is constraint-clean, decision-faithful, and cobra-correct; only minor test/wording nits remain.
MILL_REVIEW_END