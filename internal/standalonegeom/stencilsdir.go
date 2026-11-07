// stencilsdir.go implements StencilsDir, the told-mode helper that constructs the standalone
// stencils directory path from a session's derived state directory.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// StencilsDir returns the standalone stencils directory for stateDir: <stateDir>/_lym/stencils.
//
// It is the one place every standalone-capable CLI builds the stencils directory.
// The directory lives under _lym, so it stays inside the standalone state tree.
//
// Callers carry the result as a plain string, not as an engine geometry field.
// Engines accept it as a told parameter.
//
// StencilsDir takes only stateDir: it never reads the environment and never touches disk.
func StencilsDir(stateDir string) string {
	return filepath.Join(stateDir, lymdirs.LymDirName, "stencils")
}
