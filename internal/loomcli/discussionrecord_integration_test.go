// discussionrecord_integration_test.go drives the Discussion approve seam and the Motif block's seed seam against a real hub pair and its hub board.

package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestApproveDiscussion_Real drives approveDiscussion over one real pair per row, each with its own board entry.
// It asserts what the seam stores and when it fails.
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

// TestSeedDiscussedRecord_Real drives seedDiscussedRecord over one real pair per row, each with its own board entry.
// It asserts what the seam seeds into the run and commits.
// It is serial because it sets the board skip variable, which is process-global.
func TestSeedDiscussedRecord_Real(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("BOARD_SKIP_GIT", "1")
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	stored := loomshed.BoardRecord("The approved goal.")
	const existingLog = "# Support log\n\nWritten by the run.\n"
	tests := []struct {
		name      string
		slug      string
		withEntry bool
		discussed bool
		// dropStoredFile removes the flagged entry's record file from the board directory.
		dropStoredFile bool
		// existingRecord and existingLog, when set, are on disk in the run's discussion directory before the seam runs.
		existingRecord string
		existingLog    string
		// blockDir puts a regular file where the discussion directory belongs.
		blockDir bool
		// blockRecordWrite puts a directory where the decision record file belongs.
		blockRecordWrite bool
		// blockStoredFile puts a directory where the flagged entry's record file belongs in the board directory.
		blockStoredFile bool
		// lockIndex leaves the run-records worktree's index locked, so the commit fails.
		lockIndex  bool
		wantSeeded bool
		// errContains, when set, is the fragment the returned error must contain; the seam must then report false.
		errContains string
		// wantNoSupportLog asserts no support log was written before the failure.
		wantNoSupportLog bool
	}{
		{name: "DiscussedEntrySeedsAndCommitsItsRecord", slug: "discussed", withEntry: true, discussed: true, wantSeeded: true},
		{name: "UndiscussedEntrySeedsNothing", slug: "undiscussed", withEntry: true},
		{name: "AbsentEntrySeedsNothing", slug: "absent"},
		{name: "FlaggedEntryWithMissingRecordFileSeedsNothing", slug: "nofile", withEntry: true, discussed: true, dropStoredFile: true},
		{name: "ExistingRecordIsOverwrittenAndExistingSupportLogKept", slug: "existing", withEntry: true, discussed: true, existingRecord: "# Old record\n", existingLog: existingLog, wantSeeded: true},
		{name: "UnwritableDiscussionDirectoryFails", slug: "blocked", withEntry: true, discussed: true, blockDir: true, errContains: "create the discussion directory", wantNoSupportLog: true},
		{name: "UnwritableRecordFileFails", slug: "recordblocked", withEntry: true, discussed: true, blockRecordWrite: true, errContains: "write the stored decision record", wantNoSupportLog: true},
		{name: "UnreadableStoredRecordFails", slug: "boardfail", withEntry: true, discussed: true, blockStoredFile: true, errContains: "read decision record", wantNoSupportLog: true},
		{name: "LockedIndexFailsTheCommit", slug: "commitfail", withEntry: true, discussed: true, lockIndex: true, errContains: "commit the stored decision record"},
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
				if _, err := board.UpsertTask(map[string]any{"slug": tt.slug, "title": "Seed", "kind": "task", "labels": []string{"bug"}}); err != nil {
					t.Fatalf("UpsertTask() error = %v; want nil", err)
				}
			}
			if tt.discussed {
				if err := board.StoreDecisionRecord(tt.slug, stored); err != nil {
					t.Fatalf("StoreDecisionRecord() error = %v; want nil", err)
				}
			}
			if tt.dropStoredFile || tt.blockStoredFile {
				if err := os.Remove(filepath.Join(fabricengine.BoardDir(location.HubPath), "decision-"+tt.slug+".md")); err != nil {
					t.Fatalf("Remove(stored record file) error = %v; want nil", err)
				}
			}
			if tt.blockStoredFile {
				if err := os.Mkdir(filepath.Join(fabricengine.BoardDir(location.HubPath), "decision-"+tt.slug+".md"), 0o755); err != nil {
					t.Fatalf("Mkdir(stored record path) error = %v; want nil", err)
				}
			}
			record := runRecordOf(location, shedrun.RecipeTapestry)
			if tt.existingRecord != "" || tt.existingLog != "" {
				if err := os.MkdirAll(record.dir, 0o755); err != nil {
					t.Fatalf("MkdirAll() error = %v; want nil", err)
				}
			}
			if tt.existingRecord != "" {
				if err := os.WriteFile(record.path, []byte(tt.existingRecord), 0o644); err != nil {
					t.Fatalf("WriteFile(record) error = %v; want nil", err)
				}
			}
			if tt.existingLog != "" {
				if err := os.WriteFile(record.supportLog, []byte(tt.existingLog), 0o644); err != nil {
					t.Fatalf("WriteFile(support log) error = %v; want nil", err)
				}
			}
			if tt.blockDir {
				if err := os.MkdirAll(filepath.Dir(record.dir), 0o755); err != nil {
					t.Fatalf("MkdirAll() error = %v; want nil", err)
				}
				if err := os.WriteFile(record.dir, []byte("not a directory"), 0o644); err != nil {
					t.Fatalf("WriteFile(blocking file) error = %v; want nil", err)
				}
			}

			if tt.blockRecordWrite {
				if err := os.MkdirAll(record.path, 0o755); err != nil {
					t.Fatalf("MkdirAll(record path) error = %v; want nil", err)
				}
			}
			if tt.lockIndex {
				out, err := exec.Command("git", "-C", fabricengine.RecordsWorktree(location), "rev-parse", "--absolute-git-dir").Output()
				if err != nil {
					t.Fatalf("git rev-parse --absolute-git-dir error = %v; want nil", err)
				}
				indexLock := filepath.Join(strings.TrimSpace(string(out)), "index.lock")
				if err := os.WriteFile(indexLock, nil, 0o644); err != nil {
					t.Fatalf("WriteFile(index.lock) error = %v; want nil", err)
				}
				t.Cleanup(func() { _ = os.Remove(indexLock) })
			}

			seeded, err := seedDiscussedRecord(location, shedrun.SelfRunID, record)
			if tt.errContains != "" {
				if seeded || err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("seedDiscussedRecord() = %v, %v; want false and an error containing %q", seeded, err, tt.errContains)
				}
				if _, statErr := os.Stat(record.supportLog); tt.wantNoSupportLog && statErr == nil {
					t.Errorf("support log %s present; want none written before the failure", record.supportLog)
				}
				return
			}
			if err != nil || seeded != tt.wantSeeded {
				t.Fatalf("seedDiscussedRecord() = %v, %v; want %v, nil", seeded, err, tt.wantSeeded)
			}

			got, readErr := os.ReadFile(record.path)
			if !tt.wantSeeded {
				if readErr == nil {
					t.Errorf("decision record = %q; want none written", got)
				}
				return
			}
			if readErr != nil || string(got) != string(stored) {
				t.Errorf("decision record = %q, %v; want the stored record %q", got, readErr, stored)
			}
			wantLog := loomshed.StoredRecordSupportLog()
			if tt.existingLog != "" {
				wantLog = []byte(tt.existingLog)
			}
			if log, err := os.ReadFile(record.supportLog); err != nil || string(log) != string(wantLog) {
				t.Errorf("support log = %q, %v; want %q", log, err, wantLog)
			}
			for rel, want := range map[string][]byte{
				record.rel: stored,
				filepath.Join(record.dirRel, filepath.Base(record.supportLog)): wantLog,
			} {
				if data, found, err := fabricengine.CommittedAnchoredFile(location, rel); err != nil || !found || string(data) != string(want) {
					t.Errorf("CommittedAnchoredFile(%s) = %q, %v, %v; want the seeded file committed", rel, data, found, err)
				}
			}
		})
	}
}
