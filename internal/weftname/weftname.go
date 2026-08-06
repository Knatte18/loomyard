// weftname.go declares the "-weft" naming convention for fabric's weft sibling
// directories: a stdlib-only leaf so both production geometry code and
// lyxtest's on-disk fixtures derive the identical name from the identical
// source, rather than risking two independently-maintained string literals
// drifting apart.

package weftname

import "path/filepath"

// Suffix is the suffix appended to a host-worktree slug to form the weft
// sibling directory name (e.g. "feat" -> "feat-weft"). It is the single
// declarer of this token; use SiblingPath/BareSiblingPath rather than this
// constant directly wherever a full path is needed.
const Suffix = "-weft"

// bareSuffix is appended after Suffix to name the bare-remote test fixture
// directory paired with a weft sibling (e.g. "feat-weft-bare").
const bareSuffix = "-bare"

// SiblingPath returns the absolute path to the weft sibling directory for the
// given base name inside container (e.g. SiblingPath("/hub", "feat") ->
// "/hub/feat-weft").
func SiblingPath(container, base string) string {
	return filepath.Join(container, base+Suffix)
}

// BareSiblingPath returns the absolute path to the bare-remote test fixture
// directory paired with a weft sibling (e.g. BareSiblingPath("/hub", "feat")
// -> "/hub/feat-weft-bare"). It exists for lyxtest's fixture builders, which
// pair every weft-sibling template with its own bare origin on disk.
func BareSiblingPath(container, base string) string {
	return filepath.Join(container, base+Suffix+bareSuffix)
}
