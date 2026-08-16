// normalize.go implements plan-format's root:/// card-path resolution rule
// (contracts/specs/loom-plan-spec.md, "Card path resolution: root: and //"): normalizeCardPath
// resolves one raw card path against the plan's root:, and normalizeCard applies it to every path
// field on a single Card.
// ParsePlan calls normalizeCard exactly once per card, right after that card's body is parsed, so
// every downstream consumer — Validate included — only ever sees plain, forward-slash,
// worktree-relative paths.

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

// normalizeCard rewrites every card path field in place against root via normalizeCardPath.
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

// normalizePathSlice normalizes every element of files in place against root, preserving nil vs empty-slice distinction.
func normalizePathSlice(files []string, root string) {
	for i, f := range files {
		files[i] = normalizeCardPath(root, f)
	}
}
