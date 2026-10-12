// pack.go builds a batch's kick-start pack: the member glyphs its cards name, resolved through quarry, and the refs that did not resolve.

package planglyph

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// packHandleReason is the reason a plan: handle sits in the pack unresolved.
const packHandleReason = "names a member a card creates and does not resolve yet"

// packRef is one ref of the pack's selection, in selection order; isHandle marks a plan: handle.
type packRef struct {
	ref      string
	isHandle bool
}

// KickstartPack selects the refs each card names, resolves the batch's distinct member glyphs against the tree at worktreeRoot in one call, and returns one pack per card, in the order of cards.
// A ref is selected from an Edit, Delete or Move group, from the old side of a Rename pair and from Uses; a plan: handle under any label is listed unresolved.
// A plain path and a self glyph are left out, and so are Create targets and the new side of a rename.
// A ref two cards name appears under both.
// Under a plan whose language opts out of glyphs the result is nil.
// When quarry cannot answer, every card lists its selected refs as unresolved and the error wraps ErrQuarryUnavailable.
func KickstartPack(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]planindex.CardPack, error) {
	return kickstartPack(plan, cards, func(targets []string) ([]quarry.ResolveResult, error) {
		return Resolve(worktreeRoot, targets)
	})
}

// kickstartPack is KickstartPack over resolve, the quarry boundary: it is called once with the batch's distinct member refs in first-seen order, and never when there are none.
func kickstartPack(plan *planparser.Plan, cards []planparser.Card, resolve func(targets []string) ([]quarry.ResolveResult, error)) ([]planindex.CardPack, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil, nil
	}

	selected := make([][]packRef, len(cards))
	var members []string
	seenMember := make(map[string]bool)
	for i, c := range cards {
		for _, ref := range selectPackRefs(c) {
			if ref.isHandle {
				selected[i] = append(selected[i], ref)
				continue
			}
			g, err := glyph.Parse(lang, ref.ref)
			if err != nil || g.IsSelf() {
				continue
			}
			selected[i] = append(selected[i], ref)
			if !seenMember[ref.ref] {
				seenMember[ref.ref] = true
				members = append(members, ref.ref)
			}
		}
	}

	var resolveErr error
	answers := make(map[string]quarry.ResolveResult, len(members))
	if len(members) > 0 {
		var results []quarry.ResolveResult
		results, resolveErr = resolve(members)
		if resolveErr == nil {
			for i, member := range members {
				answers[member] = results[i]
			}
		}
	}

	packs := make([]planindex.CardPack, len(cards))
	for i, c := range cards {
		packs[i].Card = c
		for _, ref := range selected[i] {
			switch {
			case ref.isHandle:
				packs[i].Pack.Unresolved = append(packs[i].Pack.Unresolved, planindex.PackUnresolved{Ref: ref.ref, Reason: packHandleReason})
			case resolveErr != nil:
				packs[i].Pack.Unresolved = append(packs[i].Pack.Unresolved, planindex.PackUnresolved{Ref: ref.ref, Reason: resolveErr.Error()})
			default:
				appendPackAnswer(&packs[i].Pack, ref.ref, answers[ref.ref])
			}
		}
	}
	return packs, resolveErr
}

// selectPackRefs lists the refs of card in selection order, each spelling once: the refs of its groups in body order, then its Uses.
func selectPackRefs(card planparser.Card) []packRef {
	var refs []packRef
	seen := make(map[string]bool)
	add := func(ref string) {
		if seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, packRef{ref: ref, isHandle: planparser.IsHandleRef(ref)})
	}
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				add(pair.Old)
				if planparser.IsHandleRef(pair.New) {
					add(pair.New)
				}
			}
		case planparser.CardTypeCreate:
			for _, ref := range group.Refs {
				if planparser.IsHandleRef(ref) {
					add(ref)
				}
			}
		default:
			for _, ref := range group.Refs {
				add(ref)
			}
		}
	}
	for _, ref := range card.Uses {
		add(ref)
	}
	return refs
}

// appendPackAnswer files one member's resolve answer into pack: found and multipart as an entry with a declaration per quarry symbol (a var or const keeping the first line of its signature only), anything else as unresolved with quarry's status, or with the grammar rejection's error and reason.
func appendPackAnswer(pack *planindex.Pack, ref string, result quarry.ResolveResult) {
	switch result.Status {
	case quarry.StatusFound, quarry.StatusMultipart:
		entry := planindex.PackEntry{Glyph: ref}
		for _, symbol := range result.Symbols {
			signature := symbol.Signature
			if symbol.Kind == quarry.KindVar || symbol.Kind == quarry.KindConst {
				firstLine, _, _ := strings.Cut(signature, "\n")
				signature = strings.TrimRight(firstLine, " \t\r")
			}
			entry.Declarations = append(entry.Declarations, planindex.PackDeclaration{
				File:      symbol.File,
				Start:     symbol.Start,
				End:       symbol.End,
				Signature: signature,
			})
		}
		pack.Resolved = append(pack.Resolved, entry)
	default:
		reason := string(result.Status)
		if result.Rejected() {
			reason = fmt.Sprintf("rejected before resolution: error %s, reason %s", result.Error, result.Reason)
		}
		pack.Unresolved = append(pack.Unresolved, planindex.PackUnresolved{Ref: ref, Reason: reason})
	}
}
