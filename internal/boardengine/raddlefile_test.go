// raddlefile_test.go — the board writes of a raddle run: FileRaddle's one write with the raddle store, ReleaseTasks and EndRaddleEntry.

package boardengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
		release         bool
		// wantProjectOf maps each filed slug to the project it belongs to.
		wantProjectOf map[string]string
		wantDraft     []string
		wantDependsOn map[string][]string
	}{
		{
			name: "TapestryShapeFilesOneReleasedTask", parts: []RaddleEntry{raddlePart("plan-t")}, release: true,
			wantProjectOf: map[string]string{"plan-t": ""},
		},
		{
			name:          "TasksShapeFilesReleasedPartsAndDropsAGoneDependency",
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b", "plan-a", "gone", "plan")},
			release:       true,
			wantProjectOf: map[string]string{"plan-a": "", "plan-b": ""},
			wantDependsOn: map[string][]string{"plan-b": {"plan-a"}},
		},
		{
			name:          "ProjectShapeFilesTheProjectAndReleasedParts",
			projectEntry:  raddleProject("plan-proj"),
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b")},
			release:       true,
			wantProjectOf: map[string]string{"plan-proj": "", "plan-a": "plan-proj", "plan-b": "plan-proj"},
		},
		{
			name:          "BeamShapeFilesTheProjectAndDraftParts",
			projectEntry:  raddleProject("plan-proj"),
			parts:         []RaddleEntry{raddlePart("plan-a"), raddlePart("plan-b")},
			wantProjectOf: map[string]string{"plan-proj": "", "plan-a": "plan-proj", "plan-b": "plan-proj"},
			wantDraft:     []string{"plan-a", "plan-b"},
		},
		{
			name:            "SketchInAGroupingProjectFilesTasksInThatProject",
			groupingProject: "grouping",
			parts:           []RaddleEntry{raddlePart("plan-a")},
			release:         true,
			wantProjectOf:   map[string]string{"plan-a": "grouping"},
		},
		{
			name:            "SketchInAGroupingProjectFilesTheNewProjectInThatProject",
			groupingProject: "grouping",
			projectEntry:    raddleProject("plan-proj"),
			parts:           []RaddleEntry{raddlePart("plan-a")},
			release:         true,
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
			checks := 0
			filing := RaddleFiling{
				Sketch: "plan", Created: run.Created, Project: tt.groupingProject, Hash: "hash-1", RunEntry: "plan",
				ProjectEntry: tt.projectEntry, Parts: tt.parts, Release: tt.release,
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
				if entry.Kind == KindTask {
					draft := entry.Status != nil && *entry.Status == StatusDraft
					if draft != slices.Contains(tt.wantDraft, slug) || (!draft && entry.Status != nil) {
						t.Errorf("entry %q status = %v; want draft = %t, otherwise released", slug, entry.Status, slices.Contains(tt.wantDraft, slug))
					}
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
			Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")}, Release: true,
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
					Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")}, Release: true,
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
			Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", Parts: []RaddleEntry{raddlePart("plan-a")}, Release: true,
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
}

func TestHoldsRaddleFiling(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	b, _ := newRaddleTestBoard(t)
	run := startRaddleRun(t, b, "plan", "")
	filing := RaddleFiling{
		Sketch: "plan", Created: run.Created, Hash: "hash-1", RunEntry: "plan", ProjectEntry: raddleProject("plan-proj"),
		Parts: []RaddleEntry{raddlePart("plan-a")}, Release: true,
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

func TestReleaseTasks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		slugs []string
		// wantReleased lists the slugs that read released afterwards, and wantWrite whether the call writes board.json.
		wantReleased []string
		wantWrite    bool
	}{
		{"TwoDraftPartsAreReleased", []string{"part-a", "part-b"}, []string{"part-a", "part-b"}, true},
		{"AbsentAndDoneAreSkipped", []string{"gone", "part-done", "part-b"}, []string{"part-b"}, true},
		{"NoneLeftWritesNothing", []string{"gone", "part-done"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, boardPath := newRaddleTestBoard(t)
			for _, slug := range []string{"part-a", "part-b", "part-done"} {
				upsertForRaddle(t, b, map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}})
			}
			done := StatusDone
			if err := b.SetStatus("part-done", &done); err != nil {
				t.Fatalf("SetStatus() error = %v; want nil", err)
			}
			old := backdateBoardFile(t, boardPath)

			if err := b.ReleaseTasks(tt.slugs); err != nil {
				t.Fatalf("ReleaseTasks() error = %v; want nil", err)
			}

			if written := boardFileWritten(t, boardPath, old); written != tt.wantWrite {
				t.Errorf("ReleaseTasks wrote board.json = %t; want %t", written, tt.wantWrite)
			}
			for _, slug := range []string{"part-a", "part-b"} {
				entry := entryForRaddle(t, b, slug)
				if released := entry.Status == nil; released != slices.Contains(tt.wantReleased, slug) {
					t.Errorf("entry %q status = %v; want released = %t", slug, entry.Status, !released)
				}
			}
			if entry := entryForRaddle(t, b, "part-done"); entry.Status == nil || *entry.Status != StatusDone {
				t.Errorf("done entry status = %v; want it left done", entry.Status)
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
