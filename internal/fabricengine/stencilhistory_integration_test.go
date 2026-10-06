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
	if _, err := fabricengine.CommitSeededStencils(hub.Path, fabricengine.StencilsSubtreeRel(), fabricengine.StencilsDir(hub.Path), []string{relPath}, message, rec); err != nil {
		t.Fatalf("CommitSeededStencils(%s): %v", relPath, err)
	}
}

// TestStencilBaseByStamp covers the three outcomes over one hub whose stencil history grows step by step.
// The steps run in order on the same stencil; each seeds bodies no earlier step seeded last, and looks up a
// stamp only its own seeding (or none) produced, so a longer history never changes a step's expectation.
func TestStencilBaseByStamp(t *testing.T) {
	t.Parallel()

	hub := hubforge.NewHub(t, ".")
	const name = "loom-template-discussion"

	// A file stamped from an older default is found by that stamp, returning the older default's body.
	t.Run("FindsOlderDefaultByStamp", func(t *testing.T) {
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
	})

	// No revision's body matches the stamp: found == false and a nil error -- the case `diff` must
	// report explicitly instead of rendering an empty diff.
	t.Run("NoMatchReturnsFoundFalse", func(t *testing.T) {
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
	})

	// A stamp computed from a working-tree copy whose bytes were written with CRLF line endings still
	// matches the LF-stored blob, in the base-recovery path specifically. go-git returns stored blob
	// bytes untouched while CLI git converts on checkout, so the two sides can differ by line ending
	// alone -- this is what keeps base recovery working on a machine with core.autocrlf=true, where a
	// regression here would silently disable it entirely.
	t.Run("HashNormalisationAcrossCRLF", func(t *testing.T) {
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
	})
}
