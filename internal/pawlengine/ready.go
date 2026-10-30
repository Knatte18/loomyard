// ready.go holds a beam's ready record, the flag that says the beam is ready to be merged into its parent, with its write and read.

package pawlengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// ReadyRecord flags a beam ready to be merged into its parent.
// It names the beam it was written for, since a pair forked from the beam inherits the beam's records.
type ReadyRecord struct {
	// Slug is the beam the record flags.
	Slug string `json:"slug"`
	// SetAt is when the flag was set.
	SetAt time.Time `json:"set_at"`
}

// WriteReady writes record to the ready record at path under the lock at lockPath, creating both directories and replacing a record already there.
func WriteReady(path, lockPath string, record ReadyRecord) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("pawlengine: create the ready record's lock directory: %w", err)
	}
	if err := state.WriteJSON(path, lockPath, record); err != nil {
		return fmt.Errorf("pawlengine: write the ready record: %w", err)
	}
	return nil
}

// ReadyFor reports whether the ready record at path flags the beam slug.
// An absent record, and one naming another slug, read as not flagged.
func ReadyFor(path, lockPath, slug string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return false, fmt.Errorf("pawlengine: create the ready record's lock directory: %w", err)
	}
	record, found, err := state.ReadJSON[ReadyRecord](path, lockPath)
	if err != nil {
		return false, fmt.Errorf("pawlengine: read the ready record: %w", err)
	}
	return found && record.Slug == slug, nil
}
