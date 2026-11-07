// destroy_containment_toctou_integration_test.go runs the real `remove` verb end to end.
// It replaces the per-slug launcher directory with a symlink that points outside the hub.
// It checks that the removal never deletes a file outside the hub through that symlink.
// It is in package fabricengine_test to reuse the shared TestMain.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemove_DoesNotDeleteOutsideHubThroughLauncherSymlink builds a real pair.
// It replaces the launcher directory with a symlink that escapes the hub, then runs `remove --force`.
// It asserts that the outside files the symlink points at are preserved.
// The gated removal must never follow the escaping segment out of the hub.
func TestRemove_DoesNotDeleteOutsideHubThroughLauncherSymlink(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "toctou-launcher"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// Build an out-of-hub directory that holds the file names removeLaunchers targets,
	// so any follow-through by the removal would delete them.
	outside := t.TempDir()
	canaries := []string{"ide.sh", "fabric-checkout.sh", "run.sh"}
	for _, name := range canaries {
		if err := os.WriteFile(filepath.Join(outside, name), []byte("PRECIOUS"), 0o644); err != nil {
			t.Fatalf("seed outside canary %s: %v", name, err)
		}
	}

	// Replace the real per-slug launcher directory with a symlink that escapes the hub.
	// The directory is an intermediate segment of every launcher-script gate target,
	// so the symlink is live at removal time.
	launcherDir := fabricengine.LauncherDir(l, slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		t.Fatalf("remove real launcher dir: %v", err)
	}
	if err := os.Symlink(outside, launcherDir); err != nil {
		t.Fatalf("plant escaping launcher symlink: %v", err)
	}

	_, _ = topology.Remove(l, slug, true, false)

	for _, name := range canaries {
		if _, err := os.Stat(filepath.Join(outside, name)); err != nil {
			t.Fatalf("outside file %s was deleted through the escaping launcher symlink: %v", name, err)
		}
	}
}
