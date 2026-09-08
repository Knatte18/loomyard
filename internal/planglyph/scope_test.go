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

// TestScopeGuard_HandleTargetCoversTheSymbolItStandsFor covers a Create card whose target is a
// plan: handle: the symbol that card just created is inside the plan, not outside it, even though
// the delta reports it under its bare glyph while the card still spells it as a handle.
func TestScopeGuard_HandleTargetCoversTheSymbolItStandsFor(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"plan:sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{DeltaAnswer: quarry.DeltaAnswer{Created: []quarry.Symbol{{ID: "sub#Foo", File: "sub/a.go"}}}}

	got := ScopeGuard(cards, delta)
	if len(got) != 0 {
		t.Errorf("ScopeGuard(handle target) = %+v; want none — the card asked for exactly this symbol", got)
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

// TestScopeGuard_RenamedPairOutsideTargetsIsReported covers the round-4 review's R4-21. Quarry
// removes an exact-tier pair's constituents from Created and Deleted entirely, so a renamed symbol
// reaches neither of those loops — a fork that renamed a symbol outside its batch's declared
// targets was the one kind of touch this guard never reported, and DetectDrift's gate two logs an
// unreferenced rename at Debug only, so nothing operator-visible named it.
func TestScopeGuard_RenamedPairOutsideTargetsIsReported(t *testing.T) {
	cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{"sub#Foo"}}}
	delta := quarry.GitDeltaAnswer{}
	delta.Renamed = []quarry.RenamedPair{{
		From: quarry.Symbol{ID: "sub#Unrelated", File: "sub/e.go"},
		To:   quarry.Symbol{ID: "sub#Renamed", File: "sub/e.go"},
	}}

	got := ScopeGuard(cards, delta)
	if len(got) != 1 {
		t.Fatalf("ScopeGuard(renamed outside) = %+v; want exactly one finding", got)
	}
	if got[0].Check != "scope-outside-plan" || got[0].Severity != SeverityInformational {
		t.Errorf("finding = %+v; want an informational scope-outside-plan", got[0])
	}
}

// TestScopeGuard_RenamedPairInsideTargetsIsNotReported covers the other half: a pair either of
// whose endpoints the batch declared is the card doing the renaming, or a card that asked for the
// new name, and is in scope on either side.
func TestScopeGuard_RenamedPairInsideTargetsIsNotReported(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
	}{
		{name: "old side declared", target: "sub#Foo"},
		{name: "new side declared", target: "sub#Bar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cards := []planparser.Card{{Number: 1, Slug: "one", Targets: []string{tc.target}}}
			delta := quarry.GitDeltaAnswer{}
			delta.Renamed = []quarry.RenamedPair{{
				From: quarry.Symbol{ID: "sub#Foo", File: "sub/e.go"},
				To:   quarry.Symbol{ID: "sub#Bar", File: "sub/e.go"},
			}}

			if got := ScopeGuard(cards, delta); len(got) != 0 {
				t.Fatalf("ScopeGuard(renamed inside, %s) = %+v; want no findings", tc.name, got)
			}
		})
	}
}
