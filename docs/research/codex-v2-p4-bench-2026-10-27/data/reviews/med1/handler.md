---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/13-wake-and-orch-owner-pause.md:36
    summary: Orch captures the owner-pause callback before the root installs it
    fix: In card 13, change the Command wiring intent to read the installed callback during the module pre-run or through an invocation-time lookup, after the root pre-run installs it. Extend the already-targeted TestStart_ReconcilesTheHubFirst coverage to construct the command before installation and invoke it after installation, asserting that the installed pause callback runs for an owned-elsewhere entry.
  - id: F2
    severity: BLOCKING
    class: consistency
    origin: handler
    location: _lyx/plan/03-board-owner-writes.md:51
    summary: The owner-setter retry test waits through a production duration without an injectable wait
    fix: In card 3, specify a request-scoped injectable wait or retry duration for SetOwners, defaulting to the production one-second retry. Specify that TestSetOwners supplies an immediate fake wait while retaining the lock across both push attempts and asserts the retry and refusal without waiting a real second. Do not use a package-global timing variable.
  - id: F3
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/08-batten-owner-check.md:50
    summary: The missing-prime claim precedes and defeats the permitted offline legacy-run resume
    fix: In card 8, explicitly obtain the resume reconcile result and handle its unreachable-local-owner fallback before attempting either a prime-owner or task-owner write. Extend the already-planned TestRunCmd_ResumeDispositions table with an unreachable report, no prime owner, and an unowned run-held task, asserting a warning and resume with no setter call.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/10-innerrun-owner-check.md:40
    summary: Lifting the completed old owner's pause schedules a check that can reinstate that pause
    fix: In card 10, specify the narrow D6 continuation for a successfully completed local landing after its owner pause is explicitly lifted. An owned-elsewhere verdict must not reinstall the owner pause and prevent the completed child from finishing its done wait and proceeding to Land; preserve holds from other standing pauses and checks for unfinished children. Strengthen the planned done-child row in TestInnerRun_OwnerCheck to keep returning owned_elsewhere after coverage clears and to keep the driver alive within its exit grace, then assert eventual Done without another owner pause.
  - id: F5
    severity: MEDIUM
    class: scope
    origin: lens:way-forward
    location: _lyx/plan/16-adopt-module.md:64
    summary: Adopt cannot resume an owner-halted LLM child on a second clone with no driver strand
    fix: In card 16, make the owner-halt recovery distinguish a parked local driver from an absent LLM driver. After ownership is successfully recorded and standing pauses permit continuation, use the existing loom start bootstrap for an owner-halted LLM child with no local driver, retaining loom resume for a usable parked driver and the existing Go-driver resume path. Extend the planned second-clone adoption coverage with an LLM-seeded blocked Finalize owner halt and no local reed driver, asserting that adoption starts its continuation; retain the assertion that unrelated halts are left alone.
---

### [BLOCKING] Orch's owner pause remains unwired in a fresh process

**Location:** `_lyx/plan/13-wake-and-orch-owner-pause.md:36`

**Issue:** Card 13 installs the process callback in the root pre-run but copies it into the receiver during `orchcli.Command` construction.
`newRoot` constructs `orchcli.Command()` at `cmd/lyx/main.go:161`, before the runtime pre-run declared at line 118 executes.
`internal/orchcli/cli.go:111` constructs the receiver at command-construction time.
Copying the initial nil function leaves that receiver nil even after the global slot is populated.
Consequently an actual `lyx orch start` warns that the pause is unwired and launches without pausing the owned-elsewhere run; rerunning the command repeats the same lifecycle.
This defeats D4's required wake/start behavior and the declared `PATTERN-owner-machine` constraint.
The planned fake-receiver cases exercise callback outcomes but do not catch this construction-order failure.
The affected-set lens and the handler independently identified the same defect; it is recorded once.

### [BLOCKING] The retry test needs an injected wait

**Location:** `_lyx/plan/03-board-owner-writes.md:51`

**Issue:** The card explicitly prescribes a one-second deferred-push retry and, at line 58, a real integration test keeping the push lock held across that retry.
Its declared request fields and setter design provide no timing injection, so implementing that test as prescribed waits through the production interval.
`pattern/PATTERN-test-speed.md`, under “Time is injectable,” requires a struct field or parameter for a production duration a test would otherwise wait out and prohibits waiting out that duration in any tier.
The existing `Bolt.PushRecorded` contract at `internal/fabricengine/bolt.go:77–89` returns nil on a deferred push; the new setter's own retry is therefore the behavior whose wait must be injectable.

### [MEDIUM] Offline fallback must precede ownership writes

**Location:** `_lyx/plan/08-batten-owner-check.md:50`

**Issue:** The resume paragraph first claims the prime when none is recorded, but its subsequent unreachable-origin paragraph promises that a run-held task with no recorded owner warns and resumes without an owner write.
On a legacy hub with neither ownership record, the first action enters card 3's pull-first setter and returns `ErrOwnerOriginUnreachable` before the permitted fallback can run.
D3 expressly allows this offline unowned-run resume at `_lyx/discussion/decision-record.md:68–72`.
The existing implementation obtains the reconcile report before branching on `Unreachable` in `internal/battencli/resumereconcile.go:39–54`; the plan must preserve that ordering relative to both new claims.

### [MEDIUM] The post-landing pause lift can immediately undo itself

**Location:** `_lyx/plan/10-innerrun-owner-check.md:40`

**Issue:** Card 10 sets `ownerDue` whenever coverage changes from covered to uncovered and answers an owned-elsewhere result by pausing the slug again.
Its promise that `lyx resume <slug>` frees the completed old-machine landing to reach Land lacks an exception to those rules.
The reconcile cannot be assumed to return ended here: `finalizeEnteredRead` accepts a Finalize history row at `internal/fabricengine/replay.go:259`, and `readOriginEnded` filters those entries through `withoutFinalizeEntered` at `internal/fabricengine/originreconcilemoves.go:528`.
The entry can therefore still return owned_elsewhere after a successful old-machine landing.
The Done arm can remain active while the driver writes its stop report (`internal/battenshed/innerrun.go:1004–1057`), and `runArm` suppresses that arm under renewed coverage (`internal/battenshed/innerrun_pause.go:38–43`).
Thus the explicit lift can reinstall the pause and strand the completion that D6 requires it to release.
The planned test must preserve the other owner's verdict instead of silently switching ownership when it clears coverage.

### [MEDIUM] A second clone has no LLM driver for loom resume to wake

**Location:** `_lyx/plan/16-adopt-module.md:64`

**Issue:** Card 16 starts batten and invokes `lyx loom resume` for an owner-halted child, including a run adopted onto the parent's machine.
For a blocked LLM-seeded run without a local driver, `loom resume` refuses; only its Go-seeded branch starts a missing driver (`internal/loomcli/resume.go:114–142`).
Its explicit way forward is `lyx loom start` (`internal/loomcli/resume.go:216–226`).
Starting batten does not supply the missing driver: InnerRun spawns an absent child or an unconfirmed running child, not a blocked child (`internal/battenshed/innerrun.go:385–405`), and its halted-arm revival returns a start fallback for `ChildDriverNone` rather than creating a strand (lines 787–801 and 848–859).
The fresh clone therefore records ownership successfully but leaves this valid owner-halted run blocked, contrary to D5's required adoption-and-continuation outcome.
The fake ResumeChild case and current second-clone test do not exercise this combination.

## Rejected

- No forward-reference finding was established for the focused owner symbols: owner records precede the setter, the owner reader precedes its installation, the pause label precedes its consumers, and PauseOwnedElsewhere precedes InstallOwnerPause.
- The Finalize past-merge exception and nil-seam defaults are explicitly described and bounded in the relevant intents; their existence alone is not a defect.
- Card 17's shortened PATTERN entry is licensed by the added-after-Discussion decision, with the complete declared rule retained verbatim in its background file.
- New owner-specific assertions are not duplicates merely because an existing test already exercises the surrounding status, pause, or adoption harness.

## Review limits

This review used read-only source inspection; no tests, board commands, or plan-validation commands were executed under the step's read-only tool-use rule.
The findings' proposed changes alter card intents and their already-planned coverage, not target lists or verify commands, so no target overlay is proposed.
