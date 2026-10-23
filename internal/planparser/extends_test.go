// extends_test.go covers the Extends type label's checks and the readers that treat an Extends group as an Edit group.

package planparser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestValidate_ExtendsChecks(t *testing.T) {
	t.Parallel()

	const ref = "pkg/ext.go"
	extendsCard := func(mutate func(card *planparser.Card)) planparser.Card {
		card := cardOfType(1, "a", planparser.CardTypeExtends, []string{ref})
		if mutate != nil {
			mutate(&card)
		}
		return card
	}

	tests := []struct {
		name string
		card planparser.Card
		// onDisk reports whether ref exists in the worktree.
		onDisk                                                                   bool
		wantOverlap, wantResign, wantFieldOverlap, wantMissingField, wantMissing int
	}{
		{name: "clean Extends card", card: extendsCard(nil), onDisk: true},
		{
			name: "ref under Extends and Edit",
			card: extendsCard(func(card *planparser.Card) {
				card.Targets = []string{ref, ref}
				card.TargetGroups = append(card.TargetGroups, planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: []string{ref}})
			}),
			onDisk:      true,
			wantOverlap: 1,
		},
		{
			name:             "ref under Extends and Uses",
			card:             extendsCard(func(card *planparser.Card) { card.Uses = []string{ref} }),
			onDisk:           true,
			wantFieldOverlap: 1,
		},
		{
			name: "arrow under Extends",
			card: extendsCard(func(card *planparser.Card) {
				card.TargetGroups[0].Resigns = []planparser.CardResign{{Target: ref, Decl: "func F(n int)"}}
			}),
			onDisk:     true,
			wantResign: 1,
		},
		{
			name:             "Extends card without ImpactSummary",
			card:             extendsCard(func(card *planparser.Card) { card.HasImpactSummary, card.ImpactSummary = false, "" }),
			onDisk:           true,
			wantMissingField: 1,
		},
		{name: "Extends path missing on disk", card: extendsCard(nil), wantMissing: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := t.TempDir()
			if tt.onDisk {
				if err := os.MkdirAll(filepath.Join(worktree, "pkg"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(worktree, ref), []byte("package pkg\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{tt.card}}

			findings := planparser.Validate(plan, worktree)
			for check, want := range map[string]int{
				"extends-overlap":    tt.wantOverlap,
				"extends-resign":     tt.wantResign,
				"card-field-overlap": tt.wantFieldOverlap,
				"card-missing-field": tt.wantMissingField,
				"path-missing":       tt.wantMissing,
			} {
				if got := countFor(findings, check); got != want {
					t.Errorf("countFor(findings, %s) = %d; want %d (%v)", check, got, want, findings)
				}
			}
		})
	}
}
