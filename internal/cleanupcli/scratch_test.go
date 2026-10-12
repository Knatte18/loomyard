// scratch_test.go covers scratchSegment over a temp .scratch directory with a fixed clock and board reads.

package cleanupcli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

var scratchNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func touch(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes %s: %v", path, err)
	}
}

func writeScratchFile(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	touch(t, path, modTime)
}

func boardTask(brief string, status *string) boardengine.Task {
	return boardengine.Task{Slug: "entry", Title: "entry", Brief: brief, Status: status}
}

func TestScratchSegment(t *testing.T) {
	t.Parallel()

	old := scratchNow.Add(-scratchMaxAge - 24*time.Hour)
	recent := scratchNow.Add(-time.Hour)
	done, running := "done", "running"

	// oldDirectory lays out dir/name as a directory whose every entry is old.
	oldDirectory := func(t *testing.T, dir, name string) {
		t.Helper()
		writeScratchFile(t, filepath.Join(dir, name, "sub", "file.txt"), old)
		touch(t, filepath.Join(dir, name, "sub"), old)
		touch(t, filepath.Join(dir, name), old)
	}

	tests := []struct {
		name string
		// now overrides scratchNow when non-zero.
		now      time.Time
		setup    func(t *testing.T, dir string)
		tasks    []boardengine.Task
		boardErr error
		noDir    bool
		// survivor is a path under the temp root that must still exist after the run.
		survivor string
		want     []string
		wantErr  bool
	}{
		{
			name: "an old child is removed",
			setup: func(t *testing.T, dir string) {
				writeScratchFile(t, filepath.Join(dir, "old.txt"), old)
				oldDirectory(t, dir, "old-dir")
			},
			want: []string{"old-dir", "old.txt"},
		},
		{
			name: "a child with one new file deep inside is kept",
			setup: func(t *testing.T, dir string) {
				oldDirectory(t, dir, "busy")
				writeScratchFile(t, filepath.Join(dir, "busy", "sub", "fresh.txt"), recent)
				touch(t, filepath.Join(dir, "busy", "sub"), old)
			},
			want: []string{},
		},
		{
			name: "an old child cited by a not-done entry is kept and one cited only by a done entry is removed",
			setup: func(t *testing.T, dir string) {
				oldDirectory(t, dir, "open-cited")
				oldDirectory(t, dir, "unstarted-cited")
				oldDirectory(t, dir, "done-cited")
			},
			tasks: []boardengine.Task{
				boardTask("keep .scratch/open-cited around", &running),
				boardTask("see .scratch/unstarted-cited", nil),
				boardTask("see .scratch/done-cited", &done),
			},
			want: []string{"done-cited"},
		},
		{
			name: "an old link is removed as a link while its target survives",
			now:  time.Now().Add(scratchMaxAge + 24*time.Hour),
			setup: func(t *testing.T, dir string) {
				target := filepath.Join(filepath.Dir(dir), "target")
				writeScratchFile(t, filepath.Join(target, "file.txt"), time.Now().Add(365*24*time.Hour))
				if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			survivor: filepath.Join("target", "file.txt"),
			want:     []string{"link"},
		},
		{
			name:  "a missing scratch directory is an empty segment",
			noDir: true,
			want:  []string{},
		},
		{
			name: "a board read error fails the segment with nothing removed",
			setup: func(t *testing.T, dir string) {
				oldDirectory(t, dir, "old-dir")
			},
			boardErr: errors.New("board unreadable"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		for _, apply := range []bool{false, true} {
			name := tt.name + "/dry run"
			if apply {
				name = tt.name + "/apply"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				root := t.TempDir()
				dir := filepath.Join(root, scratchDirName)
				if !tt.noDir {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("MkdirAll: %v", err)
					}
					tt.setup(t, dir)
				}
				before := childNames(t, dir)
				now := scratchNow
				if !tt.now.IsZero() {
					now = tt.now
				}

				got, err := scratchSegment(dir, func() ([]boardengine.Task, error) { return tt.tasks, tt.boardErr }, now, apply)
				if (err != nil) != tt.wantErr {
					t.Fatalf("scratchSegment error = %v, want error: %v", err, tt.wantErr)
				}
				if !slices.Equal(got, tt.want) {
					t.Errorf("segment listed %v, want %v", got, tt.want)
				}

				wantLeft := before
				if apply {
					wantLeft = nil
					for _, child := range before {
						if !slices.Contains(tt.want, child) {
							wantLeft = append(wantLeft, child)
						}
					}
				}
				if left := childNames(t, dir); !slices.Equal(left, wantLeft) {
					t.Errorf("children left = %v, want %v", left, wantLeft)
				}
				if tt.survivor != "" {
					if _, err := os.Stat(filepath.Join(root, tt.survivor)); err != nil {
						t.Errorf("%s did not survive: %v", tt.survivor, err)
					}
				}
			})
		}
	}
}

// childNames lists the direct children of dir, none when dir is missing.
func childNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
