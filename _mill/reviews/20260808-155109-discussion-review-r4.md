MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] `raddle_guard_test.go` repurposing is infeasible as written
**Section:** `_raddle` un-reservation keeps a positive guard test
**Issue:** The file is `package lyxcwd` (internal test) and is a source-scan guard that no production file in `internal/lyxcwd` contains `_raddle` — not a reserved-name assertion; making it call `fabricengine.IsReservedHubName` would import `fabricengine`, which imports `lyxcwd`, an illegal test-binary import cycle, and would delete a still-valid unrelated invariant.
**Fix:** State that the existing tree-scan guard is retained as-is, and put any positive "not reserved" assertion in the external `lyxcwd_test` package (e.g. alongside the inverted `TestIsReservedHubName_Pattern`) instead.

### [GAP] `weftgit_pathspec_integration_test.go` cannot take the `_extra` substitution
**Section:** Testing — Junction teardown
**Issue:** That file's `resolvedDefaultRoutingNames` (`:231-243`) resolves `template.yaml`'s **real** default and `TestCommitWeft_...NoPattern` (`:261-265`) hard-fails unless it yields exactly `[_lyx _pattern]`; with `pathspec: ""` the routing set becomes single-entry `[_lyx]`, so the multi-entry pathspec-tolerance regression (`git add -- _lyx _pattern` failing wholesale when `_pattern` matches nothing) loses its subject and `_extra` cannot be injected through the template.
**Fix:** Decide explicitly whether that test hand-supplies a second routing name (dropping its "REAL default, not a literal" property) or is re-scoped, and say so rather than folding the file into the generic `_extra` list.

### [GAP] The bare-literal `_pattern` inventory is incomplete and partly misfiled
**Section:** Technical context — `pattern.DirName`'s blast radius / "Four more files hardcode `_pattern`"
**Issue:** `internal/fabricengine/add_test.go:142,166` (`Config{Pathspec: "_lyx _pattern"}` / `"_lyx _pattern _extra"`) appears in neither list — it is cited only for its `_raddle` expectations; and `internal/loomengine/plan_test.go:127` is listed as a `pattern.DirName` compile break when it is a bare `"_pattern"` literal, so the "compile break in every file below" framing is wrong for it.
**Fix:** Move `plan_test.go:127` into the bare-literal set, add `add_test.go:142,166` to it (five files, not four), and state whether add_test's hand-supplied pathspec stays valid post-change.

### [NOTE] `ReportOnly` has a third convergence site (a reader, not a writer)
**Section:** `PollutionEntry.ReportOnly` is deleted
**Issue:** `internal/fabricengine/junction_pattern_integration_test.go:302-303` reads `found.ReportOnly`; the discussion enumerates only the two writers, so the compile break there is unnamed.
**Fix:** Add that read site to the ReportOnly convergence list.

### [NOTE] The byte-exact `pathspec: ""` spelling is asserted, not verified
**Section:** `template.yaml`'s `pathspec` becomes empty — "The exact literal to write"
**Issue:** `configsync_test.go:480-481` matches the *written* file after a `yamlengine.Resolve` + marshal round-trip, and the discussion asserts double-quoted `""` will survive that round-trip without evidence; yaml.v3 may re-emit `''`.
**Fix:** Note that the new assertion string must be taken from the actual round-tripped output, not assumed.

### [NOTE] Scope list omits the new exported PATTERN path constants
**Section:** Scope / `internal/pattern` owns the PATTERN path spellings
**Issue:** Scope still says only "drop `DirName` and `Dir()`"; the r3 decision adds exported `PathspecFile`/`PathspecDir` and a new production `fabricengine → pattern` import, neither reflected in Scope.
**Fix:** Add both to the In-scope bullets, and say whether `File()` and `PathspecFile` share a single `PATTERN.md` segment const.

## Verdict

GAPS_FOUND
Guard-test repurposing is cycle-illegal; two test-inventory holes remain.
MILL_REVIEW_END
