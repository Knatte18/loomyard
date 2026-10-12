// root_test.go — tests for how the temp root is located, created and checked (root.go).

package lyxtemp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fslink"
)

func fixedClock() time.Time {
	return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
}

// entryNames lists the names directly inside dir.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestOpenRoot(t *testing.T) {
	t.Parallel()
	uid := os.Getuid()

	t.Run("creates a private root", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), rootName(uid))
		root, err := openRoot(path, uid, fixedClock)
		if err != nil {
			t.Fatalf("openRoot: %v", err)
		}
		if root.Path() != path {
			t.Errorf("Path() = %q, want %q", root.Path(), path)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("root not created as a directory: %v", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Errorf("mode = %04o, want 0700", info.Mode().Perm())
		}
	})

	t.Run("reopens an existing root", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), rootName(uid))
		for range 2 {
			if _, err := openRoot(path, uid, fixedClock); err != nil {
				t.Fatalf("openRoot: %v", err)
			}
		}
	})

	t.Run("refuses a root that is a link", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(base, rootName(uid))
		if err := fslink.CreateDirLink(path, target); err != nil {
			t.Fatalf("create link: %v", err)
		}
		_, err := openRoot(path, uid, fixedClock)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want one naming %q", err, path)
		}
		if names := entryNames(t, target); len(names) != 0 {
			t.Errorf("link target holds %v, want nothing created", names)
		}
	})

	t.Run("refuses a missing parent", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "absent", rootName(uid))
		_, err := openRoot(path, uid, fixedClock)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want one naming %q", err, path)
		}
		if _, statErr := os.Lstat(filepath.Dir(path)); statErr == nil {
			t.Error("a missing parent was created")
		}
	})

	if runtime.GOOS == "windows" {
		return
	}

	t.Run("refuses a mode other than 0700", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), rootName(uid))
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := openRoot(path, uid, fixedClock)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want one naming %q", err, path)
		}
		if names := entryNames(t, path); len(names) != 0 {
			t.Errorf("root holds %v, want nothing created", names)
		}
	})

	t.Run("refuses another owner", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), rootName(uid))
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := openRoot(path, uid+1, fixedClock)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want one naming %q", err, path)
		}
		if names := entryNames(t, path); len(names) != 0 {
			t.Errorf("root holds %v, want nothing created", names)
		}
	})
}

func TestResolveRootPath(t *testing.T) {
	t.Parallel()
	uid := os.Getuid()
	name := rootName(uid)
	base := filepath.Join(string(filepath.Separator), "var", "tmp")
	root := filepath.Join(base, name)

	rows := []struct {
		name    string
		tempDir string
		want    string
	}{
		{"plain temp directory", base, root},
		{"inherited run directory under a root", filepath.Join(root, runPrefix+"123456"), root},
		{"run-prefixed directory whose parent is not a root", filepath.Join(base, "elsewhere", runPrefix+"123456"), filepath.Join(base, "elsewhere", runPrefix+"123456", name)},
		{"directory under a root without the run prefix", filepath.Join(root, "x123456"), filepath.Join(root, "x123456", name)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveRootPath(row.tempDir, uid); got != row.want {
				t.Errorf("resolveRootPath(%q) = %q, want %q", row.tempDir, got, row.want)
			}
		})
	}
}
