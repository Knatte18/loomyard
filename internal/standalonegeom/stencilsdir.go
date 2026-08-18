// stencilsdir.go implements StencilsDir, the told-mode helper that constructs the standalone
// stencils directory path from a session's derived state directory.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// StencilsDir returns the standalone stencils directory for stateDir: <stateDir>/_lyx/stencils.
//
// It is the sole construction site for the standalone stencils directory across every
// standalone-capable CLI, deliberately mirroring fabricengine.StencilsDir(hub), with stateDir
// playing the role hub plays in hub mode — which is why the result is _lyx-resident rather than a
// third top-level <state>/stencils.
//
// The returned value is carried by callers as a plain string, never as an engine geometry field:
// neither burlerengine.Geometry nor perchengine.Geometry has a StencilsDir field, and both engines
// already accept the directory as a told parameter.
//
// Like every other builder in this package, StencilsDir takes only stateDir — it never calls
// standalonestate.Derive, never reads the environment, and never touches disk.
func StencilsDir(stateDir string) string {
	return filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils")
}
