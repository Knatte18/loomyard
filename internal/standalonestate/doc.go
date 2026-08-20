// doc.go carries the package-level doc comment for internal/standalonestate.

// Package standalonestate is a stdlib-only leaf that derives a per-target-directory hash8 and
// per-OS state directory, so every standalone CLI package can import it with no cycle risk.
//
// Derive requires target to already be absolute: consulting the process working directory to
// resolve a relative one belongs at the CLI argument-parsing boundary, per the Cwd Resolution
// Invariant, never inside this package.
//
// Told-geometry tier: standalonestate is told its geometry — Derive takes an already-absolute
// target from its caller and requires none of the three resolution tiers.
// This property is machine-enforced by internal/standalonestate/leaf_enforcement_test.go's
// TestLeafInvariant_AllowlistOnly, whose stdlib-only allowlist omits internal/lyxcwd.
// See CONSTRAINTS.md's Told-Geometry Invariant.
//
// The target is normalized -- symlinks resolved, the result cleaned, and lower-cased on Windows --
// before hashing, so two spellings of the same directory (a symlink and its target, or two
// differently-cased paths on a case-insensitive filesystem) hash identically.
// Without that, two standalone runs against the same target would land on different sockets,
// sessions, and state directories.
//
// hash8 collisions are accepted rather than handled: at 32 bits, two distinct targets can in
// principle share one state directory, which is wrong-but-not-corrupting.
// The fix, if it ever matters, is to widen hash8 here -- a one-line change, because every consumer
// takes the value from Derive rather than re-deriving it.
package standalonestate
