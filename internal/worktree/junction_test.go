//go:build integration

// junction_test.go covers the platform junction/symlink create helper, including
// its refuse-to-clobber behavior when the link already exists.
// Spawns mklink on Windows so it must be tagged integration.

package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateJunction covers the createJunction helper: creates a junction/symlink,
// refuses to clobber an existing path, and the created link resolves to the target.
func TestCreateJunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup prepares the test directory and returns (link, target, tempdir).
		// It may call t.Skip when the platform cannot create symlinks.
		setup func(t *testing.T) (link, target string)
		// verify runs post-condition checks after createJunction.
		verify  func(t *testing.T, link, target string)
		wantErr bool
	}{
		{
			name: "CreatesJunction",
			setup: func(t *testing.T) (link, target string) {
				tmpdir := t.TempDir()
				link = filepath.Join(tmpdir, "subdir", "link")
				target = filepath.Join(tmpdir, "mytarget")
				if err := os.Mkdir(target, 0755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				// Try to create a test junction/symlink to verify the platform supports it.
				testLink := filepath.Join(tmpdir, "test-link")
				if err := createJunction(testLink, target); err != nil {
					t.Skipf("junction/symlink creation not permitted on this platform: %v", err)
				}
				// Clean up the test link.
				os.Remove(testLink)
				return link, target
			},
			verify: func(t *testing.T, link, target string) {
				// The link should exist.
				_, err := os.Lstat(link)
				if err != nil {
					t.Errorf("lstat junction: %v", err)
					return
				}
				// Readlink should resolve to the target.
				resolved, err := os.Readlink(link)
				if err != nil {
					t.Errorf("readlink: %v", err)
					return
				}
				// Normalize paths for comparison.
				resolvedAbs, _ := filepath.Abs(resolved)
				targetAbs, _ := filepath.Abs(target)
				if filepath.Clean(resolvedAbs) != filepath.Clean(targetAbs) && filepath.Clean(resolved) != filepath.Clean(target) {
					// Allow relative paths to work (e.g., on POSIX).
					if resolved != target {
						t.Errorf("readlink = %q; want %q", resolved, target)
					}
				}
			},
		},
		{
			name: "RefusesToClobber",
			setup: func(t *testing.T) (link, target string) {
				tmpdir := t.TempDir()
				link = filepath.Join(tmpdir, "existing")
				target = filepath.Join(tmpdir, "mytarget")
				// Pre-create a regular file at the link path.
				if err := os.WriteFile(link, []byte("existing"), 0644); err != nil {
					t.Fatalf("create existing file: %v", err)
				}
				if err := os.Mkdir(target, 0755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				return link, target
			},
			wantErr: true,
		},
		{
			name: "CreatesParentDir",
			setup: func(t *testing.T) (link, target string) {
				tmpdir := t.TempDir()
				link = filepath.Join(tmpdir, "a", "b", "c", "link")
				target = filepath.Join(tmpdir, "mytarget")
				if err := os.Mkdir(target, 0755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				return link, target
			},
			verify: func(t *testing.T, link, target string) {
				// The link should exist.
				if _, err := os.Lstat(link); err != nil {
					t.Errorf("lstat junction: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			link, target := tt.setup(t)

			err := createJunction(link, target)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("createJunction() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("createJunction() error = %v; want nil", err)
			}

			if !tt.wantErr && tt.verify != nil {
				tt.verify(t, link, target)
			}
		})
	}
}
