// specsdir.go implements SpecsDir, the told-mode helper for the standalone deployed-specs path.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// SpecsDir returns the standalone deployed-specs directory for stateDir: <stateDir>/_lym/specs.
//
// It is the one place every standalone-capable CLI builds that directory.
//
// SpecsDir takes only stateDir: it never reads the environment and never touches disk.
// No CLI flag overrides this directory, so this result is the only spelling standalone mode uses.
func SpecsDir(stateDir string) string {
	return filepath.Join(stateDir, lymdirs.LymDirName, "specs")
}
