// definition_test.go is the untagged, spawn-free counterpart to any future
// definition_integration_test.go, mirroring refs_test.go's style exactly:
// it exercises Definition's error-mapping path that does not require a
// real language server. exec.LookPath failing for a nonexistent binary
// happens before any subprocess is spawned, so this test needs no
// //go:build integration tag and no installed language server.

package codeintelengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDefinition_NonExistentServerBinaryYieldsErrServerNotFound points a
// synthetic registry entry's Command at a binary that cannot exist on
// $PATH and asserts Definition maps the resulting exec.LookPath failure to
// ErrServerNotFoundSentinel — a direct copy of
// TestReferences_NonExistentServerBinaryYieldsErrServerNotFound (refs_test.go)
// but calling Definition instead of References, proving Definition goes
// through acquireConnection's same error-mapping for the legacy path since
// lookup is shared code between the two.
func TestDefinition_NonExistentServerBinaryYieldsErrServerNotFound(t *testing.T) {
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

	_, err := Definition(ctx, Options{
		Registry:  reg,
		TargetDir: dir,
		Lang:      "go",
		Query:     Query{Symbol: "Resolve"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, ErrServerNotFoundSentinel) {
		t.Errorf("Definition() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
	}
}
