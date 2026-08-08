# Batch: geometry-token-retirement

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "geometry-token-retirement"
number: 6
cards: 3
verify: go test ./... && go test -tags integration ./...
depends-on: [5]
```

## Batch Scope

This batch retires the `_pattern` and `_raddle` geometry tokens from the repo: it deletes `pattern.DirName` and `pattern.Dir()`, drops both tokens from `TestEnforcement_GeometryLiterals`' `geometryToken` switch and its `geometryTokenOwners` map, and closes the loop with a repo-wide grep sweep.

It is the plan's proof batch.
Batches 1-5 removed every consumer; this batch removes the declarations and the enforcement rows that sanctioned them, so any surviving path-construction literal in production code now fails the build rather than passing under a stale ownership row.

The `geometryToken` switch and the `geometryTokenOwners` map must change together — dropping a token from one but not the other leaves them disagreeing, and the map row would then sanction a token the switch no longer polices, or the switch would police a token with no registered owner and fail on every occurrence.

Batch-local decision: the enforcement scan excludes `*_test.go` by design, so test-side occurrences are a **review obligation**, not machine-caught.
Card 30's grep sweep is what covers them, and its expected-residue list is what distinguishes a deliberate survivor from a miss.

## Cards

### Card 28: Delete `pattern.DirName` and `pattern.Dir()`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/pattern/doc.go`
- **Edits:**
  - `internal/pattern/pattern.go`
  - `internal/pattern/patternpath_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `DirName` const at lines 14-18 of `internal/pattern/pattern.go` and the `Dir(baseDir string) string` function at lines 20-23, together with their doc comments.
  `File` already builds from `lyxdirs.LyxDirName` after batch 1 and must not change here; confirm it no longer calls `Dir` before deleting.
  `FileHere` is unchanged.
  In `internal/pattern/patternpath_test.go`, delete the `pattern.Dir` table and its test function — the accessor is gone, so the table has no subject.
  Do not fold its cases into the `File` table; `File`'s own table already covers the same base-dir shapes.
  Keep every `File`, `FileHere`, `PathspecFile`, and `PathspecDir` case.
  After this card the token `_pattern` must not appear anywhere in `internal/pattern/`.
- **Commit:** `refactor(pattern): delete the DirName const and Dir accessor`

### Card 29: Drop the `_pattern` and `_raddle` rows from the geometry-literal enforcement

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/status.go`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `geometryToken` closure at lines 252-258, remove `"_raddle"` and `"_pattern"` from the `case` list, leaving `"_board", "-weft", "-HUB", "_portals", "_launchers", "_lyx", ".lyx"`.
  In the `geometryTokenOwners` map at lines 267-300, delete the `"_raddle": {"internal/fabricengine"}` row (line 283) and the `"_pattern": {"internal/pattern", "internal/fabricengine"}` row (line 299) outright.
  The two comment blocks are **not** treated the same way, because they do not have the same scope.
  Lines 293-298 are `_pattern`-exclusive and are deleted outright along with their row.
  Lines 277-280 are one shared block covering `_portals`, `_launchers`, **and** `_raddle` together, feeding all three rows at 281-283 — so it is **rewritten, not deleted**: drop only its `_raddle` clause and keep the `_portals`/`_launchers` rationale intact.
  Both edits must land in the same card: dropping a token from the switch but leaving its map row, or the reverse, leaves the two disagreeing.
  Add a short comment recording why the two tokens left — `_pattern` because the PATTERN surface now lives inside `_lyx`, `_raddle` because raddle converged on an anchor-level `_lyx/raddle/` design with no hub-level presence — so a future reader does not re-add them on the assumption they were dropped by accident.
  Do not change the `TestEnforcement_FabricVocabulary` owner set or any other test in this file.
- **Commit:** `test(lyxcwd): retire the _pattern and _raddle geometry tokens from the enforcement map`

### Card 30: Repo-wide sweep and residue confirmation

- **Context:**
  - `internal/lyxcwd/raddle_guard_test.go`
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/loomengine/coherence.go`
  - `README.md`
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `grep -rn '_pattern\|_raddle' internal/ cmd/ docs/ manifest/ tools/ README.md CLAUDE.md CONSTRAINTS.md` and confirm every reported line is a **deliberate** survivor or a docs-only line that batch 7 owns.
  Do not edit anything in this card; its output is the input to batch 7 and the evidence that batches 1-5 were complete.
  The deliberate survivors, which must all still be present:

  - `internal/lyxcwd/raddle_guard_test.go` — nine `_raddle` occurrences, all intentional.
    The file is exempt from every edit in this task: it is `package lyxcwd`, it is a tree-scan guard on an unrelated and still-valid invariant, and calling `fabricengine.IsReservedHubName` from it would close a `fabricengine -> lyxcwd` import cycle in the test binary.
  - `internal/fabricengine/structuraldirs_test.go` — the two `_pattern` occurrences in `TestDeployedLyxPathspec_YieldsNoDuplicateLyx`, retained deliberately as this repo's only test exercising a real deployed config value.

  Note that `internal/loomengine/coherence.go`'s `"raddle"` phase-name enum value is explicitly out of scope and is bare `raddle`, not `_raddle`, so it will not match this grep at all — it is listed here only so nobody "fixes" it on sight.

  Everything else the grep reports must be either a docs/comment line batch 7 owns (`docs/`, `manifest/`, `tools/`, `README.md`, `CLAUDE.md`, `CONSTRAINTS.md`, plus the Go comment sites in `internal/fabricengine/{doc,junction,unwire,reconcile,weftwiring,cleanup}.go` and `internal/fabriccli/{fabric,weft_verbs}.go`) or a genuine miss.
  A genuine miss in `internal/` or `cmd/` Go code is fixed in this card's own batch, not deferred — if one is found, extend card 28 or card 29 rather than leaving it.
  Record the grep's full output in the batch's implementation report so batch 7 has an explicit worklist.
- **Commit:** none

## Batch Tests

`verify:` is `go test ./... && go test -tags integration ./...` — the full repo-wide sweep, both tiers.
The unbounded scope is justified and is the point of this batch rather than an oversight: deleting an exported const and an exported function from `internal/pattern` is a compile break reachable from any package in the tree, and dropping two rows from `geometryTokenOwners` turns any surviving production literal into a hard enforcement failure.
No narrower scope can prove either.
This command is identical to the configured `pipeline.done_gate`, so running it here means the done gate has effectively already passed by the time batch 7's docs work begins.

Card 30 runs no test of its own — it is a zero-diff verification card whose sole output is the grep worklist.
Its `Commit:` is `none` and its `Edits:`/`Creates:`/`Deletes:`/`Moves:` are all `none`, consistent with the zero-diff convention.

`TestEnforcement_GeometryLiterals` in `internal/lyxcwd/enforcement_test.go` is the machine-enforced half of this batch's proof; `TestLeafInvariant_AllowlistOnly` in `internal/pattern` re-confirms the widened allowlist still holds after the const deletion.
