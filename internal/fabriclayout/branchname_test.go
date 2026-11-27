// branchname_test.go — unit tests for RecordsBranchName's uniform derivation.

package fabriclayout_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecordsBranchName covers the uniform <warp>/<warp>-weft scheme.
// Its cases are the primary branch, a prefixed task branch, and a plain (empty-prefix) slug.
//
//testtiming:keep the <warp>/<warp>-weft scheme for the primary branch, a prefixed task branch and an empty-prefix slug; coverage of its blocks by other tests does not show an assertion of this
func TestRecordsBranchName(t *testing.T) {
	tiergate.Require(t, 0)
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
			got := fabriclayout.RecordsBranchName(tt.warpBranch)
			if got != tt.want {
				t.Errorf("RecordsBranchName(%q) = %q; want %q", tt.warpBranch, got, tt.want)
			}
		})
	}
}
