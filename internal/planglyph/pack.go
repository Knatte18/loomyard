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

// KickstartPack selects the refs cards name, resolves their member glyphs against the tree at worktreeRoot in one call, and returns the pack.
// A ref is selected from an Edit, Delete or Move group, from the old side of a Rename pair and from Uses; a plan: handle under any label is listed unresolved.
// A plain path and a self glyph are left out, and so are Create targets and the new side of a rename.
// Under a plan whose language opts out of glyphs the pack is empty.
// When quarry cannot answer, the pack lists every selected ref as unresolved and the error wraps ErrQuarryUnavailable.
func KickstartPack(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) (planindex.Pack, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return planindex.Pack{}, nil
	}

	var selected []packRef
	var members []string
	for _, ref := range selectPackRefs(cards) {
		if ref.isHandle {
			selected = append(selected, ref)
			continue
		}
		g, err := glyph.Parse(lang, ref.ref)
		if err != nil || g.IsSelf() {
			continue
		}
		selected = append(selected, ref)
		members = append(members, ref.ref)
	}

	var results []quarry.ResolveResult
	var resolveErr error
	if len(members) > 0 {
		results, resolveErr = Resolve(worktreeRoot, members)
	}

	var pack planindex.Pack
	next := 0
	for _, ref := range selected {
		switch {
		case ref.isHandle:
			pack.Unresolved = append(pack.Unresolved, planindex.PackUnresolved{Ref: ref.ref, Reason: packHandleReason})
		case resolveErr != nil:
			pack.Unresolved = append(pack.Unresolved, planindex.PackUnresolved{Ref: ref.ref, Reason: resolveErr.Error()})
		default:
			result := results[next]
			next++
			appendPackAnswer(&pack, ref.ref, result)
		}
	}
	return pack, resolveErr
}

// selectPackRefs lists the refs of cards in selection order, each spelling once: per card the refs of its groups in body order, then its Uses.
func selectPackRefs(cards []planparser.Card) []packRef {
	var refs []packRef
	seen := make(map[string]bool)
	add := func(ref string) {
		if seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, packRef{ref: ref, isHandle: planparser.IsHandleRef(ref)})
	}
	for _, c := range cards {
		for _, group := range c.TargetGroups {
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
		for _, ref := range c.Uses {
			add(ref)
		}
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
