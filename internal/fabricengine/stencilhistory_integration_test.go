//go:build integration

// stencilhistory_integration_test.go covers StencilBaseByStamp against a real hubforge hub whose
// board has been seeded and re-seeded with a changed default, via CommitSeededStencils exactly as
// the real seeding pass would drive it.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// seedStencil writes content at name's path under hub's stencils directory and commits it via
// CommitSeededStencils, mirroring the real per-process seeding pass.
func seedStencil(t *testing.T, hub *hubforge.Hub, name string, content []byte, message string) {
	t.Helper()

	relPath := stencilstore.RelPath(name)
	fullPath := filepath.Join(fabricengine.StencilsDir(hub.Path), filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}

	rec := fabricengine.NewMutations(filepath.Dir(hub.Path))
	if _, err := fabricengine.CommitSeededStencils(hub.Path, []string{relPath}, message, rec); err != nil {
		t.Fatalf("CommitSeededStencils(%s): %v", relPath, err)
	}
}

// TestStencilBaseByStamp_FindsOlderDefaultByStamp asserts StencilBaseByStamp finds the forked-from
// revision for a file stamped from an older default, returning that older default's body.
func TestStencilBaseByStamp_FindsOlderDefaultByStamp(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const name = "loom-template-discussion"

	older := []byte("older default body\n")
	seedStencil(t, hub, name, older, "lyx: seed stencils (v1)")
	olderStamp := stencilstore.BodyHash(older)

	newer := []byte("newer default body\n")
	seedStencil(t, hub, name, newer, "lyx: seed stencils (v2)")

	base, rev, found, err := fabricengine.StencilBaseByStamp(hub.Path, name, olderStamp)
	if err != nil {
		t.Fatalf("StencilBaseByStamp() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("StencilBaseByStamp() found = false; want true")
	}
	if rev == "" {
		t.Errorf("StencilBaseByStamp() rev = \"\"; want a non-empty revision SHA")
	}
	if string(base) != string(older) {
		t.Errorf("StencilBaseByStamp() base = %q; want %q", base, older)
	}
}

// TestStencilBaseByStamp_NoMatchReturnsFoundFalse asserts StencilBaseByStamp returns found == false
// and a nil error when no revision's body matches the stamp -- the case `diff` must report
// explicitly instead of rendering an empty diff.
func TestStencilBaseByStamp_NoMatchReturnsFoundFalse(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const name = "loom-template-discussion"

	seedStencil(t, hub, name, []byte("only default body\n"), "lyx: seed stencils")

	const neverMatchedStamp = "0000000000000000000000000000000000000000000000000000000000000000"
	base, rev, found, err := fabricengine.StencilBaseByStamp(hub.Path, name, neverMatchedStamp)
	if err != nil {
		t.Fatalf("StencilBaseByStamp() error = %v; want nil", err)
	}
	if found {
		t.Errorf("StencilBaseByStamp() found = true; want false (no revision matches the stamp)")
	}
	if base != nil {
		t.Errorf("StencilBaseByStamp() base = %q; want nil", base)
	}
	if rev != "" {
		t.Errorf("StencilBaseByStamp() rev = %q; want \"\"", rev)
	}
}

// TestStencilBaseByStamp_HashNormalisationAcrossCRLF asserts that a stamp computed from a
// working-tree copy whose bytes were written with CRLF line endings still matches the LF-stored
// blob, in the base-recovery path specifically. go-git returns stored blob bytes untouched while
// CLI git converts on checkout, so the two sides can differ by line ending alone -- this is what
// keeps base recovery working on a machine with core.autocrlf=true, where a regression here would
// silently disable it entirely.
func TestStencilBaseByStamp_HashNormalisationAcrossCRLF(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const name = "loom-template-discussion"

	olderLF := []byte("older default body\nsecond line\n")
	seedStencil(t, hub, name, olderLF, "lyx: seed stencils (v1)")

	// The stamp a CRLF working-tree checkout would have produced: BodyHash normalises LF internally,
	// so this equals BodyHash(olderLF) even though the raw bytes differ.
	olderCRLF := []byte("older default body\r\nsecond line\r\n")
	stampFromCRLFCopy := stencilstore.BodyHash(olderCRLF)

	newer := []byte("newer default body\n")
	seedStencil(t, hub, name, newer, "lyx: seed stencils (v2)")

	base, _, found, err := fabricengine.StencilBaseByStamp(hub.Path, name, stampFromCRLFCopy)
	if err != nil {
		t.Fatalf("StencilBaseByStamp() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("StencilBaseByStamp() found = false; want true (CRLF-derived stamp must still match the LF-stored blob)")
	}
	if string(base) != string(olderLF) {
		t.Errorf("StencilBaseByStamp() base = %q; want %q", base, olderLF)
	}
}
