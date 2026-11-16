# Input L grades (run-fsm-tracked-records), base 055ef994

Plans graded alone from their own text. Columns are the plan dirs cm-la, cm-lb, cm-lc. Grades: R repeated, P partly, A avoided, n/a design differs.

## Key table

| Key | cm-la | cm-lb | cm-lc |
|---|---|---|---|
| F1 resume sets hold aside | A | A | A |
| F2 shed pause stays paused, no way forward | A | A | A |
| F3 act verdict before drivable check | P | A | P |
| F4 cleanup probe local worktrees only | R | P | R |
| F5 new top-level batten pre-run guard test | A | R | P |
| F6 new dobby-reading test | A | A | R |
| F7 teardown fake seam / nil remover unlisted | P | n/a | P |
| F8 PR-record writer optional in wiring check | A | P | R |
| F9 vacuous "no cache" test | A | A | A |
| F10 new attempts-move test duplicates rows | A | A | R |
| F11 cancel origin criteria lack origin-read test | A | A | A |
| F12 local parent branch read, not origin's fetched | A | A | A |
| F13 interrupted-cancel refusal omits machine | R | A | P |
| F14 landed wait lacks done-slug refusal | R | R | P |
| F15 push-owed aside for cancel/tender | n/a | n/a | n/a |
| F16 step, first pass, loom start skip act rule | R | R | R |
| F17 docs/help naming moved read left false | P | P | P |

Totals: cm-la R = F4, F13, F14, F16 (4), P = F3, F7, F17 (3). cm-lb R = F5, F14, F16 (3), P = F4, F8, F17 (3). cm-lc R = F4, F6, F8, F10, F16 (5), P = F3, F5, F7, F13, F14, F17 (6).

## Evidence per key (one line each plan)

F1. la: card 14 "setting aside the run's pause intent and shed pause, and passing pause coverage false" (no hold). lb: card 10 "`Aside{OwnPause: true, ShedPause: true}`", and "`Hold` (for `lym batten cancel`)". lc: card 14 "`RunAct` asks ... with the pause and the shed pause set aside".
F2. la: card 14 sets aside "the run's pause intent and shed pause"; card 4 WayForward "`lym tender resume <slug>` for a pause". lb: card 2 "`ShedPause` ... for `lym batten run`, `lym loom resume`, `lym batten cancel` and `lym tender resume`" and refusal "`lym batten run <slug>` for a shed pause". lc: ActRefusal "paused by its shed state: `lym batten run <slug>` from the prime"; resume sets ShedPause aside.
F3. la: card 7 "`startGuard.refuse` asks it first for the verb `run`, before every other check" (act first, only adds refusals, falls through). lb: card 3 "asks the act rule right after its run-status check and before the owner preflight" (drivable check first). lc: card 12 "`battenPreRun` calls it after the covered and continue-pass checks and before it reads or writes the run's status"; a landed verdict "passes" on to the guard.
F4. la: card 11 "`PairStands` ... filled in `fabricCleanupDeps` from the topology's `PairParts`" (local parts only; the plan's own review F1 says so). lb: card 8 "`cleanupDeps.PairPresent` seam, set in `Cleanup` to the same pair probe the cancel uses" (probe not stated to include origin; no origin-only row in TestCleanup). lc: card 10 "`PairStands` ... which `fabricCleanupDeps` reads from the pair's parts".
F5. la: card 7 "`TestStartGuard_Refuse` gains rows where a `wait` verdict and a `hand_over` verdict refuse `run`" (existing table). lb: card 3 creates "`TestBattenPreRun_RefusesWhatTheActRuleHolds` is a tier0 test with the seam faked" while the existing pause-refusal tests are only under Uses. lc: card 12 creates `TestRefuseByAct` (tests the new function with a fake ask; overlaps pause rows only partly).
F6. la: card 6 edits `TestPlanPass` and `TestDrivePasses_ExitHandling` only. lb: card 5 "`TestDobbyRunCells_ReadsTheRunsFromDisk` asserts the records `DobbyRuns` fills" (existing test). lc: card 11 creates "`TestDobbyRuns_CarriesTheActAnswer` (tier 1, hubforge)".
F7. la: card 11 "`Teardown` gains the seam that calls it; `New` wires it" with only `TestTeardown_CallSequence` listed, no fake or nil remover named. lb: no new Teardown seam; card 9 reads `runfsm.ReadIntents` directly in `RemovePair` (n/a). lc: card 10 "`Teardown` gains `dropCancelledRecord`, which `New` wires", fakes and nil case unlisted.
F8. la: card 12 "`envkit.LandingDeps` fills `RecordWritten` with a no-op ... production wiring fills the new field" (required seam). lb: card 11 "`PullRequestDeps` gains `Publish func() error`" with envkit and the intentionallyNil map only under Uses; nil handling not stated. lc: card 7 "a nil `PublishRecord` publishes nothing".
F9. la: card 5 "every state reading the same after the prime's and the pair's `.lym` directories are removed". lb: card 3 "removes the worktree's and the hub's `.lym`, and reads the same state". lc: card 6 "reads the same state again after removing `.lym`".
F10. la: card 13 adds no test beyond `TestPathConstructors` and `TestAcceptFixReview` ("the existing fixer and Finalize tests keep passing through their fixtures"). lb: card 12 extends the card 11 tier1 test with an attempt record. lc: card 8 creates "`TestFixerAttemptsLiveInTheRecordsDir` (tier 0) runs a fixer turn over the fixture".
F11. la: card 9 "The two integration tests (tier1) assert the record on origin after a kept cancel". lb: card 6 `TestCancelIntegration_AnInterruptedCancelReadsCancellingOnAnotherClone` and "the cancel's intent is on origin when `lym batten cancel` returns". lc: card 10 `TestCancelIntegration_IntentAndFinishReachOrigin`.
F12. la: card 5 "The local parent branch never decides it, except under the push skip". lb: card 3 "never the local parent branch and never the pair's checkout". lc: card 5 "The local parent branch never decides it".
F13. la: card 9 refuses a run with an intent "naming `lym batten cancel <slug>` to finish it, or the wait for a live one" (no machine). lb: card 6 "`cancelMarkerRefusal` names the machine and start time". lc: card 9 live cancel "under way since the intent's start on its machine"; the interrupted wording only names the rerun.
F14. la: card 4 "nothing to do for a landed run". lb: card 2 "nothing is left to do for `landed`". lc: card 12 "A landed verdict passes: a done record goes on to the start guard, which clears a done motif run or refuses with its own way forward".
F15. All three leave the owed-push input at zero this part (la "`PushOwedOffline` ... stay zero"; lb "owed-push inputs stay at their neutral zero values"; lc "owed-push ... inputs stay false"), so no aside is needed yet.
F16. la: card 7 "leave `step` and `goto` unjudged by it"; no loom start card. lb: only batten run, loom resume, dobby, tender resume (own review F15). lc: same four verbs (own review F3). The dobby and `batten run` cover the first pass; step and loom start are covered by none.
F17. la: card 9 omits `cli.go` and the cancel help (review F4), though doc.go files are edited. lb: dobbyengine/doc.go and cancel help left under Uses (review F13). lc: "keep under Uses the package docs ... that describe the cancel marker" (review F12).

## cm-la

- Cards: 14. Source files named (Create/Edit/Delete/Extends): 33 entries; 20 explicit file paths and 13 package paths (symbols resolve to their package, so file level is only exact for explicit files).
- Packages touched: PATTERN.md, pattern/PATTERN-lym-files.md, internal/{battencli, dobbyengine, landingshed, loomcli, lymdirs, pairteardown, pullrequest, runfsm, shedengine, shedrun, tenderengine, tendercli, testkit/envkit}.
- Versus landed commit 27eefef6 (149 files, 29 dirs; see note): 15 of 15 named dirs landed; named-not-landed dirs 0; landed-not-named dirs 14 (cmd/lym, contracts/orchrole, docs, docs/research, docs/research/loom/adopt-build-revision-2026-11-16, internal/beamcli, boardengine, fabriccli, fabricengine, orchcli, orchengine, runproc, shuttleengine, shuttleengine/claudeengine).
- Explicit files: 13 named and landed (PATTERN.md, pattern/PATTERN-lym-files.md, internal/battencli/cancelmarker.go, cancelmarker_test.go, internal/dobbyengine/doc.go, internal/pairteardown/doc.go, internal/pullrequest/doc.go, internal/runfsm/act_test.go, doc.go, testmain_test.go, internal/shedrun/doc.go, internal/tendercli/doc.go, internal/tenderengine/doc.go); 7 named not landed (internal/landingshed/doc.go, internal/loomcli/prrecord_integration_test.go, internal/lymdirs/doc.go, internal/runfsm/derive_test.go, intent_test.go, read_test.go, internal/shedengine/doc.go). Landed files not explicitly named: 136 (97 in dirs it touches, 39 in other dirs).
- Review: verdict BLOCKING; BLOCKING 2, MEDIUM 11, LOW 9, NIT 1.
  - F1 B: cleanup judges pair standing from local parts only, deletes a kept cancel's record on a machine without the pair.
  - F2 B: owner refusal and intent write in tenderengine.Pause also refuse the OwnerCaller owner pause.
  - F3 M: card 9 does not list tests and fixtures that call the renamed marker seams; card 10 breaks the test binary.
  - F4 M: card 9 falsifies cli.go package doc, cancel Long help and pairteardown doc it does not list.
  - F5 M: battencli exhaustive state switches (checkRunStatus, battenPreStepUnmarked, battenRunEnvelope) unlisted for `cancelled`.
  - F6 M: card 13 lists only fixture constructors; fixture methods still build attempt paths from ScratchDir.
  - F7 M: record deletion after every Teardown.Run removal also fires for a local-only removal on a non-owner.
  - F8 M: cancel with no batten record ends processes with no intent recorded.
  - F9 M: runfsm leaf claim vs imports reaching dobbyengine/landingshed (cycle).
  - F10 M: PR record's session id not started fresh on another machine; PR-Review halts.
  - F11 M: landing read from Finalize history entry misreads a re-opened child.
  - F12 M: new test re-covers goto and relaunch refusal rows.
  - F13 M: new loom resume test repeats TestResumeVerb shape.
  - F14 L: tier1 tender test builds a second hub duplicating an existing test.
  - F15 L: refuseNothingLeft retype lacks re-sign arrow.
  - F16 L: judgeStatus skip reason misleads after the change.
  - F17 L: BattenFollowUp advice wrong for a cancelled record.
  - F18 L: PR-record test and lock doc left describing scratch.
  - F19 L: new function seams state no nil reading.
  - F20 L: cancel-intent refusal unreachable for verb run.
  - F21 L: act check on tender resume refuses the owner pause hand lift.
  - F22 L: cancel asides have no caller; later parts not stated.
  - F23 N: PATTERN bullet wording differs from D3 (notice or revive).
- Contradictions with the decision record: (1) Derive reads `landing` from "current row is Finalize or whose history holds a Finalize entry" against D1 "at Finalize or a later row" (own review F11). (2) Dobby sets the shed-pause aside only for a run whose work reached its parent; D1 grants the aside to the named verbs, not the dobby pass. (3) Intent writers (`RecordCancelIntent`, `RecordPauseIntent`) sit in runfsm, where the Exports line puts them under shedrun's tracked paths. (4) Offline pause/resume spawn today's detached push, a bounded departure from D3's "owes its push under D2's retry" which the plan names as later part. No other direct contradiction found.

## cm-lb

- Cards: 13. Entries named: 28; 16 explicit file paths, 12 package paths.
- Packages touched: PATTERN.md, pattern/PATTERN-lym-files.md, internal/{battencli, dobbyengine, landingshed, loomcli, lymdirs, pairteardown, pullrequest, runfsm, shedengine, shedrun, tenderengine, tendercli}.
- Versus landed: 14 of 14 named dirs landed; named-not-landed dirs 0; landed-not-named dirs 15 (the 14 as for la plus internal/testkit/envkit).
- Explicit files: 12 named and landed (PATTERN.md, pattern/PATTERN-lym-files.md, internal/pairteardown/doc.go, internal/pullrequest/doc.go, internal/runfsm/act.go, act_test.go, doc.go, state.go, state_test.go, internal/shedrun/doc.go, internal/tendercli/doc.go, internal/tenderengine/doc.go); 4 named not landed (internal/landingshed/doc.go, internal/runfsm/read.go, read_test.go, record.go). Landed files not explicitly named: 137 (97 in dirs it touches, 40 other).
- Review: verdict BLOCKING; BLOCKING 2, MEDIUM 11, LOW 6, NIT 0.
  - F1 B: import cycle dobbyengine -> runfsm -> hubgeom -> dobbyengine; card 5 cannot compile.
  - F2 B: judgeStatus candidate change lets ClearDoneRun remove a kept cancel and its task branch.
  - F3 M: only case switches searched; `== done` readers hold a slot, treat a cancelled run live.
  - F4 M: envkit.LandingDeps and intentionallyNil map left without the new Publish and AttemptsDir seams.
  - F5 M: batten builds no landingshed.Deps, so card 12's batten wiring has no substrate.
  - F6 M: under --remove RemovePair already commits the record deletion; card 7's publish has nothing to push.
  - F7 M: dobby planPass drops the CancelMarker check, spawns passes in the window before the intent is committed.
  - F8 M: act check on resume can never answer hand_over (owner refusal runs first).
  - F9 M: "grep it first" fall-back for PR-Review session left undecided, no test.
  - F10 M: takeCancelMarker lacks the remove input; seams not re-signed.
  - F11 M: PastParentSideMerge ignores precedence, act for a paused or cancelling run past merge; no record entry.
  - F12 M: D1/D3 steps no card carries (board status, session fresh start, pause entry at catch-up, hand-over notice).
  - F13 M: docs and help text left false across cards.
  - F14 L: batten run of a kept cancel stops at checkRunStatus with done wording.
  - F15 L: Aside.Cancel/Hold have no caller.
  - F16 L: Derive result for no matching state unspecified.
  - F17 L: TestPlanPass hold row unreachable.
  - F18 L: interrupted-cancel integration test has no stop seam.
  - F19 L: kept-cancel integration test duplicates KeptPairJudgeOnly.
- Contradictions with the decision record: (1) `PastParentSideMerge` "ignores the states' precedence", so a shed-paused or cancelling run past its merge answers act; D1 gives act only to a `landing` run (own review F11). (2) Pairteardown deletes a batten record when it holds any cancel intent, not only a `cancelled` one. (3) Landing read from "history names it" as in la. (4) Intent writers live in runfsm, not under shedrun's tracked paths (shedengine declares neither key). (5) Card 11 leaves the D3 fresh-session fall-back as "grep it first" (the record fixes it). No other direct contradiction found.

## cm-lc

- Cards: 14. Entries named: 18; 4 explicit file paths, 14 package paths.
- Packages touched: PATTERN.md, pattern/PATTERN-leaf-packages.md, pattern/PATTERN-lym-files.md, internal/{battencli, dobbyengine, fabricengine, landingshed, loomcli, lymdirs, pairteardown, pullrequest, runfsm, shedengine, shedrun, tenderengine, tendercli, testkit/envkit}.
- Versus landed: 16 of 16 named dirs landed; named-not-landed dirs 0; landed-not-named dirs 13 (cmd/lym, contracts/orchrole, docs, docs/research, docs/research/loom/adopt-build-revision-2026-11-16, internal/beamcli, boardengine, fabriccli, orchcli, orchengine, runproc, shuttleengine, shuttleengine/claudeengine).
- Explicit files: 4 named and landed (PATTERN.md, pattern/PATTERN-leaf-packages.md, pattern/PATTERN-lym-files.md, internal/runfsm/doc.go); 0 named not landed. Landed files not explicitly named: 145 (109 in dirs it touches, 36 other). Only lc names fabricengine, which landed (originreads.go, replay.go).
- Review: verdict BLOCKING; BLOCKING 2, MEDIUM 10, LOW 7, NIT 0.
  - F1 B: pairteardown importing battenrecipe forms a test import cycle; card 6 cannot build its gate.
  - F2 B: PR record crosses machines but ensureSession resumes the recorded session id; PR-Review halts.
  - F3 M: D1/D3 steps no card carries and no record entry drops.
  - F4 M: push-skip rule (local parent decides past-merge) carried by no card.
  - F5 M: DobbyTasks starts a released task with a kept cancelled record every tick.
  - F6 M: refuseByAct's cancelling branch unreachable from batten run.
  - F7 M: landed pass-through to start guard has a false stated bound.
  - F8 M: refuseOwnedElsewhere runs first against origin, so act-rule hand_over fires only on a stale Bolt.
  - F9 M: endRunPhase publishes an intent that recorded nothing; commit of an untracked run dir fails.
  - F10 M: ActRefusal wordings for owed-push, code-tip, hold reachable only from a constructor test.
  - F11 M: seven attempt-record test members still use ScratchDir.
  - F12 M: package docs and headers describing the marker and scratch record kept under Uses.
  - F13 L: TestPawlRetiredMove_CarriesTheScratchPullRequestIntoTheLandingRecord omitted.
  - F14 L: Resume clears the pause intent before a lift that can still refuse.
  - F15 L: refuseNothingLeft change never decides.
  - F16 L: planPass covered block becomes dead and the card does not say so.
  - F17 L: new integration test belongs in KeptPairJudgeOnly; duplicates TestReadRun row.
  - F18 L: TestOrder duplicates TestDerive overlap rows.
  - F19 L: dropCancelledRecord names no lock and no composition with the cancel's own --remove deletion.
- Contradictions with the decision record: (1) A landed verdict is let through to the start guard, while D1's act rule answers `wait` for a terminal state and verbs refuse a `wait`. (2) Adds a `PATTERN-leaf-packages` section, but the record declares only the `PATTERN-lym-files` change. (3) `Derive` takes bool facts and the reader lives in `pairteardown`, not `runfsm`; the record's Delivers has runfsm deriving the states from tracked records. (4) Landing read from "history holds a done Finalize entry", as in la. (5) Dobby sets the shed pause aside for one class of run, as in la. (6) Its fabricengine edit is outside the record's scope list; the overview says so.

## Notes

- Landed commit: 27eefef6 "One run FSM over tracked records, and the orch resumes its own session (#132)". It changes 149 files (not 116) by `git diff --name-only 27eefef6^ 27eefef6`: 90 non-test files and 59 tests. It includes the orch session work (orchcli, orchengine, runproc, shuttleengine, claudeengine, contracts/orchrole) and docs outside this task's Goal, which explains most "landed not named" dirs. In-scope packages that landed but no plan named: boardengine (la, lb, lc), fabricengine (la, lb), testkit/envkit (lb).
- File-set comparison is exact only for explicit file paths; plan entries `path#Symbol` were resolved to their package directory, so the count of landed files "not named" is an upper bound.
- Plans' own texts were the only evidence; F3, F4 (lb), F7 and F8 (lb) are judgment calls on short evidence.
