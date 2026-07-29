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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/lock"
)

// userCacheDir is the seam goToolchainCacheDir/goToolchainInstallLock call
// through instead of os.UserCacheDir() directly, so tests can redirect the
// toolchain manager at a t.TempDir() without ever touching the real
// machine-global cache. Production leaves it at the stdlib default.
var userCacheDir = os.UserCacheDir

// goToolchainCacheDir returns the machine-global cache directory a pinned
// gopls version's binary lives in: filepath.Join(userCacheDir(), "lyx",
// "tools", "go", version). Every pinned version gets its own subdirectory so
// two worktrees pinned to different gopls versions never collide.
func goToolchainCacheDir(version string) string {
	// userCacheDir() only fails when neither $XDG_CACHE_HOME nor $HOME (or
	// their per-OS equivalents) is set; the error is ignored here because the
	// function signature returns a bare string, matching resolveGoToolchain's
	// own no-inputs-to-validate contract for this helper — an empty root
	// simply yields a path rooted at "lyx/tools/...", which os.MkdirAll then
	// reports as a normal filesystem error.
	dir, _ := userCacheDir()
	return filepath.Join(dir, "lyx", "tools", "go", version)
}

// goToolchainInstallLock returns the path to the advisory lock file fencing
// a Go toolchain install: filepath.Join(userCacheDir(), "lyx", "tools", "go",
// "install.lock"). Deliberately not version-scoped — one lock per language,
// not per version — so two processes installing two different pinned
// versions of the same language still serialize through the same lock file,
// matching toolchain-manager-authority's "one per language" decision.
func goToolchainInstallLock() string {
	// See goToolchainCacheDir's comment for why userCacheDir()'s error is
	// ignored here.
	dir, _ := userCacheDir()
	return filepath.Join(dir, "lyx", "tools", "go", "install.lock")
}

// toolchainInstaller installs the Go toolchain binary for version into
// destDir. It is the seam resolveGoToolchain calls through instead of
// invoking `go install` directly, so unit tests can substitute a fake that
// never spawns a real subprocess (see installGoToolchain below).
type toolchainInstaller func(ctx context.Context, version, destDir string) error

// installGoToolchain is the production toolchainInstaller resolveGoToolchain
// calls. Tests overwrite this package-level var with a fake and restore the
// original (runGoInstall) via t.Cleanup, mirroring lspclient.go's
// newLSPClient/newLSPClientFromRW test-seam convention.
var installGoToolchain toolchainInstaller = runGoInstall

// runGoInstall installs the pinned gopls version into destDir by running
// `go install golang.org/x/tools/gopls@<version>` with GOBIN set to destDir
// via the subprocess's environment, so the built binary lands directly in
// the toolchain cache directory rather than the default $GOPATH/bin.
func runGoInstall(ctx context.Context, version, destDir string) error {
	target := fmt.Sprintf("golang.org/x/tools/gopls@%s", version)
	cmd := exec.CommandContext(ctx, "go", "install", target)
	// Append GOBIN rather than replacing the environment outright, so the
	// subprocess still inherits GOPATH/GOCACHE/GOPROXY/etc. from this
	// process's own environment; a later GOBIN entry wins over any earlier
	// one Go's os/exec passes through unchanged.
	cmd.Env = append(os.Environ(), "GOBIN="+destDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("codeintelengine: go install %s: %w: %s", target, err, out)
	}
	return nil
}

// resolveGoToolchain resolves the on-disk path to the pinned gopls binary
// for pinnedVersion, installing it into the toolchain cache on a cold cache.
//
// The fast path (binary already present) takes no lock at all — the common
// case must not pay a lock round trip. On a cold cache it acquires the
// blocking, per-language install lock (goToolchainInstallLock), double-checks
// the binary's presence again now that the lock is held (a second caller
// that was waiting may find the first lock holder already finished the
// install), and only then calls installGoToolchain. There is no "loser
// reconnects to a live server" shortcut here, unlike EnsureServer's daemon
// strategies: an install either finished before this call reached the lock,
// or this call must wait for one to finish before it exits.
func resolveGoToolchain(ctx context.Context, pinnedVersion string) (string, error) {
	cacheDir := goToolchainCacheDir(pinnedVersion)
	binName := "gopls"
	if runtime.GOOS == "windows" {
		binName = "gopls.exe"
	}
	binPath := filepath.Join(cacheDir, binName)

	// Fast path: no lock taken. Most calls hit an already-installed pinned
	// version, and a lock round trip on every one of those would be pure
	// overhead.
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	// gofrs/flock opens the lock file with O_CREATE but never creates missing
	// parent directories, so a brand-new machine's very first Go toolchain
	// install (before the shared "go" cache directory exists at all) must
	// create it here first, matching internal/state's and
	// internal/reedengine's own MkdirAll-before-lock pattern.
	lockPath := goToolchainInstallLock()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return "", fmt.Errorf("codeintelengine: create go toolchain install lock dir %s: %w", filepath.Dir(lockPath), err)
	}
	fileLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return "", fmt.Errorf("codeintelengine: acquire go toolchain install lock: %w", err)
	}
	defer fileLock.Release()

	// Double-check: whoever held the lock first may have finished installing
	// this exact pinned version while this call was blocked waiting for it.
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("codeintelengine: create go toolchain cache dir %s: %w", cacheDir, err)
	}
	if err := installGoToolchain(ctx, pinnedVersion, cacheDir); err != nil {
		return "", fmt.Errorf("codeintelengine: install go toolchain %s: %w", pinnedVersion, err)
	}
	return binPath, nil
}
