package worktree_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestWriteLaunchers covers launcher file creation on Windows.
// On non-Windows platforms, tests are skipped.
func TestWriteLaunchers(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("launcher tests only run on Windows")
	}

	tests := []struct {
		name       string
		slug       string
		relPath    string
		verifyIde  func(t *testing.T, content string)
		verifyMenu func(t *testing.T, content string)
	}{
		{
			name:    "EmptyRelPath",
			slug:    "test-slug",
			relPath: "",
			verifyIde: func(t *testing.T, content string) {
				expected := "@cd /d \"%~dp0..\\..\\test-slug\" && mhgo ide spawn test-slug\r\n"
				if content != expected {
					t.Errorf("ide.cmd content = %q; want %q", content, expected)
				}
			},
			verifyMenu: func(t *testing.T, content string) {
				// Menu content should have the hub name but no relpath segment
				if !contains(content, "mhgo ide menu") {
					t.Errorf("ide-menu.cmd does not contain 'mhgo ide menu': %q", content)
				}
			},
		},
		{
			name:    "DotRelPath",
			slug:    "task-a",
			relPath: ".",
			verifyIde: func(t *testing.T, content string) {
				expected := "@cd /d \"%~dp0..\\..\\task-a\" && mhgo ide spawn task-a\r\n"
				if content != expected {
					t.Errorf("ide.cmd content = %q; want %q", content, expected)
				}
			},
			verifyMenu: func(t *testing.T, content string) {
				if !contains(content, "mhgo ide menu") {
					t.Errorf("ide-menu.cmd does not contain 'mhgo ide menu': %q", content)
				}
			},
		},
		{
			name:    "NonEmptyRelPath",
			slug:    "task-b",
			relPath: "subdir/nested",
			verifyIde: func(t *testing.T, content string) {
				expected := "@cd /d \"%~dp0..\\..\\task-b\\subdir\\nested\" && mhgo ide spawn task-b\r\n"
				if content != expected {
					t.Errorf("ide.cmd content = %q; want %q", content, expected)
				}
			},
			verifyMenu: func(t *testing.T, content string) {
				if !contains(content, "subdir\\nested") {
					t.Errorf("ide-menu.cmd does not contain relpath segment: %q", content)
				}
				if !contains(content, "mhgo ide menu") {
					t.Errorf("ide-menu.cmd does not contain 'mhgo ide menu': %q", content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newTestRepo(t)

			// Modify the hub to have the desired RelPath by creating subdirectories
			// and cd'ing into the deepest one before resolving
			var cwd string
			if tt.relPath != "" && tt.relPath != "." {
				testDir := filepath.Join(hub, tt.relPath)
				if err := os.MkdirAll(testDir, 0755); err != nil {
					t.Fatalf("mkdir test relpath: %v", err)
				}
				cwd = testDir
			} else {
				cwd = hub
			}

			l, err := paths.Resolve(cwd)
			if err != nil {
				t.Fatalf("paths.Resolve(%q): %v", cwd, err)
			}

			// Write launchers
			if err := worktree.WriteLaunchers(l, tt.slug); err != nil {
				t.Fatalf("WriteLaunchers: %v", err)
			}

			// Verify ide.cmd was created with correct content
			ideCmdPath := filepath.Join(l.LauncherDir(tt.slug), "ide.cmd")
			ideCmdContent, err := os.ReadFile(ideCmdPath)
			if err != nil {
				t.Fatalf("read ide.cmd: %v", err)
			}
			tt.verifyIde(t, string(ideCmdContent))

			// Verify ide-menu.cmd was created
			menuCmdPath := filepath.Join(l.LaunchersDir(), "ide-menu.cmd")
			menuCmdContent, err := os.ReadFile(menuCmdPath)
			if err != nil {
				t.Fatalf("read ide-menu.cmd: %v", err)
			}
			tt.verifyMenu(t, string(menuCmdContent))

			// Call WriteLaunchers again with a different slug to verify ide-menu.cmd is not clobbered
			originalMenuContent := string(menuCmdContent)
			if err := worktree.WriteLaunchers(l, "another-slug"); err != nil {
				t.Fatalf("WriteLaunchers again: %v", err)
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

// TestRemoveLaunchers covers launcher directory removal.
func TestRemoveLaunchers(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("launcher tests only run on Windows")
	}

	hub := newTestRepo(t)
	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hub, err)
	}

	// Write launchers for two slugs
	if err := worktree.WriteLaunchers(l, "slug1"); err != nil {
		t.Fatalf("WriteLaunchers slug1: %v", err)
	}
	if err := worktree.WriteLaunchers(l, "slug2"); err != nil {
		t.Fatalf("WriteLaunchers slug2: %v", err)
	}

	// Verify both launcher dirs exist
	slug1Dir := l.LauncherDir("slug1")
	slug2Dir := l.LauncherDir("slug2")
	if _, err := os.Stat(slug1Dir); err != nil {
		t.Fatalf("slug1 launcher dir does not exist: %v", err)
	}
	if _, err := os.Stat(slug2Dir); err != nil {
		t.Fatalf("slug2 launcher dir does not exist: %v", err)
	}

	// Remove slug1 launchers
	if err := worktree.RemoveLaunchers(l, "slug1"); err != nil {
		t.Fatalf("RemoveLaunchers slug1: %v", err)
	}

	// Verify slug1 dir is gone but slug2 remains
	if _, err := os.Stat(slug1Dir); err == nil {
		t.Error("slug1 launcher dir still exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat slug1 dir: %v", err)
	}

	if _, err := os.Stat(slug2Dir); err != nil {
		t.Fatalf("slug2 launcher dir was removed: %v", err)
	}

	// Verify ide-menu.cmd is still there
	menuCmdPath := filepath.Join(l.LaunchersDir(), "ide-menu.cmd")
	if _, err := os.Stat(menuCmdPath); err != nil {
		t.Fatalf("ide-menu.cmd was removed: %v", err)
	}

	// Second RemoveLaunchers call should be idempotent
	if err := worktree.RemoveLaunchers(l, "slug1"); err != nil {
		t.Fatalf("second RemoveLaunchers slug1: %v", err)
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0))
}
