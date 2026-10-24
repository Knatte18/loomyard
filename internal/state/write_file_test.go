//go:build tier0

// write_file_test.go tests WriteFile's verbatim atomic write of raw record bytes using only temp files — no git, no spawning.

package state

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteFile pins that WriteFile lands the bytes verbatim, replaces an earlier write whole and creates the parent directory.
// On an unwritable path (the target is a directory) it returns an error and leaves no temp file beside it.
func TestWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("VerbatimReplaceAndParent", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nested", "deeper", "task.patch")
		lockPath := path + ".lock"

		first := []byte("diff --git a/x b/x\n\x00not json\n")
		if err := WriteFile(path, lockPath, first); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error: %v", err)
		}
		if !bytes.Equal(got, first) {
			t.Errorf("file = %q; want %q", got, first)
		}

		second := []byte("short")
		if err := WriteFile(path, lockPath, second); err != nil {
			t.Fatalf("second WriteFile() error: %v", err)
		}
		got, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() after overwrite error: %v", err)
		}
		if !bytes.Equal(got, second) {
			t.Errorf("file after overwrite = %q; want %q", got, second)
		}
	})

	t.Run("UnwritablePath", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "target")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("Mkdir() error: %v", err)
		}

		if err := WriteFile(path, path+".lock", []byte("data")); err == nil {
			t.Fatal("WriteFile() onto a directory error = nil; want an error")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Errorf("temp file %q left beside the target", entry.Name())
			}
		}
	})
}
