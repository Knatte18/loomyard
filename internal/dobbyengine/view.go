// view.go builds the View of one dobby from what a reader outside its process can read.
// It draws on the dobby's liveness, its state record, beam.yaml, the runs it drives and the board.

package dobbyengine

import (
	"slices"
	"strings"
	"time"
)

// View is one reading of a dobby: whether it is alive, its max_runs and land_notice.
// It also lists the runs it drives and the released tasks it has not started.
// Each run and waiting task says whether it asks for a landing notice.
type View struct {
	// Alive reports whether the dobby's lock is held.
	Alive bool `json:"alive"`
	// MaxRuns is beam.yaml's cap on the runs in flight.
	MaxRuns int `json:"max_runs"`
	// LandNotice is beam.yaml's land_notice: which runs get a landing notice.
	LandNotice string `json:"land_notice"`
	// Runs are the runs the dobby drives that are not done, by slug.
	Runs []ViewRun `json:"runs"`
	// Waiting are the released tasks not yet started, in board order.
	Waiting []WaitingTask `json:"waiting"`
}

// ViewRun is one run a dobby drives, as read from its batten status and the dobby's state record.
type ViewRun struct {
	Slug     string `json:"slug"`
	State    string `json:"state"`
	Producer string `json:"producer"`
	// PassInFlight reports whether the run's driver lock is held, which means a pass is in flight.
	PassInFlight bool `json:"pass_in_flight,omitempty"`
	// LastPassExit is when the run's last pass exited; zero when the record holds none.
	LastPassExit     time.Time `json:"last_pass_exit,omitzero"`
	FullPassOwed     bool      `json:"full_pass_owed,omitempty"`
	OwnedElsewhere   bool      `json:"owned_elsewhere,omitempty"`
	FailingPassError string    `json:"failing_pass_error,omitempty"`
	FailingPassCount int       `json:"failing_pass_count,omitempty"`
	// NotifyOnLand reports whether the run's board entry asks for a landing notice, or asked for it while the dobby saw the run ahead of its Land row.
	NotifyOnLand bool `json:"notify_on_land,omitempty"`
}

// WaitingTask is one released task a dobby has not started, with everything that holds it back.
type WaitingTask struct {
	Slug string `json:"slug"`
	// Dependencies are the slugs of the entries it depends on that are not done.
	Dependencies   []string `json:"dependencies,omitempty"`
	OwnedElsewhere bool     `json:"owned_elsewhere,omitempty"`
	// Paused reports whether a pause entry covers the hub or the slug.
	Paused bool `json:"paused,omitempty"`
	// StartRefusal is the refusal every start of the slug gets, such as one for a finished run record of the same slug;
	// while it stands, no pass is spawned.
	StartRefusal string `json:"start_refusal,omitempty"`
	// Refusal is the text of the last refused start of the entry, if that start was for the entry as it now reads.
	Refusal string `json:"refusal,omitempty"`
	// NextTry is when the refused start's backoff runs out; zero once it has.
	NextTry time.Time `json:"next_try,omitzero"`
	// AtMaxRuns reports whether the runs in flight fill max_runs before this task's turn.
	AtMaxRuns bool `json:"at_max_runs,omitempty"`
	// NotifyOnLand reports whether the task's board entry asks for a landing notice.
	NotifyOnLand bool `json:"notify_on_land,omitempty"`
	// FrontFirst is the slug of the worktree's open front task; while it stands, no other task starts.
	FrontFirst string `json:"front_first,omitempty"`
	// ReadyHold reports whether the beam is flagged ready, which holds every start of the worktree back.
	ReadyHold bool `json:"ready_hold,omitempty"`
}

// Ready reports whether nothing holds the task back, so the dobby starts it on its next tick.
func (w WaitingTask) Ready() bool {
	return len(w.Dependencies) == 0 && !w.OwnedElsewhere && !w.Paused && w.StartRefusal == "" && w.NextTry.IsZero() && !w.AtMaxRuns && w.FrontFirst == "" && !w.ReadyHold
}

// ViewInput is everything one view is built from.
type ViewInput struct {
	// Worktree is the name of the dobby's worktree; for the prime's dobby, it is the prime's name.
	Worktree string
	// Project is the dobby's beam slug, empty for the prime's dobby; it names the front tasks that hold this dobby's starts.
	Project string
	Alive   bool
	Config  Config
	Record  State
	Runs    []RunView
	Tasks   []TaskView
	// Covers reports whether a live pause entry covers the hub or slug.
	Covers func(slug string) bool
	Now    time.Time
}

// BuildView returns the View of the dobby in.
// The runs are the listed ones that are not done; a done run takes a slot only while its driver lock is held.
// A task waits while it is released to the worktree and not started.
// Its reasons are the conditions the dobby's start step checks, in the start step's own order for max_runs.
// An open front task of the worktree holds every other waiting task back and takes no slot under max_runs.
// While the record carries the ready hold, every waiting task is held and none takes a slot.
func BuildView(in ViewInput) View {
	view := View{Alive: in.Alive, MaxRuns: in.Config.MaxRuns, LandNotice: in.Config.LandNotice, Runs: []ViewRun{}, Waiting: []WaitingTask{}}
	requested := make(map[string]bool, len(in.Tasks))
	for _, task := range in.Tasks {
		requested[task.Slug] = task.NotifyOnLand
	}
	// A done run whose driver lock is held has a pass in flight, which keeps it in the scheduler's list and so in its slot count.
	// A reader outside the dobby's process cannot tell the dobby's own pass from a hand-run driver, so the count is the closest the held lock gives.
	slotRuns := slices.DeleteFunc(slices.Clone(in.Runs), func(run RunView) bool { return run.State == runStateDone && !run.DriverLockHeld })
	runs := slices.DeleteFunc(slices.Clone(in.Runs), func(run RunView) bool { return run.State == runStateDone })
	slices.SortFunc(runs, func(a, b RunView) int { return strings.Compare(a.Slug, b.Slug) })
	for _, run := range runs {
		rs := in.Record.Runs[run.Slug]
		view.Runs = append(view.Runs, ViewRun{
			Slug: run.Slug, State: run.State, Producer: run.Producer,
			PassInFlight: run.DriverLockHeld, LastPassExit: rs.LastPassExit,
			FullPassOwed: rs.FullPassOwed, OwnedElsewhere: rs.OwnedElsewhere,
			FailingPassError: rs.FailingPassError, FailingPassCount: rs.FailingPassCount,
			NotifyOnLand: requested[run.Slug] || in.Record.LandNotices[run.Slug].Requested,
		})
	}

	slots := in.Config.MaxRuns - slotsTaken(slotRuns, in.Record)
	front, frontOpen := openFront(in.Tasks, in.Project)
	for _, task := range in.Tasks {
		if !released(task, in.Worktree) {
			continue
		}
		covered := in.Covers(task.Slug)
		waiting := WaitingTask{Slug: task.Slug, Dependencies: task.PendingDependencies, OwnedElsewhere: task.OwnedElsewhere, Paused: covered, StartRefusal: task.StartRefusal, NotifyOnLand: task.NotifyOnLand, ReadyHold: in.Record.ReadyHold != nil}
		if frontOpen && task.Slug != front.Slug {
			waiting.FrontFirst = front.Slug
		}
		if attempt, found := in.Record.Starts[task.Slug]; found && attempt.EntryFingerprint == task.Fingerprint {
			waiting.Refusal = attempt.Refusal
			if in.Now.Before(attempt.NextTry) {
				waiting.NextTry = attempt.NextTry
			}
		}
		isFront := frontOpen && task.Slug == front.Slug
		if !heldByEntry(task, covered) && waiting.NextTry.IsZero() && waiting.FrontFirst == "" && !isFront && !waiting.ReadyHold {
			waiting.AtMaxRuns = slots <= 0
			slots--
		}
		view.Waiting = append(view.Waiting, waiting)
	}
	return view
}
