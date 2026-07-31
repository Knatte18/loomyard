MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude (Anthropic), Opus-class; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Third silent-drop path in commitWeftLocked unaddressed
**Section:** Technical context (`weftgit.go:349-400`) / `snapshot-tags-always-force-a-weft-commit`
**Issue:** The discussion names only two early returns (`!positive` at 368, `!committed` at 383), but `commitWeftLocked` has a third `return "", false, nil` at 378-380 — the `"did not match any files"` tolerance after `StageAndCommit` — so tags still drop silently there, contradicting "always force a weft commit".
**Fix:** State whether that tolerance path also falls through to the empty commit, and add it to the empty-commit case enumeration and its test list.

### [GAP] Unborn weft HEAD is reachable by the empty-commit path
**Section:** Testing (`internal/gitrepo` — `CommitEmpty`) / `unborn-warp-keeps-todays-behaviour`
**Issue:** "The `fabricengine` caller never reaches it" is justified by unborn *warp*, but unborn *weft* (born warp, zero weft commits) is a state the discussion itself tests for in the reader, and there the rule makes `CommitEmpty` create weft's **root** commit.
**Fix:** Decide the unborn-weft-HEAD behaviour explicitly as a fabricengine-level rule, not as "pin whatever `git commit --allow-empty` happens to do" in a primitive assumed unreachable.

### [GAP] Promoted parseSnapshotTags has no production caller
**Section:** Scope (In) / `scan-on-demand-no-index`
**Issue:** The reader parses git's `%(trailers:key=Snapshot,valueonly)` field (values only, newline-split), never a full commit message, so promoting `parseSnapshotTags` (verified test-only at `trailer_test.go:131`) creates dead production code — the exact thing `delete-ref-mechanism-outright` rejects for `remoteName`/`isStrictDescendant`.
**Fix:** Either name the production call site or drop the promotion and keep it a test helper.

### [GAP] Deletion footprint understates real test loss
**Section:** Technical context (Deletion footprint) / Testing (Deletion coverage)
**Issue:** "Not a coverage regression — the deleted code has no callers" is false for `keyvalidation_test.go`, which also covers `validSHA` — surviving production used by `ResetHard`, `ChangedFilesSince`, `CheckoutDetached`; and `gogit_test.go` loses `TestRemoteName_Parity`, `TestIsStrictDescendant_Parity`, and `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead` (the hard fingerprint-gated-reindex variant), plus the `freezePackIndex` helper.
**Fix:** Enumerate what survives (keep `validSHA`'s table; decide whether the repack/reindex case is re-anchored on a surviving method or accepted as lost, given `parity_test.go`'s `forcePackIndexFreeze` cases).

### [GAP] Affected godoc comments missing from the docs list
**Section:** Technical context (Docs to update)
**Issue:** The list names `fabricengine/doc.go`, `gitrepo/doc.go`, `CONSTRAINTS.md`, `raddle.md`, `fabric-unified-view.md` — but `Commit`'s own doc ("A fully degenerate no-op call … takes no lock, runs no `ensureWeftLockDir`, and spawns no push"), `commitWeftLocked`'s doc (both early returns, and "no Snapshot tags" on the unborn arm) and `CommitWeft`'s doc all become wrong on this commit.
**Fix:** Add those three godoc comments to the same-commit documentation list.

### [NOTE] Guard file's own header comment also names SnapshotSHA
**Section:** Technical context (Guard tests)
**Issue:** Only "lines 38-49" is flagged, but `gitrepoboundary_test.go`'s file header (lines 9-19, "# The one blind spot this guard cannot see") uses `SnapshotSHA` as its sole worked example too; and the replacement example is left as "must be chosen from a surviving pinned method" without confirming one exists.
**Fix:** Include the header comment, and confirm a surviving go-git/`r.run` mixed method exists before promising a replacement example.

### [NOTE] Tags-only Commit with zero files is not in the case list
**Section:** `snapshot-tags-always-force-a-weft-commit`
**Issue:** With `weftSide = (len(weftFiles) > 0 || len(snapshotTags) > 0)`, `Commit(nil, msg, []string{"raddle"}, opts)` now takes the combined lock and lands an empty weft commit — a fourth shape beyond the three enumerated.
**Fix:** State whether tags-only-with-no-files is a supported call shape or incidental, and pin it in the no-op/over-firing test.

## Verdict

GAPS_FOUND
Rule has an uncovered drop path; unborn-weft, dead helper, deletion footprint, and godoc unresolved.
MILL_REVIEW_END
