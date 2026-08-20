//go:build integration

// cli_integration_test.go covers lookupContext's hub-mode branch — a real hubforge.NewHub fixture,
// at both an unanchored (".") and a subpath-anchored ("backend") hub — so it is integration-tagged
// per the Test Tier Purity Invariant. The out-of-hub branch is covered by cli_test.go's untagged
// TestLookupContext_OutsideHubReturnsAbsoluteAnchorRootAndBuiltinRegistry.

package scoutcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/scoutengine"
)

// TestLookupContext_HubModeReturnsAnchorRootAndRegistry verifies lookupContext's hub-mode branch at
// both an unanchored and a subpath-anchored hub: the returned anchor root is always
// h.Location.AnchorPath(), never a mismatched dir argument or the worktree path, and at a
// subpath-anchored hub the servers.yaml overlay is still loaded from the anchor.
func TestLookupContext_HubModeReturnsAnchorRootAndRegistry(t *testing.T) {
	t.Run("unanchored hub", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")

		// dir is deliberately a separate t.TempDir(), never h.PrimeWorktree(): a same-value fixture
		// would not discriminate an implementation that wrongly took the out-of-hub branch and
		// returned filepath.Abs(dir) instead of the hub's anchor root.
		dir := t.TempDir()

		_, anchorRoot, err := lookupContext(h.PrimeWorktree(), dir)
		if err != nil {
			t.Fatalf("lookupContext(%q, %q) error = %v; want nil", h.PrimeWorktree(), dir, err)
		}

		if anchorRoot != h.Location.AnchorPath() {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want %q (h.Location.AnchorPath())", h.PrimeWorktree(), dir, anchorRoot, h.Location.AnchorPath())
		}
		absDir, absErr := filepath.Abs(dir)
		if absErr != nil {
			t.Fatalf("filepath.Abs(%q) error = %v", dir, absErr)
		}
		if anchorRoot == absDir {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want it not to equal filepath.Abs(dir) = %q — this would mean the out-of-hub branch ran instead of the hub branch", h.PrimeWorktree(), dir, anchorRoot, absDir)
		}
	})

	t.Run("subpath-anchored hub", func(t *testing.T) {
		h := hubforge.NewHub(t, "backend")
		hubforge.SeedConfig(t, h, map[string]string{
			"servers": `
lyxtest:
  markers:
    - go.mod
  match: any
  command:
    - lyxtest-langserver
  install_hint: go install example.com/lyxtest-langserver@latest
`,
		})

		// dir is deliberately a separate t.TempDir(), matching the unanchored case above.
		dir := t.TempDir()

		// cwd is h.Location.AnchorPath(), never h.PrimeWorktree(): the latter returns
		// WorktreePath(), which at a subpath-anchored hub sits outside the anchor and makes
		// lyxcwd.Resolve return ErrCwdOutsideAnchor, exercising the degraded out-of-hub branch
		// while appearing to test the hub branch.
		registry, anchorRoot, err := lookupContext(h.Location.AnchorPath(), dir)
		if err != nil {
			t.Fatalf("lookupContext(%q, %q) error = %v; want nil", h.Location.AnchorPath(), dir, err)
		}

		if anchorRoot != h.Location.AnchorPath() {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want %q (h.Location.AnchorPath())", h.Location.AnchorPath(), dir, anchorRoot, h.Location.AnchorPath())
		}
		if anchorRoot == h.Location.WorktreePath() {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want it not to equal h.Location.WorktreePath() = %q at a subpath anchor", h.Location.AnchorPath(), dir, anchorRoot, h.Location.WorktreePath())
		}

		if _, ok := registry["lyxtest"]; !ok {
			t.Errorf("lookupContext(%q, %q) registry missing seeded entry %q; got keys %v — LoadRegistry must still read servers.yaml from AnchorPath(), not WorktreePath()", h.Location.AnchorPath(), dir, "lyxtest", registryKeys(registry))
		}
	})
}

// registryKeys returns the keys of registry, for use in a test failure message.
func registryKeys(registry scoutengine.Registry) []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}
