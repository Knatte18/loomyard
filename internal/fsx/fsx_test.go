// fsx_test.go — unit tests for filesystem-safety primitives (fsx.go).
//
// PathGuard rejection, AtomicWrite and AtomicWriteBytes behavior.

package fsx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fsx"
)

func TestPathGuard(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty string", "", true},
		{"absolute path on unix", "/absolute/path", true},
		{"absolute path on windows", "C:\\absolute\\path", true},
		{"path with .. component", "foo/../bar", true},
		{"path with .. at start", "../foo", true},
		{"valid relative path", "valid/path.txt", false},
		{"valid relative single file", "file.txt", false},
		{"valid nested path", "a/b/c/d.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fsx.PathGuard(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("PathGuard(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("creates file with correct content", func(t *testing.T) {
		relPath := "file.txt"
		content := "test content"
		if err := fsx.AtomicWrite(tmpDir, relPath, content); err != nil {
			t.Fatalf("AtomicWrite failed: %v", err)
		}

		fullPath := filepath.Join(tmpDir, relPath)
		got, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", string(got), content)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		relPath := "deep/nested/path/file.txt"
		content := "nested content"
		if err := fsx.AtomicWrite(tmpDir, relPath, content); err != nil {
			t.Fatalf("AtomicWrite failed: %v", err)
		}

		fullPath := filepath.Join(tmpDir, relPath)
		got, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", string(got), content)
		}
	})

	t.Run("no temp file left on disk", func(t *testing.T) {
		relPath := "atomic.txt"
		if err := fsx.AtomicWrite(tmpDir, relPath, "content"); err != nil {
			t.Fatalf("AtomicWrite failed: %v", err)
		}

		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Errorf("found temp file: %s", entry.Name())
			}
		}
	})
}

func TestAtomicWriteBytes(t *testing.T) {
	t.Run("writes raw bytes to absolute path", func(t *testing.T) {
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "f.json")
		data := []byte(`{"key": "value"}`)

		if err := fsx.AtomicWriteBytes(absPath, data); err != nil {
			t.Fatalf("AtomicWriteBytes failed: %v", err)
		}

		got, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != string(data) {
			t.Errorf("content = %q, want %q", string(got), string(data))
		}
	})

	t.Run("creates missing parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "deep/nested/f.bin")
		data := []byte{0x01, 0x02, 0x03}

		if err := fsx.AtomicWriteBytes(absPath, data); err != nil {
			t.Fatalf("AtomicWriteBytes failed: %v", err)
		}

		got, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != string(data) {
			t.Errorf("content = %v, want %v", got, data)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "overwrite.txt")

		// Write initial content
		initialData := []byte("initial")
		if err := fsx.AtomicWriteBytes(absPath, initialData); err != nil {
			t.Fatalf("first AtomicWriteBytes failed: %v", err)
		}

		// Overwrite with new content
		newData := []byte("overwritten")
		if err := fsx.AtomicWriteBytes(absPath, newData); err != nil {
			t.Fatalf("second AtomicWriteBytes failed: %v", err)
		}

		got, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != string(newData) {
			t.Errorf("content = %q, want %q", string(got), string(newData))
		}
	})

	t.Run("leaves no temp file in target dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "target.txt")

		if err := fsx.AtomicWriteBytes(absPath, []byte("content")); err != nil {
			t.Fatalf("AtomicWriteBytes failed: %v", err)
		}

		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Errorf("found temp file: %s", entry.Name())
			}
		}
	})
}
