Guard reachability:

- Cards 1–3: empty-machine and changed-owner payload refusals are reachable; the task-owner guard is reachable after the setter’s pull. The prime claim lacks the corresponding post-pull guard.
- Cards 5–6: owner classification, replay suppression and nil-reader behavior are reachable through the reconcile and rejected-push paths.
- Cards 8–9: start, resume, window and shed owner refusals are reachable with an existing entry/status and no covering pause. A covering pause intentionally refuses earlier.
- Cards 10–11: sleep, hold-end, owner-labelled pause, spawn-refusal and Finalize lock-exemption branches are reachable with the states the card names.
- Card 12: owner checks are reachable after the parent guard passes. An unreachable records origin can reach the prescribed unreachable report even when the earlier code-parent fetch succeeds; a code-origin fetch failure is intercepted earlier.
- Card 13: wake owner pauses are reachable without a shared-branch stop. The orch pause depends on resolving the installed seam after installation.
- Card 15: window reopening is reachable on a resumed status past Open-Window with no attached client.
- Card 16: invalid selection, local-commit stop, live-pair refusal, push refusal, lost-write and owner-halted-child branches have reachable inputs. The owner halt prefix survives shed persistence unchanged.

Findings:

1. **MEDIUM — consistency — `_lyx/plan/03-board-owner-writes.md`, SetOwners Intent.**
   **First-act prime claims can overwrite an owner discovered by the setter’s pull.**
   The card unconditionally writes `prime_owner` when `Prime` is set, while its non-`Takeover` protection explicitly covers slugs only.
   Card 8 calls `claimPrimeOwner` before resume reconciliation.
   A machine with an old local Bolt lacking `prime_owner` therefore requests the claim; `PullThenCommitWritten` first fetches the other machine’s already-recorded claim, then runs the write on top of it.
   This is an ordinary fast-forward push, so origin-wins replay does not rescue the other claim.
   D2 requires the first-act loser to read the other claim; D5 reserves takeover for adopt.
   Require the non-takeover prime claim to recheck `prime_owner` under the setter’s post-pull write lock and report the existing other owner.
   **Evidence:** `internal/fabricengine/bolt.go:145`, particularly the pull at line 154 and write callback at line 164; cards 3 and 8’s inherited Intent text.

2. **MEDIUM — consistency — `_lyx/plan/13-wake-and-orch-owner-pause.md`, InstallOwnerPause wiring.**
   **Copying the owner-pause seam during Command construction captures nil before the root pre-run installs it.**
   The card prescribes installation in `newRoot`’s pre-run and copying the installed function into `orchCLI` in `Command`.
   The root constructs `orchcli.Command()` while assembling the command tree, before any pre-run executes.
   Following that construction-time copy leaves a fresh production invocation on the explicitly allowed “unwired pause” warning path, so `orch start` launches without D4’s owner pause.
   Resolve the installed function in the orch pre-run or at invocation time, or install it before command construction.
   **Evidence:** `cmd/lyx/main.go:119` defines the execution-time pre-run; `cmd/lyx/main.go:147` constructs the command tree; `internal/orchcli/cli.go:101` constructs the receiver and command.

Additional reads:

1. `landingshed/parentguard.go`, production `ReadParentTips` wiring/implementation, and `orchcli/start.go`: upstream guards and launch sequence were absent from the manifest excerpts.
2. `holdParent`/`rerouteIfMoved`, Finalize test construction, and the parent-lock harness: established which upstream guards real calls and fake tests exercise.
3. `tendercli/gate.go`, InnerRun pause wiring, resume/status call sites, and `originTargets`: checked pause exemptions and run-scope entry prerequisites.
4. Shed blocked-state persistence and `stuckVerdict`: checked whether the owner-halt discriminator survives into adopt’s input.
5. Bolt pull-before-write implementation/tests, notice call sites, pause test rows and Finalize reconcile tests: checked first-act claim ordering and refusal fixtures.
6. InnerRun.Call, batten loop configuration references and CLI execution references: checked respawn entry ordering and command construction versus execution.
