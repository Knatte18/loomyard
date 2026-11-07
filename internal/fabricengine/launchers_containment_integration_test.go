// launchers_containment_integration_test.go drives the real `add` verb end-to-end against a symlink
// planted at the per-slug launcher directory (<hub>/_launchers/<anchor>/<slug>) and at the _launchers
// container itself, both pointing outside the hub.
//
// The test checks the whole Add call.
// It must fail closed and write nothing outside the hub.
// It must also never record a launcher file_written that did not land in the hub.
// writeLaunchers roots every write through an os.Root at the hub;
// a raw os.MkdirAll and os.WriteFile would follow the planted symlink.
// It would write executable launcher scripts to the out-of-hub target while reporting success.
//
// The file is package fabricengine_test, which reuses the shared TestMain as other integration tests here do.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAdd_DoesNotWriteOutsideHubThroughLauncherSymlink asserts that a launcher directory, or the
// _launchers container, pre-planted as a symlink that escapes the hub, makes Add fail closed:
// it must not write launcher scripts to the symlink's out-of-hub target.
func TestAdd_DoesNotWriteOutsideHubThroughLauncherSymlink(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// plant returns the path the test replaces with an escaping symlink, given the hub and slug.
		plant func(hub, slug string) string
	}{
		{
			name: "leaf",
			plant: func(hub, slug string) string {
				return filepath.Join(hub, "_launchers", slug)
			},
		},
		{
			name: "container",
			plant: func(hub, slug string) string {
				return filepath.Join(hub, "_launchers")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const slug = "toctou-write"
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location

			// An out-of-hub directory the launcher writes would land in if containment were bypassed.
			outside := t.TempDir()

			planted := tt.plant(l.HubPath, slug)
			// The container vector plants the symlink at _launchers;
			// the leaf vector plants it one level down, so _launchers must exist first for the leaf case.
			if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
				t.Fatalf("mkdir parent of planted symlink: %v", err)
			}
			if err := os.Symlink(outside, planted); err != nil {
				t.Fatalf("plant escaping symlink at %s: %v", planted, err)
			}

			topology := h.Topology
			res, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
			if err == nil {
				t.Fatalf("Add(%q) succeeded through an escaping launcher symlink; want a fail-closed error", slug)
			}

			// No launcher script may have landed in the out-of-hub target.
			for _, name := range []string{"ide.sh", "fabric-checkout.sh", "run.sh", "ide-menu.sh"} {
				if _, statErr := os.Stat(filepath.Join(outside, name)); statErr == nil {
					t.Fatalf("launcher %s was written outside the hub through the escaping symlink", name)
				}
			}

			// The mutation record must not claim a launcher file_written:
			// a record naming a hub-relative launcher path while the bytes escaped would report a false success.
			for _, m := range res.Mutated().Entries() {
				if m.Kind == fabricengine.KindFileWritten && strings.Contains(m.Target, "_launchers") {
					t.Fatalf("mutation record claims a launcher write %q that did not land inside the hub", m.Target)
				}
			}
		})
	}
}
