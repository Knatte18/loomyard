// ancestry_test.go covers IsAncestor's and CommitsNotIn's argument-validation guard — pure string-matching logic that
// must reject a malformed sha or a leading-dash ref before ever spawning git.
// It is deliberately untagged (no //go:build constraint) and in the internal package so it can
// assert no git spawn happens on the rejected path, keeping it in Tier 1 alongside plain `go test`
// per the Test Tier Purity Invariant.

package gitrepo

import (
	"errors"
	"testing"
)

// TestIsAncestor_RejectsInvalidArgs asserts a validSHA-failing sha and a leading-dash ref (which
// git would otherwise parse as a flag) each return ErrInvalidSHA via errors.Is, without spawning
// git — this test's own untagged Tier-1 status is the proof of that: it runs under plain `go test`,
// with no hermetic git test-binary requirement, so a call that actually reached git would fail this
// file's own build/run contract.
func TestIsAncestor_RejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	repo := New(t.TempDir())

	tests := []struct {
		name string
		sha  string
		ref  string
	}{
		{"InvalidSHA_TooShort", "abc", "HEAD"},
		{"InvalidSHA_LongOption", "--help", "HEAD"},
		{"InvalidRef_LeadingDash", "0123456789abcdef0123456789abcdef01234567", "-d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repo.IsAncestor(tt.sha, tt.ref)
			if !errors.Is(err, ErrInvalidSHA) {
				t.Errorf("IsAncestor(%q, %q) error = %v; want errors.Is(err, ErrInvalidSHA)", tt.sha, tt.ref, err)
			}
			if got {
				t.Errorf("IsAncestor(%q, %q) = %v; want false alongside the error", tt.sha, tt.ref, got)
			}
		})
	}
}

// TestCommitsNotIn_RejectsInvalidArgs asserts a validSHA-failing tip or base returns ErrInvalidSHA and no commits, without spawning git, for the same untagged Tier-1 reason as TestIsAncestor_RejectsInvalidArgs.
func TestCommitsNotIn_RejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	const valid = "0123456789abcdef0123456789abcdef01234567"
	repo := New(t.TempDir())

	tests := []struct {
		name string
		tip  string
		base string
	}{
		{"InvalidTip_TooShort", "abc", valid},
		{"InvalidBase_LongOption", valid, "--help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repo.CommitsNotIn(tt.tip, tt.base)
			if !errors.Is(err, ErrInvalidSHA) {
				t.Errorf("CommitsNotIn(%q, %q) error = %v; want errors.Is(err, ErrInvalidSHA)", tt.tip, tt.base, err)
			}
			if got != nil {
				t.Errorf("CommitsNotIn(%q, %q) = %v; want nil alongside the error", tt.tip, tt.base, got)
			}
		})
	}
}
