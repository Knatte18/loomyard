# Batch: muxpoc

```yaml
task: Prune and consolidate the test suite (board first)
batch: muxpoc
number: 5
cards: 3
verify: go test ./internal/muxpoc/
depends-on: [1]
```

## Batch Scope

Apply the board pattern to `internal/muxpoc` (19 non-smoke funcs → ~14). All targets are
white-box `package muxpoc` with no build tag; the `muxpoc_smoke_test.go`
(`//go:build smoke`) E2E is out of scope and never runs in the default build. No test uses
`t.Parallel` today; folds keep that (use `t.Run` without `t.Parallel`). The already-clean
funcs (`TestExpandTpl`, `TestParseWindowSize`, `TestParsePaneList`,
`TestBuildColumnLayoutBottomDominatesAndAncestorsEqual`, and the distinct state funcs
`TestSaveLoadRoundtrip`, `TestLoadStateMissing`, `TestLoadStateCorrupt`,
`TestDeleteStateMissing`, `TestNewSessionID`) are **not** edited. Depends on batch 1.
Coverage floor: **33.0%** (default build).

## Cards

### Card 14: Fold layoutChecksum tests

- **Context:**
  - `internal/muxpoc/cmd.go`
  - `_mill/plan/baseline/muxpoc.txt`
- **Edits:**
  - `internal/muxpoc/cmd_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestLayoutChecksumMatchesPsmux` and
  `TestLayoutChecksumIsFourHexDigits` into one table-driven `TestLayoutChecksum`: two
  pinned-value rows (named after `TestLayoutChecksumMatchesPsmux`) plus an `"arbitrary"`
  row, with the 4-hex-digit shape applied as a per-row post-assertion (covering
  `TestLayoutChecksumIsFourHexDigits`). Preserve both original names in the name-map. Do
  not touch the other cmd_test funcs.
- **Commit:** `test(muxpoc): fold layoutChecksum tests`

### Card 15: Fold socketName and env-filtering tests

- **Context:**
  - `internal/muxpoc/state.go`
  - `_mill/plan/baseline/muxpoc.txt`
- **Edits:**
  - `internal/muxpoc/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** (a) Fold `TestSocketName`, `TestSocketNameStability`, and the inline
  stability re-check inside `TestSocketName` into one func `TestSocketName` whose subtests
  keep the original names — `TestSocketName` (the charset/prefix table) and
  `TestSocketNameStability` (root-vs-subdir + same-input stability); the inline re-check
  folds into the `TestSocketNameStability` subtest (record in name-map). (b) Fold
  `TestSanitizeEnv` and `TestStrippedEnvKeys` (complementary halves over the same input
  slice) into one func `TestEnvFiltering` with two subtests keeping the original names
  `TestSanitizeEnv` and `TestStrippedEnvKeys`, sharing one `environ` fixture (removes the
  duplicated literal, not coverage). No `t.Setenv` is involved (literal slices). Preserve
  the `isValidUUIDv4` helper. Preserve every assertion.
- **Commit:** `test(muxpoc): fold socketName and env-filtering tests`

### Card 16: Fold muxpoc CLI error tests

- **Context:**
  - `internal/muxpoc/cli.go`
  - `_mill/plan/baseline/muxpoc.txt`
- **Edits:**
  - `internal/muxpoc/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestRunCLINoSubcommandFails`,
  `TestRunCLIUnknownSubcommandFails`, `TestRunCLIUnknownFlagFails` into one table-driven
  `TestRunCLIErrors` (each case named after its original func). Give the flag-error row
  the `out.Len()==0` (stdout-empty) assertion the other two already have, so no assertion
  is lost. Assert exit code per row. Preserve the name-map entries.
- **Commit:** `test(muxpoc): fold CLI error tests`

## Batch Tests

`verify: go test ./internal/muxpoc/` runs the default (non-smoke) muxpoc package; the
`//go:build smoke` E2E is excluded. After the batch, run `go test ./internal/muxpoc/ -cover`
and confirm coverage **≥ 33.0%**; diff `go test ./internal/muxpoc/ -list '.*'` against
`_mill/plan/baseline/muxpoc.txt`. Folds use `t.Run` without `t.Parallel` to preserve the
package's current serial ordering.
