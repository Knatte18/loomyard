//go:build !windows

// latewriter_unix.go classifies the removal failures a late writer causes on a task worktree directory off Windows.

package fabricengine

import (
	"errors"
	"syscall"
)

// isLateWriterError reports whether err is a directory that gained an entry while it was being deleted.
func isLateWriterError(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}
