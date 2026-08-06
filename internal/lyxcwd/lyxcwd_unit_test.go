// lyxcwd_unit_test.go — pure path-math unit tests for a couple of Location
// methods with distinct anchoring rules (DotLyxDir, HubLogsDir). These
// tests do not require a git repository and run under standard unit test
// verification.

package lyxcwd

import (
	"path/filepath"
	"testing"
)

// TestDotLyxDir verifies that DotLyxDir resolves to "<AnchorPath>/.lyx" and is distinct
// from LyxDir ("<AnchorPath>/_lyx"), since the two directories serve different durability
// contracts (ephemeral/machine-bound vs. durable/weft-synced).
func TestDotLyxDir(t *testing.T) {
	t.Parallel()

	hubPath := filepath.Join("home", "user")
	loc := &Location{HubPath: hubPath, WorktreeName: "project", AnchorRel: "."}
	want := filepath.Join(hubPath, "project", ".lyx")

	got := loc.DotLyxDir()

	if got != want {
		t.Errorf("DotLyxDir() = %q; want %q", got, want)
	}

	if got == loc.LyxDir() {
		t.Errorf("DotLyxDir() = %q; must be distinct from LyxDir() = %q", got, loc.LyxDir())
	}
}

// TestHubLogsDir verifies that HubLogsDir resolves to "<HubPath>/.lyx/logs" — pure
// path math, hub-anchored (not worktree-anchored), with no filesystem I/O.
func TestHubLogsDir(t *testing.T) {
	t.Parallel()

	hubPath := filepath.Join("home", "user", "project-HUB")
	loc := &Location{HubPath: hubPath}

	got := loc.HubLogsDir()
	want := filepath.Join(hubPath, ".lyx", "logs")

	if got != want {
		t.Errorf("HubLogsDir() = %q; want %q", got, want)
	}
}
