MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [NIT:consistency] Batch 7's Batch Tests paragraph miscounts the tagged fixture files
**Location:** 07-webster-told-deps.md, Batch Tests section
**Issue:** The paragraph claims "five of the six fixture files cards 31 and 32 touch ... are `//go:build integration`" and lists `beginbatch_test.go, recordbatch_test.go, recoverbatch_test.go, runlevel_test.go and webstercli's verbs_test.go`. Verified against source: of the six websterengine files cards 31+32 actually touch (`beginbatch_test.go, recordbatch_test.go, recoverbatch_test.go, runlevel_test.go, audit_test.go, template_test.go`), only four carry `//go:build integration` (`audit_test.go` and `template_test.go` are untagged); `verbs_test.go` is a separate, correctly-tagged file from a different package edited by card 33, not one of "the six."
**Fix:** Reword to state four of the six websterengine fixture files (naming `audit_test.go`/`template_test.go` as the untagged pair) are tagged, and separately note `webstercli/verbs_test.go` (card 33, different package) is also `//go:build integration`.

### [NIT:consistency] Card 13 leaves one accessor's doc comment citing a non-existent invariant
**Location:** 05-webster-accessors-told.md, Card 13
**Issue:** `internal/websterengine/state.go`'s `ReportsDir` doc comment currently reads "Per the Hub Geometry Invariant" (no such invariant exists in CONSTRAINTS.md) while `Dir`, `ScratchDir`, and `PromptsDir` all correctly say "Per the Cwd Resolution Invariant." Card 13 requires updating every one of these four doc comments' told-parameter wording and explicitly says to "keep each one's existing 'no other package may construct this path' sentence," which preserves this stray wrong reference right alongside three correct ones in the same edit.
**Fix:** Add a one-line requirement to Card 13 correcting `ReportsDir`'s "Hub Geometry Invariant" reference to "Cwd Resolution Invariant" while the doc comment is already being touched.

## Verdict

APPROVE
Extensively source-verified across all 8 batches/43 cards; only two non-blocking prose/doc-comment nits found.
MILL_REVIEW_END
