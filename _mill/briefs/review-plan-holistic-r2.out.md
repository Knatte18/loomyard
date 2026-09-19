MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] cli.go struct comments miss four drive/run hits
**Location:** batch 1 (go-cli-rename), card 2 **Issue:** Card 2's Requirements name only the `env`, `shedPaths`, and `frictionDir` struct-field comments on `loomCLI` (`internal/loomcli/cli.go`) for rewrite, but the struct also carries `cfg` ("Batch 5's run/bootstrap verb reads..."), `registry` ("carried onto the struct so drive.go can pass it to landingDeps..."), `runner` ("carried onto the struct so drive.go can pass it to landingDeps..."), and `landingCfg` ("not only inside drive's detached driver log") — all four verified in the current file and all naming the retired verb or the pre-move filename `drive.go`, none listed in card 2's Requirements or any later card's Edits for `cli.go`. **Fix:** Extend card 2's Requirements to cover all seven struct-field comments naming `drive`/`run`/`drive.go`, not just three.

### [BLOCKING:scope] step.go header and a RunE comment miss the rename
**Location:** batch 1, card 3 **Issue:** Card 3's Requirements for `internal/loomcli/step.go` cover only the `Long` string's opening sentence. The file's own header comment ("It bootstraps idempotently exactly as `run` does... Unlike `run`, it spawns no detached driver...") and a RunE-body comment ("A producer hard error mirrors drive's handling of the same condition") also name the retired bootstrap/foreground verbs — verified in the current file — and no other card edits `step.go`. **Fix:** Add the header comment and the `drive`-naming RunE comment to card 3's Requirements for `step.go`.

### [BLOCKING:scope] smoke_test.go prose and test names retain drive/run
**Location:** batch 1, card 6 **Issue:** Card 6 scopes `internal/loomcli/smoke_test.go` to argv literals, `findDriverPIDs`, and nothing else. Verified against the current file, its header doc comment (lines 1-29, "`lyx loom run` spawns its detached driver... a binary `loom drive` knows how to dispatch"), `newWiredPairFixture`'s doc comment ("so a caller never needs a --parent flag on its own first \"loom run\" call"), and three test function names — `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed`, `TestSmokeDriveStandalone_RefusesOnNeverSeededPair`, `TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog` — all narrate or encode the retired `drive` verb, and no card in the plan edits this file's prose or identifiers. **Fix:** Add a card-6 requirement (or a new card) to rewrite the file's header doc, the fixture-doc comment, and the three `Drive`-named test functions. Relatedly, `smoke_bootstrapwiring_test.go` line ~51 ("written, documented, and unit-tested in run.go... neither runCmd's RunE...") cites the pre-move filename `run.go` and method `runCmd`, which card 6's generic "rewrite comments naming lyx loom run/lyx loom drive/ly-supervise" instruction for that file does not cover.

### [BLOCKING:scope] ly-drive SKILL.md Self-report section keeps `drive`
**Location:** batch 4 (skill-rename), card 13 **Issue:** Card 13 rewrites only the single `lyx loom run` bootstrap-remedy hit in the pre-loop-baseline section. Verified against the current `plugins/ly/skills/ly-supervise/SKILL.md`, the "## Self-report" section separately states "Loom's two automatic self-report tiers both hang off `lyx loom drive`'s own run" and "`step` never spawns the reflection pass that `drive` runs" — both naming the escape-hatch verb that becomes `run` after this rename, and neither is mentioned in card 13's Requirements. **Fix:** Extend card 13's Requirements to rewrite both Self-report-section hits (`lyx loom drive` → `lyx loom run`, and bare `` `drive` `` → `` `run` ``).

## Verdict

REQUEST_CHANGES
Four verified scope gaps leave the retired `run`/`drive` verb names surviving in comments, test identifiers, and skill prose.
MILL_REVIEW_END
