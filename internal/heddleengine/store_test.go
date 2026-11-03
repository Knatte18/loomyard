// store_test.go drives the store through its file contract: load, save, ordering, entry edits and their refusals.

package heddleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// storeWithUnknownKeys is a heddle.json with an unknown top-level key and an unknown key on each of three entries.
const storeWithUnknownKeys = `{
  "entries": [
    {"slug": "a", "text": "first", "order": 1, "targets": ["claude-file"], "extra_a": 1},
    {"slug": "b", "text": "second", "order": 2, "targets": ["codex-file"], "extra_b": {"k": [1, 2]}},
    {"slug": "c", "text": "third", "order": 3, "targets": ["claude-prompt"], "extra_c": "x"}
  ],
  "top_extra": true
}`

func writeStoreFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadStore(t *testing.T, dir string) Store {
	t.Helper()
	store, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%q) = %v", dir, err)
	}
	return store
}

func saveStore(t *testing.T, dir string, store Store) {
	t.Helper()
	if err := Save(dir, store); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func textOf(s string) *string { return &s }

func orderOf(n int) *int { return &n }

// assertUnknownKeysKept fails when the store lost the top-level key or an entry's unknown key.
func assertUnknownKeysKept(t *testing.T, store Store, wantSlugs ...string) {
	t.Helper()
	if _, ok := store.Unknown["top_extra"]; !ok {
		t.Errorf("top-level unknown key lost: %v", store.Unknown)
	}
	for _, slug := range wantSlugs {
		entry, ok := store.Get(slug)
		if !ok {
			t.Errorf("entry %q missing", slug)
			continue
		}
		if len(entry.Unknown) != 1 {
			t.Errorf("entry %q unknown keys = %v; want one", slug, entry.Unknown)
		}
	}
}

func TestStore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("UnknownKeysSurviveEdits", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeStoreFile(t, dir, storeWithUnknownKeys)

		store := loadStore(t, dir)
		if err := store.Set("b", Patch{Text: textOf("second, edited")}); err != nil {
			t.Fatal(err)
		}
		saveStore(t, dir, store)
		store = loadStore(t, dir)
		assertUnknownKeysKept(t, store, "a", "b", "c")
		if got, _ := store.Get("b"); got.Text != "second, edited" {
			t.Errorf("b.Text = %q; want the edit", got.Text)
		}

		if err := store.Set("d", Patch{Text: textOf("fourth"), Order: orderOf(4), Targets: []string{"claude-file"}}); err != nil {
			t.Fatal(err)
		}
		saveStore(t, dir, store)
		store = loadStore(t, dir)
		assertUnknownKeysKept(t, store, "a", "b", "c")

		if err := store.Remove("a"); err != nil {
			t.Fatal(err)
		}
		saveStore(t, dir, store)
		store = loadStore(t, dir)
		assertUnknownKeysKept(t, store, "b", "c")
		if _, ok := store.Get("a"); ok {
			t.Error("a still held after Remove")
		}
		data, err := os.ReadFile(filepath.Join(dir, FileName))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(data), "}\n") {
			t.Errorf("saved file does not end in a newline: %q", data)
		}
	})

	t.Run("AbsentFileHoldsNoEntry", func(t *testing.T) {
		t.Parallel()
		store := loadStore(t, t.TempDir())
		if got := store.Ordered(); len(got) != 0 {
			t.Errorf("Ordered() = %v; want none", got)
		}
	})

	t.Run("OrderedBreaksTiesBySlug", func(t *testing.T) {
		t.Parallel()
		store := Store{Entries: []Entry{
			{Slug: "z", Order: 1}, {Slug: "m", Order: 2}, {Slug: "a", Order: 2}, {Slug: "k", Order: 0},
		}}
		var got []string
		for _, entry := range store.Ordered() {
			got = append(got, entry.Slug)
		}
		if want := "k z a m"; strings.Join(got, " ") != want {
			t.Errorf("Ordered() = %v; want %s", got, want)
		}
	})

	t.Run("SetRefusals", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			slug  string
			patch Patch
			want  string
		}{
			{"BadSlug", "Bad Slug", Patch{Text: textOf("t"), Order: orderOf(1), Targets: []string{"claude-file"}}, "[a-z0-9-]+"},
			{"EmptyText", "x", Patch{Text: textOf("  \n"), Order: orderOf(1), Targets: []string{"claude-file"}}, "--text-file"},
			{"UnknownTarget", "x", Patch{Text: textOf("t"), Order: orderOf(1), Targets: []string{"nope"}}, "claude-file"},
			{"NewWithoutOrder", "x", Patch{Text: textOf("t"), Targets: []string{"claude-file"}}, "--order"},
			{"NewWithoutTargets", "x", Patch{Text: textOf("t"), Order: orderOf(1)}, "--targets"},
			{"ExistingWithEmptyTargets", "a", Patch{Targets: []string{}}, "--targets"},
			{"ShippedSlug", shippedStore.Ordered()[0].Slug, Patch{Text: textOf("t"), Order: orderOf(1), Targets: []string{"claude-file"}}, "--layer " + OverrideLayer},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				writeStoreFile(t, dir, storeWithUnknownKeys)
				store := loadStore(t, dir)
				before := store.Ordered()
				err := store.Set(tt.slug, tt.patch)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("Set(%q) = %v; want an error naming %q", tt.slug, err, tt.want)
				}
				if got := store.Ordered(); len(got) != len(before) {
					t.Errorf("store changed on a refused Set: %v", got)
				}
			})
		}
	})

	t.Run("ShippedEntriesArePinned", func(t *testing.T) {
		t.Parallel()
		// The rows are the shipped layer in render order.
		// wantHeading is the entry's first line: a level-two heading of its own, so the rendered file steps from the shipped section's level-one heading with no skip.
		tests := []struct {
			slug        string
			wantHeading string
		}{
			{"lym-batching", "## Batch your calls"},
			{"lym-plan-edits-before-gate", "## Make every round of edits in this order"},
		}
		ordered := shippedStore.Ordered()
		if len(ordered) != len(tests) {
			t.Fatalf("shipped layer holds %d entries; want %d", len(ordered), len(tests))
		}
		for i, tt := range tests {
			got := ordered[i]
			if got.Slug != tt.slug || len(got.Targets) != 1 || got.Targets[0] != "claude-file" {
				t.Errorf("shipped entry %d = %+v; want slug %s aimed at claude-file alone", i, got, tt.slug)
			}
			if heading, _, _ := strings.Cut(got.Text, "\n"); heading != tt.wantHeading {
				t.Errorf("shipped entry %s opens with %q; want %q", tt.slug, heading, tt.wantHeading)
			}
		}
	})

	t.Run("RemoveAndGetOfUnknownSlug", func(t *testing.T) {
		t.Parallel()
		store := Store{Entries: []Entry{{Slug: "a"}}}
		if err := store.Remove("nope"); err == nil || !strings.Contains(err.Error(), "lym heddle list") {
			t.Errorf("Remove(nope) = %v; want an error naming lym heddle list", err)
		}
		if _, ok := store.Get("nope"); ok {
			t.Error("Get(nope) reported the slug held")
		}
	})

	t.Run("RemoveRefusesAShippedSlugTheStoreLacks", func(t *testing.T) {
		t.Parallel()
		shipped := shippedStore.Ordered()[0].Slug
		store := Store{Entries: []Entry{{Slug: "a", Text: "t", Order: 1, Targets: []string{"claude-file"}}}}
		if err := store.Remove(shipped); err == nil || !strings.Contains(err.Error(), "ships with lym") {
			t.Errorf("Remove(%s) = %v; want an error naming the shipped layer", shipped, err)
		}
		if len(store.Entries) != 1 {
			t.Errorf("store changed on a refused Remove: %v", store.Entries)
		}
	})

	t.Run("EntryCollidingWithAShippedSlugCanBeChangedAndRemoved", func(t *testing.T) {
		t.Parallel()
		shipped := shippedStore.Ordered()[0].Slug
		store := Store{Entries: []Entry{{Slug: shipped, Text: "t", Order: 1, Targets: []string{"claude-file"}}}}
		if err := store.Set(shipped, Patch{Text: textOf("edited")}); err != nil {
			t.Fatalf("Set(%s) on a colliding entry = %v; want it applied", shipped, err)
		}
		if got, _ := store.Get(shipped); got.Text != "edited" {
			t.Errorf("colliding entry text = %q; want the edit", got.Text)
		}
		if err := store.Remove(shipped); err != nil || len(store.Entries) != 0 {
			t.Errorf("Remove(%s) = %v, leaving %v; want the colliding entry gone", shipped, err, store.Entries)
		}
	})

	t.Run("PartialSetChangesOnlyGivenField", func(t *testing.T) {
		t.Parallel()
		store := Store{Entries: []Entry{{Slug: "a", Text: "t", Order: 1, Targets: []string{"claude-file"}}}}
		if err := store.Set("a", Patch{Order: orderOf(9)}); err != nil {
			t.Fatal(err)
		}
		got, _ := store.Get("a")
		if got.Order != 9 || got.Text != "t" || len(got.Targets) != 1 || got.Targets[0] != "claude-file" {
			t.Errorf("entry after partial Set = %+v", got)
		}
	})

	t.Run("MalformedBlobFailsDecodeWithBlobWayForward", func(t *testing.T) {
		t.Parallel()
		_, err := Decode([]byte(`{"entries": [`))
		if err == nil {
			t.Fatal("Decode of malformed JSON succeeded")
		}
		if !strings.Contains(err.Error(), "with a new commit") {
			t.Errorf("Decode error %q lacks a way forward fitting a committed blob", err)
		}
		if strings.Contains(err.Error(), "checkout HEAD") || strings.Contains(err.Error(), "<Bolt checkout>") {
			t.Errorf("Decode error %q carries the Load-only advice", err)
		}
	})

	t.Run("MalformedFileFailsLoadWithWayForward", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeStoreFile(t, dir, `{"entries": [`)
		_, err := Load(dir)
		if err == nil {
			t.Fatal("Load of malformed JSON succeeded")
		}
		for _, want := range []string{FileName, "git -C " + dir + " checkout HEAD -- " + FileName} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Load error %q lacks %q", err, want)
			}
		}
	})
}
