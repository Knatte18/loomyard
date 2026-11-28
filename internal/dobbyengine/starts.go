// starts.go starts the beam's ready tasks from the board.
// It applies the start condition, the cap on runs in flight, and the hold under the cgroup task limit.
// It also posts the notice for a refused first pass and applies its backoff.

package dobbyengine

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
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
	// PendingDependencies holds the slugs of the entries the task depends on that are not done.
	PendingDependencies []string
	// Parent names the parent worktree, the prime's name for the prime; read for released tasks and NotifyOnLand entries.
	Parent string
	// OwnedElsewhere reports whether another machine owns the entry; read for released tasks and NotifyOnLand entries.
	OwnedElsewhere bool
	// StartRefusal is the refusal every start of the slug gets now, such as a finished run record; empty when none.
	// Dobby spawns no pass for it and starts it on the first tick after the refusal is gone; read for released tasks.
	StartRefusal string
	// NotifyOnLand reports whether the entry carries notify_on_land, a request for a landing notice.
	NotifyOnLand bool
	// Front reports whether the entry is a front task, filed ahead of its worktree's other starts; read for every entry.
	Front bool
	// Project is the slug of the beam project the entry belongs to, empty for the prime's own; read for every entry.
	Project string
	// Fingerprint changes whenever the entry does.
	Fingerprint string
}

// openFront returns the open front task of the worktree whose beam slug is project, empty for the prime.
// An open front task is a task entry with Front set whose status is empty or a run status.
func openFront(tasks []TaskView, project string) (TaskView, bool) {
	for _, task := range tasks {
		if task.Kind == boardengine.KindTask && task.Front && task.Project == project &&
			(boardengine.IsEmptyStatus(task.Status) || boardengine.IsRunStatus(task.Status)) {
			return task, true
		}
	}
	return TaskView{}, false
}

// released reports whether task is released to the dobby of the worktree named here.
// A released task is a task entry with an empty status whose parent is here.
func released(task TaskView, here string) bool {
	return task.Kind == boardengine.KindTask && boardengine.IsEmptyStatus(task.Status) && task.Parent == here
}

// heldByEntry reports whether a released task is held.
// A task is held while a dependency is not done, another machine owns it, or a refusal awaits its slug's starts.
// A pause entry covering the hub or the slug holds it too.
func heldByEntry(task TaskView, covered bool) bool {
	return len(task.PendingDependencies) > 0 || task.OwnedElsewhere || task.StartRefusal != "" || covered
}

// readyToStart reports whether dobby of the worktree named here starts task.
// It is true when task is released here and nothing holds it.
func readyToStart(task TaskView, here string, covered bool) bool {
	return released(task, here) && !heldByEntry(task, covered)
}

// startReady starts each ready task of the local board while fewer than max_runs runs it drives are in flight.
// Starting runs the task's first pass, a full pass that claims it as batten does.
// A slug with a pass in flight is never started twice.
// The count covers the runs listed for this tick, halted and done ones included.
// It leaves out runs recorded as owned elsewhere.
// It leaves out pawl runs without their landing hold.
// It leaves out runs whose board entry is done or gone.
// It adds first passes that have not yet left a status file.
// A task whose backoff has not run out waits, unless its entry changed since the refusal.
// While the cgroup task limit leaves fewer than task_headroom tasks free, the step starts no task.
// While an open front task of the worktree stands, the step starts no other task.
// It starts the front task itself once it is released, even when the runs in flight fill max_runs.
// While the beam is flagged ready, the step starts no task at all.
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
	if d.holdReadyBeam(ctx, tasks) {
		return nil
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	running := d.runsInFlight(current)

	candidates, pastMaxRuns := tasks, false
	if front, open := openFront(tasks, d.geom.Slug); open {
		candidates, pastMaxRuns = []TaskView{front}, true
	}
	for _, task := range candidates {
		if running >= cfg.MaxRuns && !pastMaxRuns {
			break
		}
		if _, flying := d.flights[task.Slug]; flying {
			continue
		}
		covered, err := d.deps.PauseCovers(task.Slug)
		if err != nil {
			return fmt.Errorf("read the pause record: %w", err)
		}
		if released(task, d.geom.WorktreeName) && task.StartRefusal != "" && !task.OwnedElsewhere && len(task.PendingDependencies) == 0 && !covered {
			d.heldStart(task)
			continue
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
		if d.taskLimitHolds(cfg.TaskHeadroom, now) {
			break
		}
		running++
		d.startFirstPass(task, now)
	}
	if len(d.flights) > 0 {
		d.busy = true
	}
	return nil
}

// holdReadyBeam reports whether the beam of this dobby is flagged ready, so the tick starts no task under it.
// While it is, each open member of tasks gets one notice naming it and the cancel that removes it;
// a member filed after the flag blocks the landing.
// An open member is a task of this beam that is not done and has no run listed;
// drafts and tasks held by a dependency count too.
// A member is recorded as noticed only once its post went out, so a failed post is retried on the next tick.
// A reading that is not flagged clears the episode and holds nothing, so a later one notices again.
// A ready read that fails holds the tick's starts, is logged once per episode and changes no record.
// A nil Deps.BeamReady holds nothing.
func (d *daemon) holdReadyBeam(ctx context.Context, tasks []TaskView) bool {
	if d.deps.BeamReady == nil {
		return false
	}
	ready, err := d.deps.BeamReady()
	if err != nil {
		d.readyRead.Fail("dobbyengine: the beam's ready record is unreadable; the dobby starts no task", "worktree", d.geom.WorktreeName, "error", err)
		return true
	}
	d.readyRead.Recover("dobbyengine: the beam's ready record reads again", "worktree", d.geom.WorktreeName)

	record, _, readErr := ReadState(d.geom.StatePath)
	if readErr != nil {
		logger.Warn("dobbyengine: the state record is unreadable, so the ready hold is neither noted nor cleared", "worktree", d.geom.WorktreeName, "error", readErr)
	}
	if !ready {
		if readErr == nil && record.ReadyHold != nil {
			err := UpdateState(d.geom.StatePath, func(s *State) error {
				s.ReadyHold = nil
				return nil
			})
			if err != nil {
				logger.Warn("dobbyengine: the ready hold could not be cleared; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
			}
		}
		return false
	}
	if readErr == nil {
		d.noticeReadyBeamMembers(ctx, tasks, record)
	}
	return true
}

// noticeReadyBeamMembers records the ready hold in the state record.
// It posts one notice for each open member of tasks that the record does not list as noticed.
func (d *daemon) noticeReadyBeamMembers(ctx context.Context, tasks []TaskView, record State) {
	hold := record.ReadyHold
	if hold == nil {
		hold = &ReadyHoldState{}
		err := UpdateState(d.geom.StatePath, func(s *State) error {
			if s.ReadyHold == nil {
				s.ReadyHold = &ReadyHoldState{}
			}
			return nil
		})
		if err != nil {
			logger.Warn("dobbyengine: the ready hold could not be recorded; the next tick tries again", "worktree", d.geom.WorktreeName, "error", err)
			return
		}
	}
	listed := make(map[string]bool, len(d.runs))
	for _, run := range d.runs {
		listed[run.Slug] = true
	}
	for _, task := range tasks {
		if !d.opensReadyBeam(task, listed) || slices.Contains(hold.Notified, task.Slug) {
			continue
		}
		line := fmt.Sprintf("%s: %s was filed after the beam was flagged ready, so it blocks the landing; run `lym batten cancel %s --remove` to end it", d.geom.Slug, task.Slug, task.Slug)
		if err := d.deps.Notice(ctx, line); err != nil {
			d.readyPost.Fail("dobbyengine: a notice of a member of the ready beam did not go out; the next tick tries again", "worktree", d.geom.WorktreeName, "slug", task.Slug, "error", err)
			continue
		}
		d.readyPost.Recover("dobbyengine: the notices of the ready beam's members go out again", "worktree", d.geom.WorktreeName)
		hold.Notified = append(hold.Notified, task.Slug)
		err := UpdateState(d.geom.StatePath, func(s *State) error {
			if s.ReadyHold == nil {
				s.ReadyHold = &ReadyHoldState{}
			}
			s.ReadyHold.Notified = append(s.ReadyHold.Notified, task.Slug)
			return nil
		})
		if err != nil {
			logger.Warn("dobbyengine: a noticed member of the ready beam could not be recorded; the next tick notices it again", "worktree", d.geom.WorktreeName, "slug", task.Slug, "error", err)
		}
	}
}

// opensReadyBeam reports whether task is an open member of this dobby's beam:
// a task of the beam that is not done and has no run among listed.
// A draft's parent is not read, so a task also belongs to the beam when its project is the beam's slug.
func (d *daemon) opensReadyBeam(task TaskView, listed map[string]bool) bool {
	if task.Kind != boardengine.KindTask || boardengine.IsDoneStatus(task.Status) || listed[task.Slug] {
		return false
	}
	return task.Parent == d.geom.WorktreeName || (d.geom.Slug != "" && task.Project == d.geom.Slug)
}

// settleFronts settles the worktree's front tasks through Deps.SettleFronts and stops nothing on its error.
// The first error of an episode is logged and posted once to the nearest live orch;
// a tick that settles cleanly ends the episode.
func (d *daemon) settleFronts(ctx context.Context) {
	if d.deps.SettleFronts == nil {
		return
	}
	err := d.deps.SettleFronts()
	if err == nil {
		d.frontFailures.Recover("dobbyengine: the front tasks settle again", "worktree", d.geom.WorktreeName)
		d.frontNoticed = false
		return
	}
	d.frontFailures.Fail("dobbyengine: settling the front tasks failed", "worktree", d.geom.WorktreeName, "error", err)
	if !d.frontNoticed {
		d.frontNoticed = true
		d.post(ctx, runNoticePrefix+" front task: "+err.Error())
	}
}

// taskLimitHolds reports whether the cgroup task limit holds a start: fewer than headroom tasks are free under it.
// A new run under such a limit could take the host to where every fork fails.
// A headroom of 0, a nil TaskLimit seam and a limit that does not read hold nothing.
// The first held start of an episode logs the reading, and the first start let through after it logs the resume.
func (d *daemon) taskLimitHolds(headroom int, now time.Time) bool {
	var limit proc.TaskLimit
	found := false
	if headroom > 0 && d.deps.TaskLimit != nil {
		limit, found = d.deps.TaskLimit()
	}
	held := found && limit.Headroom() < headroom
	switch {
	case held && !d.taskLimitHeld:
		d.logLine(now, fmt.Sprintf("hold starts: %d of %d tasks in %s, below task_headroom %d", limit.Current, limit.Max, limit.Cgroup, headroom))
	case !held && d.taskLimitHeld && found:
		d.logLine(now, fmt.Sprintf("starts resume: %d of %d tasks in %s", limit.Current, limit.Max, limit.Cgroup))
	case !held && d.taskLimitHeld:
		d.logLine(now, "starts resume: no task limit applies")
	}
	d.taskLimitHeld = held
	return held
}

// runsInFlight counts the runs this dobby drives that take a slot.
// These are the listed runs slotsTaken counts, plus first passes that left no run yet.
func (d *daemon) runsInFlight(current State) int {
	listed := make(map[string]bool, len(d.runs))
	for _, run := range d.runs {
		listed[run.Slug] = true
	}
	count := slotsTaken(d.runs, current)
	for slug, f := range d.flights {
		if f.first && !listed[slug] {
			count++
		}
	}
	return count
}

// slotsTaken counts the runs that take a slot under max_runs:
// a run takes one unless the record marks it owned elsewhere, it is a pawl run without its landing hold,
// or its board entry is done or gone.
// A run whose entry ended has nothing left to start, so batten stopping it short of its teardown holds no slot.
func slotsTaken(runs []RunView, record State) int {
	count := 0
	for _, run := range runs {
		if !record.Runs[run.Slug].OwnedElsewhere && !run.NoSlot && !run.EntryEnded {
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
	d.logLine(now, "start "+task.Slug)
	d.flights[task.Slug] = flight{exit: exit, full: true, first: true, fingerprint: task.Fingerprint}
}

// firstPassExited reads how a first pass exited.
// A pass that left a status file claimed the task: its start record is cleared, and the exit is read as any pass's.
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

// refusedStart records a refused start of slug and schedules its retry backoff.
// It posts one notice per refusal text to the nearest live orch.
func (d *daemon) refusedStart(slug, fingerprint, text string, now time.Time) {
	d.recordRefusal(slug, fingerprint, text, func(s *StartState) { *s = nextStartTry(*s, now, d.probe) })
}

// heldStart records the refusal task carries as its slug's start refusal, with no backoff.
// It is called only for a task nothing else holds; the task starts on the first tick after the refusal is gone.
// It posts one notice per refusal text to the nearest live orch.
func (d *daemon) heldStart(task TaskView) {
	d.recordRefusal(task.Slug, task.Fingerprint, task.StartRefusal, func(s *StartState) { s.NextTry, s.Interval = time.Time{}, 0 })
}

// recordRefusal records text as the last refused start of slug for the entry fingerprint names.
// schedule sets the retry; the notice is posted when the start record has not noticed text yet.
// A start record of another fingerprint is reset first, so a changed entry is noticed afresh.
func (d *daemon) recordRefusal(slug, fingerprint, text string, schedule func(*StartState)) {
	noticeDue := false
	err := d.updateStart(slug, func(s *StartState) {
		if s.EntryFingerprint != fingerprint {
			*s = StartState{}
		}
		schedule(s)
		s.EntryFingerprint, s.Refusal = fingerprint, text
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

// nextStartTry returns st with the next attempt scheduled from now:
// one probe interval after the first refusal, then doubling up to startBackoffCeiling.
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
		case st.NextTry.IsZero() && st.Interval == 0 && len(st.Noticed) == 0 && st.Refusal == "" && st.EntryFingerprint == "":
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
