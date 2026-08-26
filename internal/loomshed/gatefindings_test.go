// gatefindings_test.go asserts that this package's mechanical gates surface the determined findings
// they used to discard.
// The rule matters most exactly where it is cheapest to skip: Loom-Preflight carries no OnStuck at
// all, so its Stuck halts the run for a human; Discussion-Validate and Plan-Validate bounce to a
// writer that is respawned with no knowledge of the complaint. In every one of those cases the
// driver log is the only place the reason can be read, and it used to say nothing.

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

func TestDiscussionValidate_StuckSurfacesItsFindings(t *testing.T) {
	dir := t.TempDir()
	decisionRecord := filepath.Join(dir, "decision-record.md")
	supportLog := filepath.Join(dir, "support-log.md")
	// A decision record missing every required heading, with the support log present, so Validate
	// reports heading findings rather than a file-missing one.
	if err := os.WriteFile(decisionRecord, []byte("# Nothing required is here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", decisionRecord, err)
	}
	if err := os.WriteFile(supportLog, []byte("# Support log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", supportLog, err)
	}

	buf := captureGateWarnings(t)
	p := NewDiscussionValidate(NameDiscussionValidate, decisionRecord, supportLog)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want the zero value", ptr)
	}

	logged := buf.String()
	if !strings.Contains(logged, "discussion artifacts failed validation") {
		t.Errorf("log = %q; want it to report the validation refusal", logged)
	}
	if !strings.Contains(logged, "findings=") {
		t.Errorf("log = %q; want it to carry a findings field -- the refusal reason is the whole point", logged)
	}
	if !strings.Contains(logged, NameDiscussionValidate) {
		t.Errorf("log = %q; want it to name the producer %q", logged, NameDiscussionValidate)
	}
}

func TestPlanValidate_StuckSurfacesItsFindings(t *testing.T) {
	anchorPath, planDir := setupPlanDir(t)
	// A minimal plan that PARSES cleanly and fails validation on one determined check: approved is
	// false, which planparser reports as plan-unapproved. A plan that failed to parse instead would
	// take Call's hard-error branch and never reach the Stuck this test is about.
	overview := "---\nformat: 4\napproved: false\n---\n\n" +
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

	buf := captureGateWarnings(t)
	p := NewPlanValidate(NamePlanValidate, anchorPath, anchorPath)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	logged := buf.String()
	if !strings.Contains(logged, "plan failed validation") {
		t.Errorf("log = %q; want it to report the validation refusal", logged)
	}
	if !strings.Contains(logged, "plan-unapproved") {
		t.Errorf("log = %q; want it to name the check that fired", logged)
	}
	if !strings.Contains(logged, NamePlanValidate) {
		t.Errorf("log = %q; want it to name the producer %q, so Plan-Validate and Plan-Revalidate are distinguishable", logged, NamePlanValidate)
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
