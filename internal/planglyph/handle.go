// handle.go implements CanonicalizeHandles, the batched-Name boundary that turns every draft
// plan: handle a plan declares — from a Create sub-bullet, or from a Rename pair's computed
// to-side — into its canonical plan:<expected-glyph> form, and rewrites every occurrence across
// the plan on disk in one planparser.RewriteRefs pass. quarry.Name is never called from a
// planning-loop caller and never exposed through the CLI: resolvePass, which calls
// CanonicalizeHandles once, is this function's only caller.

package planglyph

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// declSource pairs one quarry.Declaration with the draft handle it derives from, plus the card that
// declares it, so quarry.Name's positional NameResult slice can be matched back to its originating
// handle by index after the batched call and any resulting finding can name the card an operator
// has to go and edit.
type declSource struct {
	handle string
	card   string
	decl   quarry.Declaration
	// resign marks a source built from an Edit card's re-sign arrow rather than from a handle: handle is then the arrow's member glyph, and the answer is only compared with it.
	resign bool
}

// identifierPattern caches one compiled word-boundary matcher per identifier, so renameSignature
// does not recompile the same pattern for every Rename pair in a plan.
var identifierPattern sync.Map // string -> *regexp.Regexp

// identifierMatcher returns a matcher for name as a whole word, so a declaration whose receiver
// type merely CONTAINS the identifier is not mistaken for the identifier itself.
func identifierMatcher(name string) *regexp.Regexp {
	if cached, ok := identifierPattern.Load(name); ok {
		return cached.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	identifierPattern.Store(name, re)
	return re
}

// renameSignature rewrites signature's own declared identifier from oldName to newName, and reports
// whether it found one to rewrite.
//
// It is deliberately not a plain first-occurrence replace. A method's signature carries its receiver
// clause verbatim — quarry's Symbol.Signature runs from the declaration's first byte to its body —
// so the first textual occurrence of the identifier is frequently inside the receiver TYPE rather
// than at the declared name: "func (c *Counter) Count() int" renamed to Tally became
// "func (c *Counter.Tallyer) Count() int", which no method could survive. Two rules fix that: the
// search starts after the receiver clause when one is present, and it matches the identifier only
// as a whole word.
func renameSignature(signature, oldName, newName string) (string, bool) {
	searchFrom := 0
	if strings.HasPrefix(signature, "func (") {
		// A Go receiver clause admits no nested parentheses — a type parameter list uses brackets —
		// so the first ")" closes it, and the declared name is the next identifier after it.
		if close := strings.Index(signature, ")"); close != -1 {
			searchFrom = close + 1
		}
	}

	loc := identifierMatcher(oldName).FindStringIndex(signature[searchFrom:])
	if loc == nil {
		return "", false
	}
	start, end := searchFrom+loc[0], searchFrom+loc[1]
	return signature[:start] + newName + signature[end:], true
}

// renameDeclSource derives a Rename pair's to-side declaration from its resolved Old glyph: the
// Old side's own found declaration text (Symbol.Signature, verbatim), with the declared identifier
// (Symbol.Glyph.Name) swapped for the bare identifier the draft to-side handle itself carries, via
// renameSignature. The derived Declaration's Unit is Old's own resolved unit (Symbol.Glyph.Unit) —
// a Rename keeps its symbol in the same package — never the draft handle's own unit half, so a
// draft that misspells the unit is corrected by canonicalization rather than propagated. Only the
// identifier is taken from the draft handle, never its glyph spelling — the spelling is what
// quarry.Name computes.
// A var or const carries its whole spec as its signature, which can declare several names and which quarry.Name rejects then;
// its declaration is derived as a one-name spec from the kind and the new identifier alone, since the declaration only feeds quarry.Name's glyph prediction.
// An interface method's signature is its bare method spec, with no "func" and no receiver, which quarry.Name rejects;
// the declaration is then derived as a method on the symbol's owner, "func (Owner) Spec".
// It reports ok false, with a rename-old-unresolved Finding, when Old did not resolve found, when Old resolved found but carries no symbol declaration (a self glyph's answer — a file or unit, not a symbol), when the new-side handle carries no member name, or when Old's own signature carries no occurrence of the identifier it is supposed to declare.
//
// The r.Status != quarry.StatusFound check below is Found-only by design, not an oversight left
// over from before the vocabulary was widened: StatusMultipart is deliberately excluded alongside
// StatusNotFound/StatusAmbiguous/the fail-closed default, because accepting a multipart answer would
// mean arbitrarily choosing r.Symbols[0] as "the" declaration to rename from, out of several
// declarations the language itself allows to differ from each other. A Rename pair's old side must
// name exactly one declaration for renameSignature to have anything unambiguous to derive from.
func renameDeclSource(card, oldRef, newHandle, worktreeRoot string, results map[string]quarry.ResolveResult) (declSource, Finding, bool) {
	r, resolved := results[oldRef]
	if !resolved || r.Status != quarry.StatusFound {
		detail := fmt.Sprintf("Rename pair's old side %q did not resolve found; nothing to derive the new declaration from", oldRef)
		if resolved && r.Status == quarry.StatusAmbiguous {
			if classified := classifyAmbiguity(worktreeRoot, r.Candidates); classified.Partitioned {
				detail = partitionedRenameOldDetail(oldRef, classified.Candidates)
			}
		}
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Card:     card,
			Detail:   detail,
			Severity: SeverityBlocking,
		}, false
	}
	if len(r.Symbols) == 0 {
		// A found answer with no Symbols is a SELF glyph's answer (a found self glyph carries a
		// Listing instead) — a shape mistake, not a resolution failure, and the old detail's "did
		// not resolve found" sent the operator at a resolution problem for what was a wrong-shaped
		// pair. checkRenamePairShape now refuses this pair up front; this arm stays as the resolve
		// pass's own accurate fail-closed answer (crucible round fable-high-r10, F5).
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Card:     card,
			Detail:   fmt.Sprintf("Rename pair's old side %q resolved found but names a file or unit, not a symbol; a symbol rename's old side must be a member glyph", oldRef),
			Severity: SeverityBlocking,
		}, false
	}

	identifier, ok := planparser.HandleIdentifier(newHandle)
	if !ok {
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Card:     card,
			Detail:   fmt.Sprintf("Rename pair's new side %q carries no member name to derive a declaration from", newHandle),
			Severity: SeverityBlocking,
		}, false
	}

	sym := r.Symbols[0]
	switch sym.Kind {
	case quarry.KindVar:
		return declSource{handle: newHandle, decl: quarry.Declaration{Unit: sym.Glyph.Unit, Decl: "var " + identifier + " int"}}, Finding{}, true
	case quarry.KindConst:
		return declSource{handle: newHandle, decl: quarry.Declaration{Unit: sym.Glyph.Unit, Decl: "const " + identifier + " = 0"}}, Finding{}, true
	}
	signature := sym.Signature
	if isInterfaceMethod(sym) {
		signature = "func (" + strings.Join(sym.Glyph.Owner, ".") + ") " + signature
	}
	decl, renamed := renameSignature(signature, sym.Glyph.Name, identifier)
	if !renamed {
		return declSource{}, Finding{
			Check:    "rename-old-unresolved",
			Card:     card,
			Detail:   fmt.Sprintf("Rename pair's old side %q declares %q, which does not appear in its own signature %q; no declaration can be derived", oldRef, sym.Glyph.Name, sym.Signature),
			Severity: SeverityBlocking,
		}, false
	}
	return declSource{handle: newHandle, decl: quarry.Declaration{Unit: sym.Glyph.Unit, Decl: decl}}, Finding{}, true
}

// partitionedRenameOldDetail is the rename-old-unresolved detail for an old side declared once per build-constraint set: it names each candidate's file and constraint, and the two ways forward.
// One declaration head serves every file, since the head only computes the new glyph and partitioned declarations share one glyph, so a Create per file is never the way forward.
func partitionedRenameOldDetail(oldRef string, candidates []constrainedCandidate) string {
	return fmt.Sprintf(
		"Rename pair's old side %q is declared once per build-constraint set (%s), and a rename derives its new declaration from exactly one; "+
			"delete the old member and create the new member's handle once, with one declaration head, on a card that edits each of the old member's files, or edit each file's body instead; "+
			"a Create per file is no way forward, since a second claim on the handle is handle-collision",
		oldRef, candidateList(candidates),
	)
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
// Every re-sign arrow whose target is a member glyph adds a third source to the same call: the glyph's own unit and the arrow's head.
// Its answer is compared with the glyph and discarded, never entering the canonical-owner bookkeeping or the rewrite, since the head names an existing member rather than a handle.
// A naming error, or an ID other than the glyph's own, is the blocking resign-head-mismatch.
//
// Under plan.Language "none" this function returns nil findings and performs no call and no
// rewrite.
//
// Its second return reports whether planDir's bytes were actually handed to RewriteRefs, so
// resolvePass knows whether re-reading the plan from disk can tell it anything new -- and therefore
// whether a failure to re-read it is a real infrastructure failure or merely an in-memory plan that
// was never on disk to begin with.
func CanonicalizeHandles(plan *planparser.Plan, planDir, worktreeRoot string, results []quarry.ResolveResult) ([]Finding, bool, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil, false, nil
	}

	resultIndex := resultByTarget(results)

	var findings []Finding
	var sources []declSource

	for _, c := range plan.Cards {
		card := c.ID()
		for _, d := range c.Declarations {
			unit, ok := planparser.HandleUnit(d.Handle)
			if !ok {
				continue // handle-malformed already reports this; nothing to derive.
			}
			sources = append(sources, declSource{handle: d.Handle, card: card, decl: quarry.Declaration{Unit: unit, Decl: d.Decl}})
		}
		for _, p := range c.Pairs {
			if !planparser.IsHandleRef(p.New) {
				continue
			}
			src, finding, ok := renameDeclSource(card, p.Old, p.New, worktreeRoot, resultIndex)
			if !ok {
				findings = append(findings, finding)
				continue
			}
			src.card = card
			sources = append(sources, src)
		}
		for _, r := range c.Resigns {
			if planparser.IsHandleRef(r.Target) {
				continue
			}
			g, err := glyph.Parse(lang, r.Target)
			if err != nil || g.IsSelf() {
				continue // resign-not-member and glyph-malformed already report these.
			}
			sources = append(sources, declSource{
				handle: g.String(),
				card:   card,
				decl:   quarry.Declaration{Unit: g.Unit, Decl: r.Decl},
				resign: true,
			})
		}
	}

	if len(sources) == 0 {
		return findings, false, nil
	}

	decls := make([]quarry.Declaration, len(sources))
	for i, s := range sources {
		decls[i] = s.decl
	}
	nameResults := quarry.Name(decls)
	// quarry.Name's contract is one positionally-matched result per declaration, and the per-result
	// echo check below is what catches a MISALIGNED answer — but it can only run after sources[i]
	// has already been indexed, so a longer answer panics before reaching it. A batched boundary
	// this file cannot see inside is exactly where a length guard belongs, and reporting the
	// mismatch as an infrastructure error keeps it out of the plan-finding vocabulary: nobody should
	// read "quarry answered a different number of declarations" as a defect in the plan.
	if len(nameResults) != len(sources) {
		return findings, false, fmt.Errorf("%w: quarry.Name returned %d result(s) for %d declaration(s)", ErrQuarryUnavailable, len(nameResults), len(sources))
	}

	canonicalOwners := make(map[string][]string) // canonical form -> every draft handle claiming it
	for i, res := range nameResults {
		src := sources[i]
		if res.Unit != src.decl.Unit || res.Target != src.decl.Decl {
			findings = append(findings, Finding{
				Check:    "handle-name-failed",
				Card:     src.card,
				Detail:   fmt.Sprintf("Name result for handle %q did not echo its own input", src.handle),
				Severity: SeverityBlocking,
			})
			continue
		}
		if src.resign {
			if res.Error != "" || res.ID != src.handle {
				findings = append(findings, resignHeadMismatch(src, res))
			}
			continue
		}
		if res.Error != "" {
			findings = append(findings, Finding{
				Check:    "handle-name-failed",
				Card:     src.card,
				Detail:   fmt.Sprintf("handle %q failed naming its declaration %q: %s (%s)", src.handle, src.decl.Decl, res.Error, res.Reason),
				Severity: SeverityBlocking,
			})
			continue
		}
		canonical := planparser.NewHandle(res.ID)
		canonicalOwners[canonical] = append(canonicalOwners[canonical], src.handle)
	}

	canonicals := make([]string, 0, len(canonicalOwners))
	for c := range canonicalOwners {
		canonicals = append(canonicals, c)
	}
	sort.Strings(canonicals)

	// The inverse of handle-canonical-collision below: ONE draft handle owning MORE than one
	// canonical, which arises when the same handle spelling is claimed by two declaring sources
	// deriving different declarations (a Create declaration and a Rename to-side, or two Rename
	// to-sides). Every such plan already carries the blocking pure handle-collision finding
	// (handleClaims counts both sources since crucible round opus-high-r9, R9-3), so the refusal is
	// owned there — but without this guard the subs loop below assigned subs[draft] once per
	// canonical, last-wins over sort order, and RewriteRefs then mutated the plan ON DISK with an
	// arbitrarily chosen canonical before that finding was ever rendered (crucible round
	// fable-high-r10, F6). A draft in this set contributes no substitution at all.
	canonicalCountOfDraft := make(map[string]int)
	for _, owners := range canonicalOwners {
		for _, o := range owners {
			canonicalCountOfDraft[o]++
		}
	}

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
		if canonicalCountOfDraft[owners[0]] > 1 {
			continue
		}
		// An already-canonical handle produces an identity pair here — every validation pass after
		// the first, since canonicalization is idempotent. Substituting it anyway rewrote every
		// declaring card file with identical bytes and reported rewrote=true, forcing resolvePass
		// into a pointless full re-parse on every begin-batch for the plan's whole life.
		if owners[0] == canonical {
			continue
		}
		subs[owners[0]] = canonical
	}

	if len(subs) == 0 {
		return findings, false, nil
	}

	if err := planparser.RewriteRefs(planDir, subs); err != nil {
		return findings, false, err
	}

	return findings, true, nil
}

// resignHeadMismatch is the blocking resign-head-mismatch finding for a re-sign arrow whose head quarry.Name could not name, or named as a member other than the arrow's own glyph.
func resignHeadMismatch(src declSource, res quarry.NameResult) Finding {
	answer := fmt.Sprintf("answered %q", res.ID)
	if res.Error != "" {
		answer = fmt.Sprintf("failed: %s (%s)", res.Error, res.Reason)
	}
	return Finding{
		Check: "resign-head-mismatch",
		Card:  src.card,
		Detail: fmt.Sprintf(
			"re-sign head %q does not name the member %q: naming it %s; write the member's new declaration head, receiver included for a method",
			src.decl.Decl, src.handle, answer,
		),
		Severity: SeverityBlocking,
		Ref:      src.handle,
	}
}

// cardOwnHandles returns every plan: handle a card's own Declarations AND Rename pairs declare, in
// body order: a Create declaration's own Handle, plus any Rename pair whose New side is still
// handle-shaped (a file-rename pair's New side is already a self glyph, never a handle, and is
// skipped here — nothing to bind).
//
// Folding a Rename pair's New side in alongside Create declarations is what closes the gap a Rename-only card fell into before this fix (crucible round sonnet-xhigh-r8, PG-2): a card carrying no Create group has an empty Declarations, so a BindHandles keyed on Declarations alone skipped it entirely, and its own New-side handle never lost its "plan:" prefix — permanently invisible to collectGlyphTargets, which excludes anything plan:-prefixed by construction, for every later card that legitimately referenced the renamed symbol.
func cardOwnHandles(c planparser.Card) []string {
	handles := make([]string, 0, len(c.Declarations)+len(c.Pairs))
	for _, d := range c.Declarations {
		handles = append(handles, d.Handle)
	}
	for _, p := range c.Pairs {
		if planparser.IsHandleRef(p.New) {
			handles = append(handles, p.New)
		}
	}
	return handles
}

// BindHandles turns a handle into the real glyph the card actually created or renamed to, from the
// record-batch delta rather than from anyone's spelling: for each completed card's own handles (its
// Create declarations, per cardOwnHandles, and any Rename pair whose New side is still
// handle-shaped), the canonical handle's expected glyph — the substring after
// planparser.HandlePrefix, already canonicalized by CanonicalizeHandles before this call ever runs
// — is matched against delta's Created symbols by Symbol.ID for a Create declaration, or against
// delta's Renamed pairs' own To.ID for a Rename pair's New side; the two sources are checked
// against different delta fields because a rename is not a create.
//
// A card's whole set of handles binds together or not at all: a count mismatch — a card owning N
// handles whose delta matches fewer — is the blocking finding bind-count-mismatch, and suppresses
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
	if _, ok := plan.GlyphLanguage(); !ok {
		return nil, nil
	}

	created := make(map[string]bool, len(delta.Created))
	for _, s := range delta.Created {
		created[s.ID] = true
	}
	renamedTo := make(map[string]bool, len(delta.Renamed))
	for _, rp := range delta.Renamed {
		renamedTo[rp.To.ID] = true
	}
	// A handle matches if EITHER delta source produced its expected glyph: a Create declaration's
	// own expected ID is checked against created, a Rename pair's own expected ID against
	// renamedTo, but a handle read from cardOwnHandles carries no tag saying which source declared
	// it, so either match is accepted here rather than routed by source. The same handle spelling
	// appearing in both a Declarations entry and a Rename pair is the blocking pure finding
	// handle-collision (planparser's checkHandleConsistency, which counts BOTH declaring sources
	// since crucible round opus-high-r9's R9-3), so such a plan never reaches this function and the
	// two-source acceptance here can never mask a real mismatch.
	bound := func(expected string) bool {
		return created[expected] || renamedTo[expected]
	}

	var findings []Finding
	subs := make(map[string]string)

	for _, c := range cards {
		handles := cardOwnHandles(c)
		if len(handles) == 0 {
			continue
		}

		cardSubs := make(map[string]string, len(handles))
		matched := 0
		for _, h := range handles {
			expected := resolveKeyFor(h)
			if bound(expected) {
				matched++
				cardSubs[h] = expected
			}
		}

		if matched < len(handles) {
			findings = append(findings, Finding{
				Check:    "bind-count-mismatch",
				Card:     c.ID(),
				Detail:   fmt.Sprintf("card %d owns %d handle(s) but the record-batch delta matched only %d", c.Number, len(handles), matched),
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
