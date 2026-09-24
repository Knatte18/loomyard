// gatefindings_test.go asserts that this package's mechanical gates surface the determined findings
// they used to discard.
// The rule matters most exactly where it is cheapest to skip, but the reason has changed with the
// row-removal batch: Loom-Preflight still carries no OnStuck at all, so its Stuck still halts the
// run for a human. NewDiscussionGate and NewPlanGate, though, no longer bounce to a respawned
// writer on a failed attempt -- a gated writer row's exhausted gate halts the run for a human too,
// so the logger.Warn line this file pins is not merely the best record of a refusal, it is the ONLY
// one that outlives the ephemeral run directory the findings file is deleted with.
//
// This file and gates_test.go both drive the two gate closures and must not silently drift into
// testing each other's subject: gates_test.go owns the closures' own pass/fail/error mapping,
// including the ParsePlan error split, in full; this file owns only that a failed gate's specific
// determined findings (not merely "some findings") reach the warn line, which is why each case below
// pins a specific check name (discussion-section-missing, index-file-mismatch) that gates_test.go's
// own findings-surfacing cases do not assert.

package loomshed

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// captureGateWarnings redirects logger output into a buffer for the duration of one test, restoring
// os.Stderr via t.Cleanup -- the same pattern internal/shedadapters/bouncer_judge_test.go uses.
func captureGateWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

// TestDiscussionGate_FailureSurfacesItsFindings replaces the deleted Discussion-Validate producer's
// own findings-surfacing case: the gate closure now carries the same obligation the producer used
// to, over the same fixture shape (a decision record missing every required heading, with the
// support log present, so Validate reports heading findings rather than a file-missing one).
func TestDiscussionGate_FailureSurfacesItsFindings(t *testing.T) {
	dir := t.TempDir()
	decisionRecord := filepath.Join(dir, "decision-record.md")
	supportLog := filepath.Join(dir, "support-log.md")
	if err := os.WriteFile(decisionRecord, []byte("# Nothing required is here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", decisionRecord, err)
	}
	if err := os.WriteFile(supportLog, []byte("# Support log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", supportLog, err)
	}

	buf := captureGateWarnings(t)
	gate := NewDiscussionGate(decisionRecord, supportLog)

	result, err := gate()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if result.Passed {
		t.Fatalf("gate() Passed = true; want false for a decision record missing every required heading")
	}

	logged := buf.String()
	if !strings.Contains(logged, "discussion gate failed validation") {
		t.Errorf("log = %q; want it to report the gate refusal", logged)
	}
	if !strings.Contains(logged, "discussion-section-missing") {
		t.Errorf("log = %q; want it to name the check that fired", logged)
	}
	if !strings.Contains(logged, "Discussion-Gate") {
		t.Errorf("log = %q; want it to name the gate", logged)
	}
}

// TestPlanGate_FailureSurfacesItsFindings replaces the deleted Plan-Validate producer's own
// findings-surfacing case: the gate closure now carries the same obligation the producer used to,
// over the same fixture shape (a plan that PARSES cleanly and fails validation on exactly one
// determined check: an extra .md file on disk that no Card Index entry names, which planparser
// reports as index-file-mismatch). This test's subject is that exactly one finding reaches the warn
// line -- plumbing, not mode behaviour -- so the fixture carries no approval dimension at all: the
// gate always runs planglyph.ValidateFormat, never the require_approved-aware planglyph.Validate.
func TestPlanGate_FailureSurfacesItsFindings(t *testing.T) {
	anchorPath, planDir := setupPlanDir(t)
	overview := "---\nformat: 5\napproved: true\nlanguage: none\n---\n\n" +
		"# Plan: add a helper\n\n" +
		"## Card Index\n\n" +
		"1 — add-helper — Add the helper\n\n" +
		"## verify:\n\n```bash\ngo test ./...\n```\n"
	card := "# Card 1 — Add the helper\n\n" +
		"**Create:**\n- `helper.go`\n\n" +
		"**Intent:** Add the helper.\n\n" +
		"**Commit:** 1: Add the helper\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("WriteFile(00-overview.md): %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "01-add-helper.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("WriteFile(01-add-helper.md): %v", err)
	}
	// The one file the Card Index does not name -- this is what makes index-file-mismatch the
	// plan's single finding.
	if err := os.WriteFile(filepath.Join(planDir, "99-unindexed.md"), []byte("stray"), 0o644); err != nil {
		t.Fatalf("WriteFile(99-unindexed.md): %v", err)
	}

	buf := captureGateWarnings(t)
	gate := NewPlanGate(anchorPath, anchorPath)

	result, err := gate()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if result.Passed {
		t.Fatalf("gate() Passed = true; want false for a plan carrying one blocking finding")
	}

	logged := buf.String()
	if !strings.Contains(logged, "plan gate failed validation") {
		t.Errorf("log = %q; want it to report the gate refusal", logged)
	}
	if !strings.Contains(logged, "index-file-mismatch") {
		t.Errorf("log = %q; want it to name the check that fired", logged)
	}
	if !strings.Contains(logged, "Plan-Gate") {
		t.Errorf("log = %q; want it to name the gate", logged)
	}
}

func TestLoomPreflight_StuckSurfacesItsFailures(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	// A coherent status file that has already advanced past the fresh-seed point, which CheckSeed
	// reports as half-finished.
	status := `{"current_producer":"Plan-Write","state":"running","error":"","pause_requested":false,` +
		`"activity":{"now":"Plan-Write","last":"Discussion-Bouncer → done","wait":""},` +
		`"history":[{"producer":"Preflight","outcome":"done","output":"","at":"2026-08-26T00:00:00Z"},` +
		`{"producer":"Discussion-Write","outcome":"done","output":"","at":"2026-08-26T00:01:00Z"}],` +
		`"product":{"slug":"s","parent":"main"}}`
	if err := os.WriteFile(statusPath, []byte(status), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", statusPath, err)
	}

	buf := captureGateWarnings(t)
	p := NewLoomPreflight(NameLoomPreflight, statusPath, statusLockPath)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	logged := buf.String()
	if !strings.Contains(logged, "seed is not a coherent fresh start") {
		t.Errorf("log = %q; want it to report the seed refusal", logged)
	}
	if !strings.Contains(logged, "failures=") {
		t.Errorf("log = %q; want it to carry a failures field -- this row has no OnStuck, so a human reads this line or nothing", logged)
	}
}

// TestBatchifier_StuckSurfacesTheBatcherError and its Webster twin close the last two rows in this
// package that mapped a fault onto Stuck while discarding the reason. Both carry no OnStuck, so
// their Stuck halts the run for a human, and batcher.Active conflates unknown-name, malformed YAML,
// and I/O failure into one bare error with no sentinel -- so the error text is the only thing that
// can tell an operator which of the three happened.
func TestBatchifier_StuckSurfacesTheBatcherError(t *testing.T) {
	anchorPath := t.TempDir()
	writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")

	buf := captureGateWarnings(t)
	producer := NewBatchifier("Batchifier", anchorPath)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := buf.String()
	for _, want := range []string{"Batchifier", "no-such-batcher"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning log = %q; want it to contain %q", got, want)
		}
	}
}

func TestWebsterProducer_StuckSurfacesTheBatcherError(t *testing.T) {
	anchorPath := t.TempDir()
	writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")

	buf := captureGateWarnings(t)
	producer := NewWebsterProducer("Webster", anchorPath, nil, websterengine.RunDeps{}, func() error { return nil })

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := buf.String()
	for _, want := range []string{"Webster", "no-such-batcher"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning log = %q; want it to contain %q", got, want)
		}
	}
}
