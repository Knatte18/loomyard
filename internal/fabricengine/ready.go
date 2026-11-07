// ready.go implements Ready, the presence probe a consumer uses to ask whether fabric is usable in this worktree.
// The caller names no side of the repo; the location argument decides it.

package fabricengine

import (
	"errors"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// Ready reports whether fabric is usable in this worktree.
// It returns (false, nil) when fabric has not been set up here and (true, nil) when it has.
// It returns (false, err) when the check itself could not be completed, such as on a permissions fault.
func Ready(l *lymcwd.Location) (bool, error) {
	_, err := os.Stat(RecordsWorktree(l))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
