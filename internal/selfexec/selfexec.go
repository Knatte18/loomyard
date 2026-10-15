// selfexec.go holds the Watcher that detects a replaced binary and re-execs the process onto it.

package selfexec

import (
	"errors"
	"os"

	"github.com/Knatte18/loomyard/internal/logger"
)

// identity is the file identity of the binary at a path.
type identity struct {
	device       uint64
	inode        uint64
	size         int64
	modTimeNanos int64
}

// errNoFileIdentity reports a platform that gives a file no comparable identity.
var errNoFileIdentity = errors.New("selfexec: no file identity on this platform")

// errUnderTest is what execSelf returns instead of exec'ing inside a test binary.
var errUnderTest = errors.New("selfexec: exec skipped under go test")

// Watcher records the binary a process started from and re-execs the process when that binary changes.
// It is not safe for concurrent use; its callers call Check from one poll loop.
type Watcher struct {
	path string
	stat func(string) (identity, error)
	exec func(path string, argv, env []string) error

	// hasStart is false when the start identity could not be recorded; Check then never execs.
	hasStart bool
	start    identity

	// previous is the changed identity the last check saw, valid while hasPrevious.
	hasPrevious bool
	previous    identity

	// failed is the identity whose exec failed, valid while hasFailed.
	hasFailed bool
	failed    identity
}

// New returns a Watcher for the running executable, resolved once now.
// A failed resolve or stat logs at Debug and yields a Watcher whose Check never execs.
func New() *Watcher {
	path, err := os.Executable()
	if err != nil {
		logger.Debug("selfexec: could not resolve the executable; it will not re-exec on a replaced binary", "err", err)
		return &Watcher{}
	}
	return newWatcher(path, statIdentity, execSelf)
}

func newWatcher(path string, stat func(string) (identity, error), exec func(path string, argv, env []string) error) *Watcher {
	w := &Watcher{path: path, stat: stat, exec: exec}
	start, err := stat(path)
	if err != nil {
		logger.Debug("selfexec: could not stat the executable at startup; it will not re-exec on a replaced binary", "path", path, "err", err)
		return w
	}
	w.start = start
	w.hasStart = true
	return w
}

// Check re-stats the recorded path and replaces the process with it when the binary there differs from the one the process started from on two consecutive checks.
// A process whose exec succeeds does not return.
func (w *Watcher) Check() {
	current, changed := w.changedTwice()
	if !changed || (w.hasFailed && w.failed == current) {
		return
	}
	logger.Info("selfexec: the binary changed on disk; the process replaces itself", "path", w.path)
	if err := w.exec(w.path, os.Args, os.Environ()); err != nil {
		logger.Warn("selfexec: could not re-exec onto the changed binary; the process keeps running", "path", w.path, "err", err)
		w.failed = current
		w.hasFailed = true
	}
}

// Replaced re-stats the recorded path and reports whether the binary there differs from the one the process started from on two consecutive calls.
// It never execs, so a process that exits on a replaced binary instead of re-execing uses it in place of Check.
func (w *Watcher) Replaced() bool {
	if _, changed := w.changedTwice(); !changed {
		return false
	}
	logger.Info("selfexec: the binary changed on disk", "path", w.path)
	return true
}

// changedTwice re-stats the recorded path, updates the previous-identity state and reports the current identity with whether it differs from the start identity on this call and the one before.
func (w *Watcher) changedTwice() (identity, bool) {
	if !w.hasStart {
		return identity{}, false
	}
	current, err := w.stat(w.path)
	if err != nil {
		logger.Debug("selfexec: could not stat the executable; treating it as unchanged", "path", w.path, "err", err)
		w.hasPrevious = false
		return identity{}, false
	}
	if current == w.start {
		w.hasPrevious = false
		return current, false
	}
	seenTwice := w.hasPrevious && w.previous == current
	w.previous = current
	w.hasPrevious = true
	return current, seenTwice
}
