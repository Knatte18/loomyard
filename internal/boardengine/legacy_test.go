// legacy_test.go — unit tests for the legacy migration and done-mark fold (legacy.go).

package boardengine

import (
	"slices"
	"strings"
	"testing"
)

func legacyRec(id int, slug, status string) legacyRecord {
	return legacyRecord{ID: id, Slug: slug, Title: slug, Status: status}
}

func entryBySlug(t *testing.T, entries []Task, slug string) Task {
	t.Helper()
	for _, e := range entries {
		if e.Slug == slug {
			return e
		}
	}
	t.Fatalf("no entry with slug %q", slug)
	return Task{}
}

func TestMigrateLegacyKindAndLabels(t *testing.T) {
	t.Parallel()
	tasks := []legacyRecord{
		{ID: 0, Slug: "plain", Type: "loom", Brief: "b", Body: "x", Isolated: true, ShortName: "pl", DependsOn: []string{"other"}},
		{ID: 1, Slug: "later", Deferred: true},
		{ID: 2, Slug: "prefixed", Brief: "[infra] [bug] real brief"},
	}
	notes := []legacyRecord{{ID: 5, Slug: "idea"}}

	entries, _, err := migrateLegacy(tasks, notes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	plain := entryBySlug(t, entries, "plain")
	if plain.Kind != KindTask || !slices.Equal(plain.Labels, []string{"enhancement"}) {
		t.Errorf("task: kind=%q labels=%v, want task [enhancement]", plain.Kind, plain.Labels)
	}
	if plain.Recipe != "loom" {
		t.Errorf("task: recipe=%q, want the legacy type loom", plain.Recipe)
	}
	if plain.ID != 0 || plain.Brief != "b" || plain.Body != "x" || !plain.Isolated || plain.ShortName != "pl" || !slices.Equal(plain.DependsOn, []string{"other"}) {
		t.Errorf("task fields not carried over: %+v", plain)
	}

	later := entryBySlug(t, entries, "later")
	if later.Kind != KindNote || !slices.Equal(later.Labels, []string{"enhancement"}) || later.ID != 1 {
		t.Errorf("deferred task: %+v, want note [enhancement] id 1", later)
	}

	idea := entryBySlug(t, entries, "idea")
	if idea.Kind != KindNote || !slices.Equal(idea.Labels, []string{"enhancement"}) || idea.ID != 5 {
		t.Errorf("note: %+v, want note [enhancement] id 5", idea)
	}

	prefixed := entryBySlug(t, entries, "prefixed")
	if !slices.Equal(prefixed.Labels, []string{"enhancement", "infra"}) || prefixed.Brief != "real brief" {
		t.Errorf("bracket prefixes: labels=%v brief=%q, want [enhancement infra] and the stripped brief", prefixed.Labels, prefixed.Brief)
	}
	for _, e := range entries {
		if err := validateTask(e); err != nil {
			t.Errorf("entry %q fails validation: %v", e.Slug, err)
		}
	}
}

//testtiming:keep pins the colliding note id reassignment, the legacy_done seed and the duplicate-slug remedy message, which its covering tests do not assert
func TestMigrateLegacyIDsDoneSeedAndDuplicateSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		tasks    []legacyRecord
		notes    []legacyRecord
		wantIDs  map[string]int
		wantDone []string
		// wantDoneStatus names the entries that must carry status done.
		wantDoneStatus []string
		wantErr        []string
	}{
		{
			name:    "colliding note id is reassigned above every id taken",
			tasks:   []legacyRecord{legacyRec(0, "a", ""), legacyRec(3, "b", "")},
			notes:   []legacyRecord{legacyRec(3, "n1", ""), legacyRec(4, "n2", "")},
			wantIDs: map[string]int{"b": 3, "n1": 5, "n2": 4},
		},
		{
			name:           "done records seed legacy_done and carry the status",
			tasks:          []legacyRecord{legacyRec(0, "t-done", "done"), legacyRec(1, "t-open", "in-progress")},
			notes:          []legacyRecord{legacyRec(2, "n-done", "done"), legacyRec(3, "n-open", "")},
			wantDone:       []string{"n-done", "t-done"},
			wantDoneStatus: []string{"t-done"},
		},
		{
			name:    "slug in both files is refused naming the slug and the remedy",
			tasks:   []legacyRecord{legacyRec(0, "dup", "")},
			notes:   []legacyRecord{legacyRec(1, "dup", "")},
			wantErr: []string{"dup", "remove the duplicate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries, legacyDone, err := migrateLegacy(tt.tasks, tt.notes, nil)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for slug, want := range tt.wantIDs {
				if got := entryBySlug(t, entries, slug).ID; got != want {
					t.Errorf("%s id = %d, want %d", slug, got, want)
				}
			}
			slices.Sort(legacyDone)
			if !slices.Equal(legacyDone, tt.wantDone) {
				t.Errorf("legacy_done = %v, want %v", legacyDone, tt.wantDone)
			}
			for _, slug := range tt.wantDoneStatus {
				if s := entryBySlug(t, entries, slug).Status; s == nil || *s != "done" {
					t.Errorf("%s: done status not carried over: %v", slug, s)
				}
			}
		})
	}
}

func TestFoldLegacyDone(t *testing.T) {
	t.Parallel()
	taskA := func() Task { return Task{ID: 0, Slug: "a", Kind: KindTask, Labels: []string{"enhancement"}} }
	tests := []struct {
		name string
		// setup returns the entries, legacy_done and legacy records the fold starts from.
		setup func(t *testing.T) ([]Task, []string, []legacyRecord)
		// rounds is how many times the fold's output is fed back into it.
		rounds int
		// wantStatus is the first entry's status after the fold; empty means nil.
		wantStatus string
		// wantGrown is legacy_done after the fold, checked only when checkGrown is set.
		wantGrown  []string
		checkGrown bool
	}{
		{
			name: "a done legacy record marks the entry once",
			setup: func(t *testing.T) ([]Task, []string, []legacyRecord) {
				entries, legacyDone, err := migrateLegacy([]legacyRecord{legacyRec(0, "a", "")}, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				return entries, legacyDone, []legacyRecord{legacyRec(0, "a", "done")}
			},
			rounds:     1,
			wantStatus: "done",
			wantGrown:  []string{"a"},
			checkGrown: true,
		},
		{
			name: "a done slug absent from the entries is only recorded",
			setup: func(t *testing.T) ([]Task, []string, []legacyRecord) {
				return []Task{taskA()}, nil, []legacyRecord{legacyRec(9, "gone", "done")}
			},
			rounds:     1,
			wantGrown:  []string{"gone"},
			checkGrown: true,
		},
		{
			name: "a status other than done folds nothing",
			setup: func(t *testing.T) ([]Task, []string, []legacyRecord) {
				return []Task{taskA()}, nil, []legacyRecord{legacyRec(0, "a", "in-progress")}
			},
			rounds:     1,
			checkGrown: true,
		},
		{
			name: "an entry reopened after migration stays reopened",
			setup: func(t *testing.T) ([]Task, []string, []legacyRecord) {
				legacy := []legacyRecord{legacyRec(0, "a", "done")}
				entries, legacyDone, err := migrateLegacy(legacy, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				reopened := "in-progress"
				entries[0].Status = &reopened
				return entries, legacyDone, legacy
			},
			rounds:     3,
			wantStatus: "in-progress",
		},
		{
			name: "a new entry reusing a done slug is not marked done",
			setup: func(t *testing.T) ([]Task, []string, []legacyRecord) {
				legacy := []legacyRecord{legacyRec(0, "a", "done")}
				_, legacyDone, err := migrateLegacy(legacy, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				fresh := Task{ID: 7, Slug: "a", Kind: KindTask, Labels: []string{"enhancement"}}
				return []Task{fresh}, legacyDone, legacy
			},
			rounds: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries, legacyDone, legacy := tt.setup(t)
			input := slices.Clone(entries)
			var grown []string
			folded := entries
			for round := range tt.rounds {
				folded, grown = foldLegacyDone(folded, legacyDone, legacy)
				legacyDone = grown
				if round == 0 {
					for i := range input {
						if entries[i].Status != input[i].Status {
							t.Fatal("fold mutated its input entries")
						}
					}
				}
			}
			if len(folded) != len(input) {
				t.Fatalf("entries changed: %+v", folded)
			}
			switch got := folded[0].Status; {
			case tt.wantStatus == "" && got != nil:
				t.Errorf("status = %q, want none", *got)
			case tt.wantStatus != "" && (got == nil || *got != tt.wantStatus):
				t.Errorf("status = %v, want %q", got, tt.wantStatus)
			}
			if tt.checkGrown && !slices.Equal(grown, tt.wantGrown) {
				t.Errorf("legacy_done = %v, want %v", grown, tt.wantGrown)
			}
		})
	}
}

func TestDecodeLegacyLenient(t *testing.T) {
	records, err := decodeLegacy([]byte(`[{"id":1,"slug":"a","unknown":true,"type":"loom"}]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 || records[0].Slug != "a" || records[0].Type != "loom" {
		t.Errorf("records = %+v", records)
	}
	if records, err := decodeLegacy(nil); err != nil || records != nil {
		t.Errorf("empty input: %v %v", records, err)
	}
}
