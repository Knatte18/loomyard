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
