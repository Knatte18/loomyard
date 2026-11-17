// remotecontrol_test.go covers remotecontrol.go's RC pane mark.
// It reads the clicking client's active pane and sets the mark.
// Every case is driven through TmuxCmd's execHook seam, with no live server.

package reedengine

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestClientAgentPaneVia pins the format asked of tmux and how its three tab-separated fields become an AgentPane.
func TestClientAgentPaneVia(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	wantArgv := []string{"display-message", "-p", "-c", "/dev/pts/3", "#{pane_id}\t#{@lym_strand_name}\t#{@lym_rc}"}
	tests := []struct {
		name    string
		out     string
		tmuxErr error
		want    AgentPane
		wantErr string
	}{
		{name: "AgentPaneWithTheMark", out: "%7\tscout\t1\n", want: AgentPane{PaneID: "%7", StrandName: "scout", RemoteControlOn: true}},
		{name: "AgentPaneWithoutTheMark", out: "%7\tscout\t\n", want: AgentPane{PaneID: "%7", StrandName: "scout"}},
		{name: "PaneWithNoStrandName", out: "%2\t\t\n", want: AgentPane{PaneID: "%2"}},
		{name: "TmuxError", tmuxErr: errors.New("no such client"), wantErr: `read the active pane of client "/dev/pts/3"`},
		{name: "TooFewFields", out: "%7\tscout\n", wantErr: "shows no active pane"},
		{name: "EmptyPaneID", out: "\tscout\t1\n", wantErr: "shows no active pane"},
		{name: "EmptyOutput", out: "", wantErr: "shows no active pane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newTmuxCmdForSocketPath("tmux", "/tmp/sock")
			fake := installFakeTmuxOn(t, &cmd)
			fake.answer("display-message", tt.out, tt.tmuxErr)

			got, err := clientAgentPaneVia(cmd, "/dev/pts/3")

			if calls := fake.Calls(); len(calls) != 1 || !slices.Equal(calls[0], wantArgv) {
				t.Errorf("tmux calls = %q; want one call %q", calls, wantArgv)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("clientAgentPaneVia error = %v; want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("clientAgentPaneVia: %v", err)
			}
			if got != tt.want {
				t.Errorf("clientAgentPaneVia = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestMarkPaneRemoteControlOnVia pins that a tmux failure is returned naming the pane.
// The set-option call itself is pinned through the public engine op in TestMarkRemoteControlOn.
func TestMarkPaneRemoteControlOnVia(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cmd := newTmuxCmdForSocketPath("tmux", "/tmp/sock")
	fake := installFakeTmuxOn(t, &cmd)
	fake.answer("set-option", "", errors.New("can't find pane"))

	err := markPaneRemoteControlOnVia(cmd, "%7")

	if err == nil || !strings.Contains(err.Error(), "set the RC mark on pane %7") {
		t.Errorf("markPaneRemoteControlOnVia error = %v; want it to name the pane", err)
	}
}

// TestMarkRemoteControlOn pins that the mark lands on the strand's own pane, and that an unknown guid sets nothing.
func TestMarkRemoteControlOn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		guid    string
		want    [][]string
		wantErr string
	}{
		{name: "MarksTheStrandsPane", guid: "a", want: [][]string{{"set-option", "-p", "-t", "%7", "@lym_rc", "1"}}},
		{name: "AnUnknownGUIDIsAnErrorAndSetsNothing", guid: "missing", wantErr: `unknown strand "missing"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEngine(t)
			fake := installIfAbsentTmux(t, e, "%7 0 0 100 20 4321\n")
			st := &ReedState{Strands: []Strand{{GUID: "a", PaneID: "%7", Display: render.Display{Anchor: render.AnchorOwnWindow}}}}
			if err := SaveState(e.stateDir(), st); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			err := e.MarkRemoteControlOn(tt.guid)

			got := fake.ArgvFor("set-option")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarkRemoteControlOn error = %v; want it to contain %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("MarkRemoteControlOn: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("set-option calls = %v; want %v", got, tt.want)
			}
		})
	}
}
