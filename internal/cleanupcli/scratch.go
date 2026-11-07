// scratch.go implements the scratch segment of the cleanup verb:
// the removal of old, uncited children of the .scratch directory at the prime's anchor.

package cleanupcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// scratchDirName is the name of the scratch directory at the prime worktree's anchor, which no other module owns.
const scratchDirName = ".scratch"

// scratchMaxAge is how long a scratch child must go unmodified before the segment removes it.
const scratchMaxAge = 14 * 24 * time.Hour

// boardDoneStatus is the status of a board entry whose work is finished;
// such an entry does not keep a scratch child alive.
const boardDoneStatus = "done"

// scratchSegment removes the direct children of dir that nothing still needs and returns their names.
// With apply false, it only lists them.
// A child qualifies when neither it nor anything below it was modified within the last scratchMaxAge,
// and no board entry that is not done cites its ".scratch/<name>" path in its title, brief or body.
// A link is judged on its own modification time and removed as a link, never followed.
// A missing dir is an empty segment.
// The board is read first; a read error fails the segment with nothing removed.
func scratchSegment(dir string, readBoard func() ([]boardengine.Task, error), now time.Time, apply bool) ([]string, error) {
	tasks, err := readBoard()
	if err != nil {
		return nil, fmt.Errorf("cleanup: read the board for the scratch segment: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cleanup: list %s: %w", dir, err)
	}

	cutoff := now.Add(-scratchMaxAge)
	removed := []string{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		newest, err := fsx.NewestModTime(path)
		if err != nil {
			return removed, fmt.Errorf("cleanup: read the age of %s: %w", path, err)
		}
		if !newest.Before(cutoff) || citedByOpenEntry(tasks, entry.Name()) {
			continue
		}
		if apply {
			remove := os.Remove
			if entry.IsDir() {
				remove = os.RemoveAll
			}
			if err := remove(path); err != nil {
				return removed, fmt.Errorf("cleanup: remove %s: %w", path, err)
			}
		}
		removed = append(removed, entry.Name())
	}
	return removed, nil
}

// citedByOpenEntry reports whether a board entry that is not done names the scratch child called name by its ".scratch/<name>" path.
func citedByOpenEntry(tasks []boardengine.Task, name string) bool {
	cited := scratchDirName + "/" + name
	for _, task := range tasks {
		if task.Status != nil && *task.Status == boardDoneStatus {
			continue
		}
		if strings.Contains(task.Title, cited) || strings.Contains(task.Brief, cited) || strings.Contains(task.Body, cited) {
			return true
		}
	}
	return false
}
