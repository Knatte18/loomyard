// newest_test.go — unit tests for NewestModTime (newest.go).

package fsx_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
)

func TestNewestModTime(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	writeFile := func(t *testing.T, path string, modTime time.Time) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}
	setTime := func(t *testing.T, path string, modTime time.Time) {
		t.Helper()
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}

	t.Run("file answers its own time", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "f.txt")
		writeFile(t, file, base)

		got, err := fsx.NewestModTime(file)
		if err != nil {
			t.Fatalf("NewestModTime: %v", err)
		}
		if !got.Equal(base) {
			t.Errorf("got %v, want %v", got, base)
		}
	})

	t.Run("empty directory answers its own time", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		setTime(t, dir, base)

		got, err := fsx.NewestModTime(dir)
		if err != nil {
			t.Fatalf("NewestModTime: %v", err)
		}
		if !got.Equal(base) {
			t.Errorf("got %v, want %v", got, base)
		}
	})

	t.Run("deep file newer than its directories wins", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		deepFile := filepath.Join(root, "a", "b", "deep.txt")
		deepTime := base.Add(48 * time.Hour)
		writeFile(t, deepFile, deepTime)
		for _, dir := range []string{filepath.Join(root, "a", "b"), filepath.Join(root, "a"), root} {
			setTime(t, dir, base)
		}

		got, err := fsx.NewestModTime(root)
		if err != nil {
			t.Fatalf("NewestModTime: %v", err)
		}
		if !got.Equal(deepTime) {
			t.Errorf("got %v, want %v", got, deepTime)
		}
	})

	t.Run("link answers its own time, never its target's", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		target := filepath.Join(root, "target")
		link := filepath.Join(root, "link")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		linkInfo, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		writeFile(t, filepath.Join(target, "ahead.txt"), linkInfo.ModTime().Add(time.Hour))

		got, err := fsx.NewestModTime(link)
		if err != nil {
			t.Fatalf("NewestModTime: %v", err)
		}
		if !got.Equal(linkInfo.ModTime()) {
			t.Errorf("got %v, want the link's own %v", got, linkInfo.ModTime())
		}
	})

	t.Run("missing path reports not-exist", func(t *testing.T) {
		t.Parallel()

		_, err := fsx.NewestModTime(filepath.Join(t.TempDir(), "absent"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v, want fs.ErrNotExist", err)
		}
	})
}
