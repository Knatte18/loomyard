// Package stencilkit seeds stencil fixtures from the registry through `stencilstore`.
// Seeding runs `stencilstore.Reconcile` over `stencils.Registry()` in production mode.
// Seeded files carry the hash stamp production's files do.
// Seed seeds a fresh temp directory.
// SeedInto seeds a caller-given directory, such as a fixture hub's stencils directory.
// SeedEditDirectiveCopies adds the edit directive copy a fake engine name lacks.
// RequireEditDirectiveOf asserts which engine's edit directive copy a rendered prompt carries.
// Remove deletes named stencils for deliberate-absence tests.
// Its other assertions are `t.Fatalf` on its own setup.
package stencilkit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/editdirective"
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
	if _, err := stencilstore.Reconcile(dir, stencils.Registry(), stencilstore.ModeProduction, stencilstore.Source{}); err != nil {
		t.Fatalf("stencilkit: seed %s: %v", dir, err)
	}
}

// SeedEditDirectiveCopies gives each named engine a copy of the edit directive in dir, taken from the seeded Claude copy.
// A test whose fake engine name has no shipped copy calls it so the prompt render finds the engine's directive.
func SeedEditDirectiveCopies(t testing.TB, dir string, engines ...string) {
	t.Helper()
	content, err := stencilstore.Read(dir, stencilstore.EngineBlockName("claude", editdirective.EngineBlock))
	if err != nil {
		t.Fatalf("stencilkit: read the claude edit directive from %s: %v", dir, err)
	}
	for _, engine := range engines {
		path := stencilstore.Path(dir, stencilstore.EngineBlockName(engine, editdirective.EngineBlock))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("stencilkit: create the directory of %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("stencilkit: write the edit directive copy of engine %q: %v", engine, err)
		}
	}
}

// RequireEditDirectiveOf fails the test unless prompt carries engine's edit directive copy from stencilsDir.
// It also fails the test when prompt carries otherEngine's copy.
// It pins which engine's copy a producer renders, since Go chooses that fragment and the stencil prose does not.
func RequireEditDirectiveOf(t testing.TB, stencilsDir, prompt, engine, otherEngine string) {
	t.Helper()
	want, err := editdirective.Directive(stencilsDir, engine)
	if err != nil {
		t.Fatalf("stencilkit: read the %s edit directive from %s: %v", engine, stencilsDir, err)
	}
	unwanted, err := editdirective.Directive(stencilsDir, otherEngine)
	if err != nil {
		t.Fatalf("stencilkit: read the %s edit directive from %s: %v", otherEngine, stencilsDir, err)
	}
	if !strings.Contains(prompt, want) {
		t.Errorf("prompt lacks the %s edit directive %q", engine, want)
	}
	if strings.Contains(prompt, unwanted) {
		t.Errorf("prompt carries the %s edit directive %q; want the %s copy only", otherEngine, unwanted, engine)
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
