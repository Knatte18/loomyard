//go:build integration

// launchers_containment_integration_test.go drives the real `add` verb end-to-end against a symlink
// planted at the per-slug launcher directory (<hub>/_launchers/<anchor>/<slug>) and at the _launchers
// container itself, both pointing OUTSIDE the hub — the create-side twin of the delete-side M3 the R2
// crucible round reproduced against removeLaunchers.
//
// It is the full-stack companion to the launchers containment fix (writeLaunchers rooting every write
// through an os.Root at the hub): a raw os.MkdirAll+os.WriteFile followed the planted symlink and wrote
// executable launcher scripts to the attacker's target while reporting ok:true, so this test proves the
// whole Add call now (a) fails closed, (b) writes nothing outside the hub, and (c) never records a
// launcher file_written that did not land inside the hub.
//
// Package fabricengine_test to reuse newFabricFixture and the shared TestMain, matching the convention
// of the other integration tests in this package.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestAdd_DoesNotWriteOutsideHubThroughLauncherSymlink asserts that a launcher directory (or the
// _launchers container) pre-planted as a symlink escaping the hub makes Add fail closed rather than
// writing launcher scripts to the symlink's out-of-hub target — the confirmed round-7 create-side
// containment defect.
func TestAdd_DoesNotWriteOutsideHubThroughLauncherSymlink(t *testing.T) {
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
			fixture := newFabricFixture(t)
			l := fixture.Layout

			// An out-of-hub directory the launcher writes would land in if containment were bypassed.
			outside := t.TempDir()

			planted := tt.plant(l.HubPath, slug)
			// The container vector plants the symlink AT _launchers; the leaf vector plants it one level
			// down, so _launchers must exist first for the leaf case.
			if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
				t.Fatalf("mkdir parent of planted symlink: %v", err)
			}
			if err := os.Symlink(outside, planted); err != nil {
				t.Fatalf("plant escaping symlink at %s: %v", planted, err)
			}

			topology := fabricengine.NewTopology(fabricengine.Config{})
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

			// The mutation record must not claim a launcher file_written: a false-success record naming a
			// hub-relative launcher path while the bytes escaped is precisely the M3 shape this fix closes.
			for _, m := range res.Mutated().Entries() {
				if m.Kind == fabricengine.KindFileWritten && strings.Contains(m.Target, "_launchers") {
					t.Fatalf("mutation record claims a launcher write %q that did not land inside the hub", m.Target)
				}
			}
		})
	}
}
