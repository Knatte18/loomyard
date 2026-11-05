- Machine-name empty/error refusal: reachable.
- Board owner-payload no-op/refusal: reachable through upsert, batch upsert and merge.
- `SetOwners` empty-machine, transport, divergent-pull, deferred-push and slug-takeover refusals: reachable; pair-parent inheritance is not wired into the setter, and the prime write lacks the corresponding no-takeover guard.
- Reconcile owner classification and replay guard: reachable for pairs with prime-side batten records; omitted for direct loom/darn runs without those records.
- Push-retry owner guard: reachable after rejection; bypassed by an explicit pull followed by a successful fast-forward push.
- Owner-pause coverage and lock-release branches: reachable.
- Batten start, resume, step and window guards: reachable; the step guard also prevents the required post-Finalize Land continuation.
- Loom start/resume owner guard: reachable for recorded batten pairs; ineffective for an existing child run without a prime-side batten record.
- InnerRun sleep, coverage-clear, error-retry and spawn-refusal branches: reachable; “done child clears its pause and ends Done” does not hold while the owner remains elsewhere.
- Finalize run-owner and parent-owner guards: reachable with origin available; the intended unreachable-origin halt and post-merge exemption are preempted when the earlier parent-tip fetch fails.
- Wake/orch owner-pause branches: reachable.
- Window-already-attached refusal: reachable.
- Adopt unknown/note/done, hub-pause, local-liveness, push-failure and owner-halt-resume branches: reachable.

**BLOCKING — consistency — `_lyx/plan/05-reconcile-owner-outcome.md:42` — An explicit pull bypasses the guarded push retry.**

Card 5 leaves `Fabric.Pull` unchanged on the stated assumption that every later push meets `retryWeftPush`.
A real sequence is: the adopter advances the weft; the old machine has local commits; the old machine runs `lyx fabric pull`; the existing pull replays those commits onto the fetched tip; its subsequent push is now a fast-forward and succeeds without invoking the owner guard.
Thus the same replay the card guards remains reachable through an unguarded route, violating the promised refusal of old-machine replay after handover.
Evidence: `internal/fabricengine/pull.go:369` reads only the ended set, `internal/fabricengine/pull.go:379` calls `replayWeft`, and `internal/fabricengine/pushclassified.go:118–126` returns immediately on successful push and invokes the retry only after rejection.
The guard must cover this replay route, or cover successful pushes carrying its result.

**BLOCKING — consistency — `_lyx/plan/05-reconcile-owner-outcome.md:36` and `_lyx/plan/09-loom-owner-check.md:32` — Existing direct loom/darn runs can evade the ownership verdict.**

Card 5 asks the owner reader only about “recorded pairs.”
That existing helper means pairs with a **batten status file at the prime**, not pairs with an existing child shed.
A directly started loom/darn run can have its own status and seed without that batten record.
Its `loom start` or `loom resume` enters card 9’s check, but the judge supplies no owned-elsewhere entry for the slug, so the guard allows it despite origin’s board naming another owner.
The same missing verdict reaches Finalize’s run-owner check.
Evidence: `internal/fabricengine/originreconcilemoves.go:435–452` filters candidates by `shedrun.StatusFile(prime, slug)`; `internal/loomcli/sharedbootstrap.go:103–126` writes the child’s `SelfRunID` seed and child status only.
The addressed run’s owner must be judged independently of the batten-record filter used for reconciliation/teardown candidates.

**BLOCKING — consistency — `_lyx/plan/10-innerrun-owner-check.md:40–51` and `_lyx/plan/08-batten-owner-check.md:54` — Clearing the completed old machine’s owner pause cannot advance it to Land.**

The fasit explicitly requires the old machine that finishes its already-merged Finalize first to proceed to Land after `lyx resume <slug>` clears its owner pause.
Under card 10, clearing that coverage sets `ownerDue`, and the still-elsewhere owner immediately invokes `ownedElsewhere` and restores the pause.
The carve-out holds only while Finalize runs or has an unrestepped transient failure, so the completed child has no exception.
Even if InnerRun is allowed to return Done, card 8 checks ownership again before the next Land step and refuses it.
This is an actual driver path: each step is a `batten step` child, and `PreStep` executes before building/running its row.
Evidence: `_lyx/discussion/decision-record.md:156–157,287`; `internal/battencli/drive.go:139`; `internal/shedverbs/step.go:367–370`; `internal/fabricengine/replay.go:239,312` retains the locally finalized slug outside the ended set, so a board “done” does not automatically remove the owner verdict.
The post-Finalize completion exception needs to survive both the coverage-clear check and the subsequent batten step preflight.

**BLOCKING — consistency — `_lyx/plan/12-finalize-owner-checks.md:42,50` — The earlier parent fetch prevents the promised unreachable-origin policy from running.**

Card 12 replaces step 1b3, but production Finalize first executes step 1b2’s `holdParent`.
With a normal merge record and pushes enabled, that guard fetches the parent’s code remote.
If origin is unreachable, it returns an infrastructure error before `ownerChecks` can emit its owner-prefixed halt.
The same predecessor prevents the promised “parent already holds the work” exemption from reaching its local test on a transient re-entry.
The proposed table can pass with a fake reconcile and a nil parent guard while missing this real entry path.
Evidence: `internal/landingshed/finalize.go:198–204`; `internal/landingshed/stuck.go:115,125–130`; `internal/landingshed/parentguard.go:126–128`; production wiring in `internal/loomcli/landingdeps.go:175–180`; `internal/fabricengine/parenttips.go:44–47`.
The plan must account for the earlier fetch and test the policy through a populated parent guard.

**BLOCKING — consistency — `_lyx/plan/03-board-owner-writes.md:43–48` — The setter’s effective-owner guard has no pair-parent resolver.**

Card 3 wires `ParentOf` through `Config → Board → Store` for `boardCriticalSection` and `loadStore`, but `SetOwners(boardDir, req)` loads its own Store directly.
Neither its request nor its described construction carries that resolver; the specified nil behavior treats every parent as prime.
For a child pair with no explicit owner, prime owned by A and pair parent owned by B, machine B correctly passes the batten preflight’s resolved owner check, then the no-takeover claim sees prime A and refuses.
The existing-pair shortcut cannot avoid this: the claim executes before that shortcut.
Evidence: `_lyx/plan/03-board-owner-writes.md:43,46–48`; `_lyx/plan/08-batten-owner-check.md:42,48`; `internal/battencli/wire.go:777–799`.
The direct setter needs the same real pair-parent resolution as the preflight and board payload guard.

**MEDIUM — consistency — `_lyx/plan/03-board-owner-writes.md:47–48` — The prime write remains unguarded after the setter’s fresh pull.**

The setter unconditionally writes the prime when `Prime` is set, whereas its post-pull no-takeover refusal applies only to a slug.
A first-act caller can observe an absent prime owner, then enter the setter after another claim has reached origin.
The setter’s pull obtains that claim, but its following prime write overwrites it as a normal descendant commit; there is no replay conflict for the “losing claim keeps origin’s owner” behavior to resolve.
Evidence: `_lyx/plan/03-board-owner-writes.md:46–48`; `_lyx/plan/08-batten-owner-check.md:47,50`.
Apply the no-takeover check to the prime’s freshly pulled owner too.

Tool-call audit:

1. Located batten window/arming, board mutation, root pause override and loom factory paths because the inherited manifest did not include those external entry details.
2. Read `armAt`, pause override, Store mutation methods, Board construction and loom start/factory because their upstream state and mutation ordering were missing.
3. Read batten driver stepping and located shed PreStep/loop and InnerRun completion paths to establish how later rows encounter guards.
4. Read shed PreStep/loop, recorded-run enumeration and pair liveness to establish predecessor checks and the batten-only candidate filter.
5. Located precise plan/code references for ownership policies, installation timing and pause exemptions.
6. Read board validation and boardcli configuration to check whether payload guards were preempted and where parent resolution enters.
7. Read loop child invocation and parent-guard setup and located setter/bootstrap symbols to establish real process and predecessor paths.
8. Read parent movement checking, parent-tip fetching, work containment and landing dependency wiring to trace the earlier network failure.
9. Read landing tip wiring, origin-target filtering and claim-area code to verify production seams rather than fake-only paths.
10. Read the actual CreateWorktree claim and located bootstrap helpers to verify claim-before-existing-pair ordering.
11. Read `sharedbootstrap.go` and located Finalize/setter plan lines to verify direct child bootstrap creates no prime batten record.
12. Located card filenames and read exact parent-fetch, recorded-pair and claim lines to ground the findings precisely.
13. Read exact cards 5/8/9/10 policy lines and located D6 acceptance text to compare reachable states with the required completion path.
14. Located pull replay and push retry implementations because the explicit-pull route was outside the inherited manifest.
15. Read pull’s replay branch and located the push rejection condition to test the claimed retry coverage.
16. Read classified-push success/retry ordering, Finalize’s predecessor and production step hooks to finish the bypass and preemption traces.
