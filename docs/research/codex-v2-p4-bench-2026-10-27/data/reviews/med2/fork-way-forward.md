Recovery-path list:

- Card 1, invalid hostname → give the machine a hostname: progresses.
- Card 3, owner payload changes → `lyx adopt`; unreachable owner write → retry when origin answers; dirty/diverged Bolt → existing pull refusal’s remedy; deferred push → `lyx board sync`, then retry: progresses.
- Cards 5–6, rejected old-owner weft replay → adopt before resuming ownership-dependent work: progresses.
- Card 8, start/resume owned elsewhere → adopt; failed claim → rerun batten; offline owner-writing refusal → retry online; offline local-other-owner refusal → retry online: progresses.
- Card 9, existing shed run owned elsewhere → adopt; verdict/read failure → fix cause and retry: progresses.
- Cards 10–11, live owner change or spawn owner refusal → owner pause and adopt; unreachable check → retry without holding; failed pause → retry at notice probe: progresses.
- Card 12, parent-only/run-only/both-owner halt → prescribed adopt forms; unreachable landing → retry when origin answers: progresses.
- Card 12, adopter-first parent-push failure → appended reset/rebase repair: **does not safely progress as specified**, because the existing error also instructs merging and pushing.
- Card 13, failed owner pause → next wake or `lyx orch start`: the named retry is appropriate; production pause-seam wiring is the guard-reach lens’s concern.
- Card 16, invalid task selection → board list/promote or done-run pause lift; branch/local-live stop → keep/drop-local; live drop refusal → keep-local; add refusal → remove leftover branch and retry; push/lost-owner refusal → retry adoption; standing pause → `lyx resume`: progresses subject to the explicitly retained standing pauses.
- Card 16, owner-halted child → resume after ownership write and owner-pause lift: progresses for the specified owner halt.
- Card 17 adds no separate recovery operation.

Finding:

**MEDIUM · decision · `_lyx/plan/12-finalize-owner-checks.md:45` — Appending adopter-first repair preserves the forbidden merge-and-push remedy.**

Card 12 explicitly says `pushParent` “appends `adopterFirstRepair`.”
The existing `pushParent` embeds `%v` of the push error in its halt reason (`internal/landingshed/finalize.go:328`).
A rejected push becomes `PushFailureNotFastForward` (`internal/fabricengine/pushclassified.go:226–227`), and `PushFailedError.Error` includes its class’s way forward (`internal/fabricengine/pushclassified.go:39–41`).
For a code-side rejection, that way forward says `git merge origin/<parent>` followed by a push (`internal/fabricengine/failedpush.go:217–219,249–255`).
Consequently, the adopter-first halt would offer both merging/pushing the duplicate local landing and dropping it.
D6 specifically requires dropping the duplicate squash, “and never a merge, which would land the task twice” (`_lyx/discussion/decision-record.md:148–155`).

Proposed correction: change the existing card’s Intent so successful adopter-first detection **replaces** the ordinary recovery instructions while retaining useful diagnostic details.
Extend its already-targeted `TestFinalize_MergeMarkPushOrderAndVerdict` repair rows with a classified non-fast-forward error and assert that the halt names the reset/rebase repair without the old merge/push remedy.
Keep the original remedy only when origin lacks the task’s work or the fresh read fails.

Tool-call manifest:

1. Read Finalize’s remaining landing/push steps, batten’s window path, and halt/status references; inherited context ended before these recovery bodies.
2. Located pause-gate wiring, read loom resume’s upstream pause check, and inspected retryWeftPush; these recovery-entry details were absent.
3. Located actual gate files, read Bolt pull/push semantics, and located tender lift functions; needed to establish retry and owner-write postconditions.
4. Read network-gate installation, loom pause refusal, tender lift/pause behavior, and parent guard; needed to verify which pauses and locks the recovery verbs encounter.
5. Read Worktree-Create ordering, batten pre-run/step behavior, parent-work test, landing verdict helpers, and parent-tip reads; needed to trace claim-failure and landing-halt re-entry.
6. Read exact repair-plan lines, network waiting, fabric removal, and teardown references; needed to check the proposed repair’s final removal.
7. Read board recovery-command declarations, push classification, Land wiring, wake recovery, and adopt’s exact Intent; needed to check remaining next steps.
8. Read failed-push recovery construction and exact card-12 repair wording; established that appending retains merge instructions.
9. Read precise D6, error-rendering, classification, and existing Finalize test references; grounded the finding and correction in the answer key and current call chain.
