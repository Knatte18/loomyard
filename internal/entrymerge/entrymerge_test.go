// entrymerge_test.go drives the merge over a two-field record, through the cases the board replay's own tests do not reach.

package entrymerge_test

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/entrymerge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// record has a plain field and a slice field that reads nil and empty alike.
type record struct {
	Slug  string
	Title string
	Tags  []string
}

var recordTable = entrymerge.Table[record]{
	Key: func(r record) string { return r.Slug },
	Fields: []entrymerge.Field[record]{
		{Name: "title", Get: func(r record) any { return r.Title }, Set: func(dst *record, src record) { dst.Title = src.Title }},
		{Name: "tags", Get: func(r record) any {
			if r.Tags == nil {
				return []string{}
			}
			return r.Tags
		}, Set: func(dst *record, src record) { dst.Tags = slices.Clone(src.Tags) }},
	},
}

// TestTable covers the field changes, add clashes and removal judgment that the board's replay rows leave out.
func TestTable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("Changed lists only differing fields and reads nil and empty slices alike", func(t *testing.T) {
		t.Parallel()
		before := record{Slug: "a", Title: "one"}
		commit := record{Slug: "a", Title: "two", Tags: []string{}}
		changed := recordTable.Changed(before, commit)
		if len(changed) != 1 || changed[0].Name != "title" {
			t.Errorf("Changed() = %v; want only the title field", changed)
		}
	})

	t.Run("Change applies the commit's value when origin still holds the commit's parent value", func(t *testing.T) {
		t.Parallel()
		base := record{Slug: "a", Title: "base"}
		before := record{Slug: "a", Title: "parent"}
		origin := record{Slug: "a", Title: "parent", Tags: []string{"kept"}}
		commit := record{Slug: "a", Title: "commit"}

		merged, applied, lost := recordTable.Change(base, before, origin, commit)
		if merged.Title != "commit" || !slices.Equal(merged.Tags, []string{"kept"}) {
			t.Errorf("Change() merged = %+v; want the commit's title and origin's tags", merged)
		}
		if !slices.Equal(applied, []string{"title"}) || len(lost) != 0 {
			t.Errorf("Change() applied = %v, lost = %v; want [title] and nothing lost", applied, lost)
		}
	})

	t.Run("AddClash reports each differing field with the commit's value", func(t *testing.T) {
		t.Parallel()
		origin := record{Slug: "a", Title: "origin", Tags: []string{"x"}}
		commit := record{Slug: "a", Title: "commit", Tags: []string{"x"}}
		want := []entrymerge.Finding{{Entry: "a", Field: "title", Reason: "origin added an entry with this slug too", Local: `"commit"`}}
		if got := recordTable.AddClash(origin, commit); !slices.Equal(got, want) {
			t.Errorf("AddClash() = %+v; want %+v", got, want)
		}
	})
}
