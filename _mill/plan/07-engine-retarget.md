# Batch: engine-retarget

```yaml
task: 'webster: rewrite for flat card list'
batch: engine-retarget
number: 7
cards: 10
verify: go test -tags integration ./internal/websterengine/...
depends-on: [2, 3, 6]
```

## Batch Scope

The atomic core of the rewrite: swap EVERY remaining `internal/builderengine` reference in
`internal/websterengine`'s verb/state/render files over to `internal/planparser`,
`internal/batcher`, and the webster-local mechanism/report/digest helpers created in batches 5–6;
delete the v2-only chain machinery; and convert the plan model from batch-supplied
(`builderengine.Plan.Batches`) to flat-cards-plus-batchifier (`planparser.Plan.Cards` grouped by
`batcher.Select(cfg.Batcher).Batch(...)`). This is ONE batch because the borrowed types
interlock — `BatchState.Digest`'s type, the fork-return report contract, the plan model, and the
fork/master templates all change together, so any file-by-file split leaves a non-compiling
package (see the `engine-retarget-is-atomic` Shared Decision). Batches 5–6 already staged the
replacement types, so each card here is a mechanical retarget of call sites: `builderengine.X` →
its documented webster-local / planparser / batcher equivalent, plus the batchifier wiring and
chain/oversized deletion. On completion `internal/websterengine` imports ZERO `builderengine`
symbols. The integration-suite fork + bisect (which also touches `runlevel.go`) is the separately
sequenced batch 8. Every edited `*_test.go` is retargeted in the same card as its source file so
the package stays green; git-spawning tests keep their `//go:build integration` tag and the
existing `TestMain`.

## Cards

### Card 25: state.go rework + delete chain machinery

- **Context:**
  - `internal/websterengine/digest.go`
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/state_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/websterengine/chain.go`
  - `internal/websterengine/chain_test.go`
- **Moves:** none
- **Requirements:** Retarget `BatchState.Digest` from `*builderengine.Digest` to the webster-local `*Digest` (batch 6). Drop the `State.ChainStartSHAs map[int]string` field entirely (chain machinery removed). Add per-card SHA capture to `BatchState`: a field `CardSHAs []string` recording the ordered per-card commit SHAs for the resume trail and SHA-bisect — in v0 (identity batcher, batch ≡ card) this holds exactly one element, the batch's single card SHA captured via `gitrepo.New(worktree).CurrentSHA()`; the multi-card enumeration path is dormant (no shipped grouping batcher) and needs a future git-log-range primitive, so do NOT add that primitive now. Keep every other `State`/`BatchState` field and `LoadState`/`SaveState`/`AcquireStateMutation`. Delete `chain.go` and `chain_test.go` (the `RestartChain`/`ChainStartSHAs` v2 concept). Update `state_test.go` for the new `Digest` type, the added `CardSHAs`, and the removed `ChainStartSHAs` (round-trip the reworked schema); remove any chain-related state assertions.
- **Commit:** `refactor(websterengine): rework BatchState onto webster Digest, drop chain machinery`

### Card 26: config + roles (batcher key, drop oversized)

- **Context:**
  - `internal/batcher/registry.go`
- **Edits:**
  - `internal/websterengine/config.go`
  - `internal/websterengine/config_test.go`
  - `internal/websterengine/roles.go`
  - `internal/websterengine/template.yaml`
- **Creates:**
  - `internal/websterengine/roles_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `config.go` add a `Batcher string` field to `Config` (yaml key `batcher`), defaulting to empty (resolved to `batcher.DefaultName` = `identity` by `batcher.Select` at the cli wiring site in batch 9). Remove the v2 oversized config keys `MasterOversized` (the `master_oversized` role spec), `BatchContextCapTokens` (`batch_context_cap_tokens`), and `BatchCardCap` (`batch_card_cap`) — all meaningless under the flat format's no-oversized model. Update `template.yaml` (webster's config template accessed by `ConfigTemplate()`) to drop the removed keys and document the new `batcher:` key. In `roles.go` remove `RoleMasterOversized` and its resolution in `ResolveRoles`, keeping `RoleMaster` and `RoleRecovery`. Update `config_test.go` and `roles_test.go` accordingly (assert the `batcher` key loads, the removed keys are gone, and `ResolveRoles` no longer maps the oversized role).
- **Commit:** `refactor(websterengine): add batcher config key, drop master_oversized and v2 caps`

### Card 27: render.go retarget + plan-level context injection

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/report.go`
- **Edits:**
  - `internal/websterengine/render.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget every `builderengine.Plan`/`builderengine.PlanBatch` in `render.go` to `planparser.Plan` and `batcher.Batch`. Change `RenderForkPrompt` to a signature that takes the plan (`*planparser.Plan`) and the fork's `batcher.Batch` (plus the existing `prevDigest`/`reportPath`/`worktreeRoot`/`selfFixCap` args): render the batch's cards (each card's `What` + the five typed file-op fields), and per the `fork-prompt-plan-level-context` decision INJECT the plan-level `## Shared Decisions` (`Plan.SharedDecisions`) into EVERY fork prompt, and INJECT the canonical `## Rename mechanic` (`Plan.RenameMechanic`) ONLY when the batch contains a card with a non-empty `Moves` field. Change `RenderBatchIndex(plan *planparser.Plan)` and `RenderProgress(plan *planparser.Plan, st *State)` to the flat model and STRIP the v2 batch annotations (`Oversized`, `VerifyDeferred`, `ChainEnd` — those `PlanBatch` fields no longer exist). Change `RenderMasterPrompt(plan *planparser.Plan, st *State, ...)`. Keep `MasterTemplate()`/`ForkTemplate()` accessors. Do NOT add the integration-suite renderer here (batch 8). The template-body rewrite is card 28; this card retargets the Go fill logic to the new types and the injected sections. `template_test.go` updates land in card 28 alongside the template text.
- **Commit:** `refactor(websterengine): retarget render onto planparser/batcher and inject plan-level context`

### Card 28: rewrite fork + master templates

- **Context:**
  - `docs/reference/plan-format-v3.md`
  - `internal/websterengine/render.go`
  - `internal/websterengine/report.go`
- **Edits:**
  - `internal/websterengine/fork-template.md`
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `fork-template.md` for the flat card-list model: instruct the fork to implement its batch's cards in declared order; run `go build ./...` + unit tests after EACH card; run each card's optional `verify:` immediately after committing that card (a non-zero `verify:` fails the card exactly like the build+unit gate); COMMIT per card; and return the minimal fork-return contract — `OK` or `FAILED`, the resulting head Git SHA, and ONLY the list of files changed outside the batch's declared file-ops (the deviation list; informational, never a reason to return `FAILED`). Drop all v2 batch/`## Scope`/oversized/chain/deferred-verify language. Rewrite `master-template.md` for the flat model: drop the `master_oversized` role, chain rollback, and scope-drift/`Distill` narrative; describe Master ingesting the minimal fork return (deviation deltas, never success narratives), capturing per-card SHAs, and the bracket-verb loop over batchifier-derived batches. Update `template_test.go` to assert the rewritten templates render correctly with the card 27 fill logic (fork prompt contains the injected `## Shared Decisions`; a `Moves:`-bearing batch's prompt contains the `## Rename mechanic`; a non-`Moves:` batch's does NOT) and that no v2 tokens (`oversized`, `chain`, `## Scope`) remain. Honor the Review Round / template pins style if any `*_test.go` asserts template invariants.
- **Commit:** `refactor(websterengine): rewrite fork/master templates for flat card list and fork-return contract`

### Card 29: runlevel.go retarget + batcher wiring

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/batcher/batcher.go`
  - `internal/batcher/registry.go`
  - `internal/websterengine/fingerprint.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/archive.go`
  - `internal/websterengine/outcome.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/strand.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the `Run` verb in `runlevel.go`: `builderengine.ParsePlan` → `planparser.ParsePlan`; `builderengine.Validate` + `builderengine.ValidateCaps` → `planparser.Validate(plan, worktreeRoot)` (drop the caps arg — no oversized); `builderengine.Fingerprint` → webster `fingerprint`; `builderengine.ClearPause` → webster `ClearPause`; `builderengine.ParseOutcome`/`OutcomeDone`/`OutcomePaused` → webster `parseOutcome`/`outcomeDone`/`outcomePaused`; `builderengine.ArchiveStateFile`/`ArchiveReportsDir`/`ArchiveStaleOutcome` → webster equivalents; `builderengine.DigestStatusDone` → webster `DigestStatusDone`; `builderengine.ErrRunBusy` → webster-local run-busy sentinel (define `var ErrRunBusy = errors.New("websterengine: run is already in progress")` locally, keeping the exported name the cli/tests use); `builderengine.RemoveStrandIfLive` → webster `removeStrandIfLive`. Wire the batchifier: after parse+validate, derive the execution batches via `batcher.Select(cfg.Batcher)` then `.Batch(plan.Cards)`, and retarget the zero-batch refusal to the batchifier output (an empty plan → zero batches → refuse). Thread `planparser.Plan` + the derived `[]batcher.Batch` through `RunDeps`/`RunResult` and the Master-loop plumbing. Keep the run-level lease, `--fresh` fingerprint crash/resume guard (now over webster `fingerprint`), fork-audit, and `MasterHandle`/`MasterStarter` interfaces. Retarget `runlevel_test.go` (the largest test file) to the fake batcher (identity), `planparser` fixtures, and the webster-local helpers; drop `ValidateCaps`/oversized/chain assertions.
- **Commit:** `refactor(websterengine): retarget runlevel onto planparser/batcher and webster helpers`

### Card 30: beginbatch.go retarget (drop chain path)

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/fingerprint.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/strand.go`
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/beginbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the `BeginBatch` bracket verb: `builderengine.PauseRequested` → webster `PauseRequested`; `builderengine.Plan`/`PlanBatch` → `planparser.Plan` + `batcher.Batch`; `builderengine.Digest` → webster `Digest`; `builderengine.Fingerprint` → webster `fingerprint`; `builderengine.BatchReportFileName` → webster `ReportFileName`; `builderengine.HeadSHA` → webster `headSHA`; `builderengine.RemoveStrandIfLive` → webster `removeStrandIfLive`. REMOVE the chain path entirely: drop `builderengine.ChainEndFor`, drop the `restartChain bool` parameter from `BeginBatch`, and delete the chain-rollback branch (the `--restart-chain` surface is gone). Keep the pause/fingerprint gates, start-SHA capture, the per-batch model assertion (now only the `master` role — no oversized selection), the previous-batch digest render into the fork prompt (via the card-27 `RenderForkPrompt` signature), and the `Injector` interface. Retarget `beginbatch_test.go` to the new types and signature; remove chain/restart and oversized-role assertions.
- **Commit:** `refactor(websterengine): retarget beginbatch, remove chain rollback path`

### Card 31: awaitbatch.go retarget

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/report.go`
- **Edits:**
  - `internal/websterengine/awaitbatch.go`
  - `internal/websterengine/awaitbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the `AwaitBatch` verb: `builderengine.Plan` → `planparser.Plan` (or the `batcher.Batch` the verb needs to resolve the report path); `builderengine.BatchReportFileName` → webster `ReportFileName`. Keep the bounded long-poll on the batch report path and the `Clock` seam. Retarget `awaitbatch_test.go` to the new types.
- **Commit:** `refactor(websterengine): retarget awaitbatch onto planparser and webster report naming`

### Card 32: recordbatch.go retarget

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/gitwrap.go`
- **Edits:**
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recordbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the `RecordBatch` verb: `builderengine.Plan` → `planparser.Plan`; `builderengine.Digest` → webster `Digest`; `builderengine.BatchReportFileName` → webster `ReportFileName`; `builderengine.ParseReport` → webster `ParseReport` (the minimal OK/FAILED + head SHA + deviations parser); `builderengine.ChangedFiles` → webster `changedFiles`; `builderengine.Dirty` → webster `dirty`; `builderengine.Distill` → webster `distill`. Capture the batch's per-card SHA(s) into `BatchState.CardSHAs` (v0: the single head SHA via the webster `headSHA` helper from `gitwrap.go`, cross-checked against the fork-reported head SHA) and persist the webster `Digest`; run the incremental fork audit unchanged. Record the fork-reported deviation list on the digest (informational, per the deviation-is-informational Shared Decision). Retarget `recordbatch_test.go` to the new report/digest and SHA-capture.
- **Commit:** `refactor(websterengine): retarget recordbatch onto webster report/digest and per-card SHA capture`

### Card 33: recoverbatch.go retarget

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/classify.go`
  - `internal/websterengine/poll.go`
  - `internal/websterengine/strand.go`
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/archive.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/recoverbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget the `RecoverBatch` verb (the heaviest `builderengine` consumer): `PollUntilTerminal` → webster `PollUntilTerminal`; `Starter` → webster `Starter`; `Plan`/`PlanBatch` → `planparser.Plan`/`batcher.Batch`; `Digest`/`DigestStatusDead`/`DigestStatusRunning`/`DigestStatusDone`/`DigestStatusStuck` → webster equivalents; `FirstFreeArchivePath` → webster `firstFreeArchivePath`; `BatchReportFileName` → webster `ReportFileName`; `ParseReport`/`Report`/`ReportStatusDone` → webster `ParseReport`/`Report`/`ReportStatusOK`; `ClassifyInputs`/`Classify` → webster equivalents; `TurnEnded`/`StrandLive`/`RemoveStrandIfLive` → webster equivalents; `ChangedFiles`/`Dirty` → webster `changedFiles`/`dirty`; `HeadSHA` → webster `headSHA`; and `builderengine.ImplementerTemplate` → webster's own fork template (`ForkTemplate()` / the recovery-strand prompt rendered from webster's `fork-template.md`), since the recovery strand implements the same batch a fork would. Keep the cold recovery-strand spawn/attach/await flow, terminal classification, and `PersistRecoveryTerminal` (now taking webster `*Digest`). Retarget `recoverbatch_test.go` fully to the webster types and fakes.
- **Commit:** `refactor(websterengine): retarget recoverbatch onto webster classify/poll/report/digest`

### Card 34: summary.go retarget

- **Context:**
  - `internal/websterengine/outcome.go`
  - `internal/websterengine/archive.go`
- **Edits:**
  - `internal/websterengine/summary.go`
  - `internal/websterengine/summary_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Retarget `summary.go`: `builderengine.ParseOutcome` → webster `parseOutcome`; `builderengine.ArchiveStaleOutcome` → webster `archiveStaleOutcome`; `builderengine.FirstFreeArchivePath` → webster `firstFreeArchivePath`. Keep `SummaryPath`, the `Summary` struct + `ParseSummary`, and `ArchiveStaleSummary`. Retarget `summary_test.go`. After this card, grep the whole `internal/websterengine` tree and confirm ZERO `builderengine` references remain (the import edge is fully cut); if `doc.go`'s prose still mentions the builderengine dependency, leave it for batch 10's doc fold but ensure no code/import references survive.
- **Commit:** `refactor(websterengine): retarget summary onto webster outcome/archive helpers`

## Batch Tests

`verify: go test -tags integration ./internal/websterengine/...` runs the full websterengine
suite — Tier-1 fake-based verb/state/render/config tests plus the integration-tagged git paths —
against the fully retargeted package. This is the batch that must leave `internal/websterengine`
importing zero `builderengine` symbols while green; the `-tags integration` flag ensures the
real-git paths (recover/record SHA capture) run, and the existing hermetic `TestMain` neutralizes
the operator gitconfig. Cross-package breakage (e.g. webstercli still on the old signatures) is
caught by the overview `go build ./...` gate and fully resolved in batch 9.
