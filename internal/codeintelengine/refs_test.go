// refs_test.go is the untagged, spawn-free counterpart to
// refs_integration_test.go: it exercises References's error-mapping paths
// that do not require a real language server. exec.LookPath failing for a
// nonexistent binary happens before any subprocess is spawned, so this test
// needs no //go:build integration tag and no installed language server.

package codeintelengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestReferences_HasNativeDaemonRoutesThroughEnsureServer proves that a
// registry entry with HasNativeDaemon: true takes the ensureServer path,
// not the legacy newLSPClient(entry.Command) path — without spawning a
// real gopls. It swaps installGoToolchain for a fake that always fails
// with a distinct, recognizable error, then asserts References's returned
// error wraps that exact sentinel: only reachable if the call chain was
// References -> lookup -> acquireConnection -> ensureServer -> ensureNative
// -> resolveGoToolchain -> the fake installer. Had References instead taken
// the legacy path, it would fail with ErrServerNotFoundSentinel from a
// literal, unresolved "gopls" lookup on $PATH — a categorically different
// error this assertion distinguishes from. This is not a proof that a real
// gopls connection works end to end — that is ensureserver_integration_test.go
// (batch 5).
func TestReferences_HasNativeDaemonRoutesThroughEnsureServer(t *testing.T) {
	withTempUserCacheDir(t)

	errFakeInstallRefused := errors.New("fake install refused")
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		return errFakeInstallRefused
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := References(ctx, Options{
		Registry: Registry{
			"go": {
				Command:         []string{"gopls"},
				PinnedVersion:   "v0.0.0-test",
				HasNativeDaemon: true,
			},
		},
		TargetDir: t.TempDir(),
		Lang:      "go",
		Query:     Query{Symbol: "X"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, errFakeInstallRefused) {
		t.Errorf("References() with HasNativeDaemon: true err = %v; want errors.Is(err, errFakeInstallRefused) (proving the ensureServer -> ensureNative -> resolveGoToolchain path was taken, not the legacy newLSPClient path)", err)
	}
}

// TestReferences_NonExistentServerBinaryYieldsErrServerNotFound points a
// synthetic registry entry's Command at a binary that cannot exist on
// $PATH and asserts References maps the resulting exec.LookPath failure to
// ErrServerNotFoundSentinel, mirroring the equivalent
// //go:build integration subtest in refs_integration_test.go but without
// any dependency on gopls being installed.
func TestReferences_NonExistentServerBinaryYieldsErrServerNotFound(t *testing.T) {
	dir := t.TempDir()
	reg := Registry{
		"go": {
			Markers:     []string{"go.mod"},
			Match:       "any",
			Command:     []string{"lyx-codeintel-nonexistent-binary-xyz"},
			InstallHint: "this binary is intentionally fake for the test",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := References(ctx, Options{
		Registry:  reg,
		TargetDir: dir,
		Lang:      "go",
		Query:     Query{Symbol: "Resolve"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, ErrServerNotFoundSentinel) {
		t.Errorf("References() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
	}
}
