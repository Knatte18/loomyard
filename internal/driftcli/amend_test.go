// amend_test.go — tier 0 test for the caller check `lym drift amend` and `lym drift defer` share.

package driftcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckParentCaller covers the operator's unnamed shell, the run's parent, another session and a parent that cannot be resolved.
func TestCheckParentCaller(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const parentName = "ly:hub:orch"
	resolved := func() (hubgeom.Parent, error) { return hubgeom.Parent{Name: parentName}, nil }
	tests := []struct {
		name   string
		strand string
		parent func() (hubgeom.Parent, error)
		// wantErr holds the fragments the refusal names; nil wants the caller admitted.
		wantErr []string
	}{
		{
			// The parent seam fails here, so a pass proves an unnamed caller never resolves the parent.
			name:   "TheOperatorsUnnamedShellPassesWithoutResolvingTheParent",
			parent: func() (hubgeom.Parent, error) { return hubgeom.Parent{}, errors.New("must not be read") },
		},
		{name: "TheRunsParentPasses", strand: parentName, parent: resolved},
		{
			name:    "AnotherSessionIsRefusedNamingTheParent",
			strand:  "ly:task:writer",
			parent:  resolved,
			wantErr: []string{"drift amend refused", `"ly:task:writer"`, "is not the run's parent", "ask " + parentName + " to run it", "operator's own unnamed shell"},
		},
		{
			name:    "AnUnresolvableParentIsRefusedNamingTheErrorAndTheOperatorsShell",
			strand:  "ly:task:writer",
			parent:  func() (hubgeom.Parent, error) { return hubgeom.Parent{}, errors.New("no origin record") },
			wantErr: []string{"drift amend refused", "could not be resolved", "no origin record", "operator's own unnamed shell"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkParentCaller("amend", tt.strand, tt.parent)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("checkParentCaller(%q) = %v; want nil", tt.strand, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkParentCaller(%q) = nil; want a refusal naming %q", tt.strand, tt.wantErr)
			}
			for _, fragment := range tt.wantErr {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("refusal %q does not name %q", err.Error(), fragment)
				}
			}
		})
	}
}
