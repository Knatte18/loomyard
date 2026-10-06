// removefields_test.go covers removeFields, the pure mapping from a RemoveResult and the composite's SessionResult to the `lyx fabric remove` envelope fields:
// the warp-branch keys, the teardown keys, the conditional keys absent when empty, and the unchanged existing ones.

package fabriccli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

func TestRemoveFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  fabricengine.RemoveResult
		session pairteardown.SessionResult
		// wantFields are keys that must be present with exactly this value.
		wantFields map[string]any
		// wantAbsent are keys that must not be present.
		wantAbsent []string
		// wantKeyCount, when non-zero, is the exact number of keys.
		wantKeyCount int
	}{
		{
			name:       "WarpBranchDeleted",
			result:     fabricengine.RemoveResult{Slug: "s", WarpBranchDeleted: true},
			wantFields: map[string]any{"warp_branch_deleted": true},
			wantAbsent: []string{"warp_branch_kept_reason"},
		},
		{
			name:   "WarpBranchKept",
			result: fabricengine.RemoveResult{Slug: "s", WarpBranchKeptReason: "unmerged commits"},
			wantFields: map[string]any{
				"warp_branch_deleted":     false,
				"warp_branch_kept_reason": "unmerged commits",
			},
		},
		{
			name: "ExistingKeysUnchanged",
			result: fabricengine.RemoveResult{
				Slug:                "s",
				Path:                "/p",
				LinksRemoved:        2,
				RemoteBranchDeleted: true,
				RemoteBranchError:   "boom",
				RemoteSkippedReason: "no origin",
			},
			wantFields: map[string]any{
				"slug":                  "s",
				"path":                  "/p",
				"links_removed":         2,
				"remote_branch_deleted": true,
				"remote_branch_error":   "boom",
				"remote_skipped_reason": "no origin",
			},
		},
		{
			name: "TeardownKeys",
			result: fabricengine.RemoveResult{
				Slug:                       "s",
				Steps:                      []string{"a", "b"},
				Finished:                   true,
				StrayPath:                  "/stray",
				RemoteWarpBranchDeleted:    true,
				RemoteWarpBranchKeptReason: "not landed",
			},
			session: pairteardown.SessionResult{Ended: true, AbandonedSession: "other"},
			wantFields: map[string]any{
				"finished":                       true,
				"stray_path":                     "/stray",
				"remote_warp_branch_deleted":     true,
				"remote_warp_branch_kept_reason": "not landed",
				"session_ended":                  true,
				"abandoned_session":              "other",
			},
		},
		{
			name:   "ConditionalKeysAbsentWhenEmpty",
			result: fabricengine.RemoveResult{Slug: "s"},
			wantAbsent: []string{
				"stray_path", "abandoned_session", "remote_warp_branch_kept_reason", "warp_branch_kept_reason",
			},
			// The always-present key set: the existing keys plus steps, finished, remote_warp_branch_deleted and session_ended.
			wantKeyCount: 7 + 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := removeFields(tt.result, tt.session)

			for key, want := range tt.wantFields {
				got, present := f[key]
				if !present || got != want {
					t.Errorf("%s = %v (present %v), want %v", key, got, present, want)
				}
			}
			for _, key := range tt.wantAbsent {
				if _, present := f[key]; present {
					t.Errorf("%s present although empty: %v", key, f)
				}
			}
			steps, ok := f["steps"].([]string)
			if !ok || steps == nil || len(steps) != len(tt.result.Steps) {
				t.Errorf("steps = %#v, want a non-nil array of %d entries", f["steps"], len(tt.result.Steps))
			}
			if tt.wantKeyCount != 0 && len(f) != tt.wantKeyCount {
				t.Errorf("key count = %d, want %d: %v", len(f), tt.wantKeyCount, f)
			}
		})
	}
}
