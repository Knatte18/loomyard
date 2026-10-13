// drive.go holds the Go driver's settle loop:
// it steps the run through the shared --until-stop loop body, decides each stop by the closed rule table and carries the decision out, leaving a settle record for every move and a stop report for every hand-back.
// The loop works over injected seams, so every branch is testable without a process; the verb that wires the real ones sits beside it.

package loomcli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// goDriverDeps is the Go driver's whole surface to the world.
type goDriverDeps struct {
	// RunID is the run the driver settles.
	RunID string
	// Step runs one --until-stop step, consuming attempt when it is non-empty, and returns the envelope it printed.
	Step func(ctx context.Context, attempt string) ([]byte, error)
	// Goto moves the halted run onto target.
	Goto func(ctx context.Context, target string) error
	// ReadStatus reads the run's status; found is false when it has no status file.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// RunLockHeld reports whether another process holds the run lock.
	RunLockHeld func() (bool, error)
	// SaveEnvelope keeps a stop envelope under the run's drive-reports directory and returns its path.
	SaveEnvelope func(raw []byte) (string, error)
	// SettlePath is the settle-records file in the friction directory.
	SettlePath string
	// EscalationPending reports whether the awaiting Bouncer row producer awaits a decision on its escalation.
	EscalationPending func(producer string) (bool, error)
	// VerifyRecord reads the Publish failure record; found is false when there is none.
	VerifyRecord func() (record battenshed.ChildPublishFailure, found bool, err error)
	// Head returns the task worktree's HEAD commit.
	Head func() (string, error)
	// VerifyRows names the rows whose verify failure the failure record can describe.
	VerifyRows []string
	// Watch reads the batten-watched marker; the bool is true only when that batten is alive.
	Watch func() (watch battenWatch, alive bool)
	// BattenRetrySpent reports whether the watching prime's batten already spent the run's one verify retry.
	BattenRetrySpent func(w battenWatch) (bool, error)
	// Pause reads what the hub's pause record says about the run's slug at now.
	Pause func(now time.Time) (pauserecord.Coverage, error)
	// PauseHold waits until no pause covers the run's slug.
	PauseHold func(ctx context.Context) error
	// NewReportPath returns a fresh stop report path.
	NewReportPath func() string
	// Park holds what a hand-back reads; its ReportPath is replaced by NewReportPath for each hand-back.
	Park driveParkDeps
	// CommitRecords commits the run's records.
	CommitRecords func() error
	// WriteFrictionNote writes the friction note of a paused stop that carries no reason.
	WriteFrictionNote func(stop driveStop) error
	// Now is the clock.
	Now func() time.Time
}

// goDriver settles one run's stops over its seams.
type goDriver struct {
	deps goDriverDeps
}

// run steps the run and settles each stop until the run is done, another holder has it or a hand-back ends the driver.
// A failure the driver cannot act on is handed back; only a failed hand-back or a cancelled context is returned.
func (d *goDriver) run(ctx context.Context) error {
	attempt := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := d.deps.Step(ctx, attempt)
		if err != nil {
			return d.handBackFailure(fmt.Errorf("the step failed to run: %w", err))
		}
		stop, err := parseDriveStop(raw)
		if err != nil {
			return d.handBackFailure(err)
		}
		path, err := d.deps.SaveEnvelope(raw)
		if err != nil {
			return d.handBackFailure(fmt.Errorf("saving the stop envelope: %w", err))
		}
		stop.EnvelopePath = path

		next, exit, err := d.settleStop(ctx, stop)
		if err != nil || exit {
			return err
		}
		attempt = next
	}
}

// settleStop decides stop and carries the decision out.
// It returns the arbiter attempt the next step consumes, and exit true when the driver is finished.
func (d *goDriver) settleStop(ctx context.Context, stop driveStop) (attempt string, exit bool, err error) {
	for {
		facts, err := d.facts(stop)
		if err != nil {
			return "", true, d.handBackStop(stop, driveDecision{Rule: "the driver could not read the facts of the stop"}, err)
		}
		decision := decideDrive(stop, facts)
		switch decision.Action {
		case driveActionDone:
			return "", true, d.finishDone(stop, decision)
		case driveActionBusyExit:
			d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleBusyExit, decision.Rule, "")
			logger.Info("loom: drive: another holder has the run; exiting", "run", d.deps.RunID, "holder", stop.Error)
			return "", true, nil
		}
		if decision.Action != driveActionHandBack && decision.Action != driveActionRelayHandBack {
			if err := d.holdPause(ctx); err != nil {
				return "", true, err
			}
		}
		next, exit, err := d.recheck(stop)
		if err != nil {
			return "", true, d.handBackStop(stop, decision, err)
		}
		if exit {
			return "", true, nil
		}
		if next.Restep {
			return "", false, nil
		}
		if next.key() != stop.key() {
			stop = next
			continue
		}

		switch decision.Action {
		case driveActionRestep:
			d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleRestep, decision.Rule, "")
			return "", false, nil
		case driveActionVerifyGoto:
			d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleVerifyRetry, decision.Rule, facts.VerifyTarget)
			if err := d.deps.Goto(ctx, facts.VerifyTarget); err != nil {
				logger.Warn("loom: drive: the verify goto was refused; rechecking the run", "run", d.deps.RunID, "target", facts.VerifyTarget, "error", err)
				moved, movedExit, recheckErr := d.recheck(stop)
				if recheckErr != nil {
					return "", true, d.handBackStop(stop, decision, recheckErr)
				}
				switch {
				case movedExit:
					return "", true, nil
				case moved.Restep:
					return "", false, nil
				}
				stop = moved
				continue
			}
			return "", false, nil
		case driveActionHandBack, driveActionRelayHandBack:
			return "", true, d.handBackStop(stop, decision, nil)
		}
		// The tackler, the arbiter and the hand-over join the loop in later steps; until then they hand back.
		return "", true, d.handBackStop(stop, decision, nil)
	}
}

// facts reads the disk facts decideDrive needs for stop.
func (d *goDriver) facts(stop driveStop) (driveFacts, error) {
	var facts driveFacts
	key := stop.key()
	if key.State == string(shedengine.StateAwaiting) {
		pending, err := d.deps.EscalationPending(key.CurrentProducer)
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the pending escalation: %w", err)
		}
		facts.EscalationPending = pending
	}

	records, err := loomengine.ReadSettleRecords(d.deps.SettlePath)
	if err != nil {
		return driveFacts{}, err
	}
	facts.RepairCount = repairCount(records, key)
	watch, alive := d.deps.Watch()
	facts.Watched = alive

	if key.State == string(shedengine.StateBlocked) {
		record, found, err := d.deps.VerifyRecord()
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the Publish failure record: %w", err)
		}
		if found {
			head, err := d.deps.Head()
			if err != nil {
				return driveFacts{}, fmt.Errorf("reading the worktree HEAD: %w", err)
			}
			status, statusFound, err := d.deps.ReadStatus()
			if err != nil || !statusFound {
				return driveFacts{}, fmt.Errorf("reading the status to match the failure record: found %t: %v", statusFound, err)
			}
			facts.VerifyMatches = battenshed.RecordMatchesHalt(record, head, status, d.deps.VerifyRows)
			facts.VerifyTarget = record.Target
		}
	}
	if facts.VerifyMatches {
		facts.VerifyRetrySpent = loomengine.GoVerifyRetrySpent(records)
		if watch.ScratchDir != "" {
			spent, err := d.deps.BattenRetrySpent(watch)
			if err != nil {
				return driveFacts{}, fmt.Errorf("reading the batten publish-retry marker: %w", err)
			}
			facts.VerifyRetrySpent = facts.VerifyRetrySpent || spent
		}
	}
	return facts, nil
}

// recheck re-reads the status before the driver acts on stop, so nothing the run's other actors did since the step goes unseen.
// A running status or a run lock another process holds ends the driver with a running-exit record.
// A status at another stop key is a moved run: it returns the stop to act on, marked Restep when an orch goto moved the run, and otherwise decided afresh from the status.
// A moved run is never parked and gets no friction note.
// An unmoved run returns stop itself.
func (d *goDriver) recheck(stop driveStop) (next driveStop, exit bool, err error) {
	status, found, err := d.deps.ReadStatus()
	if err != nil {
		return driveStop{}, false, fmt.Errorf("re-reading the status: %w", err)
	}
	if !found {
		return driveStop{}, false, fmt.Errorf("re-reading the status: the run has no status file")
	}
	held, err := d.deps.RunLockHeld()
	if err != nil {
		return driveStop{}, false, fmt.Errorf("probing the run lock: %w", err)
	}
	if held || (status.State == shedengine.StateRunning && stop.key().State != string(shedengine.StateRunning)) {
		d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleRunningExit, "another process is running the run", "")
		logger.Info("loom: drive: the run is running elsewhere; exiting", "run", d.deps.RunID, "lock_held", held, "state", status.State)
		return stop, true, nil
	}
	if loomengine.StopKeyOf(status) == stop.key() {
		return stop, false, nil
	}

	if n := len(status.History); n > 0 && status.History[n-1].Outcome == shedengine.OutcomeGoto {
		d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleMoved, "an orch goto moved the run: re-step", status.CurrentProducer)
		moved := stopFromStatus(status)
		moved.Restep = true
		return moved, false, nil
	}

	moved := stopFromStatus(status)
	raw, err := json.Marshal(map[string]any{
		"state":  moved.State,
		"reason": "",
		"loop":   map[string]any{"current_producer": status.CurrentProducer, "history_length": len(status.History)},
	})
	if err != nil {
		return driveStop{}, false, fmt.Errorf("encoding the moved run's envelope: %w", err)
	}
	path, err := d.deps.SaveEnvelope(raw)
	if err != nil {
		return driveStop{}, false, fmt.Errorf("saving the moved run's envelope: %w", err)
	}
	moved.Raw = raw
	moved.EnvelopePath = path
	d.settle(stop, loomengine.SettlerGoRule, loomengine.SettleMoved, "the run moved to another stop: decide it afresh", status.CurrentProducer)
	return moved, false, nil
}

// holdPause returns once no pause covers the run's slug, waiting in PauseHold while one does.
// An unreadable pause record holds nothing, with one Warn.
func (d *goDriver) holdPause(ctx context.Context) error {
	for {
		coverage, err := d.deps.Pause(d.deps.Now())
		if err != nil {
			logger.Warn("loom: drive: the pause record is unreadable; not holding", "run", d.deps.RunID, "error", err)
			return nil
		}
		if !coverage.Covered {
			return nil
		}
		if err := d.deps.PauseHold(ctx); err != nil {
			return err
		}
	}
}

// settle appends one settle record for the action taken at stop; a failed append is logged and never stops the driver.
func (d *goDriver) settle(stop driveStop, settler, action, outcome, target string) {
	record := loomengine.SettleRecord{StopKey: stop.key(), Settler: settler, Action: action, Outcome: outcome, At: d.deps.Now(), Target: target}
	if err := loomengine.AppendSettleRecord(d.deps.SettlePath, record); err != nil {
		logger.Warn("loom: drive: appending a settle record failed", "run", d.deps.RunID, "action", action, "error", err)
	}
}

// finishDone commits the run's records, and hands a run that finished with friction: failed to the parent without parking it.
func (d *goDriver) finishDone(stop driveStop, decision driveDecision) error {
	if err := d.deps.CommitRecords(); err != nil {
		return fmt.Errorf("loom: drive: commit the run's records: %w", err)
	}
	if stop.Friction != "failed" {
		return nil
	}
	return d.handBack(stop, driveHandBack{Rules: []string{decision.Rule}, DoneFrictionFailed: true})
}

// handBackStop hands the run back at stop on the decision's rule, after the friction note a paused stop without a reason asks for.
// cause, when set, is a failure that kept the driver from acting and goes into the rules the report quotes.
func (d *goDriver) handBackStop(stop driveStop, decision driveDecision, cause error) error {
	if decision.FrictionNote {
		if err := d.deps.WriteFrictionNote(stop); err != nil {
			logger.Warn("loom: drive: writing the friction note failed", "run", d.deps.RunID, "error", err)
		}
	}
	rules := []string{decision.Rule}
	if cause != nil {
		rules = append(rules, "the driver could not act: "+cause.Error())
	}
	d.settle(stop, loomengine.SettlerParent, loomengine.SettleHandBack, decision.Rule, "")
	h := driveHandBack{Rules: rules, WatchedVerifyLine: decision.WatchedVerifyLine}
	if decision.Action == driveActionRelayHandBack {
		h.Relay = stop.ParentNotice
	}
	return d.handBack(stop, h)
}

// handBackFailure hands the run back at the stop its status shows, or at an empty stop when the status cannot be read, for a failure that left no stop to decide.
func (d *goDriver) handBackFailure(cause error) error {
	var stop driveStop
	if status, found, err := d.deps.ReadStatus(); err == nil && found {
		stop = stopFromStatus(status)
	}
	return d.handBackStop(stop, driveDecision{Action: driveActionHandBack, Rule: "a failure the driver cannot act on: hand back"}, cause)
}

// handBack builds the stop report for stop and h, and leaves the report, the park marker and the parent's notices through handBackRun.
func (d *goDriver) handBack(stop driveStop, h driveHandBack) error {
	reportPath := d.deps.NewReportPath()
	park := d.deps.Park
	park.ReportPath = reportPath

	h.RunID = d.deps.RunID
	h.ReportPath = reportPath
	h.Stop = stop
	reason := stop.Reason
	if reason == "" {
		reason = stop.Error
	}
	if reason == "" {
		reason = stop.key().State
	}
	report := driveReport{
		RunID:         d.deps.RunID,
		HistoryLength: stop.HistoryLength,
		Progress:      fmt.Sprintf("%s at %s", stop.key().State, stop.CurrentProducer),
		Reason:        reason,
		Rules:         h.Rules,
		Attempts:      h.Attempts,
		Question:      h.Question,
		FrictionDir:   filepath.Dir(d.deps.SettlePath),
		RepairDir:     filepath.Dir(reportPath),
	}
	_, watched := d.deps.Watch()
	return handBackRun(park, h, report, watched)
}
