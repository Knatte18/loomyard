// structuraldirs_test.go — pure set-arithmetic coverage over a hand-built Config for the structural
// directory sets (structuralCommittedDirs, structuralNeverCommittedDirs) and the name-sets built on
// top of them: the wired name-set, the pathspec/commit-routing set, and the slug-reservation set.
// These assertions are the trio a naive one-list implementation fails: any two of the three can pass
// while the third is broken (see the batch's own doc comment for the full argument), so all three
// live together in this one file rather than being scattered across the package's other test files.

package fabricengine

import "testing"

// wiredNamesFromConfig reproduces junctionNames' pure name-arithmetic (dedupUnion of
// structuralCommittedDirs and the hub-reserved-filtered pathspec directories) without the file I/O
// junctionNames itself performs, so this file can assert over a hand-built Config with no fixture on
// disk.
func wiredNamesFromConfig(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs()))
}

// TestWiredNames_ContainsLyxEvenForAConfigNamingNeitherStructuralDirectory asserts that the wired
// name-set always contains `_lyx`, even for a Config whose Pathspec names neither structural
// directory: `_lyx` arrives structurally, not from config, so an empty or unrelated pathspec cannot
// remove it.
func TestWiredNames_ContainsLyxEvenForAConfigNamingNeitherStructuralDirectory(t *testing.T) {
	cfg := Config{Pathspec: "_extra"}
	got := wiredNamesFromConfig(cfg)
	if !containsName(got, "_lyx") {
		t.Errorf("wiredNamesFromConfig(%+v) = %v; want it to contain %q", cfg, got, "_lyx")
	}
}

// TestDeployedLyxPathspec_YieldsNoDuplicateLyx asserts that a deployed `pathspec: "_lyx _pattern"`
// Config — `_lyx` arriving from both the structural set and cfg.Dirs() in the same call — yields no
// duplicate `_lyx` in the wired set, the routing set, or the slug-reservation set. Without dedup,
// duplicate names would reach HostJunctions, ScopedPathspec, and status output.
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

// TestPathspecNames_ContainsLyxButNeverDotLyx asserts that the pathspec/commit-routing set always
// contains `_lyx` but never `.lyx`, for a Config naming neither structural directory: `.lyx` reaching
// this set would let a caller's classifyPaths/Commit call route never-committed content into the
// weft-side commit.
func TestPathspecNames_ContainsLyxButNeverDotLyx(t *testing.T) {
	cfg := Config{Pathspec: "_extra"}
	got := pathspecNames(cfg)
	if !containsName(got, "_lyx") {
		t.Errorf("pathspecNames(%+v) = %v; want it to contain %q", cfg, got, "_lyx")
	}
	if containsName(got, ".lyx") {
		t.Errorf("pathspecNames(%+v) = %v; want it to NEVER contain %q", cfg, got, ".lyx")
	}
}

// TestIsReservedHubName_RefusesDotLyxAsAWorktreeSlug asserts that `.lyx` is refused as a worktree
// slug even when the caller-supplied junctionNames argument is empty — the refusal must come from
// IsReservedHubName's own structuralNeverCommittedDirs union, not from whatever a particular config
// happens to wire.
func TestIsReservedHubName_RefusesDotLyxAsAWorktreeSlug(t *testing.T) {
	if !IsReservedHubName(".lyx", nil) {
		t.Errorf("IsReservedHubName(%q, nil) = false; want true", ".lyx")
	}
}

// TestHubReservedNames_StillReturnsExactlyTheFourHubStructuralTokens asserts that HubReservedNames()
// — the junction-wiring block set scanOnDiskJunctionNames relies on to see `.lyx` at all — still
// returns exactly the four hub-structural tokens, with `.lyx` absent.
func TestHubReservedNames_StillReturnsExactlyTheFourHubStructuralTokens(t *testing.T) {
	want := []string{BoardDirName, portalsDirName, launchersDirName, "_raddle"}
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
