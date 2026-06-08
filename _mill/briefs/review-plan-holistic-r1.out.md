MILL_REVIEW_BEGIN
# Review: board-modul (rename fra wiki) + _mhgo-konfigurasjon — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-06-08
```

## Findings

### [BLOCKING] Config struct missing yaml struct tags for ProposalPrefix

**Location:** Batch 2, Card 5
**Issue:** Card 5 specifies `type Config struct { Path, Home, Sidebar, ProposalPrefix string }` with no yaml struct tags. `gopkg.in/yaml.v3` lowercases exported field names verbatim, producing the YAML key `proposalprefix` for `ProposalPrefix` — not `proposal_prefix` as required by the discussion's `config-schema` decision and as assumed by card 18 ("The YAML key for the prefix is `proposal_prefix` (snake_case), matching the `Config` yaml tag used in batch 2") and card 20's tests.
**Fix:** Card 5 Requirements must name the required yaml struct tags explicitly: `ProposalPrefix string `yaml:"proposal_prefix"`` and confirm `Path string `yaml:"path"``, `Home string `yaml:"home"``, `Sidebar string `yaml:"sidebar"`` (these happen to match by default but naming them removes ambiguity for the implementer).

### [NIT] Card 2 Requirements ambiguously references cli.go for a non-existent struct-field rename

**Location:** Batch 1, Card 2
**Issue:** "rename the struct field `wikiPath` → `boardPath` (in `board.go`, and any references in `cli.go`)" — `cli.go` has a local variable named `wikiPath`, not a struct field reference, and card 2 explicitly says to keep that local variable unchanged until batch 4. An implementer may attempt to rename a variable they were told to preserve.
**Fix:** Drop the parenthetical mention of `cli.go`; clarify that the only struct field is in `board.go` and that `cli.go`'s local variable `wikiPath` is explicitly left unchanged (already stated later in the card, so a light reword suffices).

### [NIT] `go.sum` listed in both Context and Edits for card 5

**Location:** Batch 2, Card 5
**Issue:** `go.sum` appears in both `Context:` and `Edits:`, which is redundant — files in `Edits:` are already implicitly readable.
**Fix:** Remove `go.sum` from the `Context:` field; keep it only in `Edits:`.

## Verdict

REQUEST_CHANGES
One blocking issue: card 5 must specify yaml struct tags for `Config` to produce the `proposal_prefix` key required by the discussion schema.
MILL_REVIEW_END
