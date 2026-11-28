// pairpush.go holds the rules of a pair's push.
// They cover pushing the records side again until it lands and the verdict of a side that did not.
// They also judge from local refs whether a worktree's branches are ahead of origin or held there.

package fabriccore

import "github.com/Knatte18/loomyard/internal/fabriclayout"

// SeamPushSide names the seam call that pushes the records side once.
const SeamPushSide = "push_side"

// FieldRecordsLanding is what is read of the records side once a push of it returned.
const FieldRecordsLanding Field = "records_landing"

// RecordsLandingFacts is what is read of the records side after a push of it.
// Only Unpushed and Ceiling are read of a side that landed.
type RecordsLandingFacts struct {
	// Ceiling is the newest commit a push may send, empty when nothing is held.
	Ceiling string
	// Unpushed reports that the side still holds commits to send under Ceiling.
	Unpushed bool
	// DeferredSinceStart reports a deferred record written since the push started.
	DeferredSinceStart bool
	// OnOrigin reports that origin still holds the branch.
	OnOrigin bool
	// LookupTransport reports a lookup of the branch that could not reach origin.
	LookupTransport bool
	// LookupFailed reports a lookup of the branch that failed for any other reason.
	LookupFailed bool
}

// RecordsSideLanding is the rule that pushes the records side until it lands.
// It pushes the side again, after each backoff delay in turn, while the side is unpushed and origin holds its branch.
// A push that failed, a deferral and a failed lookup of the branch end it at once.
func RecordsSideLanding(snapshot Snapshot) Answer {
	push := Action{Kind: ActionSeamCall, Seam: SeamPushSide}
	pushes := 0
	var last Outcome
	for _, outcome := range snapshot.Outcomes {
		if outcome.Action.Kind == ActionSeamCall && outcome.Action.Seam == SeamPushSide {
			pushes, last = pushes+1, outcome
		}
	}
	if pushes == 0 {
		return Answer{Actions: []Action{push}, Reread: []Field{FieldRecordsLanding}}
	}
	facts := snapshot.RecordsLanding
	again := last.Class == OutcomeDone && facts.Unpushed && facts.OnOrigin &&
		!facts.DeferredSinceStart && !facts.LookupTransport && !facts.LookupFailed
	if !again || pushes > len(snapshot.Backoff) {
		return Answer{}
	}
	return Answer{Actions: []Action{waitFor(snapshot.Backoff[pushes-1]), push}, Reread: []Field{FieldRecordsLanding}}
}

// RecordsLandingVerdict classes a records side that RecordsSideLanding left unpushed.
// It returns PushFailureDeferred for a deferral since the push started or a branch origin still holds.
// It returns PushFailureTransport for a lookup that could not reach origin.
// It returns "" when no record is owed: the branch is gone from origin, or its lookup failed otherwise.
func RecordsLandingVerdict(snapshot Snapshot) fabriclayout.PushFailureClass {
	facts := snapshot.RecordsLanding
	switch {
	case facts.DeferredSinceStart:
		return fabriclayout.PushFailureDeferred
	case facts.LookupTransport:
		return fabriclayout.PushFailureTransport
	case !facts.LookupFailed && facts.OnOrigin:
		return fabriclayout.PushFailureDeferred
	default:
		return ""
	}
}

// BranchReading is a checked-out branch against its tracking ref, as last fetched.
// The three ancestry answers are read only of a tracked upstream, and only the one a caller asks for.
type BranchReading struct {
	// Tip is empty for a branch with no commits.
	Tip      string
	Upstream PushUpstream
	// LacksCommit reports that the tracking ref lacks a commit of Tip.
	LacksCommit bool
	// HoldsTip reports that the tracking ref is Tip or a descendant of it.
	HoldsTip bool
	// Ceiling is the newest commit a push may send while a later one is held, empty when nothing is held.
	Ceiling string
	// LacksCeiling reports that the tracking ref lacks Ceiling.
	LacksCeiling bool
}

// BranchAhead reports whether the branch holds a commit its tracking ref lacks.
// A branch with no commits is never ahead.
// One with a commit and no upstream is ahead, and one whose tracking ref is pruned reads level.
func BranchAhead(b BranchReading) bool {
	switch {
	case b.Tip == "":
		return false
	case b.Upstream == PushUpstreamNone:
		return true
	case b.Upstream == PushUpstreamPruned:
		return false
	}
	return b.LacksCommit
}

// BranchHeldOnOrigin reports whether the tracking ref holds the branch's tip.
// A branch with no commits is held, and so is one whose tracking ref is pruned: origin ended it.
// A branch with a commit and no upstream is not held.
func BranchHeldOnOrigin(b BranchReading) bool {
	switch {
	case b.Tip == "", b.Upstream == PushUpstreamPruned:
		return true
	case b.Upstream == PushUpstreamNone:
		return false
	}
	return b.HoldsTip
}

// ceilingLacked reports whether origin lacks the branch's ceiling.
// With no upstream it lacks every commit, and a pruned tracking ref reads level.
func ceilingLacked(b BranchReading) bool {
	switch b.Upstream {
	case PushUpstreamNone:
		return true
	case PushUpstreamPruned:
		return false
	}
	return b.LacksCeiling
}

// RunAhead reports whether a run's worktree holds commits origin lacks.
// The prime is ahead when its records branch is, since its code branch is the operator's.
// A pair is ahead when either branch is, its records branch counted only up to the ceiling when it has one.
func RunAhead(prime bool, records, code BranchReading) bool {
	if prime {
		return BranchAhead(records)
	}
	recordsAhead := BranchAhead(records)
	if records.Ceiling != "" {
		recordsAhead = ceilingLacked(records)
	}
	return recordsAhead || BranchAhead(code)
}

// TipsHeldOnOrigin reports whether origin holds the records tip, and for a pair the code tip too.
func TipsHeldOnOrigin(prime bool, records, code BranchReading) bool {
	return BranchHeldOnOrigin(records) && (prime || BranchHeldOnOrigin(code))
}
