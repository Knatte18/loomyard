# PATTERN-run-fsm

Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule;
a transition commits and the run acts at once, a background pusher bringing every commit to origin in order;
a run whose record holds another machine's takeover hands over, never replays.

## States and the act rule

Rule: Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule, over the inputs `pairteardown.ReadRun` reads.

The states and the act rule live in `internal/runfsm`.
`pairteardown.ReadRun` is their one input reader:
it reads the tracked records and the machine-local facts the act rule decides over.
A verb that acts on a run, or a view that draws its state, takes the verdict from `runfsm.Judge`.
Whether a run's work is past the merge is read from the local parent branch while origin's parent lacks the work,
since the landing's push may still be owed.

Held by `TestAct` and `TestDerive` in `internal/runfsm` and by `TestReadRun` in `internal/pairteardown`.

## A transition commits and the run acts

Rule: A transition commits and the run acts at once: a background pusher brings every commit to origin, records side first, then the code branch.

A transition commits its records, naming the code tip through the `Warp-SHA` trailer and marked a transition by its `Run-Transition` trailer.
It then starts the worktree's pusher.
It pushes, fetches and reconciles nothing and takes no gate admission.
The pusher, one per target (`internal/runpusher`), pushes the records side first and the code side after, in order, none left out.
Its restarts are the next commit, the dobby's tick and a resume.
The tracked intents and the pull-request record keep their own synchronous push.
The landing's board `done` is the parent target's, written once the parent's push landed.

`webster reset` marks a transition too, outside `statuscommit`, and is the one writer whose code push goes first:
the reset moves the code branch on origin, and only then does its records commit, which carries `Run-Transition` and names that tip.
The verb then pushes the records itself, so a push it cannot land leaves an owed record that holds the run.
Until it lands, the run's last transition on origin names a code tip origin no longer holds, so the run waits as below.

Held by `TestNew_OrdinaryPath` in `internal/statuscommit`, by `TestNewCommitStatusSeam` in `internal/battencli`,
by `TestRun` in `internal/runpusher`,
by `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide` in `internal/fabricengine`
and by `TestResetCmd_RecordsTheRunTransitionAtTheNewTip` in `internal/webstercli`.

### An owed push no longer holds the run

A push that cannot land because origin is unreachable or a teardown lock is held is owed.
The pusher retries it once per interval in force and writes no owed record, so `PushOwed` stays unset while it retries.
`webster reset`'s own records push is the exception: its owed record sets `PushOwed` until a push lands.
A refused push's record keeps its refusal and way forward.
`ReadRun` reads that record from the failed-push records naming the run into `PushOwed`.
The act rule's refusal names the record's own way forward, as `FailedPushWayForward` gives it.

Held by `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide` and `TestRun`.

## A takeover hands over

Rule: A run whose record holds another machine's takeover hands over, never replays.

A rejected records push reads the run's record at origin's fetched tip.
Another machine's takeover entry hands the run over through `pairteardown.HandOver`:
its processes end, its local tips go under owner archive tags, and its local pair is removed.
Nothing is replayed: a takeover re-runs from what origin holds.
The pair's pusher hands its run over.
The prime's pusher hands over each batten run its unpushed commits name, archiving them and replaying the rest.
A hand-over mark under the hub's `.lym` stops a second hand-over.

Held by `TestRejectedRecordsPush_ReadsTheTakeover` and `TestHandOverTips` in `internal/fabricengine`,
by `TestPushRun_HandsOverOnATakeover` in `internal/pairteardown`
and by `TestAdoptIntegration_SecondCloneTakesOver`.

## A missing code tip waits

Rule: A code tip the run's last transition names, missing on origin, makes the run wait; the owner gives it up after `runfsm.CodeTipGrace`.

`fabricengine.OriginCodeTip` reads the tip and whether origin holds it, and `ReadRun` fills the act rule's inputs from it.
A run whose pair stands here and holds the tip is not held, since its pusher owes it.
The owner's dobby pushes a tip its code branch holds and, past the grace, returns the run to its previous transition.

Held by `TestOriginCodeTip` and `TestReturnToPreviousTransition` in `internal/fabricengine`
and by `TestSettleCodeTip` in `internal/battencli`.
