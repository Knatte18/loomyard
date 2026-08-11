//go:build integration

// manifest_test.go proves CaptureManifest, DiffManifest, and AssertNoUnpermittedChange against real
// hubs from NewHub: the round-trip properties a permit diff must hold, both directions of the ".git"
// allowlist, the two portability guarantees the package's own doc comments promise, and the one walk
// property a Linux-only run would otherwise never catch — that a wired junction's target is recorded
// once, under its own path, never a second time under the junction's path.

package fabrictest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
)

// firstWiredJunction returns the absolute path of one of h's wired junction links, along with its
// hub-relative key, failing tb if the hub carries none.
func firstWiredJunction(tb testing.TB, h *Hub) (linkAbs, linkRel string) {
	tb.Helper()

	names, err := fabricengine.WiredNames(h.BoardDir())
	if err != nil {
		tb.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
	}
	if len(names) == 0 {
		tb.Fatalf("fabricengine.WiredNames(%s): want at least one wired name, got none", h.BoardDir())
	}

	linkAbs = filepath.Join(h.PrimeWorktree(), h.Anchor, names[0])
	rel, relErr := filepath.Rel(h.Path, linkAbs)
	if relErr != nil {
		tb.Fatalf("filepath.Rel(%s, %s): %v", h.Path, linkAbs, relErr)
	}
	return linkAbs, filepath.ToSlash(rel)
}

// changeAt returns the Change recorded for path in changes, or nil if none was.
func changeAt(changes []Change, path string) *Change {
	for i := range changes {
		if changes[i].Path == path {
			return &changes[i]
		}
	}
	return nil
}

func TestManifestRoundTrip(t *testing.T) {
	t.Run("UnchangedHubDiffsEmpty", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		before := CaptureManifest(t, h.Path)
		after := CaptureManifest(t, h.Path)

		if changes := DiffManifest(before, after, nil); len(changes) != 0 {
			t.Errorf("DiffManifest(unchanged hub) = %v; want no changes", changes)
		}
	})

	t.Run("DeletionOutsidePermittedRootIsReported", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		scratch := filepath.Join(h.Path, "scratch-unpermitted.txt")
		if err := os.WriteFile(scratch, []byte("scratch"), 0o644); err != nil {
			t.Fatalf("write %s: %v", scratch, err)
		}
		before := CaptureManifest(t, h.Path)

		if err := os.Remove(scratch); err != nil {
			t.Fatalf("remove %s: %v", scratch, err)
		}
		after := CaptureManifest(t, h.Path)

		changes := DiffManifest(before, after, nil)
		if change := changeAt(changes, "scratch-unpermitted.txt"); change == nil || change.Kind != ChangeRemoved {
			t.Errorf("DiffManifest = %v; want scratch-unpermitted.txt reported as removed", changes)
		}
	})

	t.Run("DeletionUnderPermittedRootIsNotReported", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		permittedDir := filepath.Join(h.Path, "scratch-permitted")
		if err := os.Mkdir(permittedDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", permittedDir, err)
		}
		nested := filepath.Join(permittedDir, "nested.txt")
		if err := os.WriteFile(nested, []byte("scratch"), 0o644); err != nil {
			t.Fatalf("write %s: %v", nested, err)
		}
		before := CaptureManifest(t, h.Path)

		if err := os.Remove(nested); err != nil {
			t.Fatalf("remove %s: %v", nested, err)
		}
		after := CaptureManifest(t, h.Path)

		changes := DiffManifest(before, after, []string{"scratch-permitted"})
		if change := changeAt(changes, "scratch-permitted/nested.txt"); change != nil {
			t.Errorf("DiffManifest(permitted root) = %v; want scratch-permitted/nested.txt absent", changes)
		}
		AssertNoUnpermittedChange(t, before, after, []string{"scratch-permitted"})
	})

	t.Run("LinkTargetChangeIsReported", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		linkAbs, linkRel := firstWiredJunction(t, h)
		before := CaptureManifest(t, h.Path)

		newTarget := filepath.Join(t.TempDir(), "elsewhere")
		if err := fslink.Remove(linkAbs); err != nil {
			t.Fatalf("fslink.Remove(%s): %v", linkAbs, err)
		}
		if err := fslink.CreateDirLink(linkAbs, newTarget); err != nil {
			t.Fatalf("fslink.CreateDirLink(%s, %s): %v", linkAbs, newTarget, err)
		}
		after := CaptureManifest(t, h.Path)

		changes := DiffManifest(before, after, nil)
		if change := changeAt(changes, linkRel); change == nil || change.Kind != ChangeLinkTargetChanged {
			t.Errorf("DiffManifest = %v; want %s reported as link target changed", changes, linkRel)
		}
	})

	t.Run("KindChangeIsReported", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		linkAbs, linkRel := firstWiredJunction(t, h)
		before := CaptureManifest(t, h.Path)

		if err := fslink.Remove(linkAbs); err != nil {
			t.Fatalf("fslink.Remove(%s): %v", linkAbs, err)
		}
		if err := os.WriteFile(linkAbs, []byte("a file where a link was"), 0o644); err != nil {
			t.Fatalf("write %s: %v", linkAbs, err)
		}
		after := CaptureManifest(t, h.Path)

		changes := DiffManifest(before, after, nil)
		if change := changeAt(changes, linkRel); change == nil || change.Kind != ChangeKindChanged {
			t.Errorf("DiffManifest = %v; want %s reported as kind changed", changes, linkRel)
		}
	})
}

func TestManifestGitAllowlist(t *testing.T) {
	t.Run("OrdinaryGitStatusAndCommitProduceEmptyDiff", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		repoDir := h.PrimeWorktree()
		before := CaptureManifest(t, h.Path)

		// An ordinary status probe followed by an empty commit only ever touches index/logs/refs/
		// objects churn inside .git — nothing this harness's allowlist should ever surface.
		GitStatusPorcelain(t, repoDir)
		mustGit(repoDir, "commit", "--allow-empty", "-m", "fabrictest: manifest git-allowlist probe")

		after := CaptureManifest(t, h.Path)
		AssertNoUnpermittedChange(t, before, after, nil)
	})

	t.Run("LinkedWorktreeDeregistrationIsReported", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		const slug = "wtdereg"
		AddPair(t, h, slug)

		registryDir := filepath.Join(h.PrimeWeft(), gitDirName, "worktrees")
		entries, err := os.ReadDir(registryDir)
		if err != nil {
			t.Fatalf("os.ReadDir(%s): %v", registryDir, err)
		}
		if len(entries) == 0 {
			t.Fatalf("os.ReadDir(%s): want at least one worktree registration, got none", registryDir)
		}
		registration := filepath.Join(registryDir, entries[0].Name())
		registrationRel, relErr := filepath.Rel(h.Path, registration)
		if relErr != nil {
			t.Fatalf("filepath.Rel(%s, %s): %v", h.Path, registration, relErr)
		}

		before := CaptureManifest(t, h.Path)
		if err := os.RemoveAll(registration); err != nil {
			t.Fatalf("os.RemoveAll(%s): %v", registration, err)
		}
		after := CaptureManifest(t, h.Path)

		changes := DiffManifest(before, after, nil)
		if change := changeAt(changes, filepath.ToSlash(registrationRel)); change == nil || change.Kind != ChangeRemoved {
			t.Errorf("DiffManifest = %v; want %s reported as removed", changes, registrationRel)
		}
	})
}

func TestManifestPortability(t *testing.T) {
	t.Run("KeysAreToSlashNormalised", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		manifest := CaptureManifest(t, h.Path)

		for key := range manifest {
			if strings.Contains(key, `\`) {
				t.Errorf("manifest key %q contains a backslash; want filepath.ToSlash-normalised keys throughout", key)
			}
		}
	})

	t.Run("PermitRootWithForwardSlashMatchesRunningPlatform", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		nestedDir := filepath.Join(h.Path, "scratch-nested", "deeper")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", nestedDir, err)
		}
		nestedFile := filepath.Join(nestedDir, "leaf.txt")
		if err := os.WriteFile(nestedFile, []byte("scratch"), 0o644); err != nil {
			t.Fatalf("write %s: %v", nestedFile, err)
		}
		before := CaptureManifest(t, h.Path)

		if err := os.Remove(nestedFile); err != nil {
			t.Fatalf("remove %s: %v", nestedFile, err)
		}
		after := CaptureManifest(t, h.Path)

		// The permit root is written with a plain "/" regardless of the running platform's own
		// separator; DiffManifest's segment-wise matching is what makes this portable.
		AssertNoUnpermittedChange(t, before, after, []string{"scratch-nested/deeper"})
	})
}

// TestManifestWiredJunctionWalk proves the one property a Linux-only run would otherwise never catch:
// a path reachable only by descending through a wired junction is recorded exactly once, under the
// weft sibling's own path, and never a second time under the junction's path.
func TestManifestWiredJunctionWalk(t *testing.T) {
	t.Parallel()

	h := NewHub(t, ".")
	linkAbs, linkRel := firstWiredJunction(t, h)

	rawTarget, err := fslink.RawTarget(linkAbs)
	if err != nil {
		t.Fatalf("fslink.RawTarget(%s): %v", linkAbs, err)
	}
	targetEntries, err := os.ReadDir(rawTarget)
	if err != nil {
		t.Fatalf("os.ReadDir(%s): %v", rawTarget, err)
	}
	if len(targetEntries) == 0 {
		t.Fatalf("os.ReadDir(%s): want at least one entry behind the wired junction %s, got none", rawTarget, linkAbs)
	}
	targetChildRel, relErr := filepath.Rel(h.Path, filepath.Join(rawTarget, targetEntries[0].Name()))
	if relErr != nil {
		t.Fatalf("filepath.Rel(%s, %s): %v", h.Path, rawTarget, relErr)
	}
	targetChildKey := filepath.ToSlash(targetChildRel)

	manifest := CaptureManifest(t, h.Path)

	entry, junctionRecorded := manifest[linkRel]
	if !junctionRecorded || entry.Kind != KindLink {
		t.Errorf("manifest[%q] = %v, %v; want a KindLink entry", linkRel, entry, junctionRecorded)
	}

	if _, doubled := manifest[linkRel+"/"+targetEntries[0].Name()]; doubled {
		t.Errorf("manifest contains %s/%s; want the junction's target never recorded a second time under the junction's own path", linkRel, targetEntries[0].Name())
	}

	if _, recordedOnce := manifest[targetChildKey]; !recordedOnce {
		t.Errorf("manifest[%q] missing; want the weft sibling's content recorded once, under its own path", targetChildKey)
	}
}
