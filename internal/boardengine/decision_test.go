// decision_test.go — tests of an entry's decision record: its store and read, and the discussed payload rule.

package boardengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// readmeFile is the rendered overview file that a test board's config names.
const readmeFile = "Home.md"

// TestStoreDecisionRecord pins what StoreDecisionRecord writes and when it writes nothing.
// It also pins what StoreDecisionRecord refuses, and whether DecisionRecord finds the record.
func TestStoreDecisionRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	record := []byte("# Decision record\n\nA\r\nbyte-exact body.\n")
	tests := []struct {
		name string
		run  func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time)
	}{
		{"StoresBytesAndFlag", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(boardPath, "decision-old.md")); err != nil || string(got) != string(record) {
				t.Errorf("decision-old.md = %q, %v; want %q", got, err, record)
			}
			if task, _, _ := b.GetTask("old"); !task.Discussed {
				t.Errorf("GetTask(old).Discussed = false; want true")
			}
			boardJSON, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
			if err != nil {
				t.Fatalf("read board.json: %v", err)
			}
			if !strings.Contains(string(boardJSON), `"discussed": true`) || strings.Contains(string(boardJSON), "byte-exact") {
				t.Errorf("board.json must hold the flag and no record text:\n%s", boardJSON)
			}
			if err := b.Rerender(); err != nil {
				t.Fatalf("Rerender: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(boardPath, "decision-old.md")); err != nil || string(got) != string(record) {
				t.Errorf("decision-old.md after Rerender = %q, %v; want it kept", got, err)
			}
		}},
		{"RunHeldEntryTakesTheStore", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			before, _, _ := b.GetTask("waiter")
			*now = now.Add(time.Hour)
			if err := b.StoreDecisionRecord("waiter", record); err != nil {
				t.Fatalf("StoreDecisionRecord on a run-held entry: %v", err)
			}
			after, _, _ := b.GetTask("waiter")
			want := before
			want.Discussed = true
			if !reflect.DeepEqual(after, want) {
				t.Errorf("run-held entry after the store = %+v; want only Discussed changed: %+v", after, want)
			}
		}},
		{"RepeatWithSameBytesWritesNothing", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			if err := os.Remove(filepath.Join(boardPath, readmeFile)); err != nil {
				t.Fatalf("remove %s: %v", readmeFile, err)
			}
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("repeat StoreDecisionRecord: %v", err)
			}
			if _, err := os.Stat(filepath.Join(boardPath, readmeFile)); !os.IsNotExist(err) {
				t.Errorf("a repeat store with the same bytes rendered %s; stat err = %v", readmeFile, err)
			}
		}},
		{"HandWrittenSameBytesStillFlags", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := os.WriteFile(filepath.Join(boardPath, "decision-old.md"), record, 0o644); err != nil {
				t.Fatalf("write decision-old.md: %v", err)
			}
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			if task, _, _ := b.GetTask("old"); !task.Discussed {
				t.Errorf("GetTask(old).Discussed = false; want true after a store onto an identical hand-written file")
			}
		}},
		{"RepeatWithNewBytesOverwrites", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			if err := b.StoreDecisionRecord("old", []byte("second\n")); err != nil {
				t.Fatalf("repeat StoreDecisionRecord: %v", err)
			}
			if got, _ := os.ReadFile(filepath.Join(boardPath, "decision-old.md")); string(got) != "second\n" {
				t.Errorf("decision-old.md = %q; want %q", got, "second\n")
			}
		}},
		{"AbsentSlugRefused", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			err := b.StoreDecisionRecord("ghost", record)
			if err == nil || !strings.Contains(err.Error(), "lym board upsert") {
				t.Errorf("StoreDecisionRecord(ghost) error = %v; want one naming lym board upsert", err)
			}
		}},
		{"DoneEntryRefused", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			done := "done"
			if err := b.SetStatus("other", &done); err != nil {
				t.Fatalf("SetStatus: %v", err)
			}
			err := b.StoreDecisionRecord("other", record)
			if !errors.Is(err, boardengine.ErrRecordEntryDone) {
				t.Errorf("StoreDecisionRecord(done entry) error = %v; want ErrRecordEntryDone", err)
			}
			if _, statErr := os.Stat(filepath.Join(boardPath, "decision-other.md")); !os.IsNotExist(statErr) {
				t.Errorf("a refused store wrote decision-other.md; stat err = %v", statErr)
			}
		}},
		{"RecordNotFoundOnUnflaggedEntry", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := os.WriteFile(filepath.Join(boardPath, "decision-old.md"), record, 0o644); err != nil {
				t.Fatalf("write decision-old.md: %v", err)
			}
			if _, found, err := b.DecisionRecord("old"); found || err != nil {
				t.Errorf("DecisionRecord(unflagged) = found %v, err %v; want not found", found, err)
			}
			if _, found, err := b.DecisionRecord("ghost"); found || err != nil {
				t.Errorf("DecisionRecord(absent) = found %v, err %v; want not found", found, err)
			}
		}},
		{"RecordReadBackAndNotFoundWhenFileDeleted", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			if got, found, err := b.DecisionRecord("old"); !found || err != nil || string(got) != string(record) {
				t.Errorf("DecisionRecord(old) = %q, %v, %v; want the stored bytes", got, found, err)
			}
			if err := os.Remove(filepath.Join(boardPath, "decision-old.md")); err != nil {
				t.Fatalf("remove decision-old.md: %v", err)
			}
			if _, found, err := b.DecisionRecord("old"); found || err != nil {
				t.Errorf("DecisionRecord(flagged, file deleted) = found %v, err %v; want not found", found, err)
			}
		}},
		{"RecordReadErrorReturned", func(t *testing.T, b *boardengine.Board, boardPath string, now *time.Time) {
			if err := b.StoreDecisionRecord("old", record); err != nil {
				t.Fatalf("StoreDecisionRecord: %v", err)
			}
			path := filepath.Join(boardPath, "decision-old.md")
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove decision-old.md: %v", err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatalf("mkdir decision-old.md: %v", err)
			}
			if _, found, err := b.DecisionRecord("old"); found || err == nil {
				t.Errorf("DecisionRecord(file is a directory) = found %v, err %v; want a read error", found, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
			b, boardPath := newRenameBoard(t, &now)
			tt.run(t, b, boardPath, &now)
		})
	}
}

// TestDiscussedPayload pins the discussed payload rule on the upsert, batch and merge paths.
// The entry under test is one that StoreDecisionRecord flagged.
func TestDiscussedPayload(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const refusal = "lym loom decision store"
	apply := map[string]func(b *boardengine.Board, fields map[string]any) error{
		"Upsert": func(b *boardengine.Board, fields map[string]any) error {
			_, err := b.UpsertTask(fields)
			return err
		},
		"Batch": func(b *boardengine.Board, fields map[string]any) error {
			_, err := b.UpsertTasksBatch([]map[string]any{fields})
			return err
		},
		"Merge": func(b *boardengine.Board, fields map[string]any) error {
			_, _, err := b.MergeTasks([]string{"near"}, fields, nil)
			return err
		},
	}
	tests := []struct {
		name    string
		fields  map[string]any
		wantErr string
	}{
		{"FalseOnFlaggedEntryRefused", map[string]any{"slug": "old", "discussed": false}, refusal},
		{"TrueOnNewEntryRefused", map[string]any{"slug": "fresh", "title": "Fresh", "labels": bugLabels, "discussed": true}, refusal},
		{"NonBoolRefused", map[string]any{"slug": "old", "discussed": "yes"}, refusal},
		{"StoredValuePasses", map[string]any{"slug": "old", "discussed": true}, ""},
	}
	for path, write := range apply {
		for _, tt := range tests {
			t.Run(path+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
				b, _ := newRenameBoard(t, &now)
				if err := b.StoreDecisionRecord("old", []byte("record\n")); err != nil {
					t.Fatalf("StoreDecisionRecord: %v", err)
				}
				err := write(b, tt.fields)
				switch {
				case tt.wantErr == "" && err != nil:
					t.Fatalf("%s(%v) error = %v; want none", path, tt.fields, err)
				case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
					t.Fatalf("%s(%v) error = %v; want one containing %q", path, tt.fields, err, tt.wantErr)
				}
				if task, _, _ := b.GetTask("old"); !task.Discussed {
					t.Errorf("GetTask(old).Discussed = false; want the flag as stored")
				}
			})
		}
	}

	t.Run("MergeRemovingAndUpsertingTheFlaggedSlugKeepsTheFlag", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
		b, _ := newRenameBoard(t, &now)
		if err := b.StoreDecisionRecord("old", []byte("record\n")); err != nil {
			t.Fatalf("StoreDecisionRecord: %v", err)
		}
		fields := map[string]any{"slug": "old", "title": "Old again", "kind": boardengine.KindTask, "labels": bugLabels, "discussed": true}
		if _, _, err := b.MergeTasks([]string{"old"}, fields, nil); err != nil {
			t.Fatalf("MergeTasks: %v", err)
		}
		if task, _, _ := b.GetTask("old"); !task.Discussed || task.Title != "Old again" {
			t.Errorf("rebuilt entry = %+v; want Discussed kept on the rebuilt entry", task)
		}
	})
}

// TestDecisionRecordFollowsEntry pins that the record file moves with a renamed entry.
// It also pins that a removed entry takes its record file with it, and which writes keep the file.
func TestDecisionRecordFollowsEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	record := []byte("the record\n")
	// store flags slug with record, failing the test when the store is refused.
	store := func(t *testing.T, b *boardengine.Board, slug string) {
		t.Helper()
		if err := b.StoreDecisionRecord(slug, record); err != nil {
			t.Fatalf("StoreDecisionRecord(%s): %v", slug, err)
		}
	}
	// wantFile asserts that name in boardPath holds record when present is true and is absent otherwise.
	wantFile := func(t *testing.T, boardPath, name string, present bool) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(boardPath, name))
		switch {
		case present && (err != nil || string(got) != string(record)):
			t.Errorf("%s = %q, %v; want the record", name, got, err)
		case !present && !os.IsNotExist(err):
			t.Errorf("%s read err = %v; want the file absent", name, err)
		}
	}
	tests := []struct {
		name string
		run  func(t *testing.T, b *boardengine.Board, boardPath string)
	}{
		{"RenameMovesTheFile", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "old")
			if _, err := b.Rename("old", "new"); err != nil {
				t.Fatalf("Rename: %v", err)
			}
			wantFile(t, boardPath, "decision-new.md", true)
			wantFile(t, boardPath, "decision-old.md", false)
			if got, found, err := b.DecisionRecord("new"); !found || err != nil || string(got) != string(record) {
				t.Errorf("DecisionRecord(new) = %q, %v, %v; want the record", got, found, err)
			}
		}},
		{"RenameRefusedBySeamLeavesTheFile", func(t *testing.T, _ *boardengine.Board, _ string) {
			boardPath := t.TempDir()
			b := boardengine.New(boardengine.Config{Path: boardPath, Readme: readmeFile, DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true,
				ValidateSlug: func(slug string) error {
					if slug == "blocked" {
						return errors.New("slug refused by the seam")
					}
					return nil
				}})
			if _, err := b.UpsertTask(map[string]any{"slug": "old", "title": "Old", "kind": boardengine.KindTask, "labels": bugLabels}); err != nil {
				t.Fatalf("UpsertTask: %v", err)
			}
			store(t, b, "old")
			if _, err := b.Rename("old", "blocked"); err == nil {
				t.Fatalf("Rename onto a slug the seam refuses succeeded; want a refusal")
			}
			wantFile(t, boardPath, "decision-old.md", true)
			wantFile(t, boardPath, "decision-blocked.md", false)
		}},
		{"RenameOntoDirectoryReportsTheMoveAfterTheSave", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "old")
			if err := os.Mkdir(filepath.Join(boardPath, "decision-new.md"), 0o755); err != nil {
				t.Fatalf("mkdir decision-new.md: %v", err)
			}
			result, err := b.Rename("old", "new")
			var recordErr *boardengine.DecisionRecordError
			if !errors.As(err, &recordErr) || !strings.Contains(err.Error(), "rename moved the entry but not its decision record") || !strings.Contains(err.Error(), "decision-old.md") {
				t.Fatalf("Rename error = %v; want a *DecisionRecordError naming decision-old.md", err)
			}
			if result.From != "old" || result.Task.Slug != "new" || result.Mentions == nil {
				t.Errorf("Rename result = %+v; want the saved rename's result beside the record error", result)
			}
			wantFile(t, boardPath, "decision-old.md", true)
			if _, found, _ := b.GetTask("new"); !found {
				t.Errorf("the entry must be saved under the new slug despite the move error")
			}
			if _, statErr := os.Stat(filepath.Join(boardPath, "proposal-new.md")); statErr != nil {
				t.Errorf("proposal-new.md missing, the render must still run: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(boardPath, "proposal-old.md")); !os.IsNotExist(statErr) {
				t.Errorf("proposal-old.md must be gone; stat err = %v", statErr)
			}
		}},
		{"RenameOfFlaggedEntryWithoutFileSucceeds", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "old")
			if err := os.Remove(filepath.Join(boardPath, "decision-old.md")); err != nil {
				t.Fatalf("remove decision-old.md: %v", err)
			}
			if _, err := b.Rename("old", "new"); err != nil {
				t.Fatalf("Rename: %v", err)
			}
		}},
		{"RenameOfFlaggedEntryWithoutFileRemovesAnOrphanAtTheTarget", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "old")
			if err := os.Remove(filepath.Join(boardPath, "decision-old.md")); err != nil {
				t.Fatalf("remove decision-old.md: %v", err)
			}
			if err := os.WriteFile(filepath.Join(boardPath, "decision-new.md"), []byte("orphan\n"), 0o644); err != nil {
				t.Fatalf("write decision-new.md: %v", err)
			}
			if _, err := b.Rename("old", "new"); err != nil {
				t.Fatalf("Rename: %v", err)
			}
			wantFile(t, boardPath, "decision-new.md", false)
			if _, found, err := b.DecisionRecord("new"); found || err != nil {
				t.Errorf("DecisionRecord(new) = found %v, err %v; want not found", found, err)
			}
		}},
		{"RemoveDeletesTheFile", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "near")
			if err := b.RemoveTask("near"); err != nil {
				t.Fatalf("RemoveTask: %v", err)
			}
			wantFile(t, boardPath, "decision-near.md", false)
		}},
		{"PruneOfDoneEntryDeletesTheFile", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "near")
			done := "done"
			if err := b.SetStatus("near", &done); err != nil {
				t.Fatalf("SetStatus: %v", err)
			}
			wantFile(t, boardPath, "decision-near.md", true)
			if _, err := b.Prune(true); err != nil {
				t.Fatalf("Prune: %v", err)
			}
			wantFile(t, boardPath, "decision-near.md", false)
		}},
		{"MergeRemovingTheSlugDeletesTheFile", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "near")
			if _, _, err := b.MergeTasks([]string{"near"}, map[string]any{"slug": "merged", "title": "Merged", "labels": bugLabels}, nil); err != nil {
				t.Fatalf("MergeTasks: %v", err)
			}
			wantFile(t, boardPath, "decision-near.md", false)
		}},
		{"MergeUpsertingTheRemovedSlugAgainKeepsFileAndFlag", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "near")
			if _, _, err := b.MergeTasks([]string{"near"}, map[string]any{"slug": "near", "title": "Near again", "labels": bugLabels}, nil); err != nil {
				t.Fatalf("MergeTasks: %v", err)
			}
			wantFile(t, boardPath, "decision-near.md", true)
			if task, _, _ := b.GetTask("near"); !task.Discussed {
				t.Errorf("GetTask(near).Discussed = false; want the flag kept")
			}
		}},
		{"ClearingTheStatusKeepsFileAndFlag", func(t *testing.T, b *boardengine.Board, boardPath string) {
			store(t, b, "old")
			if err := b.SetStatus("old", nil); err != nil {
				t.Fatalf("SetStatus: %v", err)
			}
			wantFile(t, boardPath, "decision-old.md", true)
			if task, _, _ := b.GetTask("old"); !task.Discussed {
				t.Errorf("GetTask(old).Discussed = false; want the flag kept")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
			b, boardPath := newRenameBoard(t, &now)
			tt.run(t, b, boardPath)
		})
	}
}
