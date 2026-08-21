MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Anthropic); the harness reports model id claude-opus-5. Self-assessment is a large Claude Opus-class model; I cannot verify the exact build from inside the session.
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [BLOCKING:scope] Moved sequence_test doc comment is never repaired
**Location:** batch 2 card 8 (and batch 6 card 26)
**Issue:** `internal/loomshed/sequence_test.go:13` says "a reordering in loomshed.go's producer table is a test failure"; card 8 prescribes only the package decl, `Name*` qualification, `New` repoint, row-1 substitution and `ShedPaths` repoint, so the sentence moves to `internal/loomrecipe/sequence_test.go` and is falsified by batch 5. Card 26's sweep tokens (`loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, `equivalence_test`, `coverageGuardLandingDeps`, `internal/loomshed` slashed/slashless, `resume_test`, `sequence_test`, `loomshed_test`) cannot match the bare `loomshed.go` spelling, and that file is not in card 26's `Edits:`.
**Fix:** have card 8 restate `wantSequenceOrder`'s doc comment against the recipe, and add a bare `loomshed.go` token to card 26's sweep list (the same spelling card 20 already repairs in `loomshed.go` and `loompreflight.go`).

### [NIT:scope] Card 8 scopes `Name*` qualification too narrowly
**Location:** batch 2 card 8
**Issue:** the card says "qualify every bare `Name*` reference in `wantSequenceOrder`", but `TestSequence_FullRunBlocksAtPublish` itself carries three bare `NamePublish` references (`sequence_test.go:58`, `:71`, `:89`) outside that var — card 9 states the unrestricted form for `resume_test.go`.
**Fix:** drop the "in `wantSequenceOrder`" qualifier so the instruction covers the whole file, matching card 9's wording.

### [NIT:consistency] Card 13's "expect zero hits" grep contradicts itself
**Location:** batch 3 card 13
**Issue:** the card says grep `internal/shedrecipe`'s `_test.go` files for `loomshed` and "expect zero hits", then in the next sentence tells the implementer to leave `internal/shedrecipe/seam_enforcement_test.go` alone — that file names `loomshed` twice today (its `shedrecipeAllowedImports` entry and its header prose), so the stated expectation is false against the tree.
**Fix:** state the expectation as "no hit outside `seam_enforcement_test.go`'s production-import allowlist and header".

### [NIT:consistency] Card 24 both corrects and deletes the same paragraph
**Location:** batch 6 card 24
**Issue:** the card tells the implementer to "close out the 'Shed recipe' group's remaining-work framing in the section intro" and to "correct" that intro's falsified `loomshed.go:137-151` reference, then later to "Delete that heading and its intro paragraph" — `manifest/roadmap.md:12-14` is the only such intro, so two of the three instructions are moot.
**Fix:** state the deletion once as the disposition of `roadmap.md:12-14` and drop the correct-it clauses that apply to the same lines.

## Verdict

REQUEST_CHANGES
One stale-comment enumeration gap plus three small self-contradictions; structure is sound.
MILL_REVIEW_END
