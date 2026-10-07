// io_test.go drives resolveLivePaneID directly against fixture ReedState values: unknown guid,
// hidden anchor, and empty-PaneID rejections, plus the happy path.
// SendText, SendKey, AND CapturePane all resolve their target pane through this exact function
// (there is no second lookup path), so this one table pins every pane-transport op's error behavior
// at once.
// It never calls SendText/SendKey/CapturePane themselves — those always make a real tmux round trip
// once resolution succeeds, matching the discipline reconcileApplyPersistLocked's own note
// establishes: hermetic tests exercise the pure lookup, never the live tmux seam.

package reedengine

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestResolveLivePaneID pins the lookup all pane-transport ops share.
func TestResolveLivePaneID(t *testing.T) {
	st := &ReedState{Strands: []Strand{
		{GUID: "live", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "hidden", Display: render.Display{Anchor: render.AnchorHidden}},
		{GUID: "unbound", Display: render.Display{Anchor: render.AnchorBelowParent}},
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

// TestSetWaitMark pins the exact tmux argument lists of a set and a clear, and the unknown-guid refusal.
func TestSetWaitMark(t *testing.T) {
	start := time.Unix(1787000000, 0)

	tests := []struct {
		name    string
		guid    string
		label   string
		want    [][]string
		wantErr string
	}{
		{
			name:  "a label sets the label and the start in epoch seconds",
			guid:  "a",
			label: "verify loom",
			want: [][]string{
				{"set-option", "-p", "-t", "%7", "@lyx_wait", "verify loom"},
				{"set-option", "-p", "-t", "%7", "@lyx_wait_start", "1787000000"},
			},
		},
		{
			name: "an empty label unsets both options",
			guid: "a",
			want: [][]string{
				{"set-option", "-p", "-u", "-t", "%7", "@lyx_wait"},
				{"set-option", "-p", "-u", "-t", "%7", "@lyx_wait_start"},
			},
		},
		{
			name:    "an unknown guid is an error and sets nothing",
			guid:    "missing",
			label:   "verify loom",
			wantErr: `unknown strand "missing"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installIfAbsentTmux(t, e, "%7 0 0 100 20 4321\n")
			st := &ReedState{Strands: []Strand{{GUID: "a", PaneID: "%7", Display: render.Display{Anchor: render.AnchorBelowParent}}}}
			if err := SaveState(e.stateDir(), st); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			err := e.SetWaitMark(tt.guid, tt.label, start)
			got := fake.ArgvFor("set-option")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("SetWaitMark error = %v; want it to contain %q", err, tt.wantErr)
				}
				if len(got) != 0 {
					t.Errorf("set-option calls = %v; want none", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetWaitMark: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("set-option calls = %v; want %v", got, tt.want)
			}
		})
	}
}
