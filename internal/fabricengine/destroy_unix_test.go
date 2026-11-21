//go:build !windows

// destroy_unix_test.go holds the destroy gate tests that plant symlinks, which Windows creates only with a privilege.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGate_ContainmentResolvesSymlinkedAncestors pins the containment check against symlinked ancestors.
// A link planted at an intermediate segment of a gate target must be refused as an escape.
// A bare filepath.Rel compares nominal paths and resolves nothing, so it would accept such a link.
// The subtests also pin two shapes that must pass:
// a link as the target itself, and a real path under a symlinked container.
func TestGate_ContainmentResolvesSymlinkedAncestors(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("SymlinkedParentSegmentEscapesAndIsRefused", func(t *testing.T) {
		hub := t.TempDir()
		root := filepath.Join(hub, "container")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("mkdir container: %v", err)
		}

		outside := t.TempDir()
		victim := filepath.Join(outside, "victim.txt")
		if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
			t.Fatalf("write victim: %v", err)
		}

		// <root>/slug links to a directory outside the hub.
		// So <root>/slug/victim.txt is nominally inside the container but outside the hub.
		if err := os.Symlink(outside, filepath.Join(root, "slug")); err != nil {
			t.Fatalf("symlink slug: %v", err)
		}
		target := filepath.Join(root, "slug", "victim.txt")

		req := pathRequest{
			what:      "remove file",
			container: root,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("the file is a generated artifact, never edited content"),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckContainment)

		// The executor must refuse too, not merely the check helper, and the file outside the container must survive.
		// A removal that followed the link would delete that file and report success.
		rec := NewMutations(hub)
		if err := removePath(rec, req); err == nil {
			t.Fatal("removePath through a symlinked parent segment = nil; want a containment refusal")
		}
		if _, err := os.Stat(victim); err != nil {
			t.Errorf("victim outside the container was destroyed: %v", err)
		}
		if rec.Len() != 0 {
			t.Errorf("rec.Len() = %d; want 0 — a refusal records nothing", rec.Len())
		}
	})

	t.Run("LinkAsTargetItselfStillPassesContainment", func(t *testing.T) {
		container := t.TempDir()
		outside := t.TempDir()
		// The link's own final component is the target, and containment must not resolve it:
		// resolving it would move the target outside the container and refuse every junction removal.
		target := filepath.Join(container, "junction")
		if err := os.Symlink(outside, target); err != nil {
			t.Fatalf("symlink junction: %v", err)
		}

		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("irrelevant: this subtest reads containment's verdict off the later ownership refusal"),
		}
		// Containment passes; ownership then refuses, since a link to an empty dir is not a hub.
		assertRefusalCheck(t, checkPathRequest(req), CheckOwnership)
	})

	t.Run("SymlinkedContainerAncestorStillPassesContainment", func(t *testing.T) {
		// Both sides are resolved, so a container reached through a link must still accept its real children.
		// macOS's /var -> /private/var is the everyday case of such a link.
		realDir := t.TempDir()
		linkParent := t.TempDir()
		container := filepath.Join(linkParent, "via-link")
		if err := os.Symlink(realDir, container); err != nil {
			t.Fatalf("symlink container: %v", err)
		}
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir child: %v", err)
		}

		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("irrelevant: this subtest reads containment's verdict off the later ownership refusal"),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckOwnership)
	})
}
