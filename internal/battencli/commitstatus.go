// commitstatus.go implements batten's own CommitStatus seam: the shedengine.Shed hook that commits
// prime's own batten status.json onto prime's own fabric pair after each transition.
//
// Modelled on internal/loomcli/wiring.go's newCommitStatusSeam/loomCommitStatusDeps pair, whose
// three-case disposition -- skip-while-mid-merge, commit-hard-errors, spawn-warns -- applies here
// verbatim rather than being reinvented. This is not fabricengine.Bolt: Bolt.Commit stages every
// change in its repo, which the Fabric Git Invariant forbids a fabric-commit caller, and Bolt is the
// Board's own carve-out scoped to the Board directory.
// The push is spawned, not run: the seam starts fabric's detached push and returns.
// The child logs a push rejected because a Board write advanced the branch at warn, and the next transition's spawn catches the branch up.
//
// Before anything else, a transition at a row listed before Run-Shed writes "<state> · <row>" on the task's board entry,
// so the board follows batten from its claim at Worktree-Create until the inner run starts and writes the entry's status itself.
// A failed board write only warns, since the board is the hub's overview, never the run's own bookkeeping.
//
// On top of loomcli's own dispositions, this seam adds one more, evaluated first: an on-disk
// no-op-transition skip. Each `lym batten step` is a fresh process, so the skip's memory cannot live
// in a closure variable -- it would start empty every call and the skip would work for `run` mode
// alone. It lives instead at shedrun.LastCommitMarker(location, runID), read before deciding and
// rewritten after a successful commit.
//
// The skip's own cost, stated here because nothing else in the code says it: shedengine rewrites the
// run's durable status file on every Run-Shed self-bounce (a history append), and the skip declines
// to commit every one of them after the first.
// Run-Shed waits on its child inside its call, so it bounces only on a state change, a pause or a failed status stat,
// and the pair carries an uncommitted change at that path for as long as a watch goes between them.
// That is the right trade against one identical commit per bounce,
// and it is why an operation requiring a clean pair, such as "lym fabric checkout", refuses hub-wide while a batten run is watching.
package battencli

import (
	"fmt"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/statuscommit"
)

// commitStatusDeps carries the three fabric calls newCommitStatusSeam drives, injected as plain
// function values rather than reached directly, so the seam's branching is drivable in a Tier 1
// test from stub closures, with no hub fixture and no git spawn -- exactly as loomcli's own
// commitStatusDeps.
type commitStatusDeps struct {
	// MergeActive reports whether the fabric sibling worktree that carries the status file is
	// mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits batten's own status file with msg and reports whether it made a commit.
	Commit func(msg string) (committed bool, err error)
	// Spawn starts the detached push of the fabric sibling worktree's unpushed commits.
	Spawn func() error
	// SetBoardStatus writes status onto the run's board entry, leaving an entry that is absent or already done untouched.
	// Nil writes nothing.
	SetBoardStatus func(status string) error
}

// spawnStatusPush starts the detached push of the status records at a location, held under a gate slug; a test swaps it for a recorder.
var spawnStatusPush = fabricengine.SpawnStatusPush

// battenRunCommitPaths returns the anchor-relative paths a commit of the run's records stages.
// It always stages the run's status file.
// It stages the seed, dobby's run notice marker, the settle records and the child-recipe record whenever each is on disk.
// It stages the driver's drive-reports, tackler and arbiter directories whenever each holds a file.
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
func battenRunCommitPaths(location *lymcwd.Location, runID string) []string {
	paths := []string{shedrun.StatusRel(location, runID)}
	if _, err := os.Stat(shedrun.SeedFile(location, runID)); err == nil {
		paths = append(paths, shedrun.SeedRel(location, runID))
	}
	if _, err := os.Stat(shedrun.NoticeEpisodeFile(location, runID)); err == nil {
		paths = append(paths, shedrun.NoticeEpisodeRel(location, runID))
	}
	if _, err := os.Stat(shedrun.SettleRecords(location, runID)); err == nil {
		paths = append(paths, shedrun.SettleRecordsRel(location, runID))
	}
	if _, err := os.Stat(shedrun.ChildRecipeFile(location, runID)); err == nil {
		paths = append(paths, shedrun.ChildRecipeRel(location, runID))
	}
	return append(paths, shedrun.DriverRecordsRel(location, runID)...)
}

// commitRunRecords commits the run's records outside a status transition and starts their push when it made a commit.
// The driver writes a hand-back's stop report, and saves a done step's envelope, after the transition's own commit, so these two stops commit them here.
// A merge in progress on the pair commits nothing, and the next commit of the run's records carries them.
// A push that fails to start is logged, never returned, since the next push carries the commit.
func commitRunRecords(deps commitStatusDeps, slug string) error {
	active, err := deps.MergeActive()
	if err != nil {
		return fmt.Errorf("battencli: read the pair's merge state: %w", err)
	}
	if active {
		logger.Info("battencli: a merge is in progress on the pair; the run records stay uncommitted", "slug", slug)
		return nil
	}
	committed, err := deps.Commit("batten: run records for " + slug)
	if err != nil || !committed {
		return err
	}
	if err := deps.Spawn(); err != nil {
		logger.Warn("battencli: the push of the run records did not start; the next push carries them", "slug", slug, "error", err)
	}
	return nil
}

// battenCommitStatusDeps builds a commitStatusDeps over location and runID, filling each field from fabric:
// MergeActive from fabricengine.MergeStateActive, Commit from fabricengine.CommitAnchoredPaths scoped to battenRunCommitPaths(location, runID), and Spawn from spawnStatusPush.
// SetBoardStatus writes the hub board's entry runID names, a batten run's id being its task's slug.
// The spawned push runs from the prime, so it is the prime's records side only, and it waits under the pair's pause entry with runID as its gate slug.
func battenCommitStatusDeps(location *lymcwd.Location, runID string) commitStatusDeps {
	return commitStatusDeps{
		MergeActive: func() (bool, error) {
			return fabricengine.MergeStateActive(location)
		},
		Commit: func(msg string) (bool, error) {
			_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, battenRunCommitPaths(location, runID), msg, fabricengine.EnvSyncOptions())
			return committed, err
		},
		Spawn: func() error {
			_, err := spawnStatusPush(location, runID)
			return err
		},
		SetBoardStatus: func(status string) error {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return err
			}
			return board.SetRunStatus(runID, status)
		},
	}
}

// commitStatusMarker is the on-disk shape newCommitStatusSeam persists at markerPath: the last
// (producer, state) pair the seam actually committed. It is the no-op-transition skip's memory,
// read before deciding and rewritten after a successful commit -- never held in a closure variable,
// so the skip survives a fresh `lym batten step` process rebuilding the seam from scratch between
// calls.
type commitStatusMarker struct {
	Producer string `json:"producer"`
	State    string `json:"state"`
}

// newCommitStatusSeam builds the shedengine.Shed.CommitStatus closure from deps, wrapped in an
// on-disk no-op-transition skip read from and written to markerPath (locked via markerLockPath).
//
// Evaluation order: the board status write of a row before Run-Shed first, then the no-op-transition skip -- when the incoming (producer, state) pair equals the marker's last-committed pair, return nil without committing or pushing --
// then the shared statuscommit core's three dispositions: skip-while-mid-merge, commit-hard-errors, spawn-warns.
//
// A missing or corrupt marker falls back to committing once rather than erroring:
// the marker is a cache, and losing it costs one redundant commit, never correctness.
// The marker is rewritten by the core's after-commit callback, right after a successful Commit and ahead of the Spawn attempt,
// so a subsequent call skips re-committing the same pair even if the spawn that followed is still only warned about, not retried.
func newCommitStatusSeam(deps commitStatusDeps, markerPath, markerLockPath string) func(producer, st string) error {
	// Rewritten unconditionally on a successful commit.
	// A failed marker write only costs one redundant commit on the next call -- the same cost a missing marker costs on read --
	// so it is warned about rather than escalated.
	core := statuscommit.New(statuscommit.Deps{
		MergeActive: deps.MergeActive,
		Commit:      deps.Commit,
		Spawn:       deps.Spawn,
	}, "batten", "battencli", func(producer, st string) {
		if err := state.WriteJSON(markerPath, markerLockPath, commitStatusMarker{Producer: producer, State: st}); err != nil {
			logger.Warn("battencli: status commit succeeded but the marker write failed; a future call may recommit", "producer", producer, "state", st, "error", err)
		}
	})
	preRunRows := battenrecipe.RowsBeforeRunShed()
	return func(producer, st string) error {
		if deps.SetBoardStatus != nil && slices.Contains(preRunRows, producer) {
			if err := deps.SetBoardStatus(boardengine.RunStatus(st, producer)); err != nil {
				logger.Warn("battencli: board status write failed, next transition retries", "producer", producer, "state", st, "error", err)
			}
		}
		marker, found, err := state.ReadJSON[commitStatusMarker](markerPath, markerLockPath)
		if err != nil {
			logger.Warn("battencli: status commit marker unreadable; treating it as absent", "producer", producer, "state", st, "error", err)
			found = false
		}
		if found && marker.Producer == producer && marker.State == st {
			return nil
		}

		return core(producer, st)
	}
}

// battenCommitStatusSeam builds the real CommitStatus closure wire() installs onto ShedPaths: real
// fabric deps from battenCommitStatusDeps, and the marker path/lock pair from
// shedrun.LastCommitMarker(location, runID) with its own ".lock" sibling.
func battenCommitStatusSeam(location *lymcwd.Location, runID string) func(producer, st string) error {
	markerPath := shedrun.LastCommitMarker(location, runID)
	return newCommitStatusSeam(battenCommitStatusDeps(location, runID), markerPath, markerPath+".lock")
}
