// dobbydeps_test.go pins how the dobby reads a hub reconcile's report on the ended runs it asked to tear down.
// It also pins the gate the describer agent's run carries over its draft, and the findings read of a description.

package beamcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// validBeamDescription is a change description that passes validation.
const validBeamDescription = "# Ship the beam\n\nGroups the work.\n\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** beam\n"

func TestDescribeGate_JudgesTheSpecsOneOutputFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// content is the draft's content; empty leaves no draft.
		content    string
		wantPassed bool
	}{
		{name: "ValidDraftPasses", content: validBeamDescription, wantPassed: true},
		{name: "DraftWithAFindingFails", content: "# Ship the beam\n"},
		{name: "MissingDraftFails"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			draft := filepath.Join(t.TempDir(), "draft.md")
			if tt.content != "" {
				if err := os.WriteFile(draft, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", draft, err)
				}
			}
			gate := describeGate(shuttleengine.Spec{OutputFiles: []string{draft}})
			if len(gate) != 1 || gate[0].Name != describeGateName || gate[0].Attempts != describeGateAttempts {
				t.Fatalf("describeGate() = %+v; want the one %s entry with %d attempts", gate, describeGateName, describeGateAttempts)
			}
			result, err := gate[0].Gate()
			if err != nil || result.Passed != tt.wantPassed {
				t.Errorf("gate() = %+v, %v; want passed %v", result, err, tt.wantPassed)
			}
		})
	}
}

func TestDescriptionFindings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// content is the description's content; empty leaves no file.
		content   string
		wantCheck string
	}{
		{name: "ValidDescriptionHasNone", content: validBeamDescription},
		{name: "EmptyBodyIsNamed", content: "# Ship the beam\n", wantCheck: summaryparser.CheckEmptyBody},
		{name: "MissingFileIsAFinding", wantCheck: summaryparser.CheckMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "summary.md")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
				}
			}
			findings, err := descriptionFindings(path)
			if err != nil {
				t.Fatalf("descriptionFindings() = _, %v; want nil", err)
			}
			if tt.wantCheck == "" {
				if len(findings) != 0 {
					t.Errorf("descriptionFindings() = %v; want none", findings)
				}
				return
			}
			if len(findings) != 1 || !strings.Contains(findings[0], tt.wantCheck) {
				t.Errorf("descriptionFindings() = %v; want the one %s finding", findings, tt.wantCheck)
			}
		})
	}
}

func TestRunNoticeTags(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := runNoticeTags("pay")
	want := creelengine.PostTags{Topic: "dobby-run-notice/pay", Run: "pay"}
	if got != want {
		t.Errorf("runNoticeTags(pay) = %+v; want %+v", got, want)
	}
}

func TestEndedTeardownResults(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	ended := func(mutate func(entry *fabricengine.EntryOutcome)) fabricengine.OriginReport {
		entry := fabricengine.EntryOutcome{Slug: "x", State: fabricengine.EntryEnded, How: "done", Local: true}
		mutate(&entry)
		return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{entry}}
	}
	notEnded := "origin's board does not show x ended, so the reconcile left its pair; way forward: read lym batten status x"
	tests := []struct {
		name         string
		report       fabricengine.OriginReport
		reconcileErr error
		want         string
	}{
		{"RemovedPairIsNoFailure", ended(func(e *fabricengine.EntryOutcome) { e.Removed = true }), nil, ""},
		{"BusyPairNamesItsWayForward", ended(func(e *fabricengine.EntryOutcome) { e.Busy, e.WayForward = true, "wait for the driver" }), nil, "its pair is busy; wait for the driver"},
		{"FailedTeardownNamesItsError", ended(func(e *fabricengine.EntryOutcome) { e.Error = "boom" }), nil, "its pair's teardown failed: boom"},
		{"AnotherRunsRemovalLeavesThisOne", ended(func(e *fabricengine.EntryOutcome) { e.Slug, e.Removed = "y", true }), nil, notEnded},
		{"ActiveEntryIsLeft", fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "x", State: fabricengine.EntryActive}}}, nil, notEnded},
		{"UnreachableOriginJudgesNothing", fabricengine.OriginReport{Unreachable: true}, nil, "origin was unreachable, so the reconcile judged no run ended"},
		{"ReconcileErrorFailsEveryRun", fabricengine.OriginReport{}, errors.New("ls-remote failed"), "the hub's origin reconcile failed: ls-remote failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := endedTeardownResults([]string{"x"}, tt.report, tt.reconcileErr)
			got, reported := results["x"]
			if !reported || len(results) != 1 {
				t.Fatalf("endedTeardownResults() = %v; want one result for x", results)
			}
			gotText := ""
			if got != nil {
				gotText = got.Error()
			}
			if gotText != tt.want {
				t.Errorf("endedTeardownResults()[x] = %q; want %q", gotText, tt.want)
			}
		})
	}
}
