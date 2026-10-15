// prompt_test.go — declares newTestStencilsDir and newMinimalStencilsDir, the package-local test helpers every loomengine test uses to seed a hermetic stencils directory from the shipped stencils package defaults.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newTestStencilsDir returns a t.TempDir() seeded with every registry stencil.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// newMinimalStencilsDir returns a seeded stencils directory with the pattern-directive and friction-directive stencils removed, for tests that must prove a composer needs none of them to render its Tier-2-off / PATTERN-inactive path.
func newMinimalStencilsDir(t *testing.T) string {
	t.Helper()

	dir := stencilkit.Seed(t)
	stencilkit.Remove(t, dir,
		"pattern-directive-implementer",
		"pattern-directive-review-fix",
		"pattern-directive-orchestrator",
		"friction-directive-implementer",
		"friction-directive-review-fix",
		"friction-directive-orchestrator",
		"friction-directive-interview",
	)
	return dir
}
