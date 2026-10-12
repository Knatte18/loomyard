// cleanup.go removes the handoff files an orch's cycles leave behind, keeping the one a restart resumes from.

package orchengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
)

// HandoffCleanup is what one handoff cleanup removed, or with apply false would remove.
type HandoffCleanup struct {
	// Kept is the name of the handoff file State.LastHandoff names, empty when cleanup decided to remove nothing.
	Kept string
	// Removed lists the names of the removed files, or in a dry run the removable ones.
	Removed []string
	// Reason says why nothing was removed, empty when cleanup went ahead.
	Reason string
}

// CleanupHandoffs removes every regular file directly under p.HandoffsDir except the one State.LastHandoff names, so a restart still resumes from it.
// With apply false it removes nothing and lists the files apply would remove.
// A newer partial handoff an aborted cycle left is removed with the rest.
// It removes nothing, saying why in the result, when the orch is not idle, no handoff is recorded or the recorded one is gone; a missing p.Dir returns at once.
// The state lock is held from the state read through the last removal, so a cycle cannot enter handoff-requested in between.
func CleanupHandoffs(p Paths, apply bool) (HandoffCleanup, error) {
	return cleanupHandoffs(p, apply, nil)
}

// cleanupHandoffs is CleanupHandoffs with beforeRemove, a seam the tests use to interleave a cycle; it runs after the decision and before the first removal.
func cleanupHandoffs(p Paths, apply bool, beforeRemove func()) (HandoffCleanup, error) {
	result := HandoffCleanup{Removed: []string{}}

	if _, err := os.Stat(p.Dir); errors.Is(err, fs.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("orch: stat %s: %w", p.Dir, err)
	}

	held, err := lock.AcquireWriteLock(p.StateLockPath)
	if err != nil {
		return result, fmt.Errorf("orch: lock the state for handoff cleanup: %w", err)
	}
	defer func() { _ = held.Release() }()

	current := State{Phase: PhaseIdle}
	data, err := os.ReadFile(p.StatePath)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &current); err != nil {
			return result, fmt.Errorf("orch: parse state for handoff cleanup: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return result, fmt.Errorf("orch: read state for handoff cleanup: %w", err)
	}

	if current.Phase != PhaseIdle {
		result.Reason = fmt.Sprintf("orch is in phase %s, so a handoff may be being written or read", current.Phase)
		return result, nil
	}
	if current.LastHandoff == "" {
		result.Reason = "no completed handoff is recorded"
		return result, nil
	}
	kept := filepath.Base(current.LastHandoff)
	if _, err := os.Stat(filepath.Join(p.HandoffsDir, kept)); errors.Is(err, fs.ErrNotExist) {
		result.Reason = fmt.Sprintf("the recorded handoff %s does not exist", kept)
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("orch: stat the recorded handoff: %w", err)
	}
	result.Kept = kept

	entries, err := os.ReadDir(p.HandoffsDir)
	if err != nil {
		return result, fmt.Errorf("orch: list %s: %w", p.HandoffsDir, err)
	}
	var removable []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && entry.Name() != kept {
			removable = append(removable, entry.Name())
		}
	}

	if !apply {
		result.Removed = append(result.Removed, removable...)
		return result, nil
	}
	if beforeRemove != nil {
		beforeRemove()
	}
	for _, name := range removable {
		if err := os.Remove(filepath.Join(p.HandoffsDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return result, fmt.Errorf("orch: remove handoff %s: %w", name, err)
		}
		result.Removed = append(result.Removed, name)
	}
	return result, nil
}
