//go:build integration

// launchers_test.go covers launcher content generation, per-subpath menus, and
// teardown (Windows-gated; skipped where symlink/junction creation is unavailable).

package warpengine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWriteLaunchers covers launcher file creation on Windows.
// On non-Windows platforms, tests are skipped.
func TestWriteLaunchers(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("launcher tests only run on Windows")
	}

	tests := []struct {
		name           string
		slug           string
		relPath        string
		verifyIde      func(t *testing.T, content string)
		verifyMenu     func(t *testing.T, content string)
		verifyCheckout func(t *testing.T, content string)
	}{
		{
			name:    "EmptyRelPath",
			slug:    "test-slug",
			relPath: "",
			verifyIde: func(t *testing.T, content string) {
				expected := "@cd /d \"%~dp0..\\..\\test-slug\" && lyx ide spawn test-slug\r\n"
				if content != expected {
					t.Errorf("ide.cmd content = %q; want %q", content, expected)
				}
			},
			verifyMenu: func(t *testing.T, content string) {
				// Menu content should have the hub name but no relpath segment.
				if !strings.Contains(content, "lyx ide menu") {
					t.Errorf("ide-menu.cmd does not contain 'lyx ide menu': %q", content)
				}
			},
			verifyCheckout: func(t *testing.T, content string) {
				expected := "@cd /d \"%~dp0..\\..\\test-slug\" && lyx warp checkout\r\n"
				if content != expected {
					t.Errorf("warp-checkout.cmd content = %q; want %q", content, expected)
				}
			},
		},
		{
			name:    "NonEmptyRelPath",
			slug:    "task-b",
			relPath: "subdir/nested",
			verifyIde: func(t *testing.T, content string) {
				// Launcher dir is now at _launchers/<RelPath>/<slug>, so the climb is deeper:
				// From _launchers/subdir/nested/task-b to <Hub>/task-b/subdir/nested
				// = ..\..\..\..\task-b\subdir\nested (2 base + 2 relpath segments).
				expected := "@cd /d \"%~dp0..\\..\\..\\..\\task-b\\subdir\\nested\" && lyx ide spawn task-b\r\n"
				if content != expected {
					t.Errorf("ide.cmd content = %q; want %q", content, expected)
				}
			},
			verifyMenu: func(t *testing.T, content string) {
				if !strings.Contains(content, "subdir\\nested") {
					t.Errorf("ide-menu.cmd does not contain relpath segment: %q", content)
				}
				if !strings.Contains(content, "lyx ide menu") {
					t.Errorf("ide-menu.cmd does not contain 'lyx ide menu': %q", content)
				}
			},
			verifyCheckout: func(t *testing.T, content string) {
				// warp-checkout.cmd uses the same climb path as ide.cmd.
				expected := "@cd /d \"%~dp0..\\..\\..\\..\\task-b\\subdir\\nested\" && lyx warp checkout\r\n"
				if content != expected {
					t.Errorf("warp-checkout.cmd content = %q; want %q", content, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := lyxtest.CopyHostHub(t)

			// Modify the hub to have the desired RelPath by creating subdirectories
			// and resolving from the deepest one.
			var cwd string
			if tt.relPath != "" && tt.relPath != "." {
				testDir := filepath.Join(f.Hub, tt.relPath)
				if err := os.MkdirAll(testDir, 0o755); err != nil {
					t.Fatalf("mkdir test relpath: %v", err)
				}
				cwd = testDir
			} else {
				cwd = f.Hub
			}

			l, err := hubgeometry.Resolve(cwd)
			if err != nil {
				t.Fatalf("hubgeometry.Resolve(%q): %v", cwd, err)
			}

			// Write launchers.
			if err := writeLaunchers(l, tt.slug); err != nil {
				t.Fatalf("writeLaunchers: %v", err)
			}

			// Verify ide.cmd was created with correct content.
			// Read via l.LauncherDir(slug) for mirrored location.
			ideCmdPath := filepath.Join(l.LauncherDir(tt.slug), "ide.cmd")
			ideCmdContent, err := os.ReadFile(ideCmdPath)
			if err != nil {
				t.Fatalf("read ide.cmd: %v", err)
			}
			tt.verifyIde(t, string(ideCmdContent))

			// Verify warp-checkout.cmd was created with correct content.
			warpCheckoutPath := filepath.Join(l.LauncherDir(tt.slug), "warp-checkout.cmd")
			warpCheckoutContent, err := os.ReadFile(warpCheckoutPath)
			if err != nil {
				t.Fatalf("read warp-checkout.cmd: %v", err)
			}
			tt.verifyCheckout(t, string(warpCheckoutContent))

			// Verify per-subpath menu was created at l.MenuLauncherPath().
			menuCmdPath := l.MenuLauncherPath()
			menuCmdContent, err := os.ReadFile(menuCmdPath)
			if err != nil {
				t.Fatalf("read ide-menu.cmd: %v", err)
			}
			tt.verifyMenu(t, string(menuCmdContent))

			// Call writeLaunchers again with a different slug to verify menu is not clobbered per-subpath.
			originalMenuContent := string(menuCmdContent)
			if err := writeLaunchers(l, "another-slug"); err != nil {
				t.Fatalf("writeLaunchers again: %v", err)
			}

			menuCmdContent2, err := os.ReadFile(menuCmdPath)
			if err != nil {
				t.Fatalf("read ide-menu.cmd again: %v", err)
			}
			if string(menuCmdContent2) != originalMenuContent {
				t.Errorf("ide-menu.cmd was modified; want unchanged")
			}
		})
	}
}

// TestRemoveLaunchers covers launcher directory removal and per-subpath menu survival.
func TestRemoveLaunchers(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("launcher tests only run on Windows")
	}

	f := lyxtest.CopyHostHub(t)
	l, err := hubgeometry.Resolve(f.Hub)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
	}

	// Write launchers for two slugs.
	if err := writeLaunchers(l, "slug1"); err != nil {
		t.Fatalf("writeLaunchers slug1: %v", err)
	}
	if err := writeLaunchers(l, "slug2"); err != nil {
		t.Fatalf("writeLaunchers slug2: %v", err)
	}

	// Verify both launcher dirs exist (via l.LauncherDir) and warp-checkout.cmd
	// is present inside each (removeLaunchers must clean it via RemoveAll).
	slug1Dir := l.LauncherDir("slug1")
	slug2Dir := l.LauncherDir("slug2")
	if _, err := os.Stat(slug1Dir); err != nil {
		t.Fatalf("slug1 launcher dir does not exist: %v", err)
	}
	if _, err := os.Stat(slug2Dir); err != nil {
		t.Fatalf("slug2 launcher dir does not exist: %v", err)
	}

	warpCheckoutSlug1 := filepath.Join(slug1Dir, "warp-checkout.cmd")
	if _, err := os.Stat(warpCheckoutSlug1); err != nil {
		t.Fatalf("warp-checkout.cmd not found in slug1 dir: %v", err)
	}

	// Remove slug1 launchers.
	if err := removeLaunchers(l, "slug1"); err != nil {
		t.Fatalf("removeLaunchers slug1: %v", err)
	}

	// Verify slug1 dir (including warp-checkout.cmd) is gone but slug2 remains.
	if _, err := os.Stat(slug1Dir); err == nil {
		t.Error("slug1 launcher dir still exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat slug1 dir: %v", err)
	}

	if _, err := os.Stat(slug2Dir); err != nil {
		t.Fatalf("slug2 launcher dir was removed: %v", err)
	}

	// Verify per-subpath ide-menu.cmd is still there (via l.MenuLauncherPath()).
	menuCmdPath := l.MenuLauncherPath()
	if _, err := os.Stat(menuCmdPath); err != nil {
		t.Fatalf("per-subpath menu was removed: %v", err)
	}

	// Second removeLaunchers call should be idempotent.
	if err := removeLaunchers(l, "slug1"); err != nil {
		t.Fatalf("second removeLaunchers slug1: %v", err)
	}
}
