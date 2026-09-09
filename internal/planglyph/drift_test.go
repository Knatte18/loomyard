// drift_test.go covers DetectDrift's gate one over the full-vs-pending plan split, with a
// hand-built delta: the gates decide before any quarry or git call, so these tests stay in the
// untagged tier — the delta-building half lives in drift_integration_test.go.

package planglyph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestDetectDrift_GateOneSeesEveryDeclaredDestinationForOneOldSide is R9-4's regression: two cards
// declaring a rename of the SAME old symbol are legal under every plan-format check, and gate one
// must recognize either card's own declared destination.
//
// Against pre-fix source, renameCardPairs was a map[old]new that kept only the LAST pair it walked,
// so the delta reporting the first card's rename fell through gate one, was treated as drift, and
// auto-repaired: a plan-wide RewriteRefs plus an amendment recording a "repair" of the very rename
// the plan had declared.
func TestDetectDrift_GateOneSeesEveryDeclaredDestinationForOneOldSide(t *testing.T) {
	dir, fullPlan := writePlanFixture(t, map[int]string{
		1: "**Rename:**\n- `sub#Foo` -> `plan:sub#Bar`\n\n**Intent:** rename Foo to Bar\n",
		2: "**Rename:**\n- `sub#Foo` -> `plan:sub#Baz`\n\n**Intent:** rename Foo to Baz\n",
		3: "**Edit:**\n- `sub#Foo`\n\n**Intent:** three\n\n**ImpactSummary:** none\n",
	})
	pending := PendingPlan(fullPlan, []planparser.Card{fullPlan.Cards[0]})

	delta := quarry.GitDeltaAnswer{}
	delta.Renamed = []quarry.RenamedPair{{
		From: quarry.Symbol{ID: "sub#Foo"},
		To:   quarry.Symbol{ID: "sub#Bar"},
	}}

	before := readCardFile(t, dir, 3, "card3")
	findings, err := DetectDrift(fullPlan, pending, dir, t.TempDir(), delta, "deadbeef", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DetectDrift(...) returned error: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("DetectDrift(first card's declared rename) = %+v; want no findings", findings)
	}
	if got := readCardFile(t, dir, 3, "card3"); got != before {
		t.Errorf("card 3 was rewritten for a declared rename:\nbefore: %q\nafter:  %q", before, got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, planparser.AmendmentsFileName)); !os.IsNotExist(statErr) {
		t.Errorf("amendments file exists after a declared rename; want none — gate one repairs nothing")
	}
}

// TestEnsurePostRepairCoverage_UnansweredTargetErrors is card 12's regression for the per-key answer
// coverage guard: a target the post-repair batch actually requested, but which the resolve answer
// does not cover, must fail closed with ErrQuarryUnavailable naming the missing target — exactly as
// doneCheckVerdicts' own per-key guard does (donecheck.go) — rather than let the miss pass silently
// through DetectDrift's introduced-glyph filter.
func TestEnsurePostRepairCoverage_UnansweredTargetErrors(t *testing.T) {
	targets := []string{"sub#Bar", "sub#Baz"}
	results := []quarry.ResolveResult{
		{Target: "sub#Bar", Status: quarry.StatusFound},
		// sub#Baz requested but never answered.
	}

	err := ensurePostRepairCoverage(targets, results)
	if err == nil {
		t.Fatal("ensurePostRepairCoverage(unanswered target) = nil error; want an error naming the unanswered target")
	}
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("ensurePostRepairCoverage(unanswered target) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
	}
	if !strings.Contains(err.Error(), "sub#Baz") {
		t.Errorf("ensurePostRepairCoverage(unanswered target) error = %v; want it to name %q", err, "sub#Baz")
	}
}

// TestEnsurePostRepairCoverage_FullCoveragePasses is the guard's positive half: a result set
// covering every requested target returns nil even when the results ALSO carry an answer for a
// glyph outside the requested batch — the introduced-glyph-absent-from-batch case stays a silent
// pass, since the guard's scope is answer coverage of the requested targets only.
func TestEnsurePostRepairCoverage_FullCoveragePasses(t *testing.T) {
	targets := []string{"sub#Bar"}
	results := []quarry.ResolveResult{
		{Target: "sub#Bar", Status: quarry.StatusFound},
		{Target: "sub#Unrelated", Status: quarry.StatusFound},
	}

	if err := ensurePostRepairCoverage(targets, results); err != nil {
		t.Errorf("ensurePostRepairCoverage(full coverage) = %v; want nil", err)
	}
}
