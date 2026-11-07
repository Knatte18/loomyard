// clone_explicit_root_subpath_test.go pins that CloneHub refuses an explicit `--subpath .`:
// the hub's recorded anchor is a real subpath.
// An unset subpath still adopts the recorded anchor.
//
// The file is in package fabricengine_test:
// it reuses the bare-remote fixture helpers from clone_adopt_test.go and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCloneHub_ExplicitRootSubpathRefusesRecordedSubpath asserts an explicit "." is refused against
// a recorded "backend", and that leaving Subpath unset still adopts the record.
func TestCloneHub_ExplicitRootSubpathRefusesRecordedSubpath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "explicit-root-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "explicit-root-weft")
	commitFileOnBranch(t, fixtures, weftBare, "main", lymcwd.AnchorFileName, "backend\n")

	cloneParent := t.TempDir()

	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: ".",
	})
	if err == nil {
		t.Fatal("CloneHub(Subpath: \".\") = nil error against a hub recorded at \"backend\"; want a refusal")
	}
	if !strings.Contains(err.Error(), "does not match the recorded anchor") {
		t.Errorf("CloneHub() error = %v; want it to name the recorded-anchor mismatch", err)
	}

	// A refused clone leaves no hub behind.
	if entries, readErr := os.ReadDir(cloneParent); readErr != nil {
		t.Fatalf("read clone parent: %v", readErr)
	} else if len(entries) != 0 {
		t.Errorf("clone parent holds %d entries after a refused clone; want none", len(entries))
	}

	// An unset subpath still adopts the recorded anchor, so the empty default must not become a refusal.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
	})
	if err != nil {
		t.Fatalf("CloneHub() with no Subpath error = %v; want the recorded anchor adopted", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
	if res.Anchor != "backend" {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, "backend")
	}
}
