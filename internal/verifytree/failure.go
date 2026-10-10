// failure.go declares the Publish failure record: the never-tracked file Publish leaves when a verify fails and a later passing Publish removes.

package verifytree

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// FailureKind names which Publish verify failed.
type FailureKind string

const (
	// FailureKindPlanVerify means the plan's `## verify:` command failed.
	FailureKindPlanVerify FailureKind = "plan_verify"
	// FailureKindPublishVerify means landing config's `publish_verify` command failed.
	FailureKindPublishVerify FailureKind = "publish_verify"
)

// FailedTest is one test a failing verify reported.
type FailedTest struct {
	// Package is the import path of the test's package.
	Package string `yaml:"package"`
	// Test is the test's path, subtests included.
	Test string `yaml:"test"`
}

// PublishFailure is the Publish failure record.
type PublishFailure struct {
	// Kind names the failing verify.
	Kind FailureKind `yaml:"kind"`
	// Tests are the failing tests the verify's log names.
	Tests []FailedTest `yaml:"tests,omitempty"`
	// LogPath is the failing run's own verify log, which pruning keeps while the record names it.
	LogPath string `yaml:"log_path"`
	// Head is the commit the verify ran at.
	Head string `yaml:"head"`
	// MergeCommit is the merge-in commit Publish made, empty when it made none.
	MergeCommit string `yaml:"merge_commit,omitempty"`
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
