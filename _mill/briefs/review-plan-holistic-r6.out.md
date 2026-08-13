MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-13
```

## Findings

### [BLOCKING:scope] Card 69's zero-hit Copy* grep fails on files no card touches
**Location:** batch 11 card 69, cross-checked against `internal/configcli/configcli_test.go` and `internal/loomengine/config_test.go`.
**Issue:** Both files carry bare-word prose mentions of the retired fixture helpers — `configcli_test.go:5` ("e2e test with real fabriccli.RunCLI over CopyPaired"), `loomengine/config_test.go:3` ("no CopyWeft, no SeedConfig") — and neither file contains the word `lyxtest`, so card 2's `grep -rl '\blyxtest\b'` sweep never selects them, and no other card lists them in `Context:`/`Edits:`. Card 69's `grep -rn 'CopyPaired\|CopyPairedLocal\|CopyWeft\|CopyWarpHub'` zero-hit check will therefore fail with no card responsible for the fix, per its own "fix it under the batch that owns the file" instruction.
**Fix:** Add both files to a batch's `Edits:` (batch 4 card 22 for `configcli_test.go`, batch 6 card 36 for `loomengine/config_test.go`) with a requirement to retarget the stale cross-reference comment.

### [BLOCKING:scope] The lyxtest sweep renames the qualifier but not the dangling helper name
**Location:** batch 1 card 2, vs. `internal/perchcli/run_test.go:10`, `internal/perchcli/cli_test.go:9`, `internal/fabricengine/warpbinding_reconcile_integration_test.go:12`, `internal/boardengine/boardtest/sync_test.go:24`, `internal/fabriccli/cli_test.go:31,318,320,450`, `internal/configcli/configcli_integration_test.go:4,29,265`.
**Issue:** These files ARE in card 2's `Edits:` list (they contain `lyxtest`), so the sweep turns e.g. "lyxtest's CopyPairedLocal" into "gitkit's CopyPairedLocal" — but `CopyPairedLocal`/`CopyPaired`/`CopyWarpHub`/`CopyWeft` are deleted outright in batch 11 card 67, so the comment still names a now-nonexistent identifier and still trips card 69's grep gate. Verified by comparing a narrow `lyxtest\.Copy\w+\(` call-count grep (141 hits, matching the plan's own arithmetic exactly) against a bare-token grep (226 hits) — the ~85-hit gap is uncounted prose the per-call "replace the N calls" requirements never target.
**Fix:** Batch 11 needs an explicit bare-word prose sweep for `CopyPaired`/`CopyPairedLocal`/`CopyWeft`/`CopyWarpHub` (mirroring card 2's `lyxtest` sweep and card 12's `fabrictest` sweep) run before card 69, not just per-call-site replacement.

### [BLOCKING:consistency] fabric_test.go's header comment goes stale; card 51 doesn't touch it
**Location:** batch 8 card 51, `internal/fabricengine/fabric_test.go` lines 7-8.
**Issue:** The header comment says `open_integration_test.go` pins the same contract "against a real paired lyxtest fixture (CopyPaired; TestOpen_MissingWarpWorktree / TestOpen_MissingSiblingWorktree))". Card 51's requirement for this file is scoped only to the four `NewPairedForTest` textual references; after batch 8 card 48 migrates `open_integration_test.go` off `CopyPaired` onto `hubforge.NewHub`, this becomes a false factual claim, not merely an unrenamed word — the exact hazard card 12 explicitly warns about for three other files, but that warning wasn't carried to this one.
**Fix:** Add a requirement to card 51 (or a new card) rewriting lines 7-8 to name `hubforge.NewHub` instead of the retired fixture.

### [BLOCKING:scope] fabriccli's hand-rolled _board scaffolding is stand-in-hub scaffolding, unnamed by card 39
**Location:** batch 7 card 39, `internal/fabriccli/cli_test.go`'s `TestRunCLI_EnvMapToOption` (lines ~316-327).
**Issue:** This test hand-writes `_board`'s `configengine.ConfigDir`/`fabric.yaml` directly because "CopyPaired never materializes a _board dir" — but a real hub from `hubforge.NewHub` already carries a real, committed `_board`/`fabric.yaml` (per batch 3 card 17's `TestNewHub_IsARealHub`). This is the same shape of stand-in-hub scaffolding that batch 4 card 27 explicitly named and deleted for perchcli's `nested`/`warpSubdir` sites, but card 39's requirements never name this test or its scaffolding specifically, only the generic field-mapping table.
**Fix:** Name `TestRunCLI_EnvMapToOption` in card 39 and require deleting its hand-rolled board seed in favor of the real hub's materialized `_board`.

## Verdict

REQUEST_CHANGES
Card 69's completeness gate will fail against several files no card edits or names; add them and a closing bare-word sweep.
MILL_REVIEW_END
