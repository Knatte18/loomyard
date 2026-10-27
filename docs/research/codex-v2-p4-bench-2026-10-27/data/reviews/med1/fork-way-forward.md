Recovery-path inventory:

- Card 1 hostname error → give the machine a hostname: progresses.
- Card 3 owner-payload refusal → `lyx adopt`; unreachable origin → retry; deferred push → `lyx board sync` then retry: progresses.
- Cards 5, 8, 9 ownership refusal → adopt from the prime: progresses subject to the halted-child defect below.
- Card 10 owner pause after the old machine finishes Finalize → `lyx resume <slug>`: not reliably progressing; finding 1.
- Card 12 parent/run ownership halts → the specified adopt forms: ownership can be corrected, but a halted LLM run on a second clone cannot resume through the prescribed command; finding 2.
- Card 12 unreachable/read-error halt → retry `lyx loom resume`: progresses once the cause clears and the driver is available.
- Card 12 adopter-first parent repair → reset or drop the squash, then remove the pair: matches the decision record; no finding.
- Card 13 failed owner pause → next wake or `lyx orch start`: recovery depends on the installed pause seam; its wiring belongs to guard-reach.
- Card 16 unknown/note/done refusal → board list/promote/resume: list and promote progress; done-run resume is subject to finding 1.
- Card 16 local-commit/live-pair stop → `--keep-local` or bounded `--drop-local`: progresses as specified.
- Card 16 standing unrelated pause → `lyx resume <slug>` or hub-wide `lyx resume`: matches `tenderengine.Resume`.
- Card 16 failed add/push/lost ownership → resolve the reported cause and repeat adopt: progresses.

Findings:

1. **MEDIUM · decision · `_lyx/plan/10-innerrun-owner-check.md:40–51` — Lifting the completed old owner’s pause schedules the check that reinstalls it.**

   Card 10 promises that a completed child proceeds to Land after `lyx resume <slug>`, but its unconditional covered→uncovered transition sets `ownerDue`; the ensuing owned-elsewhere verdict calls `PauseOwned` again.
   The real reconcile does not necessarily report this completed landing as ended: `finalizeEnteredRead` returns true for any Finalize history row (`internal/fabricengine/replay.go:259–260`), and `readOriginEnded` removes those entries through `withoutFinalizeEntered` (`internal/fabricengine/originreconcilemoves.go:528`).
   Thus the other machine’s recorded owner remains the owner verdict even after the old machine successfully pushed its landing.
   InnerRun’s Done arm can still be waiting for its driver’s stop report (`internal/battenshed/innerrun.go:1004–1057`), and a reinstated pause suppresses that arm through `runArm` (`internal/battenshed/innerrun_pause.go:38–43`).
   The recovery can therefore re-pause the completed run instead of letting it reach Land, contrary to D6 and the card’s own bound.
   The planned done-child test should retain a real owned-elsewhere verdict while coverage clears, including a driver still within its exit grace; the plan needs an explicit completed-landing continuation rule.

2. **MEDIUM · decision · `_lyx/plan/16-adopt-module.md:63–64` — The prescribed owner-halt recovery cannot wake an LLM run on a second clone with no driver strand.**

   Step 5 starts batten in a window and invokes `lyx loom resume` for the blocked owner-halted child.
   `loom resume` creates a missing driver only for a Go-seeded run; otherwise its no-live-driver branch refuses and names `lyx loom start` (`internal/loomcli/resume.go:114–142`, `216–226`).
   Reed’s strand state is machine-local `.lyx/reed.json` (`internal/reedengine/state.go:1`, `146–154`), so a freshly added pair on the adopter has no old LLM driver to wake.
   Starting batten does not fill that gap: `Call` spawns only an absent child or an unconfirmed running child (`internal/battenshed/innerrun.go:389–405`); a blocked child goes to `haltedStep`, whose revival does nothing for `ChildDriverNone` (`internal/battenshed/innerrun.go:787–801`, `848–859`).
   Consequently the documented “adopt on the parent’s owner, resume, and land” recovery leaves this valid run blocked.
   The planned fake `ResumeChild` assertion and second-clone test do not exercise this case.
   The plan needs a concrete recovery for an owner-halted LLM child without a local driver, while retaining normal resume behavior for a parked driver.

Tool-call manifest:

1. Read Finalize’s merge/push flow and searched resume/pause/parent-tip definitions, absent from inherited code bodies.
2. Read loom resume, batten window launch, pause refusal, and tender lift; checked board recovery verbs.
3. Read Finalize-entered filtering, InnerRun pause handling, and D3 details to trace completed-landing recovery.
4. Read the reconcile’s ended-filter call, InnerRun check/Done entry, and batten preflight to test pause-lift continuation.
5. Read InnerRun Call and Done grace behavior to distinguish a completed child from a respawn and establish the re-pause consequence.
6. Read loom’s driverless resume branch and shed halt persistence to check owner-halt recognition and recovery.
7. Read InnerRun revival, loom’s no-driver refusal, and reed window launch to establish what batten startup actually guarantees.
8. Read reed’s machine-local state declaration and batten’s driver classification/revival wiring to confirm second-clone behavior.
