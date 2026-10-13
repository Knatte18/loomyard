//go:build windows

// latewriter_windows.go classifies the removal failures a late writer causes on a task worktree directory on Windows.

package fabricengine

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isLateWriterError reports whether err is a directory that gained an entry while it was being deleted, or a file under it that its writer still holds open.
// Go opens Windows files without FILE_SHARE_DELETE, so a file a writer such as internal/lock holds open cannot be deleted until the writer closes it.
func isLateWriterError(err error) bool {
	return errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
