//go:build tier0

// logfile_test.go pins the per-line append and the size-bound rotation of the dobby's log and output files over temp files.

package dobbyengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

// readFile returns the content of path, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v; want nil", path, err)
	}
	return string(data)
}

func TestAppendLog(t *testing.T) {
	t.Parallel()
	full := strings.Repeat("x", logRotateBytes)

	t.Run("an append below the bound adds the line", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), LogFileName)
		writeFile(t, path, "first\n")
		if err := AppendLog(path, "second", true); err != nil {
			t.Fatalf("AppendLog() = %v; want nil", err)
		}
		if got := readFile(t, path); got != "first\nsecond\n" {
			t.Errorf("log = %q; want both lines", got)
		}
	})

	t.Run("a rotating append past the bound moves the old content to .1, replacing an older one", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), LogFileName)
		writeFile(t, path+".1", "older\n")
		writeFile(t, path, full)
		if err := AppendLog(path, "new", true); err != nil {
			t.Fatalf("AppendLog() = %v; want nil", err)
		}
		if got := readFile(t, path); got != "new\n" {
			t.Errorf("log = %q; want only the new line", got)
		}
		if got := readFile(t, path+".1"); got != full {
			t.Errorf("previous log holds %d bytes; want the rotated %d", len(got), len(full))
		}
	})

	t.Run("a non-rotating append past the bound grows the file and writes no .1", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), LogFileName)
		writeFile(t, path, full)
		if err := AppendLog(path, "new", false); err != nil {
			t.Fatalf("AppendLog() = %v; want nil", err)
		}
		if got := readFile(t, path); got != full+"new\n" {
			t.Errorf("log holds %d bytes; want the old content and the new line", len(got))
		}
		if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
			t.Errorf("stat of the previous log = %v; want it absent", err)
		}
	})

	t.Run("an append into a missing directory fails and creates no directory", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "gone")
		if err := AppendLog(filepath.Join(dir, LogFileName), "line", true); err == nil {
			t.Error("AppendLog() = nil; want the open error")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("stat of the directory = %v; want it still absent", err)
		}
	})
}

func TestRotateOutBeforeSpawn(t *testing.T) {
	t.Parallel()
	full := strings.Repeat("x", logRotateBytes)

	tests := []struct {
		name        string
		goos        string
		content     string
		holdLock    bool
		wantRotated bool
	}{
		{"WindowsFreeLockPastBoundRotates", "windows", full, false, true},
		{"WindowsHeldLockLeavesIt", "windows", full, true, false},
		{"LinuxFreeLockLeavesIt", "linux", full, false, false},
		{"WindowsFreeLockUnderBoundLeavesIt", "windows", "small\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			geom := Geometry{LockPath: filepath.Join(dir, LockFileName), OutPath: filepath.Join(dir, OutFileName)}
			writeFile(t, geom.OutPath, tt.content)
			if tt.holdLock {
				held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
				if err != nil || !acquired {
					t.Fatalf("TryAcquireWriteLock() = %v, %v; want the lock", acquired, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			rotateOutBeforeSpawn(geom, tt.goos)

			_, err := os.Stat(geom.OutPath + ".1")
			if rotated := err == nil; rotated != tt.wantRotated {
				t.Errorf("rotated = %v; want %v", rotated, tt.wantRotated)
			}
		})
	}
}
