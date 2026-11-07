// prompt_test.go — declares the package-local test helpers newTestStencilsDir and newMinimalStencilsDir.
// They seed a hermetic stencils directory from the shipped stencils package defaults.

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

// newMinimalStencilsDir returns a seeded stencils directory without the pattern and friction directive stencils.
// Tests use it to prove a composer renders its Tier-2-off and PATTERN-inactive path without them.
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
