# PATTERN-run-fsm

Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule;
a transition commits and pushes before the run acts, bar a run's uninterrupted start from Seed-Child to Run-Shed;
a run whose record holds another machine's takeover hands over, never replays.

## States and the act rule

Rule: Reading or acting on a run's lifecycle state: only `internal/runfsm`'s states and act rule, over the inputs `pairteardown.ReadRun` reads.

The states and the act rule live in `internal/runfsm`.
`pairteardown.ReadRun` is their one input reader:
it reads the tracked records and the machine-local facts the act rule decides over.
A verb that acts on a run, or a view that draws its state, takes the verdict from `runfsm.Judge`.

Held by `TestAct` and `TestDerive` in `internal/runfsm` and by `TestReadRun` in `internal/pairteardown`.

## A transition pushes before the run acts

Rule: A transition commits and pushes before the run acts: records side first, then the code branch.

A transition takes the gate admission through `statuscommit`.
It commits its records, naming the code tip through the `Warp-SHA` trailer and marked a transition by its `Run-Transition` trailer.
It pushes the records and then the code before the next producer starts.
The tracked intents, the pull-request record and the landing's board `done` push the same way.

Held by `TestNew_OrdinaryPath` in `internal/statuscommit` and by `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide` in `internal/fabricengine`.

### An owed push holds the run

A push that cannot land because origin is unreachable or a teardown lock is held is owed.
It holds the run at its transition, and the transition's seam retries it behind the gate.
`ReadRun` reads it from the failed-push records naming the run into `PushOwed`.
The act rule's refusal names the owed record's own way forward, as `FailedPushWayForward` gives it.

Held by `TestNew_OwedPushRetriesBehindTheGate` in `internal/statuscommit`.

### The start rows commit only

A batten run's uninterrupted start commits each transition from Seed-Child to Run-Shed and pushes none of them.
A start transition enters such a row running right after a running row before Run-Shed.
The first later transition's push carries the start's commits;
a resumed or halted start row, and one after a push that failed or was cut off, pushes as any transition does.
A machine that adopts the run before that push reads it at its last pushed row and runs the start rows again,
and the starting machine's next push meets the takeover and hands the run over.

Held by `TestNewCommitStatusSeam` in `internal/battencli`.

## A takeover hands over

Rule: A run whose record holds another machine's takeover hands over, never replays.

A rejected records push reads the run's record at origin's fetched tip.
Another machine's takeover entry hands the run over through `pairteardown.HandOver`:
its processes end, its local tips go under owner archive tags, and its local pair is removed.
Nothing is replayed.
A hand-over mark under the hub's `.lym` stops a second hand-over.

Held by `TestRejectedRecordsPush_ReadsTheTakeover` and `TestHandOverTips` in `internal/fabricengine`
and by `TestPushRun_HandsOverOnATakeover` in `internal/pairteardown`.

## A missing code tip waits

Rule: A code tip the run's last transition names, missing on origin, makes the run wait; the owner gives it up after `runfsm.CodeTipGrace`.

`fabricengine.OriginCodeTip` reads the tip and whether origin holds it, and `ReadRun` fills the act rule's inputs from it.
The owner's dobby pushes a tip its code branch holds and, past the grace, returns the run to its previous transition.

Held by `TestOriginCodeTip` and `TestReturnToPreviousTransition` in `internal/fabricengine`
and by `TestSettleCodeTip` in `internal/battencli`.
