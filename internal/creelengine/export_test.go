// export_test.go exposes the store internals the external tests drive.
// It holds the single-purpose post wrappers and a callback that journals and then fails.
// It also holds an injected journal write fault.

package creelengine

import "errors"

// ErrAfterJournaling is what ReadThenFail returns once it has journaled the read.
var ErrAfterJournaling = errors.New("creel: failed after journaling")

// Post stores a message for receiver and returns its id.
func (s *Store) Post(sender Sender, receiver, body string) (int64, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{})
	return posted.ID, err
}

// Reply posts text as an answer to message id and returns the id of the reply.
func (s *Store) Reply(sender Sender, id int64, text string) (int64, error) {
	replied, err := s.ReplyWith(sender, id, text)
	return replied.ID, err
}

// PostWait stores a message for receiver and registers a wait for its reply, before any reply can arrive.
func (s *Store) PostWait(sender Sender, receiver, body string) (int64, *Waiter, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{Wait: true})
	return posted.ID, posted.Waiter, err
}

// PostInterrupt stores a message for receiver after stopping the receiver's running turn.
// With wait set it also registers a wait for the reply.
// It reports whether a running turn was stopped; a receiver with no running turn still gets the post.
func (s *Store) PostInterrupt(sender Sender, receiver, body string, wait bool) (int64, bool, *Waiter, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{Wait: wait, Interrupt: true})
	return posted.ID, posted.Interrupted, posted.Waiter, err
}

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
