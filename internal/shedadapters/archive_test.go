package shedadapters

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedClock(instant time.Time) func() time.Time {
	return func() time.Time { return instant }
}

var archiveTestInstant = time.Date(2026, 8, 16, 15, 13, 26, 0, time.UTC)

//testtiming:keep pins the stamped archive name, the numeric collision suffix and that an absent entry in the list is skipped, none of which the callers' tests assert
func TestArchiveStaleOutputs(t *testing.T) {
	t.Parallel()
	// Each step creates its files, then archives its listed names; a listed name that was
	// never created is an absent entry.
	type step struct {
		create  []string
		archive []string
	}
	tests := []struct {
		name       string
		steps      []step
		wantExists []string
		wantGone   []string
	}{
		{
			name:       "renames an existing file",
			steps:      []step{{create: []string{"review.md"}, archive: []string{"review.md"}}},
			wantExists: []string{"review-20260816T151326Z.md"},
			wantGone:   []string{"review.md"},
		},
		{
			name: "a collision takes a numeric suffix",
			steps: []step{
				{create: []string{"review.md"}, archive: []string{"review.md"}},
				{create: []string{"review.md"}, archive: []string{"review.md"}},
			},
			wantExists: []string{"review-20260816T151326Z.md", "review-20260816T151326Z-1.md"},
			wantGone:   []string{"review.md"},
		},
		{
			name:     "an absent entry is a no-op",
			steps:    []step{{archive: []string{"missing.md"}}},
			wantGone: []string{"missing.md"},
		},
		{
			name:       "a mixed list archives only the existing entry",
			steps:      []step{{create: []string{"present.md"}, archive: []string{"present.md", "absent.md"}}},
			wantExists: []string{"present-20260816T151326Z.md"},
			wantGone:   []string{"present.md", "absent.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, s := range tt.steps {
				for _, name := range s.create {
					if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o644); err != nil {
						t.Fatalf("WriteFile: %v", err)
					}
				}
				paths := make([]string, len(s.archive))
				for i, name := range s.archive {
					paths[i] = filepath.Join(dir, name)
				}
				if err := archiveStaleOutputs(paths, fixedClock(archiveTestInstant)); err != nil {
					t.Fatalf("archiveStaleOutputs(%v) = %v; want nil", s.archive, err)
				}
			}

			for _, name := range tt.wantExists {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("expected file %s to exist: %v", name, err)
				}
			}
			for _, name := range tt.wantGone {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Errorf("path %s exists after archive; want it gone", name)
				}
			}
		})
	}
}

//testtiming:keep pins that the run dir moves whole to a stamped sibling and is recreated empty, the numeric collision suffix, and that an absent run dir is an error
func TestArchiveRunDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// files are created in the run dir before the first archive.
		files        []string
		runDirAbsent bool
		archives     int
		wantErr      bool
		// wantSiblings maps each archived sibling dir to its entry count.
		wantSiblings map[string]int
	}{
		{
			name:         "moves every entry and recreates the run dir empty",
			files:        []string{"round-1-review.md", "round-1-bouncer-verdict.md"},
			archives:     1,
			wantSiblings: map[string]int{"run-20260816T151326Z": 2},
		},
		{
			name:         "a collision takes a numeric suffix",
			archives:     2,
			wantSiblings: map[string]int{"run-20260816T151326Z": 0, "run-20260816T151326Z-1": 0},
		},
		{
			name:         "an absent run dir is an error",
			runDirAbsent: true,
			archives:     1,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			runDir := filepath.Join(parent, "run")
			if !tt.runDirAbsent {
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
			}
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(runDir, name), []byte("content"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			for i := 0; i < tt.archives; i++ {
				err := archiveRunDir(runDir, fixedClock(archiveTestInstant))
				if tt.wantErr {
					if err == nil {
						t.Fatalf("archiveRunDir(absent run dir) = nil; want error")
					}
					return
				}
				if err != nil {
					t.Fatalf("archiveRunDir (call %d) = %v; want nil", i+1, err)
				}
			}

			for sibling, wantEntries := range tt.wantSiblings {
				entries, err := os.ReadDir(filepath.Join(parent, sibling))
				if err != nil {
					t.Fatalf("expected archived sibling %s to exist: %v", sibling, err)
				}
				if len(entries) != wantEntries {
					t.Errorf("archived sibling %s has %d entries; want %d", sibling, len(entries), wantEntries)
				}
			}
			freshEntries, err := os.ReadDir(runDir)
			if err != nil {
				t.Fatalf("expected recreated run dir %s to exist: %v", runDir, err)
			}
			if len(freshEntries) != 0 {
				t.Errorf("recreated run dir %s has %d entries; want 0", runDir, len(freshEntries))
			}
		})
	}
}
