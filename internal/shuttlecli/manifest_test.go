// manifest_test.go covers the `manifest` verb over a stubbed manifest report.

package shuttlecli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleparity"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestManifestReportOf_RefusesAHubWithNoShortname checks the refusal of a hub that records no shortname.
// The refusal names the way forward.
func TestManifestReportOf_RefusesAHubWithNoShortname(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	layout := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."}
	_, err := manifestReportOf(layout, shuttleengine.Config{}, configengine.Dirs{Anchor: t.TempDir()}, "darn")
	if err == nil || !strings.Contains(err.Error(), "lym fabric shortname") {
		t.Errorf("manifestReportOf() error = %v; want a refusal naming `lym fabric shortname`", err)
	}
}

// TestManifestCmd runs the verb over a manifestReport stub that answers each seat name its own report or error.
// It builds shuttleCLI by hand, so the verb runs no resolution of its own.
func TestManifestCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	both := func() map[string]shuttleengine.Manifest {
		manifest := shuttleengine.Manifest{Items: []shuttleengine.ManifestItem{{Channel: shuttleengine.ChannelStandingPassage, Key: "agents-prompt", Delivery: shuttleengine.DeliveryDelivered}}}
		return map[string]shuttleengine.Manifest{shuttleparity.ReferenceEngine: manifest, shuttleparity.ComparedEngine: manifest}
	}
	lacking := shuttleengine.ManifestItem{Channel: shuttleengine.ChannelGuard, Key: "shell-deny", Delivery: shuttleengine.DeliveryDelivered}
	stubError := errors.New("no seat is named other")
	reports := map[string]func() (shuttleparity.ManifestReport, error){
		"matching": func() (shuttleparity.ManifestReport, error) {
			return shuttleparity.ManifestReport{Seat: "matching", Manifests: both()}, nil
		},
		"missing": func() (shuttleparity.ManifestReport, error) {
			return shuttleparity.ManifestReport{Seat: "missing", Manifests: both(), Missing: []shuttleengine.ManifestItem{lacking}}, nil
		},
		"unrun": func() (shuttleparity.ManifestReport, error) {
			omitted := shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPluginContext, Key: "p@m/emit", Delivery: shuttleengine.DeliveryOmitted, Reason: "not run"}
			manifest := shuttleengine.Manifest{Items: []shuttleengine.ManifestItem{omitted}, Verdict: []shuttleengine.ManifestItem{omitted}}
			return shuttleparity.ManifestReport{
				Seat:      "unrun",
				Manifests: map[string]shuttleengine.Manifest{shuttleparity.ReferenceEngine: {}, shuttleparity.ComparedEngine: manifest},
			}, nil
		},
		"unmet": func() (shuttleparity.ManifestReport, error) {
			return shuttleparity.ManifestReport{
				Seat: "unmet", Manifests: both(),
				Unmet: map[string][]shuttleengine.Requirement{shuttleparity.ComparedEngine: {shuttleengine.RequirementForks}},
			}, nil
		},
		"broken": func() (shuttleparity.ManifestReport, error) { return shuttleparity.ManifestReport{}, stubError },
	}

	tests := []struct {
		name     string
		seat     string
		wantExit int
		// wantOutput are the parts the printed envelope holds.
		wantOutput []string
		// wantAbsent are the parts it does not hold.
		wantAbsent []string
	}{
		{name: "MatchingSeatPrintsOkWithBothManifests", seat: "matching", wantExit: 0, wantOutput: []string{`"ok":true`, `"` + shuttleparity.ReferenceEngine + `":`, `"` + shuttleparity.ComparedEngine + `":`, `"missing":[]`}},
		{name: "MissingCodexItemFailsNamingChannelAndKey", seat: "missing", wantExit: 1, wantOutput: []string{`"ok":false`, "guard shell-deny", "way forward", `"missing":["guard shell-deny"]`}},
		{
			name: "OmittedPluginContextIsListedUnrunWithNoVerdict", seat: "unrun", wantExit: 0,
			wantOutput: []string{`"ok":true`, `"unrun":{"` + shuttleparity.ComparedEngine + `":["plugin-context p@m/emit"]}`, `"verdict":null`},
			wantAbsent: []string{`"verdict":[`},
		},
		{name: "UnmetRequirementNamesTheEngineAndPasses", seat: "unmet", wantExit: 0, wantOutput: []string{`"ok":true`, `"unmet":{"` + shuttleparity.ComparedEngine + `":["` + string(shuttleengine.RequirementForks) + `"]}`}},
		{name: "ReportErrorIsPrintedAndFails", seat: "broken", wantExit: 1, wantOutput: []string{`"ok":false`, stubError.Error()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &shuttleCLI{manifestReport: func(name string) (shuttleparity.ManifestReport, error) { return reports[name]() }}
			var out bytes.Buffer
			exit := clihelp.Execute(c.manifestCmd(), &out, []string{tt.seat})
			if exit != tt.wantExit {
				t.Errorf("Execute(manifest %s) = %d; want %d; output: %s", tt.seat, exit, tt.wantExit, out.String())
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("Execute(manifest %s) output %q does not contain %q", tt.seat, out.String(), want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("Execute(manifest %s) output %q contains %q; want it absent", tt.seat, out.String(), absent)
				}
			}
		})
	}
}
