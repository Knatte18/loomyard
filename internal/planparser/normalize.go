// normalize.go implements plan-format-v3's root:/// card-path resolution rule
// (docs/reference/plan-format-v3.md, "Card path resolution: root: and //"):
// normalizeCardPath resolves one raw card path against the plan's root:, and
// normalizeCard applies it to every path field on a single Card. ParsePlan calls
// normalizeCard exactly once per card, right after that card's body is parsed, so
// every downstream consumer — Validate included — only ever sees plain,
// forward-slash, worktree-relative paths.

package planparser

import (
	"path"
	"path/filepath"
)

// normalizeCardPath resolves one card file-op path per the three-case rule
// docs/reference/plan-format-v3.md pins: a "//"-prefixed path always strips
// exactly that prefix and is worktree-root-relative, whether or not root is set;
// otherwise, a non-empty root joins as "<root>/<raw>", except the degenerate
// "root: ." (the worktree root itself), which resolves to raw unchanged rather
// than the unclean "./<raw>" a literal string join would produce. This is
// parse-time normalization of a well-formed degenerate case, not well-formedness
// rejection: an actually malformed root or raw (an absolute path, a ".." escape)
// still reaches Validate's card-path-malformed check, present in the normalized
// result — cleanPosixPath collapses only redundant internal "." /".." segments a
// legitimate path may carry, never a genuine leading escape past the worktree root.
func normalizeCardPath(root, raw string) string {
	if hasWorktreeRootEscape(raw) {
		return cleanPosixPath(raw[2:])
	}
	if root != "" && root != "." {
		return cleanPosixPath(root + "/" + raw)
	}
	return cleanPosixPath(raw)
}

// hasWorktreeRootEscape reports whether raw carries plan-format-v3's "//"
// worktree-root-relative escape prefix.
func hasWorktreeRootEscape(raw string) bool {
	return len(raw) >= 2 && raw[0] == '/' && raw[1] == '/'
}

// cleanPosixPath converts p to forward slashes and lexically cleans it with
// path.Clean, preserving the malformed markers Validate's card-path-malformed
// check keys on: a leading, unresolvable ".." (a genuine escape past the
// worktree root) or a leading "/" (an absolute path) both survive path.Clean
// unchanged, while only harmless internal "."/".." segments collapse away. An
// empty input stays empty rather than becoming path.Clean's "." — an empty card
// path is itself a distinct malformed case for Validate to flag.
func cleanPosixPath(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(p))
}

// normalizeCard rewrites every card path field on card in place — all five typed
// file-op fields and both endpoints of every Moves: pair — through
// normalizeCardPath against root (Plan.Root). It leaves MovesRaw (malformed Moves:
// bullets) untouched, since those never resolved into a MovePair to normalize.
func normalizeCard(card *Card, root string) {
	normalizePathSlice(card.ContextFiles, root)
	normalizePathSlice(card.EditsFiles, root)
	normalizePathSlice(card.CreatesFiles, root)
	normalizePathSlice(card.DeletesFiles, root)
	for i, m := range card.Moves {
		card.Moves[i] = MovePair{
			Old: normalizeCardPath(root, m.Old),
			New: normalizeCardPath(root, m.New),
		}
	}
}

// normalizePathSlice normalizes every element of files in place against root. A
// nil or empty slice is left as-is (nil stays nil, distinguishing "field never
// seen" from "field present but empty" all the way through normalization).
func normalizePathSlice(files []string, root string) {
	for i, f := range files {
		files[i] = normalizeCardPath(root, f)
	}
}
