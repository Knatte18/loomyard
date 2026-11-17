// slackbridge_test.go pins which orchs the Slack bridge lets post to another machine and the refusal of each other orch.

package creelcli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSlackBridge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const short = "ly"
	tests := []struct {
		name  string
		orch  string
		beams []string
		// buttonOn turns the orch's slack button on.
		buttonOn bool
		appToken string
		setup    bool
		wantErr  string
	}{
		{"PrimeOrchWithButtonTokensAndSetupIsReady", "ly:orch", nil, true, "xapp-1", true, ""},
		{"BeamOrchWithButtonTokensAndSetupIsReady", "ly:pay:orch", []string{"pay"}, true, "xapp-1", true, ""},
		{"PairsOrchIsRefusedNamingTheHubsOrch", "ly:zz:orch", []string{"pay"}, true, "xapp-1", true, "ask the hub's orch or a beam's orch to post it"},
		{"ButtonOffIsRefusedNamingTheStatusBar", "ly:orch", nil, false, "xapp-1", true, "press the button on its status bar"},
		{"MissingAppTokenIsRefusedNamingLogin", "ly:orch", nil, true, "", true, "lym slack login"},
		{"MissingSetupIsRefusedNamingSetup", "ly:orch", nil, true, "xapp-1", false, "lym slack setup --operator <user-id>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			geom := slackengine.Geometry{
				Shortname:      short,
				TokenPath:      filepath.Join(dir, "slack.json"),
				SetupPath:      filepath.Join(dir, "setup.json"),
				ForwardingPath: filepath.Join(dir, "forwarding.json"),
			}
			if err := slackengine.WriteCredentials(geom.TokenPath, slackengine.Credentials{BotToken: "xoxb-1", AppToken: tt.appToken, AppID: "A1"}); err != nil {
				t.Fatalf("WriteCredentials() = %v", err)
			}
			if tt.setup {
				if err := slackengine.WriteSetup(geom.SetupPath, slackengine.Setup{OperatorID: "U1"}); err != nil {
					t.Fatalf("WriteSetup() = %v", err)
				}
			}
			if _, err := slackengine.WriteForwarding(geom.ForwardingPath, tt.orch, tt.buttonOn, time.Unix(0, 0)); err != nil {
				t.Fatalf("WriteForwarding() = %v", err)
			}
			bridge := &slackBridge{
				geometry: func() (slackengine.Geometry, error) { return geom, nil },
				beams:    func() ([]string, error) { return tt.beams, nil },
				names:    func() (string, []string, error) { return "neve", []string{"old"}, nil },
			}

			err := bridge.SendReady(tt.orch)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("SendReady(%q) = %v; want nil", tt.orch, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("SendReady(%q) = %v; want an error containing %q", tt.orch, err, tt.wantErr)
			}
			if current, former, err := bridge.Names(); err != nil || current != "neve" || len(former) != 1 || former[0] != "old" {
				t.Errorf("Names() = %q, %v, %v; want neve with the former name old", current, former, err)
			}
		})
	}

	t.Run("AFailedGeometryReadIsReturnedAsItIs", func(t *testing.T) {
		t.Parallel()
		failure := errors.New("no geometry")
		bridge := &slackBridge{geometry: func() (slackengine.Geometry, error) { return slackengine.Geometry{}, failure }}
		if err := bridge.SendReady("ly:orch"); !errors.Is(err, failure) {
			t.Errorf("SendReady() = %v; want the geometry's own error", err)
		}
	})
}
