package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file with the given content, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRemoveLinks covers the junction/symlink scanner: regular files and real
// directories are left untouched, symlinks are removed and counted, and a
// missing directory surfaces the os.ReadDir error.
func TestRemoveLinks(t *testing.T) {
	tests := []struct {
		name string
		// setup populates a scratch dir and returns the path to scan. It may
		// call t.Skip when the platform cannot create symlinks.
		setup func(t *testing.T) string
		// verify runs scenario-specific post-conditions after removeLinks; nil
		// when the count/err assertions are sufficient.
		verify    func(t *testing.T, dir string)
		wantCount int
		wantErr   bool
	}{
		{
			name: "IgnoresRegularFilesAndDirs",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "regular.txt"), "content")
				if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
					t.Fatalf("create subdir: %v", err)
				}
				return dir
			},
			wantCount: 0,
			verify: func(t *testing.T, dir string) {
				// Neither the regular file nor the real directory may be touched.
				if _, err := os.Stat(filepath.Join(dir, "regular.txt")); err != nil {
					t.Errorf("regular.txt was removed: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dir, "subdir")); err != nil {
					t.Errorf("subdir was removed: %v", err)
				}
			},
		},
		{
			name: "RemovesSymlinks",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "target1.txt"), "target1")
				writeFile(t, filepath.Join(dir, "target2.txt"), "target2")
				writeFile(t, filepath.Join(dir, "regular.txt"), "regular")
				if err := os.Symlink(filepath.Join(dir, "target1.txt"), filepath.Join(dir, "link1")); err != nil {
					t.Skipf("symlinks not permitted on this platform: %v", err)
				}
				if err := os.Symlink(filepath.Join(dir, "target2.txt"), filepath.Join(dir, "link2")); err != nil {
					t.Skipf("symlinks not permitted on this platform: %v", err)
				}
				return dir
			},
			wantCount: 2,
			verify: func(t *testing.T, dir string) {
				// The links must be gone but their targets and the control file
				// must survive — removeLinks only deletes the link entries.
				if _, err := os.Lstat(filepath.Join(dir, "link1")); err == nil {
					t.Error("link1 still exists")
				}
				if _, err := os.Lstat(filepath.Join(dir, "link2")); err == nil {
					t.Error("link2 still exists")
				}
				for _, survivor := range []string{"target1.txt", "target2.txt", "regular.txt"} {
					if _, err := os.Stat(filepath.Join(dir, survivor)); err != nil {
						t.Errorf("%s was removed: %v", survivor, err)
					}
				}
			},
		},
		{
			name: "NonexistentDir",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			count, err := removeLinks(dir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("removeLinks() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("removeLinks() error = %v; want nil", err)
			}

			if count != tt.wantCount {
				t.Errorf("removeLinks() count = %d; want %d", count, tt.wantCount)
			}

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
