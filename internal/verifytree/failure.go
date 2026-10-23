// failure.go declares the Publish failure record: the never-tracked file Publish leaves when a verify fails and a later passing Publish removes.

package verifytree

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FailureKind names which Publish verify failed: the gate a record's tier belongs to, or a retired spelling of one.
type FailureKind string

const (
	// FailureKindLanding means a tier of the `landing` gate failed.
	FailureKindLanding FailureKind = "landing"
	// FailureKindPublish means a tier of the `publish` gate failed.
	FailureKindPublish FailureKind = "publish"
	// FailureKindPlanVerify is the retired spelling of a `landing` failure: the plan's `## verify:` command failed.
	FailureKindPlanVerify FailureKind = "plan_verify"
	// FailureKindPublishVerify is the retired spelling of a `publish` failure: landing config's `publish_verify` command failed.
	FailureKindPublishVerify FailureKind = "publish_verify"
)

// FailedTest is one test a failing verify reported.
type FailedTest struct {
	// Package is the import path of the test's package.
	Package string `yaml:"package"`
	// Test is the test's path, subtests included.
	Test string `yaml:"test"`
}

// CommitRange is a range of commits named by full commit names; its zero value is the empty range.
type CommitRange struct {
	// From is the range's first commit.
	From string `yaml:"from"`
	// To is the range's last commit.
	To string `yaml:"to"`
}

// TracedCard is one plan card a failing verify's locations were traced to.
// The record stores no slug or intent text, since a reader takes them from the live plan.
type TracedCard struct {
	// Number is the card's number in the plan.
	Number int `yaml:"number"`
	// Glyphs are the card's glyph targets that hold a traced location.
	Glyphs []string `yaml:"glyphs"`
	// Locations are the failing `path:line` locations matched to the card.
	Locations []string `yaml:"locations"`
	// Batch is the Webster batch that ran the card, 0 when Webster holds no record of it.
	Batch int `yaml:"batch"`
	// Range is the commits of the card's batch, omitted when empty.
	Range CommitRange `yaml:"range,omitempty"`
}

// FailureTrace is what tracing a failed verify log to the plan returns and the Publish failure record stores.
type FailureTrace struct {
	// Cards are the plan cards the failing locations were traced to.
	Cards []TracedCard
	// LaterRange is the commits made after the traced batches.
	LaterRange CommitRange
	// Untraced are the failing `path:line` locations no card holds.
	Untraced []string
}

// PublishFailure is the Publish failure record.
// Cards, LaterRange and Untraced are the optional trace of the failure: a record without them reads as it always did.
type PublishFailure struct {
	// Kind names the failing verify.
	Kind FailureKind `yaml:"kind"`
	// Tier is the number of the failing tier of Kind's gate; it is always written, since tier 0 is a real tier, and a record of a retired Kind carries none.
	Tier int `yaml:"tier"`
	// Row is the producer row whose verify failed, empty for a record written before the field existed.
	Row string `yaml:"row,omitempty"`
	// Tests are the failing tests the verify's log names.
	Tests []FailedTest `yaml:"tests,omitempty"`
	// LogPath is the failing run's own verify log, which pruning keeps while the record names it.
	LogPath string `yaml:"log_path"`
	// Head is the commit the verify ran at.
	Head string `yaml:"head"`
	// MergeCommit is the merge-in commit Publish made, empty when it made none.
	MergeCommit string `yaml:"merge_commit,omitempty"`
	// Target is the producer row a fix is routed back to, empty when the writer told none.
	Target string `yaml:"target,omitempty"`
	// WrittenAt is the record's write time in RFC 3339 whole-second UTC, empty for a record written before the field existed.
	WrittenAt string `yaml:"written_at,omitempty"`
	// Cards are the plan cards the failing locations were traced to.
	Cards []TracedCard `yaml:"cards,omitempty"`
	// LaterRange is the commits made after the traced batches.
	LaterRange CommitRange `yaml:"later_range,omitempty"`
	// Untraced are the failing locations no plan card holds.
	Untraced []string `yaml:"untraced,omitempty"`
}

// WritePublishFailure writes f as given to p.PublishFailure.
func WritePublishFailure(p Paths, f PublishFailure) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("verifytree: encode publish failure: %w", err)
	}
	if err := os.WriteFile(p.PublishFailure, data, 0o644); err != nil {
		return fmt.Errorf("verifytree: write publish failure %s: %w", p.PublishFailure, err)
	}
	return nil
}

// ReadPublishFailure reads the record at p.PublishFailure.
// It reports false for an absent record and an error for a malformed one.
func ReadPublishFailure(p Paths) (PublishFailure, bool, error) {
	data, err := os.ReadFile(p.PublishFailure)
	if errors.Is(err, os.ErrNotExist) {
		return PublishFailure{}, false, nil
	}
	if err != nil {
		return PublishFailure{}, false, fmt.Errorf("verifytree: read publish failure %s: %w", p.PublishFailure, err)
	}
	var f PublishFailure
	if err := yaml.Unmarshal(data, &f); err != nil {
		return PublishFailure{}, false, fmt.Errorf("verifytree: decode publish failure %s: %w", p.PublishFailure, err)
	}
	return f, true, nil
}

// RemovePublishFailure removes the record and leaves the log it names in place; an absent record is not an error.
func RemovePublishFailure(p Paths) error {
	if err := os.Remove(p.PublishFailure); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verifytree: remove %s: %w", p.PublishFailure, err)
	}
	return nil
}
