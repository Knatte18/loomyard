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
	tasks := []legacyRecord{
		{ID: 0, Slug: "plain", Type: "loom", Brief: "b", Body: "x", Isolated: true, ShortName: "pl", DependsOn: []string{"other"}},
		{ID: 1, Slug: "later", Deferred: true},
	}
	notes := []legacyRecord{{ID: 5, Slug: "idea"}}

	entries, _, err := migrateLegacy(tasks, notes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
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
	for _, e := range entries {
		if err := validateTask(e); err != nil {
			t.Errorf("entry %q fails validation: %v", e.Slug, err)
		}
	}
}

func TestMigrateLegacyBracketPrefixesBecomeLabels(t *testing.T) {
	tasks := []legacyRecord{{ID: 0, Slug: "a", Brief: "[infra] [bug] real brief"}}
	entries, _, err := migrateLegacy(tasks, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a := entryBySlug(t, entries, "a")
	if !slices.Equal(a.Labels, []string{"enhancement", "infra"}) || a.Brief != "real brief" {
		t.Errorf("labels=%v brief=%q, want [enhancement infra] and the stripped brief", a.Labels, a.Brief)
	}
}

func TestMigrateLegacyCollidingNoteIDReassigned(t *testing.T) {
	tasks := []legacyRecord{legacyRec(0, "a", ""), legacyRec(3, "b", "")}
	notes := []legacyRecord{legacyRec(3, "n1", ""), legacyRec(4, "n2", "")}

	entries, _, err := migrateLegacy(tasks, notes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := entryBySlug(t, entries, "b").ID; got != 3 {
		t.Errorf("task id changed to %d", got)
	}
	if got := entryBySlug(t, entries, "n1").ID; got != 5 {
		t.Errorf("colliding note id = %d, want 5 (above every id taken)", got)
	}
	if got := entryBySlug(t, entries, "n2").ID; got != 4 {
		t.Errorf("non-colliding note id = %d, want 4", got)
	}
}

func TestMigrateLegacyDuplicateSlugRefused(t *testing.T) {
	_, _, err := migrateLegacy([]legacyRecord{legacyRec(0, "dup", "")}, []legacyRecord{legacyRec(1, "dup", "")}, nil)
	if err == nil {
		t.Fatal("expected an error for a slug in both files")
	}
	if !strings.Contains(err.Error(), "dup") || !strings.Contains(err.Error(), "remove the duplicate") {
		t.Errorf("error should name the slug and the remedy: %v", err)
	}
}

func TestMigrateLegacyDoneSeed(t *testing.T) {
	tasks := []legacyRecord{legacyRec(0, "t-done", "done"), legacyRec(1, "t-open", "in-progress")}
	notes := []legacyRecord{legacyRec(2, "n-done", "done"), legacyRec(3, "n-open", "")}

	entries, legacyDone, err := migrateLegacy(tasks, notes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slices.Sort(legacyDone)
	if want := []string{"n-done", "t-done"}; !slices.Equal(legacyDone, want) {
		t.Errorf("legacy_done = %v, want %v", legacyDone, want)
	}
	if s := entryBySlug(t, entries, "t-done").Status; s == nil || *s != "done" {
		t.Errorf("done status not carried over: %v", s)
	}
}

func TestFoldLegacyDoneAppliesOnce(t *testing.T) {
	entries, legacyDone, err := migrateLegacy([]legacyRecord{legacyRec(0, "a", "")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy :=[]legacyRecord{legacyRec(0, "a", "done")}

	folded, grown := foldLegacyDone(entries, legacyDone, legacy)
	if s := entryBySlug(t, folded, "a").Status; s == nil || *s != "done" {
		t.Errorf("entry not marked done: %v", s)
	}
	if !slices.Equal(grown, []string{"a"}) {
		t.Errorf("legacy_done = %v, want [a]", grown)
	}

	// The input slice is not mutated.
	if entries[0].Status != nil {
		t.Error("fold mutated its input entries")
	}
}

func TestFoldLegacyDoneAbsentSlugRecorded(t *testing.T) {
	entries := []Task{{ID: 0, Slug: "kept", Kind: KindTask, Labels: []string{"enhancement"}}}
	folded, grown := foldLegacyDone(entries, nil, []legacyRecord{legacyRec(9, "gone", "done")})
	if len(folded) != 1 || folded[0].Status != nil {
		t.Errorf("entries changed: %+v", folded)
	}
	if !slices.Equal(grown, []string{"gone"}) {
		t.Errorf("legacy_done = %v, want [gone]", grown)
	}
}

func TestFoldLegacyDoneIgnoresOtherStatuses(t *testing.T) {
	entries := []Task{{ID: 0, Slug: "a", Kind: KindTask, Labels: []string{"enhancement"}}}
	folded, grown := foldLegacyDone(entries, nil, []legacyRecord{legacyRec(0, "a", "in-progress")})
	if folded[0].Status != nil || len(grown) != 0 {
		t.Errorf("non-done status folded: %+v %v", folded, grown)
	}
}

func TestFoldLegacyDoneReopenedStaysReopened(t *testing.T) {
	legacy := []legacyRecord{legacyRec(0, "a", "done")}
	entries, legacyDone, err := migrateLegacy(legacy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The entry is reopened after migration.
	reopened := "in-progress"
	entries[0].Status = &reopened

	for range 3 {
		entries, legacyDone = foldLegacyDone(entries, legacyDone, legacy)
	}
	if s := entries[0].Status; s == nil || *s != "in-progress" {
		t.Errorf("reopened entry was re-marked done: %v", s)
	}
}

func TestFoldLegacyDoneReusedSlugNotMarkedDone(t *testing.T) {
	legacy := []legacyRecord{legacyRec(0, "a", "done")}
	_, legacyDone, err := migrateLegacy(legacy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A new entry reuses the slug the legacy file still has done.
	fresh := []Task{{ID: 7, Slug: "a", Kind: KindTask, Labels: []string{"enhancement"}}}
	folded, _ := foldLegacyDone(fresh, legacyDone, legacy)
	if folded[0].Status != nil {
		t.Errorf("new entry reusing a done slug was marked done: %v", *folded[0].Status)
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
