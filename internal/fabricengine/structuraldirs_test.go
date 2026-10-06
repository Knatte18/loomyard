// structuraldirs_test.go — pure set-arithmetic coverage over a hand-built Config for the structural
// directory sets (structuralCommittedDirs, structuralNeverCommittedDirs) and the name-sets built on
// top of them: the wired name-set, the pathspec/commit-routing set, and the slug-reservation set.
// These assertions are the trio a naive one-list implementation fails: any two of the three can pass
// while the third is broken (see the batch's own doc comment for the full argument), so all three
// live together in this one file rather than being scattered across the package's other test files.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// wiredNamesFromConfig reproduces junctionNames' pure name-arithmetic (dedupUnion of
// structuralCommittedDirs, structuralNeverCommittedDirs, and the hub-reserved-filtered pathspec
// directories) without the file I/O junctionNames itself performs, so this file can assert over a
// hand-built Config with no fixture on disk.
func wiredNamesFromConfig(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs()))
}

// TestStructuralNameSets_ForAConfigNamingNeitherStructuralDirectory covers the name-sets for a
// Config whose Pathspec names neither structural directory:
// the wired name-set always contains `_lyx`, which arrives structurally, not from config, so an empty
// or unrelated pathspec cannot remove it;
// the pathspec/commit-routing set always contains `_lyx` but never `.lyx`, since `.lyx` reaching it
// would let a caller's classifyPaths/Commit call route never-committed content into the weft-side
// commit;
// the wired name-set does contain `.lyx` (structuralNeverCommittedDirs folded in) while the
// routing set still never does, the one assertion that pins the deliberate asymmetry between the two
// sets;
// `.lyx` is refused as a worktree slug even when the caller-supplied junctionNames argument is
// empty, the refusal coming from IsReservedHubName's own structuralNeverCommittedDirs union and not
// from whatever a particular config happens to wire;
// and slugReservedNames contains lyxdirs.DotLyxDirName exactly once, sourced from
// structuralNeverCommittedDirs, proving the removal of hubSlugReservedNames() was
// behaviour-preserving: the refusal holds identically on both sides of that fold.
//
//testtiming:keep _lyx always wired and routed, .lyx wired but never routed, .lyx refused as a slug and counted once in the reserved set; coverage of its blocks by other tests does not show an assertion of this
func TestStructuralNameSets_ForAConfigNamingNeitherStructuralDirectory(t *testing.T) {
	cfg := Config{Pathspec: "_extra"}

	wired := wiredNamesFromConfig(cfg)
	if !containsName(wired, "_lyx") {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want it to contain %q", cfg, wired, "_lyx")
	}
	if !containsName(wired, ".lyx") {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want it to contain %q", cfg, wired, ".lyx")
	}

	routing := pathspecNames(cfg)
	if !containsName(routing, "_lyx") {
		t.Errorf("pathspecNames(%+v) = %v; want it to contain %q", cfg, routing, "_lyx")
	}
	if containsName(routing, ".lyx") {
		t.Errorf("pathspecNames(%+v) = %v; want it to NEVER contain %q", cfg, routing, ".lyx")
	}

	if !IsReservedHubName(".lyx", nil) {
		t.Errorf("IsReservedHubName(%q, nil) = false; want true", ".lyx")
	}

	slugReserved := slugReservedNames(cfg)
	if count := countName(slugReserved, lyxdirs.DotLyxDirName); count != 1 {
		t.Errorf("slugReservedNames(%+v) = %v; want exactly one %q, got %d", cfg, slugReserved, lyxdirs.DotLyxDirName, count)
	}
}

// TestDeployedLyxPathspec_YieldsNoDuplicateLyx asserts that a deployed `pathspec: "_lyx _pattern"`
// Config — `_lyx` arriving from both the structural set and cfg.Dirs() in the same call — yields no
// duplicate `_lyx` in the wired set, the routing set, or the slug-reservation set. Without dedup,
// duplicate names would reach WarpJunctions, ScopedPathspec, and status output.
// The `"_lyx _pattern"` value is deliberate and is not retargeted to `_extra` like this batch's other
// exemplar Config values: per the no-migration decision (see the plan's Shared Decisions), a deployed
// repo's pathspec naming `_pattern` is exactly what an already-deployed fabric.yaml keeps indefinitely
// — this task never migrates existing config values — so this is the only test in the package
// exercising a real stale deployed config value rather than a synthetic one.
// See internal/fabricengine/doc.go's narrow-pathspec-asymmetry discussion for the fresh-clone-only
// limitation this models: reconcile keeps a `pathspec:` key already present in a worktree's
// fabric.yaml and never widens it, so an already-deployed repo stays on its existing value forever
// and only a fresh clone picks up the new empty template default.
//
//testtiming:keep a deployed "_lyx _pattern" pathspec yielding exactly one _lyx in the wired, routing and slug-reserved sets; coverage of its blocks by other tests does not show an assertion of this
func TestDeployedLyxPathspec_YieldsNoDuplicateLyx(t *testing.T) {
	cfg := Config{Pathspec: "_lyx _pattern"}

	wired := wiredNamesFromConfig(cfg)
	if got := countName(wired, "_lyx"); got != 1 {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want exactly one %q, got %d", cfg, wired, "_lyx", got)
	}

	routing := pathspecNames(cfg)
	if got := countName(routing, "_lyx"); got != 1 {
		t.Errorf("pathspecNames(%+v) = %v; want exactly one %q, got %d", cfg, routing, "_lyx", got)
	}

	slugReserved := slugReservedNames(cfg)
	if got := countName(slugReserved, "_lyx"); got != 1 {
		t.Errorf("slugReservedNames(%+v) = %v; want exactly one %q, got %d", cfg, slugReserved, "_lyx", got)
	}
}

// TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens asserts that HubReservedNames()
// — the junction-wiring block set scanOnDiskJunctionNames relies on to see `.lyx` at all — still
// returns exactly the three hub-structural tokens, with `.lyx` absent.
//
//testtiming:keep HubReservedNames returning exactly the three hub tokens without .lyx; coverage of its blocks by other tests does not show an assertion of this
func TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens(t *testing.T) {
	want := []string{BoardDirName, portalsDirName, launchersDirName}
	got := HubReservedNames()
	if !stringSlicesEqual(got, want) {
		t.Errorf("HubReservedNames() = %v; want %v", got, want)
	}
	if containsName(got, ".lyx") {
		t.Errorf("HubReservedNames() = %v; want it to NEVER contain %q", got, ".lyx")
	}
}

// containsName reports whether names contains name.
func containsName(names []string, name string) bool {
	return countName(names, name) > 0
}

// countName reports how many times name appears in names, so a duplicate-detection assertion can
// state its expectation as an exact count rather than a boolean.
func countName(names []string, name string) int {
	count := 0
	for _, n := range names {
		if n == name {
			count++
		}
	}
	return count
}
