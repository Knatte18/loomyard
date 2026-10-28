//go:build tier0

// export_test.go exposes the store internals the external tests drive: a callback that journals and then fails.

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
		if id, err = s.appendPostedLocked(sender, receiver, kindMail, body, 0); err != nil {
			return err
		}
		return s.deliverMailLocked(id, receiver)
	})
	return id, err
}
