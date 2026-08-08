// classify.go — the pure warp-vs-weft-vs-never-committed path classifier Fabric.Commit uses to split
// a caller-supplied path list into the three pathspecs it treats differently: the warp-side and
// weft-side paths it commits separately, and the never-committed paths it must refuse rather than
// silently drop or commit.
// Omission is not enough and is actively worse than a dedicated third bucket: classifyPaths is a
// strict split where everything not matching a weft prefix falls through to **warp** — the user's
// own repo — where `git add` on an ignored path fails the whole invocation with exit 1 and stages
// nothing, taking every legitimate `_lyx` file named in the same call down with it.
// A `.lyx` path routed to warp by accident would hit exactly that failure mode.
// classifyPaths itself stays policy-free: the never-committed bucket is reported, and turning it
// into an error is Commit's job, not this file's.
// classifyPaths does no I/O and no path validation: it trusts its caller, the same posture
// ScopedPathspec takes, and leaves config loading and hub-reserved-name filtering to the caller
// (WiredNames/pathspecNames in junctionnames.go).

package fabricengine

import "path/filepath"

// classifyPaths partitions files into warp, weft, and neverCommitted paths, in input order,
// preserving originals unchanged (significant on Windows for path separators).
// The neverCommittedNames prefixes (built from routingNames the same way ScopedPathspec builds the
// weft prefixes) are evaluated FIRST: a path under one of them goes to neverCommitted and is not
// considered for either of the other two buckets, so a never-committed path can never fall through
// to warp — see this file's header comment for why that fallthrough would be actively worse than
// omission.
// Otherwise the existing weft-then-warp fallthrough is unchanged: a path is weft iff it equals or
// falls under a routingNames prefix with segment boundary, else it is warp.
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

// normalizedScopedPathspec builds relPath-scoped prefixes from names via ScopedPathspec, then
// filepath.ToSlash-normalizes each one so isUnderAnyWeftPrefix can compare them against
// filepath.ToSlash-normalized file paths regardless of platform separator.
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
