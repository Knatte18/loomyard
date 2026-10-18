// report.go declares the seams the pause, resume, revive and status sequences read through and the reports they return, with the holds report shared by pause and status.

package tenderengine

import (
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// SessionInfo is one live shuttle-run strand of a pair.
type SessionInfo struct {
	// Strand is the reed strand name.
	Strand string `json:"strand"`
	GUID   string `json:"guid"`
	// Slug is the pair's slug.
	Slug string `json:"slug"`
}

// Pair is one hub pair as the sequences see it.
type Pair interface {
	// Slug is the pair's slug.
	Slug() string
	// Sessions lists the pair's live shuttle-run strands, the loom driver included.
	Sessions() ([]SessionInfo, error)
	// State reads the session's state from its run's files, the zero state when the strand has none.
	State(guid string) (shuttleengine.SessionState, error)
	// Busy reports whether the session is in a running turn: busy with any cause but background work.
	Busy(guid string) (bool, error)
	// Background reports whether the session's turn ended with background work still running, with the outstanding tasks' labels.
	Background(guid string) (labels []string, background bool, err error)
	// PaneRunningTurn reports whether the session's pane shows a running turn, from one fresh capture.
	PaneRunningTurn(guid string) (bool, error)
	// InterruptSettle is how long after an interrupt key the strand's pane needs before it is read again.
	// It fails when guid resolves to no run or to an engine the set does not hold.
	InterruptSettle(guid string) (time.Duration, error)
	// Live reports whether the strand exists with a live pane.
	Live(guid string) (bool, error)
	// Interrupt sends the provider's interrupt key through shuttle's guarded interrupt.
	// It fails with shuttleengine.ErrNoRunningTurn when the pane shows no running turn, and with shuttleengine.ErrRewindPickerOpen when it closed an open rewind picker.
	Interrupt(guid string) error
	// Send types text into the session through the guarded paste-submit.
	Send(guid, text string) error
	// SendConditioned types text into the session only while one ready reading shows it in the state want names, and reports whether a turn start followed.
	// It fails with shuttleengine.ErrSendConditionUnmet when the session reads another state and with shuttleengine.ErrSubmissionNotLanded when the text did not land.
	SendConditioned(guid, text string, want shuttleengine.SendCondition) (turnStarted bool, err error)
	// Running reports the pair's current producer and whether its run reads running.
	Running() (producer string, running bool, err error)
}

// Deps is everything the sequences read and write, all of it told.
type Deps struct {
	// RecordPath is the pause record file.
	RecordPath string
	// Pairs are the hub's task pairs, the prime excluded.
	Pairs []Pair
	// Prime is the prime's sessions, nil when the hub has no prime that wires.
	Prime Pair
	// Processes lists the processes running on the machine.
	Processes func() ([]proc.Process, error)
	// HubPath is the hub directory.
	HubPath string
	// Executable is the running lyx's own executable path, empty when it is unknown.
	Executable string
	// NetOps lists the network operations in flight.
	NetOps func() ([]pauserecord.NetOp, error)
	// Locks lists the parent lock records.
	Locks func() ([]parentlock.Entry, error)
	// OrchWatcherLive reports whether an orch watcher is running.
	OrchWatcherLive func() bool
	// OrchStrand is the orch strand recorded in the prime's orch state, empty when none.
	OrchStrand func() (string, error)
	// OrchRevivePending reports whether a revive inject of tender's own to the orch strand stands with no outcome yet.
	OrchRevivePending func() (bool, error)
	// InjectOrchRevive injects line into the orch strand and returns the inject's id.
	InjectOrchRevive func(line string) (int64, error)
	// Caller is the caller's strand name, empty for an external caller.
	Caller string
	// Now is the clock.
	Now func() time.Time
	// Sleep waits for a duration.
	Sleep func(time.Duration)
}

// SessionNote names a session with the reason the sequence did what it did to it.
type SessionNote struct {
	Strand string `json:"strand"`
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// NetOpReport is one network operation in flight.
type NetOpReport struct {
	Op     string `json:"op"`
	Strand string `json:"strand"`
	Slug   string `json:"slug"`
	// UnderParentLock reports whether the process held its slug's parent lock when the operation began.
	UnderParentLock bool `json:"under_parent_lock"`
}

// HeldBranch is a parent branch a covered run holds the lock on, with the runs queued behind it.
type HeldBranch struct {
	Branch string   `json:"branch"`
	Slug   string   `json:"slug"`
	Queued []string `json:"queued"`
}

// WaitingRun is a covered run waiting in a parent lock queue, with the runs queued behind it.
type WaitingRun struct {
	Slug   string   `json:"slug"`
	Branch string   `json:"branch"`
	Behind []string `json:"behind"`
}

// holdReport is what a pause leaves held on the parent locks.
type holdReport struct {
	Held    []HeldBranch `json:"held"`
	Waiting []WaitingRun `json:"waiting"`
}

// GoProducer is a covered run that reads running with no live session: it advances through local Go work until it reaches a held path.
type GoProducer struct {
	Slug     string `json:"slug"`
	Producer string `json:"producer"`
}

// OrchCase says what became of the orch under a hub pause.
type OrchCase string

const (
	// OrchRanPause means the orch ran the pause itself, so nothing interrupts it.
	OrchRanPause OrchCase = "ran-the-pause"
	// OrchDelegated means a live orch watcher interrupts the orch at the grace end.
	OrchDelegated OrchCase = "delegated-to-the-orch-watcher"
	// OrchNotInterrupted means no orch watcher is live, so the orch is not interrupted.
	OrchNotInterrupted OrchCase = "not-interrupted-no-orch-watcher-is-live"
)

// readHolds reports the parent branches the covered runs hold and the covered runs waiting in a queue.
// A run is covered when a live entry of rec covers its slug now.
func readHolds(deps Deps, rec pauserecord.Record) (holdReport, error) {
	report := holdReport{Held: []HeldBranch{}, Waiting: []WaitingRun{}}
	if deps.Locks == nil {
		return report, nil
	}
	entries, err := deps.Locks()
	if err != nil {
		return report, err
	}
	covered := func(slug string) bool {
		_, ok := rec.Covering(slug)
		return ok
	}
	for _, entry := range entries {
		if entry.Err != "" {
			continue
		}
		waiters := append([]parentlock.Ticket(nil), entry.Waiters...)
		sort.SliceStable(waiters, func(i, j int) bool { return waiters[i].QueuedAt.Before(waiters[j].QueuedAt) })
		queued := make([]string, 0, len(waiters))
		for _, waiter := range waiters {
			queued = append(queued, waiter.Owner.Slug)
		}
		if holder := entry.Record.Owner.Slug; covered(holder) {
			report.Held = append(report.Held, HeldBranch{Branch: entry.Record.Branch, Slug: holder, Queued: queued})
		}
		for i, waiter := range waiters {
			if covered(waiter.Owner.Slug) {
				report.Waiting = append(report.Waiting, WaitingRun{Slug: waiter.Owner.Slug, Branch: entry.Record.Branch, Behind: queued[i+1:]})
			}
		}
	}
	return report, nil
}

// netOpReports lists the network operations in flight that covers selects, each marked when run under a parent lock.
func netOpReports(deps Deps, covers func(slug string) bool) ([]NetOpReport, error) {
	reports := []NetOpReport{}
	if deps.NetOps == nil {
		return reports, nil
	}
	ops, err := deps.NetOps()
	if err != nil {
		return reports, err
	}
	for _, op := range ops {
		if covers(op.Slug) {
			reports = append(reports, NetOpReport{Op: op.Op, Strand: op.Strand, Slug: op.Slug, UnderParentLock: op.UnderParentLock})
		}
	}
	return reports, nil
}

// isOrchStrand reports whether strand names the orch of the prime.
func isOrchStrand(strand string) bool {
	name, err := agentname.Parse(strand)
	return err == nil && name.Role == agentname.RoleOrch && name.Slug == ""
}
