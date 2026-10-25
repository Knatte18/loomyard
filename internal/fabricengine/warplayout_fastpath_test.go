//go:build tier0

// warplayout_fastpath_test.go pins warpLayoutFor's spawn-free hub-sibling branch in Tier 0.
// Every Location field must be carried over from the caller's layout or derived from the sibling path,
// and the anchor must come from the sibling's own marker rather than the caller's.
// The integration-tagged warplayout_test.go proves equivalence against the spawning resolver, but an untagged run never compiles it.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

//testtiming:keep the fast-path warp layout carrying every field; coverage of its blocks by other tests does not show an assertion of this
func TestWarpLayoutFor_FastPathCarriesEveryField(t *testing.T) {
	t.Parallel()

	base := &lyxcwd.Location{
		RepoName:     "mono",
		HubPath:      filepath.Join(t.TempDir(), "mono-LYXHUB"),
		WorktreeName: "mono",
		AnchorRel:    "backend",
	}
	sibling := filepath.Join(base.HubPath, "task1")
	siblingRecords := weftname.SiblingPath(base.HubPath, "task1")
	if err := os.MkdirAll(siblingRecords, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", siblingRecords, err)
	}
	if err := os.WriteFile(filepath.Join(siblingRecords, lyxcwd.AnchorFileName), []byte("frontend\n"), 0o644); err != nil {
		t.Fatalf("write the sibling's own marker: %v", err)
	}

	got, err := warpLayoutFor(base, sibling)
	if err != nil {
		t.Fatalf("warpLayoutFor(hub sibling) error = %v", err)
	}

	want := lyxcwd.Location{
		RepoName:     "mono",
		HubPath:      base.HubPath,
		WorktreeName: "task1",
		AnchorRel:    "frontend",
	}
	if *got != want {
		t.Errorf("warpLayoutFor(hub sibling) = %+v; want %+v", *got, want)
	}
}
