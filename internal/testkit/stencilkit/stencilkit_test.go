package stencilkit

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

func TestSeed_ReadsBackEveryRegistryName(t *testing.T) {
	dir := Seed(t)
	reg := stencils.Registry()
	for _, name := range reg.Names() {
		want, _ := reg.Default(name)
		got, err := stencilstore.Read(dir, name)
		if err != nil {
			t.Fatalf("Read(%q): %v", name, err)
		}
		if stencil.StripLeadingComment(string(got)) != stencil.StripLeadingComment(string(want)) {
			t.Errorf("stencil %q body differs from the registry default", name)
		}
	}
}

func TestSeedInto_UsesGivenDirectory(t *testing.T) {
	dir := t.TempDir()
	SeedInto(t, dir)
	for _, name := range stencils.Registry().Names() {
		if _, err := os.Stat(stencilstore.Path(dir, name)); err != nil {
			t.Errorf("stencil %q not seeded into %s: %v", name, dir, err)
		}
	}
}

func TestRemove_LeavesTheRest(t *testing.T) {
	dir := Seed(t)
	names := stencils.Registry().Names()
	gone := names[0]
	Remove(t, dir, gone)
	if _, err := stencilstore.Read(dir, gone); err == nil {
		t.Errorf("stencil %q still readable after Remove", gone)
	}
	for _, name := range names[1:] {
		if _, err := stencilstore.Read(dir, name); err != nil {
			t.Errorf("stencil %q lost by Remove(%q): %v", name, gone, err)
		}
	}
	Remove(t, dir, gone)
}
