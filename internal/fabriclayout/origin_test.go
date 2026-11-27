// origin_test.go — unit tests for the Origin provenance record type.

package fabriclayout_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOrigin_JSONRoundTrip asserts that Origin marshals and unmarshals its parent_branch and parent_worktree wire keys.
// A record without the parent_worktree key still decodes, with that field empty.
//
//testtiming:keep Origin's parent_branch and parent_worktree wire keys round-tripping and a legacy record decoding with an empty ParentWorktree; coverage of its blocks by other tests does not show an assertion of this
func TestOrigin_JSONRoundTrip(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		want    fabriclayout.Origin
		wantKey string
	}{
		{"parent_branch", fabriclayout.Origin{ParentBranch: "main"}, `"parent_branch":"main"`},
		{"parent_worktree", fabriclayout.Origin{ParentBranch: "main", ParentWorktree: "prime"}, `"parent_worktree":"prime"`},
		{"beam", fabriclayout.Origin{ParentBranch: "main", Beam: true}, `"beam":true`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if !strings.Contains(string(data), tt.wantKey) {
				t.Errorf("Marshal() = %s; want it to contain %s", data, tt.wantKey)
			}

			var got fabriclayout.Origin
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal() = %+v; want %+v", got, tt.want)
			}
		})
	}

	var legacy fabriclayout.Origin
	if err := json.Unmarshal([]byte(`{"parent_branch":"main"}`), &legacy); err != nil {
		t.Fatalf("Unmarshal(legacy) error = %v", err)
	}
	if legacy.ParentWorktree != "" {
		t.Errorf("legacy ParentWorktree = %q; want empty", legacy.ParentWorktree)
	}
	if legacy.Beam {
		t.Errorf("legacy Beam = true; want false")
	}

	data, err := json.Marshal(fabriclayout.Origin{ParentBranch: "main"})
	if err != nil {
		t.Fatalf("Marshal(no beam) error = %v", err)
	}
	if strings.Contains(string(data), `"beam"`) {
		t.Errorf("Marshal(no beam) = %s; want no beam key", data)
	}
}
