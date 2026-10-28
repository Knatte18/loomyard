//go:build tier0 && !windows

// config_unix_test.go holds the config loader tests that rest on file mode bits, which Windows does not enforce.

package configengine_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// TestLoadOrTemplate_LymDirStatFailure_DoesNotFallback tests absence-only discrimination: an _lym/ that exists but cannot be stat'd makes LoadOrTemplate return an error rather than falling back to the template.
//
//testtiming:keep pins that an _lym/ that cannot be stat'd errors instead of resolving the template, which no covering test reaches
func TestLoadOrTemplate_LymDirStatFailure_DoesNotFallback(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory mode bits")
	}

	tmpDir, _ := newBase(t, baseLym, "board", "")
	t.Cleanup(func() {
		if err := os.Chmod(tmpDir, 0755); err != nil {
			t.Errorf("failed to restore tmpDir mode: %v", err)
		}
	})

	if err := os.Chmod(tmpDir, 0o000); err != nil {
		t.Fatalf("failed to chmod tmpDir: %v", err)
	}

	resolved, err := configengine.LoadOrTemplate(configengine.Dirs{Anchor: tmpDir}, boardSpec("path: _board\n", nil))
	if err == nil {
		t.Fatalf("expected error for unstattable _lym/, got nil")
	}
	if errors.Is(err, configengine.ErrNotInitialized) {
		t.Errorf("expected a stat failure to NOT satisfy errors.Is(err, ErrNotInitialized), got: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil resolved bytes, got: %v", resolved)
	}
}
