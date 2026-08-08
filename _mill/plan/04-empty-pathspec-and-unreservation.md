# Batch: empty-pathspec-and-unreservation

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "empty-pathspec-and-unreservation"
number: 4
cards: 8
verify: go test -tags integration ./internal/fabricengine/ ./internal/configsync/ ./internal/configcli/ ./internal/lyxcwd/ ./cmd/lyx/
depends-on: [3]
```

## Batch Scope

This batch empties `template.yaml`'s `pathspec` default, drops `"_raddle"` from `HubReservedNames()`, and converges the reserved-name arithmetic that both changes move.
The two un-reservations are one batch and not two because they converge in the same tests: `junctionnames_test.go`'s "default pathspec union reserves exactly six names" case loses `_raddle` (because `HubReservedNames()` drops it) **and** `_pattern` (because an empty `pathspec` removes it from the `junctionNames` union `IsReservedHubName` folds in), so the set drops to exactly **four**: `{_lyx, _board, _portals, _launchers}`.
Splitting them would leave that one assertion half-correct in an intermediate commit.

`.lyx` is **not** part of that arithmetic.
It was never one of the six and it stays reserved independently via `hubSlugReservedNames()` and `structuralNeverCommittedDirs`, regardless of `pathspec`.
Do not fold `.lyx` into the count in either direction.

Batch-local decision: `TestDeployedLyxPathspec_YieldsNoDuplicateLyx` **keeps** its `Config{Pathspec: "_lyx _pattern"}` value, against the `_extra` substitution used everywhere else — see card 21.

## Cards

### Card 17: Empty `template.yaml`'s `pathspec` default

- **Context:**
  - `internal/fabricengine/config.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/fabricengine/template.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Line 2 of `internal/fabricengine/template.yaml` currently reads:

  ```
pathspec: _pattern  # OPTIONAL per-repo directory path(s) relative to worktree root, whitespace-separated; _lyx and .lyx are structural and injected in code by internal/fabricengine, never read from here
  ```

  The value becomes an explicit **double-quoted empty string** — `pathspec: ""` — never a bare `pathspec:`, which YAML parses as a null scalar with tag `!!null`.
  The trailing comment is retained verbatim, still on the same line, with the same two-space gap before `#`.
  Do not remove the `pathspec` key: `configengine.Load` reports a missing-key error and tells the operator to run `lyx config reconcile`, so removing it would break every deployed `fabric.yaml`.
  The empty-string-versus-null distinction matters because `yamlengine.applyExistingOverrides` copies an existing leaf's value, tag **and** style, so a tag difference would propagate into every deployed config's round-trip.
  `strings.Fields("")` returns nil either way, so runtime behaviour is identical — the choice is about keeping the YAML type stable as a string.
  Do not set the value to `_lyx`: the Durable-vs-Ephemeral State Invariant makes `_lyx`/`.lyx` structural and injected in code via `structuralCommittedDirs`/`structuralNeverCommittedDirs` precisely so no operator-editable config value can tear the durable tree away.
- **Commit:** `refactor(fabricengine): empty template.yaml's pathspec default`

### Card 18: Converge the config-plumbing assertions on the empty default

- **Context:**
  - `internal/fabricengine/template.yaml`
  - `internal/fabricengine/config.go`
  - `internal/yamlengine/reconcile.go`
- **Edits:**
  - `internal/configsync/configsync_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/fabricengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/configsync/configsync_test.go` lines 480-481 assert the template default by byte-exact substring (`contains(string(got), "pathspec: _pattern")`) and must assert the new empty default instead.
  **Take that assertion string from the actual round-tripped output, never from this plan.** The test matches the file as *written*, after a `yamlengine.Resolve` plus marshal round-trip, and `yaml.v3` may re-emit a double-quoted empty string as `''` or reflow the trailing comment.
  Apply card 17 first, run this test, read the real bytes out of the failure message, and copy those into the assertion — do not assume `pathspec: ""` survives verbatim.
  Update the failure message on line 481 so it names the empty default rather than `_pattern`.
  In `internal/configcli/configcli_integration_test.go` line 67 and `internal/fabricengine/template_test.go`, the `{"_lyx", "_pattern"}` expectations become whatever the empty default now yields — for the wired-name set that is `{_lyx, .lyx}`, and for the routing set `{_lyx}`.
  Add a case to `internal/fabricengine/template_test.go` proving an empty `pathspec` yields `Config.Dirs() == nil` and that `pathspecNames`/`junctionNames` degrade to the structural sets alone without panicking on the nil slice — that nil-slice path is new behaviour this batch creates and nothing currently pins it.
- **Commit:** `test(config): assert the empty pathspec default and its nil Dirs() degradation`

### Card 19: Drop `"_raddle"` from `HubReservedNames()` and correct its doc comments

- **Context:**
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/junctionnames_test.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `HubReservedNames()` at line 123 returns `[]string{BoardDirName, portalsDirName, launchersDirName, "_raddle"}`; drop the `"_raddle"` entry so it returns the three hub-structural tokens.
  Correct the three doc comments that enumerate or reference the set.
  Line 111 names "`_raddle`, `_board`, `_portals`, `_launchers`" — drop `_raddle`.
  Line 119 is the subtle one: it says the set "deliberately excludes `lyxdirs.LyxDirName` and `pattern.DirName`, which are config-migrated junction names folded into the reserved set by `IsReservedHubName`'s `junctionNames` parameter instead" — that is false in both halves once `pattern.DirName` is gone and `pathspec` is empty.
  Rewrite it to say the set excludes `lyxdirs.LyxDirName`, which is reserved via `structuralCommittedDirs` rather than via this set, and that the `junctionNames` parameter now folds in whatever a repo's `pathspec` names, which is nothing by default.
  Do not mention `pattern.DirName` — the const is deleted in batch 6 and the comment must not outlive it.
  Line 173's `filterHubReserved` doc comment lists "(`_board`, `_portals`, `_launchers`, `_raddle`)" — drop `_raddle`.
  Leave the `.lyx`-is-deliberately-not-a-member paragraph at lines 115-118 exactly as it is; that rationale is unchanged.
  Do not change `hubSlugReservedNames`, `IsReservedHubName`, `filterHubReserved`, or either structural set — only `HubReservedNames`' return value and the comments.
- **Commit:** `refactor(fabricengine): un-reserve _raddle as a hub-level name`

### Card 20: Recount the reserved-name arithmetic in `junctionnames_test.go`

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabricengine/junctionnames_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The shared `junctionNames` fixture at line 172 is currently `[]string{"_lyx", "_pattern"}` and becomes empty — `[]string{}` — modelling the post-change production call site, where `pathspec: ""` makes `Config.Dirs()` nil and `Topology.Add` passes nil through.
  This is safe for the injected-junction-name arm because the "junction-only name is reserved" sub-test at lines 213-220 supplies its own local `[]string{"_custom"}` fixture and does not read the shared one — verify that before editing.
  The sub-test at lines 232-246, "default pathspec union reserves exactly six names", must be renamed and recounted, not decremented: `wantReserved` at line 238 becomes exactly `[]string{"_lyx", "_board", "_portals", "_launchers"}` — **four** names.
  Two names leave, not one: `_raddle` because `HubReservedNames()` drops it, and `_pattern` because an empty `pathspec` removes it from the `junctionNames` union.
  Update the explanatory comment at lines 233-234 accordingly, and state in it which source proves each surviving name, since with an empty `junctionNames` they no longer share one: `_lyx` from `structuralCommittedDirs` (never the config arm); `_board`, `_portals`, `_launchers` from `hubSlugReservedNames()` via `HubReservedNames()`.
  Add a note that `.lyx` is reserved via `structuralNeverCommittedDirs` and `hubSlugReservedNames()` but was never in this list and still is not — the list is a positive "these are reserved" set, not an exhaustiveness assertion.
  The `{"raddle dir", "_raddle", true}` table row at line 180 flips to `false`.
  The r1-regression sub-test at lines 200-209, which loops over `{_board, _portals, _launchers, _raddle}`, is **narrowed to `{_board, _portals, _launchers}`, not deleted** — its point is that hub-structural tokens stay reserved even for an empty `junctionNames`, which is exactly the configuration this batch creates, making it more load-bearing than before.
  The loop at line 77 over `{"_board", "_portals", "_launchers", "_raddle"}` drops `_raddle`.
  The `filterHubReserved` table rows at lines 44-45 and 54-55 use `"_pattern"` purely as a stand-in for an ordinary non-reserved name; both already carry `"_extra"` alongside it, so substitute a **second distinct** name (`"_other"`) rather than producing a duplicate entry.
  Update the file header comment at line 19.
  Add a positive case proving a worktree slug named `_raddle` is now accepted by `IsReservedHubName` — that is the observable behaviour change and nothing else pins it.
- **Commit:** `test(fabricengine): recount the reserved-name union to four and un-reserve _raddle`

### Card 21: Converge the remaining reserved-name expectations across five test files

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/add_test.go`
  - `internal/fabricengine/config_test.go`
  - `internal/fabricengine/fabric_test.go`
  - `cmd/lyx/tierpurity_test.go`
  - `internal/fabricengine/hostjunction_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `structuraldirs_test.go`, rename `TestHubReservedNames_StillReturnsExactlyTheFourHubStructuralTokens` to name **three** tokens, and change `want` at line 103 to `[]string{BoardDirName, portalsDirName, launchersDirName}`.
  Update its doc comment at lines 99-101.
  Leave the `containsName(got, ".lyx")` assertion intact — `.lyx` must still never appear.
  `TestDeployedLyxPathspec_YieldsNoDuplicateLyx` at lines 32-45 **keeps** its `Config{Pathspec: "_lyx _pattern"}` value; do **not** retarget it to `_extra`.
  Per the no-migration decision, a `pathspec` naming `_pattern` is exactly what deployed repos keep indefinitely, making this the only test exercising a real deployed config value rather than a synthetic one.
  Add a comment to it saying the value deliberately models a stale deployed config this task no longer *produces* but does not migrate away, and cross-referencing the fresh-clone-only limitation documented in `internal/fabricengine/doc.go`.
  In `add_test.go`, the `{"RaddleDir", "_raddle"}` reserved-slug table row at line 130 must be removed from the rejected set and re-added as an **accepted** slug case, since `_raddle` is now legal.
  The `Config{Pathspec: "_lyx _pattern"}` at line 142 and `Config{Pathspec: "_lyx _pattern _extra"}` at line 166 stay mechanically valid but `_pattern` is now a misleading exemplar — retarget those to `"_lyx _extra"` and `"_lyx _extra _other"`.
  Rewrite the comment at lines 137-141: its claim that "`_lyx`/`_pattern` are rejected only via this injected pathspec" is no longer true for `_pattern`, and it must also drop `_raddle` from its list of names rejected by `HubReservedNames()`.
  In `config_test.go` lines 36-52 and `fabric_test.go` line 147, `_raddle` is used as an ordinary optional pathspec name; those cases stay valid (an un-reserved name is exactly what a pathspec may hold), but retarget them to `_extra` so no test implies `_raddle` has a special role.
  In `cmd/lyx/tierpurity_test.go`, remove the `"_raddle": true` entry at line 68 only if that map is a geometry-token set that must stay in sync with the reserved names — read the map's declaration and its doc comment first, and leave it untouched if it serves an unrelated purpose.
  In `internal/fabricengine/hostjunction_test.go`, finish the half of the `no_raddle_names` sub-test batch 3 deliberately left alone: batch 3 already retargeted its junction-name input at line 199, so what remains is the assertion body at lines 200-204 and the sub-test's own title.
  Its stated premise — that `HostJunctions` never yields a `_raddle` entry, "forbidden by design" — **inverts**: once `_raddle` is un-reserved, a `pathspec` entry naming it would legitimately wire a junction.
  Re-point the sub-test at a still-reserved name (`_board`, `_portals`, or `_launchers`), rename it accordingly, and update the scope-guard comment at line 191, rather than leaving a guard whose stated rationale is no longer true.
  Do not delete it: it is the only assertion that `HostJunctions` respects the hub-reserved block set at all.
  Converge this file's remaining `_raddle` expectations in the same pass.
- **Commit:** `test(fabricengine): converge structural, add, and config expectations on the un-reserved names`

### Card 22: Invert `reconcile_stale_removal_test.go` onto the empty pathspec

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabricengine/reconcile_stale_removal_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every `pattern.DirName` reference (lines 114, 271, 375) and every bare `"_pattern"` junction-name literal (lines 97, 154, 191, 206, 243, 294, 336, 371, plus the hand-written config contents) with `"_extra"`, renaming `patternLink` locals to `extraLink` and updating the header and inline comments at lines 42, 56, 70-71, 93-94, 112, 191, and 201.
  Line 206's config content `pathspec: _lyx _pattern _extra` would collapse to a duplicate — use `pathspec: _lyx _extra _other` and retarget the third-name assertions accordingly.
  Remove the `internal/pattern` import once unused.
  `TestReconcile_NeverRemovesReservedHubName` at lines 279-312 currently seeds `_raddle` on disk as its still-reserved exemplar; `_raddle` is no longer reserved, so re-point it at `_board` (or `_portals`/`_launchers`) and rename the `reservedLink` messages accordingly.
  Do **not** delete this test: it is the sole coverage of `scanOnDiskJunctionNames`' reserved-name exclusion, and it becomes *more* load-bearing once `pathspec` is empty, since every non-reserved on-disk link is then stale by definition.
  Update the header comment at line 10 which lists the permanently-excluded set to drop `_raddle`.
  Add one new test proving the post-change default directly: with the repo-wide `fabric.yaml` carrying the new empty `pathspec`, an on-disk optional junction is classified stale and removed by `Reconcile`, while the `_lyx` and `.lyx` junctions are not.
  That case is what pins the actual junction-teardown behaviour this task delivers.
  Do **not** write a test asserting teardown-on-upgrade for an already-deployed repo — `yamlengine.applyExistingOverrides` preserves an existing `pathspec` value, so a deployed repo keeps its old value and never sees the junction as stale; that upgrade path does not exist and must not be pinned.
- **Commit:** `test(fabricengine): pin stale-removal against the empty pathspec and a still-reserved name`

### Card 23: Split `weftgit_pathspec_integration_test.go`'s default-pathspec regression

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/template.yaml`
- **Edits:**
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `resolvedDefaultRoutingNames` at lines 231-243 deliberately resolves `template.yaml`'s **real** default rather than a literal, so `_extra` cannot be injected through it.
  With `pathspec: ""` the routing set becomes the single entry `[_lyx]`, so `TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern`'s guard at lines 262-265 loses its subject entirely.
  Split the test in two rather than losing either property.
  First, keep a real-default assertion: a test asserting `resolvedDefaultRoutingNames(t)` equals exactly `[]string{"_lyx"}`, which still guards against a future template change silently re-widening the default.
  Second, convert the multi-entry tolerance regression to a **hand-supplied** two-name routing set `{"_lyx", "_extra"}`, keeping both existing shapes it covers (the optional directory wholly absent, and present-but-empty) and both existing assertions that the `_lyx` change actually commits.
  Add a comment on the converted test stating explicitly that it no longer rides the real template default and why: `weftPathspecFilter` remains live production behaviour, and an empty default is exactly what would let its removal go unnoticed, so the coverage must be preserved by hand-supplying the wider set.
  Rename both tests off "WidenedDefaultPathspec"/"NoPattern" to names describing what each now pins.
  Update `resolvedDefaultRoutingNames`' own doc comment at lines 226-230, which currently states the resolved default is `_pattern`.
  Update the comment at line 77 that describes "the exact shape a first-ever `_pattern/PATTERN.md` commit needs".
- **Commit:** `test(fabricengine): split the default-pathspec guard from the pathspec-tolerance regression`

### Card 24: Invert the `_pattern` reserved-slug guard and add the two positive guards

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/raddle_guard_test.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestIsReservedHubName_Pattern` at lines 132-141 asserts the exact opposite of the new truth and must be **inverted and renamed**, not merely edited — it is the only test pinning `_pattern` as a reserved slug.
  Rename it to `TestIsReservedHubName_PatternNoLongerReserved` (or equivalent) and assert `fabricengine.IsReservedHubName("_pattern", nil) == false`.
  Pass `nil` rather than the old `[]string{"_lyx", "_pattern"}` fixture: with `pathspec` empty, nil is what the production call site now supplies, and injecting `_pattern` through `junctionNames` would make the assertion trivially reserved again and prove nothing.
  Rewrite the doc comment at lines 132-135, which currently claims `_pattern` sits in the reserved set alongside `_raddle`.
  Add a second test in the same file asserting `fabricengine.IsReservedHubName("_raddle", nil) == false`, with a doc comment stating that raddle has converged on an anchor-level `_lyx/raddle/` design with no hub-level presence, so the name is no longer reserved.
  This guard belongs here, in `package lyxcwd_test`, because this file already imports `internal/fabricengine` for exactly this kind of assertion.
  **Do not touch `internal/lyxcwd/raddle_guard_test.go`** — it is `package lyxcwd`, it is a tree-scan guard on an unrelated and still-valid invariant, and calling `fabricengine.IsReservedHubName` from it would close a `fabricengine -> lyxcwd` import cycle in the test binary.
  Leave its nine `_raddle` occurrences and its header prose alone, including the "a future nested/ignored `_raddle`" wording; that tension is recorded deliberately and belongs to the raddle implementation task.
- **Commit:** `test(lyxcwd): invert the _pattern reserved-slug guard and pin _raddle as un-reserved`

## Batch Tests

`verify:` runs the integration-tagged suites for `internal/fabricengine`, `internal/configsync`, `internal/configcli`, `internal/lyxcwd`, and `cmd/lyx`.
The unbounded per-package scope is justified for `internal/fabricengine`: this batch changes the template default that its whole config-driven junction surface reads, so a narrower `-run` filter would leave most of the affected behaviour unverified.
`-tags integration` is required by four edited files — `internal/configcli/configcli_integration_test.go`, `internal/fabricengine/reconcile_stale_removal_test.go`, `internal/fabricengine/weftgit_pathspec_integration_test.go`, and `internal/lyxcwd/lyxcwd_test.go` — and the tag is additive, so the same invocation also covers the untagged files this batch edits.

Card 18 has an explicit ordering dependency that the implementer must respect: apply card 17's template change **first**, run `go test ./internal/configsync/`, and copy the real round-tripped bytes out of the failure into the assertion.
Guessing the assertion string is the one way this card fails silently.

Card 22's new empty-pathspec staleness case and card 24's two positive guards are the only new coverage this batch adds; everything else is convergence of existing assertions.

After this batch, `grep -rn 'pattern\.DirName' internal/ cmd/` must report only `internal/pattern/pattern.go` and `internal/pattern/patternpath_test.go` — batch 6 clears those.
