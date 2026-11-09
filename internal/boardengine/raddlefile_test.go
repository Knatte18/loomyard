// raddlefile_test.go — the board writes of a raddle run:
// FileRaddle's one write with the raddle store, and EndRaddleEntry.

package boardengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newRaddleTestBoard returns a board whose vocabulary carries the sketch, bug and enhancement type labels and the board label, and the board's directory.
func newRaddleTestBoard(t *testing.T) (*Board, string) {
	t.Helper()
	path := t.TempDir()
	types := []Label{{Name: SketchType}, {Name: "bug"}, {Name: "enhancement"}}
	return New(Config{Path: path, Readme: "Home.md", DesignPrefix: "proposal-", Types: types, Labels: []Label{{Name: "board"}}, SkipGit: true}), path
}

// upsertForRaddle upserts fields onto b, failing t on a refusal.
func upsertForRaddle(t *testing.T, b *Board, fields map[string]any) {
	t.Helper()
	if _, err := b.UpsertTask(fields); err != nil {
		t.Fatalf("UpsertTask(%v) error = %v; want nil", fields, err)
	}
}

// entryForRaddle returns the entry slug of b, failing t when it is absent.
func entryForRaddle(t *testing.T, b *Board, slug string) Task {
	t.Helper()
	task, found, err := b.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%q) = %v, %v; want the entry", slug, found, err)
	}
	return task
}

// startRaddleRun makes the Sketch slug, with its record, a raddle run's entry inside project, and returns the entry.
func startRaddleRun(t *testing.T, b *Board, slug, project string) Task {
	t.Helper()
	fields := map[string]any{"slug": slug, "title": "Plan", "kind": "note", "labels": []string{SketchType, "board"}}
	if project != "" {
		fields["project"] = project
	}
	upsertForRaddle(t, b, fields)
	if err := b.StoreDecisionRecord(slug, []byte("sketch record\n")); err != nil {
		t.Fatalf("StoreDecisionRecord() error = %v; want nil", err)
	}
	if _, err := b.PromoteTo(slug, shedrun.RecipeRaddle); err != nil {
		t.Fatalf("PromoteTo() error = %v; want nil", err)
	}
	return entryForRaddle(t, b, slug)
}

// raddlePart returns a filed task of recipe darn carrying the bug type label and the board label.
func raddlePart(slug string, dependsOn ...string) RaddleEntry {
	return RaddleEntry{Slug: slug, Title: slug, Kind: KindTask, Labels: []string{"bug", "board"}, Recipe: "darn", DependsOn: dependsOn, Body: RaddlePartBodyPrefix("plan") + ".\n\nbody of " + slug, Record: []byte("record of " + slug + "\n")}
}

// raddleProject returns the project a filing of the project or beam shape adds, with the type label enhancement.
func raddleProject(slug string) *RaddleEntry {
	return &RaddleEntry{Slug: slug, Title: slug, Kind: KindProject, Labels: []string{"enhancement", "board"}, Body: RaddleProjectBodyPrefix("plan") + ": body of " + slug + ".\n", Record: []byte("record of " + slug + "\n")}
}

// startArrasRun puts an arras run's entry slug on b at its Raddle-File row:
// a task with a run status, a body and a stored decision record.
func startArrasRun(t *testing.T, b *Board, slug string) Task {
	t.Helper()
	upsertForRaddle(t, b, map[string]any{"slug": slug, "title": "Arras", "kind": "task", "labels": []string{"bug", "board"}, "body": "former body", "recipe": shedrun.RecipeArras, "status": RunStatus("running", RaddleFileProducer)})
	if err := b.StoreDecisionRecord(slug, []byte("arras record\n")); err != nil {
		t.Fatalf("StoreDecisionRecord() error = %v; want nil", err)
	}
	return entryForRaddle(t, b, slug)
}

// familyFiling returns the family filing of the arras run plan:
// its project is the run's entry, and plan-b depends on plan-a.
func familyFiling(run Task) RaddleFiling {
	return RaddleFiling{
		Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", ProjectEntry: raddleProject("plan"),
		Parts:    []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b", "plan-a"), raddlePart("plan-c")},
		Proposal: []byte("proposal\n"), ColdProposal: []byte("cold\n"), SketchRecord: []byte("sketch record\n"),
	}
}

// backdateBoardFile sets board.json's modification time an hour back and returns it, so a later write shows as a newer time.
func backdateBoardFile(t *testing.T, boardPath string) time.Time {
	t.Helper()
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(boardPath, boardFile), old, old); err != nil {
		t.Fatalf("Chtimes(board.json) = %v; want nil", err)
	}
	return old
}

// boardFileWritten reports whether board.json was written after backdateBoardFile set its time to old.
func boardFileWritten(t *testing.T, boardPath string, old time.Time) bool {
	t.Helper()
	info, err := os.Stat(filepath.Join(boardPath, boardFile))
	if err != nil {
		t.Fatalf("Stat(board.json) = %v; want nil", err)
	}
	return !info.ModTime().Equal(old)
}

func TestFileRaddle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// groupingProject, when set, is a project the Sketch lives in.
		groupingProject string
		projectEntry    *RaddleEntry
		parts           []RaddleEntry
		// wantProjectOf maps each filed slug to the project it belongs to.
		wantProjectOf map[string]string
		wantDependsOn map[string][]string
		// setup, when set, adds entries to the board once the run's entry stands.
		setup func(t *testing.T, b *Board)
	}{
		{
			name: "TapestryShapeFilesOneDraftTask", parts: []RaddleEntry{raddlePart("plan-t")},
			wantProjectOf: map[string]string{"plan-t": ""},
		},
		{
			name:          "TasksShapeFilesDraftPartsAndDropsAGoneDependency",
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b", "plan-a", "gone", "plan")},
			wantProjectOf: map[string]string{"plan-a": "", "plan-b": ""},
			wantDependsOn: map[string][]string{"plan-b": {"plan-a"}},
		},
		{
			name:          "TasksShapeMovesOpenDependentsOntoTheSinkPartAndLeavesDoneOnes",
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b", "plan-a")},
			wantProjectOf: map[string]string{"plan-a": "", "plan-b": ""},
			wantDependsOn: map[string][]string{"waiter": {"other", "plan-b"}, "finished": {"plan"}},
			setup: func(t *testing.T, b *Board) {
				upsertForRaddle(t, b, map[string]any{"slug": "other", "title": "Other", "kind": "task", "labels": []string{"bug"}})
				upsertForRaddle(t, b, map[string]any{"slug": "waiter", "title": "Waiter", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{"other", "plan"}})
				upsertForRaddle(t, b, map[string]any{"slug": "finished", "title": "Finished", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{"plan"}})
				done := StatusDone
				if err := b.SetStatus("finished", &done); err != nil {
					t.Fatalf("SetStatus() error = %v; want nil", err)
				}
			},
		},
		{
			name:          "ProjectShapeFilesTheProjectAndDraftParts",
			projectEntry:  raddleProject("plan-proj"),
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b")},
			wantProjectOf: map[string]string{"plan-proj": "", "plan-a": "plan-proj", "plan-b": "plan-proj"},
		},
		{
			name:            "SketchInAGroupingProjectFilesTasksInThatProject",
			groupingProject: "grouping",
			parts:           []RaddleEntry{raddlePart("plan-a")},
			wantProjectOf:   map[string]string{"plan-a": "grouping"},
		},
		{
			name:            "SketchInAGroupingProjectFilesTheNewProjectInThatProject",
			groupingProject: "grouping",
			projectEntry:    raddleProject("plan-proj"),
			parts:           []RaddleEntry{raddlePart("plan-a")},
			wantProjectOf:   map[string]string{"plan-proj": "grouping", "plan-a": "plan-proj"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, boardPath := newRaddleTestBoard(t)
			if tt.groupingProject != "" {
				upsertForRaddle(t, b, map[string]any{"slug": tt.groupingProject, "title": "Grouping", "kind": "project", "labels": []string{"bug"}})
			}
			run := startRaddleRun(t, b, "plan", tt.groupingProject)
			if tt.setup != nil {
				tt.setup(t, b)
			}
			checks := 0
			filing := RaddleFiling{
				Sketch: "plan", Created: run.Created, Project: tt.groupingProject, Hash: "hash-1", RunEntry: "plan",
				ProjectEntry: tt.projectEntry, Parts: tt.parts,
				Proposal: []byte("proposal\n"), ColdProposal: []byte("cold\n"), SketchRecord: []byte("sketch record\n"),
				Check: func([]Task, RaddleStore) error { checks++; return nil },
			}

			if err := b.FileRaddle(filing); err != nil {
				t.Fatalf("FileRaddle() error = %v; want nil", err)
			}

			var wantFiled []string
			for slug, wantProject := range tt.wantProjectOf {
				wantFiled = append(wantFiled, slug)
				entry := entryForRaddle(t, b, slug)
				if entry.Project != wantProject || !entry.Discussed {
					t.Errorf("entry %q project, discussed = %q, %t; want %q, true", slug, entry.Project, entry.Discussed, wantProject)
				}
				if record, held, err := b.DecisionRecord(slug); err != nil || !held || string(record) != "record of "+slug+"\n" {
					t.Errorf("DecisionRecord(%q) = %q, %t, %v; want the filed record", slug, record, held, err)
				}
				if slices.Contains(entry.Labels, SketchType) {
					t.Errorf("entry %q labels = %v; want no sketch label", slug, entry.Labels)
				}
				if entry.Kind == KindProject && !slices.Contains(entry.Labels, "enhancement") {
					t.Errorf("project %q labels = %v; want the proposal's type label enhancement", slug, entry.Labels)
				}
				if entry.Kind == KindTask && (entry.Status == nil || *entry.Status != StatusDraft) {
					t.Errorf("entry %q status = %v; want draft", slug, entry.Status)
				}
			}
			for slug, wantDeps := range tt.wantDependsOn {
				if got := entryForRaddle(t, b, slug).DependsOn; !slices.Equal(got, wantDeps) {
					t.Errorf("entry %q depends_on = %v; want %v", slug, got, wantDeps)
				}
			}
			if got := entryForRaddle(t, b, "plan"); got.Status != run.Status {
				t.Errorf("run entry status = %v; want it left as it was, open", got.Status)
			}

			store, err := ReadRaddleStore(boardPath)
			if err != nil {
				t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
			}
			held, isHeld := store.Holds("plan")
			if !isHeld || held.Hash != "hash-1" || !held.Created.Equal(run.Created) || len(held.Filed) != len(wantFiled) {
				t.Errorf("store entry = %+v, held %t; want hash-1, the Sketch's created stamp and %d filed slugs", held, isHeld, len(wantFiled))
			}
			for _, slug := range wantFiled {
				if !slices.Contains(held.Filed, slug) {
					t.Errorf("store filed = %v; want it to hold %q", held.Filed, slug)
				}
			}
			wantProject := ""
			if tt.projectEntry != nil {
				wantProject = tt.projectEntry.Slug
			}
			if held.Project != wantProject {
				t.Errorf("store project = %q; want %q", held.Project, wantProject)
			}
			for name, want := range map[string]string{raddleProposalFileName: "proposal\n", raddleColdProposalFileName: "cold\n", raddleRecordFileName: "sketch record\n"} {
				if got, err := os.ReadFile(filepath.Join(RaddleStoreDir(boardPath, "plan"), name)); err != nil || string(got) != want {
					t.Errorf("store file %s = %q, %v; want %q", name, got, err, want)
				}
			}

			old := backdateBoardFile(t, boardPath)
			if err := b.FileRaddle(filing); err != nil {
				t.Fatalf("second FileRaddle() error = %v; want nil", err)
			}
			if boardFileWritten(t, boardPath, old) || checks != 1 {
				t.Errorf("second FileRaddle with the same hash wrote board.json = %t, ran the check %d times in all; want no write and the one check of the first call", boardFileWritten(t, boardPath, old), checks)
			}
		})
	}

	t.Run("ARetryAfterALostStoreWriteWritesTheStoreWithoutTheCheck", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newRaddleTestBoard(t)
		run := startRaddleRun(t, b, "plan", "")
		checks := 0
		filing := RaddleFiling{
			Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")},
			Proposal: []byte("proposal\n"), ColdProposal: []byte("cold\n"), SketchRecord: []byte("sketch record\n"),
			Check: func([]Task, RaddleStore) error {
				checks++
				if checks > 1 {
					return errors.New("the filed slug counts as taken")
				}
				return nil
			},
		}
		if err := b.FileRaddle(filing); err != nil {
			t.Fatalf("FileRaddle() error = %v; want nil", err)
		}
		// Removing the store directory leaves what a crash between the board save and the store write leaves:
		// filed entries and no done marker.
		if err := os.RemoveAll(RaddleStoreDir(boardPath, "plan")); err != nil {
			t.Fatalf("RemoveAll(store directory) = %v; want nil", err)
		}

		if err := b.FileRaddle(filing); err != nil {
			t.Fatalf("retried FileRaddle() error = %v; want nil", err)
		}

		if checks != 1 {
			t.Errorf("check ran %d times in all; want the one of the first call", checks)
		}
		store, err := ReadRaddleStore(boardPath)
		if err != nil {
			t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
		}
		held, isHeld := store.Holds("plan")
		if !isHeld || held.Hash != "hash-1" || !slices.Equal(held.Filed, []string{"plan-a"}) {
			t.Errorf("store entry = %+v, held %t; want hash-1 filing plan-a, the done marker the retry wrote", held, isHeld)
		}
		if entry := entryForRaddle(t, b, "plan-a"); !entry.Discussed {
			t.Errorf("entry plan-a discussed = false; want the filed entry kept")
		}
	})

	t.Run("ADifferentHashOrCreatedStampIsNotTheDoneMarker", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			hash         string
			createdShift time.Duration
		}{
			{"Hash", "hash-2", 0},
			{"CreatedStamp", "hash-1", time.Hour},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b, boardPath := newRaddleTestBoard(t)
				run := startRaddleRun(t, b, "plan", "")
				filing := RaddleFiling{
					Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")},
					Proposal: []byte("proposal\n"), ColdProposal: []byte("cold\n"), SketchRecord: []byte("sketch record\n"),
				}
				if err := b.FileRaddle(filing); err != nil {
					t.Fatalf("FileRaddle() error = %v; want nil", err)
				}

				filing.Hash, filing.Created = tt.hash, run.Created.Add(tt.createdShift)
				if err := b.FileRaddle(filing); err != nil {
					t.Fatalf("second FileRaddle() error = %v; want nil", err)
				}

				store, err := ReadRaddleStore(boardPath)
				if err != nil {
					t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
				}
				held, isHeld := store.Holds("plan")
				if !isHeld || held.Hash != tt.hash || !held.Created.Equal(filing.Created) {
					t.Errorf("store entry = %+v, held %t; want the second filing's hash %q and created stamp, since it is no done marker", held, isHeld, tt.hash)
				}
			})
		}
	})

	t.Run("CheckRefusalWritesNothingAndMatchesErrRaddleRecheck", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newRaddleTestBoard(t)
		run := startRaddleRun(t, b, "plan", "")
		filing := RaddleFiling{
			Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")},
			Proposal: []byte("proposal\n"), SketchRecord: []byte("sketch record\n"),
			Check: func([]Task, RaddleStore) error { return errors.New("another entry took the slug") },
		}

		err := b.FileRaddle(filing)
		if !errors.Is(err, ErrRaddleRecheck) {
			t.Fatalf("FileRaddle() error = %v; want it to match ErrRaddleRecheck", err)
		}
		if _, found, _ := b.GetTask("plan-a"); found {
			t.Errorf("a refused filing left the entry plan-a on the board")
		}
		if _, err := os.Stat(RaddleStoreDir(boardPath, "plan")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(store directory) error = %v; want it absent", err)
		}
		if _, err := os.Stat(filepath.Join(boardPath, DecisionFileName("plan-a"))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(decision record of plan-a) error = %v; want it absent", err)
		}
	})

	t.Run("AFamilyFilingTurnsTheRunEntryIntoTheProjectAndRepointsItsDependents", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newRaddleTestBoard(t)
		run := startArrasRun(t, b, "plan")
		upsertForRaddle(t, b, map[string]any{"slug": "other", "title": "Other", "kind": "task", "labels": []string{"bug"}})
		upsertForRaddle(t, b, map[string]any{"slug": "waiter", "title": "Waiter", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{"other", "plan"}})

		if err := b.FileRaddle(familyFiling(run)); err != nil {
			t.Fatalf("FileRaddle() error = %v; want nil", err)
		}

		parts := []string{"plan-a", "plan-b", "plan-c"}
		project := entryForRaddle(t, b, "plan")
		if project.Kind != KindProject || project.Status != nil || !project.Discussed || project.Recipe != "" || len(project.DependsOn) != 0 {
			t.Errorf("run entry = %+v; want a discussed project with no status, recipe or depends_on", project)
		}
		if project.Title != run.Title || !slices.Equal(project.Labels, run.Labels) {
			t.Errorf("run entry title, labels = %q, %v; want %q, %v kept", project.Title, project.Labels, run.Title, run.Labels)
		}
		wantBody := RaddleProjectBodyPrefix("plan") + ": plan-a, plan-b, plan-c.\n\nformer body"
		if !slices.Equal(project.Order, parts) || project.Body != wantBody {
			t.Errorf("run entry order, body = %v, %q; want %v, %q", project.Order, project.Body, parts, wantBody)
		}
		if record, held, err := b.DecisionRecord("plan"); err != nil || !held || string(record) != "record of plan\n" {
			t.Errorf("DecisionRecord(plan) = %q, %t, %v; want the whole record the filing stored", record, held, err)
		}
		for _, slug := range parts {
			if part := entryForRaddle(t, b, slug); part.Project != "plan" || part.Status == nil || *part.Status != StatusDraft {
				t.Errorf("part %q project, status = %q, %v; want plan, draft", slug, part.Project, part.Status)
			}
		}
		if got := entryForRaddle(t, b, "plan-b").DependsOn; !slices.Equal(got, []string{"plan-a"}) {
			t.Errorf("plan-b depends_on = %v; want [plan-a]", got)
		}
		if got := entryForRaddle(t, b, "waiter").DependsOn; !slices.Equal(got, []string{"other", "plan-b", "plan-c"}) {
			t.Errorf("waiter depends_on = %v; want [other plan-b plan-c], the family's last parts in place of plan", got)
		}
		store, err := ReadRaddleStore(boardPath)
		if err != nil {
			t.Fatalf("ReadRaddleStore() error = %v; want nil", err)
		}
		if held, isHeld := store.Holds("plan"); !isHeld || held.Project != "plan" || !slices.Contains(held.Filed, "plan") {
			t.Errorf("store entry = %+v, held %t; want plan filed as the project", held, isHeld)
		}
		if held, err := b.HoldsRaddleFiling("plan", append([]string{"plan"}, parts...)); err != nil || !held {
			t.Errorf("HoldsRaddleFiling(plan) = %t, %v; want true, so a retry finds the filing as its own", held, err)
		}

		// A run status the run wrote back onto the project is cleared by the repair.
		if err := b.SetRunStatus("plan", RunStatus("running", RaddleFileProducer)); err != nil {
			t.Fatalf("SetRunStatus() error = %v; want nil", err)
		}
		if err := b.ClearProjectRunStatus("plan"); err != nil {
			t.Fatalf("ClearProjectRunStatus() error = %v; want nil", err)
		}
		if got := entryForRaddle(t, b, "plan"); got.Status != nil {
			t.Errorf("project status = %q; want none", *got.Status)
		}

		// The clear writes nothing for an absent entry, a task that holds a run status and a project with none.
		otherStatus := RunStatus("running", "Webster")
		if err := b.SetRunStatus("other", otherStatus); err != nil {
			t.Fatalf("SetRunStatus(other) error = %v; want nil", err)
		}
		old := backdateBoardFile(t, boardPath)
		for _, slug := range []string{"gone", "other", "plan"} {
			if err := b.ClearProjectRunStatus(slug); err != nil {
				t.Errorf("ClearProjectRunStatus(%q) error = %v; want nil", slug, err)
			}
		}
		if boardFileWritten(t, boardPath, old) {
			t.Errorf("ClearProjectRunStatus of an absent entry, a task and a project with no status wrote board.json; want no write")
		}
		if other := entryForRaddle(t, b, "other"); other.Status == nil || *other.Status != otherStatus {
			t.Errorf("task other status = %v; want %q left as it was", other.Status, otherStatus)
		}

		// A null status written over a run status on the project releases no member, whichever write carries it.
		nullWrites := []struct {
			name  string
			write func() error
		}{
			{"SetStatus", func() error { return b.SetStatus("plan", nil) }},
			{"Upsert", func() error { _, err := b.UpsertTask(map[string]any{"slug": "plan", "status": nil}); return err }},
			{"Merge", func() error {
				_, _, err := b.MergeTasks(nil, map[string]any{"slug": "plan"}, &MergeStatusUpdate{Selector: "plan", Status: nil})
				return err
			}},
		}
		for _, nullWrite := range nullWrites {
			if err := b.SetRunStatus("plan", RunStatus("running", RaddleFileProducer)); err != nil {
				t.Fatalf("%s: SetRunStatus() error = %v; want nil", nullWrite.name, err)
			}
			if err := nullWrite.write(); err != nil {
				t.Fatalf("%s: null status write error = %v; want nil", nullWrite.name, err)
			}
			if got := entryForRaddle(t, b, "plan"); got.Status != nil {
				t.Errorf("%s: project status = %q; want none", nullWrite.name, *got.Status)
			}
			for _, slug := range parts {
				if part := entryForRaddle(t, b, slug); part.Status == nil || *part.Status != StatusDraft {
					t.Errorf("%s: part %q status = %v; want draft, released by no write but the project's own release", nullWrite.name, slug, part.Status)
				}
			}
		}
	})

	t.Run("ARepointTheBoardRefusesFilesNothingAndRestoresTheEntrysRecord", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			formerRecord bool
		}{
			{"WithAFormerRecord", true},
			{"WithNoFormerRecord", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b, boardPath := newRaddleTestBoard(t)
				run := startArrasRun(t, b, "plan")
				if !tt.formerRecord {
					if err := os.Remove(filepath.Join(boardPath, DecisionFileName("plan"))); err != nil {
						t.Fatalf("Remove(decision record of plan) = %v; want nil", err)
					}
				}
				upsertForRaddle(t, b, map[string]any{"slug": "waiter", "title": "Waiter", "kind": "task", "labels": []string{"bug"}, "depends_on": []string{"plan"}, "status": RunStatus("running", "Webster")})

				err := b.FileRaddle(familyFiling(run))

				if !errors.Is(err, ErrRaddleRepointRefused) || !errors.Is(err, ErrRunLocked) || !strings.Contains(err.Error(), `"waiter"`) {
					t.Fatalf("FileRaddle() error = %v; want the run lock refusal of waiter wrapped in ErrRaddleRepointRefused", err)
				}
				record, held, err := b.DecisionRecord("plan")
				if err != nil || held != tt.formerRecord || (held && string(record) != "arras record\n") {
					t.Errorf("DecisionRecord(plan) = %q, %t, %v; want the former record restored exactly when the entry had one (%t)", record, held, err, tt.formerRecord)
				}
				if entry := entryForRaddle(t, b, "plan"); entry.Kind != KindTask || entry.Status == nil || *entry.Status != *run.Status {
					t.Errorf("run entry = %+v; want the task as it was", entry)
				}
				if _, found, _ := b.GetTask("plan-a"); found {
					t.Errorf("a refused filing left the entry plan-a on the board")
				}
				if _, err := os.Stat(filepath.Join(boardPath, DecisionFileName("plan-a"))); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Stat(decision record of plan-a) error = %v; want it absent", err)
				}
			})
		}
	})

	t.Run("ARepointOntoASinkTheBoardDoesNotHoldIsRefusedNamingTheEntry", func(t *testing.T) {
		t.Parallel()
		store := NewStore("")
		store.tasks = []Task{
			{Slug: "plan", Title: "Plan", Kind: KindTask, Labels: []string{"bug"}},
			{Slug: "waiter", Title: "Waiter", Kind: KindTask, Labels: []string{"bug"}, DependsOn: []string{"plan"}},
		}

		err := repointDependents(store, "plan", []string{"gone"})

		if !errors.Is(err, ErrRaddleRepointRefused) || !strings.Contains(err.Error(), `"waiter"`) {
			t.Errorf("repointDependents() error = %v; want the board's refusal of waiter wrapped in ErrRaddleRepointRefused", err)
		}
		if got := store.tasks[1].DependsOn; !slices.Equal(got, []string{"plan"}) {
			t.Errorf("waiter depends_on = %v; want [plan], left as it was", got)
		}
	})
}

// TestCheckRunLocks_FamilyProject pins the run lock's admission of a family filing's change of the run's entry.
// The lock's judgement of a write's net change is no public verb, and only FileRaddle adds discussed parts,
// so the refused shapes are built here as the entries before and after the write.
func TestCheckRunLocks_FamilyProject(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	draft := StatusDraft
	part := func(slug string, discussed bool, dependsOn ...string) Task {
		return Task{Slug: slug, Title: slug, Kind: KindTask, Labels: []string{"bug"}, Project: "arras", DependsOn: dependsOn, Discussed: discussed, Status: &draft}
	}
	inOrder := []Task{part("p1", true), part("p2", true, "p1")}
	ready := "ready"
	released := part("p1", true)
	released.Status = &ready
	tests := []struct {
		name     string
		producer string
		parts    []Task
		// loadedParts are parts the board already held before the write.
		loadedParts []Task
		editNow     func(now *Task)
		refused     bool
	}{
		{name: "TheDerivedProjectPasses", producer: RaddleFileProducer, parts: inOrder},
		{name: "AChangedTitleIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.Title = "Renamed" }, refused: true},
		{name: "AChangedDependsOnIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.DependsOn = []string{"dep"} }, refused: true},
		{name: "AnotherProducersStatusIsRefused", producer: "Raddle-Write", parts: inOrder, refused: true},
		{name: "PartsOutOfDependencyOrderAreRefused", producer: RaddleFileProducer, parts: []Task{part("p2", true, "p1"), part("p1", true)}, refused: true},
		{name: "AnUndiscussedPartIsRefused", producer: RaddleFileProducer, parts: []Task{part("p1", false), part("p2", true, "p1")}, refused: true},
		{name: "AReleasedPartIsRefused", producer: RaddleFileProducer, parts: []Task{released, part("p2", true, "p1")}, refused: true},
		{name: "APartTheBoardAlreadyHeldIsRefused", producer: RaddleFileProducer, parts: inOrder, loadedParts: []Task{part("p1", true)}, refused: true},
		{name: "NoAddedPartIsRefused", producer: RaddleFileProducer, refused: true},
		{name: "AnOrderOtherThanThePartsIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.Order = []string{"p2", "p1"} }, refused: true},
		{name: "AChangedBodyIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.Body = "another body" }, refused: true},
		{name: "AKeptProfileIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.Profile = "fast" }, refused: true},
		{name: "KeptRunSettingsAreRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.RunSettings = map[string]any{"key": "value"} }, refused: true},
		{name: "AKeptNotifyOnLandIsRefused", producer: RaddleFileProducer, parts: inOrder, editNow: func(now *Task) { now.NotifyOnLand = true }, refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status := RunStatus("running", tt.producer)
			before := Task{Slug: "arras", Title: "Arras", Kind: KindTask, Labels: []string{"bug"}, Body: "former body", Recipe: shedrun.RecipeArras, DependsOn: []string{"dep"}, Discussed: true, Status: &status}
			var slugs []string
			for _, p := range tt.parts {
				slugs = append(slugs, p.Slug)
			}
			now := arrasProjectOf(before, slugs)
			if tt.editNow != nil {
				tt.editNow(&now)
			}
			dep := Task{Slug: "dep", Title: "Dep", Kind: KindTask, Labels: []string{"bug"}}

			loaded := append([]Task{dep, before}, tt.loadedParts...)
			err := checkRunLocks(loaded, append([]Task{dep, now}, tt.parts...), Vocabulary{})

			if got := errors.Is(err, ErrRunLocked); got != tt.refused {
				t.Errorf("checkRunLocks() error = %v; want refused %t", err, tt.refused)
			}
		})
	}
}

func TestHoldsRaddleFiling(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	b, _ := newRaddleTestBoard(t)
	run := startRaddleRun(t, b, "plan", "")
	filing := RaddleFiling{
		Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", ProjectEntry: raddleProject("plan-proj"),
		Parts:    []RaddleEntry{raddlePart("plan-a")},
		Proposal: []byte("proposal\n"), ColdProposal: []byte("cold\n"), SketchRecord: []byte("sketch record\n"),
	}
	if err := b.FileRaddle(filing); err != nil {
		t.Fatalf("FileRaddle() error = %v; want nil", err)
	}

	tests := []struct {
		name   string
		sketch string
		slugs  []string
		want   bool
	}{
		{"EveryFiledEntry", "plan", []string{"plan-proj", "plan-a"}, true},
		{"AnEntryNotOnTheBoard", "plan", []string{"plan-a", "gone"}, false},
		{"AnotherSketch", "other", []string{"plan-a"}, false},
		{"TheRunsOwnEntry", "plan", []string{"plan"}, false},
		{"NoSlug", "plan", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := b.HoldsRaddleFiling(tt.sketch, tt.slugs)
			if err != nil || got != tt.want {
				t.Errorf("HoldsRaddleFiling(%q, %v) = %t, %v; want %t, nil", tt.sketch, tt.slugs, got, err, tt.want)
			}
		})
	}
}

func TestEndRaddleEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("OpenEntryEndsAndClearsNotifyOnLand", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newRaddleTestBoard(t)
		upsertForRaddle(t, b, map[string]any{"slug": "run", "title": "Run", "kind": "task", "labels": []string{"bug"}, "notify_on_land": true})
		old := backdateBoardFile(t, boardPath)

		if err := b.EndRaddleEntry("run"); err != nil {
			t.Fatalf("EndRaddleEntry() error = %v; want nil", err)
		}

		entry := entryForRaddle(t, b, "run")
		if entry.Status == nil || *entry.Status != StatusDone || entry.NotifyOnLand {
			t.Errorf("entry status, notify_on_land = %v, %t; want done, false", entry.Status, entry.NotifyOnLand)
		}
		if !boardFileWritten(t, boardPath, old) {
			t.Errorf("EndRaddleEntry did not write board.json; want one write")
		}
	})

	t.Run("AbsentAndDoneEntriesWriteNothing", func(t *testing.T) {
		t.Parallel()
		b, boardPath := newRaddleTestBoard(t)
		upsertForRaddle(t, b, map[string]any{"slug": "landed", "title": "Landed", "kind": "task", "labels": []string{"bug"}, "notify_on_land": true})
		done := StatusDone
		if err := b.SetStatus("landed", &done); err != nil {
			t.Fatalf("SetStatus() error = %v; want nil", err)
		}
		old := backdateBoardFile(t, boardPath)

		for _, slug := range []string{"absent", "landed"} {
			if err := b.EndRaddleEntry(slug); err != nil {
				t.Errorf("EndRaddleEntry(%q) error = %v; want nil", slug, err)
			}
		}

		if boardFileWritten(t, boardPath, old) {
			t.Errorf("EndRaddleEntry of an absent or a done entry wrote board.json; want no write")
		}
		if entry := entryForRaddle(t, b, "landed"); !entry.NotifyOnLand {
			t.Errorf("done entry notify_on_land = false; want it left as it was")
		}
	})
}
