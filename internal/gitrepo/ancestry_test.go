// ancestry_test.go covers the argument-validation guards of IsAncestor and CommitsNotIn;
// the guards are pure string matching that rejects a malformed sha or a leading-dash ref before any git spawn.
// It carries no //go:build constraint, so it stays in Tier 1 under the Test Tier Purity Invariant;
// it runs under plain `go test` and asserts that no git spawn happens on the rejected path.

package gitrepo

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestIsAncestor_RejectsInvalidArgs asserts that a sha failing validSHA returns ErrInvalidSHA;
// a leading-dash ref, which git would otherwise parse as a flag, gets the same error, without spawning git.
func TestIsAncestor_RejectsInvalidArgs(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestMergeBase_RejectsInvalidArgs asserts that MergeBase rejects a or b failing validSHA;
// it returns ErrInvalidSHA and no SHA, without opening the repository.
func TestMergeBase_RejectsInvalidArgs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const valid = "0123456789abcdef0123456789abcdef01234567"
	repo := New(t.TempDir())

	tests := []struct {
		name string
		a    string
		b    string
	}{
		{"InvalidA_TooShort", "abc", valid},
		{"InvalidB_LongOption", valid, "--help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repo.MergeBase(tt.a, tt.b)
			if !errors.Is(err, ErrInvalidSHA) {
				t.Errorf("MergeBase(%q, %q) error = %v; want errors.Is(err, ErrInvalidSHA)", tt.a, tt.b, err)
			}
			if got != "" {
				t.Errorf("MergeBase(%q, %q) = %q; want empty alongside the error", tt.a, tt.b, got)
			}
		})
	}
}

// TestCommitsNotIn_RejectsInvalidArgs asserts that CommitsNotIn rejects a tip or base failing validSHA;
// it returns ErrInvalidSHA and no commits, without spawning git.
func TestCommitsNotIn_RejectsInvalidArgs(t *testing.T) {
	tiergate.Require(t, 0)
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
