// legacy.go — pure conversion from the pre-upgrade tasks.json and notes.json records into store entries.
//
// Declares the legacy record shape and file names, the one-time migration, and the done-mark fold that carries a pre-upgrade binary's later done marks into the new store.
// Everything here works on decoded slices, so the I/O layer stays thin and every rule is testable without a disk.

package boardengine

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	// legacyTasksFile and legacyNotesFile name the pre-upgrade stores a board directory may still hold.
	legacyTasksFile = "tasks.json"
	legacyNotesFile = "notes.json"

	// legacyDoneStatus is the status value a pre-upgrade binary wrote to mark a record done.
	legacyDoneStatus = "done"

	// legacyTaskTier, legacyDeferredTier and legacyNoteTier are the tiers migration assigns.
	legacyTaskTier     = 1
	legacyDeferredTier = 3
	legacyNoteTier     = 3
)

// legacyRecord is one record of the pre-upgrade tasks.json or notes.json.
// Its Type is the old recipe name, not an entry kind.
type legacyRecord struct {
	ID        int      `json:"id"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	DependsOn []string `json:"depends_on"`
	Isolated  bool     `json:"isolated"`
	Deferred  bool     `json:"deferred"`
	Brief     string   `json:"brief"`
	Body      string   `json:"body"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	ShortName string   `json:"short_name"`
}

// decodeLegacy decodes a legacy file's bytes leniently: unknown fields are ignored and empty input is no records.
func decodeLegacy(data []byte) ([]legacyRecord, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var records []legacyRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode legacy records: %w", err)
	}
	return records, nil
}

// migrateLegacy converts decoded legacy tasks and notes into store entries and seeds the legacy_done slug list.
// A note whose id collides with an id already taken is given a fresh id above every id in use.
func migrateLegacy(tasks, notes []legacyRecord) ([]Task, []string, error) {
	seen := make(map[string]bool, len(tasks)+len(notes))
	taken := make(map[int]bool, len(tasks)+len(notes))
	maxID := -1
	for _, r := range tasks {
		if seen[r.Slug] {
			return nil, nil, duplicateLegacySlugError(r.Slug)
		}
		seen[r.Slug] = true
		taken[r.ID] = true
		maxID = max(maxID, r.ID)
	}
	for _, r := range notes {
		if seen[r.Slug] {
			return nil, nil, duplicateLegacySlugError(r.Slug)
		}
		seen[r.Slug] = true
		maxID = max(maxID, r.ID)
	}

	entries := make([]Task, 0, len(tasks)+len(notes))
	legacyDone := []string{}
	for _, r := range tasks {
		tier := legacyTaskTier
		if r.Deferred {
			tier = legacyDeferredTier
		}
		entries = append(entries, entryFromLegacy(r, r.ID, tier))
		if r.Status == legacyDoneStatus {
			legacyDone = append(legacyDone, r.Slug)
		}
	}
	for _, r := range notes {
		id := r.ID
		if taken[id] {
			maxID++
			id = maxID
		}
		taken[id] = true
		entries = append(entries, entryFromLegacy(r, id, legacyNoteTier))
		if r.Status == legacyDoneStatus {
			legacyDone = append(legacyDone, r.Slug)
		}
	}
	return entries, legacyDone, nil
}

// duplicateLegacySlugError names a slug found in both legacy files.
func duplicateLegacySlugError(slug string) error {
	return fmt.Errorf("slug %q appears more than once across the legacy tasks and notes files: remove the duplicate from one legacy file", slug)
}

// entryFromLegacy builds a store entry from a legacy record, moving the old type into recipe.
func entryFromLegacy(r legacyRecord, id, tier int) Task {
	t := Task{
		ID:        id,
		Slug:      r.Slug,
		Title:     r.Title,
		Tier:      tier,
		Type:      DefaultType,
		Recipe:    r.Type,
		DependsOn: r.DependsOn,
		Isolated:  r.Isolated,
		Brief:     r.Brief,
		Body:      r.Body,
		ShortName: r.ShortName,
	}
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	if r.Status != "" {
		status := r.Status
		t.Status = &status
	}
	return t
}

// foldLegacyDone carries done marks a pre-upgrade binary wrote after migration into the store.
// Each legacy record marked done whose slug is not yet in legacyDone sets the matching entry done when one exists,
// and the slug joins legacyDone either way.
// No other status or field is folded.
func foldLegacyDone(entries []Task, legacyDone []string, legacy []legacyRecord) ([]Task, []string) {
	folded := slices.Clone(entries)
	grown := slices.Clone(legacyDone)
	for _, r := range legacy {
		if r.Status != legacyDoneStatus || slices.Contains(grown, r.Slug) {
			continue
		}
		for i := range folded {
			if folded[i].Slug == r.Slug {
				done := legacyDoneStatus
				folded[i].Status = &done
				break
			}
		}
		grown = append(grown, r.Slug)
	}
	return folded, grown
}
