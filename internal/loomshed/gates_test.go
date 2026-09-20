// gates_test.go covers NewDiscussionGate and NewPlanGate's own outcome mapping, the ParsePlan error
// split that is the subtlest rule in the task, and the fail-closed severity predicate both gates key
// their pass/fail split on.
//
// It reuses three fixture helpers declared alongside the two producers this batch does not touch --
// validDecisionRecord and writeDiscussionFixture (discussionvalidate_test.go), seedPlanValidateFixture
// (planvalidate_test.go) -- rather than writing new ones, since all three already build exactly the
// on-disk shapes these cases need. Batch 5 relocates all three into a surviving fixture file when it
// deletes their current hosts, so this reuse is deliberate rather than a dependency on files that are
// about to disappear.
//
// All of it is untagged and offline, per the Test Tier Purity Invariant; the quarry-unavailable case
// asserts on the error path rather than requiring a resolvable fixture, which is what keeps the
// Quarry CGO Requirement Invariant from making this tier depend on a resolvable repository.

package loomshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planglyph"
)

func TestNewDiscussionGate(t *testing.T) {
	t.Run("Pass", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "support log")

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() Passed = %v; want true", result.Passed)
		}
		if result.Findings != "" {
			t.Errorf("gate() Findings = %q; want empty on a pass", result.Findings)
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		dir := t.TempDir()
		withoutGoal := strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1)
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutGoal, "support log")

		buf := captureGateWarnings(t)
		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for a decision record missing a required heading")
		}
		if !strings.Contains(result.Findings, "discussion-section-missing") {
			t.Errorf("gate() Findings = %q; want it to name the check that fired", result.Findings)
		}

		// The warn line is the only durable record of the refusal: the findings file itself lives in
		// the ephemeral run directory finalize deletes on the Done cleanup, so this assertion is what
		// keeps that promise honest against the closure, not just against the producer that is about to
		// be deleted.
		logged := buf.String()
		if !strings.Contains(logged, result.Findings) {
			t.Errorf("log = %q; want it to carry the same formatted findings as GateResult.Findings", logged)
		}
		if !strings.Contains(logged, "discussion gate failed validation") {
			t.Errorf("log = %q; want it to report the gate refusal", logged)
		}
	})

	// ErrorReturnsRatherThanAFailedGate is the case a validator error is returned rather than reported
	// as a failed gate: a decision record that is a directory is the error fixture the existing
	// validate tests already use (discussionvalidate_test.go's
	// DecisionRecordUnreadableReturnsErrorNotStuck).
	t.Run("ErrorReturnsRatherThanAFailedGate", func(t *testing.T) {
		dir := t.TempDir()
		_, supportLogPath := writeDiscussionFixture(t, dir, "", "support log")
		decisionRecordPath := filepath.Join(dir, "decision-record.md")
		if err := os.MkdirAll(decisionRecordPath, 0o755); err != nil {
			t.Fatalf("mkdir decision record: %v", err)
		}

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error (decision record path is a directory)")
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})
}

func TestNewPlanGate(t *testing.T) {
	t.Run("Pass", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanValidateFixture(t, anchorPath, false)

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() Passed = %v; want true", result.Passed)
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedFormatInvalidPlanValidateFixture(t, anchorPath)

		buf := captureGateWarnings(t)
		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unrecognized plan format")
		}
		if !strings.Contains(result.Findings, "format-unrecognized") {
			t.Errorf("gate() Findings = %q; want it to name the check that fired", result.Findings)
		}
		logged := buf.String()
		if !strings.Contains(logged, result.Findings) {
			t.Errorf("log = %q; want it to carry the same formatted findings as GateResult.Findings", logged)
		}
		if !strings.Contains(logged, "plan gate failed validation") {
			t.Errorf("log = %q; want it to report the gate refusal", logged)
		}
	})

	t.Run("PlanglyphErrorReturnsRatherThanAFailedGate", func(t *testing.T) {
		anchorPath := t.TempDir()
		seedGlyphPlanFixture(t, anchorPath, true, "sub#Foo", "")
		// A language: go plan pointed at a worktreeRoot that does not exist: openRepo cannot open it,
		// so the gate must return an error rather than reporting GateResult{Passed: false}.
		worktreeRoot := filepath.Join(t.TempDir(), "does-not-exist")

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for a quarry-unavailable worktreeRoot")
		}
		if !errors.Is(err, planglyph.ErrQuarryUnavailable) {
			t.Errorf("gate() error = %v; want it to wrap planglyph.ErrQuarryUnavailable", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})

	t.Run("InformationalOnlyFindingsPassWithAWarnLine", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := writeGlyphRepoFixture(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" resolves not_found with unit: not_found, which createFindings reports as the
		// informational create-new-unit finding -- no blocking finding in this plan.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "")

		buf := captureGateWarnings(t)
		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Fatalf("gate() Passed = false; want true for an informational-only findings set")
		}
		logged := buf.String()
		if !strings.Contains(logged, "create-new-unit") {
			t.Errorf("log = %q; want it to surface the informational finding for visibility on the pass path", logged)
		}
	})

	t.Run("MixedBlockingAndInformationalFailsTheGate", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := writeGlyphRepoFixture(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" is informational (create-new-unit); "sub#Missing" resolves not_found with
		// unit: found, which statusFindings reports as the blocking glyph-not-found finding. One
		// blocking finding is enough to fail the gate.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "sub#Missing")

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for a set carrying one blocking finding")
		}
	})
}

// writeOverviewOnlyPlanDir creates the plan directory under anchorPath and writes only overview
// (00-overview.md's content), leaving every card file absent -- the shape the ParsePlan
// per-card-read-fault fixtures below build on top of.
func writeOverviewOnlyPlanDir(t *testing.T, anchorPath, overview string) (planDir string) {
	t.Helper()
	planDir = filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
	return planDir
}

// TestNewPlanGate_ParsePlanSplit covers the ParsePlan error split -- the subtlest rule in the task:
// a malformed overview, an unparseable card index, and an absent 00-overview.md each produce
// Passed false with the error's own text as findings, while both of ParsePlan's read faults produce
// a returned error -- the overview read and the per-card read reached through parseCardFile.
func TestNewPlanGate_ParsePlanSplit(t *testing.T) {
	t.Run("MalformedOverviewFrontmatterIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// Unparseable YAML frontmatter: yaml.Decode fails, which ParsePlan wraps as a plain
		// fmt.Errorf, never a *fs.PathError.
		writeOverviewOnlyPlanDir(t, anchorPath, "---\nformat: [not, valid\n---\n\n## Card Index\n\n1 — c — c\n")

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (a malformed overview is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unparseable overview")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("UnparseableCardIndexLineIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// A Card Index line that matches none of cardIndexLineRe's accepted separators.
		writeOverviewOnlyPlanDir(t, anchorPath, "---\nformat: 5\napproved: true\nlanguage: none\n---\n\n## Card Index\n\nnot a valid card index line\n")

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (an unparseable card index line is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unparseable card index line")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("AbsentOverviewIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// No _lyx/plan/00-overview.md at all: ParsePlan's os.IsNotExist branch returns a plain
		// fmt.Errorf, never wrapping the underlying *fs.PathError with %w -- so it is findings, the same
		// disposition discussionparser.Validate already gives a missing file.

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (an absent overview is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an absent plan overview")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("OverviewReadFaultIsAReturnedError", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		// A directory at 00-overview.md's own path forces os.ReadFile to fail with a non-not-exist
		// *fs.PathError, which ParsePlan wraps with %w -- the overview read fault.
		if err := os.MkdirAll(filepath.Join(planDir, "00-overview.md"), 0o755); err != nil {
			t.Fatalf("mkdir overview path: %v", err)
		}

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for an unreadable overview file")
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("gate() error = %v; want it to wrap a *fs.PathError", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})

	t.Run("PerCardReadFaultIsAReturnedError", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		planDir := writeOverviewOnlyPlanDir(t, anchorPath, "---\nformat: 5\napproved: true\nlanguage: none\n---\n\n## Card Index\n\n1 — first-card — placeholder card 1\n")
		// A directory at the card file's own path, reached through parseCardFile, forces its
		// os.ReadFile to fail the same non-not-exist way the overview read fault above does.
		if err := os.MkdirAll(filepath.Join(planDir, "01-first-card.md"), 0o755); err != nil {
			t.Fatalf("mkdir card file path: %v", err)
		}

		gate := NewPlanGate(anchorPath, worktreeRoot)
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for an unreadable card file")
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("gate() error = %v; want it to wrap a *fs.PathError", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})
}

// TestHasBlockingFinding_AgainstTheGate exercises the same fail-closed severity table
// planvalidate_test.go's TestHasBlockingFinding_UnrecognizedSeverityFailsClosed pins, asserted here
// against the exact predicate NewPlanGate's own pass/fail split calls: an unrecognized or zero-value
// Severity fails the gate closed rather than silently passing, because planglyph.Severity is an open
// string type and neither shape can occur through a real resolve-backed findings set -- only a
// hand-built Finding, or a future producer that forgets to stamp one, can carry either.
func TestHasBlockingFinding_AgainstTheGate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		severity planglyph.Severity
		want     bool
	}{
		{"blocking blocks the gate", planglyph.SeverityBlocking, true},
		{"informational passes the gate", planglyph.SeverityInformational, false},
		{"the zero value blocks the gate", planglyph.Severity(""), true},
		{"an unrecognized severity blocks the gate", planglyph.Severity("advisory"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBlockingFinding([]planglyph.Finding{{Check: "some-check", Severity: tt.severity}})
			if got != tt.want {
				t.Errorf("hasBlockingFinding(severity %q) = %v; want %v", tt.severity, got, tt.want)
			}
		})
	}
}
