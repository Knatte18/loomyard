//go:build integration

// remove_reserved_integration_test.go proves Remove refuses the same reserved slugs Add refuses,
// against a real hub with those directories actually present on disk.
// Before the shared validator, `lyx fabric remove _board` and `lyx fabric remove <prime>-weft`
// reached the teardown path and destroyed the hub's weft:main records worktree and the entire weft
// prime respectively, both reported as success.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

func TestRemove_RefusesReservedSlugsAndLeavesThemOnDisk(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	topology := h.Topology

	weftPrimeSlug := filepath.Base(l.WorktreePath()) + weftname.Suffix

	reserved := []string{
		fabricengine.BoardDirName,
		lyxdirs.LyxDirName,
		lyxdirs.DotLyxDirName,
		weftPrimeSlug,
	}

	for _, slug := range reserved {
		dir := filepath.Join(l.HubPath, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		marker := filepath.Join(dir, "keep-me")
		if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", marker, err)
		}

		_, err := topology.Remove(l, slug, true, false)
		if err == nil {
			t.Errorf("Remove(%q) = nil error; want an invalid-slug refusal", slug)
		} else if !errors.Is(err, fabricengine.ErrInvalidSlug) {
			t.Errorf("Remove(%q) error = %v; want errors.Is ErrInvalidSlug", slug, err)
		}

		if _, statErr := os.Stat(marker); statErr != nil {
			t.Errorf("Remove(%q) destroyed hub geometry at %s: %v", slug, marker, statErr)
		}
	}
}
