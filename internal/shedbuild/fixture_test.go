// fixture_test.go holds the one test helper shedbuild's tests keep to themselves:
// writeStencilFile.
// The full Env every test builds from comes from envkit.FullEnv.

package shedbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// writeStencilFile writes content to the on-disk location stencilstore.Path(dir, name) resolves
// name to, creating any parent directory the family convention requires -- stencilstore.RelPath
// splits a hyphenated stencil name on its first hyphen into a family subdirectory, so writing
// straight into a fresh temp directory would otherwise fail with a missing-parent error.
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
