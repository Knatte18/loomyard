// portals_test.go covers the retired-artifact sweep where it needs no git: a hub whose prime cannot be resolved.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// TestSweepRetiredHubArtifacts_UnresolvablePrime proves the sweep removes a fabric-shaped portal link and keeps the retired workspace file when the prime cannot be resolved.
func TestSweepRetiredHubArtifacts_UnresolvablePrime(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	l := locationkit.Location(hub, "prime", ".")

	link := filepath.Join(hub, portalsDirName, l.AnchorRel, "slug")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("create the _portals anchor dir: %v", err)
	}
	if err := fslink.CreateDirLink(link, retiredPortalTarget(l, "slug")); err != nil {
		t.Fatalf("plant the portal link: %v", err)
	}
	workspaceFile := filepath.Join(launchersDir(l), l.AnchorRel, "prime"+hubWorkspaceExt)
	if err := os.MkdirAll(filepath.Dir(workspaceFile), 0o755); err != nil {
		t.Fatalf("create the _launchers anchor dir: %v", err)
	}
	if err := os.WriteFile(workspaceFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("plant the workspace file: %v", err)
	}

	details := sweepRetiredHubArtifacts(NewMutations(hub), l)

	if len(details) != 0 {
		t.Errorf("sweepRetiredHubArtifacts details = %v; want none", details)
	}
	if _, err := os.Lstat(filepath.Join(hub, portalsDirName)); !os.IsNotExist(err) {
		t.Errorf("_portals stat err = %v; want it removed with its link", err)
	}
	if _, err := os.Stat(workspaceFile); err != nil {
		t.Errorf("workspace file stat err = %v; want it kept while the prime cannot be resolved", err)
	}
}
