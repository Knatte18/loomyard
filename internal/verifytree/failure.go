// failure.go declares the landing failure record: the never-tracked file a landing row leaves when its gate fails and a later passing gate removes.

package verifytree

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FailureKind names which gate failed: the gate a record's tier belongs to, or a retired spelling of one.
type FailureKind string

const (
	// FailureKindLanding is the retired `landing` kind an older build wrote.
	// A record carrying it reads as the gate key of the row that reads it: Merge-In, Finalize or the merge fixer either spawns, since the record names no row and only those rows read it.
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

// FailureTrace is what tracing a failed verify log to the plan returns and the landing failure record stores.
type FailureTrace struct {
	// Cards are the plan cards the failing locations were traced to.
	Cards []TracedCard
	// LaterRange is the commits made after the traced batches.
	LaterRange CommitRange
	// Untraced are the failing `path:line` locations no card holds.
	Untraced []string
}

// LandingFailure is the landing failure record.
// Cards, LaterRange and Untraced are the optional trace of the failure: a record without them reads as it always did.
type LandingFailure struct {
	// Kind names the failing verify.
	Kind FailureKind `yaml:"kind"`
	// Tier is the number of the failing tier of Kind's gate; it is always written, since tier 0 is a real tier, and a record of a retired Kind carries none.
	Tier int `yaml:"tier"`
	// Tests are the failing tests the verify's log names.
	Tests []FailedTest `yaml:"tests,omitempty"`
	// LogPath is the failing run's own verify log, which pruning keeps while the record names it.
	LogPath string `yaml:"log_path"`
	// Head is the commit the verify ran at.
	Head string `yaml:"head"`
	// MergeCommit is the merge-in commit the row made, empty when it made none.
	MergeCommit string `yaml:"merge_commit,omitempty"`
	// WrittenAt is the record's write time in RFC 3339 whole-second UTC, empty for a record written before the field existed.
	WrittenAt string `yaml:"written_at,omitempty"`
	// Cards are the plan cards the failing locations were traced to.
	Cards []TracedCard `yaml:"cards,omitempty"`
	// LaterRange is the commits made after the traced batches.
	LaterRange CommitRange `yaml:"later_range,omitempty"`
	// Untraced are the failing locations no plan card holds.
	Untraced []string `yaml:"untraced,omitempty"`
}

// WriteLandingFailure writes f as given to p.LandingFailure.
func WriteLandingFailure(p Paths, f LandingFailure) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("verifytree: encode landing failure: %w", err)
	}
	if err := os.WriteFile(p.LandingFailure, data, 0o644); err != nil {
		return fmt.Errorf("verifytree: write landing failure %s: %w", p.LandingFailure, err)
	}
	return nil
}

// ReadLandingFailure reads the record at p.LandingFailure.
// It reports false for an absent record and an error for a malformed one.
func ReadLandingFailure(p Paths) (LandingFailure, bool, error) {
	data, err := os.ReadFile(p.LandingFailure)
	if errors.Is(err, os.ErrNotExist) {
		return LandingFailure{}, false, nil
	}
	if err != nil {
		return LandingFailure{}, false, fmt.Errorf("verifytree: read landing failure %s: %w", p.LandingFailure, err)
	}
	var f LandingFailure
	if err := yaml.Unmarshal(data, &f); err != nil {
		return LandingFailure{}, false, fmt.Errorf("verifytree: decode landing failure %s: %w", p.LandingFailure, err)
	}
	return f, true, nil
}

// RemoveLandingFailure removes the record and leaves the log it names in place; an absent record is not an error.
func RemoveLandingFailure(p Paths) error {
	if err := os.Remove(p.LandingFailure); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verifytree: remove %s: %w", p.LandingFailure, err)
	}
	return nil
}
