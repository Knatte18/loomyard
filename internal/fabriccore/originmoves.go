// originmoves.go holds the rules of moving the branches a reconcile judged.
// They give the order, say whether each branch is moved and judge a records branch once its locks are held.

package fabriccore

import (
	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// MoveTarget is one judged branch as the order of the moves sees it.
type MoveTarget struct {
	// Repo is the repository side: code, records or bolt.
	Repo  string
	Slug  string
	Prime bool
}

// MoveOrder returns the indexes into targets in the order their branches are moved.
// Bolt goes first.
// The prime's records branch goes next, before its code branch.
// Each pair of pairs follows in turn, its records before its code.
// The three groups are returned apart, since the prime's move under a lock of their own.
func MoveOrder(targets []MoveTarget, pairs []string) (bolt, prime, pair []int) {
	of := func(repo string, pick func(MoveTarget) bool) []int {
		var found []int
		for i, target := range targets {
			if target.Repo == repo && pick(target) {
				found = append(found, i)
			}
		}
		return found
	}
	every := func(MoveTarget) bool { return true }
	isPrime := func(target MoveTarget) bool { return target.Prime }

	bolt = of(fabriclayout.RepoBolt, every)
	prime = append(of(fabriclayout.RepoRecords, isPrime), of(fabriclayout.RepoCode, isPrime)...)
	for _, slug := range pairs {
		inPair := func(target MoveTarget) bool { return target.Slug == slug && !target.Prime }
		pair = append(pair, of(fabriclayout.RepoRecords, inPair)...)
		pair = append(pair, of(fabriclayout.RepoCode, inPair)...)
	}
	return bolt, prime, pair
}

// MoveFacts is what is known of one judged branch before it is moved.
type MoveFacts struct {
	State fabriclayout.BranchState
	// Run is the slug of the run the scope covers, empty for a hub scope.
	Run string
	// Repo is the repository side: code, records or bolt.
	Repo string
	Slug string
	// OwnedByEnded reports that the removal of an ended run owns the pair.
	OwnedByEnded bool
	// OwnedElsewhere reports that another machine owns the entry.
	OwnedElsewhere bool
	// BusyLock names the held lock that keeps the branch from moving, empty when none does.
	BusyLock string
	// LiveProbed reports that Live was read.
	LiveProbed bool
	Live       bool
	// MoveStanding reports that a move record stands on the records worktree.
	MoveStanding bool
	// AnchorStopped reports that the anchor marker holds the records branch back.
	AnchorStopped bool
}

// MoveStep is what is done with one judged branch.
type MoveStep string

const (
	// MoveNone leaves the branch as it was judged.
	MoveNone MoveStep = "none"
	// MoveBusy marks the branch busy on the held lock.
	MoveBusy MoveStep = "busy"
	// MoveProbeLive asks whether the pair is live before a verdict can be given.
	MoveProbeLive MoveStep = "probe_live"
	// MoveLive leaves the branch of a live pair.
	MoveLive MoveStep = "live"
	// MoveAnchorStopped leaves a records branch its anchor marker or a move record holds back.
	MoveAnchorStopped MoveStep = "anchor_stopped"
	// MoveFastForward fast-forwards a code branch.
	MoveFastForward MoveStep = "fast_forward"
	// MoveRecords brings a records branch up to origin under its locks.
	MoveRecords MoveStep = "records"
)

// MoveVerdict returns what is done with a judged branch, from what is known of it so far.
// Only a branch behind origin, or a records branch needing a replay, moves.
// A run scope moves Bolt and its own pair alone.
// A pair the removal of ended runs owns is not touched.
// Nor is a records branch needing a replay of an entry another machine owns.
// A held lock makes the branch busy.
// A pair other than the scope's own is first asked whether it is live, and a live one is left.
// A records branch a move record or the anchor marker holds back is anchor stopped.
// The two are facts of the records worktree, read once the verdict would otherwise move it.
func MoveVerdict(facts MoveFacts) MoveStep {
	switch {
	case facts.State != fabriclayout.BranchFastForward && facts.State != fabriclayout.BranchNeedsReplay:
		return MoveNone
	case facts.Run != "" && facts.Repo != fabriclayout.RepoBolt && facts.Slug != facts.Run:
		return MoveNone
	case facts.OwnedByEnded:
		return MoveNone
	case facts.OwnedElsewhere && facts.State == fabriclayout.BranchNeedsReplay:
		return MoveNone
	case facts.BusyLock != "":
		return MoveBusy
	case facts.Slug != "" && facts.Slug != facts.Run && !facts.LiveProbed:
		return MoveProbeLive
	case facts.Live:
		return MoveLive
	case facts.Repo == fabriclayout.RepoRecords && (facts.MoveStanding || facts.AnchorStopped):
		return MoveAnchorStopped
	case facts.Repo == fabriclayout.RepoCode:
		return MoveFastForward
	default:
		return MoveRecords
	}
}

// RecordsMoveFacts is what is read of a records branch once its locks were asked for.
// The ancestry answers are read only as far as the verdict needs them, and a replay's only once one ran.
type RecordsMoveFacts struct {
	// BusyLock names the lock another holder has, empty when the locks are held.
	BusyLock string
	// PushLockHeld reports that the held lock is the push lock.
	PushLockHeld bool
	// Level reports that the branch is at origin's head.
	Level  bool
	Behind bool
	Ahead  bool
	// Replayed reports that the local commits were replayed onto origin's head.
	Replayed bool
	// ReplayDirty reports a replay an uncommitted change blocked.
	ReplayDirty bool
	// ReplayConflict reports a replay that stopped on a conflict.
	ReplayConflict bool
	// Paths are the uncommitted paths of a dirty replay, or the conflicting paths.
	Paths []string
	// Commits are the local commits a conflicting replay could not apply.
	Commits []string
}

// RecordsMoveState returns the verdict on a records branch.
// BranchFastForward asks for the fast-forward and BranchNeedsReplay for the replay; every other state stands.
// A replayed branch is level when it reached origin's head and ahead when commits of its own sit on it.
// It counts as moved either way.
func RecordsMoveState(facts RecordsMoveFacts) fabriclayout.BranchOutcome {
	var outcome fabriclayout.BranchOutcome
	switch {
	case facts.BusyLock != "":
		outcome.State = fabriclayout.BranchBusy
		outcome.BusyLock = facts.BusyLock
		outcome.PushLockHeld = facts.PushLockHeld
	case facts.Replayed:
		outcome.State = fabriclayout.BranchAhead
		if facts.Level {
			outcome.State = fabriclayout.BranchLevel
		}
		outcome.Moved = true
	case facts.ReplayDirty:
		outcome.State = fabriclayout.BranchDirty
		outcome.Paths = facts.Paths
	case facts.ReplayConflict:
		outcome.State = fabriclayout.BranchDiverged
		outcome.Paths = facts.Paths
		outcome.Commits = facts.Commits
	case facts.Level:
		outcome.State = fabriclayout.BranchLevel
	case facts.Behind:
		outcome.State = fabriclayout.BranchFastForward
	case facts.Ahead:
		outcome.State = fabriclayout.BranchAhead
	default:
		outcome.State = fabriclayout.BranchNeedsReplay
	}
	return outcome
}
