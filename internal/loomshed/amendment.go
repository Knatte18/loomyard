// amendment.go owns the plan amendment record, the tracked file recording that a decision amends the approved plan.
// It reads and writes the record over a told path and resolves no geometry.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AmendmentRecord is the decoded plan amendment record.
// Titles holds the title of every pending amending decision entry in append order, and WrittenAt the latest write in UTC.
type AmendmentRecord struct {
	Titles    []string  `yaml:"titles"`
	WrittenAt time.Time `yaml:"written_at"`
}

// ReadAmendment decodes the amendment record at path and reports false for an absent file.
// Unknown keys are read and dropped.
// A file that does not decode is an error naming the path and the way forward, which every reader returns unchanged.
func ReadAmendment(path string) (AmendmentRecord, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return AmendmentRecord{}, false, nil
	}
	if err != nil {
		return AmendmentRecord{}, false, fmt.Errorf("read amendment record %s: %w", path, err)
	}
	var record AmendmentRecord
	if err := yaml.Unmarshal(data, &record); err != nil {
		return AmendmentRecord{}, false, fmt.Errorf("amendment record %s does not decode: %w; way forward: rewrite the file as a valid record, `titles` listing the pending amending decision entries' titles and `written_at` the current UTC time, or remove it when no amendment is pending, run `lym fabric commit`, then re-run the refused step, which is the goto, `lym loom resume` for a failed Plan-Bouncer commit seam, or the `decision add` call; a session lym refuses the verb from reports status: FAILED", path, err)
	}
	return record, true, nil
}

// AppendAmendment adds title to the record at path and sets its write time to at in UTC, creating the file and its directory when absent.
func AppendAmendment(path, title string, at time.Time) error {
	record, _, err := ReadAmendment(path)
	if err != nil {
		return err
	}
	record.Titles = append(record.Titles, title)
	record.WrittenAt = at.UTC()
	data, err := yaml.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode amendment record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create amendment record directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write amendment record %s: %w", path, err)
	}
	return nil
}

// RemoveAmendment deletes the record at path; an absent file is no error.
func RemoveAmendment(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove amendment record %s: %w", path, err)
	}
	return nil
}

// AmendmentSettledBy reports whether an approved settle whose judged round started at roundStart carried the amendment.
// It is true exactly when roundStart is non-zero and after the record's write time, compared as instants.
// A zero roundStart, a settle with no known round start, keeps the record.
// The write time keeps full precision while a round start is floored to the second,
// so an amendment written in the second its round started reads as after the start and is kept:
// the floor costs at most an extra round and never loses an amendment.
func AmendmentSettledBy(record AmendmentRecord, roundStart time.Time) bool {
	return !roundStart.IsZero() && roundStart.After(record.WrittenAt)
}
