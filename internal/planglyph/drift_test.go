// drift_test.go covers DetectDrift's gate one over the full-vs-pending plan split, with a
// hand-built delta: the gates decide before any quarry or git call, so these tests stay in the
// untagged tier — the delta-building half lives in drift_integration_test.go.

package planglyph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// TestDetectDrift_GateOneSeesTheRecordingBatchOwnPair is F6's (round fable5-high-r3) regression
// test: the card whose rename the delta reports is exactly the one record-batch excludes from the
// pending view, so gate one must read the FULL plan's declared pairs — against pre-fix source
// (pair index built over pending) the declared rename fell through to the repair path and was
// rewritten plan-wide as exact-tier drift, amendment and all.
func TestDetectDrift_GateOneSeesTheRecordingBatchOwnPair(t *testing.T) {
	dir, fullPlan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Foo` -> `plan:sub#Bar`\n\n**Intent:** rename Foo\n",
		2: "**Edit:**\n- `sub#Foo`\n\n**Intent:** two\n\n**ImpactSummary:** none\n",
	})
	pending := PendingPlan(fullPlan, []planparser.Card{fullPlan.Cards[0]})

	delta := quarry.GitDeltaAnswer{}
	delta.Renamed = []quarry.RenamedPair{{
		From: quarry.Symbol{ID: "sub#Foo"},
		To:   quarry.Symbol{ID: "sub#Bar"},
	}}

	before := readCardFile(t, dir, 2, "card2")
	findings, err := DetectDrift(fullPlan, pending, dir, t.TempDir(), delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("DetectDrift(declared rename) = %+v; want no findings — the delta matches the declaring card's own pair", findings)
	}
	if got := readCardFile(t, dir, 2, "card2"); got != before {
		t.Errorf("card 2 was rewritten for a declared rename:\nbefore: %q\nafter:  %q", before, got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); !os.IsNotExist(statErr) {
		t.Errorf("amendments file exists after a declared rename; want none — gate one repairs nothing")
	}
}
