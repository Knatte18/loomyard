// commitstatus.go implements batten's CommitStatus seam, a shedengine.Shed hook.
// After each transition it commits prime's own batten status.json onto prime's own fabric pair.
//
// It stages only the run's own paths.
// The Fabric Git Invariant forbids a fabric-commit caller from staging every change in its repo.
// The push is spawned, not run: the seam starts fabric's detached push and returns.
// The child logs a push rejected because a Board write advanced the branch at warn,
// and the next transition's spawn catches the branch up.
//
// A transition at a row listed before Run-Shed first writes "<state> · <row>" on the task's board entry.
// The board follows batten from its claim at Worktree-Create until the inner run writes the entry's status itself.
// A failed board write only warns, since the board is the hub's overview, never the run's own bookkeeping.
//
// The seam adds an on-disk no-op-transition skip ahead of the statuscommit core's three dispositions.
// Each `lym batten step` is a fresh process, so the skip's memory cannot live in a closure variable.
// It would start empty every call, and the skip would work for `run` mode alone.
// It lives instead at `shedrun.LastCommitMarker(location, runID)`.
// The seam reads it before deciding and rewrites it after a successful commit.
//
// The skip's cost is that shedengine rewrites the run's durable status file on every Run-Shed self-bounce.
// Each such write appends to the history, and the skip declines to commit each one after the first.
// Run-Shed waits on its child inside its call.
// It bounces only on a state change, a pause, or a failed status stat.
// The pair carries an uncommitted change at that path for as long as a watch goes between bounces.
// That is the right trade against one identical commit per bounce.
// Operations that need a clean pair, such as `lym fabric checkout`, refuse hub-wide while a batten run is watching.
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

// commitStatusDeps carries the three fabric calls the commit seam drives.
// The calls are plain function values rather than direct calls,
// so a test can drive the seam's branching from stub closures, with no hub fixture and no git spawn.
type commitStatusDeps struct {
	// MergeActive reports whether the status file's fabric sibling worktree is mid-merge at the git level.
	MergeActive func() (bool, error)
	// Commit commits batten's own status file with msg and reports whether it made a commit.
	Commit func(msg string) (committed bool, err error)
	// Spawn starts the detached push of the fabric sibling worktree's unpushed commits.
	Spawn func() error
	// SetBoardStatus writes status onto the run's board entry, leaving an absent or already done entry untouched.
	// A nil SetBoardStatus writes nothing.
	SetBoardStatus func(status string) error
}

// spawnStatusPush starts the detached push of the status records at a location, held under a gate slug.
// A test replaces it with a recorder.
var spawnStatusPush = fabricengine.SpawnStatusPush

// battenRunCommitPaths returns the anchor-relative paths a commit of the run's records stages.
// It always stages the run's status file.
// It stages the seed, dobby's run notice marker, settle records and child-recipe record whenever each is on disk.
// It stages the driver's drive-reports, tackler and arbiter directories whenever each holds a file.
//
// The status and the seed belong on the pair, since a run directory is durable, fabric-synced state.
// Without its seed, a committed status lets a resumed machine read how far the run came but not what it is running.
// Batten's own auto-seed would then re-seed the run from flag defaults.
// The seed goes in on every transition rather than once at seeding time.
// Committing an already-clean tracked path is a no-op,
// so a seed written by an invocation that crashed before any commit lands on the next one.
//
// Each optional path is included only when present, since a pathspec matching no file is a hard git error.
// Every caller outside the CLI verbs drives a Shed without batten's own auto-seed, so it has no seed to commit.
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
// The driver writes a hand-back's stop report and saves a done step's envelope after the transition's own commit,
// so the two stops that follow those writes commit them here.
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

// battenCommitStatusDeps builds the commitStatusDeps that drive the fabric pair for location and runID.
// SetBoardStatus writes the hub board's entry runID names, a batten run's id being its task's slug.
// The spawned push runs from the prime, so it covers only the prime's records side,
// and it waits under the pair's pause entry with runID as its gate slug.
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

// commitStatusMarker is the on-disk record of the last (producer, state) pair the commit seam committed.
// It is the no-op-transition skip's memory, read before deciding and rewritten after a successful commit.
// It lives on disk rather than in a closure variable,
// so the skip survives a fresh `lym batten step` process rebuilding the seam between calls.
type commitStatusMarker struct {
	Producer string `json:"producer"`
	State    string `json:"state"`
}

// newCommitStatusSeam builds the status-commit hook of a shed run from deps.
// The hook wraps an on-disk no-op-transition skip, kept at markerPath and locked via markerLockPath.
//
// Evaluation order: the board status write of a row before Run-Shed first, then the no-op-transition skip.
// The skip returns nil without committing or pushing when the incoming (producer, state) pair matches the marker.
// Then the statuscommit core applies its three dispositions:
// skip while a merge is in progress, return a failed commit as an error, and warn on a failed spawn.
//
// A missing or corrupt marker falls back to committing once rather than erroring:
// the marker is a cache, and losing it costs one redundant commit, never correctness.
// The core's after-commit callback rewrites the marker right after a successful Commit and before the Spawn attempt.
// So a later call skips re-committing the same pair.
// That holds even if the spawn that followed is only warned about, not retried.
func newCommitStatusSeam(deps commitStatusDeps, markerPath, markerLockPath string) func(producer, st string) error {
	// A failed marker write costs one redundant commit on the next call, the same cost a missing marker costs on read,
	// so it is logged at warn rather than returned.
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

// battenCommitStatusSeam builds the status-commit hook for a run, backed by the real fabric pair at location.
// Its marker is the run's last-commit marker, locked through a sibling file with a ".lock" suffix.
func battenCommitStatusSeam(location *lymcwd.Location, runID string) func(producer, st string) error {
	markerPath := shedrun.LastCommitMarker(location, runID)
	return newCommitStatusSeam(battenCommitStatusDeps(location, runID), markerPath, markerPath+".lock")
}
