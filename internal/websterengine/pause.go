// pause.go implements webster's pause-flag mechanics: a webster-local copy of builderengine's
// pause.go (RequestPause/PauseRequested/ClearPause), since builder's own versions have an in-tree
// builder caller (frozen, per the Shared Decision builder-is-frozen-copy-not-move) and cannot be
// moved.
// RequestPause writes a flag file that the batch boundary refuses against, PauseRequested observes
// it, and ClearPause removes it.
// The clearing rules mirror builder's exactly: ClearPause must be called once a run has passed its
// refusal gates and is committed to spawning (never instantly re-pause on the flag that requested
// the pause a resumed run is now resuming from — while a run that refuses on validation or a
// fingerprint mismatch leaves a pending pause intact) and again at every terminal outcome (a pause
// requested while the last batch was still in flight must not linger in a finished run's webster
// dir).
// Exported because internal/webstercli calls these directly (pause.go/status.go/weft.go retarget in
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

// pauseFlagPath returns the path to the pause flag file inside websterDir.
func pauseFlagPath(websterDir string) string {
	return filepath.Join(websterDir, PauseFlagName)
}

// RequestPause creates websterDir's pause flag file, creating websterDir if needed.
// Creating an already-present flag is idempotent.
func RequestPause(websterDir string) error {
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		return fmt.Errorf("websterengine: create webster dir %s: %w", websterDir, err)
	}

	path := pauseFlagPath(websterDir)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("websterengine: create pause flag %s: %w", path, err)
	}
	return f.Close()
}

// PauseRequested reports whether websterDir's pause flag file is currently present.
func PauseRequested(websterDir string) bool {
	_, err := os.Stat(pauseFlagPath(websterDir))
	return err == nil
}

// ClearPause removes websterDir's pause flag file.
// Clearing an already-clear flag is not an error.
// Callers must invoke this once committed to spawning Master and again at terminal outcome (per
// double-clear pattern).
func ClearPause(websterDir string) error {
	path := pauseFlagPath(websterDir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("websterengine: remove pause flag %s: %w", path, err)
	}
	return nil
}
