//go:build integration

// remove_reserved_integration_test.go proves Remove refuses the same reserved slugs Add refuses,
// against a real hub with those directories actually present on disk.
// Before the shared validator, `lyx fabric remove _board` and `lyx fabric remove <prime>-weft`
// reached the teardown path and destroyed the hub's weft:main records worktree and the entire weft
// prime respectively, both reported as success.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestRemove_RefusesReservedSlugsAndLeavesThemOnDisk walks the reserved set against a live hub and
// asserts each name is refused with an "invalid slug" error and its directory survives.
func TestRemove_RefusesReservedSlugsAndLeavesThemOnDisk(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

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

		_, err := topology.Remove(l, slug, true)
		if err == nil {
			t.Errorf("Remove(%q) = nil error; want an invalid-slug refusal", slug)
		} else if !strings.Contains(err.Error(), "invalid slug") {
			t.Errorf("Remove(%q) error = %v; want error containing %q", slug, err, "invalid slug")
		}

		if _, statErr := os.Stat(marker); statErr != nil {
			t.Errorf("Remove(%q) destroyed hub geometry at %s: %v", slug, marker, statErr)
		}
	}
}
