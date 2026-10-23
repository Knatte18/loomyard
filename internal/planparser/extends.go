// extends.go implements the two checks of the Extends type label: extends-overlap and extends-resign.
// An Extends group only adds to the refs it names, so both refuse a card that also changes one of them.

package planparser

import "fmt"

// checkExtendsOverlap implements extends-overlap: a ref a card names under an Extends group and also under any other type label of the same card.
// It emits one finding per ref, naming the first other label the ref sits under; an Extends ref also under Uses is card-field-overlap's, not this check's.
// The way forward is to keep the ref under one label, Edit when the card changes what is there.
func checkExtendsOverlap(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		reported := make(map[string]bool)
		for _, extends := range c.TargetGroups {
			if extends.Type != CardTypeExtends {
				continue
			}
			for _, ref := range extends.Refs {
				if reported[ref] {
					continue
				}
				other, found := otherLabelNaming(c, ref)
				if !found {
					continue
				}
				reported[ref] = true
				findings = append(findings, ValidationError{
					Check: "extends-overlap",
					Card:  cardID(c),
					Ref:   ref,
					Detail: fmt.Sprintf(
						"card %d names %q under both %s and %s; keep it under one label, %s when the card changes what is there",
						c.Number, ref, groupBoldLabel(CardTypeExtends), other, groupBoldLabel(CardTypeEdit),
					),
				})
			}
		}
	}

	return findings
}

// otherLabelNaming returns the bold form of the first type label other than Extends under which card names ref, and whether there is one.
func otherLabelNaming(card Card, ref string) (string, bool) {
	for _, group := range card.TargetGroups {
		if group.Type == CardTypeExtends {
			continue
		}
		for _, groupRef := range group.Refs {
			if groupRef == ref {
				return groupBoldLabel(group.Type), true
			}
		}
	}
	return "", false
}

// checkExtendsResign implements extends-resign: an arrow sub-bullet in an Extends group.
// A re-sign changes a member that already exists, which an Extends card never does, so the way forward is to move that ref under Edit.
func checkExtendsResign(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, group := range c.TargetGroups {
			if group.Type != CardTypeExtends {
				continue
			}
			for _, resign := range group.Resigns {
				findings = append(findings, ValidationError{
					Check: "extends-resign",
					Card:  cardID(c),
					Ref:   resign.Target,
					Detail: fmt.Sprintf(
						"card %d re-signs %q under %s; a re-sign changes what is already there, so move that ref under %s",
						c.Number, resign.Target, groupBoldLabel(CardTypeExtends), groupBoldLabel(CardTypeEdit),
					),
				})
			}
		}
	}

	return findings
}
