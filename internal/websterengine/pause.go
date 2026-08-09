// pause.go implements webster's own pause-flag mechanics (RequestPause/PauseRequested/ClearPause),
// deliberately module-local rather than shared.
// RequestPause writes a flag file that the batch boundary refuses against, PauseRequested observes
// it, and ClearPause removes it.
// The clearing rules: ClearPause must be called once a run has passed its
// refusal gates and is committed to spawning (never instantly re-pause on the flag that requested
// the pause a resumed run is now resuming from — while a run that refuses on validation or a
// fingerprint mismatch leaves a pending pause intact) and again at every terminal outcome (a pause
// requested while the last batch was still in flight must not linger in a finished run's webster
// dir).
// Exported because internal/webstercli calls these directly (pause.go/status.go/sync.go retarget in
// batch 9).

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
)

// PauseFlagName is the pause flag file's name inside a webster dir.
// Exported so webstercli's pause verb can name the same file it writes without recomputing the join
// itself.
const PauseFlagName = "pause"

// pauseFlagPath returns the path to the pause flag file inside scratchDir.
func pauseFlagPath(scratchDir string) string {
	return filepath.Join(scratchDir, PauseFlagName)
}

// RequestPause creates scratchDir's pause flag file, creating scratchDir if needed.
// Creating an already-present flag is idempotent.
func RequestPause(scratchDir string) error {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: create webster scratch dir %s: %w", scratchDir, err)
	}

	path := pauseFlagPath(scratchDir)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("websterengine: create pause flag %s: %w", path, err)
	}
	return f.Close()
}

// PauseRequested reports whether scratchDir's pause flag file is currently present.
func PauseRequested(scratchDir string) bool {
	_, err := os.Stat(pauseFlagPath(scratchDir))
	return err == nil
}

// ClearPause removes scratchDir's pause flag file.
// Clearing an already-clear flag is not an error.
// Callers must invoke this once committed to spawning Master and again at terminal outcome (per
// double-clear pattern).
func ClearPause(scratchDir string) error {
	path := pauseFlagPath(scratchDir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("websterengine: remove pause flag %s: %w", path, err)
	}
	return nil
}
