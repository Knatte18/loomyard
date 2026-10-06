// overview_test.go runs the format checker against loomyard's own PATTERN.

package pattern

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverview_RealFilePassesCheck(t *testing.T) {
	t.Parallel()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test's working directory")
		}
		dir = parent
	}

	findings := Check(os.DirFS(dir))
	for _, f := range findings {
		t.Errorf("%s %s (line %d): %s", f.Kind, f.Subject, f.Line, f.Message)
	}
}
