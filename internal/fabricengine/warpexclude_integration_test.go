// warpexclude_integration_test.go covers the warp exclude list.
// Wiring and SeedWarpExclude seed its anchored lines into the shared info/exclude exactly once, never into the weft or the wired junction names, and removing a pair keeps them.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// warpExcludeLines are the anchored exclude lines the template's default warp_exclude list yields.
var warpExcludeLines = []string{"/CLAUDE.local.md", "/AGENTS.override.md", "/.heddle"}

// countLine returns how many of lines trim to want.
func countLine(lines []string, want string) int {
	n := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == want {
			n++
		}
	}
	return n
}

// requireWarpExcludeOnce fails unless every warp exclude line is in dir's repository exclude file exactly once.
func requireWarpExcludeOnce(t *testing.T, dir string) {
	t.Helper()

	lines := gitkit.ExcludeLines(t, dir)
	for _, want := range warpExcludeLines {
		if got := countLine(lines, want); got != 1 {
			t.Errorf("%s exclude holds %q %d times; want 1 (lines: %v)", dir, want, got, lines)
		}
	}
}

// TestSeedWarpExclude_SeedsWarpOnlyOnce covers the warp exclude list end to end.
// A pair's warp exclude holds each listed path once however often wiring and SeedWarpExclude run.
// The weft exclude and the wired names hold none, and removing the pair keeps the lines.
// A re-wired pair gets stripped lines back, AddLocal seeds them, and an unparseable fabric config fails the wiring before it seeds anything.
//
//testtiming:keep the warp exclude lines seeded once into the warp only, outside the wired names and surviving Remove, re-seeded by Reconcile's re-wire and AddLocal, and an unparseable config failing wiring; coverage of its blocks by other tests does not show an assertion of this
func TestSeedWarpExclude_SeedsWarpOnlyOnce(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("seeded_once_warp_only_and_kept_by_remove", func(t *testing.T) {
		t.Parallel()
		const slug = "excludepair"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		l := h.Location

		names, err := fabricengine.WiredNames(fabricengine.ConfigDirs(l))
		if err != nil {
			t.Fatalf("WiredNames: %v", err)
		}
		paths, err := fabricengine.SeedWarpExclude(l)
		if err != nil {
			t.Fatalf("SeedWarpExclude: %v", err)
		}
		if got, want := strings.Join(paths, " "), "CLAUDE.local.md AGENTS.override.md .heddle"; got != want {
			t.Errorf("SeedWarpExclude() = %q; want %q", got, want)
		}
		for i := 0; i < 2; i++ {
			if err := fabricengine.WireJunctions(l, slug, names); err != nil {
				t.Fatalf("WireJunctions: %v", err)
			}
		}

		requireWarpExcludeOnce(t, fabricengine.WorktreePath(l, slug))

		weftLines := gitkit.ExcludeLines(t, fabricengine.RecordsWorktreePath(l, slug))
		for _, line := range warpExcludeLines {
			if countLine(weftLines, line) != 0 {
				t.Errorf("weft exclude holds %q; want none (lines: %v)", line, weftLines)
			}
		}
		for _, name := range names {
			for _, line := range warpExcludeLines {
				if line == "/"+name {
					t.Errorf("WiredNames holds %q, a warp exclude path; want none", name)
				}
			}
		}

		if _, err := h.Topology.Remove(l, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		requireWarpExcludeOnce(t, h.PrimeWorktree())
	})

	t.Run("reconcile_rewire_seeds_stripped_lines_again", func(t *testing.T) {
		t.Parallel()
		const slug = "rewirepair"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		l := h.Location

		link := resetDotLymJunction(t, l, slug)
		warp := fabricengine.WorktreePath(l, slug)
		excludePath := strings.TrimSpace(gitkit.Git(t, warp, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude"))
		var kept []string
		for _, line := range gitkit.ExcludeLines(t, warp) {
			if countLine(warpExcludeLines, strings.TrimSpace(line)) == 0 {
				kept = append(kept, line)
			}
		}
		if err := os.WriteFile(excludePath, []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("strip warp exclude lines: %v", err)
		}

		if _, err := h.Topology.Reconcile(l); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		if _, err := os.Lstat(link); err != nil {
			t.Errorf("junction %s not repaired by Reconcile: %v", link, err)
		}
		requireWarpExcludeOnce(t, warp)
	})

	t.Run("add_local_seeds_the_lines", func(t *testing.T) {
		t.Parallel()
		const slug = "localexcludepair"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
			t.Fatalf("AddLocal: %v", err)
		}
		requireWarpExcludeOnce(t, fabricengine.WorktreePath(h.Location, slug))
	})

	t.Run("unparseable_config_fails_wiring_and_seeds_nothing", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		before := gitkit.ExcludeLines(t, l.WorktreePath())

		hubforge.SeedFabricConfig(t, h, "branch_prefix: [unclosed\n")
		err := fabricengine.WireJunctions(l, l.WorktreeName, []string{"_lym"})
		if err == nil {
			t.Fatalf("WireJunctions over an unparseable fabric config succeeded; want a config load failure")
		}
		if !strings.Contains(err.Error(), "load fabric config") {
			t.Errorf("WireJunctions error = %q; want it to name the config load", err.Error())
		}
		if after := gitkit.ExcludeLines(t, l.WorktreePath()); len(after) != len(before) {
			t.Errorf("exclude lines changed from %v to %v; want nothing seeded", before, after)
		}
	})
}
