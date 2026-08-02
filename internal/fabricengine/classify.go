// classify.go — the pure warp-vs-weft path classifier Fabric.Commit (a later
// batch) uses to split a caller-supplied path list into the warp-side and
// weft-side pathspecs it commits separately. classifyPaths does no I/O and no
// path validation: it trusts its caller, the same posture ScopedPathspec
// takes, and leaves config loading and hub-reserved-name filtering to the
// caller (WiredNames in junctionnames.go).

package fabricengine

import "path/filepath"

// classifyPaths partitions files into weft and warp paths, in input order,
// preserving originals unchanged (significant on Windows for path separators).
// A path is weft iff it equals or falls under a wired junction prefix with segment boundary.
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
