package permkit_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChmod_SetsTheModeAndRestoresItAtCleanup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps only the owner write bit of a mode")
	}
	dir := filepath.Join(t.TempDir(), "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir(%s): %v", dir, err)
	}
	const before fs.FileMode = 0o750
	if err := os.Chmod(dir, before); err != nil {
		t.Fatalf("Chmod(%s): %v", dir, err)
	}

	// Not parallel: the outer test reads the mode once the subtest's cleanups have run.
	t.Run("Inside", func(t *testing.T) {
		permkit.Chmod(t, dir, 0o555)
		if got := permOf(t, dir); got != 0o555 {
			t.Errorf("mode inside the test = %v; want %v", got, fs.FileMode(0o555))
		}
	})
	if got := permOf(t, dir); got != before {
		t.Errorf("mode after the test = %v; want %v", got, before)
	}
}

// permOf returns path's permission bits.
func permOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	return info.Mode().Perm()
}
