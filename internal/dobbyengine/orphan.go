// orphan.go notices the orphan pairs, those that neither a board entry nor a batten record names.
// It keeps the first-sight record that decides when an idle one is torn down.

package dobbyengine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// OrphanGrace is how long an orphan pair stands after the notice of its first sight before the dobby tears it down.
// Both its branches must also have gone without a commit for as long.
const OrphanGrace = 24 * time.Hour

// OrphanView is what the dobby reads of one orphan pair.
type OrphanView struct {
	Slug string
	// LastCommit is the newer commit time of the tips of the pair's code and records branches.
	LastCommit time.Time
	// ProcessLive reports whether a process registered under the pair's slug lives.
	ProcessLive bool
}

// OrphanSights is the machine-local record of when the dobby first noticed each orphan pair, by slug.
// A dobby restart reads it, so the grace is neither reset nor skipped.
type OrphanSights struct {
	Seen map[string]time.Time `json:"seen,omitempty"`
}

// updateOrphanSights applies mutate to the first-sight record at path under its lock.
// It keeps the keys it does not declare.
func updateOrphanSights(path string, mutate func(*OrphanSights)) error {
	return state.UpdateJSON(path, lockPath(path), func(current OrphanSights, _ bool) (OrphanSights, error) {
		if current.Seen == nil {
			current.Seen = map[string]time.Time{}
		}
		mutate(&current)
		return current, nil
	})
}

// dueOrphans returns the orphan pairs whose teardown is due.
// It reads the orphans afresh on a probe tick, and between probe ticks it returns what the last reading found.
// An orphan seen for the first time gets one notice saying when it goes.
// Its first-sight time is recorded once that notice posted.
// An orphan whose notice failed stays out of the record and is noticed again at the next probe tick,
// so it is never due before a notice.
// A pair no longer read as an orphan leaves the record.
// An orphan is due once the orphan grace has passed since its first sight and since its last commit.
// No process of its slug may be alive.
// A failed read, notice or record write is logged once per episode and leaves the last reading standing.
// A nil Deps.Orphans or an empty Deps.OrphanRecord notices nothing.
func (d *daemon) dueOrphans(ctx context.Context, now time.Time) []string {
	if d.deps.Orphans == nil || d.deps.OrphanRecord == "" {
		return nil
	}
	if d.orphansRead && !d.probeDue {
		return d.orphansDue
	}
	orphans, err := d.deps.Orphans()
	if err != nil {
		d.orphanFailures.Fail("dobbyengine: the orphan pairs are unreadable; the last reading stands", "worktree", d.geom.WorktreeName, "error", err)
		return d.orphansDue
	}
	recorded, _, err := state.ReadJSON[OrphanSights](d.deps.OrphanRecord, lockPath(d.deps.OrphanRecord))
	if err != nil {
		d.orphanFailures.Fail("dobbyengine: the orphan first-sight record could not be read; the last reading stands", "worktree", d.geom.WorktreeName, "error", err)
		return d.orphansDue
	}
	noticed := map[string]bool{}
	var noticeErr error
	for _, orphan := range orphans {
		if _, known := recorded.Seen[orphan.Slug]; known {
			continue
		}
		if err := d.postOrphanNotice(ctx, orphan.Slug, now); err != nil {
			noticeErr = err
			continue
		}
		noticed[orphan.Slug] = true
	}
	seen := map[string]time.Time{}
	err = updateOrphanSights(d.deps.OrphanRecord, func(s *OrphanSights) {
		listed := make(map[string]bool, len(orphans))
		for _, orphan := range orphans {
			listed[orphan.Slug] = true
			if _, known := s.Seen[orphan.Slug]; !known && noticed[orphan.Slug] {
				s.Seen[orphan.Slug] = now
			}
			if at, known := s.Seen[orphan.Slug]; known {
				seen[orphan.Slug] = at
			}
		}
		for slug := range s.Seen {
			if !listed[slug] {
				delete(s.Seen, slug)
			}
		}
	})
	if err != nil {
		d.orphanFailures.Fail("dobbyengine: the orphan first-sight record could not be written; the last reading stands", "worktree", d.geom.WorktreeName, "error", err)
		return d.orphansDue
	}
	if noticeErr != nil {
		d.orphanFailures.Fail("dobbyengine: an orphan notice was not posted; the next probe tick posts it again", "worktree", d.geom.WorktreeName, "error", noticeErr)
	} else {
		d.orphanFailures.Recover("dobbyengine: the orphan pairs read again", "worktree", d.geom.WorktreeName)
	}
	d.orphansRead = true
	d.orphansDue = nil
	for _, orphan := range orphans {
		first, noticedBefore := seen[orphan.Slug]
		if noticedBefore && !orphan.ProcessLive && now.Sub(first) >= OrphanGrace && now.Sub(orphan.LastCommit) >= OrphanGrace {
			d.orphansDue = append(d.orphansDue, orphan.Slug)
		}
	}
	return d.orphansDue
}

// postOrphanNotice posts the notice of the orphan pair slug first seen at seenAt, and returns the post's failure.
// A nil Deps.Notice posts nothing and fails nothing.
func (d *daemon) postOrphanNotice(ctx context.Context, slug string, seenAt time.Time) error {
	if d.deps.Notice == nil {
		return nil
	}
	return d.deps.Notice(ctx, fmt.Sprintf("%s: orphan pair: neither a board entry nor a batten record names it; the dobby tears it down at %s or later, "+
		"its branches kept under archive tags, once it has gone that long without a commit or a live process; "+
		"way forward: put it on the board to keep it, or run `lym fabric remove %s` to remove it now",
		slug, seenAt.Add(OrphanGrace).UTC().Format(time.RFC3339), slug))
}

// dropOrphanSight removes slug from the first-sight record once its pair is torn down.
// A failed write is logged at Warn; the next reading drops a slug no longer listed anyway.
func (d *daemon) dropOrphanSight(slug string) {
	if d.deps.OrphanRecord == "" {
		return
	}
	err := updateOrphanSights(d.deps.OrphanRecord, func(s *OrphanSights) { delete(s.Seen, slug) })
	if err != nil {
		logger.Warn("dobbyengine: a removed orphan could not leave the first-sight record", "slug", slug, "error", err)
	}
	d.orphansDue = slices.DeleteFunc(d.orphansDue, func(due string) bool { return due == slug })
}
