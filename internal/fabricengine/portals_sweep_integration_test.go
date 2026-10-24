//go:build tier1

// portals_sweep_integration_test.go proves Reconcile sweeps the retired portal links and the old hub workspace file, keeps and reports everything else.
// It also proves a regeneration warns while a retired artifact remains.
// Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// retiredArtifactWarning is the text of the regeneration's retired-artifact warning that tells it from the failed-regeneration one.
const retiredArtifactWarning = "retired portal links"

// portalsAnchorDir returns <hub>/_portals/<AnchorRel>, where the retired portal links lived.
func portalsAnchorDir(h *hubforge.Hub) string {
	return filepath.Join(h.Path, "_portals", h.Location.AnchorRel)
}

// plantPortalLink plants the link a retired fabric verb created for name: <hub>/_portals/<AnchorRel>/<name> pointing at <hub>/<name>/<AnchorRel>/_lyx, which need not exist.
func plantPortalLink(t *testing.T, h *hubforge.Hub, name string) string {
	t.Helper()
	link := filepath.Join(portalsAnchorDir(h), name)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("create the _portals anchor dir: %v", err)
	}
	if err := fslink.CreateDirLink(link, filepath.Join(h.Path, name, h.Location.AnchorRel, lyxdirs.LyxDirName)); err != nil {
		t.Fatalf("plant the portal link %s: %v", name, err)
	}
	return link
}

// retiredWorkspacePath returns the path of the old hub workspace file under _launchers.
func retiredWorkspacePath(h *hubforge.Hub) string {
	return filepath.Join(h.Path, "_launchers", h.Location.AnchorRel, filepath.Base(h.PrimeWorktree())+".code-workspace")
}

// plantRetiredWorkspaceFile writes a file at the old workspace path and returns the path.
func plantRetiredWorkspaceFile(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	file := retiredWorkspacePath(h)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("create the _launchers anchor dir: %v", err)
	}
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatalf("plant the old workspace file: %v", err)
	}
	return file
}

// reconcileAt runs Reconcile at l on h and fails the test on an error.
func reconcileAt(t *testing.T, h *hubforge.Hub, l *lyxcwd.Location) fabricengine.ReconcileResult {
	t.Helper()
	result, err := h.Topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return result
}

// recordedEntry reports whether the result's mutation record holds an entry of kind for the hub-relative target.
func recordedEntry(result fabricengine.ReconcileResult, kind fabricengine.Kind, target string) bool {
	for _, m := range result.Mutated().Entries() {
		if m.Kind == kind && m.Target == target {
			return true
		}
	}
	return false
}

// assertAbsent fails the test when path exists, a link counting as present.
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s stat err = %v; want it removed", path, err)
	}
}

// assertPresent fails the test when path does not exist, a link counting as present.
func assertPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("%s stat err = %v; want it kept", path, err)
	}
}

// assertDetailNames fails the test unless details holds exactly one entry per want, each naming it.
func assertDetailNames(t *testing.T, details []string, want ...string) {
	t.Helper()
	if len(details) != len(want) {
		t.Errorf("HubDetails = %v; want %d entries naming %v", details, len(want), want)
		return
	}
	for _, name := range want {
		found := false
		for _, detail := range details {
			if strings.Contains(detail, name) {
				found = true
			}
		}
		if !found {
			t.Errorf("HubDetails = %v; want one naming %s", details, name)
		}
	}
}

// TestReconcile_SweepsRetiredHubArtifacts runs each sweep row on a root-anchored and a nested-anchor hub.
// The top level is not parallel: the warning row captures the logger, so every serial row runs before its parent releases the parallel ones.
func TestReconcile_SweepsRetiredHubArtifacts(t *testing.T) {
	for _, anchor := range []string{".", "backend"} {
		t.Run("Anchor_"+anchor, func(t *testing.T) {
			shape := hubforge.Shape{Anchor: anchor}
			portalDirs := []string{"_portals"}
			if anchor != "." {
				portalDirs = append(portalDirs, path.Join("_portals", anchor))
			}
			portalRel := path.Join("_portals", anchor)

			t.Run("RemovesLinksChainAndFileThenNothingMore", func(t *testing.T) {
				t.Parallel()
				for _, from := range []string{"prime", "pair"} {
					t.Run(from, func(t *testing.T) {
						t.Parallel()
						h := hubforge.CopyHub(t, shape)
						hubforge.AddPairWith(t, h, "live", fabricengine.AddOptions{SkipPush: true})
						plantPortalLink(t, h, "live")
						plantPortalLink(t, h, "gone")
						file := plantRetiredWorkspaceFile(t, h)
						l := h.Location
						if from == "pair" {
							pairLocation, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("live"))
							if err != nil {
								t.Fatalf("resolve the pair's location: %v", err)
							}
							l = pairLocation
						}

						first := reconcileAt(t, h, l)

						for _, name := range []string{"live", "gone"} {
							if !recordedEntry(first, fabricengine.KindLinkRemoved, path.Join(portalRel, name)) {
								t.Errorf("Reconcile recorded no link_removed for %s: %+v", name, first.Mutated().Entries())
							}
						}
						for _, dir := range portalDirs {
							if !recordedEntry(first, fabricengine.KindPathRemoved, dir) {
								t.Errorf("Reconcile recorded no path_removed for %s: %+v", dir, first.Mutated().Entries())
							}
						}
						fileRel, _ := filepath.Rel(h.Path, file)
						if !recordedEntry(first, fabricengine.KindPathRemoved, filepath.ToSlash(fileRel)) {
							t.Errorf("Reconcile recorded no path_removed for the old workspace file: %+v", first.Mutated().Entries())
						}
						assertAbsent(t, filepath.Join(h.Path, "_portals"))
						assertAbsent(t, file)
						assertPresent(t, h.PairCodeWorktree("live"))

						second := reconcileAt(t, h, l)

						for _, m := range second.Mutated().Entries() {
							if m.Kind == fabricengine.KindLinkRemoved || m.Kind == fabricengine.KindPathRemoved {
								t.Errorf("second Reconcile recorded %+v; want no removal", m)
							}
						}
						if len(second.HubDetails) != 0 {
							t.Errorf("second Reconcile HubDetails = %v; want none", second.HubDetails)
						}
					})
				}
			})

			t.Run("KeepsForeignEntriesAndReportsThem", func(t *testing.T) {
				t.Parallel()
				h := hubforge.CopyHub(t, shape)
				anchorDir := portalsAnchorDir(h)
				if err := os.MkdirAll(anchorDir, 0o755); err != nil {
					t.Fatalf("create the _portals anchor dir: %v", err)
				}
				notes := filepath.Join(anchorDir, "notes.txt")
				if err := os.WriteFile(notes, []byte("not a portal link"), 0o644); err != nil {
					t.Fatalf("plant the foreign file: %v", err)
				}
				elsewhere := filepath.Join(anchorDir, "elsewhere")
				if err := fslink.CreateDirLink(elsewhere, t.TempDir()); err != nil {
					t.Fatalf("plant the link pointing elsewhere: %v", err)
				}
				want := []string{"notes.txt", "elsewhere"}
				kept := []string{notes, elsewhere}
				if anchor != "." {
					outside := filepath.Join(h.Path, "_portals", "x")
					if err := os.WriteFile(outside, []byte("foreign"), 0o644); err != nil {
						t.Fatalf("plant the foreign file outside the anchor chain: %v", err)
					}
					want = append(want, "_portals/x")
					kept = append(kept, outside)
				}

				result := reconcileAt(t, h, h.Location)

				for _, entry := range kept {
					assertPresent(t, entry)
				}
				assertPresent(t, filepath.Join(h.Path, "_portals"))
				assertDetailNames(t, result.HubDetails, want...)
			})

			t.Run("KeepsADirectoryAtTheOldWorkspacePath", func(t *testing.T) {
				t.Parallel()
				h := hubforge.CopyHub(t, shape)
				dir := retiredWorkspacePath(h)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("plant the directory: %v", err)
				}

				result := reconcileAt(t, h, h.Location)

				assertPresent(t, dir)
				assertDetailNames(t, result.HubDetails, filepath.Base(dir))
			})

			t.Run("RemovesASymlinkAtTheOldWorkspacePathAsALink", func(t *testing.T) {
				t.Parallel()
				h := hubforge.CopyHub(t, shape)
				target := filepath.Join(t.TempDir(), "real.code-workspace")
				if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
					t.Fatalf("write the link target: %v", err)
				}
				link := retiredWorkspacePath(h)
				if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
					t.Fatalf("create the _launchers anchor dir: %v", err)
				}
				if err := os.Symlink(target, link); err != nil {
					if os.IsPermission(err) {
						t.Skipf("symlinks need a privilege this host does not grant: %v", err)
					}
					t.Fatalf("plant the symlink: %v", err)
				}

				result := reconcileAt(t, h, h.Location)

				assertAbsent(t, link)
				assertPresent(t, target)
				if len(result.HubDetails) != 0 {
					t.Errorf("HubDetails = %v; want none", result.HubDetails)
				}
			})

			t.Run("RefusesEscapingDirectoryLinks", func(t *testing.T) {
				t.Parallel()
				t.Run("Portals", func(t *testing.T) {
					t.Parallel()
					h := hubforge.CopyHub(t, shape)
					outside := t.TempDir()
					ghost := filepath.Join(outside, h.Location.AnchorRel, "ghost")
					if err := os.MkdirAll(filepath.Dir(ghost), 0o755); err != nil {
						t.Fatalf("create the outside anchor dir: %v", err)
					}
					if err := fslink.CreateDirLink(ghost, filepath.Join(h.Path, "ghost", h.Location.AnchorRel, lyxdirs.LyxDirName)); err != nil {
						t.Fatalf("plant the fabric-shaped link outside the hub: %v", err)
					}
					if err := fslink.CreateDirLink(filepath.Join(h.Path, "_portals"), outside); err != nil {
						t.Fatalf("replace _portals with a link leaving the hub: %v", err)
					}

					result := reconcileAt(t, h, h.Location)

					assertPresent(t, ghost)
					assertDetailNames(t, result.HubDetails, "_portals")
				})

				t.Run("Launchers", func(t *testing.T) {
					t.Parallel()
					h := hubforge.CopyHub(t, shape)
					outside := t.TempDir()
					outsideFile := filepath.Join(outside, filepath.Base(retiredWorkspacePath(h)))
					if err := os.WriteFile(outsideFile, []byte("{}"), 0o644); err != nil {
						t.Fatalf("plant the file outside the hub: %v", err)
					}
					anchorLaunchers := filepath.Dir(retiredWorkspacePath(h))
					if err := os.RemoveAll(anchorLaunchers); err != nil {
						t.Fatalf("remove the real launchers anchor dir: %v", err)
					}
					if err := fslink.CreateDirLink(anchorLaunchers, outside); err != nil {
						t.Fatalf("replace the launchers anchor dir with a link leaving the hub: %v", err)
					}

					result := reconcileAt(t, h, h.Location)

					assertPresent(t, outsideFile)
					assertDetailNames(t, result.HubDetails, filepath.Base(outsideFile))
				})
			})

			t.Run("RegenerationWarnsWhileAnArtifactRemains", func(t *testing.T) {
				// Serial: it captures the logger.
				h := hubforge.CopyHub(t, shape)
				logs := logcapture.Capture(t)
				regenerationWarns := func() bool {
					logs.Reset()
					if _, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(h.Path), h.Location); err != nil {
						t.Fatalf("RegenerateHubWorkspace: %v", err)
					}
					return strings.Contains(logs.String(), retiredArtifactWarning)
				}

				if regenerationWarns() {
					t.Fatalf("a clean hub warned: %s", logs.String())
				}

				link := plantPortalLink(t, h, "live")
				if !regenerationWarns() || !strings.Contains(logs.String(), "lyx fabric reconcile") {
					t.Errorf("a fabric-shaped link did not warn naming lyx fabric reconcile: %q", logs.String())
				}
				if err := os.Remove(link); err != nil {
					t.Fatalf("remove the link: %v", err)
				}
				plantRetiredWorkspaceFile(t, h)
				if !regenerationWarns() {
					t.Errorf("an old workspace file did not warn: %q", logs.String())
				}

				logs.Reset()
				reconcileAt(t, h, h.Location)
				if strings.Contains(logs.String(), retiredArtifactWarning) {
					t.Errorf("the Reconcile that swept the artifacts warned: %q", logs.String())
				}
				if regenerationWarns() {
					t.Errorf("a regeneration after the sweep warned: %q", logs.String())
				}

				foreign := filepath.Join(portalsAnchorDir(h), "notes.txt")
				if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
					t.Fatalf("create the _portals anchor dir: %v", err)
				}
				if err := os.WriteFile(foreign, []byte("x"), 0o644); err != nil {
					t.Fatalf("plant the foreign file: %v", err)
				}
				if regenerationWarns() {
					t.Errorf("a foreign entry under _portals warned: %q", logs.String())
				}
				if err := os.RemoveAll(filepath.Join(h.Path, "_portals")); err != nil {
					t.Fatalf("remove _portals: %v", err)
				}

				if err := os.MkdirAll(retiredWorkspacePath(h), 0o755); err != nil {
					t.Fatalf("plant a directory at the old workspace path: %v", err)
				}
				if regenerationWarns() {
					t.Errorf("a directory at the old workspace path warned: %q", logs.String())
				}
				if err := os.Remove(retiredWorkspacePath(h)); err != nil {
					t.Fatalf("remove the planted directory: %v", err)
				}

				outside := t.TempDir()
				if err := fslink.CreateDirLink(filepath.Join(h.Path, "_portals"), outside); err != nil {
					t.Fatalf("replace _portals with a link leaving the hub: %v", err)
				}
				if regenerationWarns() {
					t.Errorf("an escaping _portals warned: %q", logs.String())
				}
			})
		})
	}
}
