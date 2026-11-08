// livestate_manifest_selftest_test.go proves the manifest walk against a real hub built by hubforge.
// It covers the one walk property a Linux-only run would otherwise never catch:
// a wired junction's target is recorded once, under its own path, never a second time under the junction's path.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// firstWiredJunction returns the absolute path of one of h's wired junction links, along with its
// hub-relative key, failing tb if the hub carries none.
func firstWiredJunction(tb testing.TB, h *hubforge.Hub) (linkAbs, linkRel string) {
	tb.Helper()

	names, err := fabricengine.WiredNames(configengine.Dirs{Board: h.BoardDir()})
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

// TestManifestWiredJunctionWalk proves a property a Linux-only run would otherwise never catch:
// a path reachable only through a wired junction is recorded exactly once, under the weft sibling's own path.
// The junction's path never records it a second time.
//
//testtiming:keep a path reachable only through a wired junction being recorded once under the weft sibling's path; coverage of its blocks by other tests does not show an assertion of this
func TestManifestWiredJunctionWalk(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
