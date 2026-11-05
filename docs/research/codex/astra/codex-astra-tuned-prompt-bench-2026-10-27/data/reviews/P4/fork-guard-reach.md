Guard reachability:

- Card 1: empty/invalid hostname refusal is reachable with `""` or `.local`; hostname-read failure is propagated.
- Card 3: changed-owner payload refusal and non-takeover owner refusal are reachable. The environment skip paths bypass the required publication guard.
- Cards 5–6: owner classification is reachable for locally recorded batten pairs; it is absent for an existing child run without a local prime-side batten status.
- Card 7: preserving an existing pause on the same scope is reachable.
- Card 8: start, resume, step and window refusals are reachable after their existing pause/state/lock checks.
- Card 9: start/resume owner refusals are reachable with an existing status and no covering pause. Foreground `loom run` and `loom step` remain unguarded entry points.
- Card 10: pause-release, owner-refusal, Finalize lock-retention and transient-restep branches are reachable. An exactly one-minute sleep does not reach the proposed check through the existing SleepWatch.
- Card 11: owner-envelope conversion and tender pause invocation are reachable.
- Card 12: run-owner, parent-owner, combined-owner and unreachable-origin halts are reachable with a valid summary and satisfied parent-lock checks. The past-merge exemption and nil-seam branches are reachable.
- Card 13: wake owner pauses are reachable. Orch owner pauses are reachable if `Command` assigns the installed seam during its existing module pre-run; the plan does not explicitly require an earlier constructor-time snapshot.
- Card 14: missing-pair owner reporting and owner-read failure are reachable.
- Card 15: detached-client reopening, disabled switch and listing-error branches are reachable.
- Card 16: invalid selection, reconcile stop, local-history stop, live-pair drop refusal, lost write, standing pause and owner-halt resumption branches are reachable.

Findings:

1. **BLOCKING · design · Cards 5 and 9 — Existing child runs without a local batten record escape the owner verdict.**  
   Card 5 explicitly asks the owner reader over `recorded`, while card 9 treats the absence of an owned-elsewhere entry as permission to resume. `recordedPairs` includes a slug only when the prime has its batten status file (`internal/fabricengine/originreconcilemoves.go:436`, especially the `found` check). `ReconcileOrigin` passes that filtered set to classification (`internal/fabricengine/originreconcile.go`, `ReconcileOrigin` and `classifyEntries`, read in inherited context). A second machine can add the task pair from origin, obtaining the child’s existing status without a local prime-side batten status; judge-only reconciliation fetches objects but does not materialize the missing prime record. `loom start` and `loom resume` require no batten record (`internal/loomcli/start.go:463`, `internal/loomcli/resume.go:230`, `internal/loomcli/arm.go:177`). Consequently their proposed guard sees no owner verdict and resumes another machine’s existing run, contrary to D3 and declared `PATTERN-owner-machine`. The requested run must participate in owner judgment independently of this local batten-record filter.

2. **BLOCKING · design · Card 9 — The same seeded run can be resumed through foreground run/step without the owner check.**  
   The card guards only `startCmd` and `resumeCmd`. The mounted foreground `lyx loom run` and `lyx loom step` execute `loomPreRun` and `loomPreStep`, respectively, and the generic shed routes reuse those hooks. `loomPreRun` explicitly describes a run-only invocation as a resume, checks status/seed identity, then brings up reed and drives the run; it performs no machine-owner check (`internal/loomcli/arm.go:385`). `loomPreStepUnmarked` similarly reaches bootstrap and execution after the local run-lock probe (`internal/loomcli/arm.go:483`). Hook wiring is in `internal/loomcli/arm.go:283`, and both verbs are mounted at `internal/loomcli/cli.go:510`. With an existing seed/status and a free local lock, another machine’s run therefore remains executable before Finalize. This violates the declared invariant covering starting/resuming a run. Any correction must preserve the explicitly decided in-process past-merge Finalize re-step exemption.

3. **BLOCKING · decision · Card 3 — Skip flags turn an unpublished owner write into a successful claim.**  
   Card 3 prescribes that `BOARD_SKIP_GIT` writes no commit or push, that `BOARD_SKIP_PUSH` performs no pull/push, and that either reports the written owners as recorded. These flags are ordinary production environment inputs, with no test-only restriction (`internal/boardengine/skipenv.go:13`). `Bolt.PushRecorded` returns success immediately for either skip (`internal/fabricengine/bolt.go:85`). Thus a start, owner-writing resume or adoption can receive `Recorded` without publishing its ownership on Bolt; this directly contradicts D3’s “An owner write needs its push” and D5’s publication-before-resumption ordering. Tests should use injected seams or an explicitly isolated test mechanism rather than production success semantics that bypass that prerequisite.

4. **LOW · consistency · Card 10 — The prescribed exactly-one-minute wake branch cannot fire through SleepWatch.**  
   The card requires a sleep “of a minute or more” to trigger one owner check, carrying D6’s “at least a minute.” The existing `SleepWatch.Observe` records a sleep only for `gap > SleepTolerance`, not equality (`internal/pauserecord/sleep.go:24`), and `TestSleepWatch` explicitly pins “excess at the tolerance is no sleep” (`internal/pauserecord/sleep_test.go:28`). Card 10 consumes this watch without changing its threshold, so its named one-minute input cannot reach the new guard. Reconcile the specified threshold and its existing contract.

Tool calls:

1. Read orch Command’s actual construction/pre-run region and additional loom bootstrap/resume code because their setup timing and upstream checks were missing from the inherited excerpts.
2. Read loom arming and the actual start/resume command entry points because the manifest’s excerpts did not establish whether a batten record is required.
3. Read ApplySkipEnv and generic loom seed/arming rules because production accessibility of skip flags and alternate entry prerequisites were not shown.
4. Read pushClassified, loom run/step hooks and network-gate execution because publication deferral and alternate execution paths needed their downstream behavior checked.
5. Read hook wiring, generic shed dispatch and SleepWatch’s boundary test because the alternate routes and exact threshold contract needed confirmation.
