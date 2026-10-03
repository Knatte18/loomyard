// removefields_test.go covers removeFields, the pure mapping from a RemoveResult and the composite's SessionResult to the `lyx fabric remove` envelope fields:
// the warp-branch keys, the teardown keys, the conditional keys absent when empty, and the unchanged existing ones.

package fabriccli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

func TestRemoveFields_WarpBranchDeleted(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{Slug: "s", WarpBranchDeleted: true}, pairteardown.SessionResult{})
	if got, ok := f["warp_branch_deleted"]; !ok || got != true {
		t.Fatalf("warp_branch_deleted = %v (present %v), want true", got, ok)
	}
	if _, ok := f["warp_branch_kept_reason"]; ok {
		t.Fatalf("warp_branch_kept_reason present for a deleted branch: %v", f)
	}
}

func TestRemoveFields_WarpBranchKept(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{Slug: "s", WarpBranchKeptReason: "unmerged commits"}, pairteardown.SessionResult{})
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
	}, pairteardown.SessionResult{})
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
}

func TestRemoveFields_TeardownKeys(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{
		Slug:                       "s",
		Steps:                      []string{"a", "b"},
		Finished:                   true,
		StrayPath:                  "/stray",
		RemoteWarpBranchDeleted:    true,
		RemoteWarpBranchKeptReason: "not landed",
	}, pairteardown.SessionResult{Ended: true, AbandonedSession: "other"})
	if steps, _ := f["steps"].([]string); len(steps) != 2 {
		t.Errorf("steps = %v, want two entries", f["steps"])
	}
	want := map[string]any{
		"finished":                       true,
		"stray_path":                     "/stray",
		"remote_warp_branch_deleted":     true,
		"remote_warp_branch_kept_reason": "not landed",
		"session_ended":                  true,
		"abandoned_session":              "other",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %v, want %v", k, f[k], v)
		}
	}
}

func TestRemoveFields_ConditionalKeysAbsentWhenEmpty(t *testing.T) {
	f := removeFields(fabricengine.RemoveResult{Slug: "s"}, pairteardown.SessionResult{})
	for _, k := range []string{"stray_path", "abandoned_session", "remote_warp_branch_kept_reason", "warp_branch_kept_reason"} {
		if _, ok := f[k]; ok {
			t.Errorf("%s present although empty: %v", k, f)
		}
	}
	steps, ok := f["steps"].([]string)
	if !ok || steps == nil {
		t.Errorf("steps = %#v, want a non-nil array", f["steps"])
	}
	// The always-present key set: the existing keys plus steps, finished, remote_warp_branch_deleted and session_ended.
	const wantKeys = 7 + 4
	if len(f) != wantKeys {
		t.Errorf("key count = %d, want %d: %v", len(f), wantKeys, f)
	}
}
