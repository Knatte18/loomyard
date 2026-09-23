# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Completion Signal tripwire deferred on a false premise
**Section:** `### Completion Signal tripwire`
**Issue:** Both branches are answerable from source, and the second is wrong: `completionSignalScannedFiles` is `{"wait.go", "attach.go"}`, so a new `startup.go` sits outside the scan entirely, and "no negative-verdict return site, since it returns a bool" does not hold — `Errorf` is a `negativeVerdictMarkers` entry, and the Result-mapping decision returns exactly that at the retry cap. The scanned-files var's own comment states the convention ("a future file that grows a third kind of verdict belongs in this list").
**Suggested fix:** Decide here rather than deferring — either add `startup.go` to `completionSignalScannedFiles` with an audited entry justifying the `Errorf` return, or put `AwaitStarted` in `wait.go`, where the scan already reaches it.

### [BLOCKING:design] Bootstrap lock is held across the widened probe
**Section:** `### Readiness signal becomes "provider TUI ready", not "pane live"`
**Issue:** `runDriverSpawnAndWait` holds `bootstrapLock` across the step-6 probe and releases it only on refusal or at step 7, and `start.go` acquires that lock through `lock.AcquireWriteLock`, which blocks with no timeout. Replacing the 50×100ms budget with `startup_timeout_s` (90 in `template.yaml`) therefore stretches the worst-case hold on a blocking, unbounded lock by roughly eighteen times. The discussion weighs the change only as caller latency.
**Suggested fix:** State the lock consequence and decide — accept the longer hold as correct serialisation of concurrent bootstraps, or release `bootstrapLock` before the probe and re-acquire after.

### [NIT:consistency] Stated cost describes only the success path
**Section:** `### Readiness signal becomes "provider TUI ready", not "pane live"`
**Issue:** "That can take a few seconds longer than today" sits in the same decision that adopts `startup_timeout_s` as the budget; on the refusal path that is up to 90s against today's 5s ceiling, so the sentence holds for a healthy boot only.
**Suggested fix:** Separate the two — a few seconds on a healthy boot, up to `startup_timeout_s` before a refusal.

## Verdict

REQUEST_CHANGES
Two decisions deferred or mis-premised; the mechanism, scope, constraints and repro recipe are otherwise sound.
