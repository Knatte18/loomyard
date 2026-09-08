// logsdir.go implements LogsDir, the told-mode helper that constructs the standalone trace-log
// directory path from a session's derived state directory.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// LogsDir returns the standalone trace-log directory for stateDir: <stateDir>/.lyx/logs.
//
// It is the sole construction site for the standalone trace-log directory across every
// standalone-capable CLI, mirroring hub mode's <anchor>/.lyx/logs, with stateDir playing the
// anchor's role, exactly as StencilsDir mirrors the hub stencils directory.
//
// reed's own <stateDir>/logs — the LogsDir field ReedGeometry sets on the reedengine.Geometry it
// returns — is a different directory for a different producer and is deliberately not converged
// with this one.
//
// Like every other builder in this package, LogsDir takes only stateDir — it never calls
// standalonestate.Derive, never reads the environment, and never touches disk.
func LogsDir(stateDir string) string {
	return filepath.Join(stateDir, lyxdirs.DotLyxDirName, "logs")
}
