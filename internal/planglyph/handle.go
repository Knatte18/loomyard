// handle.go implements CanonicalizeHandles, the batched-Name boundary that turns every draft
// plan: handle a plan declares — from a Create sub-bullet, or from a Rename pair's computed
// to-side — into its canonical plan:<expected-glyph> form, and rewrites every occurrence across
// the plan on disk in one planparser.RewriteRefs pass. quarry.Name is never called from a
// planning-loop caller and never exposed through the CLI: resolvePass, which calls
// CanonicalizeHandles once, is this function's only caller.

package planglyph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// declSource pairs one quarry.Declaration with the draft handle it derives from, so quarry.Name's
// positional NameResult slice can be matched back to its originating handle by index after the
// batched call.
type declSource struct {
	handle string
	decl   quarry.Declaration
}

// draftHandleMember returns the member-name portion of a plan: handle — the substring after its
// first "#" — local string work over planglyph's own plan: token, never glyph grammar, so it does
// not touch the glyph-conversion-chokepoint Shared Decision.
func draftHandleMember(handle string) (string, bool) {
	idx := strings.Index(handle, "#")
	if idx == -1 {
		return "", false
	}
	return handle[idx+1:], true
}

// renameDeclSource derives a Rename pair's to-side declaration from its resolved Old glyph: the
// Old side's own found declaration text (Symbol.Signature, verbatim), with the declared identifier
// (Symbol.Glyph.Name) swapped for the member name the draft to-side handle itself carries. The
// derived Declaration's Unit is Old's own resolved unit (Symbol.Glyph.Unit) — a Rename keeps its
// symbol in the same package — never the draft handle's own unit half, so a draft that misspells
// the unit is corrected by canonicalization rather than propagated. Only the identifier is taken
// from the draft handle, never its glyph spelling — the spelling is what quarry.Name computes. It
// reports ok false, with a rename-old-unresolved Finding, when Old did not resolve found or the
// new-side handle carries no member name.
func renameDeclSource(oldRef, newHandle string, results map[string]quarry.ResolveResult) (declSource, Finding, bool) {
	r, resolved := results[oldRef]
	if !resolved || r.Status != quarry.StatusFound || len(r.Symbols) == 0 {
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Detail:   fmt.Sprintf("Rename pair's old side %q did not resolve found; nothing to derive the new declaration from", oldRef),
			Severity: SeverityBlocking,
		}, false
	}

	member, ok := draftHandleMember(newHandle)
	if !ok {
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Detail:   fmt.Sprintf("Rename pair's new side %q carries no member name to derive a declaration from", newHandle),
			Severity: SeverityBlocking,
		}, false
	}

	sym := r.Symbols[0]
	decl := strings.Replace(sym.Signature, sym.Glyph.Name, member, 1)
	return declSource{handle: newHandle, decl: quarry.Declaration{Unit: sym.Glyph.Unit, Decl: decl}}, Finding{}, true
}

// CanonicalizeHandles turns every draft plan: handle plan declares into its canonical
// plan:<expected-glyph> form, mechanically, via one batched quarry.Name call, and rewrites every
// occurrence across planDir's card files. results is the same single batched Resolve answer
// resolvePass already collected over the plan's full glyph target set — including every Rename
// pair's Old side, which is glyph-shaped like any other target — so this function issues no
// Resolve call of its own, preserving the one-batched-Resolve invariant.
//
// Declarations are collected from two sources, covered by the one batched Name call: every
// planparser.CardDeclaration across every card (Unit from planparser.HandleUnit, Decl from the
// declaration head verbatim), and every Rename pair whose New side is a handle (its declaration
// computed via renameDeclSource, never trusted from the planner's draft spelling).
//
// Under plan.Language "none" this function returns nil findings and performs no call and no
// rewrite.
func CanonicalizeHandles(plan *planparser.Plan, planDir string, results []quarry.ResolveResult) ([]Finding, error) {
	if _, ok := resolveLanguage(plan); !ok {
		return nil, nil
	}

	resultIndex := resultByTarget(results)

	var findings []Finding
	var sources []declSource

	for _, c := range plan.Cards {
		for _, d := range c.Declarations {
			unit, ok := planparser.HandleUnit(d.Handle)
			if !ok {
				continue // handle-malformed already reports this; nothing to derive.
			}
			sources = append(sources, declSource{handle: d.Handle, decl: quarry.Declaration{Unit: unit, Decl: d.Decl}})
		}
		for _, p := range c.Pairs {
			if !strings.HasPrefix(p.New, planparser.HandlePrefix) {
				continue
			}
			src, finding, ok := renameDeclSource(p.Old, p.New, resultIndex)
			if !ok {
				findings = append(findings, finding)
				continue
			}
			sources = append(sources, src)
		}
	}

	if len(sources) == 0 {
		return findings, nil
	}

	decls := make([]quarry.Declaration, len(sources))
	for i, s := range sources {
		decls[i] = s.decl
	}
	nameResults := quarry.Name(decls)

	canonicalOwners := make(map[string][]string) // canonical form -> every draft handle claiming it
	for i, res := range nameResults {
		src := sources[i]
		if res.Unit != src.decl.Unit || res.Target != src.decl.Decl {
			findings = append(findings, Finding{
				Check:    "handle-name-failed",
				Detail:   fmt.Sprintf("Name result for handle %q did not echo its own input", src.handle),
				Severity: SeverityBlocking,
			})
			continue
		}
		if res.Error != "" {
			findings = append(findings, Finding{
				Check:    "handle-name-failed",
				Detail:   fmt.Sprintf("handle %q failed naming: %s (%s)", src.handle, res.Error, res.Reason),
				Severity: SeverityBlocking,
			})
			continue
		}
		canonical := planparser.HandlePrefix + res.ID
		canonicalOwners[canonical] = append(canonicalOwners[canonical], src.handle)
	}

	canonicals := make([]string, 0, len(canonicalOwners))
	for c := range canonicalOwners {
		canonicals = append(canonicals, c)
	}
	sort.Strings(canonicals)

	subs := make(map[string]string, len(canonicalOwners))
	for _, canonical := range canonicals {
		owners := canonicalOwners[canonical]
		if len(owners) > 1 {
			sort.Strings(owners)
			findings = append(findings, Finding{
				Check:    "handle-canonical-collision",
				Detail:   fmt.Sprintf("handles %s all canonicalize to %q", strings.Join(owners, ", "), canonical),
				Severity: SeverityBlocking,
			})
			continue
		}
		subs[owners[0]] = canonical
	}

	if len(subs) == 0 {
		return findings, nil
	}

	if err := planparser.RewriteRefs(planDir, subs); err != nil {
		return findings, err
	}

	return findings, nil
}

// BindHandles turns a handle into the real glyph the card actually created, from the record-batch
// delta rather than from anyone's spelling: for each completed card's own Create declarations, its
// canonical handle's expected glyph — the substring after planparser.HandlePrefix, already
// canonicalized by CanonicalizeHandles before this call ever runs — is matched against delta's
// Created symbols by Symbol.ID.
//
// A card's whole set of handles binds together or not at all: a count mismatch — a card declaring
// N handles whose delta matches fewer — is the blocking finding bind-count-mismatch, and suppresses
// the rewrite for every one of that card's handles, matched or not, so the plan is never half-bound.
// A handle whose expected glyph matches nothing at all is exactly the case that produces the
// mismatch; it degrades to card 37's candidate path rather than binding silently, and this
// function raises no separate finding for it beyond the card's own bind-count-mismatch.
//
// Every card that binds cleanly contributes its substitutions to one shared map, applied through
// exactly one planparser.RewriteRefs(planDir, subs) call across the whole plan — never one call per
// card — so the plan bytes are rewritten once and a handle's rewrite reaches every referencing
// card, not only its declaring one.
//
// Under plan.Language "none" this function returns nil findings and performs no call and no
// rewrite, mirroring CanonicalizeHandles.
func BindHandles(plan *planparser.Plan, planDir string, delta quarry.GitDeltaAnswer, cards []planparser.Card) ([]Finding, error) {
	if _, ok := resolveLanguage(plan); !ok {
		return nil, nil
	}

	created := make(map[string]bool, len(delta.Created))
	for _, s := range delta.Created {
		created[s.ID] = true
	}

	var findings []Finding
	subs := make(map[string]string)

	for _, c := range cards {
		if len(c.Declarations) == 0 {
			continue
		}

		cardSubs := make(map[string]string, len(c.Declarations))
		matched := 0
		for _, d := range c.Declarations {
			expected := strings.TrimPrefix(d.Handle, planparser.HandlePrefix)
			if created[expected] {
				matched++
				cardSubs[d.Handle] = expected
			}
		}

		if matched < len(c.Declarations) {
			findings = append(findings, Finding{
				Check:    "bind-count-mismatch",
				Card:     cardIDOf(c),
				Detail:   fmt.Sprintf("card %d declared %d handle(s) but the record-batch delta matched only %d", c.Number, len(c.Declarations), matched),
				Severity: SeverityBlocking,
			})
			continue // Suppress the rewrite for this card entirely; never half-bound.
		}

		for h, id := range cardSubs {
			subs[h] = id
		}
	}

	if len(subs) == 0 {
		return findings, nil
	}

	if err := planparser.RewriteRefs(planDir, subs); err != nil {
		return findings, err
	}

	return findings, nil
}
