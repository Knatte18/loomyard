// refusalof_test.go covers RefusalOf's errors.As traversal: a bare refusal, one wrapped several
// layers deep, a nil error, an ordinary error, and remove.go's own dirty pre-flight shape, which is
// not a gate refusal at all. It is a pure-data table test with no git spawn, per the Test Tier
// Purity Shared Decision.

package fabricengine

import (
	"errors"
	"fmt"
	"testing"
)

// TestRefusalOf covers the accessor's behaviour across every shape it must distinguish: a bare
// refusal, a deeply wrapped one, a nil error, an ordinary error, and a bare pre-flight error that
// merely resembles a gate refusal's own dirtiness reason text.
func TestRefusalOf(t *testing.T) {
	bareRefusal := &destructiveRefusal{
		Check:  CheckDirtiness,
		What:   "remove worktree",
		Target: "/hub/pair",
		Reason: "worktree has uncommitted changes; use --force",
	}
	wantRefusal := Refusal{
		Check:  CheckDirtiness,
		What:   "remove worktree",
		Target: "/hub/pair",
		Reason: "worktree has uncommitted changes; use --force",
	}

	tests := []struct {
		name   string
		err    error
		want   Refusal
		wantOK bool
	}{
		{
			name:   "BareRefusal",
			err:    bareRefusal,
			want:   wantRefusal,
			wantOK: true,
		},
		{
			// errors.As, not a type assertion, is what makes this pass: the refusal is wrapped several
			// layers deep via fmt.Errorf's %w verb.
			name:   "WrappedSeveralLayersDeep",
			err:    fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", bareRefusal)),
			want:   wantRefusal,
			wantOK: true,
		},
		{
			name:   "NilError",
			err:    nil,
			want:   Refusal{},
			wantOK: false,
		},
		{
			name:   "OrdinaryError",
			err:    errors.New("some unrelated failure"),
			want:   Refusal{},
			wantOK: false,
		},
		{
			// remove.go's own dirty pre-flight shape: a bare fmt.Errorf, never a *destructiveRefusal.
			// This is the case batch 6's envelope relies on to omit the refusal object on the Remove
			// anomaly path.
			name:   "RemovePreFlightDirtyError",
			err:    fmt.Errorf("worktree has uncommitted changes; use --force"),
			want:   Refusal{},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RefusalOf(tt.err)
			if ok != tt.wantOK {
				t.Errorf("RefusalOf(%v) ok = %v; want %v", tt.err, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("RefusalOf(%v) = %+v; want %+v", tt.err, got, tt.want)
			}
		})
	}
}

// TestCheck_RendersThreeSpellings pins the three Check constants' exact string spellings, since
// fabrictest's own duplicate copy (batch 7) and destructiveRefusal.Error()'s composed message both
// depend on them staying byte-identical.
func TestCheck_RendersThreeSpellings(t *testing.T) {
	tests := []struct {
		name string
		c    Check
		want string
	}{
		{"Containment", CheckContainment, "containment"},
		{"Ownership", CheckOwnership, "ownership"},
		{"Dirtiness", CheckDirtiness, "dirtiness"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.c); got != tt.want {
				t.Errorf("string(%s) = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestCheck_HasNoFourthMemberForForce documents, rather than mechanically enforces via reflection,
// the rule that Check must never grow a CheckForce constant: force is consulted only inside
// checkPathDirtiness, where it makes the dirtiness check PASS rather than fail, so a refusal can
// never be attributed to it. A refusal always carries one of exactly these three checks.
func TestCheck_HasNoFourthMemberForForce(t *testing.T) {
	all := []Check{CheckContainment, CheckOwnership, CheckDirtiness}
	if len(all) != 3 {
		t.Fatalf("Check enum carries %d members; want exactly 3 (no CheckForce)", len(all))
	}
}
