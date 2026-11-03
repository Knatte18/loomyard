// work.go declares advisorWork, the predicate the chair's held-turn notice defers on: whether advisor work is still outstanding.

package seatengine

import (
	"sync"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// advisorWork holds the advisors a table run has started and the notice sender that types their lines to the chair.
// It reports whether any of that work is outstanding, which is what keeps a chair's planned wait on its advisors from reaching the parent as a stall.
type advisorWork struct {
	mu     sync.Mutex
	sender *noticeSender
	runs   []*seatRun
}

// add records a started advisor.
func (w *advisorWork) add(run *seatRun) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.runs = append(w.runs, run)
}

// bindSender sets the notice sender whose delivery of the up lines outstanding checks.
func (w *advisorWork) bindSender(sender *noticeSender) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sender = sender
}

// outstanding reports whether any started seat is still working.
// An advisor whose run has not ended counts when it has an up line the sender has not landed or reads mid-turn.
// An unended assistant, which gets no up line either, counts when its session reads mid-turn.
// A writer, which gets no up line, counts when its session reads mid-turn, whether its run has ended done or not, so a chair waiting on a fix it asked of a done writer is deferred.
// Any other ended run never counts, a timed-out one included.
// Each seat's session state is read before its up line: one reading asking, idle-stalled, idle-done or dead is held or gone, so it never counts, whether or not an advisor's up line has landed.
// A session-state reading that errors counts as neither held nor mid-turn, so only an advisor's up line can still count that seat.
func (w *advisorWork) outstanding() bool {
	w.mu.Lock()
	sender := w.sender
	runs := append([]*seatRun(nil), w.runs...)
	w.mu.Unlock()

	for _, run := range runs {
		if run.handle == nil || (run.ended() && !(run.seat.Writer && run.finishedDone())) {
			continue
		}
		state, err := run.handle.SessionState()
		if err == nil {
			switch state.State.Name {
			case shuttleengine.SessionAsking, shuttleengine.SessionIdleStalled, shuttleengine.SessionIdleDone, shuttleengine.SessionDead:
				continue
			}
		}
		if !run.seat.Writer && !run.seat.Assistant && sender != nil && !sender.wasSent(advisorUpNotice(run.name)) {
			return true
		}
		if err == nil && state.State.Name == shuttleengine.SessionBusy && state.State.Cause == shuttleengine.SessionCauseTurn {
			return true
		}
	}
	return false
}
