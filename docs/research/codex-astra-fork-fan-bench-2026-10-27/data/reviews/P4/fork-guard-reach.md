Guard reachability list:

- Card 1 hostname normalization: empty and dotted-empty names reach `FromHostname`’s error; `Name` propagates the OS error.
- Cards 2–3 owner records and payload rules: effective-owner, note normalization, payload no-op/refusal and task takeover branches are reachable. The prime claim has a separate unguarded overwrite path (finding 1).
- Card 5 reconcile classification: ended/owned-elsewhere/stopped precedence and nil-reader branches are reachable. The rejected-push owner guard is reachable after a non-fast-forward rejection; it does not govern an ordinary successful push.
- Card 6 seam wiring: production hub callers obtain the owner reader; explicitly nil test seams remain reachable.
- Card 7 pause labeling: both insertion and already-paused branches are reachable; the existing scope check leaves an earlier entry standing.
- Card 8 batten entry checks: fresh start, existing-run resume, pre-pair Worktree-Create recovery, step and window paths reach their proposed checks. Offline resume can reach local-owner fallback. A stale missing prime owner can trigger finding 1 before reconcile.
- Card 9 loom start/resume: an existing status with no covering pause reaches the owner refusal before bootstrap. An existing covering pause refuses earlier, as designed. Absent-status bootstrap and nil test seams bypass the owner reader explicitly.
- Cards 10–11 InnerRun: sleep, hold-end, spawn-error and owner-labeled coverage branches have reachable inputs. The held run-lock fixture suffices to make `LivePair` true; `probeQuiet` checks that lock.
- Card 12 Finalize: run-owner and parent-owner checks are reachable after the parent guard succeeds. A fully unreachable origin is intercepted by that earlier guard, defeating the prescribed past-merge continuation (finding 2).
- Card 13 wake/orch: wake report mapping and pause labeling are reachable. The prescribed orch callback capture occurs before installation, so production orch start reaches the unwired branch instead of pausing (finding 3).
- Card 14 status: owner results, nil seam and error-part branches are reachable.
- Card 15 window reopen: an existing resumed status beyond Open-Window reaches the attachment/switch decision.
- Card 16 adopt: normal preflight, local-commit, live-pair, drop-local, push-loss and owner-halt selection branches are reachable with suitable inputs. Under a standing hub pause, the upstream network gate prevents reaching the advertised adoption/standing-pause behavior (finding 4).
- Card 17 contains no executable guard.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/03-board-owner-writes.md`, SetOwners Intent; card 8 resume claim.**  
   **A first-act prime claim can overwrite an existing remote prime owner without `Takeover`.** Card 3 expressly protects slugs after the pull, but says to set the prime owner whenever `Prime` is set. Card 8 claims a missing local prime owner before the resume reconcile. A clone whose local Bolt predates another machine’s prime claim therefore requests `Prime`; `PullThenCommitWritten` first imports the other machine’s claim, then the prescribed callback overwrites it. No competing push or replay loss is necessary. This bypasses D2’s first-claim behavior and the plan’s claim that only adopt moves an existing owner. The prime decision must be checked inside the post-pull locked write, retaining an already-recorded prime owner unless takeover was requested.  
   Evidence: `_lyx/plan/03-board-owner-writes.md:1` (SetOwners/Takeover paragraphs), `_lyx/plan/08-batten-owner-check.md:1` (resume paragraph); `internal/fabricengine/bolt.go:145` calls `pullFirst` before `write.Write`, and `bolt.go:201` fast-forwards the current board before that callback.

2. **MEDIUM — consistency — `_lyx/plan/12-finalize-owner-checks.md`, replacement of step 1b3.**  
   **The past-merge unreachable-origin carve-out is still blocked by step 1b2’s parent-tip fetch.** The card places `ownerChecks` after the existing `holdParent`. With normal pushes enabled, `holdParent` calls `parentGuard.moved`, which fetches parent tips and returns its error immediately if origin is unreachable. Consequently an in-process Finalize re-step after its local parent merge never reaches `ParentHoldsTaskWork`, the carve-out or the parent push that D7 says should record the failure. The planned fake-reconcile rows do not exercise this upstream production guard.  
   Evidence: `internal/landingshed/finalize.go:199` onward places `holdParent` before step 1b3; `internal/landingshed/stuck.go:103` calls `rerouteIfMoved`; `internal/landingshed/parentguard.go:124` onward calls `g.readParentTips(!g.pushSkipped)` and returns its error; `internal/fabricengine/parenttips.go:44` fetches and returns a fetch failure before reading local tips.

3. **BLOCKING — consistency — `_lyx/plan/13-wake-and-orch-owner-pause.md`, InstallOwnerPause wiring.**  
   **Orch captures the owner-pause callback before the root installs it.** Card 13 says `Command` copies the installed callback onto `orchCLI.pauseOwned`, while root `PersistentPreRunE` installs it. `newRoot` builds `orchcli.Command()` while constructing the command tree; pre-run executes only later. In a fresh production process the captured callback is nil, so every owned-elsewhere run takes the prescribed “unwired pause” warning and remains unpaused. Fake `pauseOwned` rows do not cover this ordering. Resolve the installed callback at execution time or install it before building the subtree, with a test of the real construction/install sequence.  
   Evidence: `_lyx/plan/13-wake-and-orch-owner-pause.md:1` (InstallOwnerPause and Command paragraphs); `cmd/lyx/main.go:101` (`newRoot`, root pre-run versus `root.AddCommand(orchcli.Command())`); `internal/orchcli/cli.go:115` (`Command` constructs its receiver).

4. **MEDIUM — consistency — `_lyx/plan/16-adopt-module.md`, hub-pause branch.**  
   **Adopt cannot reach its promised behavior while a hub pause remains standing.** Card 16 explicitly preserves a hub pause, reports its lift after adoption, and describes `--drop-local` operating on hub-paused pairs. Yet adopt runs its remote reconcile and owner push before that reporting, with no network exemption. Every production root process installs the pause network gate; from the prime a hub pause covers these operations. The first remote call waits for the pause to lift, so the named hub-pause branch cannot complete while preserving that pause. The plan must settle this interaction explicitly and exercise the production network gate rather than only fake reconcile/Lift seams.  
   Evidence: `_lyx/plan/16-adopt-module.md:1` (steps 1, 3, 4 and Bound); `cmd/lyx/main.go:101` installs the network gate in pre-run; `internal/tendercli/gate.go:21`–29 defines it as holding this process’s network calls while covered. For comparison, `internal/hibernatecli/deps.go:103` explicitly installs an exemption for wake reconciliation.

Tool-call log:

1. Read Bolt’s pull-before-write/push contracts, searched additional Pull callers, and read the local replay path; inherited context lacked the setter’s underlying ordering and replay caller boundary.
2. Read landing `stuck.go` and searched parent/pair guard definitions; inherited context lacked the upstream implementation reached by `holdParent`.
3. Located network-gate and quiet-probe files and additional production Pull callers; those process/guard definitions were absent from the manifest.
4. Read `parentGuard`, `probeQuiet`, fabric pull CLI and initial classified-push dispatch; these established upstream fetch ordering, live-fixture reachability and successful-push versus retry dispatch.
5. Read network-gate installation, parent-tip fetching and Bolt’s `pullFirst`; these established the actual remote operations preceding the planned branches.
6. Located pause-record files and searched explicit network exemptions; the manifest contained wake reconciliation but not its installed exemption.
