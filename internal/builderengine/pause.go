// pause.go implements builder's pause-flag mechanics, mirroring
// perchengine's PauseFlagPath/clearPauseFlag discipline (internal/
// perchengine/state.go) against a builder dir instead of a perch run dir:
// RequestPause writes a flag file that spawn-batch's batch-boundary check
// refuses against, PauseRequested observes it, and ClearPause removes it.
// The clearing rules the discussion pins mirror perch's exactly: ClearPause
// must be called at run's own entry (never instantly re-pause on the flag
// that requested the pause a resumed run is now resuming from) and again at
// every terminal outcome (a pause requested while the last batch was still
// in flight can lose the race against the boundary check settling on its
// own — the flag must not linger in a finished run's builder dir).

package builderengine

import (
	"fmt"
	"os"
	"path/filepath"
)

// PauseFlagName is the pause flag file's name inside a builder dir.
// Exported so buildercli's pause verb can name the same file it writes
// without recomputing the join itself.
const PauseFlagName = "pause"

// PauseFlagPath returns the path to the pause flag file inside builderDir.
// buildercli's pause verb writes this file, and spawn-batch's batch-
// boundary check reads it via PauseRequested; both must resolve the same
// path, which is why this is exported rather than duplicated at each call
// site.
func PauseFlagPath(builderDir string) string {
	return filepath.Join(builderDir, PauseFlagName)
}

// RequestPause creates builderDir's pause flag file, creating builderDir
// itself first if it does not yet exist — a pause may be requested before
// any batch has ever spawned. Creating an already-present flag file is not
// an error: RequestPause is idempotent.
func RequestPause(builderDir string) error {
	if err := os.MkdirAll(builderDir, 0o755); err != nil {
		return fmt.Errorf("builder: create builder dir %s: %w", builderDir, err)
	}

	path := PauseFlagPath(builderDir)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("builder: create pause flag %s: %w", path, err)
	}
	return f.Close()
}

// PauseRequested reports whether builderDir's pause flag file is currently
// present.
func PauseRequested(builderDir string) bool {
	_, err := os.Stat(PauseFlagPath(builderDir))
	return err == nil
}

// ClearPause removes builderDir's pause flag file, doing nothing if it is
// already absent — clearing an already-clear flag is not an error. Callers
// MUST invoke this at run's own entry (so a resumed run never instantly
// re-pauses on the flag that requested the very pause it is now resuming
// from) and again at every terminal outcome (so a pause request that lost
// the race against the last batch settling on its own never lingers in a
// finished run's builder dir).
func ClearPause(builderDir string) error {
	path := PauseFlagPath(builderDir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("builder: remove pause flag %s: %w", path, err)
	}
	return nil
}
