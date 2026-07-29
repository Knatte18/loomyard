// toolchain.go implements the Go-only toolchain manager: resolving (and, on
// a cold cache, installing) a pinned gopls binary into a codeintel-owned,
// machine-global cache directory. It ignores $PATH entirely for Go — unlike
// the other four languages' legacy cold-spawn-per-call path, which resolves
// entry.Command[0] on $PATH via newLSPClient, the native Go strategy
// (batch 5's ensureNative) always launches the exact pinned version this
// file resolved, never whatever gopls happens to be on the operator's PATH.
// The cache root is os.UserCacheDir(), not a Hub Geometry Invariant path:
// os.UserCacheDir() is the idiomatic stdlib answer to "OS-appropriate cache
// root" with no platform-specific logic to get wrong, and it is explicitly
// machine-global rather than worktree/hub geometry, which is why this file
// hand-joins it directly instead of routing through internal/hubgeometry
// (see _mill/discussion.md's toolchain-manager-authority decision).

package codeintelengine

import (
	"os"
	"path/filepath"
)

// goToolchainCacheDir returns the machine-global cache directory a pinned
// gopls version's binary lives in: filepath.Join(os.UserCacheDir(), "lyx",
// "tools", "go", version). Every pinned version gets its own subdirectory so
// two worktrees pinned to different gopls versions never collide.
func goToolchainCacheDir(version string) string {
	// os.UserCacheDir() only fails when neither $XDG_CACHE_HOME nor $HOME
	// (or their per-OS equivalents) is set; the error is ignored here
	// because the function signature returns a bare string, matching
	// resolveGoToolchain's own no-inputs-to-validate contract for this
	// helper — an empty root simply yields a path rooted at "lyx/tools/...",
	// which os.MkdirAll then reports as a normal filesystem error.
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "lyx", "tools", "go", version)
}

// goToolchainInstallLock returns the path to the advisory lock file fencing
// a Go toolchain install: filepath.Join(os.UserCacheDir(), "lyx", "tools",
// "go", "install.lock"). Deliberately not version-scoped — one lock per
// language, not per version — so two processes installing two different
// pinned versions of the same language still serialize through the same
// lock file, matching toolchain-manager-authority's "one per language"
// decision.
func goToolchainInstallLock() string {
	// See goToolchainCacheDir's comment for why os.UserCacheDir()'s error is
	// ignored here.
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "lyx", "tools", "go", "install.lock")
}
