// pushside.go holds the rules of one side's push.
// They cover the transport retry, the lookup of the branch on origin and the retry of a rejection.
// They also class a failure and word the refusal of a board that was never seeded onto origin.

package fabriccore

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// The classes an action of a side's push ends in, besides the general ones.
const (
	// OutcomeTakeover is a step stopped because another machine takes the run over.
	OutcomeTakeover OutcomeClass = "takeover"
	// OutcomeProtected is a push the remote rejected because the branch is protected.
	OutcomeProtected OutcomeClass = "protected"
	// OutcomeProtectedUnrejected is a protected-branch refusal of a ref change that is no rejection of a push.
	OutcomeProtectedUnrejected OutcomeClass = "protected_unrejected"
	// OutcomeReplayConflict is a replay of local commits that stopped on a conflict.
	OutcomeReplayConflict OutcomeClass = "replay_conflict"
	// OutcomeOriginAhead is a retry that found origin holding commits the local branch lacks.
	OutcomeOriginAhead OutcomeClass = "origin_ahead"
	// OutcomeDeferred is a retry that left the commits to the branch's next push.
	OutcomeDeferred OutcomeClass = "deferred"
	// OutcomeRetriesExhausted is a push the remote refused retryably until its attempts ran out.
	OutcomeRetriesExhausted OutcomeClass = "retries_exhausted"
)

const (
	// FieldPushSide is what a side's push reads of its branch, its repository and origin.
	// The read phase decides which part: the branch and repository, then origin, then the unpushed commits.
	FieldPushSide Field = "push_side"
	// FieldPushJudgment is what a rejection the retries did not settle is judged by.
	FieldPushJudgment Field = "push_judgment"
)

// SeamRejectionRetry names the seam call that retries a rejected push.
const SeamRejectionRetry = "rejection_retry"

// PushUpstream is the state of a branch's upstream before its push.
type PushUpstream string

const (
	// PushUpstreamNone is a branch with no upstream, which was never pushed.
	PushUpstreamNone PushUpstream = "none"
	// PushUpstreamPruned is a branch with an upstream whose tracking ref is gone.
	PushUpstreamPruned PushUpstream = "pruned"
	// PushUpstreamTracked is a branch with an upstream and its tracking ref.
	PushUpstreamTracked PushUpstream = "tracked"
)

// PushSideFacts is what the push of one side reads.
type PushSideFacts struct {
	// Read reports that the branch and repository were read.
	Read bool
	// Side is "code", "records" or "bolt".
	Side string
	// Branch is empty when the checked-out branch cannot be read.
	Branch string
	// Prime reports a code side pushed from the prime worktree.
	Prime bool
	// SingleAttempt is the push's option that retries nothing over a transport failure.
	SingleAttempt bool
	// NamespaceErr is the failed read of the repository's origin layout.
	NamespaceErr error
	Upstream     PushUpstream
	// UpstreamErr is a failed read of the upstream that names none of its states.
	UpstreamErr error
	// UnpushedRead reports that Unpushed and UnpushedErr were read.
	UnpushedRead bool
	Unpushed     bool
	UnpushedErr  error
	Origin       PushOriginView
	Board        PushBoardFacts
	Judgment     PushJudgmentFacts
}

// PushOriginView is what the lookup of a branch on origin found.
// The zero value is a lookup that has not answered.
type PushOriginView struct {
	Read bool
	// Head is origin's commit of a code branch.
	Head string
	// Gone says origin lacks the branch, and Ended that its entry is ended on origin.
	Gone  bool
	Ended bool
	// Unchecked says the lookup failed for no reason of reaching origin, so nothing is known of the branch.
	Unchecked bool
	// JudgeErr is a failed judgment of a gone branch's entry.
	JudgeErr error
	// Rewound says origin reset the branch back past the commits in Dropped.
	Rewound bool
	Dropped []string
}

// PushBoardFacts is what the refusal of an unseeded board reads.
// The zero value is a board that is not refused.
type PushBoardFacts struct {
	// Unseeded reports a board with no upstream in a one-repository records store.
	Unseeded bool
	Path     string
	HubPath  string
	// DiscardWorkFlag is the clone's flag, without its dashes, that lets --reset discard unpushed work.
	DiscardWorkFlag string
	// OriginReadErr is a failed read of origin's board.
	OriginReadErr error
	OriginFound   bool
	OriginTip     string
	FetchErr      error
	// NoCommits reports a local board that holds no commits.
	NoCommits bool
	LocalTip  string
	// CompareErr is a failed comparison of origin's board with the local one.
	CompareErr error
	// Shared reports that the two boards share history.
	Shared           bool
	RecordsBranchErr error
	Remote           string
	// Upstreams are the upstream settings the seeding clone never wrote.
	Upstreams []PushUpstreamSetting
}

// PushUpstreamSetting is one branch's upstream: its name and the ref it merges.
type PushUpstreamSetting struct {
	Branch   string
	MergeRef string
}

// PushJudgmentFacts is what a rejection the retries did not settle is judged by.
// The zero value has not been read.
type PushJudgmentFacts struct {
	Read bool
	// Unmade is a commit on origin's branch that this machine did not make, which refused a reset.
	// UnmadeAt is origin's tip then.
	Unmade   string
	UnmadeAt string
	// Err is a failed fetch or tip read, which leaves origin unknown.
	Err error
	// OriginHoldsMore reports that origin holds commits the branch lacks.
	OriginHoldsMore bool
}

// PushVerdictKind is how a side's push ends.
type PushVerdictKind string

const (
	// PushVerdictDone ends the push with nothing more to do.
	PushVerdictDone PushVerdictKind = "done"
	// PushVerdictReturn returns an error as it is, with no record.
	PushVerdictReturn PushVerdictKind = "return"
	// PushVerdictLanded records the landed push and clears the side's failed-push record.
	PushVerdictLanded PushVerdictKind = "landed"
	// PushVerdictCleared clears the side's failed-push record, since nothing was unpushed.
	PushVerdictCleared PushVerdictKind = "cleared"
	// PushVerdictDeferred records a deferral.
	PushVerdictDeferred PushVerdictKind = "deferred"
	// PushVerdictRecord records a failed push under Class.
	PushVerdictRecord PushVerdictKind = "record"
)

// PushVerdict is how a side's push ends and what its error rests on.
type PushVerdict struct {
	Kind  PushVerdictKind
	Class fabriclayout.PushFailureClass
	// Outcome is the index of the outcome whose error the verdict rests on, or -1.
	Outcome int
	// Rejection is the index of a rejection whose error is joined ahead of Outcome's, or -1.
	Rejection int
	// Err is an error of the facts that the verdict returns, records or joins.
	Err error
	// NothingLacking reports a rejection after which origin holds nothing the branch lacks.
	NothingLacking bool
	// LocalCommits asks for the local commits origin lacks to be recorded.
	LocalCommits bool
	// Commits and OriginHead are recorded as they are.
	Commits    []string
	OriginHead string
}

// PushSide is the rule of one side's push, from the first read to the push's end.
func PushSide(snapshot Snapshot) Answer {
	answer, _ := stepPush(snapshot)
	return answer
}

// JudgePush returns how the push of a snapshot ends, once the push's rule answers nothing more to do.
func JudgePush(snapshot Snapshot) PushVerdict {
	_, verdict := stepPush(snapshot)
	return verdict
}

// UnseededBoardRefusal refuses a board push for a board with no upstream in a one-repository records store.
// It returns nil for any other board.
// Such a board is a --single-repo seed whose clone stopped before it set the upstreams.
// A push of it would put an unseeded board onto origin unleased.
// Origin holding no board names a --reset --discard-work clone, which discards the unpushed seed.
// Origin holding a board that shares history with this one names the upstream settings the clone never wrote.
// Origin holding an unrelated board, or any board while this one has no commits, names a join of the hub that seeded it.
func UnseededBoardRefusal(facts PushSideFacts) *Refusal {
	board := facts.Board
	if !board.Unseeded {
		return nil
	}
	refused := fmt.Sprintf("fabricengine: the board at %s has no upstream in a one-repository hub, so it is not pushed", board.Path)
	refuse := func(reason, wayForward string) *Refusal {
		return &Refusal{Reason: refused + reason, WayForward: wayForward}
	}
	const onceOriginAnswers = "way forward: run the same command again once origin answers"
	join := fmt.Sprintf("way forward: remove the hub at %s and run `lym fabric clone <url>` without `--single-repo` to join that hub", board.HubPath)

	switch {
	case board.OriginReadErr != nil:
		return refuse(fmt.Sprintf(", and origin's board could not be read to tell whether its seed landed: %v", board.OriginReadErr), onceOriginAnswers)
	case !board.OriginFound:
		return refuse(fmt.Sprintf(", and origin holds no board under %s, so it was never seeded onto origin", fabriclayout.RecordsNamespaceRoot),
			fmt.Sprintf("way forward: run the same `lym fabric clone --single-repo` again with `--reset --%s`", board.DiscardWorkFlag))
	case board.FetchErr != nil:
		return refuse(fmt.Sprintf(", and origin's board could not be fetched to compare it with this board: %v", board.FetchErr), onceOriginAnswers)
	case board.NoCommits:
		return refuse(fmt.Sprintf(", and origin holds another board at %s while this board holds no commits, so another clone seeded origin", board.OriginTip), join)
	case board.CompareErr != nil:
		return refuse(fmt.Sprintf(", and origin's board could not be compared with this board: %v", board.CompareErr),
			fmt.Sprintf("way forward: run the same command again once the records store at %s reads", board.Path))
	case !board.Shared:
		return refuse(fmt.Sprintf(", and origin holds another board at %s, which shares no history with this board at %s, so another clone seeded origin",
			board.OriginTip, board.LocalTip), join)
	case board.RecordsBranchErr != nil:
		return refuse(fmt.Sprintf(", and %s could not be read from the records store at %s to name the upstream settings: %v",
			fabriclayout.PrimeRecordsBranchKey, board.Path, board.RecordsBranchErr),
			fmt.Sprintf("way forward: set %s there to the prime's records branch, then run the same command again", fabriclayout.PrimeRecordsBranchKey))
	}
	var settings []string
	for _, upstream := range board.Upstreams {
		settings = append(settings,
			fmt.Sprintf("`git -C %s config branch.%s.remote %s`", board.Path, upstream.Branch, board.Remote),
			fmt.Sprintf("`git -C %s config branch.%s.merge %s`", board.Path, upstream.Branch, upstream.MergeRef))
	}
	return &Refusal{
		Reason: fmt.Sprintf("%s, but origin holds this hub's board at %s, so the seed landed and its clone stopped before it set the upstreams",
			refused, board.OriginTip),
		WayForward: fmt.Sprintf("way forward: set them with %s, then run the same command again", strings.Join(settings, ", ")),
	}
}

// pushProgress sorts the outcomes of a side's push by what they ran.
type pushProgress struct {
	// originWaits counts the waits before the first push.
	originWaits int
	// pushes, retries and refDeletes hold the outcome indexes of each kind, in order.
	pushes     []int
	retries    []int
	refDeletes []int
}

func readPushProgress(outcomes []Outcome) pushProgress {
	var progress pushProgress
	for i, outcome := range outcomes {
		switch outcome.Action.Kind {
		case ActionWait:
			if len(progress.pushes) == 0 {
				progress.originWaits++
			}
		case ActionPush:
			progress.pushes = append(progress.pushes, i)
		case ActionSeamCall:
			progress.retries = append(progress.retries, i)
		case ActionRefDelete:
			progress.refDeletes = append(progress.refDeletes, i)
		}
	}
	return progress
}

// ending returns the verdict of kind, resting on no outcome.
func ending(kind PushVerdictKind) PushVerdict {
	return PushVerdict{Kind: kind, Outcome: -1, Rejection: -1}
}

func failedAs(class fabriclayout.PushFailureClass) PushVerdict {
	verdict := ending(PushVerdictRecord)
	verdict.Class = class
	return verdict
}

// finish answers that nothing is left to do, with verdict.
func finish(verdict PushVerdict) (Answer, PushVerdict) {
	return Answer{}, verdict
}

// returnOutcome ends the push with the error of the outcome at index, or as done when it succeeded.
func returnOutcome(outcomes []Outcome, index int) (Answer, PushVerdict) {
	if outcomes[index].Class == OutcomeDone {
		return finish(ending(PushVerdictDone))
	}
	verdict := ending(PushVerdictReturn)
	verdict.Outcome = index
	return finish(verdict)
}

func returnError(err error) (Answer, PushVerdict) {
	verdict := ending(PushVerdictReturn)
	verdict.Err = err
	return finish(verdict)
}

func run(actions ...Action) (Answer, PushVerdict) {
	return Answer{Actions: actions}, PushVerdict{}
}

func readPhase(phase Phase, field Field) (Answer, PushVerdict) {
	return Answer{Phase: phase, Read: []Field{field}}, PushVerdict{}
}

func stepPush(snapshot Snapshot) (Answer, PushVerdict) {
	facts := snapshot.Push
	if !facts.Read {
		return readPhase(PhaseUnderLock, FieldPushSide)
	}
	progress := readPushProgress(snapshot.Outcomes)
	push := Action{Kind: ActionPush, Store: Store(facts.Side), Refspec: facts.Branch}
	if facts.Branch == "" {
		if len(progress.pushes) == 0 {
			return run(push)
		}
		return returnOutcome(snapshot.Outcomes, progress.pushes[0])
	}

	if facts.Side == fabriclayout.RepoBolt {
		if refusal := UnseededBoardRefusal(facts); refusal != nil {
			return Answer{Refusal: refusal}, PushVerdict{}
		}
		if facts.NamespaceErr != nil {
			return returnError(facts.NamespaceErr)
		}
	} else {
		if facts.NamespaceErr != nil {
			verdict := failedAs(fabriclayout.PushFailureOther)
			verdict.Err = facts.NamespaceErr
			return finish(verdict)
		}
		if facts.UpstreamErr != nil {
			return returnError(facts.UpstreamErr)
		}
		if facts.Upstream != PushUpstreamNone {
			if answer, verdict, stopped := judgePushOrigin(snapshot, progress); stopped {
				return answer, verdict
			}
		}
	}

	if !facts.UnpushedRead {
		return readPhase(PhaseUnderLock, FieldPushSide)
	}
	if facts.Side == fabriclayout.RepoBolt && facts.UnpushedErr == nil && !facts.Unpushed {
		return finish(ending(PushVerdictCleared))
	}
	if len(progress.pushes) == 0 {
		return run(push)
	}
	if len(progress.retries) > 0 {
		return judgeRetry(snapshot, progress)
	}

	last := progress.pushes[len(progress.pushes)-1]
	switch snapshot.Outcomes[last].Class {
	case OutcomeDone:
		return finish(ending(PushVerdictLanded))
	case OutcomeTransportFailure:
		if retried := len(progress.pushes) - 1; !facts.SingleAttempt && retried < len(snapshot.Backoff) {
			return run(waitFor(snapshot.Backoff[retried]), push)
		}
	case OutcomeRejected, OutcomeProtected:
		return run(retryRejection())
	}
	return judgePushFailure(snapshot, last, -1)
}

func waitFor(delay time.Duration) Action {
	return Action{Kind: ActionWait, Delay: delay}
}

func retryRejection() Action {
	return Action{Kind: ActionSeamCall, Seam: SeamRejectionRetry}
}

// judgePushOrigin looks at the lookup of the branch on origin.
// stopped is false when the push goes on.
// A gone branch whose entry is not ended and a lookup that failed in transit are retried once per backoff delay.
func judgePushOrigin(snapshot Snapshot, progress pushProgress) (answer Answer, verdict PushVerdict, stopped bool) {
	stop := func(answer Answer, verdict PushVerdict) (Answer, PushVerdict, bool) {
		return answer, verdict, true
	}
	facts := snapshot.Push
	view := facts.Origin
	failed := !view.Read && slices.ContainsFunc(snapshot.ReadFailures, func(f ReadFailure) bool {
		return f.Phase == PhaseOrigin && f.Field == FieldPushSide
	})
	if !view.Read && !failed {
		return stop(readPhase(PhaseOrigin, FieldPushSide))
	}
	again := func() (Answer, PushVerdict, bool) {
		return Answer{Actions: []Action{waitFor(snapshot.Backoff[progress.originWaits])}, Reread: []Field{FieldPushSide}}, PushVerdict{}, true
	}
	spent := progress.originWaits >= len(snapshot.Backoff)

	switch {
	case failed:
		if facts.SingleAttempt || spent {
			return stop(finish(failedAs(fabriclayout.PushFailureTransport)))
		}
		return again()
	case view.Unchecked:
		return Answer{}, PushVerdict{}, false
	case view.JudgeErr != nil:
		return stop(returnError(view.JudgeErr))
	case view.Ended:
		if len(progress.refDeletes) == 0 {
			return stop(run(Action{Kind: ActionRefDelete, Store: Store(facts.Side)}))
		}
		return stop(returnOutcome(snapshot.Outcomes, progress.refDeletes[0]))
	case view.Gone:
		if spent {
			return stop(finish(failedAs(fabriclayout.PushFailureGone)))
		}
		return again()
	}
	if facts.Side == fabriclayout.RepoCode && !facts.Prime && facts.Upstream != PushUpstreamPruned && view.Rewound {
		rewound := failedAs(fabriclayout.PushFailureRewound)
		rewound.Commits, rewound.OriginHead = view.Dropped, view.Head
		return stop(finish(rewound))
	}
	return Answer{}, PushVerdict{}, false
}

// judgeRetry decides what follows the last retry of a rejected push.
// A retry runs again while it ends in a bare rejection or an unnamed failure, once per backoff delay.
func judgeRetry(snapshot Snapshot, progress pushProgress) (Answer, PushVerdict) {
	rejection := progress.pushes[len(progress.pushes)-1]
	last := progress.retries[len(progress.retries)-1]
	switch snapshot.Outcomes[last].Class {
	case OutcomeDone:
		return finish(ending(PushVerdictLanded))
	case OutcomeDeferred:
		return finish(ending(PushVerdictDeferred))
	case OutcomeTakeover:
		return returnOutcome(snapshot.Outcomes, last)
	case OutcomeOriginAhead:
		return judgeRejection(snapshot, rejection, -1)
	case OutcomeRejected, OutcomeFailed:
		if retried := len(progress.retries) - 1; retried < len(snapshot.Backoff) {
			return run(waitFor(snapshot.Backoff[retried]), retryRejection())
		}
		if snapshot.Outcomes[last].Class == OutcomeFailed {
			// An unnamed failure keeps the record reading as a rejection and names the retry's error.
			return judgeRejection(snapshot, last, rejection)
		}
	}
	return judgePushFailure(snapshot, last, -1)
}

// judgePushFailure classes the failure of the outcome at index, joined onto the rejection at joined when that is not -1.
func judgePushFailure(snapshot Snapshot, index, joined int) (Answer, PushVerdict) {
	var class fabriclayout.PushFailureClass
	localCommits := false
	switch snapshot.Outcomes[index].Class {
	case OutcomeAnchorStop:
		class = fabriclayout.PushFailureAnchorChange
	case OutcomeAuthRefused:
		class = fabriclayout.PushFailureAuth
	case OutcomeRejected, OutcomeProtected:
		return judgeRejection(snapshot, index, joined)
	case OutcomeReplayConflict, OutcomeDestructiveRefusal:
		class, localCommits = fabriclayout.PushFailureNotFastForward, true
	case OutcomeTransportFailure, OutcomeRetriesExhausted:
		class = fabriclayout.PushFailureTransport
	default:
		class = fabriclayout.PushFailureOther
	}
	verdict := failedAs(class)
	verdict.Outcome, verdict.Rejection, verdict.LocalCommits = index, joined, localCommits
	return finish(verdict)
}

// judgeRejection classes a rejection the retries did not settle by what origin holds.
// Only origin holding commits the branch lacks, or a commit this machine did not make, is a divergence.
func judgeRejection(snapshot Snapshot, index, joined int) (Answer, PushVerdict) {
	judgment := snapshot.Push.Judgment
	if !judgment.Read {
		return readPhase(PhaseOrigin, FieldPushJudgment)
	}
	verdict := failedAs(fabriclayout.PushFailureOther)
	verdict.Outcome, verdict.Rejection = index, joined
	switch {
	case judgment.Unmade != "":
		verdict.Class = fabriclayout.PushFailureNotFastForward
		verdict.Commits, verdict.OriginHead = []string{judgment.Unmade}, judgment.UnmadeAt
	case judgment.Err != nil:
		verdict.Err = judgment.Err
	case judgment.OriginHoldsMore:
		verdict.Class, verdict.LocalCommits = fabriclayout.PushFailureNotFastForward, true
	default:
		verdict.NothingLacking = true
	}
	return finish(verdict)
}
