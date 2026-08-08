# Batch: pollution-scan-and-reportonly

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "pollution-scan-and-reportonly"
number: 5
cards: 3
verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
depends-on: [4]
```

## Batch Scope

This batch re-scopes fabric's host-pollution scan from `{_lyx, _pattern, _raddle}` to `_lyx` alone, and deletes `PollutionEntry.ReportOnly` along with **both** its writers.
The two changes are one batch because deleting the `_raddle` classification branch is what leaves the `ReportOnly` field with a single synthetic writer, and removing that writer is what makes the field dead.

There are **three** convergence sites, not one: the `_raddle` classification branch, the synthetic scan-error entry, and one **reader** in `junction_pattern_integration_test.go` that asserts `!found.ReportOnly`.
That read is a compile break when the field is deleted; its intent (a `_lyx` match carries an automated remedy) is already preserved by the adjacent `found.Remedy == ""` assertion, so the `ReportOnly` check is dropped rather than replaced.

Batch-local decision: no replacement error field is introduced, and `Status`'s non-fatal-and-continue behaviour is unchanged.
`Remedy == ""` is an exact substitute for both writers — `Remedy`'s own doc comment already defines empty as report-only, and the field is `json:"remedy,omitempty"`, so a JSON consumer already sees the key absent on a report-only entry.
The observable JSON change is exactly one lost key, `report_only`.

## Cards

### Card 25: Re-scope the host-pollution scan and delete `ReportOnly`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/status.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `ReportOnly bool \`json:"report_only"\`` field and its doc comment from the `PollutionEntry` type at lines 35-36.
  Delete both of its writers: the `ReportOnly: true` assignment in the synthetic scan-error entry at lines 149-152, and the entire `_raddle` classification branch at lines 222-227.
  The scan-error entry keeps its existing shape otherwise — `Path` holding the `<scan error: %v>` text, `Remedy` left empty — and `Status` still records the error inline and continues rather than failing the pair.
  In `detectHostPollution`, the `git ls-files` pathspec at line 184 drops both `"_pattern"` and `"_raddle"`, leaving `[]string{"ls-files", "--", lyxdirs.LyxDirName}`.
  This loses no PATTERN coverage: `_lyx/PATTERN.md` and `_lyx/pattern/` are both under `_lyx`, so the surviving pathspec already matches them.
  In the classification switch at lines 209-228, drop the `strings.HasPrefix(tracked, "_pattern")` half of the first case so it matches on `lyxdirs.LyxDirName` alone, and delete the `_raddle` case entirely.
  Rewrite the doc comments that enumerate the scanned set: the file header at lines 4-5, `PollutionEntry`'s type comment at line 29 (which names `_raddle` as an example), `Status`' doc comment at line 72, the inline comment at lines 144-145, and `detectHostPollution`'s doc comment at lines 163-179.
  That last comment block contains a paragraph justifying the bare `"_pattern"`/`"_raddle"` literals under the Cwd Resolution Invariant's comparison-and-git-pathspec carve-out — delete it outright rather than rewording it, since no such literal survives.
  State in the rewritten comment that every remaining pollution class carries an automated `git rm --cached` remedy, which is why no report-only class exists.
- **Commit:** `refactor(fabricengine): scope host pollution to _lyx and delete PollutionEntry.ReportOnly`

### Card 26: Converge the pollution-scan tests

- **Context:**
  - `internal/fabricengine/status.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestDetectHostPollution_PatternTrackedAsRestorable` at lines 258-311 is the last remaining `pattern.DirName` consumer in this file.
  Re-point it at the new layout: rename it to `TestDetectHostPollution_LyxTrackedAsRestorable`, change `wantPath` at line 291 from `"_pattern/PATTERN.md"` to `pattern.PathspecFile`, and change the fixture at lines 271-280 so it creates and `git add`s `<host>/_lyx/PATTERN.md` instead of `<host>/_pattern/PATTERN.md`.
  Delete the `if found.ReportOnly { ... }` assertion at lines 302-304 — the field no longer exists, and the adjacent `found.Remedy == ""` assertion at line 305 plus the `rm --cached` substring check at line 308 already carry its intent.
  Update the doc comment at lines 258-261, which currently contrasts `_pattern` against report-only `_raddle`.
  Add a test pinning that a scan failure still produces its synthetic `<scan error: ...>` entry with an empty `Remedy`, and that `Status` returns the pair rather than failing — that behaviour previously carried a `ReportOnly: true` marker and now relies on the empty `Remedy` alone, so it needs a guard.
  Add a test pinning that a tracked `_raddle` path is **no longer** reported as pollution at all — that is the observable behaviour change this card delivers and nothing else pins it.
  Remove the `internal/pattern` import only if it becomes unused; `pattern.PathspecFile` keeps it in use here.
- **Commit:** `test(fabricengine): pin the _lyx-only pollution scan and the Remedy-based report-only shape`

### Card 27: Converge `fabriccli`'s pollution and residue JSON surface

- **Context:**
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Grep `internal/fabriccli/cli_test.go` for `report_only`, `ReportOnly`, and any pollution-entry JSON shape assertion.
  If a test pins the emitted JSON keys for a pollution entry, drop `report_only` from the expected set and state in a comment that the key is gone because `Remedy == ""` now carries the report-only signal on its own.
  If no such assertion exists, this card is a no-op on that front — say so in the commit message rather than inventing coverage.
  Independently, add or extend a CLI-level assertion that `lyx fabric status`' JSON still emits a pollution entry for a tracked `_lyx` path with a non-empty `remedy`, so the envelope shape stays pinned at the CLI boundary and not only at the engine boundary.
  Do not rename any exported identifier and do not change `internal/fabriccli/weft_verbs.go` in this card — its `PatternResidue` JSON flattening is unchanged by this batch, and its prose is batch 7's subject.
- **Commit:** `test(fabriccli): pin the pollution JSON shape after the report_only key removal`

## Batch Tests

`verify:` runs the integration-tagged suites for `internal/fabricengine` and `internal/fabriccli`.
`-tags integration` is required by both edited test files (`internal/fabricengine/junction_pattern_integration_test.go` and `internal/fabriccli/cli_test.go` each carry `//go:build integration`), and the tag is additive so the untagged tests in those packages run in the same invocation.
The per-package scope rather than a `-run` filter is justified because deleting a struct field is a compile-level change that can break any test in either package, not only the ones this batch edits.

Card 25 is a TDD candidate in the compile-break sense: delete the field first, let the build fail, and let the failure list confirm the three convergence sites are exactly the ones this batch names.
If the compiler reports a fourth site, that is a genuine miss and belongs in this batch, not deferred.

After this batch, `grep -rn 'ReportOnly\|report_only' internal/ cmd/` must return nothing.
