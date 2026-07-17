# Batch: bracket-verbs

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'bracket-verbs'
number: 5
cards: 6
verify: go test ./internal/websterengine/...
depends-on: [4]
```

## Batch Scope

The engine halves of the two bracket verbs — the core of webster's mechanism.
`BeginBatch` runs the gates, records the start-SHA, idempotently asserts the
Master's model, and writes the fork prompt file; `RecordBatch` runs the
incremental audit with the pinned check order, distills the digest, and
persists it. Plus webster's chain-restart variant. Everything is
dir-parameterized and driven through injectable deps (fake injector, fake
engine, fake clock) — no CLI, no weft (that is batch 8). External interface:
`BeginBatch`/`BeginResult`, `RecordBatch`/`RecordResult`, `RestartChain`,
`ErrPaused`/`ErrFingerprintMismatch`/`ErrNoBeginRecord` sentinels.

## Cards

### Card 19: webster RestartChain

- **Context:**
  - `internal/builderengine/chain.go`
  - `internal/builderengine/gitquery.go`
  - `internal/builderengine/spawn.go`
  - `internal/websterengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/chain.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `RestartChain(worktree string, st *State, plan *builderengine.Plan, member int, reportsDir string) (lowest int, err error)`
  — webster's variant of builder's chain rollback against webster's own
  `State` (builder's `RestartChain` couples to builder's `State` type, so this
  is a re-expression, not a copy of shared contract parsing): resolve the
  chain via `builderengine.ChainEndFor`/`ChainMembers` (any member may be
  named); require a recorded `ChainStartSHAs` entry (no caller-supplied SHA
  ever — hard error when unrecorded); `builderengine.ResetHard` to it; delete
  every member's report file (`builderengine.BatchReportFileName`) and clear
  their `Batches` entries (digests included); reset `CurrentBatch = 0`;
  return the chain's LOWEST member for the caller to re-point the begin at
  (builder's re-point rule). Lands first in this batch: `BeginBatch`
  (card 20) calls it.
- **Commit:** `webster: chain rollback against webster state`

### Card 20: BeginBatch

- **Context:**
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/pause.go`
  - `internal/builderengine/fingerprint.go`
  - `internal/builderengine/gitquery.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/roles.go`
  - `internal/websterengine/render.go`
  - `internal/shuttleengine/engine.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/beginbatch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type Injector interface { Inject(guid string, inputs []shuttleengine.PaneInput) error }`
  (satisfied by `*shuttleengine.Runner`). `type BeginDeps struct` carrying:
  `Plan *builderengine.Plan`, `State *State`, `Roles map[Role]modelspec.Resolved`,
  `Config Config`, `Engine shuttleengine.Engine`, `Injector Injector`,
  `WorktreeRoot, WebsterDir, ReportsDir, PromptsDir string`.
  `BeginBatch(deps BeginDeps, batchNumber int, restartChain bool) (*BeginResult, error)`
  sequence (caller holds the mutate lease and saves state):
  (1) pause gate — `builderengine.PauseRequested(deps.WebsterDir)` →
  webster-local `ErrPaused`;
  (2) fingerprint gate — `builderengine.Fingerprint(plan.Dir)` vs
  `State.PlanFingerprint` → webster-local `ErrFingerprintMismatch` naming both
  and pointing at `lyx webster run --fresh`;
  (3) optional `RestartChain` (card 19) when `restartChain` — then continue
  with the re-pointed lowest member exactly as builder's spawn does;
  (4) resolve the batch from the plan; capture
  `builderengine.HeadSHA(deps.WorktreeRoot)`; record the chain-start SHA in
  `ChainStartSHAs` if this batch is a chain member whose chain has no
  recorded SHA yet (first-member-records, never overwritten);
  (5) IDEMPOTENT MODEL ASSERTION: target role = `RoleMasterOversized` when
  the batch's `Oversized` flag is set, else `RoleMaster`; target model =
  `deps.Roles[target].Model`; when `State.AssertedModel != targetModel`, call
  `deps.Injector.Inject(State.MasterStrand, deps.Engine.ModelSwitchSequence(targetModel))`
  and set `State.AssertedModel = targetModel`; equal → no injection. This is
  the ONLY model-injection site (discussion.md `oversized-model-escalation`);
  (6) previous-batch digest: for batchNumber > 1 read
  `State.Batches[batchNumber-1].Digest` (nil-safe → `none (first batch)`
  fallback wording documented as resume-tolerant);
  (7) render the fork prompt via `RenderForkPrompt` and write it to
  `PromptsDir/NN-<slug>.md` (`MkdirAll`, overwrite allowed — re-renderable
  artifact);
  (8) update state: `CurrentBatch = batchNumber`,
  `Batches[batchNumber] = &BatchState{Slug, StartSHA, Kind: "fork", SpawnedAt}`.
  Return `BeginResult{BatchName, PromptPath, StartSHA, AssertedModel}`.
- **Commit:** `webster: BeginBatch with gates and idempotent model assertion`

### Card 21: RecordBatch

- **Context:**
  - `internal/builderengine/report.go`
  - `internal/builderengine/digest.go`
  - `internal/builderengine/gitquery.go`
  - `internal/builderengine/spawn.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/state.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/forkaudit.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/recordbatch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `type RecordDeps struct`: `Plan`, `State`, `Config`,
  `Engine shuttleengine.Engine`, `Layout *hubgeometry.Layout`,
  `WorktreeRoot, ReportsDir string`, `OutcomePath, SummaryPath string`,
  `Sleeper Sleeper`.
  `RecordBatch(deps RecordDeps, batchNumber int) (*RecordResult, error)`
  sequence (caller holds the lease and saves state):
  (1) `ErrNoBeginRecord` sentinel when `State.Batches[batchNumber]` is absent
  or already `Terminal` (record without begin — the bracket-discipline
  fail-loud);
  (2) incremental audit with settle: `fetch` closure calling
  `deps.Engine.AuditForksIncremental(State.MasterSessionID, deps.Layout.Cwd, seenSet)`;
  run `SettleRetry(fetch, State.SeenForkTranscripts, window, tick, deps.Sleeper)`;
  then `ClassifyAttribution` per the pinned order — zero-new after settle →
  hard error regardless of report presence; multi-new → warning carried on
  the result;
  (3) policy: `CheckParent(audit, deps.OutcomePath, deps.SummaryPath, weftRef)`
  and `CheckFork` on every new report — any violation is a hard error naming
  the transcript; `ForkWarnings` collected;
  (4) ATTRIBUTION ADVANCES UNCONDITIONALLY — append the new transcript paths
  to `State.SeenForkTranscripts` and `Batches[batchNumber].ForkTranscripts`
  BEFORE the report check, so a `no_report` retry sees only its own new
  transcript (discussion.md: attribution advances on every call,
  `no_report` included);
  (5) report presence at
  `ReportsDir/builderengine.BatchReportFileName(number, slug)`: absent →
  `RecordResult{NoReport: true}` — batch stays non-terminal and
  `CurrentBatch` unchanged (Master's ladder re-forks once); present →
  `builderengine.ParseReport` (parse failure = hard error, never guessed),
  verify `report.Batch` equals the polled batch id (mismatch = malformed,
  fail loud);
  (6) distill: `builderengine.ChangedFiles(WorktreeRoot, StartSHA)`,
  `builderengine.Dirty(WorktreeRoot)`, `builderengine.Distill(report,
  changed, batch.Scope, dirty)`;
  (7) persist: `BatchState.Digest = &digest`, `Terminal = true`,
  `Status = digest.Status`, clear `CurrentBatch`.
  Return `RecordResult{Digest *builderengine.Digest, NoReport bool, Warnings []string}`.
- **Commit:** `webster: RecordBatch with incremental audit and digest persistence`

### Card 22: BeginBatch tests

- **Context:**
  - `internal/websterengine/beginbatch.go`
  - `internal/builderengine/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fake `Injector` (records every `(guid, inputs)` call) and
  a stub `shuttleengine.Engine` whose `ModelSwitchSequence` returns a
  recognizable marker sequence. Plan values are constructed as
  `*builderengine.Plan` literals (no plan-dir parsing needed beyond the
  fingerprint gate, which uses a `t.TempDir()` with seed `*.md` files).
  Cover: pause flag → `ErrPaused` (flag written via
  `builderengine.RequestPause`); fingerprint mismatch → `ErrFingerprintMismatch`;
  model assertion — oversized batch with `AssertedModel` at the master model
  → exactly one Inject with the oversized model then `AssertedModel` updated;
  same-model batch → zero Inject calls (idempotence); prompt file written
  under the prompts dir with `prev_digest` populated from the persisted
  digest for batch N>1 and the first-batch fallback for batch 1; state
  updated (CurrentBatch, BatchState fields, chain-start SHA recorded once and
  never overwritten). Git-backed pieces (`HeadSHA`) make this file
  `//go:build integration`-tagged, with a package `testmain_test.go` calling
  `lyxtest.HermeticGitEnv()` (create it in this card; later test cards reuse
  it).
- **Commit:** `webster: BeginBatch tests`

### Card 23: RecordBatch tests

- **Context:**
  - `internal/websterengine/recordbatch.go`
  - `internal/builderengine/report.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/recordbatch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Stub engine returning scripted `ForkAudit` sequences;
  recording fake `Sleeper`. Cover: no begin record → `ErrNoBeginRecord`;
  zero new transcripts through the whole settle window → hard error even when
  a report file exists (the unfakeable-report rule); transcript appears on a
  later settle tick → clean; one new transcript + valid report → digest
  distilled and persisted, `Terminal` set, `CurrentBatch` cleared, transcript
  attributed to both `SeenForkTranscripts` and the batch; one new transcript +
  NO report → `NoReport` result, batch non-terminal, attribution still
  advanced (the retry then sees exactly one new transcript — assert the
  second call's clean path); two new transcripts → warning, never error;
  parent write outside the two contract files → hard error; report `batch:`
  mismatch → hard error; malformed report YAML → hard error.
  Integration-tagged (git-backed `ChangedFiles`/`Dirty`), reusing the
  package `testmain_test.go`.
- **Commit:** `webster: RecordBatch tests`

### Card 24: chain tests

- **Context:**
  - `internal/websterengine/chain.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/chain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Integration-tagged (uses a real scratch git repo for
  `ResetHard`): recorded chain-start SHA → hard reset performed, member
  reports deleted, member `Batches` entries (and their digests) cleared,
  `CurrentBatch` zeroed, lowest member returned regardless of which member
  was named; unrecorded chain → hard error, nothing touched.
- **Commit:** `webster: chain rollback tests`

## Batch Tests

`go test ./internal/websterengine/...` (Tier 1: audit/template/state/config
tests from earlier batches keep running untagged; this batch's new tests are
integration-tagged where they spawn git, under the package's hermetic
`TestMain`). The full integration tier runs under `go test -tags integration
./internal/websterengine/...` on demand; the batch verify stays the plain
Tier-1 invocation per the Test Tier Purity Invariant.
