// dialog.go holds the pending-dialog latch of an autonomous run's wait loop and the notice it sends the parent once a dialog has stood for a bound.

package shuttleengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// permissionNoticeBound is how long a permission or input dialog stands on a run's pane before its parent is notified.
// It is long enough that an operator answering a dialog at once sees no notice, and short enough to surface a stall well inside any run's timeout.
const permissionNoticeBound = 60 * time.Second

// dialogLatch is the in-memory state of one Wait's pending-dialog watch.
type dialogLatch struct {
	// cursor is the events-file offset the next ask read resumes from.
	cursor int64
	// set is true while an ask has been folded and no input box has shown since.
	set bool
	// since is the stamp of the ask that set the latch.
	since time.Time
	// latestOffset is the cursor after the batch holding the latest ask folded into the current episode.
	latestOffset int64
	// notified is true once the current episode's notice was sent.
	notified bool
}

// dialogNotice builds the single-line notice for a dialog on the pane of the agent named strandName.
// It names the strand and says the pane shows a permission or input dialog lyx cannot answer.
// The way forward it gives is that the operator answers it in the pane, reached with `lyx reed attach` in the run's worktree.
func dialogNotice(strandName string) string {
	return fmt.Sprintf("Shuttle notice: Agent %s has a permission or input dialog on its pane that lyx cannot answer. "+
		"To answer it, attach to the pane with `lyx reed attach` in the run's worktree.", strandName)
}

// watchDialog advances the dialog latch one tick and notifies the parent once when the latch has stood for permissionNoticeBound with no input box on the pane.
// It does nothing for an interactive run, a runner with no notifier, or an engine that cannot parse session signals or read an input box.
// A failed capture, which warns once per episode, or a pane too short to draw the box changes nothing and is retried on a later tick.
// While a pause covers the run the latch stands but no notice is sent.
// It never types into the pane and never changes the run's outcome.
func (run *Run) watchDialog() {
	if run.runner.notifier == nil || run.spec.Interactive {
		return
	}
	parser, canParse := run.runner.engine.(SessionSignalParser)
	_, canRead := run.runner.engine.(InputBoxReader)
	if !canParse || !canRead {
		return
	}
	run.foldDialogAsks(parser)
	if !run.dialog.set {
		return
	}
	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	if err != nil {
		run.failures.Fail("dialog capture", "shuttle: capture pane for the dialog watch failed, retrying on a later tick", "strandGUID", run.state.StrandGUID, "error", err)
		return
	}
	run.failures.Recover("dialog capture", "shuttle: capture pane for the dialog watch works again", "strandGUID", run.state.StrandGUID)
	box := readBox(run.runner.engine, capture)
	switch {
	case box.tooShort:
		return
	case box.read:
		run.dialog.set, run.dialog.notified = false, false
	case !run.dialog.notified && !run.covered && run.clock.Now().Sub(run.dialog.since) >= permissionNoticeBound:
		run.dialog.notified = run.notifyDialog()
	}
}

// foldDialogAsks reads the events file from the latch's cursor and folds each ask that is not also an event into the latch.
// The first ask while the latch is clear sets it, stamped with the signal's time or, when that is zero, the run clock's;
// a further ask while it is set keeps the stamp.
// A later turn end or turn start never clears the latch.
// When an ask folds into an episode whose notice was sent, the cursor after the batch is persisted as RunState.DialogNotifiedOffset,
// so a reattach never rebuilds a latch from that episode's later asks.
func (run *Run) foldDialogAsks(parser SessionSignalParser) {
	data, _, err := readEventsFrom(run.state.EventsPath, run.dialog.cursor)
	if err != nil || len(data) == 0 {
		return
	}
	signals, consumed := parser.ParseSessionSignals(data)
	run.dialog.cursor += int64(consumed)
	folded := false
	for _, signal := range signals {
		if signal.Kind != SessionSignalAsk || signal.Event {
			continue
		}
		folded = true
		if !run.dialog.set {
			since := signal.At
			if since.IsZero() {
				since = run.clock.Now()
			}
			run.dialog.set, run.dialog.since, run.dialog.notified = true, since, false
		}
	}
	if !folded {
		return
	}
	run.dialog.latestOffset = run.dialog.cursor
	if run.dialog.notified {
		if err := run.persistDialogNotifiedOffset(run.dialog.latestOffset); err != nil {
			logger.Warn("shuttle: persist dialog notified offset failed", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
		}
	}
}

// notifyDialog sends the parent one notice for the current dialog episode and reports whether the notice counts as sent.
// The latest folded offset is persisted as RunState.DialogNotifiedOffset before the notifier is called, so a crash between the write and the call loses that one notice and never repeats it.
// A write that fails skips the notice and returns false; a notifier error is only logged and still counts as sent.
func (run *Run) notifyDialog() bool {
	if err := run.persistDialogNotifiedOffset(run.dialog.latestOffset); err != nil {
		logger.Warn("shuttle: persist dialog notified offset failed, skipping this dialog notice", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
		return false
	}
	strandName := run.state.StrandName
	if strandName == "" {
		strandName = run.state.StrandGUID
	}
	if err := run.runner.notifier(dialogNotice(strandName)); err != nil {
		logger.Warn("shuttle: dialog notice failed", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
	}
	return true
}

// persistDialogNotifiedOffset writes offset as RunState.DialogNotifiedOffset, restoring the previous value when the write fails.
func (run *Run) persistDialogNotifiedOffset(offset int64) error {
	run.recordMu.Lock()
	defer run.recordMu.Unlock()
	previous := run.state.DialogNotifiedOffset
	run.state.DialogNotifiedOffset = offset
	if err := run.saveState(); err != nil {
		run.state.DialogNotifiedOffset = previous
		return err
	}
	return nil
}
