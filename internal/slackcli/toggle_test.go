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

	const (
		slackOn  = reedengine.SlackToggleLit
		slackOff = reedengine.SlackToggleUnlit
		awayOn   = reedengine.AwayToggleLit
	)
	tests := []struct {
		name            string
		dir             string
		locationUnknown bool
		wireErr         error
		orch            string
		orchErr         error
		onOrch          string
		presence        slackengine.Presence
		corruptPresence bool
		machineErr      error
		want            string
	}{
		{name: "BothLitForAnOrchWhoseButtonIsOnWhileAway", dir: "/wt", orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, want: slackOn + " " + awayOn + "\n"},
		{name: "OnlySlackLitForAnOrchWhoseButtonIsOnAtTheTerminal", dir: "/wt", orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceTerminal, want: slackOn + " " + awayToggleOff + "\n"},
		{name: "OnlyAwayLitForAnOrchWhoseButtonIsOff", dir: "/wt", orch: "hub:orch", onOrch: "hub:b:orch",
			presence: slackengine.PresenceSlack, want: slackOff + " " + awayOn + "\n"},
		{name: "NeitherLitWithNoButtonOnAndNoPresence", dir: "/wt", orch: "hub:orch",
			want: slackOff + " " + awayToggleOff + "\n"},
		{name: "ABeamOrchReadsItsOwnButton", dir: "/wt", orch: "hub:b:orch", onOrch: "hub:b:orch",
			presence: slackengine.PresenceTerminal, want: slackOn + " " + awayToggleOff + "\n"},
		{name: "AWorktreeWithNoOrchButtonDrawsOnlyTheAwayWord", dir: "/wt", orch: "", onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, want: awayOn + "\n"},
		{name: "NoDirDrawsOnlyTheAwayWord", orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, want: awayOn + "\n"},
		{name: "ADirThatIsNoWorktreeDrawsOnlyTheAwayWord", dir: "/wt", locationUnknown: true, orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceTerminal, want: awayToggleOff + "\n"},
		{name: "AFailedWiringDrawsOnlyTheAwayWord", dir: "/wt", wireErr: errors.New("no hub"), orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, want: awayOn + "\n"},
		{name: "AFailedOrchLookupDrawsOnlyTheAwayWord", dir: "/wt", orchErr: errors.New("beams unreadable"), onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, want: awayOn + "\n"},
		{name: "UnreadablePresenceReadsAsOff", dir: "/wt", orch: "hub:orch", onOrch: "hub:orch", corruptPresence: true,
			want: slackOn + " " + awayToggleOff + "\n"},
		{name: "UnreadableMachinePathsReadAwayOff", dir: "/wt", orch: "hub:orch", onOrch: "hub:orch",
			presence: slackengine.PresenceSlack, machineErr: errors.New("no state directory"),
			want: slackOn + " " + awayToggleOff + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.locationOK = !tt.locationUnknown
			h.wireErr, h.orch, h.orchErr, h.machineErr = tt.wireErr, tt.orch, tt.orchErr, tt.machineErr
			if tt.onOrch != "" {
				h.storeForwarding(tt.onOrch)
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

			code, out := h.runRaw("", "bar", "--dir", tt.dir)

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
		name         string
		toggle       string
		orch         string
		on           []string
		presence     slackengine.Presence
		wantCalls    []string
		wantOn       []string
		wantPresence slackengine.Presence
		wantFields   map[string]any
	}{
		{"SlackWithTheButtonOffSwitchesItOnAndStartsTheRelay", toggleSlack, "hub:orch", nil, slackengine.PresenceTerminal,
			[]string{"ensure"}, []string{"hub:orch"}, slackengine.PresenceTerminal,
			map[string]any{"orch": "hub:orch", "forwarding": "on", "relay": started}},
		{"SlackOnLeavesThePresenceAlone", toggleSlack, "hub:orch", nil, slackengine.PresenceSlack,
			[]string{"ensure"}, []string{"hub:orch"}, slackengine.PresenceSlack,
			map[string]any{"orch": "hub:orch", "forwarding": "on", "relay": started}},
		{"SlackOnForABeamKeepsAnotherOrchsButton", toggleSlack, "hub:b:orch", []string{"hub:orch"}, slackengine.PresenceTerminal,
			[]string{"ensure"}, []string{"hub:b:orch", "hub:orch"}, slackengine.PresenceTerminal,
			map[string]any{"orch": "hub:b:orch", "forwarding": "on", "relay": started}},
		{"SlackWithTheLastButtonOnSwitchesOffAndEndsTheRelay", toggleSlack, "hub:orch", []string{"hub:orch"}, slackengine.PresenceSlack,
			[]string{"stop"}, nil, slackengine.PresenceSlack,
			map[string]any{"orch": "hub:orch", "forwarding": "off", "relay_ended": float64(1)}},
		{"SlackOffLeavesTheRelayWhileAnotherOrchsButtonIsOn", toggleSlack, "hub:orch", []string{"hub:orch", "hub:b:orch"}, slackengine.PresenceSlack,
			nil, []string{"hub:b:orch"}, slackengine.PresenceSlack,
			map[string]any{"orch": "hub:orch", "forwarding": "off"}},
		{"AwayWhileSlackSetsTerminalAsBackDoes", toggleAway, "hub:orch", nil, slackengine.PresenceSlack,
			nil, nil, slackengine.PresenceTerminal, map[string]any{"presence": "terminal", "changed": true}},
		{"AwayWhileTerminalGoesAwayAsAwayDoes", toggleAway, "hub:orch", nil, slackengine.PresenceTerminal,
			[]string{"ensure", "await"}, nil, slackengine.PresenceSlack,
			map[string]any{"presence": "slack", "changed": true, "relay": started}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.endedPerStop = 1
			h.orch = tt.orch
			h.storePresence(tt.presence)
			h.storeForwarding(tt.on...)

			code, env := h.run("", toggleArgs(tt.toggle)...)

			wantOK(t, code, env)
			h.wantCalls(tt.wantCalls...)
			if diff := cmp.Diff(tt.wantOn, h.forwarding(), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("buttons on mismatch (-want +got):\n%s", diff)
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

func TestToggleSlackRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		unconfigured bool
		orch         string
		orchErr      error
		on           []string
		ensureErr    error
		stopErr      error
		wantRefusal  []string
		wantOn       []string
		wantCalls    []string
	}{
		{name: "APairsWorktreeHasNoButton", orch: "",
			wantRefusal: []string{"a pair's orch has none", "way forward"}},
		{name: "AFailedOrchLookupIsRefused", orch: "hub:orch", orchErr: errors.New("beams unreadable"),
			wantRefusal: []string{"beams unreadable"}},
		{name: "SwitchingOnNeedsLoginAndSetup", unconfigured: true, orch: "hub:orch",
			wantRefusal: []string{"lym slack login"}},
		{name: "ARelayThatWillNotStartKeepsTheButtonOnAndNamesStart", orch: "hub:orch", ensureErr: errors.New("lock held"),
			wantRefusal: []string{"button of hub:orch is on", "relay did not start", "lock held", "lym slack start"},
			wantOn:      []string{"hub:orch"}, wantCalls: []string{"ensure"}},
		{name: "ARelayThatWillNotEndKeepsTheButtonOffAndNamesStop", orch: "hub:orch", on: []string{"hub:orch"},
			stopErr:     errors.New("a process would not end"),
			wantRefusal: []string{"button of hub:orch is off", "relay was not ended", "a process would not end", "lym slack stop"},
			wantCalls:   []string{"stop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			if !tt.unconfigured {
				h.configure()
			}
			h.orch, h.orchErr, h.ensureErr, h.stopErr = tt.orch, tt.orchErr, tt.ensureErr, tt.stopErr
			h.storeForwarding(tt.on...)

			code, env := h.run("", toggleArgs(toggleSlack)...)

			wantRefused(t, code, env, tt.wantRefusal...)
			h.wantCalls(tt.wantCalls...)
			if diff := cmp.Diff(tt.wantOn, h.forwarding(), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("buttons on mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantRedraw("lym: "+env.Error), h.redraws, cmp.AllowUnexported(toggleRedraw{})); diff != "" {
				t.Errorf("redraws mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
