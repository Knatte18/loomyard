// weftname_test.go exercises SiblingPath and BareSiblingPath over a range of container/base
// combinations,
// and locks in the relationship between them that gitkit's fixture builders rely on, so the two
// on-disk shapes can never independently drift.

package weftname_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestSiblingPaths covers SiblingPath's and BareSiblingPath's container/base joins over a range of path shapes, including a nested container and a multi-segment base.
// Every row also asserts the relationship gitkit's fixture builders depend on: a weft sibling's bare-remote fixture name is SiblingPath's own result with "-bare" appended, never an independently-derived literal.
// That is the drift BareSiblingPath exists to prevent between production geometry and the on-disk shape test fixtures must reproduce for the same input.
func TestSiblingPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		container   string
		base        string
		wantSibling string
		wantBare    string
	}{
		{"simple", "/h", "feat", filepath.Join("/h", "feat-weft"), filepath.Join("/h", "feat-weft-bare")},
		{"nested_container", "/repos/loomyard-LYXHUB", "main", filepath.Join("/repos/loomyard-LYXHUB", "main-weft"), filepath.Join("/repos/loomyard-LYXHUB", "main-weft-bare")},
		{"multi_segment_base", "/h", "my-feature", filepath.Join("/h", "my-feature-weft"), filepath.Join("/h", "my-feature-weft-bare")},
		{"hub_base", "/tmp/x", "hub", filepath.Join("/tmp/x", "hub-weft"), filepath.Join("/tmp/x", "hub-weft-bare")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sibling := weftname.SiblingPath(tt.container, tt.base)
			if sibling != tt.wantSibling {
				t.Errorf("SiblingPath(%q, %q) = %q; want %q", tt.container, tt.base, sibling, tt.wantSibling)
			}
			bare := weftname.BareSiblingPath(tt.container, tt.base)
			if bare != tt.wantBare {
				t.Errorf("BareSiblingPath(%q, %q) = %q; want %q", tt.container, tt.base, bare, tt.wantBare)
			}
			if want := sibling + "-bare"; bare != want {
				t.Errorf("BareSiblingPath(%q, %q) = %q; want SiblingPath+\"-bare\" = %q", tt.container, tt.base, bare, want)
			}
		})
	}
}
