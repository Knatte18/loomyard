// view.go builds the View of one dobby from what a reader outside its process can read: its liveness, its state record, beam.yaml, the runs it drives and the board.

package dobbyengine

import (
	"slices"
	"strings"
	"time"
)

// View is what one reading of a dobby shows: whether it lives, the runs it drives and the released tasks it has not started.
type View struct {
	// Alive reports whether the dobby's lock is held.
	Alive bool `json:"alive"`
	// MaxRuns is beam.yaml's cap on the runs in flight.
	MaxRuns int `json:"max_runs"`
	// Runs are the runs the dobby drives that are not done, by slug.
	Runs []ViewRun `json:"runs"`
	// Waiting are the released tasks not yet started, in board order.
	Waiting []WaitingTask `json:"waiting"`
}

// ViewRun is one run a dobby drives, as its batten status and the dobby's state record read.
type ViewRun struct {
	Slug     string `json:"slug"`
	State    string `json:"state"`
	Producer string `json:"producer"`
	// PassInFlight reports whether the run's driver lock is held, which a pass in flight does.
	PassInFlight bool `json:"pass_in_flight,omitempty"`
	// LastPassExit is when the run's last pass exited; zero when the record holds none.
	LastPassExit     time.Time `json:"last_pass_exit,omitzero"`
	FullPassOwed     bool      `json:"full_pass_owed,omitempty"`
	OwnedElsewhere   bool      `json:"owned_elsewhere,omitempty"`
	FailingPassError string    `json:"failing_pass_error,omitempty"`
	FailingPassCount int       `json:"failing_pass_count,omitempty"`
}

// WaitingTask is one released task a dobby has not started, with everything that holds it back.
type WaitingTask struct {
	Slug string `json:"slug"`
	// Dependencies are the slugs of the entries it depends on that are not done.
	Dependencies   []string `json:"dependencies,omitempty"`
	OwnedElsewhere bool     `json:"owned_elsewhere,omitempty"`
	// Paused reports whether a pause entry covers the hub or the slug.
	Paused bool `json:"paused,omitempty"`
	// Refusal is the text of the last refused start of the entry as it reads now.
	Refusal string `json:"refusal,omitempty"`
	// NextTry is when the refused start's backoff runs out; zero once it has.
	NextTry time.Time `json:"next_try,omitzero"`
	// AtMaxRuns reports whether the runs in flight fill max_runs before this task's turn.
	AtMaxRuns bool `json:"at_max_runs,omitempty"`
}

// Ready reports whether nothing holds the task back, so the dobby starts it on its next tick.
func (w WaitingTask) Ready() bool {
	return len(w.Dependencies) == 0 && !w.OwnedElsewhere && !w.Paused && w.NextTry.IsZero() && !w.AtMaxRuns
}

// ViewInput is everything BuildView reads.
type ViewInput struct {
	// Worktree is the name of the dobby's worktree, the prime's name for the prime.
	Worktree string
	Alive    bool
	Config   Config
	Record   State
	Runs     []RunView
	Tasks    []TaskView
	// Covers reports whether a live pause entry covers the hub or slug.
	Covers func(slug string) bool
	Now    time.Time
}

// BuildView returns the View of the dobby in.
// The runs are the listed ones that are not done, and a done run takes a slot only while its driver lock is held; a task waits while it is released to the worktree and not started, and its reasons are the conditions the dobby's start step checks, in the start step's own order for max_runs.
func BuildView(in ViewInput) View {
	view := View{Alive: in.Alive, MaxRuns: in.Config.MaxRuns, Runs: []ViewRun{}, Waiting: []WaitingTask{}}
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
		})
	}

	slots := in.Config.MaxRuns - slotsTaken(slotRuns, in.Record)
	for _, task := range in.Tasks {
		if !released(task, in.Worktree) {
			continue
		}
		covered := in.Covers(task.Slug)
		waiting := WaitingTask{Slug: task.Slug, Dependencies: task.PendingDependencies, OwnedElsewhere: task.OwnedElsewhere, Paused: covered}
		if attempt, found := in.Record.Starts[task.Slug]; found && attempt.EntryFingerprint == task.Fingerprint {
			waiting.Refusal = attempt.Refusal
			if in.Now.Before(attempt.NextTry) {
				waiting.NextTry = attempt.NextTry
			}
		}
		if !heldByEntry(task, covered) && waiting.NextTry.IsZero() {
			waiting.AtMaxRuns = slots <= 0
			slots--
		}
		view.Waiting = append(view.Waiting, waiting)
	}
	return view
}
