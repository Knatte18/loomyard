// manifest_test.go covers the manifest verb's answers over a stubbed report.

package shuttlecli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleparity"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestManifestCmd pins the exit code and envelope of the manifest verb for each kind of report.
func TestManifestCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	manifests := map[string]shuttleengine.Manifest{
		shuttleparity.ReferenceEngine: {Engine: shuttleparity.ReferenceEngine},
		shuttleparity.ComparedEngine:  {Engine: shuttleparity.ComparedEngine},
	}
	reports := map[string]shuttleparity.ManifestReport{
		"matching": {Seat: "matching", Manifests: manifests},
		"missing": {Seat: "missing", Manifests: manifests, Missing: []shuttleengine.ManifestItem{
			{Channel: shuttleengine.ManifestChannelNotice, Key: "deny-notice"},
		}},
		"unmet": {Seat: "unmet", Manifests: manifests, Unmet: map[string][]shuttleengine.Requirement{
			shuttleparity.ComparedEngine: {shuttleengine.RequirementForks},
		}},
	}
	c := &shuttleCLI{manifestReport: func(name string) (shuttleparity.ManifestReport, error) {
		report, ok := reports[name]
		if !ok {
			return shuttleparity.ManifestReport{}, errors.New("no seat is called " + name)
		}
		return report, nil
	}}

	tests := []struct {
		seat     string
		wantExit int
		want     []string
	}{
		{seat: "matching", want: []string{`"ok":true`, `"engine":"` + shuttleparity.ReferenceEngine + `"`, `"engine":"` + shuttleparity.ComparedEngine + `"`, `"missing":[]`}},
		{seat: "missing", wantExit: 1, want: []string{`"ok":false`, "notice deny-notice", "way forward", `"manifests"`}},
		{seat: "unmet", want: []string{`"ok":true`, shuttleparity.ComparedEngine + " cannot start this seat: unmet requirements " + string(shuttleengine.RequirementForks)}},
		{seat: "nobody", wantExit: 1, want: []string{`"ok":false`, "no seat is called nobody"}},
	}
	for _, tt := range tests {
		t.Run(tt.seat, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if got := clihelp.Execute(c.manifestCmd(), &out, []string{tt.seat}); got != tt.wantExit {
				t.Errorf("manifest %s exit = %d; want %d", tt.seat, got, tt.wantExit)
			}
			parseSingleEnvelope(t, out.Bytes())
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("manifest %s output = %s; want it to contain %q", tt.seat, out.String(), want)
				}
			}
		})
	}
}
