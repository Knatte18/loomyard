// Package stencilkit seeds stencil fixtures from the registry through `stencilstore`.
// Seeding runs `stencilstore.Reconcile` over `stencils.Registry()`, the owner the Stencil Ownership Invariant names, so seeded files carry the hash stamp production's do.
// Seed seeds a fresh temp directory, SeedInto seeds a caller-given directory such as a fixture hub's stencils directory, and Remove deletes named stencils for deliberate-absence tests.
// Its only assertions are `t.Fatalf` on its own setup.
package stencilkit

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Seed seeds every registry stencil into a fresh `t.TempDir()` and returns it.
func Seed(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	SeedInto(t, dir)
	return dir
}

// SeedInto seeds every registry stencil into dir.
func SeedInto(t testing.TB, dir string) {
	t.Helper()
	if _, err := stencilstore.Reconcile(dir, stencils.Registry(), stencilstore.ModeProduction, ""); err != nil {
		t.Fatalf("stencilkit: seed %s: %v", dir, err)
	}
}

// Remove deletes the named stencils from dir, which is how a test seeds everything and then makes one absent.
func Remove(t testing.TB, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Remove(stencilstore.Path(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("stencilkit: remove stencil %q from %s: %v", name, dir, err)
		}
	}
}
