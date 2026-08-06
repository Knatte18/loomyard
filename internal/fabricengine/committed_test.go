// committed_test.go table-drives CommitResult.Committed() over its four WarpCommitted/WeftCommitted
// combinations. No fixture: CommitResult is a plain value type.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestCommitResult_Committed covers all four WarpCommitted/WeftCommitted combinations.
func TestCommitResult_Committed(t *testing.T) {
	tests := []struct {
		name          string
		warpCommitted bool
		weftCommitted bool
		want          bool
	}{
		{"neither committed", false, false, false},
		{"warp only", true, false, true},
		{"weft only", false, true, true},
		{"both committed", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := fabricengine.CommitResult{WarpCommitted: tt.warpCommitted, WeftCommitted: tt.weftCommitted}
			if got := r.Committed(); got != tt.want {
				t.Errorf("CommitResult{WarpCommitted: %v, WeftCommitted: %v}.Committed() = %v; want %v",
					tt.warpCommitted, tt.weftCommitted, got, tt.want)
			}
		})
	}
}
