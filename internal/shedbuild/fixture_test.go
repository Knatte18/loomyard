// fixture_test.go holds writeStencilFile, the one helper shedbuild's tests keep to themselves.
// Tests that need the full Env take it from envkit.FullEnv.

package shedbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// writeStencilFile writes content to the on-disk location stencilstore.Path(dir, name) resolves name to.
// It creates the parent directory first:
// stencilstore.RelPath puts a stencil name's family, the text before its first hyphen, in a subdirectory.
func writeStencilFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := stencilstore.Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir stencil parent %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", path, err)
	}
}
