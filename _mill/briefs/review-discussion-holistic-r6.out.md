MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Fable 5, claude-fable-5)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:consistency] landingshed cannot reference PushWarpRebaseFreeAt
**Section:** publish-repo-resolution / told-values-via-landingshed-deps / Constraints **Issue:** The Constraints section says neither new package "may name warp or weft", and `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) flags every `*ast.Ident` containing "warp"/"weft" as a case-insensitive substring in non-owner production files — `fabricVocabularyHits` walks all identifiers, so a call expression `fabricengine.PushWarpRebaseFreeAt(...)` inside `internal/landingshed` trips it, as would a Deps field or doc comment for the told "warp path" and the "Only warp is pushed" package doc content; yet the design has `Publish` calling that verb directly and `Deps` carrying a warp path. **Fix:** State the resolution — e.g. inject the push as another closure in `landingshed.Deps` filled by the CLI layer (matching the two Fabric opener closures) or give the verb/field warp-free names — and note the same constraint governs every fabricengine identifier the two packages touch.

### [NIT:consistency] Round-4 Q&A entry still names PushWarpAt
**Section:** Q&A log (round 4 push entry) **Issue:** The round-4 answer says the push goes "via `fabricengine.PushWarpAt`", superseded by round 5's `PushWarpRebaseFreeAt` decision, with no supersession marker on the stale entry. **Fix:** Annotate the round-4 entry as superseded by round 5 so a skimming plan writer cannot implement the rejected verb.

### [NIT:design] ScratchDir creation unassigned
**Section:** mergeresolve-drives-shuttle-directly / stuck-reasons-are-logged-and-filed-never-returned **Issue:** `<AnchorPath>/.lyx/landing` must exist before the session writes `conflict-resolution-r<attempt>.md` and before either producer writes `<producer>-stuck.md`, but no decision says which component mkdirs the told ScratchDir. **Fix:** State that `landingshed` (or `mergeresolve`) creates its told ScratchDir on first use — creating a told path is legal under the Told-Geometry Invariant, mirroring shedengine's lock-parent precedent.

## Verdict

REQUEST_CHANGES
One vocabulary-invariant contradiction blocks; the push call path needs a warp-token-free shape.
MILL_REVIEW_END
