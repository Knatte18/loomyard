// weftname.go declares the "-weft" naming convention for fabric's weft sibling directories.
// It is a stdlib-only leaf: production geometry code and gitkit's on-disk fixtures derive the same name from it.
// Two independently maintained string literals could drift apart.

package weftname

import "path/filepath"

// Suffix is appended to a warp-worktree slug to form the weft sibling directory name (e.g. "feat" -> "feat-weft").
// It is the single declarer of this token;
// Use SiblingPath/BareSiblingPath rather than this constant directly wherever a full path is needed.
const Suffix = "-weft"

// bareSuffix is appended after Suffix to name the bare-remote test fixture paired with a weft sibling.
// For example, the fixture for "feat-weft" is "feat-weft-bare".
const bareSuffix = "-bare"

// SiblingPath returns the path to the weft sibling directory for base inside container.
// For example, SiblingPath("/hub", "feat") returns "/hub/feat-weft".
func SiblingPath(container, base string) string {
	return filepath.Join(container, base+Suffix)
}

// BareSiblingPath returns the path inside container of the bare-remote fixture paired with a weft sibling.
// For example, BareSiblingPath("/hub", "feat") returns "/hub/feat-weft-bare".
// It exists for gitkit's fixture builders, which pair every weft-sibling template with its own bare origin on disk.
func BareSiblingPath(container, base string) string {
	return filepath.Join(container, base+Suffix+bareSuffix)
}
