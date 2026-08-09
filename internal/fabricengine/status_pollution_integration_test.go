//go:build integration

// status_pollution_integration_test.go pins the anchor scoping of Topology.Status's warp-pollution
// scan.
// The scan used a bare "_lyx" pathspec at the warp worktree ROOT, so a subpath-anchored hub whose
// index genuinely tracked <anchor>/_lyx/... was reported clean — a false negative in the one verb
// (`lyx fabric pairs`) that advertises the check.
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestStatus_DetectsWarpPollutionUnderSubpathAnchor tracks a file at <anchor>/_lyx/ in the warp
// index of a subpath-anchored hub and asserts Status reports it with a git rm --cached remedy.
func TestStatus_DetectsWarpPollutionUnderSubpathAnchor(t *testing.T) {
	t.Parallel()

	const anchor = "backend"

	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Record the subpath anchor so every resolver agrees the hub is anchored at <anchor>.
	subDir := filepath.Join(l.WorktreePath(), anchor)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", subDir, err)
	}
	anchorMarker := filepath.Join(fabricengine.BoardDir(l.HubPath), lyxcwd.AnchorFileName)
	if err := os.WriteFile(anchorMarker, []byte(anchor+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorMarker, err)
	}

	anchoredLayout, err := lyxcwd.Resolve(subDir)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", subDir, err)
	}
	if anchoredLayout.AnchorRel != anchor {
		t.Fatalf("AnchorRel = %q; want %q", anchoredLayout.AnchorRel, anchor)
	}

	// Pollute the warp index at the anchored durable path, the shape an operator produces by
	// committing _lyx content before fabric wired the junction.
	pollutedRel := filepath.ToSlash(filepath.Join(anchor, lyxdirs.LyxDirName, "legacy.md"))
	pollutedAbs := filepath.Join(l.WorktreePath(), filepath.FromSlash(pollutedRel))
	if err := os.MkdirAll(filepath.Dir(pollutedAbs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(pollutedAbs), err)
	}
	if err := os.WriteFile(pollutedAbs, []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", pollutedAbs, err)
	}
	lyxtest.MustRun(t, l.WorktreePath(), "git", "add", pollutedRel)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	result, err := topology.Status(anchoredLayout)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(result.Pairs) == 0 {
		t.Fatal("Status() returned no pairs")
	}

	var found bool
	for _, entry := range result.Pairs[0].Pollution {
		if entry.Path != pollutedRel {
			continue
		}
		found = true
		if !strings.Contains(entry.Remedy, "rm --cached") {
			t.Errorf("pollution remedy for %q = %q; want a git rm --cached command", pollutedRel, entry.Remedy)
		}
	}
	if !found {
		t.Errorf("Status() pollution = %+v; want it to report %q tracked in the warp index",
			result.Pairs[0].Pollution, pollutedRel)
	}
}
