//go:build tier0

// override_test.go drives the lym overrides through their file contract: setting, replacing and removing an override of a shipped entry, and the refusals.

package heddleengine

import (
	"strings"
	"testing"
)

func TestOverrides(t *testing.T) {
	t.Parallel()
	shipped := shippedStore.Ordered()[0].Slug

	t.Run("SetRefusals", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			slug string
			text string
			want string
		}{
			{"UnshippedSlug", "house-rules", "t", "drop --layer " + OverrideLayer},
			{"EmptyText", shipped, "  \n", "--text-file"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				var overrides Store
				err := overrides.SetOverride(tt.slug, tt.text)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("SetOverride(%q) = %v; want an error naming %q", tt.slug, err, tt.want)
				}
				if len(overrides.Entries) != 0 {
					t.Errorf("overrides changed on a refused SetOverride: %v", overrides.Entries)
				}
			})
		}
	})

	t.Run("SetReplacesAndRemoveDrops", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		var overrides Store
		for _, text := range []string{"first", "second"} {
			if err := overrides.SetOverride(shipped, text); err != nil {
				t.Fatalf("SetOverride(%s, %s) = %v", shipped, text, err)
			}
		}
		if err := SaveOverrides(dir, overrides); err != nil {
			t.Fatalf("SaveOverrides: %v", err)
		}
		loaded, err := LoadOverrides(dir)
		if err != nil {
			t.Fatalf("LoadOverrides: %v", err)
		}
		if len(loaded.Entries) != 1 || loaded.Entries[0].Slug != shipped || loaded.Entries[0].Text != "second" {
			t.Errorf("LoadOverrides() = %v; want one override of %s holding the second text", loaded.Entries, shipped)
		}
		if err := loaded.RemoveOverride(shipped); err != nil || len(loaded.Entries) != 0 {
			t.Errorf("RemoveOverride(%s) = %v, leaving %v; want none left", shipped, err, loaded.Entries)
		}
		if err := loaded.RemoveOverride(shipped); err == nil || !strings.Contains(err.Error(), "lym heddle list") {
			t.Errorf("RemoveOverride of an absent override = %v; want an error naming lym heddle list", err)
		}
	})
}
