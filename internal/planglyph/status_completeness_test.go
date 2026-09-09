// status_completeness_test.go guards quarry's status vocabulary against silent widening.
// quarry.Status.Known() moved the definition of "readable" from lyx to quarry, so a status quarry
// adds later would pass the guard in doneCheckVerdicts and land on the resolved and stillExists
// booleans, which derive only from quarry.StatusFound, quarry.StatusMultipart and
// quarry.StatusNotFound — a fifth status would read as not-resolved-but-still-existing and produce
// an ordinary-looking verdict rather than the honest glyph-rejected diagnostic. This file's
// coverage assertion is the tripwire: it fails the moment quarry.Statuses and this file's own
// expectation table diverge in either direction.

package planglyph

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// statusOutcome names the expected doneCheckVerdicts outcome for one status across all four
// checkID arms doneCheckVerdicts switches on: true means the arm fires its blocking finding, false
// means it produces no finding.
type statusOutcome struct {
	createNotDone    bool
	deleteNotDone    bool
	renameNotDoneOld bool
	renameNotDoneNew bool
}

// expectedOutcomeByStatus is the table a human has consciously taught doneCheckVerdicts to handle,
// one entry per status quarry.Statuses names today.
var expectedOutcomeByStatus = map[quarry.Status]statusOutcome{
	quarry.StatusFound:     {createNotDone: false, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: false},
	quarry.StatusMultipart: {createNotDone: false, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: false},
	quarry.StatusAmbiguous: {createNotDone: true, deleteNotDone: true, renameNotDoneOld: true, renameNotDoneNew: true},
	quarry.StatusNotFound:  {createNotDone: true, deleteNotDone: false, renameNotDoneOld: false, renameNotDoneNew: true},
}

// TestStatusCompleteness_TableCoversQuarryStatuses is the whole reason this file exists: the table
// key set and quarry.Statuses cover each other exactly, in both directions, as genuine set
// membership tests. A length comparison would pass a same-size swap of one status for another, and
// sizing a range loop off len(quarry.Statuses) would assert nothing at all — both are rejected by
// design, per the statuses-is-read-only Shared Decision.
//
// This file deliberately does NOT also assert that every value in quarry.Statuses avoids
// glyph-rejected in TestStatusCompleteness_MatchesDoneCheckVerdicts below: that would hold for a
// hypothetical fifth status too, since Known() would admit it, so it cannot detect the drift this
// file exists to catch. Do not add it.
func TestStatusCompleteness_TableCoversQuarryStatuses(t *testing.T) {
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

// runDoneCheckVerdict drives doneCheckVerdicts through its own seam — a hand-built index holding a
// synthetic result carrying status — for one checkID, and returns the findings produced.
func runDoneCheckVerdict(t *testing.T, checkID string, r quarry.ResolveResult) []Finding {
	t.Helper()

	const key = "internal/greet#Thing"
	r.Target = key
	card := planparser.Card{Number: 1, Slug: "card"}
	entries := []doneCheckEntry{{card: card, checkID: checkID, key: key, display: key}}
	index := map[string]quarry.ResolveResult{key: r}

	findings, err := doneCheckVerdicts(entries, index)
	if err != nil {
		t.Fatalf("doneCheckVerdicts(%s) error = %v; want nil", checkID, err)
	}
	return findings
}

// TestStatusCompleteness_MatchesDoneCheckVerdicts drives doneCheckVerdicts through its own seam,
// one checkID at a time so the two rename arms — which both emit a Finding whose Check field is
// rename-not-done — are told apart by which arm produced the finding rather than by Check alone,
// and compares the result against expectedOutcomeByStatus's row for status.
func TestStatusCompleteness_MatchesDoneCheckVerdicts(t *testing.T) {
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

// TestStatusCompleteness_UnreadableStatusesFailClosed pins the two unreadable shapes — the
// zero-value status, quarry's pre-resolution rejection carrying Error and Reason, and a synthetic
// out-of-vocabulary status string — to exactly one blocking glyph-rejected finding for every one of
// the four checkID arms, with no create, delete or rename verdict, and confirms the two shapes
// render the two different Detail strings unreadableStatusDetail produces for them.
func TestStatusCompleteness_UnreadableStatusesFailClosed(t *testing.T) {
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
