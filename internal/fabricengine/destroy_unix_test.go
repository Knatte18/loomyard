//go:build tier0 && !windows

// destroy_unix_test.go holds the destroy gate tests that plant symlinks, which Windows creates only with a privilege.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGate_ContainmentResolvesSymlinkedAncestors is R2's regression for the symlink-mediated containment bypass:
// a link planted at an intermediate segment of a gate target used to satisfy the containment check, because the check related NOMINAL paths through filepath.Rel and never resolved anything.
// Each subtest pins one half of the fix — the escape must be refused,
// and the two shapes that legitimately involve a link (a link AS the target, a real path under a symlinked container) must still pass — so a future simplification back to a bare filepath.Rel fails here rather than silently reopening the hole.
func TestGate_ContainmentResolvesSymlinkedAncestors(t *testing.T) {
	t.Run("SymlinkedParentSegmentEscapesAndIsRefused", func(t *testing.T) {
		// This mirrors the shape R2's live repro used against removeLaunchers exactly, ownership kind included:
		// ownedUnderGeometryRoot is declared here deliberately rather than a kind that fails anyway, because a kind that refuses on its own would leave the subtest passing for the wrong reason and prove nothing about containment.
		hub := t.TempDir()
		root := filepath.Join(hub, launchersDirName)
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("mkdir geometry root: %v", err)
		}

		outside := t.TempDir()
		victim := filepath.Join(outside, "victim.txt")
		if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
			t.Fatalf("write victim: %v", err)
		}

		// <root>/slug links to a directory outside the hub, so <root>/slug/victim.txt is nominally inside the geometry root and really outside the hub entirely.
		if err := os.Symlink(outside, filepath.Join(root, "slug")); err != nil {
			t.Fatalf("symlink slug: %v", err)
		}
		target := filepath.Join(root, "slug", "victim.txt")

		req := pathRequest{
			what:      "remove launcher script",
			container: root,
			target:    target,
			ownership: ownedUnderGeometryRoot(root),
			dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckContainment)

		// Assert the executor itself refuses too, not merely the check helper, and that the file outside the container survives:
		// before the fix this call deleted it and reported success.
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
		// The link's own final component is the target.
		// Resolving it would relocate the target outside container and refuse every junction removal, so containment must not resolve it.
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
		// Both sides are resolved, so a container reached through a link (macOS's /var -> /private/var is the everyday case) must not start refusing its own real children.
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

	t.Run("GeometryRootOwnershipRefusesASymlinkedRoot", func(t *testing.T) {
		// ownedUnderGeometryRoot is the one ownership kind with no independent resolved-path authority to cross-check against,
		// so a link standing where the geometry root belongs must be refused by the base-name test running against the RESOLVED root.
		hub := t.TempDir()
		outside := t.TempDir()
		root := filepath.Join(hub, launchersDirName)
		if err := os.Symlink(outside, root); err != nil {
			t.Fatalf("symlink geometry root: %v", err)
		}

		ok, reason := resolvePathOwnership(ownedUnderGeometryRoot(root), filepath.Join(root, "slug"))
		if ok {
			t.Fatalf("resolvePathOwnership(ownedUnderGeometryRoot(<symlinked %s>)) = true; want false", launchersDirName)
		}
		if !strings.Contains(reason, "not a fabric geometry root") {
			t.Errorf("reason = %q; want it to name the geometry-root refusal", reason)
		}
	})
}
