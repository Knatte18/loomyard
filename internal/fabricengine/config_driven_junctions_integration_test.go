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
// imports and lyxtest.CopyPairedLocal(t) fixture pattern; shares the single
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
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWireJunctions_WiresEveryPassedName is the extensibility proof under the hybrid seam: WireJunctions/UnwireJunctions wire and unwire exactly the name-set they are given — here a three-name set including "_extra", a name that is neither part of the default pathspec nor hub-reserved — with no SeedConfig, because WireJunctions no longer reads config at all.
// This is the proof that a future raddle/board append (one extra pathspec token) is wired with no fabric/lyxcwd code change: a caller sourcing an extended pathspec would pass its names exactly this way.
func TestWireJunctions_WiresEveryPassedName(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)
	names := []string{"_lyx", "_pattern", "_extra"}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	junctions := fabricengine.HostJunctions(l, slug, names)
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
		if !containsLine(lines, name) {
			t.Errorf(".git/info/exclude does not contain %q after WireJunctions: %v", name, lines)
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

// TestHealthy_NarrowPathspecIsHealthy is the narrow-pathspec-is-healthy proof: Healthy loads its junction name-set from the repo-wide fabricengine.BoardDir(l.HubPath) fabric.yaml (card 7), so a worktree whose pathspec names only "_lyx" — narrower than the "_lyx _pattern" default — is reported in sync once "_lyx" alone is wired.
// A narrow pathspec is a legitimate, unenforced reality (doc.go's narrow-pathspec asymmetry note), not a drift shape Healthy should flag.
//
// Healthy checks weft-branch correspondence (weftBranch == WeftBranchName(hostBranch), drift.go:69-72) before the junction loop, and raw CopyPairedLocal leaves the weft prime on "main" (not "main-weft"), so this checks out the weft branch first — the same TestHealthy_JunctionDriftShapes pattern (junction_pattern_integration_test.go:~400).
func TestHealthy_NarrowPathspecIsHealthy(t *testing.T) {
	t.Parallel()

	fixture := lyxtest.CopyPairedLocal(t)
	// The repo-wide pathspec (not fixture.WeftPrime's own weft base) is what
	// Healthy reads after card 7; lyxtest.CopyPairedLocal does not create
	// a _board dir, so create it and its _lyx/config/ first, mirroring
	// seedRepoWideFabricConfig but with this test's narrow "_lyx"-only
	// pathspec instead of the default template.
	boardDir := fabricengine.BoardDir(fixture.Layout.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "fabric"), []byte("branch_prefix: \"\"\npathspec: _lyx\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	slug := filepath.Base(fixture.Hub)

	if err := fabricengine.WireJunctions(l, slug, []string{"_lyx"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(l)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok {
		t.Errorf("Healthy ok = false (reason %q); want true with only _lyx wired (narrow-pathspec reality)", reason)
	}
}
