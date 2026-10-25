// starts.go starts the beam's ready tasks from the board: the start condition, the cap on runs in flight, and the notice and backoff of a refused first pass.

package dobbyengine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// startBackoffCeiling is the longest wait between two attempts to start a refused task.
const startBackoffCeiling = time.Hour

// TaskView is what dobby reads of one board entry.
type TaskView struct {
	// Slug and Kind identify the entry; only a task starts.
	Slug string
	Kind string
	// Status is the entry's status, nil or empty when the task is released and not yet started, and draft until then.
	Status *string
	// DependenciesDone reports whether every entry the task depends on is done.
	DependenciesDone bool
	// Parent is the name of the task's parent worktree, the prime's name for the prime.
	Parent string
	// OwnedElsewhere reports whether the entry's effective owner is another machine.
	OwnedElsewhere bool
	// Fingerprint changes whenever the entry does.
	Fingerprint string
}

// readyToStart reports whether dobby of the worktree named here starts task:
// the entry is a task with an empty status and every dependency done, its parent is here, no other machine owns it, and no pause entry covers the hub or the slug.
func readyToStart(task TaskView, here string, covered bool) bool {
	return task.Kind == boardengine.KindTask &&
		boardengine.IsEmptyStatus(task.Status) &&
		task.DependenciesDone &&
		task.Parent == here &&
		!task.OwnedElsewhere &&
		!covered
}

// startReady starts each ready task of the local board while fewer than max_runs runs it drives are in flight.
// Starting is the run's first pass, a full pass, which claims the task as batten does; a slug with a pass in flight is never started twice.
// The runs counted are the ones listed for this tick, halted ones included and ones recorded owned elsewhere not, plus first passes that have not yet left a status file.
// A task whose backoff has not run out waits, unless its entry changed since the refusal.
func (d *daemon) startReady(ctx context.Context, now time.Time) error {
	cfg, err := d.deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("load beam.yaml: %w", err)
	}
	if cfg.MaxRuns == 0 {
		return nil
	}
	tasks, err := d.deps.Tasks()
	if err != nil {
		return fmt.Errorf("read the board: %w", err)
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	running := d.runsInFlight(current)

	for _, task := range tasks {
		if running >= cfg.MaxRuns {
			break
		}
		if _, flying := d.flights[task.Slug]; flying {
			continue
		}
		covered, err := d.deps.PauseCovers(task.Slug)
		if err != nil {
			return fmt.Errorf("read the pause record: %w", err)
		}
		if !readyToStart(task, d.geom.WorktreeName, covered) {
			continue
		}
		attempt := current.Starts[task.Slug]
		if attempt.EntryFingerprint != "" && attempt.EntryFingerprint != task.Fingerprint {
			attempt = StartState{}
			if err := d.updateStart(task.Slug, func(s *StartState) { *s = StartState{} }); err != nil {
				return err
			}
		}
		if now.Before(attempt.NextTry) {
			continue
		}
		running++
		d.startFirstPass(task, now)
	}
	if len(d.flights) > 0 {
		d.busy = true
	}
	return nil
}

// runsInFlight counts the runs this dobby drives that take a slot: the listed ones not recorded owned elsewhere, and first passes that left no run yet.
func (d *daemon) runsInFlight(current State) int {
	listed := make(map[string]bool, len(d.runs))
	count := 0
	for _, run := range d.runs {
		listed[run.Slug] = true
		if !current.Runs[run.Slug].OwnedElsewhere {
			count++
		}
	}
	for slug, f := range d.flights {
		if f.first && !listed[slug] {
			count++
		}
	}
	return count
}

// startFirstPass spawns the first pass of task, a full pass, and keeps it as a flight.
// A spawn that fails is a refused start with the spawn error as its text.
func (d *daemon) startFirstPass(task TaskView, now time.Time) {
	exit, err := d.deps.SpawnPass(task.Slug, false, false)
	if err != nil {
		d.refusedStart(task.Slug, task.Fingerprint, err.Error(), now)
		return
	}
	d.flights[task.Slug] = flight{exit: exit, full: true, first: true, fingerprint: task.Fingerprint}
}

// firstPassExited reads how a first pass exited.
// A pass that left a status file claimed the task, so its backoff and noticed texts are cleared and the exit is read as any pass's.
// A covered pass is no refusal, and any other exit that left no status file is a refused start.
func (d *daemon) firstPassExited(slug string, f flight, exit PassExit, now time.Time) {
	claimed, err := d.deps.StatusExists(slug)
	if err != nil {
		logger.Warn("dobbyengine: whether the first pass left a status file is unreadable; the exit is read as a pass's", "slug", slug, "error", err)
		claimed = true
	}
	switch {
	case claimed:
		if err := d.updateStart(slug, func(s *StartState) { *s = StartState{} }); err != nil {
			logger.Warn("dobbyengine: the start record could not be cleared", "slug", slug, "error", err)
		}
		d.passExited(slug, f.full, exit.Code, exit.Error, now)
	case exit.Code == PassExitCovered:
	case exit.Code == 0:
		if err := d.updateStart(slug, func(s *StartState) { *s = StartState{} }); err != nil {
			logger.Warn("dobbyengine: the start record could not be cleared", "slug", slug, "error", err)
		}
	default:
		text := exit.Error
		if text == "" {
			text = fmt.Sprintf("the pass exited %d", exit.Code)
		}
		d.refusedStart(slug, f.fingerprint, text, now)
	}
}

// refusedStart records a refused start of slug: the retry backoff, and one notice per refusal text to the nearest live orch.
func (d *daemon) refusedStart(slug, fingerprint, text string, now time.Time) {
	noticeDue := false
	err := d.updateStart(slug, func(s *StartState) {
		if s.EntryFingerprint != fingerprint {
			*s = StartState{}
		}
		*s = nextStartTry(*s, now, d.probe)
		s.EntryFingerprint = fingerprint
		noticeDue = !slices.Contains(s.Noticed, text)
	})
	if err != nil {
		logger.Warn("dobbyengine: the refused start could not be recorded", "slug", slug, "error", err)
		return
	}
	if !noticeDue {
		return
	}
	if err := d.deps.Notice(d.tickCtx, fmt.Sprintf("%s: dobby could not start the task: %s", slug, text)); err != nil {
		logger.Warn("dobbyengine: the refused-start notice did not go out", "slug", slug, "error", err)
		return
	}
	err = d.updateStart(slug, func(s *StartState) { s.Noticed = append(s.Noticed, text) })
	if err != nil {
		logger.Warn("dobbyengine: the noticed refusal text could not be recorded", "slug", slug, "error", err)
	}
}

// nextStartTry returns st with the next attempt scheduled from now: one probe interval after the first refusal, then doubling up to an hour.
func nextStartTry(st StartState, now time.Time, probe time.Duration) StartState {
	interval := probe
	if st.Interval > 0 {
		interval = min(st.Interval*2, startBackoffCeiling)
	}
	st.Interval = interval
	st.NextTry = now.Add(interval)
	return st
}

// updateStart applies mutate to slug's start record, and removes the record once it is empty.
func (d *daemon) updateStart(slug string, mutate func(*StartState)) error {
	return UpdateState(d.geom.StatePath, func(s *State) error {
		st := s.Starts[slug]
		mutate(&st)
		switch {
		case st.NextTry.IsZero() && st.Interval == 0 && len(st.Noticed) == 0 && st.EntryFingerprint == "":
			delete(s.Starts, slug)
		default:
			if s.Starts == nil {
				s.Starts = map[string]StartState{}
			}
			s.Starts[slug] = st
		}
		return nil
	})
}
