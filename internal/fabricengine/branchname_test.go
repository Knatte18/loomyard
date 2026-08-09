// branchname_test.go — unit tests for WeftBranchName's uniform derivation.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestWeftBranchName covers the uniform <warp>/<warp>-weft scheme across the primary branch, a
// prefixed task branch, and a plain (empty-prefix) slug.
func TestWeftBranchName(t *testing.T) {
	tests := []struct {
		name       string
		warpBranch string
		want       string
	}{
		{"primary", "main", "main-weft"},
		{"prefixed_task_branch", "hanf/foo", "hanf/foo-weft"},
		{"empty_prefix_plain_slug", "foo", "foo-weft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fabricengine.WeftBranchName(tt.warpBranch)
			if got != tt.want {
				t.Errorf("WeftBranchName(%q) = %q; want %q", tt.warpBranch, got, tt.want)
			}
		})
	}
}

// TestWeftBranchName_RoundTripsWithWeftWarpSlug asserts that fabricengine.WeftWarpSlug, the
// documented inverse, recovers the original warp branch from every WeftBranchName output.
func TestWeftBranchName_RoundTripsWithWeftWarpSlug(t *testing.T) {
	warpBranches := []string{"main", "hanf/foo", "foo"}
	for _, warp := range warpBranches {
		weft := fabricengine.WeftBranchName(warp)
		gotWarp, ok := fabricengine.WeftWarpSlug(weft)
		if !ok {
			t.Errorf("WeftWarpSlug(%q) ok = false; want true", weft)
			continue
		}
		if gotWarp != warp {
			t.Errorf("WeftWarpSlug(%q) = %q; want %q", weft, gotWarp, warp)
		}
	}
}
