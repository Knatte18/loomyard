// io_test.go drives resolveLivePaneID directly against fixture MuxState
// values: unknown guid, hidden anchor, and empty-PaneID rejections, plus the
// happy path. It never calls SendText/SendKey/CapturePane themselves — those
// always make a real psmux round trip once resolution succeeds, matching the
// discipline reconcileApplyPersistLocked's own note establishes: hermetic
// tests exercise the pure lookup, never the live psmux seam.

package muxengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func TestResolveLivePaneID(t *testing.T) {
	st := &MuxState{Strands: []Strand{
		{GUID: "live", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "hidden", Display: render.Display{Anchor: render.AnchorHidden}},
		{GUID: "unbound", Display: render.Display{Anchor: render.AnchorTop}},
	}}

	tests := []struct {
		name       string
		guid       string
		wantPaneID string
		wantErr    bool
	}{
		{"UnknownGuidErrors", "does-not-exist", "", true},
		{"HiddenAnchorErrors", "hidden", "", true},
		{"EmptyPaneIDErrors", "unbound", "", true},
		{"LivePaneResolves", "live", "%1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLivePaneID(st, tt.guid)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveLivePaneID(%q) = nil error, want error", tt.guid)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLivePaneID(%q): %v", tt.guid, err)
			}
			if got != tt.wantPaneID {
				t.Errorf("resolveLivePaneID(%q) = %q, want %q", tt.guid, got, tt.wantPaneID)
			}
		})
	}
}
