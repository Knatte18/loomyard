// redundancy.go implements redundant-package-target: a card whose own targets list a package self glyph beside a member, a file or a plan: handle of that same package.
// The check reads plan text alone, so its verdict is the same before and after record-batch binds a handle, and it runs at every gate that runs ValidateFormat.

package planparser

import (
	"fmt"
	"path"

	"github.com/Knatte18/quarry/glyph"
)

// redundancyOwner is what a card target belongs to for redundant-package-target: the package directory it lies in, and whether the target is that package's own self glyph.
type redundancyOwner struct {
	unit      string
	container bool
}

// redundancyMember is a card target contained in the package directory unit.
type redundancyMember struct {
	ref  string
	unit string
}

// checkRedundantPackageTarget implements redundant-package-target: a card's own flat Targets (every group, both sides of a Rename pair, never Uses) list a package self glyph and also a member, a file self glyph or a handle belonging to that package.
// One finding per container and contained target, attributed to the card, with Ref the contained target.
// Skipped entirely when plan.Language does not enable the glyph alphabet.
func checkRedundantPackageTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	lang, ok := planLanguage(plan)
	if !ok {
		return findings
	}

	renameUnits := renameToSideUnits(lang, plan)

	for _, c := range plan.Cards {
		containers := make(map[string]string)
		var members []redundancyMember
		seen := make(map[string]bool)

		for _, t := range c.Targets {
			if seen[t] {
				continue
			}
			seen[t] = true
			owner, ok := redundancyOwnerOf(lang, renameUnits, t)
			if !ok {
				continue
			}
			if owner.container {
				containers[owner.unit] = t
				continue
			}
			members = append(members, redundancyMember{ref: t, unit: owner.unit})
		}

		for _, m := range members {
			container, has := containers[m.unit]
			if !has {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "redundant-package-target",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d lists the package self glyph %q beside %q, which belongs to that package; keep the member glyphs and drop the package self glyph, or keep the package self glyph when the card changes the whole package",
					c.Number, container, m.ref,
				),
				Ref: m.ref,
			})
		}
	}

	return findings
}

// renameToSideUnits maps every handle some Rename pair claims as its to-side to the UnitPath of that pair's old glyph, whose unit CanonicalizeHandles writes into the handle.
// A pair whose old side does not parse is skipped; glyph-malformed already reports it.
func renameToSideUnits(lang glyph.Language, plan *Plan) map[string]string {
	units := make(map[string]string)
	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if _, disp := lookup(gateHandleClaims, p.New); disp != dispKeep {
				continue
			}
			old, err := parseGlyph(lang, p.Old)
			if err != nil {
				continue
			}
			unitPath, ok := old.UnitPath()
			if !ok {
				continue
			}
			if _, claimed := units[p.New]; !claimed {
				units[p.New] = unitPath
			}
		}
	}
	return units
}

// redundancyOwnerOf classifies one target for redundant-package-target.
// Every owner is keyed by a UnitPath result, so a member, a file and a package compare as paths.
// ok is false for a target that is neither glyph- nor handle-shaped, that fails to parse, or whose handle body is not a member glyph.
func redundancyOwnerOf(lang glyph.Language, renameUnits map[string]string, ref string) (redundancyOwner, bool) {
	_, disp := lookup(gateRedundantTarget, ref)
	if disp != dispKeep {
		return redundancyOwner{}, false
	}

	if body, isHandle := HandleBody(ref); isHandle {
		if unitPath, claimed := renameUnits[ref]; claimed {
			return redundancyOwner{unit: unitPath}, true
		}
		g, err := parseGlyph(lang, body)
		if err != nil || g.IsSelf() {
			return redundancyOwner{}, false
		}
		unitPath, ok := g.UnitPath()
		return redundancyOwner{unit: unitPath}, ok
	}

	g, err := parseGlyph(lang, ref)
	if err != nil {
		return redundancyOwner{}, false
	}
	unitPath, ok := g.UnitPath()
	if !ok {
		return redundancyOwner{}, false
	}
	if !g.IsSelf() {
		return redundancyOwner{unit: unitPath}, true
	}
	if !hasFileExtension(unitPath) {
		return redundancyOwner{unit: unitPath, container: true}, true
	}
	return redundancyOwner{unit: path.Dir(unitPath)}, true
}
