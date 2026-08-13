//go:build integration

// config_driven_junctions_integration_test.go proves the two behaviors batch
// 3 exists to prove: (1) the extensibility promise of the hybrid name-sourcing
// seam — WireJunctions/UnwireJunctions wire and unwire exactly the name-set a
// caller passes them, with no fabric/lyxcwd code change needed for a
// future module to append its own junction name to pathspec — and (2) that a
// worktree whose pathspec is narrower than the default (only "_lyx") is still
// reported healthy by Healthy(), since a narrow pathspec is a legitimate,
// unenforced reality (see doc.go's narrow-pathspec asymmetry note), not a
// drift shape.
//
// Package fabricengine_test, mirroring junction_pattern_integration_test.go's
// imports and gitkit.CopyPairedLocal(t) fixture pattern; shares the single
// TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestWireJunctions_WiresEveryPassedName is the extensibility proof under the hybrid seam:
// WireJunctions/UnwireJunctions wire and unwire exactly the name-set they are given — here a
// three-name set including "_extra" and "_other", names that are neither part of the default
// pathspec nor hub-reserved — with no SeedConfig, because WireJunctions no longer reads config at all.
// This is the proof that a future raddle/board append (one extra pathspec token) is wired with no
// fabric/lyxcwd code change: a caller sourcing an extended pathspec would pass its names exactly
// this way.
func TestWireJunctions_WiresEveryPassedName(t *testing.T) {
	t.Parallel()

	fixture := gitkit.CopyPairedLocal(t)

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	names := []string{"_lyx", "_other", "_extra"}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	junctions := fabricengine.WarpJunctions(l, slug, names)
	for _, j := range junctions {
		isLink, err := fslink.IsLink(j.Link)
		if err != nil || !isLink {
			t.Errorf("junction %s (%s) is not a link: isLink=%v err=%v", j.Name, j.Link, isLink, err)
		}
		if info, err := os.Stat(j.Target); err != nil || !info.IsDir() {
			t.Errorf("weft target for %s (%s) not materialised: stat err=%v", j.Name, j.Target, err)
		}
	}

	lines := readExcludeLines(t, l, slug)
	for _, name := range names {
		pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, name)
		if !containsLine(lines, pattern) {
			t.Errorf(".git/info/exclude does not contain %q after WireJunctions: %v", pattern, lines)
		}
	}

	result, err := fabricengine.UnwireJunctions(l, slug, names)
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}
	if !slices.Equal(result.JunctionsRemoved, names) {
		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, names)
	}

	for _, j := range junctions {
		if _, statErr := os.Lstat(j.Link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s (%s) still exists after UnwireJunctions", j.Name, j.Link)
		}
	}
}

// TestHealthy_NarrowPathspecIsHealthy is the narrow-pathspec-is-healthy proof, re-based for the
// structural-directories batch: `pathspec: _lyx` is now redundant rather than narrow (`_lyx` arrives
// structurally regardless of pathspec), so this uses a repo-wide config whose pathspec names neither
// structural directory at all.
// Healthy loads its junction name-set from the repo-wide fabricengine.BoardDir(l.HubPath) fabric.yaml
// (card 7), unions it with both structural directory sets (structuralCommittedDirs,
// structuralNeverCommittedDirs), so a worktree with only "_lyx" and ".lyx" wired is still reported in
// sync even though the config on disk names nothing recognisable.
// A pathspec naming neither structural directory is a legitimate, unenforced reality (doc.go's
// narrow-pathspec asymmetry note), not a drift shape Healthy should flag.
//
// Healthy checks weft-branch correspondence (weftBranch == WeftBranchName(warpBranch),
// drift.go:69-72) before the junction loop, and raw CopyPairedLocal leaves the weft prime on "main"
// (not "main-weft"), so this checks out the weft branch first — the same
// TestHealthy_JunctionDriftShapes pattern (junction_pattern_integration_test.go:~400).
func TestHealthy_NarrowPathspecIsHealthy(t *testing.T) {
	t.Parallel()

	fixture := gitkit.CopyPairedLocal(t)
	// The repo-wide pathspec (not fixture.WeftPrime's own weft base) is what
	// Healthy reads after card 7; gitkit.CopyPairedLocal does not create
	// a _board dir, so create it and its _lyx/config/ first, mirroring
	// seedRepoWideFabricConfig but with a pathspec naming neither
	// structural directory, proving _lyx is still wired structurally.
	boardDir := fabricengine.BoardDir(fixture.Layout.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "fabric"), []byte("branch_prefix: \"\"\npathspec: _other\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
	gitkit.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)

	// The wired name-set must match what WiredNames/Healthy compute: both
	// structural directories (_lyx, .lyx) unioned with the hub-reserved-filtered
	// pathspec ("_other" here) — neither structural directory is named by the
	// config itself.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", ".lyx", "_other"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(l)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok {
		t.Errorf("Healthy ok = false (reason %+v); want true with _lyx wired structurally even though the config names neither structural directory", reason)
	}
	if reason.Cause == fabricengine.CauseConfigLoadFailed {
		t.Errorf("Healthy reason = %+v; want a real verdict, not CauseConfigLoadFailed", reason)
	}
}
