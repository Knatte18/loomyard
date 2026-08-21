MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [NIT:consistency] Self-qualified `loomrecipe.New` inside `package loomrecipe`
**Location:** batch 2 cards 6/7/8/9, batch 3 card 12 **Issue:** each card sets the file to `package loomrecipe` and then prescribes calls spelled `loomrecipe.New(env, paths)`, a qualifier that does not compile from inside the declaring package. **Fix:** spell the call `New(env, paths)` in those cards, keeping `loomshed.Name*` qualified as written.

### [NIT:consistency] Two hand-maintained thirteen-row tables land in one package
**Location:** batch 2 card 6 (`shape_test.go`'s `wantProducerTable`) vs card 7 (`recipe_test.go`'s expected-value table) **Issue:** card 7's shape assertion is a strict superset of `TestNew_ProducerTable`/`TestNew_ProducerTableOrderUnchangedByWiring` (same Name/OnDone/OnStuck/Segment/MaxBounces, plus `reflect.TypeOf`), and neither card states a disposition for the overlap, so a future row change must be applied to two tables. **Fix:** say explicitly which table is authoritative and what the other adds, or fold the two into one.

### [NIT:consistency] Card 26 says "these four" for six enumerated repairs
**Location:** batch 6 card 26 **Issue:** the card enumerates six comment repairs (matching the batch scope's "six Go doc comments") but the sweep sentence reads "Then sweep wider than these four". **Fix:** change to "these six".

### [NIT:consistency] `NamePublish` count in card 8 is off
**Location:** batch 2 card 8 **Issue:** the card says `TestSequence_FullRunBlocksAtPublish` "carries four more `NamePublish` references outside that var"; `internal/loomshed/sequence_test.go` carries five (lines 58, 59, 71, 89, 90). **Fix:** drop the count and keep the operative "qualify every bare `Name*` reference", or correct it to five.

### [NIT:consistency] Batch 4 does not flag cards 15-17 as one compile unit
**Location:** batch 4 Batch Scope / cards 15, 16, 17 **Issue:** card 15 assigns `c.env`/`c.shedPaths` before card 16 declares them, and card 16 removes `deps` while cards 17-19 still read it, so three consecutive per-card commits do not compile — batch 2 states this coupling explicitly for cards 5-6, batch 4 does not. **Fix:** add the same one-line note to batch 4's Batch Scope that the three cards are green only at the batch boundary.

## Verdict

APPROVE
Accurate, well-grounded plan; only cosmetic and wording defects remain.
MILL_REVIEW_END
