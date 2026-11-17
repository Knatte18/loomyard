// gatefindings_test.go asserts that a failed mechanical gate logs the specific determined findings behind its refusal.
// The warn line is the only record of those findings that outlives the ephemeral run directory.
// The findings file is deleted with that directory.
// NewDiscussionGate and NewPlanGate do not bounce a failed attempt to a respawned writer,
// and an exhausted gate on a gated writer row halts the run for a human,
// so the warn line is where an operator learns why.
//
// gates_test.go covers each gate's pass, fail and error mapping, including the ParsePlan error split, in full.
// This file covers only that a failed gate's named findings reach the warn line, not merely "some findings".
// Each case pins a specific check name (discussion-section-missing, index-file-mismatch).
// gates_test.go's findings-surfacing cases do not assert that name.

package loomshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestDiscussionGate_FailureSurfacesItsFindings checks that a refused decision record logs its determined findings.
// The fixture is a decision record missing every required heading, with the support log present,
// so the findings are heading findings rather than a file-missing one.
func TestDiscussionGate_FailureSurfacesItsFindings(t *testing.T) {
	tiergate.Require(t, 0)
	dir := t.TempDir()
	decisionRecord := filepath.Join(dir, "decision-record.md")
	supportLog := filepath.Join(dir, "support-log.md")
	if err := os.WriteFile(decisionRecord, []byte("# Nothing required is here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", decisionRecord, err)
	}
	if err := os.WriteFile(supportLog, []byte("# Support log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", supportLog, err)
	}

	buf := logcapture.Capture(t)
	gate := NewDiscussionGate(DiscussionCheck{
		RecordPath:     decisionRecord,
		SupportLogPath: supportLog,
		RelevantPath:   filepath.Join(dir, "relevant.md"),
		WorktreeRoot:   dir,
		Index:          planglyph.NewIndex(fabricengine.NewReferenceRule()),
		History:        func() ([][]byte, error) { return nil, nil },
		Citations:      true,
	})

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

// TestPlanGate_FailureSurfacesItsFindings checks that a refused plan logs its determined findings.
// The fixture is a plan that parses cleanly and fails validation on exactly one determined check:
// an extra .md file on disk that no Card Index entry names, which planparser reports as index-file-mismatch.
// The subject is that this one finding reaches the warn line, which is plumbing, not mode behaviour,
// so the fixture carries no approval dimension:
// the gate always runs planglyph.ValidateFormat, never the require_approved-aware planglyph.Validate.
func TestPlanGate_FailureSurfacesItsFindings(t *testing.T) {
	tiergate.Require(t, 0)
	anchorPath, planDir := setupPlanDir(t)
	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Language: "none",
		Sections: []plankit.Section{{Heading: "verify:", Body: "```bash\ngo test ./...\n```"}},
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "add-helper",
			Summary: "Add the helper",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"helper.go"}}},
			Intent:  "Add the helper.",
			Commit:  "1: Add the helper",
		}},
	})
	// The one file the Card Index does not name; it is what makes index-file-mismatch the plan's single finding.
	if err := os.WriteFile(filepath.Join(planDir, "99-unindexed.md"), []byte("stray"), 0o644); err != nil {
		t.Fatalf("WriteFile(99-unindexed.md): %v", err)
	}

	buf := logcapture.Capture(t)
	gate := NewPlanGate(anchorPath, anchorPath, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})

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

// TestBatcherRows_StuckSurfacesTheBatcherError checks that a batcher-backed row logs its batcher error.
// The row carries no OnStuck, so its Stuck halts the run for a human.
// batcher.Active reports unknown-name, malformed YAML and I/O failure as one bare error with no sentinel,
// so the error text is the only thing that tells an operator which of the three happened.
// It does not call t.Parallel, because logcapture.Capture redirects the process-global logger output.
//
//testtiming:keep pins that each batcher-backed row logs its name and the batcher error text on a stuck verdict, which its covering tests never read
func TestBatcherRows_StuckSurfacesTheBatcherError(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		new  func(anchorPath string) shedengine.ShedProducer
	}{
		{"Webster", func(anchorPath string) shedengine.ShedProducer {
			return NewWebsterProducer("Webster", configengine.Dirs{Anchor: anchorPath}, nil, websterengine.RunDeps{}, func() error { return nil })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchorPath := t.TempDir()
			writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")

			buf := logcapture.Capture(t)
			shedfake.RequireOutcome(t, tt.new(anchorPath), shedengine.Stuck)
			got := buf.String()
			for _, want := range []string{tt.name, "no-such-batcher"} {
				if !strings.Contains(got, want) {
					t.Errorf("warning log = %q; want it to contain %q", got, want)
				}
			}
		})
	}
}
