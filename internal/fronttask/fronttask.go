// fronttask.go implements filing, removing and settling a front task.
// It also holds the open and lookup rules over board entries, the action table and the record file.

package fronttask

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// ActionAdoptCheck is the action key of the adopt check of the subject whose worktree holds the front task.
const ActionAdoptCheck = "adopt-check"

// action is one row of the closed action table.
// It holds the lym arguments that run the action and the step an operator takes by hand when it fails.
type action struct {
	args     func(subject string) []string
	handStep func(subject string) string
}

// actions is the closed action table; no record or board entry adds a row.
var actions = map[string]action{
	ActionAdoptCheck: {
		args:     func(subject string) []string { return []string{"adopt", "check", subject} },
		handStep: func(subject string) string { return "lym adopt " + subject },
	},
}

// actionKeys returns the keys of the action table, sorted.
func actionKeys() []string {
	keys := make([]string, 0, len(actions))
	for key := range actions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Request describes a front task to file: its board fields, the project that holds it,
// and the action it runs once it lands.
type Request struct {
	Slug    string
	Title   string
	Body    string
	Labels  []string
	Recipe  string
	Profile string
	// Project is the beam the task belongs to, empty for the prime.
	Project string
	// OnLand is the key of the action table's row that runs once the task lands.
	OnLand string
}

// Deps holds every seam over the hub that filing and removing a front task read.
type Deps struct {
	// Tasks returns the local board's entries.
	Tasks func() ([]boardengine.Task, error)
	// Create creates or updates the entry fields describe as a front task with action onLand.
	Create func(fields map[string]any, onLand string) (boardengine.Task, error)
	// Release clears the entry's draft status.
	Release func(slug string) error
	// RemoveEntry removes the entry from the board.
	RemoveEntry func(slug string) error
	// HasRun reports whether the slug has a run claim, a batten run record or a pair.
	HasRun func(slug string) (bool, error)
	// Cancel ends the slug's run for good and removes what it left.
	Cancel func(ctx context.Context, slug string) error
	// RecordPath is the record file; its lock sits beside it.
	RecordPath string
}

// SettleDeps holds every seam that settling the front tasks of one worktree reads.
type SettleDeps struct {
	// Project is the worktree's project: the beam's slug, empty for the prime.
	Project string
	// Subject is what the worktree's adopt takes: the beam's slug, `root` for the prime.
	Subject string
	// Tasks returns the local board's entries.
	Tasks func() ([]boardengine.Task, error)
	// OwnedHere reports whether this machine is the effective owner of the subject.
	OwnedHere func() (bool, error)
	// RecordPath is the record file; its lock sits beside it.
	RecordPath string
	// RunAction starts the action of the table's key for subject.
	RunAction func(action, subject string) error
}

// RecordEntry is what the record holds of one front task: the action to run once it lands and the project that holds it.
type RecordEntry struct {
	OnLand  string `json:"on_land"`
	Project string `json:"project"`
}

// records is the record file: one entry per front task, by slug.
type records struct {
	Entries map[string]RecordEntry `json:"entries"`
}

// lockPathOf returns the lock file beside the record file at recordPath.
func lockPathOf(recordPath string) string {
	return strings.TrimSuffix(recordPath, filepath.Ext(recordPath)) + ".lock"
}

// RemoveError is a removal that stopped at one step, with the earlier steps left done.
type RemoveError struct {
	Slug string
	// Step is `cancel`, `record` or `entry`.
	Step string
	Err  error
}

// Error reports the step, its error and the hand removal.
func (e *RemoveError) Error() string {
	return fmt.Sprintf("remove front task %s: the %s step failed: %v; way forward: %s", e.Slug, e.Step, e.Err, e.HandRemoval())
}

// Unwrap returns the step's error.
func (e *RemoveError) Unwrap() error { return e.Err }

// HandRemoval names the commands that finish the removal by hand.
// After a failed cancel these are the cancel and the board removal; after a later step, the board removal alone.
func (e *RemoveError) HandRemoval() string {
	if e.Step == "cancel" {
		return fmt.Sprintf("run `lym batten cancel %s --remove`, then `%s`", e.Slug, boardengine.RemoveCommand(e.Slug))
	}
	return fmt.Sprintf("run `%s`", boardengine.RemoveCommand(e.Slug))
}

// IsOpen reports whether task is an open front task: a task entry with Front set whose status is empty or a run status.
// A done, draft or otherwise-statused front task is closed.
func IsOpen(task boardengine.Task) bool {
	return task.Kind == boardengine.KindTask && task.Front &&
		(boardengine.IsEmptyStatus(task.Status) || boardengine.IsRunStatus(task.Status))
}

// isDone reports whether task carries the done status.
func isDone(task boardengine.Task) bool {
	return task.Status != nil && *task.Status == "done"
}

// OpenFront returns the open front entry of project, "" for the prime.
// It finds the entry by its Front field and project, never by slug.
func OpenFront(tasks []boardengine.Task, project string) (boardengine.Task, bool) {
	for _, task := range tasks {
		if IsOpen(task) && task.Project == project {
			return task, true
		}
	}
	return boardengine.Task{}, false
}

// FrontOf returns the front entry of project whose action is onLand and that is not done, whatever its status.
// A draft or halted entry is found too.
func FrontOf(tasks []boardengine.Task, project, onLand string) (boardengine.Task, bool) {
	for _, task := range tasks {
		if task.Front && task.Project == project && task.OnLand == onLand && !isDone(task) {
			return task, true
		}
	}
	return boardengine.Task{}, false
}

// File files req as a front task.
// It creates the entry, runs between when it is not nil, then releases the entry.
// It records the action and project under the slug.
// It refuses an action outside the table and a project that already holds an open front task, both before any write.
// A between error or a release error leaves the entry as a draft, which is closed and holds no start.
// A failed record write after the release is logged and the call still returns nil;
// the released entry starts anyway, and the next settle of the worktree records the missing record.
func File(deps Deps, req Request, between func() error) error {
	if _, known := actions[req.OnLand]; !known {
		return fmt.Errorf("front task %s: the action %q is not in the action table; way forward: use one of %s", req.Slug, req.OnLand, strings.Join(actionKeys(), ", "))
	}
	tasks, err := deps.Tasks()
	if err != nil {
		return fmt.Errorf("front task %s: read the board: %w", req.Slug, err)
	}
	if open, found := OpenFront(tasks, req.Project); found {
		return fmt.Errorf("front task %s: the front task %s stands open in this worktree, and one front task stands open per worktree; way forward: let %s land, or remove it with `lym batten cancel %s --remove` then `%s`, which runs its action", req.Slug, open.Slug, open.Slug, open.Slug, boardengine.RemoveCommand(open.Slug))
	}

	fields := map[string]any{"slug": req.Slug, "title": req.Title, "kind": boardengine.KindTask, "body": req.Body, "labels": req.Labels}
	if req.Recipe != "" {
		fields["recipe"] = req.Recipe
	}
	if req.Profile != "" {
		fields["profile"] = req.Profile
	}
	if req.Project != "" {
		fields["project"] = req.Project
	}
	if _, err := deps.Create(fields, req.OnLand); err != nil {
		return fmt.Errorf("front task %s: create the entry: %w", req.Slug, err)
	}
	if between != nil {
		if err := between(); err != nil {
			return fmt.Errorf("front task %s: %w", req.Slug, err)
		}
	}
	if err := deps.Release(req.Slug); err != nil {
		return fmt.Errorf("front task %s: release the entry: %w", req.Slug, err)
	}
	err = state.UpdateJSON(deps.RecordPath, lockPathOf(deps.RecordPath), func(cur records, _ bool) (records, error) {
		if cur.Entries == nil {
			cur.Entries = map[string]RecordEntry{}
		}
		cur.Entries[req.Slug] = RecordEntry{OnLand: req.OnLand, Project: req.Project}
		return cur, nil
	})
	if err != nil {
		// The entry is released and starts, so the filing stands;
		// the next settle records an open front entry that has no record.
		logger.Warn("fronttask: the record of a filed front task was not written; Settle records it from the entry", "slug", req.Slug, "error", err)
	}
	return nil
}

// Remove removes the front task slug in three steps.
// It cancels the run when there is one, drops the record and removes the entry.
// A cancel that finds nothing left counts as done;
// any other cancel error stops the removal before the record is touched, so no action runs for it.
// The record's drop and the entry's removal run under the record's lock.
// A concurrent settle therefore reads either the record still there or the entry gone with no record.
// A failing step returns a *RemoveError, the earlier steps left done.
func Remove(deps Deps, slug string) error {
	hasRun, err := deps.HasRun(slug)
	if err != nil {
		return &RemoveError{Slug: slug, Step: "cancel", Err: err}
	}
	if hasRun {
		if err := deps.Cancel(context.Background(), slug); err != nil && !errors.Is(err, battencli.ErrNothingLeft) {
			return &RemoveError{Slug: slug, Step: "cancel", Err: err}
		}
	}

	// The entry's removal error is carried out of the locked update;
	// returning it would skip the record write that drops the record.
	var entryErr error
	err = state.UpdateJSON(deps.RecordPath, lockPathOf(deps.RecordPath), func(cur records, _ bool) (records, error) {
		delete(cur.Entries, slug)
		entryErr = deps.RemoveEntry(slug)
		return cur, nil
	})
	if err != nil {
		return &RemoveError{Slug: slug, Step: "record", Err: err}
	}
	if entryErr != nil {
		return &RemoveError{Slug: slug, Step: "entry", Err: entryErr}
	}
	return nil
}

// Settle runs the land actions of the worktree's front tasks and keeps its record true, under the record's lock.
// When this machine does not own the subject it drops the project's records and runs nothing.
// Otherwise it records each open front entry that has no record;
// then it runs the action of each record whose entry is gone or done, and drops that record.
// A record whose entry stands in any other status is left;
// a record naming an action outside the table is dropped with a warning, and a failed action keeps its record.
// The error of a failed action names the slug, the action, the failure and the hand step from the table.
func Settle(deps SettleDeps) error {
	var failures []error
	err := state.UpdateJSON(deps.RecordPath, lockPathOf(deps.RecordPath), func(cur records, _ bool) (records, error) {
		if cur.Entries == nil {
			cur.Entries = map[string]RecordEntry{}
		}
		owned, err := deps.OwnedHere()
		if err != nil {
			return cur, fmt.Errorf("read the owner of %s: %w", deps.Subject, err)
		}
		if !owned {
			for slug, entry := range cur.Entries {
				if entry.Project == deps.Project {
					delete(cur.Entries, slug)
				}
			}
			return cur, nil
		}
		tasks, err := deps.Tasks()
		if err != nil {
			return cur, fmt.Errorf("read the board: %w", err)
		}
		for _, task := range tasks {
			if _, recorded := cur.Entries[task.Slug]; !recorded && IsOpen(task) && task.Project == deps.Project {
				cur.Entries[task.Slug] = RecordEntry{OnLand: task.OnLand, Project: task.Project}
			}
		}

		slugs := make([]string, 0, len(cur.Entries))
		for slug, entry := range cur.Entries {
			if entry.Project == deps.Project {
				slugs = append(slugs, slug)
			}
		}
		slices.Sort(slugs)
		for _, slug := range slugs {
			entry := cur.Entries[slug]
			index := slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == slug })
			if index >= 0 && !isDone(tasks[index]) {
				continue
			}
			row, known := actions[entry.OnLand]
			if !known {
				logger.Warn("fronttask: a record names an action outside the table; it is dropped", "slug", slug, "action", entry.OnLand)
				delete(cur.Entries, slug)
				continue
			}
			if err := deps.RunAction(entry.OnLand, deps.Subject); err != nil {
				failures = append(failures, fmt.Errorf("front task %s: the action %s failed: %w; way forward: run `%s`", slug, entry.OnLand, err, row.handStep(deps.Subject)))
				continue
			}
			delete(cur.Entries, slug)
		}
		return cur, nil
	})
	if err != nil {
		return fmt.Errorf("settle the front tasks of %s: %w", deps.Subject, err)
	}
	return errors.Join(failures...)
}
