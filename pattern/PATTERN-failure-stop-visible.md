# PATTERN-failure-stop-visible

When a `--until-stop` loop stops on a failure and no other holder has the run lock, the status file is not `running` by the time the invocation that waited for the loop returns.

- It covers a loop that died without an envelope, a child that ended in an error and an arming refusal of the `--until-stop` invocation itself, which `ReportLoopArmError` reports.
  A dead loop is reported by the waiter's dead-loop stop, which writes `failed` through `shedengine.WriteFailedStop` before it prints.
- `lyx loom resume` therefore never answers "already running" for such a stopped run: the run reads `failed`, and resuming it is an ordinary re-step.
- No failure waits for the quiet notice, since the status file already says the run stopped.
- A stop that finds another holder of the run lock writes nothing and reports `busy`, and the next invocation retries it.
- The Go driver and a bare `step` stay outside it: they do not run the loop, and a bare `step` reports its own failure on its envelope.

## Enforcement

`TestLoop_FailureStopLeavesStatusFailedUnlessBusy` for a failing child, `TestUntilStop_DeadLoopStopsLoopExited` for a loop that is gone, and `TestLoopIntegration_ArmingRefusalStopsAsBootstrap` for an arming refusal.
