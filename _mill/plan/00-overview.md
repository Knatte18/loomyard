# Plan: Shuttle guarantees a started run is past its startup gates

```yaml
task: Shuttle guarantees a started run is past its startup gates
slug: shuttle-start-guarantees-readiness
approved: true
started: 20260923-120131
parent: main
root: ""
verify: null
discussion_sha: e1a275790c6d6e34a70cb9a17200119f5e4a108d
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shuttle-blocking-start
    file: 01-shuttle-blocking-start.md
    depends-on: []
    verify: go test ./internal/shuttleengine/... ./internal/loomcli/... ./internal/shuttlecli/... ./internal/shedadapters/... && go test -tags integration ./internal/loomcli/ && go test -tags smoke -run 'TestSmokeDriverStrand|TestSmokeGate|TestSmokeBurlerRound|TestSmokeSingleLLM' ./internal/loomcli/
  - number: 2
    name: webster-startup-window-docs
    file: 02-webster-startup-window-docs.md
    depends-on: [1]
    verify: go test ./internal/websterengine/ ./internal/webstercli/ && go test -tags integration ./internal/websterengine/ ./internal/webstercli/ && go test -tags smoke -run '^$' ./internal/webstercli/
```

## Shared Decisions

### Decision: every card commit compiles

- **Decision:** cards are ordered so no intermediate commit breaks the build.
  Loom stops calling `Run.AwaitStarted` (card 3) before shuttle deletes it (card 4), and the `Run.attached` field is removed only after the startup step no longer reads it (card 5).
  Between card 3 and card 4 loom's llm arm briefly does not await readiness; that intermediate state never reaches `main`, because the task lands as one squash-merge.
- **Rationale:** each card produces its own commit and a builder verifies at batch end; a commit that does not compile makes bisecting and per-card review harder for no gain.
- **Applies to:** shuttle-blocking-start

### Decision: startup step shape and names

- **Decision:** the startup step is one private method `(run *Run) awaitStartup() (Result, error)` in `internal/shuttleengine/wait.go`, with its not-ready teardown in a second private method `(run *Run) abandonStartup(outcome Outcome) (Result, error)` in the same file.
  `awaitStartup` returns `(Result{}, nil)` when the handle may be issued (provider reached `StartupReady` with `Started` persisted, or the file contract is satisfied);
  `(result, err)` with `errors.Is(err, ErrNotStarted)` and `result.Outcome` `died`/`timeout` on a not-ready resolution (teardown already done);
  `(run.identity(), err)` without `ErrNotStarted` on a startup mechanism failure.
  `internal/shuttleengine/run.go` gains a private `(r *Runner) start(spec Spec, gate GateSpec) (*Run, Result, error)` holding `StartGated`'s old body plus the `awaitStartup` call; `StartGated` returns `(nil, err)` for any error, and `RunGated` returns `(result, nil)` when `errors.Is(err, ErrNotStarted)` and `(result, err)` for any other error.
  The exported sentinel is `ErrNotStarted`, declared in `wait.go`.
  `awaitStartedTickCap` is renamed `startupTickCap`.
  The startup-capture file name constant is `startupCaptureFileName = "startup-capture.txt"` in `run.go`'s artifact-name `const` block.
  The last successful startup capture is carried on a new `Run` field `lastStartupCapture string`, set by `checkLivenessTick` after every successful `CapturePane`.
- **Rationale:** the discussion leaves the finalize-reuse and capture-threading choices to the plan.
  `abandonStartup` reuses `finalize(outcome, "")` for the Outcome write and the "run finished" log line: for a non-`OutcomeDone` outcome `finalize` never evaluates the gate, never audits forks and never cleans up, which is exactly the not-ready contract, so no parallel finalize path exists to drift.
  Every negative-verdict construction stays in `wait.go`, the file the completion-signal tripwire scans (discussion decision "Startup step lives in a tripwire-scanned file"); `run.go` only branches on `errors.Is` and passes values through.
- **Applies to:** shuttle-blocking-start

### Decision: tripwire marker for the new sentinel

- **Decision:** add `ErrNotStarted` to `negativeVerdictMarkers` in `internal/shuttleengine/completionsignal_enforcement_test.go`, beside `errStrandNotTracked`/`errStrandPaneBindingCleared`.
- **Rationale:** `ErrNotStarted` is a new spelling of a negative verdict; listing it makes any future return of it trip the tripwire and force a human look, the same reason the two existing sentinels are listed.
- **Applies to:** shuttle-blocking-start

### Decision: Go verify commands, scoped by package

- **Decision:** verify commands use `go test` directly (no `PYTHONPATH=` prefix — this is a Go repo) and name only the packages each batch touches or whose behavior depends on the change, plus the tagged tiers of touched packages that run hermetically or against a local stub.
  `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) already covers the repo-wide regression check at handoff.
- **Rationale:** measured on this worktree: the untagged set runs in about 9 s, the integration set about 11 s, and the four loomcli smoke tests batch 1 selects about 20 s, against stub launches and real tmux.
- **Applies to:** all batches

### Decision: webstercli smoke tier is not run

- **Decision:** batch 2 touches `internal/webstercli/recoverbatch.go` (help text and header comment only) and compiles, but does not execute, the smoke tier: its verify runs `go test -tags smoke -run '^$' ./internal/webstercli/`.
- **Rationale:** that tier launches a real logged-in headless `claude` (see `internal/webstercli/smoke_test.go`'s file comment), which CLAUDE.md's "Agent execution: interactive tmux, never `claude -p`" rule and billing make unsuitable for a per-round verify, and it already fails on this worktree's baseline for reasons unrelated to this task.
  None of its tests reach `recover-batch`'s spawn path, so the batch's text-only edit cannot change their outcome.
- **Applies to:** webster-startup-window-docs

## All Files Touched

- `CONSTRAINTS.md`
- `contracts/stencils/webster/webster-template-master.md`
- `docs/overview.md`
- `docs/reference/claude-trust-dialog-repro.md`
- `internal/loomcli/driverlaunch.go`
- `internal/loomcli/driverspec.go`
- `internal/loomcli/smoke_driverstrand_test.go`
- `internal/loomcli/start.go`
- `internal/loomcli/start_driver_test.go`
- `internal/shuttleengine/attach.go`
- `internal/shuttleengine/attach_test.go`
- `internal/shuttleengine/completionsignal_enforcement_test.go`
- `internal/shuttleengine/doc.go`
- `internal/shuttleengine/fakes_test.go`
- `internal/shuttleengine/run.go`
- `internal/shuttleengine/run_test.go`
- `internal/shuttleengine/rundir.go`
- `internal/shuttleengine/startup_test.go`
- `internal/shuttleengine/wait.go`
- `internal/shuttleengine/wait_test.go`
- `internal/webstercli/recoverbatch.go`
- `internal/websterengine/awaitbatch.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/recoverbatch.go`
- `internal/websterengine/recoverbatch_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/state.go`
- `internal/websterengine/strand.go`
