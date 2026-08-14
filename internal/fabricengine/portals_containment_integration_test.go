//go:build integration

// portals_containment_integration_test.go drives the real `add` verb against a symlink planted at the
// _portals container escaping the hub — the create-side portal twin of the delete-side M3.
//
// fslink.CreateDirLink's own os.MkdirAll(filepath.Dir(link)) followed such a container symlink and planted
// the portal junction OUTSIDE the hub while Add reported ok:true; this test proves createPortal now
// materialises the portal's parent through an os.Root at the hub, so the escape is refused and no junction
// lands outside. The leaf vector is already refused by fslink's refuse-to-clobber guard and is covered by
// existing portal tests, so this test targets the container vector specifically.
//
// Package fabricengine_test to reuse newFabricFixture and the shared TestMain.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestAdd_DoesNotCreatePortalOutsideHubThroughContainerSymlink pre-plants <hub>/_portals as a symlink
// escaping the hub and asserts Add fails closed rather than creating the portal junction in the symlink's
// out-of-hub target.
func TestAdd_DoesNotCreatePortalOutsideHubThroughContainerSymlink(t *testing.T) {
	t.Parallel()

	const slug = "toctou-portal"
	fixture := newFabricFixture(t)
	l := fixture.Layout

	outside := t.TempDir()
	portalsContainer := fabricengine.PortalsDir(l)
	if err := os.Symlink(outside, portalsContainer); err != nil {
		t.Fatalf("plant escaping _portals symlink: %v", err)
	}

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("Add(%q) succeeded through an escaping _portals symlink; want a fail-closed error", slug)
	}

	// No portal entry may have been created in the out-of-hub target.
	if entries, err := os.ReadDir(outside); err == nil && len(entries) > 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("portal artefacts landed outside the hub through the escaping _portals symlink: %v", names)
	}

	// The planted container must remain the symlink the test placed — Add must not have followed it and
	// materialised a portal beneath it.
	if _, err := os.Stat(filepath.Join(outside, slug)); err == nil {
		t.Fatalf("portal %q was created inside the escaping symlink's target", slug)
	}
}
