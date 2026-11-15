// codelocation_test.go tests fabricengine.CodeLocation's paths that resolve nothing and return the location it was given.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestCodeLocationReturnsTheLocationUnchanged pins the paths on which CodeLocation resolves nothing:
// a worktree that is no weft sibling, and a weft sibling whose paired code directory is absent.
func TestCodeLocationReturnsTheLocationUnchanged(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	tests := []struct {
		name     string
		location lymcwd.Location
	}{
		{"NotAWeftSibling", lymcwd.Location{HubPath: hub, WorktreeName: "feat"}},
		{"NoCodeDirectory", lymcwd.Location{HubPath: hub, WorktreeName: "gone" + weftname.Suffix}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := fabricengine.CodeLocation(&tt.location)
			if err != nil {
				t.Fatalf("CodeLocation() error = %v; want nil", err)
			}
			if got != &tt.location {
				t.Errorf("CodeLocation() = %+v; want the location passed in, %+v", got, &tt.location)
			}
		})
	}
}
