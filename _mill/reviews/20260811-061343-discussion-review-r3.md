MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

Verified against source: `destructiveguard_test.go` is directory-scoped and does reach a `fabrictest` subdir (walk skips only `_test.go`); `fabricVocabularyOwners`/`weftnameImportOwners` are exact-match maps at `enforcement_test.go:597`/`:609`; `checkForce` is declared (`destroy.go:39`) and never constructed into a `destructiveRefusal`; `remove.go:61-66` portal/launcher removal precedes the dirty pre-flight at `:68-76`; `clone.go:585` is `dirtinessNA`; `checkout.go:42` is `scopeTracked` on the weft worktree; `pull.go:143` is `scopeTracked` on warp; every verb signature in Technical Context matches the tree; `runCloneWithReset`'s post-clone sequence matches the extraction description exactly.

## Findings

### [BLOCKING:design] Manifest entry model and `.git` handling undecided
**Section:** §survival-assertion-mechanism / Testing item 3 **Issue:** The decision says "fail on any disappearance or mutation" but never says what a manifest entry records (content hash? size? link raw target? existence only?) nor how `.git` churn is kept out — Testing item 3 states ".git internals churning does not produce noise" as a required property with no chosen mechanism, and the two options (exclude `.git` subtrees globally vs permit them per cell) have opposite consequences: global exclusion blinds the harness to `.git/worktrees` registration destruction, which is exactly R3's shape. **Fix:** Decide the per-entry record and the `.git` rule explicitly, and state which git-admin paths remain observable under it.

### [NIT:scope] Hostile-input cells' state crossing is unspecified
**Demoted-from:** BLOCKING
**Section:** §tranche-1-verb-table / §cross-product-shape **Issue:** Hostile inputs are scoped to three verbs but never to a state set; under the stated `[]State × []VerbCase` shape a hostile input expressed as an extra `VerbCase` inherits all nine states and both anchors (7 inputs × 3 verbs × 9 × 2 ≈ 378 cells), nearly all vacuous since `remove ..` refuses at `remove.go:45` regardless of hub state — the same vacuity the `Reset` column was explicitly re-scoped to avoid. **Fix:** State whether hostile-input cells run in `clean` only (and how that exception is expressed without forfeiting the cross-product property), and give the resulting cell count.

### [BLOCKING:design] Sabotage-proving has no mechanism or artifact
**Section:** §Testing, "Sabotage-proving" **Issue:** "should be confirmed to fail when the corresponding gate check is neutered" is non-committal and names no mechanism — neutering `destroy.go` is a production edit not in scope, and nothing says whether the exercise is manual, scripted, recorded in `doc.go`, or a completion gate. **Fix:** Name the mechanism and the durable artifact (or explicitly move sabotage-proving out of this slice with a reason).

### [NIT:consistency] Build tag for `testmain_test.go` stated two ways
**Section:** §package-file-layout-and-build-tags **Issue:** "`doc.go` carries no build tag; every other file carries `//go:build integration`" contradicts the same section's file list ("`testmain_test.go` (untagged)") and the Constraints section ("everything except `doc.go` and `testmain_test.go`"). **Fix:** Say `doc.go` and `testmain_test.go` are untagged, matching `boardtest`.

### [NIT:consistency] Package path written as `internal/fabrictest`
**Section:** §Testing, second bold heading **Issue:** Reads "`internal/fabrictest` — the harness", where every other mention is `internal/fabricengine/fabrictest`; the nesting is load-bearing for the guard and owner-map work. **Fix:** Use the full path.

### [NIT:consistency] "Snapshot" collides with existing fabric vocabulary
**Section:** §survival-assertion-mechanism **Issue:** `fabricengine/snapshot.go` already uses Snapshot for the weft `Snapshot:` trailer recording a warp SHA; the harness reuses the word for a filesystem manifest. **Fix:** Note the distinction in `doc.go`, or name the harness operation something unambiguous.

## Verdict

REQUEST_CHANGES
Three undecided items: manifest entry model, hostile-input cell count, sabotage-proving mechanism.
MILL_REVIEW_END
