// normalize.go implements plan-format's root:/// card-path resolution rule
// (contracts/specs/loom-plan-spec.md, "Card path resolution: root: and //"): normalizeCardPath
// resolves one raw card path against the plan's root:, and normalizeCard applies it to every path
// field on a single Card — both its card-level Targets/Uses/Pairs and every one of its own
// TargetGroups' Refs/Pairs.
// Normalization is classifier-gated: normalizeCard and normalizeRefSlice consult classify.go's
// isPathRef before touching an entry, so only path-shaped refs are rewritten — a symbol entry
// (e.g. "shedrecipe.Lookup") passes through verbatim, never picking up a spurious root: prefix.
// ParsePlan calls normalizeCard exactly once per card, right after that card's body is parsed, so
// every downstream consumer — Validate included — only ever sees plain, forward-slash,
// worktree-relative paths for the refs that are paths at all.
//
// This file also implements canonicalizeCard, which ParsePlan calls immediately after
// normalizeCard, on the same card, to canonicalize an extension-carrying path-shaped ref into its
// glyph string via the one path->glyph call, glyph.Self — see canonicalizeCard's own doc comment
// for why the ordering and the file-extension gate are both load-bearing.

package planparser

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/quarry/glyph"
)

// normalizeCardPath resolves one card file-op path per the plan-format three-case rule: "//" paths are always worktree-root-relative; otherwise join with root unless root is "."; malformed paths (absolute, ".." escapes) are left in place for Validate's card-path-malformed check.
//
// The empty and single-"/"-prefixed cases are returned UNTOUCHED rather than joined onto root,
// because joining destroys the very marker card-path-malformed keys on. With root: internal/boardcli,
// path.Clean("internal/boardcli" + "/" + "/etc/passwd") collapsed the doubled separator into
// internal/boardcli/etc/passwd — a clean relative path the validator then had nothing to say about —
// and an empty entry became path.Clean("internal/boardcli/") == internal/boardcli, silently naming
// the root directory and making the validator's own "empty entry" branch unreachable whenever a root
// was set. Both are flagged correctly when root is absent or ".", so the guarantee this function's
// own doc states was silently root-dependent (crucible round opus-medium-r6, R6-5).
// A ".." escape needs no such carve-out: path.Clean preserves a leading "..", so it survives the join
// on its own.
func normalizeCardPath(root, raw string) string {
	if hasWorktreeRootEscape(raw) {
		return cleanPosixPath(raw[2:])
	}
	if raw == "" || strings.HasPrefix(raw, "/") {
		return cleanPosixPath(raw)
	}
	if root != "" && root != "." {
		return cleanPosixPath(root + "/" + raw)
	}
	return cleanPosixPath(raw)
}

// hasWorktreeRootEscape reports whether raw carries plan-format's "//"
// worktree-root-relative escape prefix.
func hasWorktreeRootEscape(raw string) bool {
	return len(raw) >= 2 && raw[0] == '/' && raw[1] == '/'
}

// cleanPosixPath converts p to forward slashes and lexically cleans it with path.Clean, preserving malformed markers (leading "..", leading "/") that Validate's card-path-malformed check keys on.
func cleanPosixPath(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(p))
}

// normalizeCard rewrites card.Targets, card.Uses, both endpoints of every card.Pairs entry, and
// every card.TargetGroups[i].Refs/Pairs entry in place against root, applying normalizeCardPath
// only to entries isPathRef classifies as paths — a symbol entry passes through verbatim.
// RenameRaw is never normalized on either side: it holds unparsed sub-bullet text captured
// verbatim so rename-format has something to report.
// The card-level fields and each group's own fields are normalized independently rather than one
// being rebuilt from the other, which is what preserves normalizeRefSlice's nil-vs-empty-slice
// distinction a rebuild-by-concatenation would flatten.
// After the call, Card.Targets equals the concatenation of TargetGroups[*].Refs in body order and
// Card.Pairs equals the concatenation of TargetGroups[*].Pairs in body order, with symbol-shaped
// entries passing through verbatim on both sides.
func normalizeCard(card *Card, root string) {
	normalizeRefSlice(card.Targets, root)
	normalizeRefSlice(card.Uses, root)
	for i, p := range card.Pairs {
		card.Pairs[i] = MovePair{
			Old: normalizeRefIfPath(root, p.Old),
			New: normalizeRefIfPath(root, p.New),
		}
	}
	for i := range card.TargetGroups {
		normalizeRefSlice(card.TargetGroups[i].Refs, root)
		for j, p := range card.TargetGroups[i].Pairs {
			card.TargetGroups[i].Pairs[j] = MovePair{
				Old: normalizeRefIfPath(root, p.Old),
				New: normalizeRefIfPath(root, p.New),
			}
		}
	}
}

// normalizeRefSlice normalizes every path-shaped element of refs in place against root, preserving
// nil vs empty-slice distinction. A symbol-shaped element is left untouched.
func normalizeRefSlice(refs []string, root string) {
	for i, r := range refs {
		refs[i] = normalizeRefIfPath(root, r)
	}
}

// normalizeRefIfPath applies normalizeCardPath to raw only when isPathRef classifies it as a
// path; a symbol-shaped raw is returned unchanged. This is the single sharpest regression this
// migration can introduce: without this gate, a non-empty root: would turn "shedrecipe.Lookup"
// into "internal/boardcli/shedrecipe.Lookup".
func normalizeRefIfPath(root, raw string) string {
	if !isPathRef(raw) {
		return raw
	}
	return normalizeCardPath(root, raw)
}

// hasFileExtension reports whether raw's final path segment (its base name) carries a ".".
// It is one of the two halves canonicalizablePath composes, and it is also checkDirectoryTarget's
// own extension test (validate.go), so the two can never disagree about what "carries an extension"
// means.
func hasFileExtension(raw string) bool {
	base := raw
	if idx := strings.LastIndex(raw, "/"); idx != -1 {
		base = raw[idx+1:]
	}
	return strings.Contains(base, ".")
}

// canonicalizablePath reports whether a path-shaped ref is eligible for glyph canonicalization:
// it carries a file extension, OR it carries no "/" at all.
//
// The extension half is load-bearing, not an optimization: without it, canonicalizeCard would
// rewrite a bare directory path such as "internal/foo" into the perfectly valid unit self glyph
// "internal/foo#", leaving directory-target nothing left to classify.
//
// The slash-free half is what keeps classifyRef's own rule 4 — the extensionless repository-root
// filename ("LICENSE", "Makefile", "Dockerfile") — usable end to end. directory-target only ever
// fires on a ref that CONTAINS a "/" (validate.go), so exempting the slash-free case from the
// extension requirement costs that check nothing while removing the one ref class the classifier
// admits and every glyph-backed layer downstream then chokes on: left as the bare token "LICENSE",
// such a ref validates 100% clean through all twenty-eight pure checks and is then handed verbatim
// to quarry, which rejects it BEFORE resolution ("a glyph needs a \"#\"") — so
// internal/planglyph's DoneChecks read the rejection as "not resolved" and reported a permanent,
// unrecoverable create-not-done against a card that had in fact created the file, while
// checkProsaSymbolTarget reported a false prosa-symbol-target for the very spelling rule 4 exists
// to make legal (crucible round opus-high-r9, R9-1).
// Canonicalized to "LICENSE#" the same ref resolves found, which is the spelling quarry's own
// rejection message recommends.
//
// A slash-free extensionless ref naming a repository-root DIRECTORY rather than a file
// canonicalizes to its unit self glyph ("docs" -> "docs#"), which resolves found exactly as the
// file case does and which checkProsaSymbolTarget already admits as the legal whole-package
// spelling. The narrow prosa-symbol-target nudge that spelling used to get is therefore gone by
// design, not by accident.
func canonicalizablePath(raw string) bool {
	return hasFileExtension(raw) || !strings.Contains(raw, "/")
}

// canonicalizeCard rewrites every path-shaped ref on card that also carries a file extension into
// its canonical glyph string, and records the pre-canonicalization surface lexeme into
// surface[cardKey][canonicalString]. It runs strictly after normalizeCard on the same card, walking
// the identical field set: the card-level Targets, Uses and both endpoints of every Pairs entry,
// and every TargetGroups[i].Refs entry and both endpoints of every TargetGroups[i].Pairs entry — the
// group-level half is not optional, since checkProsaSymbolTarget, checkPathMissing,
// checkCardFieldEmpty and createTargetsUnion all read group Refs rather than the card-level union.
//
// A ref not classified as refKindPath (a glyph, a plan: handle, or a symbol) is left byte-identical:
// a glyph is already a complete repository-relative string, so prefixing it through glyph.Self would
// corrupt it — this is also why a glyph is never root:-joined. A path-shaped ref canonicalizablePath
// declines — a SLASHED extensionless path, i.e. a directory — is left untouched too, per the
// directory-target-preserving gate documented there. A glyph.Self error on an eligible path-shaped
// ref leaves the ref untouched and is not a parse failure: the classification checks from
// classify.go already report a malformed entry, and planparser is deliberately lenient at card
// level.
func canonicalizeCard(card *Card, cardKey string, lang glyph.Language, surface map[string]map[string][]string) {
	canon := func(raw string) string {
		if !isPathRef(raw) || !canonicalizablePath(raw) {
			return raw
		}
		g, err := glyph.Self(lang, raw)
		if err != nil {
			return raw
		}
		canonical := g.String()
		if surface[cardKey] == nil {
			surface[cardKey] = make(map[string][]string)
		}
		// Appended, never overwritten, and deduplicated: one card may spell one canonical ref two
		// ways across two of its own fields, and keeping only the last left the other bullet
		// un-rewritten (R6-13). The same lexeme repeated on the same card yields one entry, so
		// RewriteRefs never builds a duplicate substitution.
		if !slices.Contains(surface[cardKey][canonical], raw) {
			surface[cardKey][canonical] = append(surface[cardKey][canonical], raw)
		}
		return canonical
	}

	canonicalizeRefSlice(card.Targets, canon)
	canonicalizeRefSlice(card.Uses, canon)
	for i, p := range card.Pairs {
		card.Pairs[i] = MovePair{Old: canon(p.Old), New: canon(p.New)}
	}
	for i := range card.TargetGroups {
		canonicalizeRefSlice(card.TargetGroups[i].Refs, canon)
		for j, p := range card.TargetGroups[i].Pairs {
			card.TargetGroups[i].Pairs[j] = MovePair{Old: canon(p.Old), New: canon(p.New)}
		}
	}
}

// canonicalizeRefSlice applies canon to every element of refs in place, preserving the nil vs
// empty-slice distinction normalizeRefSlice already preserves.
func canonicalizeRefSlice(refs []string, canon func(string) string) {
	for i, r := range refs {
		refs[i] = canon(r)
	}
}
