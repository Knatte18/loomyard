MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: claude-opus-5 (runtime-reported; best-effort self-assessment)
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Bare-literal `_pattern` inventory is not exhaustive
**Section:** Testing — "Six files hardcode `_pattern` as a bare string literal"
**Issue:** The list is presented as closed ("nothing else would catch them"), but `internal/fabricengine/junctionnames_test.go` carries bare `_pattern` literals at `:44-45`, `:54-55` (the `filterHubReserved` table) and `:172` (`junctionNames := []string{"_lyx", "_pattern"}`), and `internal/fabricengine/structuraldirs_test.go:32,37` carries `Config{Pathspec: "_lyx _pattern"}` — none of these appear in the compile-break list, the six-file bare-literal list, or the junction-teardown file list (which names only `junction_pattern`, `junction_repoint`, `classify`, `config_driven`, `remove_junctions`, `unwire`).
**Fix:** Extend the bare-literal enumeration to `junctionnames_test.go:19,44-45,54-55,172` and `structuraldirs_test.go:32,37`, or state explicitly that the enumeration is indicative and a `grep -n '"_pattern"'` sweep over `internal/fabricengine/*_test.go` is the authoritative closing step.

### [GAP] `junctionnames_test.go:172`'s fixture is unresolved but the six→four arithmetic depends on it
**Section:** Testing — "Reserved-name un-reservation — both tokens, and the arithmetic is six → four"
**Issue:** The recount to `{_lyx, _board, _portals, _launchers}` is stated for `:232-246`, but the whole `TestIsReservedHubName` table (including the `:180` row and the `:232-246` sub-test) reads the shared `junctionNames` fixture at `:172`, whose value is `{"_lyx", "_pattern"}`; the discussion never says what that fixture becomes, and `_lyx` stays reserved via `structuralCommittedDirs` regardless of it.
**Fix:** Name the new fixture value explicitly (e.g. `[]string{"_lyx"}` to model the post-change default, or `{"_lyx", "_extra"}` to keep an injected-junction-name case live) and say which reservation source each of the four names is proved by.

### [GAP] `TestDeployedLyxPathspec_YieldsNoDuplicateLyx`'s intent post-change is undecided
**Section:** Testing / "No migration"
**Issue:** `structuraldirs_test.go:32-37` asserts dedup for a *deployed* `pathspec: "_lyx _pattern"` — which the "No migration" decision says is exactly what deployed repos keep forever — so it is ambiguous whether this test re-targets to `_extra` like the other bare literals, or deliberately keeps `_pattern` as the documented stale-deployed reality.
**Fix:** State the choice and its reason in the same place the fresh-clone-only limitation is recorded, so the plan does not silently erase the only test exercising a real deployed pathspec value.

### [NOTE] Closing `grep` will hit `raddle_guard_test.go`'s nine deliberate occurrences
**Section:** Testing — "Repo-wide proof"
**Issue:** The closing `grep -rn '_pattern\|_raddle'` sweep is described as the completeness check, but `internal/lyxcwd/raddle_guard_test.go` holds nine `_raddle` occurrences by design and the r4 decision says leave it entirely untouched.
**Fix:** Name that file (and any other intentional survivor) as expected residue in the closing-check description.

### [NOTE] `raddle_guard_test.go`'s doc comment describes geometry the task deletes
**Section:** "Every `_raddle`-as-hub/junction reference is corrected"
**Issue:** The guard's file header (`:1-4`) frames it as protecting against "a future nested/ignored `_raddle`" directory — a directory that will not exist under the settled `_lyx/raddle/` design — which sits in tension with the sweep-every-reference decision while the file is exempted from all edits.
**Fix:** Say explicitly that the exemption covers the file's prose too, and why the mechanical guard stays valid regardless of where raddle content lands.

## Verdict

GAPS_FOUND
Test-side `_pattern` inventory incomplete; two fixture-intent decisions unresolved before planning.
MILL_REVIEW_END
