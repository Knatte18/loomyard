// connection.go reads whether the machine is connected to Slack.

package slackengine

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
)

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
