// pairrule_test.go covers CardRefKind and DependencyEdge, the pair rule that decides whether two cards' overlapping refs make a dependency edge.

package planparser

import "testing"

// extendsCard returns a card whose only target group is an Extends group of ref, fencing a PATTERN.md line for each of entryIDs.
func extendsCard(number int, ref string, entryIDs ...string) Card {
	text := ""
	if len(entryIDs) > 0 {
		text = "```\n"
		for _, id := range entryIDs {
			text += "- `" + id + "` — rule.\n"
		}
		text += "```\n"
	}
	return Card{
		Number:       number,
		Targets:      []string{ref},
		TargetGroups: []TargetGroup{{Type: CardTypeExtends, Refs: []string{ref}}},
		Text:         text,
	}
}

// editCard returns a card whose only target group is an Edit group of ref.
func editCard(number int, ref string) Card {
	return Card{
		Number:       number,
		Targets:      []string{ref},
		TargetGroups: []TargetGroup{{Type: CardTypeEdit, Refs: []string{ref}}},
	}
}

func TestDependencyEdge(t *testing.T) {
	t.Parallel()

	const ref = "PATTERN.md"
	usesCard := Card{Number: 3, Uses: []string{ref}}

	kinds := []struct {
		name string
		card Card
		ref  string
		want RefKind
	}{
		{"Edit group", editCard(1, ref), ref, RefChange},
		{"Create group", Card{Targets: []string{ref}, TargetGroups: []TargetGroup{{Type: CardTypeCreate, Refs: []string{ref}}}}, ref, RefChange},
		{"Extends group", extendsCard(1, ref), ref, RefExtends},
		{"Extends and Edit groups", Card{TargetGroups: []TargetGroup{
			{Type: CardTypeExtends, Refs: []string{ref}},
			{Type: CardTypeEdit, Refs: []string{ref}},
		}}, ref, RefChange},
		{"groupless target", Card{Targets: []string{ref}}, ref, RefChange},
		{"Uses entry", usesCard, ref, RefUse},
		{"ref with surrounding space", editCard(1, ref), " " + ref + " ", RefChange},
		{"unnamed ref", editCard(1, ref), "other.md", 0},
		{"blank ref", editCard(1, ref), " ", 0},
	}
	for _, tt := range kinds {
		t.Run("CardRefKind_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CardRefKind(tt.card, tt.ref); got != tt.want {
				t.Errorf("CardRefKind(%q) = %d; want %d", tt.ref, got, tt.want)
			}
		})
	}

	pairs := []struct {
		name         string
		earlier      Card
		earlierKind  RefKind
		later        Card
		laterKind    RefKind
		wantEdgeMade bool
	}{
		{"Extends against Extends", extendsCard(1, ref), RefExtends, extendsCard(2, ref), RefExtends, false},
		{"Extends against Extends fencing one entry", extendsCard(1, ref, "PATTERN-x"), RefExtends, extendsCard(2, ref, "PATTERN-x"), RefExtends, true},
		{"Extends against Extends fencing different entries", extendsCard(1, ref, "PATTERN-x"), RefExtends, extendsCard(2, ref, "PATTERN-y"), RefExtends, false},
		{"Extends against Edit", extendsCard(1, ref), RefExtends, editCard(2, ref), RefChange, true},
		{"Edit against Extends", editCard(1, ref), RefChange, extendsCard(2, ref), RefExtends, true},
		{"Uses against Extends", extendsCard(1, ref), RefExtends, usesCard, RefUse, true},
		{"Uses against Uses", usesCard, RefUse, Card{Number: 4, Uses: []string{ref}}, RefUse, false},
		{"groupless target against groupless target", Card{Targets: []string{ref}}, RefChange, Card{Targets: []string{ref}}, RefChange, true},
		{"unnamed ref on one side", editCard(1, ref), RefChange, Card{Number: 2}, 0, false},
	}
	for _, tt := range pairs {
		t.Run("DependencyEdge_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DependencyEdge(tt.earlier, tt.earlierKind, tt.later, tt.laterKind); got != tt.wantEdgeMade {
				t.Errorf("DependencyEdge(%d, %d) = %v; want %v", tt.earlierKind, tt.laterKind, got, tt.wantEdgeMade)
			}
		})
	}
}
