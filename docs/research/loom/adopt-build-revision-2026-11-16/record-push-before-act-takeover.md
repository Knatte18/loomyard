## Goal

Every transition pushes its records before the run acts, and a takeover entry in the run's records hands it over without replay

Takes: runfsm's `Judge`, `Aside` and `Inputs`, whose `PushOwedOffline` and `CodeTipMissing` nothing fills yet, and `pairteardown.ReadRun`, the one reader of a run's inputs; the batten record's cancel and pause intents with their shedrun writers, and the detached commit-and-push that `lym tender pause`, `lym tender resume`, `lym batten cancel`, the pull-request record and the fixer attempt counts make today; fabricengine's PushPairAnchored, SpawnStatusPush, teardown-lock respawn, pushclassified's rejection path, originreads and ArchiveOwnerTips; boardengine's Sync, SpawnSync and pushOwnerCommit; statuscommit and sheddriver's records commit; landingshed's Finalize; pairteardown's EndSubjectProcesses over runproc's EndSubject
Delivers: a shed transition that takes a gate admission, commits its records with the code tip it was made with and pushes records side first, code side after, before the next producer starts; the run intents, the pull-request record and the fixer counts pushing through the same synchronous push; an owed push retried on a backoff and shown in lym beam status, which the offline pause, resume and cancel verbs also ride; pairteardown's ReadRun filling the owed-push and code-tip inputs; the per-transition detached push and its teardown-lock respawn retired; synchronous Bolt writes for a run claim, a landing's done and an owner; Finalize pushing the parent branch before its transition and board done; a rejected records push that reads origin's record and hands over on another machine's takeover entry, ending the run's processes, archiving local tips and the local parent merge; the code-tip wait and its grace
Exports: the takeover entry type and its reader, tested with entries a testkit writer makes; the owed-push and code-tip facts `pairteardown.ReadRun` puts into runfsm's Inputs; fabricengine's records-first PushPairAnchored and its hand-over result

## Scope

Decision numbers are those of the Sketch `adopt-redesign`.
The run FSM (D1) and the tracked run decisions (D3) landed in `run-fsm-tracked-records`,
and the orch's own-session resume and the subject-process end (D9, D10) in `orch-resume-own-session`.
The registry, the catch-up and the holds (D5 to D7) belong to `bolt-holds-catch-up`,
and the foreground adopt and the takeover entry's writer (D8) to `adopt-foreground-leftovers`.

The part changes these packages:

- internal/fabricengine (about 1000 changed lines)
- internal/battencli (about 300 changed lines)
- internal/sheddriver (about 200 changed lines)
- internal/loomcli (about 200 changed lines)
- internal/landingshed (about 250 changed lines)
- internal/boardengine (about 150 changed lines)
- internal/statuscommit (about 100 changed lines)
- internal/tendercli (about 100 changed lines)
- internal/pairteardown (about 80 changed lines)
- internal/shedengine (about 60 changed lines)
- internal/beamcli (about 60 changed lines)
- internal/fabriccli (about 40 changed lines)
- internal/tenderengine (about 40 changed lines)
- internal/pauserecord (about 40 changed lines)
- internal/pullrequest (about 30 changed lines)
- pattern (about 30 changed lines)

## Decisions

### D2 — A transition pushes its records before the run acts

- A shed transition commits its records as today, the files D3 moved into tracked records included, then pushes them synchronously:
  both sides in a task pair, records only in the prime.
  The records commit names the code tip the transition was made with (D4).
  The next producer starts only once every side has landed.
- The writes D3 moved into tracked records push through the same synchronous path:
  the cancel intent and its finish, the run pause intent and its clearing, the pull-request record and the fixer attempt counts.
  Today each commits and spawns a detached push (`internal/tendercli/runintents.go`, `internal/battencli/cancelcmd.go`, `internal/pullrequest/record.go`, `internal/landingshed/fixerattempts.go`).
  `lym tender pause`, `lym tender resume` and `lym batten cancel` return with their intent on origin,
  or with the push owed when origin is unreachable (below).
- The pair push itself, `fabricengine.PushPairAnchored`, sends the records side first.
  It sends the code side only once the records side has landed, and none when the records side fails or is rejected,
  so the order holds for every caller of a pair push, a transition's and `lym loom commit-records`' among them.
  Today that push sends the code side first.
  A code side withheld this way is reported with the records side's failure, and the caller's next pair push sends it.
- A Finalize's parent-side merge pushes the parent branch before its transition's records and before the landing's board `done` (D4).
- Every detached status push (`fabricengine.SpawnStatusPush`) that carries a run transition retires:
  the per-transition spawn behind `internal/statuscommit/statuscommit.go`,
  and the respawn of a pair's push that exited on a held teardown lock (`internal/fabricengine/teardownlock.go`).
  A transition's push that meets a held teardown lock waits for its release and pushes before the next producer starts.
  A wait that outlasts the push lock wait (`fabriclayout.StatusPushLockWait`) turns the push owed;
  that releases the admission below, so a pause written meanwhile reaches the process again.
  Fabric's pause deferral of a push no longer applies to a transition's push, which is synchronous;
  it stays for the detached pushes below.
  A transition takes a network gate admission (`pauserecord.Admit`) before its commit,
  and releases it once every side has landed or its push turns owed,
  so a pause or hold written after the commit stops the run's next producer and never that push's first attempt.
  An owed push's retries wait at the gate like any network call, so a pause holds them, and the run waits on the owed push meanwhile.
  `lym loom commit-records` keeps its synchronous push.
- The anchor change's push (`internal/fabricengine/anchorchange.go`) and tender's prime push (`internal/tendercli/deps.go`) stay detached.
  Neither carries a run transition: the first pushes a pair's anchor move,
  and the second pushes only commits no run waits on, since every transition pushed before its run acted.
- A push that fails for a transient reason, such as an unreachable origin, leaves the run waiting at that transition with the push owed.
  It retries on a backoff under `PATTERN-daemon-wakeups` and starts no producer meanwhile.
  `pairteardown.ReadRun` reads the owed push into `runfsm.Inputs.PushOwedOffline`,
  and `lym beam status` shows the owed push and its last error; the existing failed-push record and creel notice stay.
- With origin unreachable, `lym tender pause <slug>`, `lym tender resume <slug>` and `lym batten cancel` act here at once and owe their push,
  as D3 says; their `runfsm.Aside` already sets an owed push aside, so the verbs themselves still run.
- A rejected push reads origin's tip for a takeover first (D4), and replays the records as today only when it finds none.
- A Bolt write that carries a decision pushes synchronously before its writer goes on:
  a run claim, the board `done` of a landing, an owner, a hold (D7) and the registry (D5).
  A board write that only displays a run status stays on the detached board sync.

Bound: no run acts while origin is unreachable, and an owed push holds the run at its transition until it lands.
Work an agent made inside a step and has not reached its transition yet is on this machine only;
a stop there loses nothing pushed, and the step runs again from its start.
Until an owed intent push lands, the intent is on this machine only, and another machine reads the run unpaused or not cancelled.

Rationale: the operator requires the whole state on GitHub at all times, and today a push can lag or fail with nothing waiting for it (state map 1.3, 7).
A push costs two network round trips or more (`internal/fabricengine/pushclassified.go`), small beside an agent step.

### D4 — A takeover entry in the run's own records hands the run over

- Adopt writes a takeover entry, naming the new owner machine, into each moved run's batten record and child record, each on its own branch, and pushes it (D8).
  This part ships the entry type, its reader and a writer in `internal/testkit` for its own tests;
  the production writer arrives with adopt, since `PATTERN-test-only-code` keeps a function only tests call out of a production file.
- When a run's records push is rejected, a transition's or any other caller's of the pair push (D2),
  lym fetches the branch and reads the run's own record from origin's fetched tip, before any replay.
  When that record holds a takeover entry naming another machine that this machine has not yet read, the run hands over without replaying:
  this machine pushes no code side for the rejected push,
  ends its processes for the slug through `pairteardown.EndSubjectProcesses`, which does not end the dobby when the caller is that dobby's own run,
  pushes the code tip the run's last transition on origin names when origin's code branch lacks it, since that transition stands,
  archives the local tips as the old owner's archive does (`fabricengine.ArchiveOwnerTips`), with uncommitted files first committed onto the archived tip,
  archives its local parent branch's tip the same way when this run's parent-side merge alone puts that branch ahead of its upstream,
  and resets that branch to its upstream,
  pushes the archive tags, and posts one notice naming the machine and any pause or cancel intent it dropped (D3).
  It pushes nothing more for the run, so its code commits after the takeover reach origin only under the archive tags.
  A run `landing` past its parent-side merge hands over too, and its new owner resumes the Finalize from origin's records.
  The Finalize reads the owner from a fetched Bolt before its parent-side merge and before each later push,
  and hands over when another machine owns the run, so the old owner writes nothing to the parent's branch after it reads the takeover.
  After the merge into the local parent branch, the Finalize pushes the parent branch, then commits and pushes its transition,
  then writes the board `done` (D2); today it marks the board `done` before the parent push (`internal/landingshed/finalize.go`).
  Under the push skip, which pushes nothing, the board `done` follows the local merge.
  So whenever origin's records or board read the run past its merge, origin's parent branch holds the task's work,
  and the adopter reads that from its fetch (D1).
  A takeover between the local merge and the parent push leaves origin's parent branch without the work:
  the adopter reads the run as before its merge and merges again,
  and the old owner's hand-over archives and resets its local parent branch as above,
  so a later landing on that machine finds the branch level with its upstream.
  A parent push rejected over an origin parent branch that already holds the task's work archives the local merge,
  resets the local parent branch to origin's, and goes on past the merge.
- Each transition's records commit names the code tip it was made with (D2).
  After a takeover, origin's code branch therefore moves only to a tip a pushed transition names,
  so the adopter's later code push meets no other machine's commit.
  Until origin's code branch holds the tip the run's last transition names, the act rule answers `wait` through `Inputs.CodeTipMissing`,
  which `pairteardown.ReadRun` fills, and `lym beam status` names the tip and the machine that owes it;
  that machine's next hand-over or catch-up (D6) pushes it,
  and the owner's dobby fast-forwards its local code branch to it before the run's next pass.
  A tip still missing after the code-tip grace is given up:
  the owner commits and pushes a records commit that returns the run to its previous transition,
  and the step runs again from there, as D2's bound says a stopped step does.
  A machine owing a tip pushes it only while the run's last transition on origin names it, so a given-up tip never lands.
- A rejection whose fetched record holds no such entry, such as one over commits of other runs on a shared records branch like `main-weft`,
  replays and pushes as today.
- An adopter's own takeover push that is rejected fetches, then re-reads its hold and the subjects' owner from a Bolt it fetches.
  When its hold is gone or either names another machine, it stops, naming the subjects' owner it reads from that Bolt (D8).
  Otherwise it writes the takeover entry again onto origin's tip and pushes again;
  it never replays its own commit over a record the old owner changed, and it never hands over.

Bound: transitions the old owner pushed before the takeover landed stand, and the adopter goes on from them.
The old owner's step in flight runs to its next transition, which the takeover rejects;
its result is archived under a tag, not lost and on neither branch.
A pause or cancel intent owed on the old owner when the takeover lands is dropped, and the notice names it to repeat on the owner machine.

Rationale: a takeover entry in the run's own record is a compare-and-swap per run that holds on records branches many runs share,
and needs no extra fetch per transition.

## Constraints

- `PATTERN-owner-machine`: the owner stays on roots and beams, written only by `boardengine`'s setter, read through `hubgeom.ParentOf`.
  The rest of a Finalize past its parent-side merge, as origin's parent branch shows it,
  finishes on the run's owner, the adopter after a takeover (D1, D4).
- `PATTERN-pair-remote`: the code branch still goes along with a pair's records push, now once the records side lands (D2),
  and a refused push still leaves a failed-push record and a notice.
- `PATTERN-lym-files`: every new tracked file sits under `_lym` and every cache under `.lym`, both named in `internal/lymdirs`.
- `PATTERN-tolerant-records`: the takeover entry and the other new tracked records read tolerantly and are written through `internal/state`.
- `PATTERN-fabric-boundary` and `PATTERN-gitrepo-client-boundary`: the synchronous push, the takeover read and the archive run in `internal/fabricengine` through `gitexec` and `gitrepo`.
- `PATTERN-child-processes`: the hand-over ends a run's processes only through `internal/runproc`, by `pairteardown.EndSubjectProcesses`.
- `PATTERN-daemon-wakeups`: D2's retry waits wake at most once a second and back off while idle.
- `PATTERN-refusal-way-forward`: every refusal this part adds names its way forward and a test reaches it.
- `PATTERN-code-conventions`: the code-tip grace is one named constant.
- `PATTERN-test-only-code`: the takeover reader has its production caller in the rejected-push path,
  and the entry writer a test uses lives in `internal/testkit`.

## Auto-mode assumptions

- The code-tip grace is 30 minutes (D4).

## Open risks

## Acceptance criteria

- D2: a shed transition in a task pair pushes its records side, then its code side once the records side landed, and starts the next producer only after both landed on origin.
- D2: a pause written between a transition's commit and its push leaves that transition on origin, and the run's next producer does not start.
- D2: a transition's push that waits on a held teardown lock past the push lock wait turns owed, and a pause written then holds the process's network calls.
- D2: with origin unreachable, a transition leaves the run waiting with the push owed, starts no producer, and goes on once origin answers.
- D2: a run claim, a landing's board `done`, an owner write and a hold write are on origin when their verb returns.
- D2: a pull-request record write and a fixer attempt count are on origin before the next producer starts.
- D3: `lym tender pause <slug>`, `lym tender resume <slug>` and `lym batten cancel` return with their intent on origin.
- D3: with origin unreachable, `lym tender pause <slug>` holds the run's processes here at once, and the pause intent is on origin once origin answers.
- D4: a records push rejected over a takeover entry naming another machine ends this machine's processes for the slug, pushes an `archive-owner/` tag holding its local commits and uncommitted files, and pushes nothing to the branch.
- D4: a push rejected over another run's commits on a shared records branch replays and pushes, and the run goes on.
- D4: a records push rejected over a takeover entry runs no replay, so no replay conflict stops the hand-over.
- D4: an adopter's takeover push rejected over the old owner's later transition writes the takeover entry onto origin's tip and lands, and the adopter reads the run at that transition.
- D4: after a takeover, a landing past its parent-side merge is driven on exactly one machine, the adopter, which finishes its Finalize.
- D4: after a takeover, the old owner's next transition leaves origin's code branch unchanged, and its code commits are under its archive tag.
- D4: a run whose last transition names a code tip origin's code branch lacks waits until that tip lands, then goes on from it; one still missing after the code-tip grace returns to its previous transition and runs the step again, and the owing machine's later push of that tip leaves origin's code branch unchanged.
- D4: an old owner past its parent-side merge that reads the takeover pushes nothing more to the parent's branch, and the adopter finishes the Finalize.
- D4: an old owner that merged into its local parent branch and was taken over before pushing it leaves origin's parent branch without the work; the adopter merges and lands the run, origin's parent branch holds the work once, and the old owner's local parent branch is level with its upstream, its merge under an archive tag.
- D4: a Finalize's board `done` reaches origin only after its parent branch push landed.
- D4: after a takeover, the old owner's `lym loom commit-records` hands over and leaves origin's code branch unchanged.

## PATTERN changes

- `PATTERN-run-fsm` — adds: Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule; a transition commits and pushes before the run acts; a run whose record holds another machine's takeover hands over, never replays.
- `PATTERN-pair-remote` — changes: Pushing run records or tearing down a pair: the code branch goes along after the records side, both before the run acts; a pair's remote steps hold the slug's teardown lock; teardown only via `internal/pairteardown`.
