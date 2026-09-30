// removefields_test.go covers removeFields, the pure mapping from a RemoveResult to the
// `lyx fabric remove` envelope fields: the warp-branch keys and the unchanged existing ones.

package fabriccli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

func TestRemoveFields_WarpBranchDeleted(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{Slug: "s", WarpBranchDeleted: true})
	if got, ok := f["warp_branch_deleted"]; !ok || got != true {
		t.Fatalf("warp_branch_deleted = %v (present %v), want true", got, ok)
	}
	if _, ok := f["warp_branch_kept_reason"]; ok {
		t.Fatalf("warp_branch_kept_reason present for a deleted branch: %v", f)
	}
}

func TestRemoveFields_WarpBranchKept(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{Slug: "s", WarpBranchKeptReason: "unmerged commits"})
	if got, ok := f["warp_branch_deleted"]; !ok || got != false {
		t.Fatalf("warp_branch_deleted = %v (present %v), want false", got, ok)
	}
	if got := f["warp_branch_kept_reason"]; got != "unmerged commits" {
		t.Fatalf("warp_branch_kept_reason = %v, want %q", got, "unmerged commits")
	}
}

func TestRemoveFields_ExistingKeysUnchanged(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{
		Slug:                "s",
		Path:                "/p",
		LinksRemoved:        2,
		RemoteBranchDeleted: true,
		RemoteBranchError:   "boom",
		RemoteSkippedReason: "no origin",
	})
	want := map[string]any{
		"slug":                  "s",
		"path":                  "/p",
		"links_removed":         2,
		"remote_branch_deleted": true,
		"remote_branch_error":   "boom",
		"remote_skipped_reason": "no origin",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %v, want %v", k, f[k], v)
		}
	}
	if len(f) != len(want)+1 {
		t.Errorf("unexpected key set: %v", f)
	}
}
