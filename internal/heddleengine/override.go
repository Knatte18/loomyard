// override.go holds the lym overrides: a hub's tuning texts for shipped entries, stored in the Bolt checkout beside heddle.json and read only while the hub's lym_override setting is on.

package heddleengine

import (
	"fmt"
	"slices"
	"strings"
)

// OverrideFileName is the file at the Bolt checkout's root that holds the lym overrides, in heddle.json's format.
// An override's slug and text are read; its order and targets stay those of the shipped entry it overrides.
const OverrideFileName = "heddle-lym-override.json"

// OverrideLayer is the --layer value that names the lym overrides on `lym heddle set` and `remove`.
const OverrideLayer = "lym"

// LoadOverrides reads the lym overrides from the Bolt checkout boltDir, an absent file holding none.
// It takes no lock; the caller does.
func LoadOverrides(boltDir string) (Store, error) {
	return loadFile(boltDir, OverrideFileName)
}

// SaveOverrides writes the lym overrides as their file in the Bolt checkout boltDir, atomically.
// It takes no lock; the caller does.
func SaveOverrides(boltDir string, s Store) error {
	return saveFile(boltDir, OverrideFileName, s)
}

// SetOverride stores text as the override of the shipped entry slug.
// It refuses a slug the shipped layer does not hold and an empty text, changing nothing.
func (s *Store) SetOverride(slug, text string) error {
	if _, found := shippedStore.Get(slug); !found {
		return fmt.Errorf("no shipped entry %q to override; way forward: run `lym heddle list` for the shipped slugs, or drop --layer %s to set a repository entry", slug, OverrideLayer)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("the override of %q has an empty text; way forward: give a non-empty --text-file", slug)
	}
	entries := slices.Clone(s.Entries)
	if index := slices.IndexFunc(entries, func(entry Entry) bool { return entry.Slug == slug }); index >= 0 {
		entries[index].Text = text
	} else {
		entries = append(entries, Entry{Slug: slug, Text: text})
	}
	s.Entries = entries
	return nil
}

// RemoveOverride deletes the override of slug, and refuses a slug no override holds.
func (s *Store) RemoveOverride(slug string) error {
	index := slices.IndexFunc(s.Entries, func(entry Entry) bool { return entry.Slug == slug })
	if index < 0 {
		return fmt.Errorf("no override of %q; way forward: run `lym heddle list` to see which entries are overridden", slug)
	}
	s.Entries = slices.Delete(slices.Clone(s.Entries), index, index+1)
	return nil
}
