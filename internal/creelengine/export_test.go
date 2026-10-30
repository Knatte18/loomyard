//go:build tier0

// export_test.go exposes the store internals the external tests drive: a callback that journals and then fails, and an injected journal write fault.

package creelengine

import "errors"

// ErrAfterJournaling is what ReadThenFail returns once it has journaled the read.
var ErrAfterJournaling = errors.New("creel: failed after journaling")

// ReadThenFail journals message id read by holder inside the store's locked section and then fails, as a verb whose later step failed would.
func (s *Store) ReadThenFail(id int64, holder string) error {
	return s.do(func() error {
		if err := s.writer.append(record{Event: eventRead, ID: id, By: holder}); err != nil {
			return err
		}
		return ErrAfterJournaling
	})
}

// FailJournalWrites lets after more journal writes pass and fails every later one with an injected error, and removes the fault for a negative after.
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

// WithBeforeOutcomeWait returns opts with hook called, for each waiting verb, after the daemon answered it pending and before it selects over the outcome, the client closing and the shutdown.
// The hook's arguments report whether an outcome already stands unread in the waiter's channel and whether the daemon's close watch has seen the client leave.
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

// PostUnchecked journals a posted record from sender to receiver and delivers it as post does, without the line check, as a binary before the check did.
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
