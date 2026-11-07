// logsdir.go implements LogsDir, the told-mode helper for the standalone trace-log directory path.
// It builds the path from a session's derived state directory.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// LogsDir returns the standalone trace-log directory for stateDir: <stateDir>/.lym/logs.
//
// It is the one construction site for the standalone trace-log directory across every standalone-capable CLI.
// It mirrors hub mode's <anchor>/.lym/logs, with stateDir playing the anchor's role.
//
// reed's own <stateDir>/logs is a different directory for a different producer, and it is not merged with this one.
//
// Like the other builders in this package, LogsDir takes only stateDir:
// it derives nothing, reads no environment, and touches no disk.
func LogsDir(stateDir string) string {
	return filepath.Join(stateDir, lymdirs.DotLymDirName, "logs")
}
