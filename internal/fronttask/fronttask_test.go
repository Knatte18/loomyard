// fronttask_test.go pins filing, removing and settling a front task over a fake board and a temp record file.

package fronttask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeBoard is a board of entries with a call log, standing in for the hub's board and the batten cancel.
type fakeBoard struct {
	mu         sync.Mutex
	tasks      []boardengine.Task
	calls      []string
	hasRun     bool
	cancelErr  error
	releaseErr error
	removeErr  error
	// onRemove runs inside RemoveEntry, before the entry is dropped.
	onRemove func()
}

func statusOf(status string) *string { return &status }

func (f *fakeBoard) log(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeBoard) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeBoard) deps(recordPath string) Deps {
	return Deps{
		Tasks: func() ([]boardengine.Task, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(f.tasks), nil
		},
		Create: func(fields map[string]any, onLand string) (boardengine.Task, error) {
			f.log("create")
			f.mu.Lock()
			defer f.mu.Unlock()
			project, _ := fields["project"].(string)
			task := boardengine.Task{Slug: fields["slug"].(string), Kind: boardengine.KindTask, Front: true, OnLand: onLand, Project: project, Status: statusOf(boardengine.StatusDraft)}
			f.tasks = append(f.tasks, task)
			return task, nil
		},
		Release: func(slug string) error {
			f.log("release")
			if f.releaseErr != nil {
				return f.releaseErr
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			for i := range f.tasks {
				if f.tasks[i].Slug == slug {
					f.tasks[i].Status = nil
				}
			}
			return nil
		},
		RemoveEntry: func(slug string) error {
			f.log("remove-entry")
			if f.onRemove != nil {
				f.onRemove()
			}
			if f.removeErr != nil {
				return f.removeErr
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.tasks = slices.DeleteFunc(f.tasks, func(task boardengine.Task) bool { return task.Slug == slug })
			return nil
		},
		HasRun: func(string) (bool, error) { return f.hasRun, nil },
		Cancel: func(context.Context, string) error {
			f.log("cancel")
			return f.cancelErr
		},
		RecordPath: recordPath,
	}
}

func recordPathIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "records.json")
}

func readRecords(t *testing.T, path string) map[string]RecordEntry {
	t.Helper()
	read, _, err := state.ReadJSON[records](path, lockPathOf(path))
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	return read.Entries
}

func seedRecords(t *testing.T, path string, entries map[string]RecordEntry) {
	t.Helper()
	if err := state.WriteJSON(path, lockPathOf(path), records{Entries: entries}); err != nil {
		t.Fatalf("seed the record: %v", err)
	}
}

func frontTask(slug, project, onLand string, status *string) boardengine.Task {
	return boardengine.Task{Slug: slug, Kind: boardengine.KindTask, Front: true, OnLand: onLand, Project: project, Status: status}
}

func TestFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		existing   []boardengine.Task
		onLand     string
		betweenErr error
		releaseErr error
		// brokenRecord puts the record file under a regular file, so its write fails.
		brokenRecord bool
		wantErr      []string
		wantCalls    []string
		wantRecords  map[string]RecordEntry
	}{
		{
			name:        "CreateBetweenReleaseRecordInOrder",
			onLand:      ActionAdoptCheck,
			wantCalls:   []string{"create", "between", "release"},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck, Project: "beam"}},
		},
		{
			name:      "OpenFrontInProjectRefusesBeforeAnyWrite",
			existing:  []boardengine.Task{frontTask("older", "beam", ActionAdoptCheck, nil)},
			onLand:    ActionAdoptCheck,
			wantErr:   []string{"older", "one front task stands open per worktree", "lym batten cancel older --remove", "lym board remove older"},
			wantCalls: nil,
		},
		{
			name:        "DraftFrontDoesNotRefuse",
			existing:    []boardengine.Task{frontTask("older", "beam", ActionAdoptCheck, statusOf(boardengine.StatusDraft))},
			onLand:      ActionAdoptCheck,
			wantCalls:   []string{"create", "between", "release"},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck, Project: "beam"}},
		},
		{
			name:        "DoneFrontDoesNotRefuse",
			existing:    []boardengine.Task{frontTask("older", "beam", ActionAdoptCheck, statusOf("done"))},
			onLand:      ActionAdoptCheck,
			wantCalls:   []string{"create", "between", "release"},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck, Project: "beam"}},
		},
		{
			name:        "OpenFrontOfAnotherProjectDoesNotRefuse",
			existing:    []boardengine.Task{frontTask("older", "", ActionAdoptCheck, nil)},
			onLand:      ActionAdoptCheck,
			wantCalls:   []string{"create", "between", "release"},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck, Project: "beam"}},
		},
		{
			name:      "UnknownActionRefusesBeforeAnyWrite",
			onLand:    "nonsense",
			wantErr:   []string{`"nonsense"`, ActionAdoptCheck},
			wantCalls: nil,
		},
		{
			name:       "BetweenErrorLeavesDraftUnreleasedAndUnrecorded",
			onLand:     ActionAdoptCheck,
			betweenErr: errors.New("between broke"),
			wantErr:    []string{"between broke"},
			wantCalls:  []string{"create", "between"},
		},
		{
			name:       "ReleaseErrorLeavesDraftUnrecorded",
			onLand:     ActionAdoptCheck,
			releaseErr: errors.New("release broke"),
			wantErr:    []string{"release broke"},
			wantCalls:  []string{"create", "between", "release"},
		},
		{
			name:         "RecordWriteErrorAfterReleaseStillFiles",
			onLand:       ActionAdoptCheck,
			brokenRecord: true,
			wantCalls:    []string{"create", "between", "release"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := recordPathIn(t)
			if tt.brokenRecord {
				blocker := filepath.Join(t.TempDir(), "blocker")
				if err := os.WriteFile(blocker, nil, 0o644); err != nil {
					t.Fatalf("write the blocker file: %v", err)
				}
				path = filepath.Join(blocker, "records.json")
			}
			board := &fakeBoard{tasks: slices.Clone(tt.existing), releaseErr: tt.releaseErr}
			between := func() error {
				board.log("between")
				return tt.betweenErr
			}
			err := File(board.deps(path), Request{Slug: "fix", Title: "fix", Project: "beam", OnLand: tt.onLand}, between)
			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("File() = %v; want nil", err)
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("File() = %v; want an error containing %q", err, want)
				}
			}
			if got := board.callLog(); !slices.Equal(got, tt.wantCalls) {
				t.Errorf("File() board calls = %v; want %v", got, tt.wantCalls)
			}
			if tt.brokenRecord {
				return
			}
			got := readRecords(t, path)
			if len(got) != len(tt.wantRecords) {
				t.Fatalf("File() records = %v; want %v", got, tt.wantRecords)
			}
			for slug, want := range tt.wantRecords {
				if got[slug] != want {
					t.Errorf("File() record of %s = %+v; want %+v", slug, got[slug], want)
				}
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name          string
		hasRun        bool
		cancelErr     error
		removeErr     error
		wantStep      string
		wantHand      string
		wantCalls     []string
		wantRecord    bool
		wantEntryKept bool
	}{
		{
			name:      "NoRunSkipsTheCancel",
			wantCalls: []string{"remove-entry"},
		},
		{
			name:      "NothingLeftToCancelCountsDone",
			hasRun:    true,
			cancelErr: fmt.Errorf("cancel: %w", battencli.ErrNothingLeft),
			wantCalls: []string{"cancel", "remove-entry"},
		},
		{
			name:          "CancelErrorStopsBeforeRecordAndEntry",
			hasRun:        true,
			cancelErr:     errors.New("the cancel could not reach origin"),
			wantStep:      "cancel",
			wantHand:      "`lym batten cancel fix --remove`, then `lym board remove fix`",
			wantCalls:     []string{"cancel"},
			wantRecord:    true,
			wantEntryKept: true,
		},
		{
			name:          "EntryErrorNamesTheBoardRemovalAlone",
			removeErr:     errors.New("the board refused"),
			wantStep:      "entry",
			wantHand:      "`lym board remove fix`",
			wantCalls:     []string{"remove-entry"},
			wantEntryKept: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := recordPathIn(t)
			seedRecords(t, path, map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}})
			board := &fakeBoard{tasks: []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, nil)}, hasRun: tt.hasRun, cancelErr: tt.cancelErr, removeErr: tt.removeErr}

			err := Remove(board.deps(path), "fix")
			if tt.wantStep == "" {
				if err != nil {
					t.Fatalf("Remove() = %v; want nil", err)
				}
			} else {
				var removal *RemoveError
				if !errors.As(err, &removal) || removal.Step != tt.wantStep || removal.Slug != "fix" {
					t.Fatalf("Remove() = %v; want a *RemoveError of fix at step %s", err, tt.wantStep)
				}
				if !strings.Contains(removal.HandRemoval(), tt.wantHand) || !strings.Contains(err.Error(), tt.wantHand) {
					t.Errorf("hand removal = %q, error = %q; want both to name %s", removal.HandRemoval(), err, tt.wantHand)
				}
			}
			if got := board.callLog(); !slices.Equal(got, tt.wantCalls) {
				t.Errorf("Remove() board calls = %v; want %v", got, tt.wantCalls)
			}
			if _, recorded := readRecords(t, path)["fix"]; recorded != tt.wantRecord {
				t.Errorf("Remove() record held = %v; want %v", recorded, tt.wantRecord)
			}
			tasks, _ := board.deps(path).Tasks()
			if kept := len(tasks) == 1; kept != tt.wantEntryKept {
				t.Errorf("Remove() entry kept = %v; want %v", kept, tt.wantEntryKept)
			}
		})
	}

	t.Run("RecordAndEntryRemovalHoldTheLockAgainstSettle", func(t *testing.T) {
		t.Parallel()
		path := recordPathIn(t)
		seedRecords(t, path, map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}})
		board := &fakeBoard{tasks: []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, nil)}}

		var settled sync.WaitGroup
		var mu sync.Mutex
		var actions []string
		settle := SettleDeps{
			Subject:    "root",
			Tasks:      board.deps(path).Tasks,
			OwnedHere:  func() (bool, error) { return true, nil },
			RecordPath: path,
			RunAction: func(action, _ string) error {
				mu.Lock()
				defer mu.Unlock()
				actions = append(actions, action)
				return nil
			},
		}
		board.onRemove = func() {
			held, free, err := lock.TryAcquireWriteLock(lockPathOf(path))
			if err != nil {
				t.Errorf("probe the record lock: %v", err)
			}
			if free {
				_ = held.Release()
				t.Error("the record lock was free while the entry was removed; want it held")
			}
			// The settle blocks on the held lock and reads the board only after the removal returned.
			settled.Add(1)
			go func() {
				defer settled.Done()
				if err := Settle(settle); err != nil {
					t.Errorf("Settle beside the removal = %v; want nil", err)
				}
			}()
		}

		if err := Remove(board.deps(path), "fix"); err != nil {
			t.Fatalf("Remove() = %v; want nil", err)
		}
		settled.Wait()
		if err := Settle(settle); err != nil {
			t.Fatalf("Settle after the removal = %v; want nil", err)
		}
		if got := readRecords(t, path); len(got) != 0 {
			t.Errorf("records after the removal and both settles = %v; want none", got)
		}
		if len(actions) != 0 {
			t.Errorf("actions run = %v; want none", actions)
		}
	})
}

func TestSettle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type step struct {
		runErr error
		// wantErr is a substring Settle's error must hold, empty for none.
		wantErr string
	}
	tests := []struct {
		name        string
		tasks       []boardengine.Task
		records     map[string]RecordEntry
		notOwned    bool
		steps       []step
		wantRuns    int
		wantRecords map[string]RecordEntry
	}{
		{
			name:        "OpenFrontWithNoRecordIsRecorded",
			tasks:       []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, nil)},
			steps:       []step{{}},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
		},
		{
			name:     "GoneEntryRunsItsActionOnceAcrossTwoCalls",
			records:  map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
			steps:    []step{{}, {}},
			wantRuns: 1,
		},
		{
			name:     "DoneEntryRunsItsActionOnceAcrossTwoCalls",
			tasks:    []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, statusOf("done"))},
			records:  map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
			steps:    []step{{}, {}},
			wantRuns: 1,
		},
		{
			name:        "HaltedEntryRunsNoAction",
			tasks:       []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, statusOf("halted · Run-Shed"))},
			records:     map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
			steps:       []step{{}},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
		},
		{
			name:        "DraftEntryRunsNoAction",
			tasks:       []boardengine.Task{frontTask("fix", "", ActionAdoptCheck, statusOf(boardengine.StatusDraft))},
			records:     map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
			steps:       []step{{}},
			wantRecords: map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
		},
		{
			name:    "UnknownActionRunsNothingAndDropsTheRecord",
			records: map[string]RecordEntry{"fix": {OnLand: "nonsense"}},
			steps:   []step{{}},
		},
		{
			name:        "FailedActionKeepsTheRecordAndRunsAgainNextCall",
			records:     map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}},
			steps:       []step{{runErr: errors.New("spawn broke"), wantErr: "lym adopt root"}, {}},
			wantRuns:    2,
			wantRecords: nil,
		},
		{
			name:        "NotOwnedHereDropsTheProjectsRecordsAndRunsNone",
			records:     map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}, "other": {OnLand: ActionAdoptCheck, Project: "elsewhere"}},
			notOwned:    true,
			steps:       []step{{}},
			wantRecords: map[string]RecordEntry{"other": {OnLand: ActionAdoptCheck, Project: "elsewhere"}},
		},
		{
			name:        "AnotherProjectsRecordIsUntouched",
			records:     map[string]RecordEntry{"fix": {OnLand: ActionAdoptCheck}, "other": {OnLand: ActionAdoptCheck, Project: "elsewhere"}},
			steps:       []step{{}},
			wantRuns:    1,
			wantRecords: map[string]RecordEntry{"other": {OnLand: ActionAdoptCheck, Project: "elsewhere"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := recordPathIn(t)
			if tt.records != nil {
				seedRecords(t, path, tt.records)
			}
			board := &fakeBoard{tasks: slices.Clone(tt.tasks)}
			runs := 0
			var runErr error
			deps := SettleDeps{
				Subject:    "root",
				Tasks:      board.deps(path).Tasks,
				OwnedHere:  func() (bool, error) { return !tt.notOwned, nil },
				RecordPath: path,
				RunAction: func(action, subject string) error {
					runs++
					if action != ActionAdoptCheck || subject != "root" {
						t.Errorf("RunAction(%q, %q); want (%q, %q)", action, subject, ActionAdoptCheck, "root")
					}
					return runErr
				},
			}
			for i, s := range tt.steps {
				runErr = s.runErr
				err := Settle(deps)
				switch {
				case s.wantErr == "" && err != nil:
					t.Fatalf("Settle() call %d = %v; want nil", i+1, err)
				case s.wantErr != "" && (err == nil || !strings.Contains(err.Error(), s.wantErr) || !strings.Contains(err.Error(), "fix")):
					t.Fatalf("Settle() call %d = %v; want an error naming fix and %q", i+1, err, s.wantErr)
				}
			}
			if runs != tt.wantRuns {
				t.Errorf("Settle() ran the action %d times; want %d", runs, tt.wantRuns)
			}
			got := readRecords(t, path)
			if len(got) != len(tt.wantRecords) {
				t.Fatalf("Settle() records = %v; want %v", got, tt.wantRecords)
			}
			for slug, want := range tt.wantRecords {
				if got[slug] != want {
					t.Errorf("Settle() record of %s = %+v; want %+v", slug, got[slug], want)
				}
			}
		})
	}
}
