// donecheck_test.go covers doneCheckVerdicts, the pure half of DoneChecks: the three done rules
// applied to a hand-built answer index, plus the coverage guard that refuses an answer set which
// does not cover a target the caller asked about.
// It is untagged because it touches no repository at all — DoneChecks' own resolve-backed half is
// covered by donecheck_integration_test.go instead.

package planglyph

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass is the sabotage-proof for R5-6: an
// answer index missing a target the entries name must NOT let that entry's blocking check pass
// silently. Every key reaching doneCheckVerdicts was put into the resolve's own target list by
// DoneChecks, so a missing answer is quarry's positional contract not holding — infrastructure,
// never a verdict on the plan.
func TestDoneCheckVerdicts_UncoveredTargetIsInfrastructureNotAPass(t *testing.T) {
	t.Parallel()

	entries := []doneCheckEntry{{
		card:    planparser.Card{Number: 3, Slug: "add-thing"},
		checkID: "create-not-done",
		key:     "internal/greet#Farewell",
		display: "internal/greet#Farewell",
	}}

	findings, err := doneCheckVerdicts(entries, map[string]quarry.ResolveResult{})

	if err == nil {
		t.Fatalf("doneCheckVerdicts() with an uncovered target = (%v, nil); want a wrapped ErrQuarryUnavailable — passing the check would be a false success", findings)
	}
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("doneCheckVerdicts() error = %v; want it to wrap ErrQuarryUnavailable so a caller can tell an outage from a plan defect", err)
	}
	if !strings.Contains(err.Error(), "internal/greet#Farewell") {
		t.Errorf("doneCheckVerdicts() error = %v; want it to name the uncovered target", err)
	}
	if len(findings) != 0 {
		t.Errorf("doneCheckVerdicts() findings = %v; want none — the uncovered target produced no verdict either way", findings)
	}
}

// TestDoneCheckVerdicts_Rules pins the three done rules against a fully covering answer index, so
// the guard above cannot be satisfied by a version that simply always errors.
func TestDoneCheckVerdicts_Rules(t *testing.T) {
	t.Parallel()

	card := planparser.Card{Number: 1, Slug: "card"}

	tests := []struct {
		name      string
		checkID   string
		status    quarry.Status
		wantCheck string
	}{
		{"create landed", "create-not-done", quarry.StatusFound, ""},
		{"create missing", "create-not-done", quarry.StatusNotFound, "create-not-done"},
		{"delete gone", "delete-not-done", quarry.StatusNotFound, ""},
		{"delete still there", "delete-not-done", quarry.StatusFound, "delete-not-done"},
		{"rename old gone", "rename-not-done-old", quarry.StatusNotFound, ""},
		{"rename old still there", "rename-not-done-old", quarry.StatusFound, "rename-not-done"},
		{"rename new landed", "rename-not-done-new", quarry.StatusFound, ""},
		{"rename new missing", "rename-not-done-new", quarry.StatusNotFound, "rename-not-done"},
		{"multipart counts as resolved", "create-not-done", quarry.StatusMultipart, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const key = "internal/greet#Thing"
			entries := []doneCheckEntry{{card: card, checkID: tt.checkID, key: key, display: key}}
			index := map[string]quarry.ResolveResult{key: {Target: key, Status: tt.status}}

			findings, err := doneCheckVerdicts(entries, index)
			if err != nil {
				t.Fatalf("doneCheckVerdicts() error = %v; want nil", err)
			}
			if tt.wantCheck == "" {
				if len(findings) != 0 {
					t.Fatalf("doneCheckVerdicts() = %v; want no finding", findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("doneCheckVerdicts() = %v; want exactly one %s finding", findings, tt.wantCheck)
			}
			if findings[0].Check != tt.wantCheck {
				t.Errorf("finding check = %q; want %q", findings[0].Check, tt.wantCheck)
			}
			if findings[0].Severity != SeverityBlocking {
				t.Errorf("finding severity = %q; want %q", findings[0].Severity, SeverityBlocking)
			}
		})
	}
}
