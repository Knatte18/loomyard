// toggle_test.go covers the hidden bar and toggle verbs behind reed's slack and away toggles.

package slackcli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBar(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name            string
		held            bool
		presence        slackengine.Presence
		corruptPresence bool
		machineErr      error
		want            string
	}{
		{"BothLitWhileConnectedAndAway", true, slackengine.PresenceSlack, false, nil,
			reedengine.SlackToggleLit + " " + reedengine.AwayToggleLit + "\n"},
		{"OnlySlackLitWhileConnectedAtTheTerminal", true, slackengine.PresenceTerminal, false, nil,
			reedengine.SlackToggleLit + " " + awayToggleOff + "\n"},
		{"OnlyAwayLitWhileTheSocketLockIsFree", false, slackengine.PresenceSlack, false, nil,
			slackToggleOff + " " + reedengine.AwayToggleLit + "\n"},
		{"NeitherLitWithNoPresenceAndNoLock", false, "", false, nil, slackToggleOff + " " + awayToggleOff + "\n"},
		{"UnreadablePresenceReadsAsOff", true, "", true, nil, reedengine.SlackToggleLit + " " + awayToggleOff + "\n"},
		{"UnreadableMachinePathsReadBothOff", true, slackengine.PresenceSlack, false, errors.New("no state directory"),
			slackToggleOff + " " + awayToggleOff + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			if tt.held {
				h.holdSocketLock()
			}
			if tt.presence != "" {
				h.storePresence(tt.presence)
			}
			if tt.corruptPresence {
				if err := os.MkdirAll(filepath.Dir(h.geom.PresencePath), 0o700); err != nil {
					t.Fatalf("MkdirAll = %v", err)
				}
				if err := os.WriteFile(h.geom.PresencePath, []byte(`{"presence":"bogus"}`), 0o600); err != nil {
					t.Fatalf("WriteFile = %v", err)
				}
			}
			h.machineErr = tt.machineErr

			code, out := h.runRaw("", "bar")

			if code != 0 || out != tt.want {
				t.Errorf("bar = exit %d printing %q; want exit 0 printing %q", code, out, tt.want)
			}
		})
	}
}

// toggleArgs is the argument list of a click on the named toggle, with the redraw target wantRedraw names.
func toggleArgs(name string) []string {
	return []string{"toggle", name, "--tmux", "/bin/tmux", "--socket", "/tmp/reed.sock", "--client", "client-1"}
}

// wantRedraw is the redraw a toggle run asks for, showing message.
func wantRedraw(message string) []toggleRedraw {
	return []toggleRedraw{{tmuxPath: "/bin/tmux", socketPath: "/tmp/reed.sock", client: "client-1", message: message}}
}

func TestToggle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	started := string(slackengine.OutcomeStarted)
	tests := []struct {
		name          string
		toggle        string
		connected     bool
		switchOff     bool
		presence      slackengine.Presence
		wantCalls     []string
		wantSwitchOff bool
		wantPresence  slackengine.Presence
		wantFields    map[string]any
	}{
		{"SlackWhileConnectedSwitchesOffAndEndsTheRelayLeavingThePresence", toggleSlack, true, false, slackengine.PresenceSlack,
			[]string{"stop"}, true, slackengine.PresenceSlack, map[string]any{"connection": "off", "relays_ended": float64(1)}},
		{"SlackWhileNotConnectedSwitchesOnAndStartsTheRelay", toggleSlack, false, true, slackengine.PresenceTerminal,
			[]string{"ensure"}, false, slackengine.PresenceTerminal, map[string]any{"relay": started}},
		{"AwayWhileSlackSetsTerminalAsBackDoes", toggleAway, false, false, slackengine.PresenceSlack,
			nil, false, slackengine.PresenceTerminal, map[string]any{"presence": "terminal", "changed": true}},
		{"AwayWhileTerminalGoesAwayAsAwayDoes", toggleAway, false, true, slackengine.PresenceTerminal,
			[]string{"ensure", "await"}, false, slackengine.PresenceSlack, map[string]any{"presence": "slack", "changed": true, "relay": started}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.endedPerStop = 1
			h.storePresence(tt.presence)
			h.storeSwitch(tt.switchOff)
			if tt.connected {
				h.holdSocketLock()
			}

			code, env := h.run("", toggleArgs(tt.toggle)...)

			wantOK(t, code, env)
			h.wantCalls(tt.wantCalls...)
			if got := h.switchedOff(); got != tt.wantSwitchOff {
				t.Errorf("connection switched off = %v; want %v", got, tt.wantSwitchOff)
			}
			if got := h.presence(); got != tt.wantPresence {
				t.Errorf("presence = %q; want %q", got, tt.wantPresence)
			}
			if diff := cmp.Diff(tt.wantFields, env.Raw); diff != "" {
				t.Errorf("toggle fields mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantRedraw(""), h.redraws, cmp.AllowUnexported(toggleRedraw{})); diff != "" {
				t.Errorf("redraws mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("AnUnknownToggleIsRefusedAndTheRedrawShowsTheRefusal", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil)
		h.configure()

		code, env := h.run("", toggleArgs("bogus")...)

		wantRefused(t, code, env, `no toggle "bogus"`, "way forward")
		h.wantCalls()
		if diff := cmp.Diff(wantRedraw("lym: "+env.Error), h.redraws, cmp.AllowUnexported(toggleRedraw{})); diff != "" {
			t.Errorf("redraws mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestToggleSlackSwitchOff(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		others       []string
		liveHubsErr  error
		stopErrs     map[string]error
		wantStopped  []string
		wantRefusal  []string
		wantRelaysOf float64
	}{
		{name: "EndsThisHubsRelayOnceThenEveryOtherLiveHubs", others: []string{"/hubs/a", "/hubs/b"},
			wantStopped: []string{"@own", "/hubs/a", "/hubs/b"}, wantRelaysOf: 3},
		{name: "AFailedStopInOneHubStillEndsTheOthersAndNamesTheFailure", others: []string{"/hubs/a", "/hubs/b"},
			stopErrs: map[string]error{"/hubs/a": errors.New("relay would not end")}, wantStopped: []string{"@own", "/hubs/a", "/hubs/b"},
			wantRefusal: []string{"switched off", "/hubs/a", "relay would not end", "lym slack stop"}},
		{name: "UnreadableLiveHubsAreRefusedAfterTheSwitchIsOff", liveHubsErr: errors.New("hubs record unreadable"),
			wantRefusal: []string{"switched off", "live hubs could not be read", "hubs record unreadable", "lym slack stop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.endedPerStop = 1
			h.holdSocketLock()
			h.liveHubs = []slackengine.Geometry{{HubPath: h.geom.HubPath}}
			for _, other := range tt.others {
				h.liveHubs = append(h.liveHubs, slackengine.Geometry{HubPath: other})
			}
			h.liveHubsErr, h.stopErrs = tt.liveHubsErr, tt.stopErrs
			wantStopped := make([]string, len(tt.wantStopped))
			for i, hub := range tt.wantStopped {
				wantStopped[i] = hub
				if hub == "@own" {
					wantStopped[i] = h.geom.HubPath
				}
			}

			code, env := h.run("", toggleArgs(toggleSlack)...)

			if got := h.switchedOff(); !got {
				t.Error("connection switched off = false; want true before any relay ends")
			}
			if diff := cmp.Diff(wantStopped, h.stoppedHubs, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("stopped hubs mismatch (-want +got):\n%s", diff)
			}
			if tt.wantRefusal != nil {
				wantRefused(t, code, env, tt.wantRefusal...)
				if diff := cmp.Diff(wantRedraw("lym: "+env.Error), h.redraws, cmp.AllowUnexported(toggleRedraw{})); diff != "" {
					t.Errorf("redraws mismatch (-want +got):\n%s", diff)
				}
				return
			}
			wantOK(t, code, env)
			if diff := cmp.Diff(map[string]any{"connection": "off", "relays_ended": tt.wantRelaysOf}, env.Raw); diff != "" {
				t.Errorf("toggle fields mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
