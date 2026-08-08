// dirs.go declares the two lyx directory-name tokens as the package's sole exported symbols.
// internal/lyxdirs is the sole declarer of either literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.

package lyxdirs

// LyxDirName is the durable, fabric-synced, git-tracked lyx directory within a worktree.
// internal/lyxdirs is the sole declarer of this literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.
const LyxDirName = "_lyx"

// DotLyxDirName is the ephemeral, machine-bound, never-git-tracked sibling of LyxDirName.
// Every never-tracked file lives under it at the mirrored subpath of the _lyx content it relates
// to.
// internal/lyxdirs is the sole declarer of this literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.
const DotLyxDirName = ".lyx"
