// switch.go holds the machine's connection switch and the reading of whether the machine is connected to Slack.

package slackengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// switchRecord is the switch file's content.
type switchRecord struct {
	Off       bool      `json:"off"`
	ChangedAt time.Time `json:"changed_at"`
}

// ReadSwitchedOff reports whether the operator switched the machine's Slack connection off.
// A missing file reads as on.
// While it is off, no start of a relay spawns one, so the prime dobby's tick does not bring a stopped relay back.
func ReadSwitchedOff(path string) (bool, error) {
	record, _, err := state.ReadJSON[switchRecord](path, recordLockPath(path))
	if err != nil {
		return false, fmt.Errorf("slack: read the connection switch %s: %w", path, err)
	}
	return record.Off, nil
}

// WriteSwitchedOff records whether the machine's Slack connection is switched off, with now as its change time.
func WriteSwitchedOff(path string, off bool, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return fmt.Errorf("slack: create the directory of the connection switch %s: %w", path, err)
	}
	err := state.UpdateJSON(path, recordLockPath(path), func(current switchRecord, found bool) (switchRecord, error) {
		if found && current.Off == off {
			return current, nil
		}
		return switchRecord{Off: off, ChangedAt: now}, nil
	})
	if err != nil {
		return fmt.Errorf("slack: write the connection switch %s: %w", path, err)
	}
	return nil
}

// Connected reports whether a relay holds the machine's Socket Mode connection: the machine's socket lock is held.
// The holder keeps the lock while it dials again after a dropped connection, so a reconnect reads as connected.
// A free lock is released again at once.
func Connected(paths MachinePaths) (bool, error) {
	if _, err := os.Stat(filepath.Dir(paths.SocketLockPath)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(paths.SocketLockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}
