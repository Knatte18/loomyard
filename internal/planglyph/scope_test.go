// scope_test.go covers ScopeGuard against fake deltas, never a real quarry.Repo, since the
// function's own contract is pure comparison over an already-computed answer.

package planglyph

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestScopeGuard_SymbolInsideTargetsNoFinding covers a symbol inside the card's own targets: no
// finding.
func TestScopeGuard_SymbolInsideTargetsNoFinding(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#Foo", File: "sub/a.go"}}}}

	got := ScopeGuard(cards, delta)
	if len(got) != 0 {
		t.Errorf("ScopeGuard(inside) = %+v; want none", got)
	}
}

// TestScopeGuard_SymbolOutsideTargetsOneInformationalFinding covers a symbol outside the card's
// own targets: exactly one informational finding.
func TestScopeGuard_SymbolOutsideTargetsOneInformationalFinding(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#Bar", File: "sub/b.go"}}}}

	got := ScopeGuard(cards, delta)
	if len(got) != 1 || got[0].Check != "scope-outside-plan" || got[0].Severity != SeverityInformational {
		t.Fatalf("ScopeGuard(outside) = %+v; want exactly one informational scope-outside-plan finding", got)
	}
}

// TestScopeGuard_DeletedAndModifiedAlsoChecked covers a Deleted and a Modified symbol outside the
// union, each producing its own informational finding.
func TestScopeGuard_DeletedAndModifiedAlsoChecked(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{
		Deleted:  []quarry.Symbol{{ID: "sub#Gone", File: "sub/c.go"}},
		Modified: []quarry.ModifiedSymbol{{ID: "sub#Changed", After: []quarry.Symbol{{ID: "sub#Changed", File: "sub/d.go"}}}},
	}}

	got := ScopeGuard(cards, delta)
	if len(got) != 2 {
		t.Fatalf("ScopeGuard(deleted+modified outside) = %+v; want exactly two findings", got)
	}
}

// TestScopeGuard_EmptyDeltaNoFindingsNoPanic covers the caller's degradation path input: a
// zero-value delta produces no findings and no panic.
func TestScopeGuard_EmptyDeltaNoFindingsNoPanic(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}

	got := ScopeGuard(cards, quarry.GitDeltaAnswer{})
	if len(got) != 0 {
		t.Errorf("ScopeGuard(zero-value delta) = %+v; want none", got)
	}
}
