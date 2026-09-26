# Batch: step-entry-point-integration

```yaml
task: 'shed: the LLM driver as a generic stepper and mender'
batch: step-entry-point-integration
number: 4
cards: 1
verify: go test -tags integration ./internal/shedcli/ -run 'TestParity_'
depends-on: [3]
```

## Batch Scope

Proves on the real entry points that the envelope keys batch 3 added reach the operator: `lyx shed step`, `lyx loom step` and `lyx batten step` each emit `scratch_dir` equal to `shedrun.ScratchDir` of the addressed run, and a real `lyx shed step` names a `trace_file` that holds its boundary records.
It extends `internal/shedcli/parity_test.go`, which already drives both invocation paths of `step` in-process through `RunCLIIn` and compares them byte for byte.
One card, since it is one test file.

## Cards

### Card 11: assert scratch_dir and trace_file on every step entry point

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/battencli/paths.go`
  - `internal/logger/sink.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/shedcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add an unexported helper `decodeEnvelope(t, out string) map[string]any` that decodes the last non-empty line of `out` as JSON.
  - In `TestParity_LoomStep_RunLockBusy`, before `runBoth`, arm the sink with `logger.SetDurableSinkDir(traceDir)` on a `t.TempDir()` and register the `t.Cleanup` reset per the overview's `trace-tests-use-sink-override` decision (both invocations run in this one process and so share one trace file, which keeps the byte-identical comparison valid).
    Capture both outputs (for example by having the two closures store `out.String()` into locals) and, for each decoded envelope, assert: `kind == "busy"`; `scratch_dir == shedrun.ScratchDir(location, shedrun.SelfRunID)`; `trace_file` is non-empty, lies inside `traceDir`, exists, and its content contains `msg="shed: step"` and a `msg="shed: step refused"` line carrying `kind=busy`.
  - Add `TestParity_BattenStep_RunLockBusy`, modelled on `TestParity_BattenRun_StateDone` and `TestParity_LoomStep_RunLockBusy`: a hubforge hub, cwd `h.PrimeWorktree()`, a batten seed at a fresh slug via `seedRunForTest`, the run lock at `battencli.RunLock(h.Location, slug)` held with `lock.AcquireWriteLock` after `MkdirAll` of its parent, then `runBoth` over `battencli.RunCLIIn(cwd, &out, []string{"step", slug})` and `RunCLIIn(cwd, &out, []string{"step", slug})`.
    For each decoded envelope assert `kind == "busy"`, `scratch_dir == shedrun.ScratchDir(h.Location, slug)`, and `friction_dir == ""`.
  - Both tests stay in this `integration`-tagged file (Test Tier Purity).
- **Commit:** `test(shedcli): assert scratch_dir and trace_file on every step entry point`

## Batch Tests

`verify:` runs every `TestParity_*` case in `internal/shedcli/parity_test.go`: card 11's new assertions plus the existing byte-identical comparisons, which now also prove the new keys (`trace_file`, `scratch_dir`, `friction_dir`, and `trace_dir` on the status cases) are identical across the module and shed paths.
