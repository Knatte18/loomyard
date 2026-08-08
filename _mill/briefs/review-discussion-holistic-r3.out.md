MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Second-junction re-target list omits four test files
**Section:** Testing → "Junction teardown" **Issue:** Four files hardcode `_pattern` as the second junction/pathspec and appear in no list: `checkout_index_refresh_test.go:40`, `checkout_rollback_test.go:44,100`, `reconcile_stale_registration_test.go:468` (all `WireJunctions(l, slug, []string{"_lyx", "_pattern"})`) and `commit_integration_test.go:61` (`pathspec: _lyx _pattern`); they are literal-string uses, so the "authoritative" `DirName` compile-break list correctly excludes them but nothing else picks them up. **Fix:** Add them to the `_extra` substitution list so the plan's card scopes them explicitly.

### [GAP] `_raddle`-as-reserved-exemplar test case unaddressed
**Section:** Testing → "Reserved-name un-reservation" **Issue:** `reconcile_stale_removal_test.go:279-312` uses `_raddle` as the exemplar hub-reserved name proving a reserved on-disk link is never swept by `applyStaleRemoval`; the discussion names that file only as the densest `_pattern` file to invert, and its `_raddle` list omits it entirely. **Fix:** State that this case is re-targeted to a still-reserved name (`_board`/`_portals`/`_launchers`), not deleted — it is the only coverage of the sweep exclusion.

### [GAP] Nobody owns the `"PATTERN.md"` / `"pattern"` spelling after `DirName` dies
**Section:** Decisions → "`internal/pattern` drops `DirName` and `Dir()`" + "`PatternResidue` is re-scoped" **Issue:** `internal/pattern.File` builds `<base>/_lyx/PATTERN.md` while `fabricengine` independently builds `lyxdirs.LyxDirName + "/PATTERN.md"` and `+ "/pattern"`, so the cross-module duplication that `patternDirName` (`pull.go:26`) represents today is re-created under new spellings, with no enforcement test covering it (as the discussion itself notes). **Fix:** Decide explicitly — export relative-path constants from `internal/pattern` for `fabricengine` to consume (no cycle: `fabricengine` production does not import `pattern` today), or record accepting the duplication with rationale.

### [GAP] Sandbox suite steps need a decided replacement, not a reword
**Section:** Technical context → "Doc surfaces" **Issue:** `SANDBOX-FABRIC-SUITE.md:185-186` are executable scenario steps asserting unwire removes a *second* junction and preserves weft-side `_pattern`, and `:205` asserts pull JSON reports `_pattern/`-touching residue; with the default `pathspec` empty the sandbox has no second junction at all, so these cannot be token-substituted. **Fix:** Decide whether the unwire scenario drops the second-junction assertion or the suite seeds an `_extra` pathspec entry, and state it in scope.

### [NOTE] `hostjunction_test.go`'s `no_raddle_names` guard premise inverts
**Section:** Testing → "Reserved-name un-reservation" **Issue:** `hostjunction_test.go:191-202` asserts `HostJunctions` never yields a `_raddle` entry "forbidden by design"; once `_raddle` is un-reserved a pathspec entry would legitimately wire it, so "converged" leaves the outcome unspecified. **Fix:** Say whether the subtest is deleted or re-pointed at a still-reserved name.

### [NOTE] `pull.go` itself needs the `lyxdirs` import added
**Section:** Decisions → "How the new pathspec strings are built" **Issue:** The claim that `fabricengine` already imports `lyxdirs` is true at package level (`status.go:25`) but `pull.go:11-19` imports only `gitexec` and `lock`. **Fix:** Note the file-level import addition so it is not read as a no-op.

## Verdict

GAPS_FOUND
Four scoping gaps: two unlisted test surfaces, an unowned path token, and undecided sandbox steps.
MILL_REVIEW_END
