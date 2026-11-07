// classify.go splits the paths a commit names into warp, weft, and never-committed buckets.
// A never-committed path must not fall through to warp.
// In the user's repo, `git add` on an ignored path fails the whole invocation with exit 1 and stages nothing.
// One `.lym` path in the call would take every legitimate file named beside it down with it.

package fabricengine

import "path/filepath"

// classifyPaths partitions files into warp, weft, and neverCommitted paths, in input order.
// Each returned path is the input string unchanged, because separators matter on Windows.
// A path under a neverCommittedNames prefix goes to neverCommitted and is never considered for warp or weft.
// Prefixes come from relPath and each name list, normalized to forward slashes.
// Otherwise a path is weft if it equals or falls under a routingNames prefix at a segment boundary, else warp.
// classifyPaths does no I/O and no path validation; it trusts its caller.
// Loading config and filtering hub-reserved names are the caller's job.
// The never-committed bucket is only reported: turning it into an error is the caller's decision.
func classifyPaths(relPath string, routingNames, neverCommittedNames []string, files []string) (warp, weft, neverCommitted []string) {
	weftPrefixes := normalizedScopedPathspec(relPath, routingNames)
	neverCommittedPrefixes := normalizedScopedPathspec(relPath, neverCommittedNames)

	for _, file := range files {
		normalizedFile := filepath.ToSlash(file)
		switch {
		case isUnderAnyWeftPrefix(normalizedFile, neverCommittedPrefixes):
			neverCommitted = append(neverCommitted, file)
		case isUnderAnyWeftPrefix(normalizedFile, weftPrefixes):
			weft = append(weft, file)
		default:
			warp = append(warp, file)
		}
	}
	return warp, weft, neverCommitted
}

// normalizedScopedPathspec returns the relPath-scoped prefixes for names, with forward slashes.
// The slash form lets them compare against filepath.ToSlash-normalized paths on any platform.
func normalizedScopedPathspec(relPath string, names []string) []string {
	prefixes := ScopedPathspec(relPath, names)
	normalized := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		normalized[i] = filepath.ToSlash(prefix)
	}
	return normalized
}

// isUnderAnyWeftPrefix reports whether normalizedFile matches or falls under one of the prefixes.
// Both arguments are assumed already filepath.ToSlash-normalized.
func isUnderAnyWeftPrefix(normalizedFile string, normalizedPrefixes []string) bool {
	for _, prefix := range normalizedPrefixes {
		if normalizedFile == prefix {
			return true
		}
		if len(normalizedFile) > len(prefix) && normalizedFile[:len(prefix)] == prefix && normalizedFile[len(prefix)] == '/' {
			return true
		}
	}
	return false
}
