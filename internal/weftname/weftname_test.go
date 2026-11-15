// weftname_test.go exercises SiblingPath over a range of container/base combinations.

package weftname_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestSiblingPaths covers the container/base joins of SiblingPath over a range of path shapes.
// The shapes include a nested container and a multi-segment base.
func TestSiblingPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		container   string
		base        string
		wantSibling string
	}{
		{"simple", "/h", "feat", filepath.Join("/h", "feat-weft")},
		{"nested_container", "/repos/loomyard-LYMHUB", "main", filepath.Join("/repos/loomyard-LYMHUB", "main-weft")},
		{"multi_segment_base", "/h", "my-feature", filepath.Join("/h", "my-feature-weft")},
		{"hub_base", "/tmp/x", "hub", filepath.Join("/tmp/x", "hub-weft")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sibling := weftname.SiblingPath(tt.container, tt.base)
			if sibling != tt.wantSibling {
				t.Errorf("SiblingPath(%q, %q) = %q; want %q", tt.container, tt.base, sibling, tt.wantSibling)
			}
		})
	}
}
