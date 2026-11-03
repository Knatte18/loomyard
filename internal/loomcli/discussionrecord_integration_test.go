// discussionrecord_integration_test.go drives the Discussion approve seam against a real hub pair and its hub board.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestApproveDiscussion_Real drives approveDiscussion over one real pair per row, each with its own board entry, and asserts what the seam stores and when it fails.
// It is serial because it sets the board skip variable, which is process-global.
func TestApproveDiscussion_Real(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	const record = "# Decision record\n\nThe approved decisions.\n"
	tests := []struct {
		name         string
		slug         string
		withEntry    bool
		markDone     bool
		withRecord   bool
		wantErr      string
		wantStored   bool
		wantFileKept bool
	}{
		{"StoresTheWorkingCopyAndFlagsTheEntry", "stores", true, false, true, "", true, true},
		{"DoneEntryPassesWithoutStoring", "landed", true, true, true, "", false, false},
		{"AbsentEntryFailsTheSettle", "noentry", false, false, true, "lym board upsert", false, false},
		{"MissingWorkingCopyFailsTheSettle", "norecord", true, false, false, "read the decision record", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hubforge.AddPair(t, hub, tt.slug)
			location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(tt.slug))
			if err != nil {
				t.Fatalf("ResolveWorktree() error = %v; want nil", err)
			}
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				t.Fatalf("OpenHub() error = %v; want nil", err)
			}
			if tt.withEntry {
				if _, err := board.UpsertTask(map[string]any{"slug": tt.slug, "title": "Approve", "kind": "task", "labels": []string{"bug"}}); err != nil {
					t.Fatalf("UpsertTask() error = %v; want nil", err)
				}
			}
			if tt.markDone {
				done := "done"
				if err := board.SetStatus(tt.slug, &done); err != nil {
					t.Fatalf("SetStatus(done) error = %v; want nil", err)
				}
			}
			if tt.withRecord {
				path := loomengine.DiscussionDecisionRecord(location)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll() error = %v; want nil", err)
				}
				if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v; want nil", err)
				}
			}

			err = approveDiscussion(location, shedrun.SelfRunID)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("approveDiscussion() error = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("approveDiscussion() error = %v; want it to contain %q", err, tt.wantErr)
			}

			got, found, err := board.DecisionRecord(tt.slug)
			if err != nil {
				t.Fatalf("DecisionRecord() error = %v; want nil", err)
			}
			if found != tt.wantStored || (found && string(got) != record) {
				t.Errorf("DecisionRecord() = %q, %v; want stored = %v with the working copy's bytes", got, found, tt.wantStored)
			}
			_, statErr := os.Stat(filepath.Join(fabricengine.BoardDir(location.HubPath), "decision-"+tt.slug+".md"))
			if (statErr == nil) != tt.wantFileKept {
				t.Errorf("record file present = %v; want %v", statErr == nil, tt.wantFileKept)
			}
		})
	}
}
