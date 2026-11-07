// doc.go carries the package-level doc comment for internal/standalonestate.

// Package standalonestate derives a per-target-directory hash8 and a per-OS state directory.
// It imports only the standard library, so every standalone CLI package can import it with no cycle risk.
//
// Derive requires target to be absolute.
// Resolving a relative target against the process working directory belongs at the CLI argument-parsing boundary;
// the Cwd Resolution Invariant forbids doing it inside this package.
//
// UserStateDir returns the per-user `lym` directory that every state directory sits under.
// It reads the same environment as Derive and creates nothing.
// A record that belongs to the machine and to no target, such as the list of hubs, lives there.
//
// Told-geometry tier: the package is told its geometry.
// Derive takes an already-absolute target from its caller and uses none of the three resolution tiers.
// `TestLeafInvariant_AllowlistOnly` in internal/standalonestate/leaf_enforcement_test.go enforces this;
// its stdlib-only allowlist omits internal/lymcwd.
// See PATTERN-cwd-resolution.
//
// Derive normalizes the target before hashing: symlinks resolved, the result cleaned, lower-cased on Windows.
// Two spellings of the same directory therefore hash identically:
// a symlink and its target and, on Windows, two paths that differ only in case.
// The case fold is Windows-only to match internal/lymcwd's samePath rule, which every other path comparison in lym uses;
// folding on more platforms would make Derive disagree with lymcwd.
// Another case-insensitive filesystem, such as the default macOS APFS, behaves differently:
// two differently-cased spellings of one repository hash to two state directories, two sockets and two tmux sessions.
//
// A hash8 collision is accepted rather than handled:
// at 32 bits, two distinct targets can in principle share one state directory, which is wrong but not corrupting.
// Widening hash8 in Derive fixes it in one line, because every consumer takes the value from Derive.
package standalonestate
