# Batch: junction-test-retarget

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "junction-test-retarget"
number: 3
cards: 8
verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./internal/loomengine/
depends-on: [2]
```

## Batch Scope

This batch retargets every test that used `_pattern` merely as "an ordinary optional, config-driven junction name" onto `_extra`, the substitute name `config_driven_junctions_integration_test.go` and `junctionnames_test.go` already use for that role.
It is **test-only** — no production file changes — and it is what makes batch 6's `pattern.DirName` deletion a small, safe edit instead of a fifteen-file compile break.

The retarget is deliberately independent of the `template.yaml` change in batch 4: every test touched here passes an explicit junction-name slice to `WireJunctions`/`HostJunctions`/`classifyPaths` and never reads the template default, so the substitution is correct both before and after `pathspec` empties.

No test is deleted.
The generic multi-junction path must stay covered by a name that is not the one being removed — deleting these cases would silently drop coverage of config-driven junction wiring, repoint, removal, rollback, and drift classification.

Batch-local decision: files whose **names** reference `_pattern` (notably `internal/fabricengine/junction_pattern_integration_test.go`) keep those names.
Only header comments, test-function names, and sub-test names that assert something about `_pattern` specifically are reworded.
Other files' comments that merely cite `junction_pattern_integration_test.go` as a cross-reference by filename are correct as-is and must not be edited.

## Cards

### Card 9: Retarget `hostjunction_test.go` — the densest consumer

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabricengine/hostjunction_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every `pattern.DirName` occurrence used as a junction name with the literal `"_extra"`, at lines 121, 128, 135, 149, 156, 199, 217, 236-239, 252, 268-269, 285, 308, and 309.
  Line 149's slice is `[]string{"_lyx", pattern.DirName, "_extra"}` and line 308's is the same shape — those would collapse to a duplicate `_extra`, so give the third entry a second distinct non-reserved name (`"_other"`) rather than producing a duplicated entry.
  Rename the local variables and helper identifiers that encode the old name — `wantPatternLink`, `wantPatternTarget`, `patternJunction` — to `_extra`-neutral spellings such as `wantExtraLink`, `wantExtraTarget`, `extraJunction`.
  Update the file header comment at lines 5-6 and the comment at line 107, which both describe "every `_pattern` row" asserting against the generic config-driven junction join; the property they state is still exactly right, only the exemplar name changes.
  Remove the `internal/pattern` import if `pattern` becomes unused in the file.
  Line 199 sits inside the `no_raddle_names` sub-test (lines 192-205) and is retargeted here like every other junction-name input: the sub-test's *input* slice is an ordinary two-name junction set and carries no `_raddle` claim.
  What is deferred to batch 4 is only that sub-test's `_raddle` **assertion** and its name — the loop body at lines 200-204 asserting `HostJunctions` never yields a `_raddle` entry "forbidden by design", and the sub-test's own title.
  Leave those alone in this batch; retarget line 199 now.
- **Commit:** `test(fabricengine): retarget hostjunction junction-name rows to _extra`

### Card 10: Retarget the junction repoint, removal, and rollback integration tests

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, replace every `pattern.DirName` reference and every bare `"_pattern"` junction-name literal with `"_extra"`, and rename the tests and identifiers whose names encode the old exemplar.
  In `junction_repoint_test.go`, `TestWireJunctions_RepointsWrongTargetJunction_Pattern` and `TestWireJunctions_RepointsDanglingJunction_Pattern` become `..._Extra`, and their doc comments at lines 88-90 and 185-187, plus the file header comment at line 16, are reworded to say the non-`_lyx` junction rather than the `_pattern` junction.
  The property each pins — that repoint works for a junction that is not `_lyx` — is unchanged and must survive verbatim.
  In `remove_junctions_integration_test.go`, retarget lines 70, 78, 80, 97 and reword the header comment at lines 10-11 and the inline comment at line 86 that cites `"_lyx _pattern"` as the default pathspec; that comment is factually about the deployed default, so state the second junction generically instead of naming the template value.
  In `add_rollback_adopt_test.go`, the two table rows at lines 159 and 228 carry `"_pattern"` as their case label and `pattern.DirName` in their path joins — retarget both, and update the doc comment at line 139 that names "the repo-wide default junctions (`_lyx` and `_pattern`)".
  In `dotlyxjunction_integration_test.go`, the table case `{name: "Pattern", dirName: pattern.DirName}` at line 301 becomes `{name: "Extra", dirName: "_extra"}`, line 314's three-name slice takes `"_extra"` in place of `pattern.DirName`, and the header comment at lines 10-12 plus the comment at line 292 are reworded — the hard-refusal guard they describe applies to any adopted junction target, not to `_pattern` specifically.
  Remove the `internal/pattern` import from any file where it becomes unused.
  The cross-reference at line 18 of `dotlyxjunction_integration_test.go` naming the file `junction_pattern_integration_test.go` is a filename citation and stays as-is.
- **Commit:** `test(fabricengine): retarget repoint, removal, rollback and adopt junction cases to _extra`

### Card 11: Retarget the bare-literal `_pattern` junction wirings

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These sites carry no `pattern.DirName` reference at all — they are bare string literals, so nothing but this card catches them.
  In `checkout_index_refresh_test.go` line 40, `checkout_rollback_test.go` lines 44 and 100, and `reconcile_stale_registration_test.go` line 468, change each `WireJunctions(l, slug, []string{"_lyx", "_pattern"})` call to `[]string{"_lyx", "_extra"}`.
  In `commit_integration_test.go` line 61, the hand-written config file content `"branch_prefix: \"\"\npathspec: _lyx _pattern\n"` becomes `pathspec: _lyx _extra`.
  In `config_driven_junctions_integration_test.go` line 45, the three-name slice `[]string{"_lyx", "_pattern", "_extra"}` would collapse to a duplicate — replace `"_pattern"` with a second distinct non-reserved name (`"_other"`) so the test keeps exercising three distinct junctions, and update the header comment at line 33 which describes the set as "a three-name set including `_extra`".
  The comment at line 13 and line 98 of that file cite `junction_pattern_integration_test.go` by filename and stay as-is.
  Do not add or remove any import in these files unless one becomes unused.
- **Commit:** `test(fabricengine): retarget bare-literal _pattern junction wirings to _extra`

### Card 12: Retarget `classify_test.go`'s weft-routing tables

- **Context:**
  - `internal/fabricengine/classify.go`
- **Edits:**
  - `internal/fabricengine/classify_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Every `routingNames` slice naming `"_pattern"` (lines 27, 35, 43, 93, 101, 109, 139, 172) takes `"_extra"` instead, and every routed path `"_pattern/PATTERN.md"` (lines 36, 38, 94, 96, 102, 104, 178) becomes `"_extra/notes.md"`.
  Rename the sub-test `under_pattern_is_weft` at line 33 to `under_extra_is_weft`.
  Do not substitute `_lyx/PATTERN.md` here: the point of these cases is that a **config-named optional** routing directory routes to weft, and `_lyx` already has its own dedicated cases at lines 25-31 and 93-96, so reusing `_lyx` would delete a distinct case rather than move it.
  The mixed-input partition test at lines 170-182 must keep exactly the same number of input paths and the same three-way partition shape — only the optional-directory name and its file change.
- **Commit:** `test(fabricengine): retarget classifyPaths optional-routing cases to _extra`

### Card 13: Retarget `internal/fabriccli`'s prime-junction and narrow-pathspec guards

- **Context:**
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At line 464, the loop `for _, name := range []string{lyxdirs.LyxDirName, pattern.DirName}` becomes `[]string{lyxdirs.LyxDirName, "_extra"}`, and the comment at line 462 naming "the prime host worktree's `_lyx`/`_pattern` junctions" is reworded.
  Do **not** substitute `"_extra"` here and do not seed a config: this test clones from a genuinely empty bare weft (zero commits, via `makeCLICloneWeftBare`), so there is no pre-existing `weft:main` `fabric.yaml` to seed before `clone` runs, and after batch 4's empty default a bare clone wires no optional junction at all.
  Use `[]string{lyxdirs.LyxDirName, lyxdirs.DotLyxDirName}` instead.
  That keeps the check's real content — the prime worktree's junctions are wired, more than one of them — and it is true both before and after batch 4, because `.lyx` is structural (`structuralNeverCommittedDirs`) and is wired by every clone regardless of `pathspec`.
  Do not weaken the loop to `_lyx` alone.
  The narrow-pathspec guard at lines 283-326 writes a repo-wide `fabric.yaml` containing `pathspec: _pattern` and asserts the sync-built pathspec still covers `_lyx` — retarget that literal to `_extra` and update the two comments at lines 283 and 291 plus the failure message at line 326.
  That guard's subject is the routing set never falling back to a raw unfiltered `Config.Dirs()`, which is unrelated to which optional name the config happens to hold.
  Remove the `internal/pattern` import if it becomes unused.
- **Commit:** `test(fabriccli): retarget prime-junction and narrow-pathspec guards to _extra`

### Card 14: Retarget `internal/loomengine`'s preflight second-junction coverage

- **Context:**
  - `internal/loomengine/preflight.go`
  - `internal/fabricengine/drift.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These two tests exist **solely** to prove `Healthy`/`Preflight` classification is not accidentally `_lyx`-specific.
  Neither may be deleted: doing so silently drops the only coverage of the second-junction path.
  At line 40, the fixture wires `[]string{"_lyx", lyxdirs.DotLyxDirName, "_pattern"}` — retarget the third entry to `"_extra"`, and update the comment at line 89.
  In `TestPreflight_JunctionBroken`, retarget the `pattern.DirName` join at line 452 to `"_extra"` and reword the doc comment at lines 384 and 392.
  The documented `check3BlocksSeed` asymmetry — a broken `_lyx` junction also fails the seed stat because `LoomStatusFile(l)` is `_lyx`-anchored, while a broken second junction does not — survives verbatim; only the second junction's name changes.
  `TestPreflight_LegacyWorktreeUpgrade` must be **renamed off "Legacy"** — to `TestPreflight_MissingOptionalJunctionIsAJunctionFault` or an equivalently descriptive name — because the pre-`_pattern` upgrade narrative dies with `_pattern`, while the mechanism it pins stays live and is otherwise untested: a missing *optional* junction is classified `CheckJunction` (never `CheckFabricSync`), blocks the run, does not fail the seed check, and is repaired by one `Reconcile`.
  Retarget lines 498 and 535 and rewrite the doc comment at lines 478-480 and the inline comment at line 495 so they describe a worktree missing its optional junction rather than a worktree wired before `_pattern` existed.
  Rename the `patternLink` local to `extraLink`.
  Remove the `internal/pattern` import if it becomes unused.
- **Commit:** `test(loomengine): retarget preflight second-junction coverage to _extra`

### Card 15: Retarget `junction_pattern_integration_test.go`'s junction cases

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is the densest single file in the batch.
  Retarget every junction-name use — every `pattern.DirName` reference and every bare `"_pattern"` inside a `WireJunctions`/`UnwireJunctions` name slice — to `"_extra"`, at lines 85, 105, 131, 140, 182, 188-189, 192, 197, 208, 215-216, 235, 316, 389, 391, 394, 417, 442, 446, 456, 465-468, 495, 500-504, 514, 523-526, 551-552, 564-568, 581-587, 597-603, and 614.
  Rename the tests whose names encode the old exemplar: `TestReconcile_RepairsPatternOnlyDrift` becomes `TestReconcile_RepairsOptionalJunctionOnlyDrift`, `TestStatus_ReportsPatternJunctionUnhealthy` becomes `TestStatus_ReportsOptionalJunctionUnhealthy`, and `TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth` keeps its shape but its doc comment at lines 564-566 must stop describing "the shape every pre-card-15 worktree is in" — that upgrade narrative dies with `_pattern`, while the mechanism it pins (a `WireJunctions` call adds only the missing optional junction and leaves a healthy `_lyx` alone) stays live.
  Rename `patternLink` locals to `extraLink`, and reword the file header comment at lines 3-6 and the inline comments at lines 131, 167, and 316.
  **Leave `TestDetectHostPollution_PatternTrackedAsRestorable` at lines 258-311 entirely alone in this batch** — its `pattern.DirName` uses, its `wantPath` const, and its `found.ReportOnly` assertion are all batch 5's subject, and splitting that test's edit across two batches would leave it in an inconsistent state.
  Keep the `internal/pattern` import for that reason; batch 5 removes it.
  The file keeps its name — see the batch scope's no-rename decision.
- **Commit:** `test(fabricengine): retarget the per-junction generalisation cases to _extra`

### Card 16: Retarget `unwire_test.go`

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/fabricengine/unwire_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Line 59 wires `[]string{"_lyx", ".lyx", "_pattern", "_extra"}` — dropping `"_pattern"` here would collapse the four-junction case to three, so replace it with a second distinct non-reserved name (`"_other"`) and update the `want` slices at lines 70 and 76 to match, replacing `pattern.DirName` with that name and keeping the slices' existing sort order.
  Retarget the two-name and three-name wirings at lines 104 and 246 to `"_extra"`.
  `TestUnwire_PreservesWeftLyxAndPattern` becomes `TestUnwire_PreservesWeftLyxAndOptionalContent`: its `weftPatternDir` local at line 119 retargets to the optional junction name, and its doc comment at lines 84-88 plus the inline comment at line 158 are reworded.
  The property it pins is load-bearing and must survive verbatim — Unwire never deletes weft-side content — and after this task `_lyx/PATTERN.md` is exactly the hand-authored content that preservation protects, so say so in the reworded comment.
  Update the file header comment at line 7.
  Remove the `internal/pattern` import once unused.
- **Commit:** `test(fabricengine): retarget unwire junction and preservation cases to _extra`

## Batch Tests

`verify:` runs the full integration-tagged suites for `internal/fabricengine`, `internal/fabriccli`, and `internal/loomengine`.
This batch deliberately uses the unbounded per-package scope rather than a `-run` filter: it edits thirteen files in `internal/fabricengine` alone, spanning junction wiring, repoint, removal, rollback, adoption, checkout, commit routing, and path classification, so any narrower filter would leave most of the edited surface unverified.
`-tags integration` is required because eleven of the sixteen edited files carry `//go:build integration`; the tag is additive, so the same invocation also runs the untagged files (`hostjunction_test.go`, `classify_test.go`) in those packages.

After this batch, `grep -rn 'pattern\.DirName' internal/fabricengine internal/fabriccli internal/loomengine` must report exactly two files: `internal/fabricengine/reconcile_stale_removal_test.go` and `internal/fabricengine/junction_pattern_integration_test.go`.
The first is held back to batch 4, because its `_pattern` retarget and its `_raddle` reserved-name retarget are the same edit and both belong with the pathspec-staleness work.
The second retains only `TestDetectHostPollution_PatternTrackedAsRestorable`'s uses, which batch 5 clears together with the `ReportOnly` deletion.
The implementer should run this grep as a closing check before committing card 16; any other file it reports is a miss in this batch.

The module-wide `go vet -tags integration ./...` from the overview catches any import left dangling by a removed `internal/pattern` reference.
