MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
duration_s: 140.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Recipe step 4 seeds a run-id start never reads
**Section:** Reproduction recipe, step 4. **Issue:** `lyx loom start` reads and writes only `shedrun.SelfRunID` (`sharedbootstrap.go` `seedAndCommitBootstrap`, `arm.go` `resolveRunID`), and `WriteSeed` refuses a disagreeing seed against `loomSeedFor`'s `{"parent": <parent>}` params. `lyx shed seed <run-id> --recipe loom --driver llm` therefore either leaves `self` to default to the go driver, so the live gate passes without exercising the llm arm (the r3 failure mode), or, seeded at `self` without `--param parent=…`, gets refused at `bootstrapStageSeed`. **Fix:** Pin the invocation as `lyx shed seed self --recipe loom --driver llm --param parent=<recorded parent>`, and add a check that the spawned strand is the ly-drive driver (`driverStrandDisplayName`) rather than the detached go runner.

### [NIT:consistency] Tick-cap exit cannot route through classifyStartupWindow
**Section:** Result mapping, tick-count cap bullet. **Issue:** The cap exists for a clock that never advances, and in that case `classifyStartupWindow` returns `""`, not a verdict. Only `classifyDeadlineExpiry(OutcomeDied)` yields the answer. Also, a `return` whose expression names `OutcomeDied` adds an `AwaitStarted [OutcomeDied]` tripwire key that the Completion Signal tripwire decision does not list. **Fix:** Name `classifyDeadlineExpiry` as the cap's route, and say whether the tripwire gains an `[OutcomeDied]` pin or the comparison stays outside the return statement.

### [NIT:scope] `--no-attach` flag usage string is missing from the doc list
**Section:** Scope, doc updates. **Issue:** The `no-attach` flag's usage text in `start.go` ("return once the driver has taken the run lock") describes only the go arm's readiness signal, and the change redefines the llm arm's signal. **Fix:** Add the flag usage string to the doc-update list.

### [NIT:design] Bootstrap-lock analysis omits the driver's own step verb
**Section:** Readiness signal, bootstrap-lock decision. **Issue:** The loom step pre-run (`arm.go`, reached from `lyx shed step --recipe loom`) takes the same `LoomBootstrapLock` with blocking `AcquireWriteLock`. So the ly-drive driver's first step blocks until `start` finishes its widened probe. On a refusal, the step then runs against a strand that `start` has reported as not up. There is no deadlock, but the discussion's analysis covers only a concurrent `lyx loom start`. **Fix:** State this waiter and the refuse-then-proceed consequence explicitly.

### [NIT:decision] Dismissal log is left conditional on something already knowable
**Section:** Constraints, Live-Substrate Spawn Observability. **Issue:** `checkLivenessTick` logs a dismissal only on failure (`Warn`), and has no log on a successful play. The "only if mill-plan finds no existing log" condition is therefore already resolved. **Fix:** Decide the log line and its level now.

## Verdict

REQUEST_CHANGES
The live-verification recipe as written seeds the wrong run and would not exercise the llm arm.
MILL_REVIEW_END
