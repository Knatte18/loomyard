// resign.go names every Edit re-sign arrow's head before the plan's batched resolve and checks the accepted C# arrows against one another and against the tree:
// the resign-head-mismatch and resign-overload-collision checks.

package planglyph

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// resignName is one accepted re-sign arrow: its card, the glyph it re-signs, its declaration head, and the id quarry minted for that head.
// The id is the target's own for a Go arrow, and differs from it by at most the parameter list for a C# arrow.
type resignName struct {
	card   string
	target glyph.Glyph
	head   string
	id     string
}

// resignArrow is one re-sign arrow awaiting its name, with the declaration quarry is asked to name.
type resignArrow struct {
	card   string
	target glyph.Glyph
	head   string
	decl   quarry.Declaration
}

// nameResigns names the head of every Edit re-sign arrow on plan's cards whose target ParseRef parses to a member glyph, in one batched quarry call.
// It returns each accepted arrow, and the blocking resign-head-mismatch finding for every arrow whose head quarry cannot name, or names as a member the arrow does not re-sign.
// A Go arrow is accepted when the minted id is its target's.
// A C# arrow is accepted when the minted id is its target's apart from the parameter list, with both written with parentheses or both without.
// An error is ErrQuarryUnavailable-wrapped, since only quarry's own call can fail.
func nameResigns(plan *planparser.Plan) ([]resignName, []Finding, error) {
	var arrows []resignArrow
	for _, c := range plan.Cards {
		for _, r := range c.Resigns {
			if planparser.IsHandleRef(r.Target) {
				continue
			}
			g, err := planparser.ParseRef(plan, r.Target)
			if err != nil || g.IsSelf() {
				continue // resign-not-member and glyph-malformed already report these.
			}
			decl := quarry.Declaration{Lang: g.Lang, Unit: g.Unit, Decl: r.Decl}
			if g.Lang == glyph.CSharp {
				decl.Owner = csharpOwner(g, r.Decl)
			}
			arrows = append(arrows, resignArrow{card: c.ID(), target: g, head: r.Decl, decl: decl})
		}
	}
	if len(arrows) == 0 {
		return nil, nil, nil
	}

	decls := make([]quarry.Declaration, len(arrows))
	for i, a := range arrows {
		decls[i] = a.decl
	}
	results, err := nameDeclarations(decls)
	if err != nil {
		return nil, nil, err
	}

	var names []resignName
	var findings []Finding
	for i, res := range results {
		a := arrows[i]
		if res.Unit != a.decl.Unit || res.Target != a.decl.Decl {
			findings = append(findings, Finding{
				Check:    "handle-name-failed",
				Card:     a.card,
				Detail:   fmt.Sprintf("Name result for handle %q did not echo its own input", a.target.String()),
				Severity: SeverityBlocking,
			})
			continue
		}
		if !resignAccepted(a.target, res) {
			findings = append(findings, resignHeadMismatch(a.card, a.target, a.head, res))
			continue
		}
		names = append(names, resignName{card: a.card, target: a.target, head: a.head, id: res.ID})
	}
	return names, findings, nil
}

// resignAccepted reports whether res names the member target re-signs.
func resignAccepted(target glyph.Glyph, res quarry.NameResult) bool {
	if res.Error != "" {
		return false
	}
	if target.Lang != glyph.CSharp {
		return res.ID == target.String()
	}
	minted, err := glyph.Parse(glyph.CSharp, res.ID)
	return err == nil && sameMemberIgnoringParams(target, minted) && (minted.Params == nil) == (target.Params == nil)
}

// sameMemberIgnoringParams reports whether a and b have the same unit, owner chain and name.
func sameMemberIgnoringParams(a, b glyph.Glyph) bool {
	return a.Unit == b.Unit && a.Name == b.Name && slices.Equal(a.Owner, b.Owner)
}

// resignHeadMismatch is the blocking resign-head-mismatch finding for a re-sign arrow whose head quarry.Name could not name, or named as a member other than the arrow's own glyph.
// Its detail names the way forward for the arrow's language and for how the head failed.
func resignHeadMismatch(card string, target glyph.Glyph, head string, res quarry.NameResult) Finding {
	answer := fmt.Sprintf("answered %q", res.ID)
	if res.Error != "" {
		answer = fmt.Sprintf("failed: %s (%s)", res.Error, res.Reason)
	}
	way := "write the member's new declaration head, receiver included for a method"
	if target.Lang == glyph.CSharp {
		way = csharpResignWay(target, head, res)
	}
	return Finding{
		Check:    "resign-head-mismatch",
		Card:     card,
		Detail:   fmt.Sprintf("re-sign head %q does not name the member %q: naming it %s; %s", head, target.String(), answer, way),
		Severity: SeverityBlocking,
		Ref:      target.String(),
	}
}

// csharpResignWay is the way forward for a C# re-sign head that res shows is not accepted.
func csharpResignWay(target glyph.Glyph, head string, res quarry.NameResult) string {
	if res.Error != "" {
		way := "correct the head to one quarry names under the owner chain"
		if res.Reason == quarry.NameReasonInvalidContainer {
			way += canonicalQualifierWayOf(target, head)
		}
		return way
	}
	if minted, err := glyph.Parse(glyph.CSharp, res.ID); err == nil && sameMemberIgnoringParams(target, minted) {
		return "keep the member's parameter list in the head, or change the member's kind through a Delete and a Create"
	}
	return "use a Rename pair for a name change, or write a head that declares the member"
}

// mintedResignIDs returns the ids of the accepted C# arrows that differ from their own target, sorted and distinct.
// Each is a member the arrow's head would declare beside the existing ones, so it is resolved to find an overload it would collide with.
func mintedResignIDs(names []resignName) []string {
	var ids []string
	for _, n := range names {
		if n.target.Lang == glyph.CSharp && n.id != n.target.String() {
			ids = append(ids, n.id)
		}
	}
	sort.Strings(ids)
	return slices.Compact(ids)
}

// resignCollisionFindings reports the blocking resign-overload-collision for each accepted C# arrow whose minted id differs from its target and collides with another declaration:
// an overload that resolves found, a Create or Rename handle that binds to the same id, or another arrow's minted id on a different target.
// A collision is attributed to the later of the two cards and its detail names the earlier one.
// A collision is lifted when a card in the span between the two declarations, the later one included, deletes the colliding member, renames it away or re-signs it away.
// For an overload that resolves found, that span runs from the first card through the arrow's own.
// plan is the current plan, answers the batched resolve's answers by target, and names every accepted arrow.
func resignCollisionFindings(plan *planparser.Plan, names []resignName, answers map[string]quarry.ResolveResult) []Finding {
	type declaredHandle struct {
		cardIndex int
		body      string
	}
	var handles []declaredHandle
	cardIndex := make(map[string]int, len(plan.Cards))
	for i, c := range plan.Cards {
		cardIndex[c.ID()] = i
		for _, d := range c.Declarations {
			if body, ok := planparser.HandleBody(d.Handle); ok {
				handles = append(handles, declaredHandle{i, body})
			}
		}
		for _, p := range c.Pairs {
			if body, ok := planparser.HandleBody(p.New); ok {
				handles = append(handles, declaredHandle{i, body})
			}
		}
	}

	retiredWithin := func(from, to int, id string) bool {
		for k := from; k <= to && k < len(plan.Cards); k++ {
			if cardRetires(plan.Cards[k], names, id) {
				return true
			}
		}
		return false
	}

	var findings []Finding
	for i, arrow := range names {
		if arrow.target.Lang != glyph.CSharp || arrow.id == arrow.target.String() {
			continue
		}
		at, ok := cardIndex[arrow.card]
		if !ok {
			continue
		}
		collision := func(later int, detail string) Finding {
			return Finding{
				Check:    "resign-overload-collision",
				Card:     plan.Cards[later].ID(),
				Detail:   detail,
				Severity: SeverityBlocking,
				Ref:      arrow.target.String(),
			}
		}

		if answer, resolved := answers[arrow.id]; resolved && len(answer.Symbols) > 0 && !retiredWithin(0, at, arrow.id) {
			findings = append(findings, collision(at, fmt.Sprintf(
				"re-sign head %q mints %q, the id of an overload that already exists; change the head to a free signature, or remove or re-sign that overload on an earlier card",
				arrow.head, arrow.id)))
			continue
		}

		collided := false
		for _, h := range handles {
			if h.body != arrow.id {
				continue
			}
			earlier, later := min(h.cardIndex, at), max(h.cardIndex, at)
			if retiredWithin(earlier+1, later, arrow.id) {
				continue
			}
			findings = append(findings, collision(later, fmt.Sprintf(
				"re-sign head %q on card %s mints %q, which the handle declared on card %s binds to; change the head to a free signature, or drop one of the two declarations",
				arrow.head, arrow.card, arrow.id, plan.Cards[h.cardIndex].ID())))
			collided = true
			break
		}
		if collided {
			continue
		}

		for j, other := range names {
			if other.id != arrow.id || other.target.String() == arrow.target.String() {
				continue
			}
			otherAt, ok := cardIndex[other.card]
			if !ok || otherAt > at || (otherAt == at && j > i) {
				continue
			}
			if retiredWithin(otherAt+1, at, arrow.id) {
				continue
			}
			findings = append(findings, collision(at, fmt.Sprintf(
				"re-sign head %q mints %q, which the re-sign arrow on %q in card %s mints too; change one head to a free signature",
				arrow.head, arrow.id, other.target.String(), other.card)))
			break
		}
	}
	return findings
}

// cardRetires reports whether card c deletes id, renames it away as a Rename old side, or re-signs it away to another id.
func cardRetires(c planparser.Card, names []resignName, id string) bool {
	for _, group := range c.TargetGroups {
		if group.Type == planparser.CardTypeDelete && slices.Contains(group.Refs, id) {
			return true
		}
	}
	for _, p := range c.Pairs {
		if p.Old == id {
			return true
		}
	}
	for _, n := range names {
		if n.card == c.ID() && n.target.String() == id && n.id != id {
			return true
		}
	}
	return false
}

// RebindResigns carries the C# re-sign arrows of cards, a batch just recorded, into plan.
// It names each arrow's head again, and resolves the minted id of every arrow that differs from its target against worktreeRoot's tree.
// An id that does not resolve found means the card's edit does not match its arrow, which is the blocking resign-not-done finding for that card, and nothing is rewritten.
// When every id resolves found it rewrites each old glyph to its minted id plan-wide in one plan.RewriteRefs call, so a later card naming the member by its old glyph names the minted id.
// A Go arrow, an arrow whose minted id is its target, and a plan whose language opts out of glyphs do nothing.
// An infrastructure error from quarry is wrapped in ErrQuarryUnavailable.
func RebindResigns(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error) {
	if !plan.GlyphLanguage() || plan.GoToolchain() {
		return nil, nil
	}
	ids := make(map[string]bool, len(cards))
	for _, c := range cards {
		ids[c.ID()] = true
	}
	// A head that fails to name is already reported at the plan gates, and names no arrow to rebind here.
	names, _, err := nameResigns(cardsByID(plan, ids))
	if err != nil {
		return nil, err
	}
	minted := mintedResignIDs(names)
	if len(minted) == 0 {
		return nil, nil
	}

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}
	results, err := resolveTargets(repo, minted)
	if err != nil {
		return nil, err
	}
	answers := resultByTarget(results)

	var findings []Finding
	subs := make(map[string]string, len(minted))
	for _, n := range names {
		if n.target.Lang != glyph.CSharp || n.id == n.target.String() {
			continue
		}
		// Fail closed: only a found answer shows the card's edit declares the arrow's head.
		if answer := answers[n.id]; answer.Status != quarry.StatusFound {
			number, _, _ := strings.Cut(n.card, "-")
			findings = append(findings, Finding{
				Check: "resign-not-done",
				Card:  n.card,
				Detail: fmt.Sprintf(
					"re-sign arrow %q -> %q mints %q, which answered %q in the tree; make the edit match the arrow's head, or correct the head to the signature the card wrote and run `lyx webster rebaseline --card %s` naming that card",
					n.target.String(), n.head, n.id, string(answer.Status), number),
				Severity: SeverityBlocking,
				Ref:      n.target.String(),
			})
			continue
		}
		subs[n.target.String()] = n.id
	}
	if len(findings) > 0 {
		return findings, nil
	}
	if err := plan.RewriteRefs(subs); err != nil {
		return nil, fmt.Errorf("rewrite re-signed refs in plan %s: %w", plan.Dir, err)
	}
	return nil, nil
}
