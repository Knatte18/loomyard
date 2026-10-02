// parent_test.go covers ResolveParent's decision table over the pure helper, and the one filesystem case a parent worktree that no longer exists.

package hubgeom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestDecideParent(t *testing.T) {
	tests := []struct {
		name          string
		shortname     string
		origin        fabricengine.Origin
		originFound   bool
		legacy        string
		parentIsPrime bool
		want          Parent
	}{
		{
			name:          "prime parent gives the two-segment orch name",
			shortname:     "tst",
			origin:        fabricengine.Origin{ParentWorktree: "prime"},
			originFound:   true,
			parentIsPrime: true,
			want:          Parent{Name: "tst:orch", Worktree: "prime"},
		},
		{
			name:        "pair parent gives the three-segment orch name",
			shortname:   "tst",
			origin:      fabricengine.Origin{ParentWorktree: "other-task"},
			originFound: true,
			want:        Parent{Name: "tst:other-task:orch", Worktree: "other-task"},
		},
		{
			name:      "no origin record and no legacy seed gives none",
			shortname: "tst",
			want:      Parent{},
		},
		{
			name:        "origin without parent worktree falls back to the legacy seed name",
			shortname:   "tst",
			origin:      fabricengine.Origin{ParentBranch: "main"},
			originFound: true,
			legacy:      "tst:legacy",
			want:        Parent{Name: "tst:legacy"},
		},
		{
			name:      "no origin record falls back to the legacy seed name",
			shortname: "tst",
			legacy:    "tst:legacy",
			want:      Parent{Name: "tst:legacy"},
		},
		{
			name:        "origin without parent worktree and no legacy seed gives none",
			shortname:   "tst",
			origin:      fabricengine.Origin{ParentBranch: "main"},
			originFound: true,
			want:        Parent{},
		},
		{
			name:          "no shortname gives none",
			origin:        fabricengine.Origin{ParentWorktree: "prime"},
			originFound:   true,
			legacy:        "tst:legacy",
			parentIsPrime: true,
			want:          Parent{},
		},
		{
			name:        "origin parent wins over the legacy seed",
			shortname:   "tst",
			origin:      fabricengine.Origin{ParentWorktree: "other-task"},
			originFound: true,
			legacy:      "tst:legacy",
			want:        Parent{Name: "tst:other-task:orch", Worktree: "other-task"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decideParent(tt.shortname, tt.origin, tt.originFound, tt.legacy, tt.parentIsPrime)
			if err != nil {
				t.Fatalf("decideParent: %v", err)
			}
			if got != tt.want {
				t.Errorf("decideParent = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideParent_InvalidWorktreeNameIsAnError(t *testing.T) {
	_, err := decideParent("tst", fabricengine.Origin{ParentWorktree: "Bad Name"}, true, "", false)
	if err == nil {
		t.Fatal("decideParent with an invalid parent worktree name: want error, got nil")
	}
}

// TestResolveParent_RemovedParentWorktreeStillResolves asserts a parent pair that no longer exists on disk still yields the pair-form name.
func TestResolveParent_RemovedParentWorktreeStillResolves(t *testing.T) {
	hub := filepath.Join(t.TempDir(), "some-hub-LYXHUB")
	l := &lyxcwd.Location{HubPath: hub, WorktreeName: "child-task", AnchorRel: "."}

	boardDir := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll board: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, fabricengine.ShortnameFileName), []byte("tst\n"), 0o644); err != nil {
		t.Fatalf("write shortname: %v", err)
	}
	// The record is written by hand: WriteOrigin seeds git excludes, which spawns git, and this file is untagged.
	// ReadOriginFor requires the weft worktree's lock directory beside the record.
	recordPath := fabricengine.OriginRecordPathFor(l, l.WorktreeName)
	for _, dir := range []string{filepath.Dir(recordPath), filepath.Join(fabricengine.WeftWorktreePath(l, l.WorktreeName), ".weft")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
	}
	record := `{"parent_branch":"main","parent_worktree":"gone-task"}`
	if err := os.WriteFile(recordPath, []byte(record), 0o644); err != nil {
		t.Fatalf("write origin record: %v", err)
	}

	got, err := ResolveParent(l)
	if err != nil {
		t.Fatalf("ResolveParent: %v", err)
	}
	want := Parent{Name: "tst:gone-task:orch", Worktree: "gone-task"}
	if got != want {
		t.Errorf("ResolveParent = %+v; want %+v", got, want)
	}
}
