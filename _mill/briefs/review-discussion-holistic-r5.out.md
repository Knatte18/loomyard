MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:design] Drift guard conflates two distinct absences
**Section:** Decision: batch-coverage-disposition (1)
**Issue:** The guard is specified over the `introduced` set (`subs` values) against answers for `collectGlyphTargets(reloaded, lang)`, but those sets are not equal: `RewriteRefs` reports nothing about which substitutions landed ("an empty or fully-unmatched subs map writes nothing at all", `rewrite.go:26-43`), and an unlanded sub — or a `newID` `glyph.Parse` rejects — leaves the glyph absent from the reloaded plan and therefore from the batch, so the guard raises `ErrQuarryUnavailable` where today the pass is silent; the sibling site got exactly this carve-out in r4 (`containment.go`'s `!resolved` half), yet drift's absence-legitimacy is never examined.
**Fix:** State the disposition for "introduced glyph not present in the post-repair batch at all" separately from "present in the batch but unanswered" — which of the two the guard covers, and what happens to the other.

### [NIT:consistency] Enforcement boundary stated twice, differently
**Section:** Decision: kind-policy-ledger
**Issue:** "The enforcement boundary is exactly the pair `{classify.go, shape.go}` — the scans exempt both files and nothing else" contradicts meta-test (3)'s own exemption of "the exported handle helpers (planparser's handle.go)", which is load-bearing: `handle.go:29`/`handle.go:39` implement the grammar with raw `HasPrefix`/`TrimPrefix` against `HandlePrefix` and must stay legal.
**Fix:** Give the exempt set per scan — `{classify.go, shape.go}` for the `refKind` scan, plus `handle.go` for the `plan:`-op scan — and drop "nothing else".

### [NIT:design] refKindName's prose path at migrated finding sites
**Section:** Decision: kind-policy-ledger / Inventory A (`validate.go:787`, `:797`)
**Issue:** `dispFinding` arms must keep emitting `refKindName(k)` prose to satisfy behavior-preservation, which needs a `refKind`-typed value at a site outside the boundary; the discussion bans only "comparisons", never saying whether a bare `classifyRef` call outside the boundary stays legal or whether the lookup helper hands back the kind/prose.
**Fix:** Say which — helper takes a string and returns disposition plus kind/prose, or bare `classifyRef` calls remain legal outside the boundary.

### [NIT:consistency] "resolveContainment left unchanged" overstated
**Section:** Decision: parse-success-glyph-test-kept vs. Decision: status-family-disposition
**Issue:** The former says `resolveContainment` is "left **unchanged**" while the latter changes `containment.go:112` inside that same function.
**Fix:** Scope the word to the glyph/handle test ("its glyph test and handle handling are unchanged; its `Status` guard is the one hardening").

## Verdict

REQUEST_CHANGES
Drift coverage guard's absence semantics undecided; three minor consistency/design gaps.
MILL_REVIEW_END
