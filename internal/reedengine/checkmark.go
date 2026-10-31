// checkmark.go sets the session check mark that line 2 of the status bar shows: a check's text, state and the start of its step in flight, as session user options.

package reedengine

import (
	"fmt"
	"strconv"
	"time"
)

// SetCheckMark sets this session's check mark to text in state, its step in flight started at start, or clears the mark when text is empty.
// A zero start clears the start alone, so the bar shows no seconds.
// The mark is display only: nothing is persisted and it dies with the session.
// A failure, a session that is not up among them, is returned for the caller to log.
func (e *Engine) SetCheckMark(state, text string, start time.Time) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		target := exactSessionWindowTarget(e.SessionName())

		if text == "" {
			for _, option := range []string{checkMarkOption, checkMarkStateOption, checkMarkStartOption} {
				if err := e.tmux.run("set-option", "-u", "-t", target, option); err != nil {
					return fmt.Errorf("clear check mark: %w", err)
				}
			}
			return nil
		}
		argvs := [][]string{
			{"set-option", "-t", target, checkMarkOption, text},
			{"set-option", "-t", target, checkMarkStateOption, state},
			{"set-option", "-u", "-t", target, checkMarkStartOption},
		}
		if !start.IsZero() {
			argvs[2] = []string{"set-option", "-t", target, checkMarkStartOption, strconv.FormatInt(start.Unix(), 10)}
		}
		for _, argv := range argvs {
			if err := e.tmux.run(argv...); err != nil {
				return fmt.Errorf("set check mark: %w", err)
			}
		}
		return nil
	})
}
