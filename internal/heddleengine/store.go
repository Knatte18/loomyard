// store.go holds heddle.json's in-memory shape, its tolerant JSON encoding, and its load, save and entry edits.

package heddleengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
)

// FileName is the store's file name at the Bolt checkout's root.
const FileName = "heddle.json"

// slugRule is the form an entry's slug takes.
var slugRule = regexp.MustCompile(`^[a-z0-9-]+$`)

// Entry is one instruction text and where it is delivered.
// Unknown holds the keys this binary does not know, each with its raw JSON value, written back after the known ones in key order.
type Entry struct {
	Slug    string
	Text    string
	Order   int
	Targets []string
	Unknown map[string]json.RawMessage
}

// entryKeys is the set of JSON keys Entry declares.
var entryKeys = map[string]bool{"slug": true, "text": true, "order": true, "targets": true}

// entryWire is the known part of Entry's JSON form.
type entryWire struct {
	Slug    string   `json:"slug"`
	Text    string   `json:"text"`
	Order   int      `json:"order"`
	Targets []string `json:"targets"`
}

// MarshalJSON encodes the known keys, then the Unknown ones in key order.
func (e Entry) MarshalJSON() ([]byte, error) {
	known := entryWire{Slug: e.Slug, Text: e.Text, Order: e.Order, Targets: e.Targets}
	if known.Targets == nil {
		known.Targets = []string{}
	}
	object, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, e.Unknown)
}

// UnmarshalJSON decodes the known keys and keeps every other key raw.
func (e *Entry) UnmarshalJSON(data []byte) error {
	var known entryWire
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	unknown, err := unknownKeys(data, entryKeys)
	if err != nil {
		return err
	}
	*e = Entry{Slug: known.Slug, Text: known.Text, Order: known.Order, Targets: known.Targets, Unknown: unknown}
	return nil
}

// Store is the content of heddle.json.
// Unknown holds the top-level keys this binary does not know, kept the way an entry's are.
type Store struct {
	Entries []Entry
	Unknown map[string]json.RawMessage
}

// storeKeys is the set of JSON keys Store declares.
var storeKeys = map[string]bool{"entries": true}

// storeWire is the known part of Store's JSON form.
type storeWire struct {
	Entries []Entry `json:"entries"`
}

// MarshalJSON encodes the entries, then the Unknown keys in key order.
func (s Store) MarshalJSON() ([]byte, error) {
	known := storeWire{Entries: s.Entries}
	if known.Entries == nil {
		known.Entries = []Entry{}
	}
	object, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, s.Unknown)
}

// UnmarshalJSON decodes the entries and keeps every other top-level key raw.
func (s *Store) UnmarshalJSON(data []byte) error {
	var known storeWire
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	unknown, err := unknownKeys(data, storeKeys)
	if err != nil {
		return err
	}
	*s = Store{Entries: known.Entries, Unknown: unknown}
	return nil
}

// unknownKeys returns the members of the JSON object data whose key is not in known, or nil when there are none.
func unknownKeys(data []byte, known map[string]bool) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	var unknown map[string]json.RawMessage
	for key, value := range members {
		if known[key] {
			continue
		}
		if unknown == nil {
			unknown = map[string]json.RawMessage{}
		}
		unknown[key] = slices.Clone(value)
	}
	return unknown, nil
}

// appendUnknown writes the unknown members into the encoded JSON object after its known ones, in key order.
func appendUnknown(object []byte, unknown map[string]json.RawMessage) ([]byte, error) {
	if len(unknown) == 0 {
		return object, nil
	}
	keys := make([]string, 0, len(unknown))
	for key := range unknown {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := bytes.TrimSuffix(slices.Clone(object), []byte("}"))
	for _, key := range keys {
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		out = append(out, ',')
		out = append(out, encodedKey...)
		out = append(out, ':')
		out = append(out, unknown[key]...)
	}
	return append(out, '}'), nil
}

// Decode decodes the bytes of a heddle.json committed in git, empty or whitespace-only bytes being an empty store.
// Bytes that do not decode are refused with the way forward for a malformed committed blob.
func Decode(data []byte) (Store, error) {
	store, err := decodeBytes(data)
	if err != nil {
		return Store{}, fmt.Errorf("%s holds malformed JSON: %w; way forward: fix the file with a new commit on the branch that holds this blob", FileName, err)
	}
	return store, nil
}

// decodeBytes decodes data, empty or whitespace-only data being an empty store.
func decodeBytes(data []byte) (Store, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Store{}, nil
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, err
	}
	return store, nil
}

// Load reads heddle.json from the Bolt checkout boltDir, an absent file holding no entry.
// It takes no lock; the caller does.
func Load(boltDir string) (Store, error) {
	data, err := os.ReadFile(filepath.Join(boltDir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Store{}, nil
	}
	if err != nil {
		return Store{}, fmt.Errorf("read %s: %w", FileName, err)
	}
	store, err := decodeBytes(data)
	if err != nil {
		return Store{}, fmt.Errorf("%s does not decode: %w; way forward: restore the last committed file with `git -C %s checkout HEAD -- %s`, or an earlier commit found with `git -C %s log -- %s`", FileName, err, boltDir, FileName, boltDir, FileName)
	}
	return store, nil
}

// Save writes s as heddle.json in the Bolt checkout boltDir, indented and ending in a newline, atomically.
// It takes no lock; the caller does.
func Save(boltDir string, s Store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", FileName, err)
	}
	return fsx.AtomicWrite(boltDir, FileName, string(data)+"\n")
}

// Ordered returns the entries by ascending order, a tie broken by slug.
func (s Store) Ordered() []Entry {
	ordered := slices.Clone(s.Entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Order != ordered[j].Order {
			return ordered[i].Order < ordered[j].Order
		}
		return ordered[i].Slug < ordered[j].Slug
	})
	return ordered
}

// Get returns the entry with slug, and false when the store holds none.
func (s Store) Get(slug string) (Entry, bool) {
	for _, entry := range s.Entries {
		if entry.Slug == slug {
			return entry, true
		}
	}
	return Entry{}, false
}

// Patch holds the fields of an entry a Set changes; a nil field is left as it is.
type Patch struct {
	Text    *string
	Order   *int
	Targets []string
}

// Set updates the entry with slug by the fields p gives, or creates it, which needs all three fields.
// The result must pass ValidateEntry or nothing changes.
func (s *Store) Set(slug string, p Patch) error {
	index := slices.IndexFunc(s.Entries, func(entry Entry) bool { return entry.Slug == slug })
	candidate := Entry{Slug: slug}
	if index >= 0 {
		candidate = s.Entries[index]
		candidate.Targets = slices.Clone(candidate.Targets)
	} else if p.Text == nil || p.Order == nil || p.Targets == nil {
		return fmt.Errorf("entry %q is new, so it needs %s; way forward: give them all in the same call", slug, missingFlags(p))
	}
	if p.Text != nil {
		candidate.Text = *p.Text
	}
	if p.Order != nil {
		candidate.Order = *p.Order
	}
	if p.Targets != nil {
		candidate.Targets = slices.Clone(p.Targets)
	}
	if err := ValidateEntry(candidate); err != nil {
		return err
	}
	entries := slices.Clone(s.Entries)
	if index >= 0 {
		entries[index] = candidate
	} else {
		entries = append(entries, candidate)
	}
	s.Entries = entries
	return nil
}

// missingFlags names the flags of the fields p leaves unset.
func missingFlags(p Patch) string {
	var flags []string
	if p.Text == nil {
		flags = append(flags, "--text-file")
	}
	if p.Order == nil {
		flags = append(flags, "--order")
	}
	if p.Targets == nil {
		flags = append(flags, "--targets")
	}
	return strings.Join(flags, ", ")
}

// Remove deletes the entry with slug, and refuses a slug the store does not hold.
func (s *Store) Remove(slug string) error {
	index := slices.IndexFunc(s.Entries, func(entry Entry) bool { return entry.Slug == slug })
	if index < 0 {
		return fmt.Errorf("no entry %q; way forward: list the entries with `lym heddle list`", slug)
	}
	s.Entries = slices.Delete(slices.Clone(s.Entries), index, index+1)
	return nil
}

// ValidateEntry refuses an entry that cannot be rendered: a slug off the slug rule, an empty text, no target or a target the table does not hold.
// Each message names the way forward.
func ValidateEntry(e Entry) error {
	if !slugRule.MatchString(e.Slug) {
		return fmt.Errorf("slug %q is not valid; way forward: use lower-case letters, digits and hyphens only, matching [a-z0-9-]+", e.Slug)
	}
	if strings.TrimSpace(e.Text) == "" {
		return fmt.Errorf("entry %q has an empty text; way forward: give a non-empty --text-file", e.Slug)
	}
	if len(e.Targets) == 0 {
		return fmt.Errorf("entry %q has no target; way forward: give at least one of %s in --targets", e.Slug, strings.Join(TargetNames(), ", "))
	}
	for _, name := range e.Targets {
		if _, ok := LookupTarget(name); !ok {
			return fmt.Errorf("entry %q names the unknown target %q; way forward: use one of %s", e.Slug, name, strings.Join(TargetNames(), ", "))
		}
	}
	return nil
}
