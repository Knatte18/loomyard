// weftname.go declares the "-weft" naming convention for fabric's weft sibling directories.
// It is a stdlib-only leaf: production geometry code and gitkit's on-disk fixtures derive the same name from it.
// Two independently maintained string literals could drift apart.

package weftname

import "path/filepath"

// Suffix is appended to a warp-worktree slug to form the weft sibling directory name (e.g. "feat" -> "feat-weft").
// It is the single declarer of this token;
// Use SiblingPath rather than this constant directly wherever a full path is needed.
const Suffix = "-weft"

// SiblingPath returns the path to the weft sibling directory for base inside container.
// For example, SiblingPath("/hub", "feat") returns "/hub/feat-weft".
func SiblingPath(container, base string) string {
	return filepath.Join(container, base+Suffix)
}
