package fslink

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

// TestCreate covers link creation: it creates links that resolve to their targets,
// refuses to clobber existing regular files/directories, and creates missing
// parent directories.
func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup prepares the test directory and returns (link, target).
		// It may call t.Skip when the platform cannot create links.
		setup func(t *testing.T) (link, target string)
		// verify runs post-condition checks after Create.
		verify  func(t *testing.T, link, target string)
		wantErr bool
	}{
		{
			name: "CreatesLink",
			setup: func(t *testing.T) (link, target string) {
				tmpdir := t.TempDir()
				link = filepath.Join(tmpdir, "subdir", "link")
				target = filepath.Join(tmpdir, "mytarget")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				// Try to create a test link to verify the platform supports it.
				testLink := filepath.Join(tmpdir, "test-link")
				if err := Create(testLink, target); err != nil {
					t.Skipf("link creation not permitted on this platform: %v", err)
				}
				// Clean up the test link.
				Remove(testLink)
				return link, target
			},
			verify: func(t *testing.T, link, target string) {
				// The link should exist.
				_, err := os.Lstat(link)
				if err != nil {
					t.Errorf("lstat link: %v", err)
					return
				}
				// PointsTo should resolve to the target (absolute, no \??\ prefix).
				resolved, err := PointsTo(link)
				if err != nil {
					t.Errorf("PointsTo: %v", err)
					return
				}
				// Normalize paths for comparison (both should be absolute).
				targetAbs, _ := filepath.Abs(target)
				if filepath.Clean(resolved) != filepath.Clean(targetAbs) {
					t.Errorf("PointsTo(%s) = %q; want %q", link, resolved, filepath.Clean(targetAbs))
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
				if err := os.Mkdir(target, 0o755); err != nil {
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
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				// Verify platform support
				testLink := filepath.Join(tmpdir, "test-link")
				if err := Create(testLink, target); err != nil {
					t.Skipf("link creation not permitted on this platform: %v", err)
				}
				Remove(testLink)
				return link, target
			},
			verify: func(t *testing.T, link, target string) {
				// The link should exist.
				if _, err := os.Lstat(link); err != nil {
					t.Errorf("lstat link: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			link, target := tt.setup(t)

			err := Create(link, target)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Create() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("Create() error = %v; want nil", err)
			}

			if !tt.wantErr && tt.verify != nil {
				tt.verify(t, link, target)
			}
		})
	}
}

// TestIsLink covers link detection: returns true for created links, false for
// regular files and real directories, and (false, nil) for missing paths.
func TestIsLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    bool
		wantErr bool
	}{
		{
			name: "CreatedLink",
			setup: func(t *testing.T) string {
				tmpdir := t.TempDir()
				target := filepath.Join(tmpdir, "target")
				link := filepath.Join(tmpdir, "link")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				if err := Create(link, target); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				return link
			},
			want: true,
		},
		{
			name: "RegularFile",
			setup: func(t *testing.T) string {
				tmpdir := t.TempDir()
				file := filepath.Join(tmpdir, "file.txt")
				writeFile(t, file, "content")
				return file
			},
			want: false,
		},
		{
			name: "RealDirectory",
			setup: func(t *testing.T) string {
				tmpdir := t.TempDir()
				dir := filepath.Join(tmpdir, "subdir")
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return dir
			},
			want: false,
		},
		{
			name: "MissingPath",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)

			got, err := IsLink(path)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("IsLink() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("IsLink() error = %v; want nil", err)
			}

			if got != tt.want {
				t.Errorf("IsLink(%s) = %v; want %v", path, got, tt.want)
			}
		})
	}
}

// TestPointsTo covers target resolution: returns the resolved absolute target
// for a valid link (no \??\ prefix), and errors for non-links and missing targets.
func TestPointsTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (link string, wantTarget string)
		wantErr bool
	}{
		{
			name: "ResolvesLink",
			setup: func(t *testing.T) (link string, wantTarget string) {
				tmpdir := t.TempDir()
				target := filepath.Join(tmpdir, "target")
				link = filepath.Join(tmpdir, "link")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				if err := Create(link, target); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				// Want the absolute target
				targetAbs, _ := filepath.Abs(target)
				return link, filepath.Clean(targetAbs)
			},
			wantErr: false,
		},
		{
			name: "ErrorsOnNonLink",
			setup: func(t *testing.T) (link string, wantTarget string) {
				tmpdir := t.TempDir()
				file := filepath.Join(tmpdir, "file.txt")
				writeFile(t, file, "content")
				return file, ""
			},
			wantErr: true,
		},
		{
			name: "ErrorsOnDanglingLink",
			setup: func(t *testing.T) (link string, wantTarget string) {
				tmpdir := t.TempDir()
				target := filepath.Join(tmpdir, "target")
				link = filepath.Join(tmpdir, "link")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				if err := Create(link, target); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				// Delete the target directory to create a dangling link
				if err := os.Remove(target); err != nil {
					t.Fatalf("remove target: %v", err)
				}
				return link, ""
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			link, wantTarget := tt.setup(t)

			got, err := PointsTo(link)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("PointsTo() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("PointsTo() error = %v; want nil", err)
			}

			if !tt.wantErr && filepath.Clean(got) != wantTarget {
				t.Errorf("PointsTo(%s) = %q; want %q", link, filepath.Clean(got), wantTarget)
			}
		})
	}
}

// TestRemove covers link removal: removes a link, leaves the target intact, and
// is idempotent on a second call against an absent link.
func TestRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		verify  func(t *testing.T, link string)
	}{
		{
			name: "RemovesLink",
			setup: func(t *testing.T) string {
				tmpdir := t.TempDir()
				target := filepath.Join(tmpdir, "target")
				link := filepath.Join(tmpdir, "link")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("create target: %v", err)
				}
				if err := Create(link, target); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				return link
			},
			wantErr: false,
			verify: func(t *testing.T, link string) {
				targetDir := filepath.Join(filepath.Dir(link), "target")
				// Verify link is gone
				if _, err := os.Lstat(link); err == nil {
					t.Error("Remove() did not delete the link")
				}
				// Verify target survives
				if _, err := os.Stat(targetDir); err != nil {
					t.Errorf("Remove() deleted target: %v", err)
				}
			},
		},
		{
			name: "IdempotentOnMissing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			wantErr: false,
			verify: func(t *testing.T, link string) {
				// Second call should also succeed
				err := Remove(link)
				if err != nil {
					t.Fatalf("Remove() second call error = %v; want nil", err)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			link := tt.setup(t)

			err := Remove(link)
			if err != nil {
				t.Fatalf("Remove() error = %v; want nil", err)
			}

			if tt.verify != nil {
				tt.verify(t, link)
			}
		})
	}
}

// TestRemoveLinksIn covers the link scanner: ignores regular files and real
// directories, removes and counts links, and surfaces ReadDir errors for
// missing directories.
func TestRemoveLinksIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantCount int
		wantErr   bool
		verify    func(t *testing.T, dir string)
	}{
		{
			name: "IgnoresRegularFilesAndDirs",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "regular.txt"), "content")
				if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
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
				link1 := filepath.Join(dir, "link1")
				link2 := filepath.Join(dir, "link2")
				// Verify platform support
				if err := Create(link1, filepath.Join(dir, "target1.txt")); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				// Now create the second link
				if err := Create(link2, filepath.Join(dir, "target2.txt")); err != nil {
					t.Skipf("link creation not permitted: %v", err)
				}
				return dir
			},
			wantCount: 2,
			verify: func(t *testing.T, dir string) {
				// The links must be gone but their targets and the control file
				// must survive — RemoveLinksIn only deletes the link entries.
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := tt.setup(t)

			count, err := RemoveLinksIn(dir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("RemoveLinksIn() error = nil; want error")
				}
			} else if err != nil {
				t.Fatalf("RemoveLinksIn() error = %v; want nil", err)
			}

			if count != tt.wantCount {
				t.Errorf("RemoveLinksIn() count = %d; want %d", count, tt.wantCount)
			}

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
