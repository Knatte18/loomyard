// hubpull.go — PullHub, the hub-wide pull: Bolt under the board write lock, then the prime pair and each local beam pair under its code branch's parent lock, each side reported on its own.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// HubPullSideKind names which part of the hub a HubPullSide reports.
type HubPullSideKind string

// The side kinds PullHub reports, in the order it pulls them.
const (
	HubPullBoard HubPullSideKind = "board"
	HubPullPrime HubPullSideKind = "prime"
	HubPullBeam  HubPullSideKind = "beam"
)

// HubPullOutcome is what a hub pull did to one side.
type HubPullOutcome string

// The outcomes of one side of a hub pull.
const (
	// HubPullMoved means the side's local branches moved to take in origin's commits.
	HubPullMoved HubPullOutcome = "moved"
	// HubPullUpToDate means the side already held everything origin had, or has no upstream to pull from.
	HubPullUpToDate HubPullOutcome = "up_to_date"
	// HubPullStopped means the side could not be brought up to date; the detail names the way forward.
	HubPullStopped HubPullOutcome = "stopped"
)

// HubPullSide is one side of a hub pull: Bolt, the prime pair or one local beam pair.
// Worktree is the board directory's name for Bolt and the pair's worktree name otherwise.
type HubPullSide struct {
	Kind     HubPullSideKind
	Worktree string
	Outcome  HubPullOutcome
	Detail   string
}

// HubPullResult reports every side of a hub pull in pull order, with the mutation record accumulated over all of them.
type HubPullResult struct {
	MutationRecord
	Sides []HubPullSide
}

// Stopped reports whether any side of the hub pull stopped.
func (r HubPullResult) Stopped() bool {
	for _, side := range r.Sides {
		if side.Outcome == HubPullStopped {
			return true
		}
	}
	return false
}

// PairLock takes the parent lock of the code branch checked out in pair and returns its release.
type PairLock func(pair *lymcwd.Location) (release func(), err error)

// PullHub brings the hub of prime up to date from origin: Bolt first, then the prime pair, then every local beam pair in worktree list order.
// It exists so one call refreshes everything this machine reads the hub through while the orch, the dobbies and runs keep writing.
// Bolt is pulled under the board write lock, and each pair, both its code and records sides, while lockPair holds its code branch's parent lock, the lock a landing into that branch takes; the lock is held for that one pair only.
// A local beam is a pair present here whose origin record carries the beam mark, whatever machine owns it, and one whose code branch has no upstream is up to date.
// A pull honoring SkipGit leaves every side up to date.
// A side that stops, including a beam whose origin record cannot be read, never keeps the next side from running.
// The error is only a failure to list the hub's worktrees, and the sides already pulled are kept in the result.
func PullHub(prime *lymcwd.Location, opts SyncOptions, lockPair PairLock) (res HubPullResult, err error) {
	rec := NewMutations(prime.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	boardDir := BoardDir(prime.HubPath)
	res.Sides = append(res.Sides, pullBoardSide(boardDir, opts, rec))
	res.Sides = append(res.Sides, pullPairSide(HubPullPrime, prime, opts, lockPair, rec))

	beams, err := localBeamCandidates(prime)
	if err != nil {
		return res, err
	}
	for _, beam := range beams {
		if beam.err != nil {
			res.Sides = append(res.Sides, HubPullSide{Kind: HubPullBeam, Worktree: beam.slug, Outcome: HubPullStopped, Detail: beam.err.Error()})
			continue
		}
		res.Sides = append(res.Sides, pullPairSide(HubPullBeam, beam.location, opts, lockPair, rec))
	}
	return res, nil
}

// pullBoardSide pulls Bolt at boardDir and reports it as one side, extending rec with what moved.
func pullBoardSide(boardDir string, opts SyncOptions, rec *Mutations) HubPullSide {
	side := HubPullSide{Kind: HubPullBoard, Worktree: filepath.Base(boardDir)}
	before := rec.Len()
	skip, detail, err := NewBolt(boardDir).Pull(opts, rec)
	switch {
	case err != nil:
		side.Outcome, side.Detail = HubPullStopped, err.Error()
	case skip != "":
		side.Outcome, side.Detail = HubPullStopped, detail
	default:
		side.Outcome = movedOrCurrent(rec, before)
	}
	return side
}

// pullPairSide pulls the pair at pair under its parent lock and reports it as one side of kind, extending rec with what moved.
// A beam whose code branch has no upstream is reported up to date without taking the lock.
func pullPairSide(kind HubPullSideKind, pair *lymcwd.Location, opts SyncOptions, lockPair PairLock, rec *Mutations) HubPullSide {
	side := HubPullSide{Kind: kind, Worktree: pair.WorktreeName}
	stop := func(err error) HubPullSide {
		side.Outcome, side.Detail = HubPullStopped, err.Error()
		return side
	}
	if opts.SkipGit {
		side.Outcome, side.Detail = HubPullUpToDate, "git is skipped, so nothing was pulled"
		return side
	}
	if kind == HubPullBeam {
		_, hasUpstream, err := upstreamSHAAt(pair.WorktreePath())
		if err != nil {
			return stop(err)
		}
		if !hasUpstream {
			side.Outcome, side.Detail = HubPullUpToDate, "the code branch has no upstream to pull from"
			return side
		}
	}
	fab, err := Open(pair)
	if err != nil {
		return stop(err)
	}
	release, err := lockPair(pair)
	if err != nil {
		return stop(err)
	}
	defer release()

	before := rec.Len()
	pullRes, pullErr := fab.Pull(opts)
	rec.Extend(pullRes.Mutated())
	if pullErr != nil {
		return stop(pullErr)
	}
	if !pullRes.WeftPulled {
		side.Outcome, side.Detail = HubPullStopped, "the records side's origin could not be reached; re-run the pull once it is reachable"
		return side
	}
	side.Outcome = movedOrCurrent(rec, before)
	return side
}

// movedOrCurrent reports a side as moved when rec grew past before while the side was pulled, and as up to date otherwise.
func movedOrCurrent(rec *Mutations, before int) HubPullOutcome {
	if rec.Len() > before {
		return HubPullMoved
	}
	return HubPullUpToDate
}

// beamCandidate is one local beam pair found by localBeamCandidates: its location, or the error that kept it from being resolved.
type beamCandidate struct {
	slug     string
	location *lymcwd.Location
	err      error
}

// localBeamCandidates lists the pairs of prime's hub present on this machine whose origin record carries the beam mark, in worktree list order.
// A pair whose origin record cannot be read or whose location cannot be resolved is listed with its error, so the others are still found;
// as its beam mark is unknown when the record is unreadable, the pull reports it as a beam side.
// The error is only a failure to list the worktrees.
func localBeamCandidates(prime *lymcwd.Location) ([]beamCandidate, error) {
	entries, err := List(prime.AnchorPath())
	if err != nil {
		return nil, err
	}
	var beams []beamCandidate
	for _, entry := range entries {
		if entry.Main || entry.Prunable {
			continue
		}
		slug := filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path)))
		origin, found, err := ReadOriginFor(prime, slug)
		if err != nil {
			beams = append(beams, beamCandidate{slug: slug, err: fmt.Errorf("fabricengine: read the origin record of %q: %w", slug, err)})
			continue
		}
		if !found || !origin.Beam {
			continue
		}
		beam, err := lymcwd.NamedWorktree(prime.HubPath, slug)
		if err != nil {
			beams = append(beams, beamCandidate{slug: slug, err: err})
			continue
		}
		beams = append(beams, beamCandidate{slug: slug, location: beam})
	}
	return beams, nil
}
