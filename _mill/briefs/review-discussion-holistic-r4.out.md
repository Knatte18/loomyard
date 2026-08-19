MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:scope] Task branch is never pushed before PR creation
**Section:** `publish-repo-resolution` / `publish-resume-reads-pr-state`
**Issue:** The word "push" appears nowhere as a decision; agents commit-per-fix on warp and never push (Review Round Invariant), so `<taskBranch>` exists only locally and `PullRequests.Create` with `head:<taskBranch>` fails 422 — the resume query for `head:<taskBranch>` likewise can never match.
**Fix:** Decide explicitly whether `Publish` pushes the warp branch (`fabricengine.PushWarpAt(warpPath, opts)` exists and is path-told, so no `lyxcwd` import), what it does on push failure, and whether the weft side pushes too — or state the precondition that something else pushes and name it.

### [BLOCKING:design] `MergeStageResolved`'s "maps to neither side" error is unreachable
**Section:** `merge-stage-resolved-verb` / Testing (fabricengine tier)
**Issue:** The stated routing is "the inverse of `unifyConflictPaths`", but that inverse is `weftPathVisible`'s prefix test (`mergepaths.go:39-50`) — a total function: under a wired name ⇒ weft, otherwise ⇒ warp. No path maps to "neither side", so the named test case cannot be written against the specified routing.
**Fix:** State the actual discriminator and its error condition — e.g. route by prefix but reject a path absent from that side's `ConflictedFiles()` (an index membership check), and say which of the two definitions the verb implements.

### [BLOCKING:design] Marker-scan verification undefined for absent or marker-bearing files
**Section:** `verify-before-conclude`
**Issue:** Verification is "re-read each path from `MergeResult.Conflicts` and check for remaining conflict markers", but `ConflictedFiles()` (`git diff --diff-filter=U`) includes delete/modify conflicts where a correct resolution deletes the file, so the re-read errors; conversely a legitimately marker-bearing resolved file (the conflict stencil itself will carry example markers) reads as unresolved forever.
**Fix:** Say what an unreadable/absent conflicted path means (resolved-by-deletion vs failure) and whether the marker scan is content-only or gated on the file still existing.

### [NIT:consistency] Inbound-reference count contradicts its own inventory
**Section:** Constraints → Markdown Link Integrity (line 436) vs `docs-lifecycle-landing-md-deletes`
**Issue:** The constraint says "twelve inbound references across six files"; the decision's inventory additionally names `CONSTRAINTS.md:427`, giving thirteen across seven. Verified counts: loom.md 4, shed.md 4, raddle.md 1, fabric-unified-view.md 1, roadmap.md 1, loomshed.go 1, CONSTRAINTS.md 1.
**Fix:** Restate as thirteen across seven files, or scope the count sentence to Markdown links only.

### [NIT:consistency] "guard-table entry" names the wrong table
**Section:** `merge-stage-resolved-verb`, Signature bullet
**Issue:** `cmd/lyx/destructiveguard_test.go` has no `Kind` table; the entry actually required is `{"StageResult", "internal/fabricengine/<file>.go"}` in `destructiveGuardMutatingResultTypes` (line 154). A new `Kind` with no recording site is explicitly caught by review, not the guard.
**Fix:** Name `destructiveGuardMutatingResultTypes` and the `StageResult` row directly.

## Verdict

REQUEST_CHANGES
Push gap, unreachable routing error, and undefined marker-scan edge cases block plan writing.
MILL_REVIEW_END
