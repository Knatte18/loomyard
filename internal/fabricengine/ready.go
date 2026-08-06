// ready.go implements Ready, the presence probe a consumer uses to ask whether fabric is usable in
// this worktree without naming which side of the repo it is asking about.

package fabricengine

import (
	"errors"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// Ready reports whether fabric is usable in this worktree.
// It returns (false, nil) when fabric has not been set up here, (true, nil) when it has, and
// (false, err) when the check itself could not be completed (e.g. a permissions fault).
func Ready(l *lyxcwd.Location) (bool, error) {
	_, err := os.Stat(WeftWorktree(l))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
