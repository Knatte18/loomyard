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
	"errors"
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

// TestPasteSubmitArgv pins the exact command list of a paste with and without the trailing Enter.
func TestPasteSubmitArgv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		submit bool
		want   []string
	}{
		{
			name: "paste with Enter", submit: true,
			want: []string{
				"load-buffer", "-b", "buf", "-",
				";", "paste-buffer", "-p", "-d", "-b", "buf", "-t", "%7",
				";", "send-keys", "-t", "%7", "Enter",
			},
		},
		{
			name: "paste without Enter", submit: false,
			want: []string{
				"load-buffer", "-b", "buf", "-",
				";", "paste-buffer", "-p", "-d", "-b", "buf", "-t", "%7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pasteSubmitArgv("%7", "buf", tt.submit); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pasteSubmitArgv = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestPasteSubmit pins that a write is one tmux invocation with no send-keys -l, that an unresolvable strand fails before any invocation, and that a failed invocation deletes its named buffer.
func TestPasteSubmit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		guid string
		// pasteErr is what the paste invocation fails with, nil for success.
		pasteErr error
		wantErr  string
	}{
		{name: "a live strand is pasted in one invocation", guid: "a"},
		{name: "an unknown strand fails before any invocation", guid: "missing", wantErr: `unknown strand "missing"`},
		{name: "a hidden strand fails before any invocation", guid: "hidden", wantErr: "is hidden"},
		{name: "a failed paste deletes its buffer", guid: "a", pasteErr: errors.New("can't find pane: %7"), wantErr: "paste text: can't find pane: %7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newTestEngine(t)
			fake := installIfAbsentTmux(t, e, "%7 0 0 100 20 4321\n")
			st := &ReedState{Strands: []Strand{
				{GUID: "a", PaneID: "%7", Display: render.Display{Anchor: render.AnchorBelowParent}},
				{GUID: "hidden", Display: render.Display{Anchor: render.AnchorHidden}},
			}}
			if err := SaveState(e.stateDir(), st); err != nil {
				t.Fatalf("SaveState: %v", err)
			}
			if tt.pasteErr != nil {
				fake.answer("load-buffer", "", tt.pasteErr)
			}

			err := e.PasteSubmit(tt.guid, "hello", true)
			pastes := fake.ArgvFor("load-buffer")
			deletes := fake.ArgvFor("delete-buffer")
			if tt.pasteErr != nil {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("PasteSubmit error = %v; want %q", err, tt.wantErr)
				}
				if len(pastes) != 1 || len(deletes) != 1 || !reflect.DeepEqual(deletes[0], []string{"delete-buffer", "-b", pastes[0][2]}) {
					t.Errorf("delete-buffer calls = %v after paste %v; want one deleting the paste's buffer", deletes, pastes)
				}
				return
			}
			if len(deletes) != 0 {
				t.Errorf("delete-buffer calls = %v; want none, the paste deletes its own buffer", deletes)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("PasteSubmit error = %v; want it to contain %q", err, tt.wantErr)
				}
				if len(pastes) != 0 {
					t.Errorf("load-buffer calls = %v; want none", pastes)
				}
				return
			}
			if err != nil {
				t.Fatalf("PasteSubmit: %v", err)
			}
			if len(pastes) != 1 {
				t.Fatalf("load-buffer invocations = %d; want 1", len(pastes))
			}
			if got := fake.StdinFor("load-buffer"); !reflect.DeepEqual(got, []string{"hello"}) {
				t.Errorf("load-buffer stdin = %q; want the text alone", got)
			}
			if !strings.HasPrefix(pastes[0][2], "lyx-inject-") {
				t.Errorf("buffer name = %q; want the lyx-inject- prefix", pastes[0][2])
			}
			if sends := fake.ArgvFor("send-keys"); len(sends) != 0 {
				t.Errorf("separate send-keys calls = %v; want none, the Enter rides the paste invocation", sends)
			}
		})
	}
}

// TestActiveClientOnPane pins which client lines count as an active operator, and the quiet-window floor of a hand-built Config.
// The floor sits here rather than in config_test.go because that file is an external test package and cannot reach operatorQuiet.
func TestActiveClientOnPane(t *testing.T) {
	t.Parallel()

	now := time.Unix(1787000100, 0)
	quiet := 5 * time.Second

	tests := []struct {
		name    string
		clients string
		want    bool
	}{
		{"a client on the pane active 1 s ago is active", "%7 1787000099\n", true},
		{"a client on the pane active 6 s ago is not", "%7 1787000094\n", false},
		{"a client on another pane is not", "%8 1787000099\n", false},
		{"an empty list is not", "", false},
		{"a malformed line is skipped", "garbage\n%7 notanumber\n%7 1787000099\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activeClientOnPane(tt.clients, "%7", now, quiet); got != tt.want {
				t.Errorf("activeClientOnPane(%q) = %v; want %v", tt.clients, got, tt.want)
			}
		})
	}

	for _, seconds := range []int{0, -1} {
		if got := operatorQuiet(Config{OperatorQuietS: seconds}); got != 5*time.Second {
			t.Errorf("operatorQuiet(%d) = %v; want 5s", seconds, got)
		}
	}
	if got := operatorQuiet(Config{OperatorQuietS: 9}); got != 9*time.Second {
		t.Errorf("operatorQuiet(9) = %v; want 9s", got)
	}
}

// TestSetWaitMark pins the exact tmux argument lists of a set and a clear, and the unknown-guid refusal.
func TestSetWaitMark(t *testing.T) {
	t.Parallel()

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
