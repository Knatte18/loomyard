// classify.go — the pure warp-vs-weft path classifier Fabric.Commit (a later
// batch) uses to split a caller-supplied path list into the warp-side and
// weft-side pathspecs it commits separately. classifyPaths does no I/O and no
// path validation: it trusts its caller, the same posture ScopedPathspec
// takes, and leaves config loading and hub-reserved-name filtering to the
// caller (WiredNames in junctionnames.go).

package fabricengine

import "path/filepath"

// classifyPaths partitions files into weft-side and warp-side paths, in
// input order, with nothing lost or duplicated. A path is weft iff it falls
// under one of the weft junction prefixes computed from relPath and
// wiredNames (via ScopedPathspec) — either equal to a prefix or beginning
// with that prefix plus a "/" segment boundary, so a sibling name that
// merely starts with the same characters (e.g. "_lyxfoo" next to "_lyx")
// classifies as warp, not weft. Everything else is warp.
//
// ScopedPathspec builds each junction prefix with filepath.Join, which
// emits OS-native separators (a backslash on Windows whenever relPath !=
// "."). The classification decision therefore compares both the computed
// prefixes and the input paths after filepath.ToSlash-normalizing each side
// — never a normalized input against an un-normalized prefix. That
// normalization is used only for the comparison: each output slice carries
// the original, unmodified input string, so a caller gets its paths back
// byte-for-byte (significant on Windows, where the normalized form would
// differ from what the caller passed in).
func classifyPaths(relPath string, wiredNames []string, files []string) (warp []string, weft []string) {
	prefixes := ScopedPathspec(relPath, wiredNames)
	normalizedPrefixes := make([]string, len(prefixes))
	for i, prefix := range prefixes {
		normalizedPrefixes[i] = filepath.ToSlash(prefix)
	}

	for _, file := range files {
		if isUnderAnyWeftPrefix(filepath.ToSlash(file), normalizedPrefixes) {
			weft = append(weft, file)
		} else {
			warp = append(warp, file)
		}
	}
	return warp, weft
}

// isUnderAnyWeftPrefix reports whether normalizedFile equals one of
// normalizedPrefixes or falls under one as a path segment (prefix + "/" +
// anything). Both arguments are assumed already filepath.ToSlash-normalized
// by the caller.
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
