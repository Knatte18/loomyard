// export_test.go exposes the wait arithmetic and the ready record's removal to the external tests.

package pawlengine

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/lock"
)

// NextAwaitWait exposes nextAwaitWait.
var NextAwaitWait = nextAwaitWait

// RemoveReady removes the ready record at path under the lock at lockPath when it flags the beam slug.
// It reports whether it removed one.
// An absent record, and one naming another slug, are left as they are.
func RemoveReady(path, lockPath, slug string) (bool, error) {
	flagged, err := ReadyFor(path, lockPath, slug)
	if err != nil || !flagged {
		return false, err
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return false, fmt.Errorf("pawlengine: lock the ready record: %w", err)
	}
	defer held.Release()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("pawlengine: remove the ready record: %w", err)
	}
	return true, nil
}
