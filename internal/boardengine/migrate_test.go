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

//testtiming:keep pins the unspaced, vocabulary-type, repeated and non-leading bracket prefixes, which its covering tests do not assert
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

func TestMigrateEntriesRewrites(t *testing.T) {
	t.Parallel()
	done := "done"
	tests := []struct {
		name string
		in   []storedEntry
		// want holds the expected fields of each migrated entry, in input order.
		want []Task
	}{
		{
			name: "a note's depends_on moves into its body",
			in: []storedEntry{
				func() storedEntry { e := oldEntry("n1", 3, "feature"); e.DependsOn = []string{"a", "b"}; return e }(),
				func() storedEntry {
					e := oldEntry("n2", 3, "feature")
					e.DependsOn = []string{"a"}
					e.Body = "Some body."
					return e
				}(),
			},
			want: []Task{
				{Kind: KindNote, Labels: []string{"enhancement"}, Body: "Depends on `a`, `b`."},
				{Kind: KindNote, Labels: []string{"enhancement"}, Body: "Some body.\n\nDepends on `a`."},
			},
		},
		{
			name: "a task's edge to a migrated note is removed",
			in: []storedEntry{
				func() storedEntry {
					e := oldEntry("t", 1, "feature")
					e.DependsOn = []string{"done-note", "real"}
					return e
				}(),
				func() storedEntry { e := oldEntry("done-note", 3, "feature"); e.Status = &done; return e }(),
				oldEntry("real", 1, "feature"),
			},
			want: []Task{
				{Kind: KindTask, Labels: []string{"enhancement"}, DependsOn: []string{"real"}},
				{Kind: KindNote, Labels: []string{"enhancement"}, Status: &done},
				{Kind: KindTask, Labels: []string{"enhancement"}},
			},
		},
		{
			name: "an entry that already has a kind is untouched",
			in: []storedEntry{
				{Task: Task{Slug: "a", Kind: KindTask, Labels: []string{"bug"}, Brief: "[infra] keep", DependsOn: []string{"n"}}, Tier: intp(3), Type: strp("design")},
				{Task: Task{Slug: "n", Kind: KindNote, Labels: []string{"bug"}}},
			},
			want: []Task{
				{Kind: KindTask, Labels: []string{"bug"}, Brief: "[infra] keep", DependsOn: []string{"n"}},
				{Kind: KindNote, Labels: []string{"bug"}},
			},
		},
		{
			name: "an entry with neither tier nor kind is a note with empty labels",
			in:   []storedEntry{{Task: Task{Slug: "a"}}},
			want: []Task{{Kind: KindNote, Labels: []string{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := migrateEntries(tt.in, nil)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}
			for i, want := range tt.want {
				e := got[i]
				if e.Kind != want.Kind || e.Brief != want.Brief || e.Body != want.Body ||
					!slices.Equal(e.Labels, want.Labels) || (e.Labels == nil) != (want.Labels == nil) ||
					!slices.Equal(e.DependsOn, want.DependsOn) || (e.Status == nil) != (want.Status == nil) {
					t.Errorf("entry %d = %+v, want %+v", i, e, want)
				}
			}
		})
	}
}
