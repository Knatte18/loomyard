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

// TestSiblingPath covers SiblingPath's container/base join over a range of path shapes, including a
// nested container and a multi-segment base.
func TestSiblingPath(t *testing.T) {
	tests := []struct {
		name      string
		container string
		base      string
		want      string
	}{
		{"simple", "/h", "feat", filepath.Join("/h", "feat-weft")},
		{"nested_container", "/repos/loomyard-HUB", "main", filepath.Join("/repos/loomyard-HUB", "main-weft")},
		{"multi_segment_base", "/h", "my-feature", filepath.Join("/h", "my-feature-weft")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftname.SiblingPath(tt.container, tt.base)
			if got != tt.want {
				t.Errorf("SiblingPath(%q, %q) = %q; want %q", tt.container, tt.base, got, tt.want)
			}
		})
	}
}

// TestBareSiblingPath covers BareSiblingPath's container/base join, the bare-remote fixture
// directory paired with a weft sibling.
func TestBareSiblingPath(t *testing.T) {
	tests := []struct {
		name      string
		container string
		base      string
		want      string
	}{
		{"simple", "/h", "feat", filepath.Join("/h", "feat-weft-bare")},
		{"nested_container", "/tmp/x", "hub", filepath.Join("/tmp/x", "hub-weft-bare")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftname.BareSiblingPath(tt.container, tt.base)
			if got != tt.want {
				t.Errorf("BareSiblingPath(%q, %q) = %q; want %q", tt.container, tt.base, got, tt.want)
			}
		})
	}
}

// TestBareSiblingPath_AgreesWithSiblingPath locks in the relationship gitkit's fixture builders
// depend on: a weft sibling's bare-remote fixture name is always SiblingPath's own result with
// "-bare" appended, never an independently-derived literal.
// This is the drift BareSiblingPath exists to prevent between production geometry and the on-disk
// shape test fixtures must reproduce for the same input.
func TestBareSiblingPath_AgreesWithSiblingPath(t *testing.T) {
	container, base := "/hub", "loomyard"
	sibling := weftname.SiblingPath(container, base)
	bare := weftname.BareSiblingPath(container, base)
	if want := sibling + "-bare"; bare != want {
		t.Errorf("BareSiblingPath(%q, %q) = %q; want SiblingPath+\"-bare\" = %q", container, base, bare, want)
	}
}
