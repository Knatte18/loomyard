//go:build integration

// destroy_containment_toctou_integration_test.go drives the real `remove` verb end-to-end against a
// symlink planted at an intermediate segment of a gate target (the per-slug launcher directory,
// <hub>/_launchers/<anchor>/<slug>) and pointing OUTSIDE the hub — the shape R3's crucible round
// reproduced as a gated removal escaping the hub.
//
// It is the full-stack companion to destroy_toctou_test.go's hermetic unit guard: the unit test pins
// removeContainedPath's act-time escape refusal directly, while this test proves the whole Remove
// call — slug validation, portal/launcher teardown, the gate pipeline, the rooted removal — never
// deletes a file outside the hub through the planted symlink, and preserves the outside content.
//
// Package fabricengine_test to reuse newFabricFixture and the shared TestMain, matching the
// convention of the other integration tests in this package.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestRemove_DoesNotDeleteOutsideHubThroughLauncherSymlink builds a real pair, replaces its launcher
// directory with a symlink escaping the hub, runs `remove --force`, and asserts the outside files the
// symlink points at are preserved — the gated removal must never follow the escaping segment out of
// the hub.
func TestRemove_DoesNotDeleteOutsideHubThroughLauncherSymlink(t *testing.T) {
	t.Parallel()

	const slug = "toctou-launcher"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Build an out-of-hub directory carrying the exact file names removeLaunchers targets, so any
	// follow-through the removal does would delete them.
	outside := t.TempDir()
	canaries := []string{"ide.sh", "fabric-checkout.sh", "run.sh"}
	for _, name := range canaries {
		if err := os.WriteFile(filepath.Join(outside, name), []byte("PRECIOUS"), 0o644); err != nil {
			t.Fatalf("seed outside canary %s: %v", name, err)
		}
	}

	// Replace the real per-slug launcher directory — an intermediate segment of every launcher-script
	// gate target — with a symlink that escapes the hub, live at removal time.
	launcherDir := fabricengine.LauncherDir(l, slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		t.Fatalf("remove real launcher dir: %v", err)
	}
	if err := os.Symlink(outside, launcherDir); err != nil {
		t.Fatalf("plant escaping launcher symlink: %v", err)
	}

	// Remove --force must not carry the removal outside the hub through the escaping symlink.
	_, _ = topology.Remove(l, slug, true)

	for _, name := range canaries {
		if _, err := os.Stat(filepath.Join(outside, name)); err != nil {
			t.Fatalf("outside file %s was deleted through the escaping launcher symlink: %v", name, err)
		}
	}
}
