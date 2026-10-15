// publishretry_test.go covers the mapping of a task worktree's Publish failure record into the retry's seam type.

package battencli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestReadChildPublishFailure_MapsTheRecord asserts readChildPublishFailure maps the record's row, head, target and write time, and reports found false for a task worktree with no record.
func TestReadChildPublishFailure_MapsTheRecord(t *testing.T) {
	t.Parallel()

	taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}

	if _, found, err := readChildPublishFailure(taskLocation); err != nil || found {
		t.Fatalf("readChildPublishFailure(no record) = found %v, err %v; want false, nil", found, err)
	}

	paths := verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath()))
	if err := os.MkdirAll(verifytree.Dir(taskLocation.AnchorPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	written := verifytree.PublishFailure{Row: "Merge-In", Head: "abc123", Target: "Webster", WrittenAt: "2026-01-01T11:20:00Z", LogPath: "verify-1.log"}
	if err := verifytree.WritePublishFailure(paths, written); err != nil {
		t.Fatal(err)
	}

	got, found, err := readChildPublishFailure(taskLocation)
	want := battenshed.ChildPublishFailure{Row: "Merge-In", Head: "abc123", Target: "Webster", WrittenAt: "2026-01-01T11:20:00Z"}
	if err != nil || !found || got != want {
		t.Errorf("readChildPublishFailure() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}
}

// TestChildGoVerifyRetrySpent_ReadsThePerRunRecords asserts childGoVerifyRetrySpent reads false for a task worktree with no settle records, true for a verify-retry record at the per-run path, and false for the same record at the friction-directory location the records used to sit in.
func TestChildGoVerifyRetrySpent_ReadsThePerRunRecords(t *testing.T) {
	t.Parallel()

	retry := sheddriver.SettleRecord{Settler: shedrun.SettlerGoRule, Action: shedrun.SettleVerifyRetry}
	newLocation := func() *lyxcwd.Location {
		return &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}
	}

	tests := []struct {
		name string
		path func(*lyxcwd.Location) string
		want bool
	}{
		{"no records file", nil, false},
		{"record at the per-run path", func(l *lyxcwd.Location) string { return shedrun.SettleRecords(l, shedrun.SelfRunID) }, true},
		{"record at the old friction-directory location", func(l *lyxcwd.Location) string {
			return filepath.Join(loomengine.LoomFrictionDir(l), "go-settle.jsonl")
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			taskLocation := newLocation()
			if tt.path != nil {
				if err := sheddriver.AppendSettleRecord(tt.path(taskLocation), retry); err != nil {
					t.Fatal(err)
				}
			}
			got, err := childGoVerifyRetrySpent(taskLocation)
			if err != nil || got != tt.want {
				t.Errorf("childGoVerifyRetrySpent() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}
