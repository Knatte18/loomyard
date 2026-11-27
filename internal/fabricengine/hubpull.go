// hubpull.go holds PullHub, the hub-wide pull of Bolt, the prime pair and each local beam pair.
// Each side is reported on its own, and each pair is pulled under its code branch's parent lock.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PairLock takes the parent lock of the code branch checked out in pair and returns its release.
type PairLock func(pair *lymcwd.Location) (release func(), err error)

// PullHub brings the hub of prime up to date from origin:
// Bolt first, then the prime pair, then every local beam pair in worktree list order.
// It lets one call refresh everything this machine reads the hub through, while the hub's writers keep writing.
// Bolt is pulled under the board write lock.
// Each pair, both its code and records sides, is pulled while lockPair holds the parent lock of its code branch.
// A landing into that branch takes the same lock, and the lock is held for that one pair only.
// A local beam is a pair present here whose origin record carries the beam mark, whatever machine owns it.
// A beam whose code branch has no upstream is up to date.
// A pull honoring SkipGit leaves every side up to date.
// A side that stops, including a beam whose origin record cannot be read, never keeps the next side from running.
// The error is only a failure to list the hub's worktrees, and the sides already pulled are kept in the result.
func PullHub(prime *lymcwd.Location, opts SyncOptions, lockPair PairLock) (res fabriclayout.HubPullResult, err error) {
	rec := NewMutations(prime.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	boardDir := fabriclayout.BoltDir(prime.HubPath)
	res.Sides = append(res.Sides, pullBoardSide(boardDir, opts, rec))
	res.Sides = append(res.Sides, pullPairSide(fabriclayout.HubPullPrime, prime, opts, lockPair, rec))

	beams, err := localBeamCandidates(prime)
	if err != nil {
		return res, err
	}
	for _, beam := range beams {
		if beam.err != nil {
			res.Sides = append(res.Sides, fabriclayout.HubPullSide{Kind: fabriclayout.HubPullBeam, Worktree: beam.slug, Outcome: fabriclayout.HubPullStopped, Detail: beam.err.Error()})
			continue
		}
		res.Sides = append(res.Sides, pullPairSide(fabriclayout.HubPullBeam, beam.location, opts, lockPair, rec))
	}
	return res, nil
}

// pullBoardSide pulls Bolt at boardDir and reports it as one side, extending rec with what moved.
func pullBoardSide(boardDir string, opts SyncOptions, rec *Mutations) fabriclayout.HubPullSide {
	side := fabriclayout.HubPullSide{Kind: fabriclayout.HubPullBoard, Worktree: filepath.Base(boardDir)}
	before := rec.Len()
	skip, detail, err := NewBolt(boardDir).Pull(opts, rec)
	switch {
	case err != nil:
		side.Outcome, side.Detail = fabriclayout.HubPullStopped, err.Error()
	case skip != "":
		side.Outcome, side.Detail = fabriclayout.HubPullStopped, detail
	default:
		side.Outcome = movedOrCurrent(rec, before)
	}
	return side
}

// pullPairSide pulls the pair under its parent lock and reports it as one side of kind.
// It extends rec with what moved.
// A beam whose code branch has no upstream is reported up to date without taking the lock.
func pullPairSide(kind fabriclayout.HubPullSideKind, pair *lymcwd.Location, opts SyncOptions, lockPair PairLock, rec *Mutations) fabriclayout.HubPullSide {
	side := fabriclayout.HubPullSide{Kind: kind, Worktree: pair.WorktreeName}
	stop := func(err error) fabriclayout.HubPullSide {
		side.Outcome, side.Detail = fabriclayout.HubPullStopped, err.Error()
		return side
	}
	if opts.SkipGit {
		side.Outcome, side.Detail = fabriclayout.HubPullUpToDate, "git is skipped, so nothing was pulled"
		return side
	}
	if kind == fabriclayout.HubPullBeam {
		_, hasUpstream, err := upstreamSHAAt(pair.WorktreePath())
		if err != nil {
			return stop(err)
		}
		if !hasUpstream {
			side.Outcome, side.Detail = fabriclayout.HubPullUpToDate, "the code branch has no upstream to pull from"
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
	// A failed worktree event is a warning on the pair's line, whatever else the line says.
	withWarning := func(side fabriclayout.HubPullSide) fabriclayout.HubPullSide {
		if pullRes.SeamWarning == "" {
			return side
		}
		if side.Detail != "" {
			side.Detail += "; "
		}
		side.Detail += pullRes.SeamWarning
		return side
	}
	if pullErr != nil {
		return withWarning(stop(pullErr))
	}
	if !pullRes.WeftPulled {
		side.Outcome, side.Detail = fabriclayout.HubPullStopped, "the records side's origin could not be reached; re-run the pull once it is reachable"
		return withWarning(side)
	}
	side.Outcome = movedOrCurrent(rec, before)
	return withWarning(side)
}

// movedOrCurrent reports a side as moved when rec grew past the length before the pull, and as up to date otherwise.
func movedOrCurrent(rec *Mutations, before int) fabriclayout.HubPullOutcome {
	if rec.Len() > before {
		return fabriclayout.HubPullMoved
	}
	return fabriclayout.HubPullUpToDate
}

// beamCandidate is one local beam pair: its location, or the error that kept it from being resolved.
type beamCandidate struct {
	slug     string
	location *lymcwd.Location
	err      error
}

// localBeamCandidates lists the local pairs of prime's hub whose origin record carries the beam mark.
// They come in worktree list order.
// A pair whose origin record cannot be read or whose location cannot be resolved is listed with its error,
// so the others are still found.
// The pair's beam mark is unknown when the record is unreadable, so the pull reports it as a beam side.
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
