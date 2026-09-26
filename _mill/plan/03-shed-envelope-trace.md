# Batch: shed-envelope-trace

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
batch: shed-envelope-trace
number: 3
cards: 6
verify: go build ./... && go test ./internal/shedverbs/ ./internal/loomcli/ ./internal/battencli/ ./internal/landingshed/ -run 'TestStep|TestStatus|TestNoResolverNoModuleCLIInvariant|TestSpecFor|TestBattenPreStep|TestPublish|TestVerbRefusals|TestRun' && go test -tags integration ./internal/battencli/ -run 'TestBattenIntegration_RealReadStatus|TestBattenIntegration_NonPrimeRefusal' && go test -tags integration ./internal/landingshed/ -run 'TestPublish_MergesInCleanlyBeforeCreatingPullRequest' && go test -tags smoke ./internal/loomcli/ -run 'TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy|TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus'
depends-on: [1, 2]
```

## Batch Scope

Puts the trace on the envelope.
The generic `step` body logs its own boundaries and every envelope it emits (success and each five-kind error) carries `trace_file`, `friction_dir` and `scratch_dir`;
the generic `status` body gains `history_length`, `interrupt_policy` and `trace_dir` on every envelope, and the absent-status refusal carries `found: false`;
loom and batten fill the two new told `Spec` fields at their one `specFor` each and drop the status keys that moved into the generic core;
landing's one GitHub write logs at `Info`.
Batch 4 asserts the new keys on real entry points; batch 5's skill reads them.
One batch because the `StepEnvelope` signature and the status key sets are pinned by tests in `internal/shedverbs`, `internal/loomcli` and `internal/battencli` together, so splitting would leave a batch that does not build.
Batch-local decision: the three path keys travel as one exported `StepLocations` struct so `StepEnvelope` and the error-envelope helper share one source.

## Cards

### Card 5: step envelope carries the trace, friction and scratch locations

- **Context:**
  - `internal/logger/sink.go`
  - `internal/logger/logger.go`
  - `internal/output/output.go`
  - `internal/shedengine/run.go`
  - `internal/shedverbs/testsupport_test.go`
- **Edits:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/step_test.go`
  - `internal/shedverbs/seam_enforcement_test.go`
  - `internal/loomcli/step_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/shedverbs/spec.go`, add two told `Spec` fields after `StatusLockPath`:
    `ScratchDir string` — the run's ephemeral shed scratch directory (`shedrun.ScratchDir` of the addressed run), reported on every step envelope as `scratch_dir`; its doc comment says it is never filled from `shedrun.RunDir`, the run's durable tracked directory, because driver records written under it must never land in tracked content, and that `""` means the arming module supplied none;
    `FrictionDir string` — the recipe's own agent friction-note directory, `""` when the recipe has none or friction is off, reported as `friction_dir`.
  - In `internal/shedverbs/step.go`, add exported `StepLocations struct { TraceFile, FrictionDir, ScratchDir string }` with a doc comment naming the three envelope keys it fills.
    Change `StepEnvelope` to `StepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string, loc StepLocations) map[string]any`, adding `trace_file`, `friction_dir`, `scratch_dir`; update its doc comment's key list and "exactly the ten" wording to thirteen.
    Add unexported `stepErrFields(kind string, loc StepLocations) map[string]any` returning `kind` plus the same three keys.
  - In `stepCmd`'s `RunE`, after the `clihelp.ShouldAbort` check, call `logger.Info("shed: step", "status_file", spec.StatusPath)`.
    Route every error envelope (the `PreStep` error, the nil `BuildShed`, a `BuildShed` error, the busy mapping, and `KindProducer`) through one local closure that first calls `logger.Warn("shed: step refused", "kind", kind, "error", msg)` and then emits `output.ErrFields(out, msg, stepErrFields(kind, locations()))`, where `locations()` builds `StepLocations{TraceFile: logger.TraceFile(), FrictionDir: spec.FrictionDir, ScratchDir: spec.ScratchDir}` — computed after the `Warn`, so the trace file named is the one holding it.
    After `shed.Step` returns without error and before `PostStep`, call `logger.Info("shed: step done", "producer", res.Producer, "outcome", string(res.Outcome), "state", string(res.State), "next", res.Next, "reason", res.Reason)`; the success envelope passes `locations()` to `StepEnvelope`.
    The existing messages and kinds stay byte-identical.
  - Rewrite the kind-disposition comment block above the `Kind*` constants: keep one meaning comment per constant (drop `KindProducer`'s "the one kind the supervisor skill may retry once" clause), state that the set is closed at five, and replace the one-retry rule and the per-kind "needs an operator decision" claims with one sentence pointing at the `ly-drive` skill as the single place a driver's disposition per kind is stated (card 13 rewrites `plugins/ly/skills/ly-drive/SKILL.md` to state them; name that path in the comment).
  - In `internal/shedverbs/seam_enforcement_test.go`, add `"github.com/Knatte18/loomyard/internal/logger": true` to `shedverbsAllowedImports`, with a comment: `logger` is admitted for step-boundary logging and for `logger.TraceFile`/`logger.TraceDir`, the only path sources admitted into this package, each returning the logger's own sink location verbatim; `shedverbsDeniedLyxcwdImport` and the `<module>cli` check stay unchanged.
  - Update callers of `StepEnvelope` in `internal/shedverbs/step_test.go` and `internal/loomcli/step_test.go` to pass a `StepLocations` value.
    Rename `TestStepEnvelope_KeySetIsExactlyTen` to `TestStepEnvelope_KeySetIsExactlyThirteen` in both files and extend both want-lists with `trace_file`, `friction_dir`, `scratch_dir`.
  - New tests in `internal/shedverbs/step_test.go`:
    `TestStepCmd_ErrorEnvelopesCarryLocations` (table over the `PreStep` error, nil `BuildShed`, `BuildShed` error, busy, and producer-error paths the existing tests already build, with `Spec.ScratchDir`/`Spec.FrictionDir` set: each envelope carries all three keys, `scratch_dir`/`friction_dir` echo the `Spec`, and `kind` is unchanged);
    `TestStepCmd_SuccessEnvelopeEchoesLocations` (stub row: `scratch_dir`/`friction_dir` echo the `Spec`, `trace_file` is `""` with no sink armed);
    `TestStepCmd_TraceFileHoldsBoundaryRecords` (sink armed per the overview's `trace-tests-use-sink-override`: success envelope's `trace_file` names an existing file containing `msg="shed: step"` and `msg="shed: step done"` with `producer=`; a `PreStep` error envelope's `trace_file` names a file containing `msg="shed: step refused"` with the kind).
- **Commit:** `feat(shedverbs): name the trace, friction and scratch locations on every step envelope`

### Card 6: generic status core carries history length, interrupt policy and trace dir

- **Context:**
  - `internal/logger/sink.go`
  - `internal/output/output.go`
  - `internal/shedengine/status.go`
  - `internal/shedverbs/testsupport_test.go`
  - `internal/loomcli/cli_test.go`
- **Edits:**
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/status_test.go`
  - `internal/loomcli/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `statusCmd`'s `RunE` in `internal/shedverbs/status.go`, every envelope carries `trace_dir` set to `logger.TraceDir()`:
    the lock-dir failure, the decode failure and the `StatusExtras` error move from `output.Err` to `output.ErrFields(out, msg, map[string]any{"trace_dir": …})` with their messages byte-identical;
    the absent-status refusal (`AbsentStatus.Refuse`) emits `output.ErrFields(out, RefuseMessage, map[string]any{"found": false, "trace_dir": …})`;
    the `found: false` success envelope gains `trace_dir`;
    and the found envelope's `core` gains `history_length` (`len(st.History)`), `interrupt_policy` (`spec.Hooks.InterruptPolicyFor(st.CurrentProducer)`, `""` when the hook is nil) and `trace_dir`.
    `StatusExtras` still merges onto the core afterwards.
  - Rewrite the `!found` branch's comment: `trace_dir` is the one generic key it carries, `StatusExtras` still never runs against a zero status, and `found: false` is the one discriminator both absent dispositions share.
    Update `Hooks.StatusExtras`'s doc comment in `internal/shedverbs/spec.go`: the generic core is now seven keys (name them), and the absent-file envelope carries only the generic `found`/`trace_dir` (plus `status_path` on the non-refusing disposition), never an extras key.
  - Tests in `internal/shedverbs/status_test.go`:
    `TestStatusCmd_AbsentFile_FoundFalse`'s exact key set gains `trace_dir`;
    `TestStatusCmd_AbsentFile_Refuse` additionally asserts `found == false` and a `trace_dir` key;
    new `TestStatusCmd_FoundEnvelope_GenericCore` (seeded status with two history entries and a `InterruptPolicyFor` hook returning `"reinvoke"`: `history_length == 2`, `interrupt_policy == "reinvoke"`, `trace_dir` present; with a nil hook, `interrupt_policy == ""`);
    new `TestStatusCmd_ErrorEnvelopesCarryTraceDirOnly` (the decode-failure and `StatusExtras`-error envelopes carry `trace_dir` and no `found` key);
    new `TestStatusCmd_TraceDirMatchesOverride` (with `logger.SetDurableSinkDir(dir)` set and reset per the overview's `trace-tests-use-sink-override`: found envelope's `trace_dir == dir` and `dir` holds no trace file afterwards).
  - In `internal/loomcli/status_test.go`'s `TestStatusCmd_EnvelopeKeySet`, add `"trace_dir"` to `wantKeys`.
- **Commit:** `feat(shedverbs): move history_length and interrupt_policy into the generic status core and add trace_dir`

### Card 7: record the envelope and import admissions in the Shed Verb-Set Invariant

- **Context:**
  - `internal/shedverbs/seam_enforcement_test.go`
  - `internal/shedverbs/spec.go`
- **Edits:**
  - `internal/shedverbs/doc.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `CONSTRAINTS.md`'s `## Shed Verb-Set Invariant`, amend the no-resolver bullet: `shedverbs` imports `internal/logger` for step-boundary logging, and `logger.TraceFile`/`logger.TraceDir` are the only path sources admitted into it;
    they are not derived paths in this invariant's sense, since each returns the logger's own sink location, which `shedverbs` reports verbatim on the envelope and never joins, reads, writes or uses to locate run state;
    every run-state path (`StatusPath`, `ScratchDir`, `FrictionDir`, …) stays told through `Spec`, and `shedverbs` still calls no `lyxcwd` function and the `lyxcwd` import stays denied.
    Amend the refusal-kind bullet: the five kinds stay closed, while the step envelope's key set (closed by doc comment and test, not by this invariant) carries `trace_file`, `friction_dir` and `scratch_dir` on the success and every error envelope, and the status envelope carries `trace_dir` on every envelope.
    Semantic line breaks; keep existing sentences not named here.
  - In `internal/shedverbs/doc.go`, add one sentence to the package comment: the one resolving import it admits is `internal/logger`, for boundary logging and the two sink-location accessors, per the Shed Verb-Set Invariant.
- **Commit:** `docs(constraints): admit logger into shedverbs and record the trace envelope keys`

### Card 8: loom fills ScratchDir and FrictionDir and drops moved status keys

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/status_test.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/loomcli/arm.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `(*loomCLI).specFor` in `internal/loomcli/arm.go`, set `FrictionDir: c.frictionDir` (already resolved in `wire` from `loomengine.LoomFrictionDir` only when `loom.yaml` enables friction, and left empty by `wireLightweight`) and, when `c.location != nil`, `ScratchDir: shedrun.ScratchDir(c.location, c.runID)`.
    The nil guard keeps the untagged tests that call `specFor` on a hand-populated receiver with no location working; add one comment line saying so.
  - In `(*loomCLI).loomStatusExtras`, drop the `history_length` and `interrupt_policy` keys (the generic core now supplies both) and update its doc comment from "five status keys" to its three remaining keys.
  - Add `TestSpecFor_ScratchAndFrictionDir` to `internal/loomcli/cli_test.go`: a receiver with `location: &lyxcwd.Location{…}` built from a `t.TempDir()` hub and `runID: "self"`:
    `specFor("step").ScratchDir == shedrun.ScratchDir(loc, "self")`;
    `FrictionDir` equals `c.frictionDir` when that is set to a non-empty path and is `""` when it is empty;
    and a receiver with a nil `location` yields `ScratchDir == ""` without panicking.
- **Commit:** `feat(loomcli): tell ScratchDir and FrictionDir to the generic verbs`

### Card 9: batten fills ScratchDir and drops history_length from its extras

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/battencli/paths.go`
  - `internal/battencli/run_test.go`
- **Edits:**
  - `internal/battencli/arm.go`
  - `internal/battencli/step_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `(*battenCLI).specFor` in `internal/battencli/arm.go`, set `ScratchDir: shedrun.ScratchDir(c.location, c.slug)` when `c.location != nil`, and leave `FrictionDir` empty with a one-line comment that batten carries no agent friction directory.
  - In `(*battenCLI).battenStatusExtras`, drop the `history_length` key (the generic core supplies it) and update its doc comment so it names the generic body as the source of `history_length` too; the `history_truncated` sentence keeps its meaning.
  - In `internal/battencli/step_test.go`, reword `TestBattenPreStep_SeedsAbsentStatus`'s doc comment so it no longer says `kind: "producer"` is "the one kind ly-drive retries": the reason stays that an unseeded first step must not surface as a producer failure the driver would repair round after round.
  - Add `TestSpecFor_ScratchDir` to `internal/battencli/step_test.go`, using `newFakeReceiver(t, nil)`: `specFor("step").ScratchDir == shedrun.ScratchDir(c.location, c.slug)` and `FrictionDir == ""`.
- **Commit:** `feat(battencli): tell ScratchDir to the generic verbs`

### Card 10: log the landing pull-request write at Info

- **Context:**
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/landingshed/publish.go`
  - `internal/landingshed/publish_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/landingshed/publish.go`, capture the `*github.PullRequest` `client.PullRequests.Create` returns and, on success, call `logger.Info("landingshed: pull request created", "owner", owner, "repo", repo, "number", created.GetNumber())` before the `stuckOrCancelled` return.
  - In `internal/landingshed/publish_test.go`, extend `TestPublish_NoExistingPR_CreatesAndReportsStuck` (or add a sibling `TestPublish_CreatedPRLogsInfo` over the same fake server): with the sink armed per the overview's `trace-tests-use-sink-override`, the trace file contains exactly one `msg="landingshed: pull request created"` line carrying `number=1` (the fake server's default create body).
- **Commit:** `feat(landingshed): log the pull request write to the durable trace`

## Batch Tests

`go build ./...` catches any other `StepEnvelope` caller.
The scoped `go test` covers every changed test file: `internal/shedverbs` (`TestStep*`, `TestStatus*`, the seam test `TestNoResolverNoModuleCLIInvariant*`), `internal/loomcli` (`TestStepEnvelope*`, `TestStatusCmd_EnvelopeKeySet`, `TestSpecFor*`, and `TestVerbRefusals` over `specFor` with a nil location), `internal/battencli` (`TestSpecFor*`, `TestBattenPreStep*`, `TestRun*` over `newFakeReceiver`), and `internal/landingshed` (`TestPublish*`).
The tagged runs are narrow on purpose: `TestBattenIntegration_RealReadStatus*`/`TestBattenIntegration_NonPrimeRefusal` drive batten's real status and step refusals through `RunCLIIn`; `TestPublish_MergesInCleanlyBeforeCreatingPullRequest` runs the real publish path around the new `Info` line; the two smoke cases run the built binary's `lyx loom status`/`pause` and `lyx loom step` against a real pair, the entry points whose envelopes this batch changes.
The remaining tagged tests in these packages (loom's tmux-driving smoke cases, batten's lifecycle runs, landing finalize) exercise no code this batch changes beyond envelope keys they do not pin, and run in the done gate.
Card 7 is docs-only.
The assertions on real entry points (`lyx shed step`, `lyx loom step`, `lyx batten step`) are batch 4's.
