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
