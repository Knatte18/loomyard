// migrate_test.go — unit tests for the old-shape conversion (migrate.go).

package boardengine

import (
	"slices"
	"testing"
)

func intp(i int) *int { return &i }

func strp(s string) *string { return &s }

func oldEntry(slug string, tier int, typ string) storedEntry {
	e := storedEntry{Task: Task{Slug: slug, Title: slug}, Tier: intp(tier)}
	if typ != "" {
		e.Type = strp(typ)
	}
	return e
}

func TestMigrateEntriesTierAndType(t *testing.T) {
	cases := []struct {
		tier       int
		typ        string
		wantKind   string
		wantLabels []string
	}{
		{1, "feature", KindTask, []string{"enhancement"}},
		{1, "bug", KindTask, []string{"bug"}},
		{2, "chore", KindNote, []string{"enhancement"}},
		{3, "design", KindNote, []string{"enhancement", "undecided"}},
		{3, "", KindNote, []string{"enhancement"}},
		{3, "mystery", KindNote, []string{"enhancement"}},
	}
	for _, c := range cases {
		got := migrateEntries([]storedEntry{oldEntry("a", c.tier, c.typ)}, nil)[0]
		if got.Kind != c.wantKind || !slices.Equal(got.Labels, c.wantLabels) {
			t.Errorf("tier %d type %q: kind=%q labels=%v, want %q %v", c.tier, c.typ, got.Kind, got.Labels, c.wantKind, c.wantLabels)
		}
	}
}

func TestMigrateEntriesBracketPrefixes(t *testing.T) {
	cases := []struct {
		name       string
		brief      string
		vocab      *Vocabulary
		wantLabels []string
		wantBrief  string
	}{
		{"spaced", "[infra] [cli] do it", nil, []string{"enhancement", "infra", "cli"}, "do it"},
		{"unspaced", "[infra][cli]do it", nil, []string{"enhancement", "infra", "cli"}, "do it"},
		{"legacy type dropped", "[feature] [infra] x", nil, []string{"enhancement", "infra"}, "x"},
		{"vocab type dropped", "[question] x", &Vocabulary{Types: []string{"question"}}, []string{"enhancement"}, "x"},
		{"repeat kept once", "[infra] [infra] [enhancement] x", nil, []string{"enhancement", "infra"}, "x"},
		{"not leading", "x [infra]", nil, []string{"enhancement"}, "x [infra]"},
	}
	for _, c := range cases {
		e := oldEntry("a", 1, "feature")
		e.Brief = c.brief
		got := migrateEntries([]storedEntry{e}, c.vocab)[0]
		if !slices.Equal(got.Labels, c.wantLabels) || got.Brief != c.wantBrief {
			t.Errorf("%s: labels=%v brief=%q, want %v %q", c.name, got.Labels, got.Brief, c.wantLabels, c.wantBrief)
		}
	}
}

func TestMigrateEntriesNoteDependsOnMovesToBody(t *testing.T) {
	empty := oldEntry("n1", 3, "feature")
	empty.DependsOn = []string{"a", "b"}
	filled := oldEntry("n2", 3, "feature")
	filled.DependsOn = []string{"a"}
	filled.Body = "Some body."

	got := migrateEntries([]storedEntry{empty, filled}, nil)
	if got[0].Body != "Depends on `a`, `b`." || len(got[0].DependsOn) != 0 {
		t.Errorf("empty body: body=%q deps=%v", got[0].Body, got[0].DependsOn)
	}
	if got[1].Body != "Some body.\n\nDepends on `a`." || len(got[1].DependsOn) != 0 {
		t.Errorf("non-empty body: body=%q deps=%v", got[1].Body, got[1].DependsOn)
	}
}

func TestMigrateEntriesTaskEdgeToMigratedNoteRemoved(t *testing.T) {
	task := oldEntry("t", 1, "feature")
	task.DependsOn = []string{"done-note", "real"}
	done := "done"
	note := oldEntry("done-note", 3, "feature")
	note.Status = &done
	real := oldEntry("real", 1, "feature")

	got := migrateEntries([]storedEntry{task, note, real}, nil)
	if !slices.Equal(got[0].DependsOn, []string{"real"}) {
		t.Errorf("task deps = %v, want [real]", got[0].DependsOn)
	}
}

func TestMigrateEntriesKindEntryUntouched(t *testing.T) {
	e := storedEntry{Task: Task{Slug: "a", Kind: KindTask, Labels: []string{"bug"}, Brief: "[infra] keep", DependsOn: []string{"n"}}, Tier: intp(3), Type: strp("design")}
	n := storedEntry{Task: Task{Slug: "n", Kind: KindNote, Labels: []string{"bug"}}}
	got := migrateEntries([]storedEntry{e, n}, nil)
	if got[0].Kind != KindTask || !slices.Equal(got[0].Labels, []string{"bug"}) || got[0].Brief != "[infra] keep" || !slices.Equal(got[0].DependsOn, []string{"n"}) {
		t.Errorf("entry with kind changed: %+v", got[0])
	}
}

func TestMigrateEntriesNeitherTierNorKindIsNote(t *testing.T) {
	got := migrateEntries([]storedEntry{{Task: Task{Slug: "a"}}}, nil)[0]
	if got.Kind != KindNote || got.Labels == nil || len(got.Labels) != 0 {
		t.Errorf("got kind=%q labels=%v, want a note with empty labels", got.Kind, got.Labels)
	}
}
