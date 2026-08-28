# Batch: retarget-callers

```yaml
task: "Producer-agnostic final-summary artifact + wire Finalize"
batch: "retarget-callers"
number: 2
cards: 2
verify: go vet ./... && go vet -tags integration ./... && go test ./internal/summaryparser/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shedadapters/... ./internal/landingshed/... ./internal/loomcli/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
depends-on: [1]
```

## Batch Scope

This batch moves every consumer off `websterengine`'s four summary names and deletes them, leaving `internal/summaryparser` the sole owner of the read contract.
It also performs the `landingshed.Deps` told-path swap, which is what makes `landingshed` producer-agnostic: `WebsterDir` out, `FinalSummaryPath` in, and `internal/websterengine` dropped from `landingshedAllowedImports`.

It is two cards, deliberately, and each one is atomic: card 5 changes an exported struct field that five packages fill, and card 6 deletes four exported names that four packages call.
Splitting either further would leave the tree uncompilable at a card commit boundary.
Card 5 lands first so `internal/landingshed` is already off `websterengine.ParseSummary` before card 6 removes it.

No behavioural change ships here.
`Publish` reads the same artifact and produces the same pull-request title and body; `websterengine.ArchiveStaleSummary` and `websterengine.AppendIntegrationFailure` keep their behaviour and only change which package resolves their path.
The interface batch 3 consumes is `Deps.FinalSummaryPath` plus `summaryparser.Parse`, both already reachable from `internal/landingshed` at the end of this batch.

## Cards

### Card 5: Swap landingshed onto a told FinalSummaryPath

- **Context:**
  - `internal/summaryparser/summary.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/landingshed/deps.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/publish_test.go`
  - `internal/landingshed/publish_integration_test.go`
  - `internal/landingshed/seam_enforcement_test.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/landingdeps_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_simple_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/landingshed/deps.go`, delete the `WebsterDir string` field and its doc comment from `Deps`, and add `FinalSummaryPath string` in the same position.
  Its doc comment states that it is the told absolute path to the final-summary artifact itself — not a directory, and not a producer's directory — and that the caller resolves it, so neither producer in this package knows which producer wrote the file.
  Do not add a second field alongside it; carrying both would be the derived near-duplicate `ScratchDir`'s own comment already argues against.

  `Deps`' field count is unchanged by this swap — one field out, one field in — but two comments that state that count are already wrong today, before this task touches anything: `internal/shedrecipe/recipe.go` says `landingshed.Deps` "already carries fourteen fields" and `internal/loomcli/landingdeps_test.go` says the reflection guard catches "a fifteenth field added later", while the struct in fact carries fifteen fields.
  Correct both counts as a drive-by — `internal/loomcli/landingdeps_test.go` is already being edited by this card, and `internal/shedrecipe/recipe.go`'s comment is about the exact struct whose field set this card changes, so leaving a knowingly-wrong count behind while editing around it would make the next reader trust neither comment.
  Do not claim in the commit message or anywhere else that the counts were accurate; they were not.

  In `internal/landingshed/publish.go`, replace the `websterengine.ParseSummary(websterengine.SummaryPath(p.deps.WebsterDir))` call in step 8 with `summaryparser.Parse(p.deps.FinalSummaryPath)`, and replace the `internal/websterengine` import with `internal/summaryparser`.
  Keep the existing `landingshed: %s: parse summary artifact: %w` wrapping and the existing disposition — a returned error, not `Stuck` — unchanged.
  `summary.Title` and `summary.Body` keep reaching `github.NewPullRequest` as separate fields, byte-identical to today.

  In `internal/landingshed/seam_enforcement_test.go`, remove the `github.com/Knatte18/loomyard/internal/websterengine` entry from `landingshedAllowedImports` and add `github.com/Knatte18/loomyard/internal/summaryparser`.
  The allowlist is a positive membership list, so the test passes either way if the stale entry is left behind — dropping it is this task's own enforcement of producer-agnosticism and must not be skipped.

  In `internal/landingshed/publish_test.go`, change `newTestDeps`'s `WebsterDir: t.TempDir()` to a `FinalSummaryPath` derived from a `t.TempDir()` via `summaryparser.Path`, and update the enclosing doc comment, which currently describes "a webster dir with no summary.md yet".
  Change `writeSummary` to take the artifact path directly (or the directory plus `summaryparser.Path`) and drop its `websterengine` import, then update every call site so the artifact it writes is the path `newTestDeps` told.
  Apply the same change to `writeSummaryLanding` and the `landingshed.Deps` literal in `internal/landingshed/publish_integration_test.go`, which is `//go:build integration`-tagged and must keep compiling under that tag.

  In `internal/loomcli/landingdeps.go`, change the `WebsterDir: geom.WebsterDir` assignment to `FinalSummaryPath: summaryparser.Path(geom.WebsterDir)` and add the `internal/summaryparser` import.
  Keep the `internal/websterengine` import — `landingDeps` still takes a `websterengine.Geometry` parameter.
  In `internal/loomcli/landingdeps_test.go` the reflection-based drift guard needs no structural change; only its `geom` fixture stays as-is, since `WebsterDir` there is a `websterengine.Geometry` field, not a `landingshed.Deps` field.

  In `internal/shedrecipe/entries_simple_test.go`'s `validLandingDeps`, `internal/shedbuild/fixture_test.go`'s `testLandingDeps`, and `internal/loomrecipe/fixture_test.go`'s `testLandingDeps`, replace `WebsterDir: dir` with a non-empty `FinalSummaryPath` derived from the same `dir`.
  None of these three packages ever calls `Call`, so no artifact is written to disk here — a told path string is enough.
- **Commit:** `refactor(landingshed): replace Deps.WebsterDir with a told FinalSummaryPath`

_The `internal/shedrecipe/recipe.go` edit in this card is a one-word field-count comment correction only.
Do not change `Env.Landing`'s type, its whole-struct passthrough shape, or any other line in that file._

### Card 6: Delete the four websterengine summary names and retarget the rest

- **Context:**
  - `internal/summaryparser/summary.go`
  - `internal/summaryparser/summary_test.go`
  - `internal/websterengine/archive.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/webstercli/smoke_test.go`
- **Edits:**
  - `internal/websterengine/summary.go`
  - `internal/websterengine/summary_test.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/integration_test.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/shedadapters/webster.go`
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/webster_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/summary.go`, delete `SummaryFileName`, `SummaryPath`, `Summary`, and `ParseSummary` outright.
  No deprecated wrapper survives.
  Keep `ArchiveStaleSummary` and `AppendIntegrationFailure`, changing each one's `path := SummaryPath(websterDir)` line to `path := summaryparser.Path(websterDir)`, and add the `internal/summaryparser` import.
  `ArchiveStaleSummary` keeps building its own archive target name with `fmt.Sprintf("summary-%s%s.md", stamp, suffix)` over `archiveTimestampFormat` and `firstFreeArchivePath` — the archive-name shape stays webster's own and does not move.
  Rewrite the file-header comment so it describes only the two write helpers that remain and points at `internal/summaryparser` for the path and the parse.

  In `internal/websterengine/runlevel.go`, change `filepath.Abs(SummaryPath(deps.Geom.WebsterDir))` to `filepath.Abs(summaryparser.Path(deps.Geom.WebsterDir))`, and change both `ParseSummary(summaryPath)` calls — the required one on the `outcome: done` path and the best-effort one on the stuck/paused path — to `summaryparser.Parse(summaryPath)`.
  Both keep their existing dispositions: a hard error on `outcome: done`, a discarded error otherwise.
  Update the `RunResult.SummaryTitle` field's doc comment, which names `ParseSummary`, to name `summaryparser.Parse`.
  Add the `internal/summaryparser` import.
  Do not touch `ArchiveStaleSummary`'s or `AppendIntegrationFailure`'s call sites in this file — those functions keep their names and signatures.

  In `internal/webstercli/recordbatch.go`, change the `SummaryPath: websterengine.SummaryPath(c.geom.WebsterDir)` assignment filling `websterengine.RecordDeps.SummaryPath` to `summaryparser.Path(c.geom.WebsterDir)`, and add the `internal/summaryparser` import.
  `RecordBatchDeps.SummaryPath` is a told string field and keeps its own name — do not rename it.

  In `internal/shedadapters/webster.go`, change the `Done` outcome's `shedengine.OutputPointer{Path: websterengine.SummaryPath(p.deps.Geom.WebsterDir)}` to use `summaryparser.Path`, adding the `internal/summaryparser` import alongside the existing `internal/websterengine` one, which this package legitimately keeps.
  In `internal/shedadapters/doc.go`, update the `WebsterProducer` bullet's parenthetical naming `websterengine.SummaryPath` to name `summaryparser.Path` instead.

  In `internal/websterengine/summary_test.go`, delete every `ParseSummary` test — the valid-parse case, the leading-blank-lines case, the missing-file case, the empty-file table, the no-heading case, and the empty-title case — since card 2 already reproduced them in `internal/summaryparser/summary_test.go`.
  Keep the `ArchiveStaleSummary` coverage and the `writeSummaryFile` and `summaryFixedClock` helpers it uses, replacing each `websterengine.SummaryFileName` reference with `summaryparser.FileName` and adding the import.
  Rewrite the file-header comment so it describes only the archive coverage that remains.

  In `internal/websterengine/integration_test.go`, replace `websterengine.SummaryPath(websterDir)` with `summaryparser.Path(websterDir)` and add the import.
  This file is `//go:build integration`-tagged and must keep compiling under that tag.
  In `internal/shedadapters/webster_test.go`, replace each occurrence of `wantPath := websterengine.SummaryPath(dir)` — there are two, one in `TestWebsterProducer_OutcomeDone` and one in `TestWebsterProducer_CancelledDuringRun_OutcomeDoneStillSucceeds` — with `summaryparser.Path(dir)`, and add the import.

  Leave the bare `"summary.md"` literals in `internal/websterengine/recordbatch_test.go`, `internal/websterengine/runlevel_test.go`, and `internal/webstercli/smoke_test.go` exactly as they are — the Summaryparser Sole-Parser Invariant is scoped to production code, and a fixture writing the literal filename is the clearer test.
- **Commit:** `refactor(websterengine): delete the summary read contract in favour of summaryparser`

## Batch Tests

`verify` runs three gates.
`go vet ./...` and `go vet -tags integration ./...` typecheck every package in the module including its test files, which is what catches a missed caller anywhere in the tree — the untagged pass alone would silently skip `internal/landingshed/publish_integration_test.go` and `internal/websterengine/integration_test.go`, both of which this batch edits.
The `go test` list then runs every package whose behaviour or fixtures this batch touches: `summaryparser`, `websterengine`, `webstercli`, `shedadapters`, `landingshed`, `loomcli`, `shedrecipe`, `shedbuild`, and `loomrecipe`.

The enumerated `go test` list is deliberate rather than a repo-wide `./...`: this batch is a rename sweep whose blast radius is exactly those nine packages, and `go vet` already covers compilation everywhere else.
The `//go:build integration` tiers are not executed here, only typechecked — batch 3's own verify runs `internal/landingshed`'s integration tier for real, and the task's `pipeline.done_gate` runs the repo-wide tagged suite before the task is marked done.

`internal/landingshed/seam_enforcement_test.go`'s allowlist edit is asserted by nothing — the test passes with or without it — so it is a plan-level checklist item that review must confirm, not a test.
