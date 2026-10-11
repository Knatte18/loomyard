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

// outstanding reports whether any advisor whose run has not ended either has an up line the sender has not landed or reads mid-turn.
// An advisor whose run ended never counts, a timed-out one included, and a session-state reading that errors counts as not mid-turn.
func (w *advisorWork) outstanding() bool {
	w.mu.Lock()
	sender := w.sender
	runs := append([]*seatRun(nil), w.runs...)
	w.mu.Unlock()

	for _, run := range runs {
		if run.handle == nil || run.ended() {
			continue
		}
		if sender != nil && !sender.wasSent(advisorUpNotice(run.name)) {
			return true
		}
		state, err := run.handle.SessionState()
		if err == nil && state.State.Name == shuttleengine.SessionBusy && state.State.Cause == shuttleengine.SessionCauseTurn {
			return true
		}
	}
	return false
}
