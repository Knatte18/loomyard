//go:build integration

// toolchain_integration_test.go exercises resolveGoToolchain against a real
// `go install`, mirroring refs_integration_test.go's header-comment style:
// it is //go:build integration-tagged and therefore excluded from the plain
// `go test` verify (the Test Tier Purity Invariant); it is run separately
// with `-tags integration`. Unlike refs_integration_test.go's gopls-presence
// skip gate, this test has no natural skip condition — it needs only the go
// toolchain itself, which is guaranteed present in any environment where
// `go test -tags integration` can even run — so it runs unconditionally
// under the tag; there is deliberately no t.Skip here. This test spawns
// `go install` and the freshly installed gopls binary, never git, so no
// TestMain/lyxtest.HermeticGitEnv is required per the Hermetic Git Test
// Environment Invariant.

package codeintelengine

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestResolveGoToolchain_Integration(t *testing.T) {
	dir := t.TempDir()
	original := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = original })

	// installGoToolchain stays at its production value (runGoInstall) here:
	// this test's entire point is proving the real `go install` path
	// produces a working gopls binary, not exercising the mocked seam
	// toolchain_test.go already covers.

	// 120 seconds is generous enough for a real module-proxy fetch and
	// build of gopls on a cold module cache.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	binPath, err := resolveGoToolchain(ctx, builtins()["go"].PinnedVersion)
	if err != nil {
		t.Fatalf("resolveGoToolchain() error = %v; want nil", err)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("os.Stat(%q) error = %v; want the installed gopls binary to exist", binPath, err)
	}

	// This file's own exec.Command call below is a direct subprocess spawn
	// in test code, proving the installed binary is actually a working
	// gopls, not just a file that happens to exist at the expected path.
	if err := exec.Command(binPath, "version").Run(); err != nil {
		t.Fatalf("exec.Command(%q, %q).Run() error = %v; want the freshly installed gopls to report its version", binPath, "version", err)
	}
}
