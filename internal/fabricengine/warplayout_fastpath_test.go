// warplayout_fastpath_test.go pins warpLayoutFor's spawn-free hub-sibling branch in Tier 1: every
// Location field must be carried over from the caller's layout or derived from the sibling path —
// the integration-tagged warplayout_test.go proves equivalence against the spawning resolver, but
// an untagged run never compiles it, leaving the pure field-carriage regression invisible there.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestWarpLayoutFor_FastPathCarriesEveryField(t *testing.T) {
	t.Parallel()

	base := &lyxcwd.Location{
		RepoName:     "mono",
		HubPath:      filepath.Join(t.TempDir(), "mono-HUB"),
		WorktreeName: "mono",
		AnchorRel:    "backend",
	}
	sibling := filepath.Join(base.HubPath, "task1")

	got, err := warpLayoutFor(base, sibling)
	if err != nil {
		t.Fatalf("warpLayoutFor(hub sibling) error = %v", err)
	}

	want := lyxcwd.Location{
		RepoName:     "mono",
		HubPath:      base.HubPath,
		WorktreeName: "task1",
		AnchorRel:    "backend",
	}
	if *got != want {
		t.Errorf("warpLayoutFor(hub sibling) = %+v; want %+v", *got, want)
	}
}
