// normalize.go implements plan-format's root:/// card-path resolution rule
// (contracts/specs/loom-plan-spec.md, "Card path resolution: root: and //"): normalizeCardPath
// resolves one raw card path against the plan's root:, and normalizeCard applies it to every path
// field on a single Card.
// Normalization is classifier-gated: normalizeCard and normalizeRefSlice consult classify.go's
// isPathRef before touching an entry, so only path-shaped refs are rewritten — a symbol entry
// (e.g. "shedrecipe.Lookup") passes through verbatim, never picking up a spurious root: prefix.
// ParsePlan calls normalizeCard exactly once per card, right after that card's body is parsed, so
// every downstream consumer — Validate included — only ever sees plain, forward-slash,
// worktree-relative paths for the refs that are paths at all.

package planparser

import (
	"path"
	"path/filepath"
)

// normalizeCardPath resolves one card file-op path per the plan-format three-case rule: "//" paths are always worktree-root-relative; otherwise join with root unless root is "."; malformed paths (absolute, ".." escapes) are left in place for Validate's card-path-malformed check.
func normalizeCardPath(root, raw string) string {
	if hasWorktreeRootEscape(raw) {
		return cleanPosixPath(raw[2:])
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

// normalizeCard rewrites card.Targets, card.Uses, and both endpoints of every card.Pairs entry in
// place against root, applying normalizeCardPath only to entries isPathRef classifies as paths —
// a symbol entry passes through verbatim. Because card.Targets already carries both endpoints of
// every Pairs entry (parseTypeLabelCase projects them there), normalizing Pairs and Targets
// independently yields the same result on both sides; both normalizations are kept rather than
// deriving one from the other.
func normalizeCard(card *Card, root string) {
	normalizeRefSlice(card.Targets, root)
	normalizeRefSlice(card.Uses, root)
	for i, p := range card.Pairs {
		card.Pairs[i] = MovePair{
			Old: normalizeRefIfPath(root, p.Old),
			New: normalizeRefIfPath(root, p.New),
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
