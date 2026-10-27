//go:build tier0

// structuraldirs_test.go — pure set-arithmetic coverage over a hand-built Config for the structural
// directory sets (structuralCommittedDirs, structuralNeverCommittedDirs) and the name-sets built on
// top of them: the wired name-set, the pathspec/commit-routing set, and the slug-reservation set.
// These assertions are the trio a naive one-list implementation fails: any two of the three can pass
// while the third is broken (see the batch's own doc comment for the full argument), so all three
// live together in this one file rather than being scattered across the package's other test files.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
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
// the wired name-set always contains `_lym`, which arrives structurally, not from config, so an empty
// or unrelated pathspec cannot remove it;
// the pathspec/commit-routing set always contains `_lym` but never `.lym`, since `.lym` reaching it
// would let a caller's classifyPaths/Commit call route never-committed content into the weft-side
// commit;
// the wired name-set does contain `.lym` (structuralNeverCommittedDirs folded in) while the
// routing set still never does, the one assertion that pins the deliberate asymmetry between the two
// sets;
// `.lym` is refused as a worktree slug even when the caller-supplied junctionNames argument is
// empty, the refusal coming from IsReservedHubName's own structuralNeverCommittedDirs union and not
// from whatever a particular config happens to wire;
// and slugReservedNames contains lymdirs.DotLymDirName exactly once, sourced from
// structuralNeverCommittedDirs, proving the removal of hubSlugReservedNames() was
// behaviour-preserving: the refusal holds identically on both sides of that fold.
//
//testtiming:keep _lym always wired and routed, .lym wired but never routed, .lym refused as a slug and counted once in the reserved set; coverage of its blocks by other tests does not show an assertion of this
func TestStructuralNameSets_ForAConfigNamingNeitherStructuralDirectory(t *testing.T) {
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

// TestDeployedLymPathspec_YieldsNoDuplicateLym asserts that a deployed `pathspec: "_lym _pattern"`
// Config — `_lym` arriving from both the structural set and cfg.Dirs() in the same call — yields no
// duplicate `_lym` in the wired set, the routing set, or the slug-reservation set. Without dedup,
// duplicate names would reach WarpJunctions, ScopedPathspec, and status output.
// The `"_lym _pattern"` value is deliberate and is not retargeted to `_extra` like this batch's other
// exemplar Config values: per the no-migration decision (see the plan's Shared Decisions), a deployed
// repo's pathspec naming `_pattern` is exactly what an already-deployed fabric.yaml keeps indefinitely
// — this task never migrates existing config values — so this is the only test in the package
// exercising a real stale deployed config value rather than a synthetic one.
// See internal/fabricengine/doc.go's narrow-pathspec-asymmetry discussion for the fresh-clone-only
// limitation this models: reconcile keeps a `pathspec:` key already present in a worktree's
// fabric.yaml and never widens it, so an already-deployed repo stays on its existing value forever
// and only a fresh clone picks up the new empty template default.
//
//testtiming:keep a deployed "_lym _pattern" pathspec yielding exactly one _lym in the wired, routing and slug-reserved sets; coverage of its blocks by other tests does not show an assertion of this
func TestDeployedLymPathspec_YieldsNoDuplicateLym(t *testing.T) {
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

// TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens asserts that HubReservedNames()
// — the junction-wiring block set scanOnDiskJunctionNames relies on to see `.lym` at all — still
// returns exactly the three hub-structural tokens, with `.lym` absent.
//
//testtiming:keep HubReservedNames returning exactly the three hub tokens without .lym; coverage of its blocks by other tests does not show an assertion of this
func TestHubReservedNames_StillReturnsExactlyTheThreeHubStructuralTokens(t *testing.T) {
	want := []string{BoardDirName, portalsDirName, launchersDirName}
	got := HubReservedNames()
	if !stringSlicesEqual(got, want) {
		t.Errorf("HubReservedNames() = %v; want %v", got, want)
	}
	if containsName(got, ".lym") {
		t.Errorf("HubReservedNames() = %v; want it to NEVER contain %q", got, ".lym")
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
