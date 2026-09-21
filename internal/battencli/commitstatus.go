// commitstatus.go implements batten's own CommitStatus seam: the shedengine.Shed hook that commits
// prime's own batten status.json onto prime's own fabric pair after each transition.
//
// Modelled on internal/loomcli/wiring.go's newCommitStatusSeam/loomCommitStatusDeps pair, whose
// three-case disposition -- skip-while-mid-merge, commit-hard-errors, push-warns -- applies here
// verbatim rather than being reinvented. This is not fabricengine.Bolt: Bolt.Commit stages every
// change in its repo, which the Fabric Git Invariant forbids a fabric-commit caller, and Bolt is the
// Board's own carve-out scoped to the Board directory. Batten does not take the Board's push lock
// either; a push rejected because a Board write advanced the branch takes the same disposition the
// loom seam already applies to every push error, including gitrepo.ErrPushRejected: warn and let
// the next transition catch the branch up.
//
// On top of loomcli's own dispositions, this seam adds one more, evaluated first: an on-disk
// no-op-transition skip. Each `lyx batten step` is a fresh process, so the skip's memory cannot live
// in a closure variable -- it would start empty every call and the skip would work for `run` mode
// alone. It lives instead at shedrun.LastCommitMarker(location, runID), read before deciding and
// rewritten after a successful commit.
package battencli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// commitStatusDeps carries the three fabric calls newCommitStatusSeam drives, injected as plain
// function values rather than reached directly, so the seam's branching is drivable in a Tier 1
// test from stub closures, with no hub fixture and no git spawn -- exactly as loomcli's own
// commitStatusDeps.
type commitStatusDeps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is
	// mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits batten's own status file with msg.
	Commit func(msg string) error
	// Push pushes the fabric sibling worktree's unpushed commits.
	Push func() error
}

// battenRunCommitPaths returns the anchor-relative paths a status transition commits: the run's
// status file, and its seed whenever one is on disk.
//
// Both belong on the pair because a run directory is durable, fabric-synced state -- a status
// committed without its seed leaves a resumed machine able to read how far the run came but not
// what it is running, and batten's own auto-seed would then re-seed it from flag defaults.
// The seed goes in on every transition rather than once at seeding time, for the self-healing
// reason loom's own bootstrap commit paths record: committing an already-clean tracked path is a
// no-op, so a seed written by an invocation that crashed before any commit lands on the next one.
//
// It is included only when present because a pathspec matching no file is a hard git error, and a
// Shed driven without batten's own auto-seed -- which every caller outside the CLI verbs is -- has
// no seed to commit.
func battenRunCommitPaths(location *lyxcwd.Location, runID string) []string {
	paths := []string{shedrun.StatusRel(runID)}
	if _, err := os.Stat(shedrun.SeedFile(location, runID)); err == nil {
		paths = append(paths, shedrun.SeedRel(runID))
	}
	return paths
}

// battenCommitStatusDeps builds a commitStatusDeps over location and runID, filling each field from
// fabric: MergeActive from fabricengine.MergeStateActive, Commit from
// fabricengine.CommitAnchoredPaths scoped to battenRunCommitPaths(location, runID), and Push from
// fabricengine.PushAnchored.
func battenCommitStatusDeps(location *lyxcwd.Location, runID string) commitStatusDeps {
	return commitStatusDeps{
		MergeActive: func() (bool, error) {
			return fabricengine.MergeStateActive(location)
		},
		// Commit discards the (sha, committed) pair in favour of the error alone, exactly as
		// loomCommitStatusDeps' own Commit closure does.
		Commit: func(msg string) error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, battenRunCommitPaths(location, runID), msg, fabricengine.EnvSyncOptions())
			return err
		},
		Push: func() error {
			_, err := fabricengine.PushAnchored(location, fabricengine.EnvSyncOptions())
			return err
		},
	}
}

// commitStatusMessage renders the commit message for a per-transition status commit, exactly as
// loomcli's own commitStatusMessage does, with batten's own prefix.
func commitStatusMessage(producer, st string) string {
	return fmt.Sprintf("batten: %s -> %s", producer, st)
}

// commitStatusFailureDisposition decides what a failed status Commit means, mirroring
// loomcli's own commitStatusFailureDisposition: the mid-merge probe is unlocked by construction,
// so a merge can become live in the window between MergeActive answering false and the commit
// running, and when it does the commit fails on git's own "cannot do a partial commit during a
// merge" -- a path-scoped commit is a partial commit by definition. Re-probing here turns that
// failure back into the skip it was always meant to be. Every other commit failure keeps the
// hard-error disposition: a git fault on the run's own bookkeeping with no merge to explain it is
// real infrastructure breakage.
func commitStatusFailureDisposition(deps commitStatusDeps, producer, st string, commitErr error) error {
	active, probeErr := deps.MergeActive()
	if probeErr != nil {
		logger.Warn("battencli: status commit failed and the merge-state re-probe failed too; continuing",
			"producer", producer, "state", st, "commit_error", commitErr, "probe_error", probeErr)
		return nil
	}
	if active {
		logger.Warn("battencli: status commit failed because the fabric sibling went mid-merge after the probe; skipping this transition",
			"producer", producer, "state", st, "error", commitErr)
		return nil
	}
	return commitErr
}

// commitStatusMarker is the on-disk shape newCommitStatusSeam persists at markerPath: the last
// (producer, state) pair the seam actually committed. It is the no-op-transition skip's memory,
// read before deciding and rewritten after a successful commit -- never held in a closure variable,
// so the skip survives a fresh `lyx batten step` process rebuilding the seam from scratch between
// calls.
type commitStatusMarker struct {
	Producer string `json:"producer"`
	State    string `json:"state"`
}

// newCommitStatusSeam builds the shedengine.Shed.CommitStatus closure from deps, wrapped in an
// on-disk no-op-transition skip read from and written to markerPath (locked via markerLockPath).
//
// Evaluation order: the no-op-transition skip first -- when the incoming (producer, state) pair
// equals the marker's last-committed pair, return nil without committing or pushing -- then
// loomcli's newCommitStatusSeam's own three dispositions, unchanged: skip-while-mid-merge,
// commit-hard-errors, push-warns.
//
// A missing or corrupt marker falls back to committing once rather than erroring: the marker is a
// cache, and losing it costs one redundant commit, never correctness. The marker is rewritten right
// after a successful Commit, ahead of the Push attempt, so a subsequent call skips re-committing the
// same pair even if the push that followed is still only warned about, not retried.
func newCommitStatusSeam(deps commitStatusDeps, markerPath, markerLockPath string) func(producer, st string) error {
	return func(producer, st string) error {
		marker, found, err := state.ReadJSON[commitStatusMarker](markerPath, markerLockPath)
		if err != nil {
			logger.Warn("battencli: status commit marker unreadable; treating it as absent", "producer", producer, "state", st, "error", err)
			found = false
		}
		if found && marker.Producer == producer && marker.State == st {
			return nil
		}

		active, err := deps.MergeActive()
		if err != nil {
			logger.Warn("battencli: skip status commit, merge-state probe failed", "producer", producer, "state", st, "error", err)
			return nil
		}
		if active {
			logger.Warn("battencli: skip status commit, fabric sibling is mid-merge", "producer", producer, "state", st)
			return nil
		}

		if err := deps.Commit(commitStatusMessage(producer, st)); err != nil {
			return commitStatusFailureDisposition(deps, producer, st, err)
		}

		// Rewritten unconditionally on a successful commit. A failed marker write only costs one
		// redundant commit on the next call -- the same cost a missing marker costs on read -- so it
		// is warned about rather than escalated.
		if err := state.WriteJSON(markerPath, markerLockPath, commitStatusMarker{Producer: producer, State: st}); err != nil {
			logger.Warn("battencli: status commit succeeded but the marker write failed; a future call may recommit", "producer", producer, "state", st, "error", err)
		}

		if err := deps.Push(); err != nil {
			logger.Warn("battencli: status push failed, next transition will catch up", "producer", producer, "state", st, "error", err)
			return nil
		}
		return nil
	}
}

// battenCommitStatusSeam builds the real CommitStatus closure wire() installs onto ShedPaths: real
// fabric deps from battenCommitStatusDeps, and the marker path/lock pair from
// shedrun.LastCommitMarker(location, runID) with its own ".lock" sibling.
func battenCommitStatusSeam(location *lyxcwd.Location, runID string) func(producer, st string) error {
	markerPath := shedrun.LastCommitMarker(location, runID)
	return newCommitStatusSeam(battenCommitStatusDeps(location, runID), markerPath, markerPath+".lock")
}
