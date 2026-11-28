// pendingteardown.go holds the pending-teardown record, which names a remote teardown that did not finish.
// The command writes the record before it hands a remote teardown job to its detached child.
// The child removes it once the job has run with no failed step, and keeps it with the way forward of a step that failed.
// PendingTeardowns lists the records no running teardown holds.
// SettleGonePairTeardown drops the record of a pair of which nothing remains.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// pendingTeardownSuffix ends the file name of a pending-teardown record, which sits beside the slug's teardown lock.
const pendingTeardownSuffix = ".pending.json"

// pendingTeardownPath returns the pending-teardown record of slug in the hub at hubPath.
func pendingTeardownPath(hubPath, slug string) string {
	return filepath.Join(filepath.Dir(fabriclayout.TeardownLockPath(hubPath, slug)), slug+pendingTeardownSuffix)
}

// pendingTeardownOf returns the record of job, handed over at handedAt, with the tips job tagged locally and its re-run.
func pendingTeardownOf(job remoteTeardownJob, handedAt time.Time) fabriclayout.PendingTeardown {
	return fabriclayout.PendingTeardown{
		Slug: job.Slug, Kind: job.Kind, HandedAt: handedAt.UTC(), Worktree: job.Worktree,
		WeftTip: job.WeftTip, TaskTip: job.TaskTip, Rerun: job.Rerun,
	}
}

// writePendingTeardown records job, handed to a detached child at handedAt, as pending.
func writePendingTeardown(hubPath string, job remoteTeardownJob, handedAt time.Time) error {
	path := pendingTeardownPath(hubPath, job.Slug)
	return state.WriteJSON(path, path+".lock", pendingTeardownOf(job, handedAt))
}

// keepFailedTeardown keeps the pending-teardown record of job and writes the ways forward of its failed steps into it.
// The listing then names what each failed step needs, in step order and each way forward once.
// A job with no record gets one.
// A failed write is logged; a record left on disk then names the way forward its own fields give.
func keepFailedTeardown(hubPath string, job remoteTeardownJob, failures []remoteStepFailure) {
	var steps []string
	for _, failure := range failures {
		if !slices.Contains(steps, failure.wayForward) {
			steps = append(steps, failure.wayForward)
		}
	}
	path := pendingTeardownPath(hubPath, job.Slug)
	err := state.UpdateJSON(path, path+".lock", func(record fabriclayout.PendingTeardown, found bool) (fabriclayout.PendingTeardown, error) {
		if !found {
			record = pendingTeardownOf(job, time.Now())
		}
		record.WayForward = strings.Join(steps, "; and ")
		return record, nil
	})
	if err != nil {
		logger.Warn("fabricengine: a failed remote step was not written to the pending teardown record", "slug", job.Slug, "error", err)
	}
}

// clearPendingTeardown removes the pending-teardown record of slug, and does nothing when none exists.
// A failed removal is logged: the record then names a re-run that finds nothing left to do.
func clearPendingTeardown(hubPath, slug string) {
	if err := os.Remove(pendingTeardownPath(hubPath, slug)); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Warn("fabricengine: pending teardown record not removed", "slug", slug, "error", err)
	}
}

// SettleGonePairTeardown drops the pending-teardown record of slug once its pair is gone, locally and on origin.
// It reports whether it dropped one.
// The result of a dropped record names the settling as its one step.
// Its stray path is a path at the pair's task worktree location that is no worktree.
// The result records the mutations of the call, a failed one included.
// It first pushes the slug's archive tags that origin lacks.
// The unfinished teardown may hold such a tag on this machine alone.
// A failed push returns an error wrapping ErrArchiveFailed and keeps the record.
// A slug with no record, or with any part of its pair left, is left as it is.
// It holds the slug's teardown lock exclusive, and the refusal of a held lock names rerun as the command to run again.
func (t *Topology) SettleGonePairTeardown(l *lymcwd.Location, slug, rerun string) (res fabriclayout.RemoveResult, settled bool, err error) {
	if !pathPresent(pendingTeardownPath(l.HubPath, slug)) {
		return res, false, nil
	}
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()
	release, err := t.lockPairTeardown(rec, l, slug, false, "re-run `"+rerun+"`")
	if err != nil {
		return res, false, err
	}
	defer release()

	pair, err := t.inspectPair(l, slug)
	if err != nil {
		return res, false, err
	}
	// A nil error means a part of the pair is left.
	if _, err := t.refuseWhenNothingRemains(l, slug, pair); !errors.Is(err, ErrPairNotFound) {
		return res, false, err
	}
	// The job reads both stores, since the record does not say which tags its teardown pushed.
	job := remoteTeardownJob{Kind: remoteTeardownRemove, Slug: slug, DeleteTaskOnOrigin: true}
	if err := pushOwedArchiveTags(rec, l, job, t.archiveSleep); err != nil {
		return res, false, err
	}
	clearPendingTeardown(l.HubPath, slug)
	res.Slug = slug
	res.Steps = []string{fabriclayout.RemoveStepPendingTeardown}
	if pair.strayPath {
		res.StrayPath = WorktreePath(l, slug)
	}
	return res, true, nil
}

// PendingTeardowns lists the hub's remote teardowns that did not finish, in slug order:
// those whose detached child ended before its job did, and those whose remote step failed.
// A record whose slug's teardown lock a process holds exclusive is a teardown still running and is not listed.
// The lock is probed shared, so the probe holds back no push of the pair.
// Each listed record carries what finishes its teardown:
// the way forward of its failed step, or else the re-run, behind the hand push of the archive tag of a tip it carries.
// A hub with no record lists none, as an empty slice.
func PendingTeardowns(hubPath string) ([]fabriclayout.PendingTeardown, error) {
	records, err := pendingTeardownRecords(hubPath)
	if err != nil {
		return nil, err
	}
	pending := []fabriclayout.PendingTeardown{}
	for _, record := range records {
		held, acquired, err := lock.TryAcquireReadLock(fabriclayout.TeardownLockPath(hubPath, record.Slug))
		if err != nil {
			return nil, fmt.Errorf("probe the teardown lock of %q: %w", record.Slug, err)
		}
		if !acquired {
			continue
		}
		_ = held.Release()
		pending = append(pending, record)
	}
	return pending, nil
}

// pendingTeardownRecords reads every pending-teardown record of the hub at hubPath, in slug order.
// The record of a teardown still running is included.
// Each carries what finishes its teardown.
func pendingTeardownRecords(hubPath string) ([]fabriclayout.PendingTeardown, error) {
	dir := filepath.Join(fabriclayout.HubScratchDir(hubPath), fabriclayout.TeardownLockDirName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the pending teardowns: %w", err)
	}
	var records []fabriclayout.PendingTeardown
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), pendingTeardownSuffix) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, found, err := state.ReadJSON[fabriclayout.PendingTeardown](path, path+".lock")
		if err != nil {
			return nil, fmt.Errorf("read the pending teardown %s: %w", entry.Name(), err)
		}
		if !found {
			continue
		}
		if record.WayForward == "" {
			record.WayForward = pendingTeardownWayForward(record)
		}
		records = append(records, record)
	}
	return records, nil
}

// pendingTeardownWayForward returns what finishes the remote teardown record names.
// It serves a record that holds no way forward of a failed step.
// A tip the record carries may be tagged on this machine alone,
// so the hand push of its archive tag comes before the re-run.
// A records tip whose records repository cannot be found from the record's worktree names the re-run alone.
func pendingTeardownWayForward(record fabriclayout.PendingTeardown) string {
	job := remoteTeardownJob{
		Kind: record.Kind, Slug: record.Slug, Worktree: record.Worktree,
		WeftTip: record.WeftTip, TaskTip: record.TaskTip, Rerun: record.Rerun,
	}
	switch record.Kind {
	case remoteTeardownRemove, remoteTeardownCleanup:
		weftRoot := ""
		if record.WeftTip != "" {
			weftRoot = recordsRootOf(record.Worktree)
		}
		return recordsArchiveWayForward(job, weftRoot)
	case remoteTeardownDiscard:
		return discardWayForward(job)
	default:
		return unknownKindWayForward(record.Slug)
	}
}

// recordsRootOf returns the root of the records repository paired with the code worktree, empty when it cannot be found.
func recordsRootOf(worktree string) string {
	l, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		return ""
	}
	root, err := RecordsRepoRoot(l)
	if err != nil {
		return ""
	}
	return root
}
