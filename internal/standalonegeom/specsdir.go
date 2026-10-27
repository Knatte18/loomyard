// specsdir.go implements SpecsDir, the told-mode helper that constructs the standalone
// deployed-specs directory path from a session's derived state directory.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// SpecsDir returns the standalone deployed-specs directory for stateDir: <stateDir>/_lym/specs.
//
// It is the sole construction site for the standalone deployed-specs directory across every
// standalone-capable CLI, deliberately mirroring fabricengine.SpecsDir(hub), with stateDir playing
// the role hub plays in hub mode.
//
// Like every other builder in this package, SpecsDir takes only stateDir — it never calls
// standalonestate.Derive, never reads the environment, and never touches disk.
//
// Unlike StencilsDir, this directory has no CLI flag that can override it, so this function's
// result is the only spelling standalone mode ever uses.
func SpecsDir(stateDir string) string {
	return filepath.Join(stateDir, lymdirs.LymDirName, "specs")
}
