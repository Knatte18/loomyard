// status_completeness_test.go guards the status vocabulary against silent widening.
// A status that quarry adds later would reach the resolved and stillExists booleans.
// Those booleans derive only from quarry.StatusFound, quarry.StatusMultipart and quarry.StatusNotFound.
// Such a status would read as not resolved yet still existing.
// It would then produce an ordinary-looking verdict instead of the glyph-rejected diagnostic.
// The coverage assertion is the tripwire:
// it fails when quarry.Statuses and the expectation table diverge in either direction.

package planglyph

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/quarry/quarry"
)

// statusOutcome holds the expected verdict for one status on each of the four check arms, one field per arm.
// A true field means the arm reports a blocking finding; a false field means it reports none.
type statusOutcome struct {
	createNotDone    bool
	deleteNotDone    bool
	renameNotDoneOld bool
	renameNotDoneNew bool
}

// expectedOutcomeByStatus maps each status in quarry.Statuses to the statusOutcome the checks must produce for it.
var expectedOutcomeByStatus = map[quarry.Status]statusOutcome{
	quarry.StatusFound:     {createNotDone: false, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: false},
	quarry.StatusMultipart: {createNotDone: false, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: false},
	quarry.StatusAmbiguous: {createNotDone: true, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: true},
	quarry.StatusNotFound:  {createNotDone: true, deleteNotDone: false, renameNotDoneOld: false, renameNotDoneNew: true},
}

// TestStatusCompleteness_TableCoversQuarryStatuses fails when the keys of the table and quarry.Statuses differ.
// It tests set membership in both directions.
// A length comparison would pass a same-size swap of one status for another.
//
// Do not add a check that every value in quarry.Statuses avoids glyph-rejected.
// quarry.Status.Known() admits a status that quarry adds later, so such a check would pass for it.
// It would then miss the drift this test catches.
//
//testtiming:keep a guard that fires when quarry.Statuses and the expectation table diverge, which no covering test asserts
func TestStatusCompleteness_TableCoversQuarryStatuses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tableStatuses := make(map[quarry.Status]bool, len(expectedOutcomeByStatus))
	for s := range expectedOutcomeByStatus {
		tableStatuses[s] = true
	}
	quarryStatuses := make(map[quarry.Status]bool, len(quarry.Statuses))
	for _, s := range quarry.Statuses {
		quarryStatuses[s] = true
	}

	for _, s := range quarry.Statuses {
		if !tableStatuses[s] {
			t.Errorf("quarry.Statuses contains %q, which has no entry in expectedOutcomeByStatus — quarry widened its vocabulary and doneCheckVerdicts was not taught the new status", s)
		}
	}
	for s := range expectedOutcomeByStatus {
		if !quarryStatuses[s] {
			t.Errorf("expectedOutcomeByStatus has an entry for %q, which quarry.Statuses no longer contains", s)
		}
	}
}

// runDoneCheckVerdict runs the done-check verdicts for one checkID over a single synthetic result.
// The result carries status, and the function returns the findings produced.
func runDoneCheckVerdict(t *testing.T, checkID string, r quarry.ResolveResult) []Finding {
	t.Helper()

	const key = "internal/greet#Thing"
	r.Target = key
	card := planparser.Card{Number: 1, Slug: "card"}
	entries := []doneCheckEntry{{card: card, checkID: checkID, key: key, display: key}}
	index := map[string]quarry.ResolveResult{key: r}

	findings, err := doneCheckVerdicts(&planparser.Plan{}, "", entries, index, nil)
	if err != nil {
		t.Fatalf("doneCheckVerdicts(%s) error = %v; want nil", checkID, err)
	}
	return findings
}

// TestStatusCompleteness_MatchesDoneCheckVerdicts checks each status against its expectedOutcomeByStatus row.
// It runs one checkID at a time, because both rename arms emit a finding whose Check is rename-not-done.
// It tells the two arms apart by the arm that produced the finding, not by Check alone.
//
//testtiming:keep pins every quarry status against all four check arms from the guard table, where TestDoneChecks_Verdicts reaches found and not_found only
func TestStatusCompleteness_MatchesDoneCheckVerdicts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for status, want := range expectedOutcomeByStatus {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			checks := []struct {
				checkID   string
				fires     bool
				wantCheck string
			}{
				{"create-not-done", want.createNotDone, "create-not-done"},
				{"delete-not-done", want.deleteNotDone, "delete-not-done"},
				{"rename-not-done-old", want.renameNotDoneOld, "rename-not-done"},
				{"rename-not-done-new", want.renameNotDoneNew, "rename-not-done"},
			}
			for _, c := range checks {
				findings := runDoneCheckVerdict(t, c.checkID, quarry.ResolveResult{Status: status})
				if !c.fires {
					if len(findings) != 0 {
						t.Errorf("%s/%s: findings = %v; want none", status, c.checkID, findings)
					}
					continue
				}
				if len(findings) != 1 {
					t.Fatalf("%s/%s: findings = %v; want exactly one %s finding", status, c.checkID, findings, c.wantCheck)
				}
				if findings[0].Check != c.wantCheck {
					t.Errorf("%s/%s: finding check = %q; want %q", status, c.checkID, findings[0].Check, c.wantCheck)
				}
			}
		})
	}
}

// TestStatusCompleteness_UnreadableStatusesFailClosed checks the two unreadable status shapes.
// The first is quarry's pre-resolution rejection, which carries Error and Reason and a zero-value status.
// The second is a synthetic out-of-vocabulary status string.
// Each shape yields exactly one blocking glyph-rejected finding on each of the four check arms.
// No create, delete or rename verdict accompanies it, and each shape renders its own Detail string.
//
//testtiming:keep pins the two unreadable-status Detail strings on every check arm, which TestCreateFindings_UnreadableStatusFailsClosed does not assert
func TestStatusCompleteness_UnreadableStatusesFailClosed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cases := []struct {
		name          string
		result        quarry.ResolveResult
		wantSubstring string
	}{
		{"rejected before resolution", quarry.ResolveResult{Error: "not a glyph", Reason: "no_separator"}, "was rejected before resolution"},
		{"unrecognized resolve status", quarry.ResolveResult{Status: quarry.Status("weird_new_status")}, "answered the unrecognized resolve status"},
	}
	checkIDs := []string{"create-not-done", "delete-not-done", "rename-not-done-old", "rename-not-done-new"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, checkID := range checkIDs {
				findings := runDoneCheckVerdict(t, checkID, tc.result)
				if len(findings) != 1 {
					t.Fatalf("%s/%s: findings = %v; want exactly one glyph-rejected finding", tc.name, checkID, findings)
				}
				if findings[0].Check != "glyph-rejected" {
					t.Errorf("%s/%s: finding check = %q; want %q", tc.name, checkID, findings[0].Check, "glyph-rejected")
				}
				if findings[0].Severity != SeverityBlocking {
					t.Errorf("%s/%s: finding severity = %q; want %q", tc.name, checkID, findings[0].Severity, SeverityBlocking)
				}
				if !strings.Contains(findings[0].Detail, tc.wantSubstring) {
					t.Errorf("%s/%s: finding detail = %q; want it to contain %q", tc.name, checkID, findings[0].Detail, tc.wantSubstring)
				}
			}
		})
	}
}
