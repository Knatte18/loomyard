// export_test.go exposes the store internals the external tests drive.
// It holds a callback that journals and then fails, and an injected journal write fault.

package creelengine

import "errors"

// ErrAfterJournaling is what ReadThenFail returns once it has journaled the read.
var ErrAfterJournaling = errors.New("creel: failed after journaling")

// ReadThenFail journals message id read by holder inside the store's locked section and then fails.
// It stands in for a verb whose step after the journal write failed.
func (s *Store) ReadThenFail(id int64, holder string) error {
	return s.do(func() error {
		if err := s.writer.append(record{Event: eventRead, ID: id, By: holder}); err != nil {
			return err
		}
		return ErrAfterJournaling
	})
}

// FailJournalWrites lets after more journal writes pass and fails every later one with an injected error.
// A negative after removes the fault.
func (s *Store) FailJournalWrites(after int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if after < 0 {
		s.writer.writeFault = nil
		return
	}
	passed := 0
	s.writer.writeFault = func() error {
		if passed < after {
			passed++
			return nil
		}
		return errors.New("creel journal: injected write fault")
	}
}

// WithBeforeOutcomeWait returns opts with hook called for each waiting verb.
// The hook runs once the daemon has answered the verb pending.
// It runs before the verb selects over the outcome, the client closing, and the shutdown.
// The hook's first argument reports whether an outcome already stands unread in the waiter's channel.
// Its second argument reports whether the daemon's close watch has seen the client leave.
func WithBeforeOutcomeWait(opts Options, hook func(outcomeQueued, clientLeft func() bool)) Options {
	opts.beforeOutcomeWait = func(waiter *Waiter, clientClosed <-chan struct{}) {
		hook(
			func() bool { return len(waiter.ch) > 0 },
			func() bool {
				select {
				case <-clientClosed:
					return true
				default:
					return false
				}
			},
		)
	}
	return opts
}

// PostUnchecked journals a posted record from sender to receiver and delivers it without the line check.
func (s *Store) PostUnchecked(sender Sender, receiver, body string) (int64, error) {
	strands, err := s.readStrands()
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.do(func() error {
		if err := s.checkReceiverLocked(strands, receiver); err != nil {
			return err
		}
		var err error
		if id, err = s.appendPostedLocked(sender, record{To: receiver, Kind: kindMail, Body: body}); err != nil {
			return err
		}
		return s.deliverMailLocked(id, receiver)
	})
	return id, err
}
