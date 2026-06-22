# Batch: weft

```yaml
task: Prune and consolidate the test suite (board first)
batch: weft
number: 3
cards: 2
verify: go test -tags integration ./internal/weft/
depends-on: [1]
```

## Batch Scope

Apply the board pattern to `internal/weft` (20 → ~15). Two independent fold targets: the
untagged `config_test.go` (Tier 1) and the integration-tagged `weft_integration_test.go`
(Tier 2). The already-clean tables (`TestConfigDirs`, `TestStatus`, `TestStatus_Junction`,
`TestStatus_JunctionOk_Windows`, `TestCommit`, `TestPush`) and the wiring-unique
`cli_test.go:TestRunCLI_StatusWithMinimalFixture`,
`weft_integration_test.go:TestRunCLI_EnvMapToOption`, and `TestCommit_ScopedPathspec` are
**not** edited. Depends on batch 1. Coverage floor: **64.6%** (`-tags integration`).

## Cards

### Card 8: Fold weft config_test LoadConfig variants

- **Context:**
  - `internal/weft/config.go`
  - `_mill/plan/baseline/weft.txt`
- **Edits:**
  - `internal/weft/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestLoadConfig_DefaultWhenNoYAML`,
  `TestLoadConfig_OverrideFromYAML`, `TestLoadConfig_MissingLyx` into one table-driven
  `TestLoadConfig` (`{name, writeYAML, mkLyx, wantPathspec, wantErrSubstr}`), each case
  named after its original func. Drop the inline `cfg.Dirs()` re-assertion inside the
  Override case — `Dirs()` is owned by `TestConfigDirs`; the `Pathspec` equality already
  proves the YAML load. Keep `TestDefaultConfig` and `TestConfigDirs` unchanged. Preserve
  every path/error assertion. This card is Tier 1 (untagged file).
- **Commit:** `test(weft): fold config_test LoadConfig variants`

### Card 9: Fold push-integration duplicates, drop redundant FF pull

- **Context:**
  - `internal/weft/sync.go`
  - `internal/weft/sync_test.go`
  - `_mill/plan/baseline/weft.txt`
- **Edits:**
  - `internal/weft/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** (a) Fold `TestPushIntegration_CommitLandsOnBare`,
  `TestPushIntegration_RebaseRetryOnNFF`, and `TestSyncIntegration_EventuallyPushed` into
  one table-driven `TestPushIntegration` (each case named after its original func). The
  three are the same straight-line commit+push happy path
  (`RebaseRetryOnNFF` does not actually set up a non-fast-forward remote);
  `EventuallyPushed` is the superset (captures HEAD SHA and verifies `cat-file -e` on the
  bare remote), so make its cat-file-on-bare assertion the strongest row and preserve all
  three named cases. All are `t.Parallel` over isolated `lyxtest.CopyWeft` fixtures. (b)
  **Drop** `TestPullIntegration_FastForward` — a strict subset of
  `sync_test.go:TestPull_FastForward` (which does the full FF cycle with content restore);
  optionally retain it as a trivial "no-op pull" row of the fold if the empty-pull edge is
  wanted. Keep `TestRunCLI_EnvMapToOption` (serial, `t.Setenv`+`t.Chdir`) untouched.
  Record each dropped/folded name in the name-map.
- **Commit:** `test(weft): fold push-integration, drop redundant FF pull`

## Batch Tests

`verify: go test -tags integration ./internal/weft/` runs the full Tier-2 weft package
(superset of the untagged `config_test` fold and the integration `weft_integration_test`
fold). After the batch, run `go test -tags integration ./internal/weft/ -cover` and
confirm coverage **≥ 64.6%**; diff `go test -tags integration ./internal/weft/ -list '.*'`
against `_mill/plan/baseline/weft.txt`. The folded `TestPushIntegration` members are all
`t.Parallel` and merge safely; do not add `t.Parallel` to `TestRunCLI_EnvMapToOption`.
