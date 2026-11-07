// structuraldirs_test.go covers structuralCommittedDirs and structuralNeverCommittedDirs over a hand-built Config.
// It checks the name-sets built on them: wired, pathspec and commit-routing, and slug reservation.
// Any two of these three can pass while the third is broken, so all three are tested together in this file.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wiredNamesFromConfig returns the wired name-set for cfg: the structural directories plus cfg's pathspec directories.
// Hub-reserved names are filtered from the pathspec, and duplicates are removed.
// It does no file I/O, so a hand-built Config needs no fixture on disk.
func wiredNamesFromConfig(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs()))
}

// TestStructuralNameSets_ForAConfigNamingNeitherStructuralDirectory asserts the following:
// The wired name-set always contains `_lym`, which arrives structurally, not from config,
// so an empty or unrelated pathspec cannot remove it.
// The pathspec/commit-routing set always contains `_lym` but never `.lym`:
// if `.lym` reached it, classifyPaths or Commit would route never-committed content into the weft-side commit.
// The wired name-set does contain `.lym`, since structuralNeverCommittedDirs is folded in; the routing set never does.
// That asymmetry is deliberate.
// `.lym` is refused as a worktree slug even when the caller-supplied junctionNames argument is empty.
// The refusal comes from the structuralNeverCommittedDirs union in IsReservedHubName.
// It does not depend on what a config wires.
// slugReservedNames contains lymdirs.DotLymDirName exactly once, sourced from structuralNeverCommittedDirs.
//
//testtiming:keep _lym always wired and routed, .lym wired but never routed, .lym refused as a slug and counted once in the reserved set; coverage of its blocks by other tests does not show an assertion of this
func TestStructuralNameSets_ForAConfigNamingNeitherStructuralDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := Config{Pathspec: "_extra"}

	wired := wiredNamesFromConfig(cfg)
	if !containsName(wired, "_lym") {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want it to contain %q", cfg, wired, "_lym")
	}
	if !containsName(wired, ".lym") {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want it to contain %q", cfg, wired, ".lym")
	}

	routing := pathspecNames(cfg)
	if !containsName(routing, "_lym") {
		t.Errorf("pathspecNames(%+v) = %v; want it to contain %q", cfg, routing, "_lym")
	}
	if containsName(routing, ".lym") {
		t.Errorf("pathspecNames(%+v) = %v; want it to NEVER contain %q", cfg, routing, ".lym")
	}

	if !IsReservedHubName(".lym", nil) {
		t.Errorf("IsReservedHubName(%q, nil) = false; want true", ".lym")
	}

	slugReserved := slugReservedNames(cfg)
	if count := countName(slugReserved, lymdirs.DotLymDirName); count != 1 {
		t.Errorf("slugReservedNames(%+v) = %v; want exactly one %q, got %d", cfg, slugReserved, lymdirs.DotLymDirName, count)
	}
}

// TestDeployedLymPathspec_YieldsNoDuplicateLym asserts a deployed `pathspec: "_lym _pattern"` Config.
// `_lym` arrives from both the structural set and cfg.Dirs() in the same call.
// The wired set, the routing set, and the slug-reservation set must each hold exactly one `_lym`.
// Without dedup, duplicate names would reach WarpJunctions, ScopedPathspec, and status output.
// The value keeps `_pattern` on purpose, unlike the package's other exemplar Configs.
// Reconcile keeps a `pathspec:` key already present in a worktree's fabric.yaml and never widens it;
// so an already-deployed repo keeps its value, and only a fresh clone gets the empty template default.
//
//testtiming:keep a deployed "_lym _pattern" pathspec yielding exactly one _lym in the wired, routing and slug-reserved sets; coverage of its blocks by other tests does not show an assertion of this
func TestDeployedLymPathspec_YieldsNoDuplicateLym(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := Config{Pathspec: "_lym _pattern"}

	wired := wiredNamesFromConfig(cfg)
	if got := countName(wired, "_lym"); got != 1 {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want exactly one %q, got %d", cfg, wired, "_lym", got)
	}

	routing := pathspecNames(cfg)
	if got := countName(routing, "_lym"); got != 1 {
		t.Errorf("pathspecNames(%+v) = %v; want exactly one %q, got %d", cfg, routing, "_lym", got)
	}

	slugReserved := slugReservedNames(cfg)
	if got := countName(slugReserved, "_lym"); got != 1 {
		t.Errorf("slugReservedNames(%+v) = %v; want exactly one %q, got %d", cfg, slugReserved, "_lym", got)
	}
}

// TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens asserts the following:
// HubReservedNames returns exactly the three hub-structural tokens, with `.lym` absent.
// HubReservedNames is the junction-wiring block set that scanOnDiskJunctionNames relies on to see `.lym`.
//
//testtiming:keep HubReservedNames returning exactly the three hub tokens without .lym; coverage of its blocks by other tests does not show an assertion of this
func TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens(t *testing.T) {
	tiergate.Require(t, 0)
	want := []string{BoardDirName, portalsDirName, launchersDirName}
	got := HubReservedNames()
	if !stringSlicesEqual(got, want) {
		t.Errorf("HubReservedNames() = %v; want %v", got, want)
	}
	if containsName(got, ".lym") {
		t.Errorf("HubReservedNames() = %v; want it to NEVER contain %q", got, ".lym")
	}
}

func containsName(names []string, name string) bool {
	return countName(names, name) > 0
}

// countName reports how many times name appears in names, so a test can assert an exact count.
func countName(names []string, name string) int {
	count := 0
	for _, n := range names {
		if n == name {
			count++
		}
	}
	return count
}
