MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] `pattern.DirName` consumers outside the enumerated surface
**Section:** Technical context → "The `_pattern` surface, enumerated" **Issue:** Deleting the `DirName` const breaks compilation in packages the enumeration never names: `cmd/lyx/constructoranchoring_test.go:83,135`, `internal/loomengine/preflight_integration_test.go:452,498`, `internal/loomengine/plan_test.go:127`, `internal/fabriccli/cli_test.go:464`, plus `internal/fabricengine/{dotlyxjunction_integration_test.go:301,314, add_rollback_adopt_test.go:159,228, remove_junctions_integration_test.go:78, hostjunction_test.go}`. **Fix:** Enumerate every `pattern.DirName`/`pattern.Dir(` call site as the authoritative blast radius, not just the three template_test files.

### [GAP] loomengine preflight tests are *about* the second junction
**Section:** Testing → "Junction teardown" **Issue:** The substitution-to-`_extra` list is fabricengine-only, but `internal/loomengine/preflight_integration_test.go` wires `{_lyx, .lyx, _pattern}` at :40 and its drift matrix (:384-394, :452) and `TestPreflight_LegacyWorktreeUpgrade` (:478-536) exist solely to prove classification for a *second, non-`_lyx`* junction — which an empty `pathspec` removes. **Fix:** State whether those loomengine cases re-target `_extra` or are deleted; the discussion currently implies neither.

### [GAP] Reserved-name arithmetic contradicts itself
**Section:** Testing → "Reserved-name un-reservation … six → four" **Issue:** The text says the set drops to four, then lists five names `{_lyx, .lyx, _board, _portals, _launchers}` — and `.lyx` was never in the six (`junctionnames_test.go:238` is `{_lyx,_pattern,_board,_portals,_launchers,_raddle}`); `.lyx` is reserved separately via `hubSlugReservedNames`/`structuralNeverCommittedDirs`. **Fix:** Fix the enumeration to `{_lyx, _board, _portals, _launchers}` and state `.lyx`'s reservation as an independent, unchanged fact.

### [GAP] Unlisted `_raddle` assertions inside `junctionnames_test.go`
**Section:** Testing → "Reserved-name un-reservation" **Issue:** Only `:73-77` and `:232-246` are named, but `junctionnames_test.go:180` (`{"raddle dir", "_raddle", true}`) and the r1-regression subtest at `:201-209` (asserting `_raddle` stays reserved for empty `junctionNames`) both pin the opposite of the new truth. **Fix:** Name both, and decide explicitly whether the r1 regression subtest is narrowed to `{_board,_portals,_launchers}` or deleted.

### [GAP] Empty `pathspec` spelling is undefined
**Section:** Decision → "`template.yaml`'s `pathspec` becomes empty" **Issue:** `template.yaml:2` is `pathspec: _pattern  # OPTIONAL …`; "empty" could be a bare `pathspec:` (null scalar, tag `!!null`) or `pathspec: ""`. `yamlengine.applyExistingOverrides` copies value, tag *and* style, and `configsync_test.go:480-481` pins the template default byte-exactly. **Fix:** Fix the exact literal to be written, including comment placement.

### [NOTE] `junctionnames.go:119` names `pattern.DirName` in prose
**Section:** Technical context → `internal/fabricengine` **Issue:** The comment-only list cites `junctionnames.go` lines 111 and 173 but not :119, whose `HubReservedNames` doc comment says the set "deliberately excludes lyxdirs.LyxDirName and pattern.DirName" — false once `DirName` is gone. **Fix:** Add :119 to the comment sweep.

### [NOTE] Design-doc counts disagree; `raddle.md` has no geometry to correct
**Section:** Decision → "Every `_raddle`-as-hub/junction reference is corrected" **Issue:** "three cross-referencing design docs" vs the Q&A's "all five affected design docs"; and `manifest/designs/raddle.md` contains no `_raddle`/junction token at all (its only geometry claim is "living in `weft`"), so this is authoring new content, not correcting. **Fix:** Fix the count against the actual set (`raddle`, `finalize`, `shed`, `loom`, `fabric-unified-view`) and say raddle.md gains a new geometry section.

## Verdict

GAPS_FOUND
Blast radius under-enumerated; reserved-name arithmetic and empty-pathspec spelling need fixing.
MILL_REVIEW_END
