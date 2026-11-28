// amend_test.go is a tier 0 test of the caller check that `lym drift amend` and `lym drift defer` share.

package driftcli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckParentCaller covers each caller the check admits and each refusal, with the chain and parent readable or not.
func TestCheckParentCaller(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const parentName = "ly:hub:orch"
	resolved := func() (hubgeom.Parent, error) { return hubgeom.Parent{Name: parentName}, nil }
	tests := []struct {
		name   string
		strand string
		// chain and chainErr are what the read of the orchs above the pair answers.
		chain    []hubgeom.Parent
		chainErr error
		parent   func() (hubgeom.Parent, error)
		// wantErr holds the fragments the refusal names; nil wants the caller admitted.
		wantErr []string
	}{
		{
			// The parent seam fails here, so a pass proves an unnamed caller never resolves the parent.
			name:   "TheOperatorsUnnamedShellPassesWithoutResolvingTheParent",
			parent: func() (hubgeom.Parent, error) { return hubgeom.Parent{}, errors.New("must not be read") },
		},
		{name: "TheRunsParentPasses", strand: parentName, parent: resolved},
		{name: "ABeamOrchAboveThePairPassesWhileAnotherOrchResolvesAsTheParent", strand: "ly:beam-b:orch", chain: []hubgeom.Parent{{Name: "ly:beam-b:orch"}, {Name: parentName}}, parent: resolved},
		{name: "TheResolvedParentPassesWhenTheChainCannotBeRead", strand: parentName, chainErr: errors.New("origin record unreadable"), parent: resolved},
		{
			name:     "AnUnreadableChainRefusesAnotherSessionNamingTheError",
			strand:   "ly:task:writer",
			chainErr: errors.New("origin record unreadable"),
			parent:   resolved,
			wantErr:  []string{"drift amend refused", "could not be resolved", "origin record unreadable", "operator's own unnamed shell"},
		},
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
			orchsAbove := func() ([]hubgeom.Parent, error) { return tt.chain, tt.chainErr }
			err := checkParentCaller("amend", tt.strand, orchsAbove, tt.parent)
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
