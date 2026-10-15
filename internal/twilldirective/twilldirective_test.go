// Every test here is untagged Tier 1: it reads a seeded temporary stencils directory and spawns nothing.

package twilldirective

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// filledWriterFork returns a WriterFork with every field set.
func filledWriterFork() *WriterFork {
	return &WriterFork{
		ExploreStep: "EXPLORE-STEP",
		InputRecord: "INPUT-RECORD",
		LastFile:    "LAST-FILE",
		FilesDir:    "FILES-DIR",
	}
}

// filledCriticalJudge returns a CriticalJudge with every field set.
func filledCriticalJudge() *CriticalJudge {
	return &CriticalJudge{
		GoverningInput:  "GOVERNING-INPUT",
		DisputeEvidence: "DISPUTE-EVIDENCE",
	}
}

// filledValues returns a Values with every field set but WriterFork and CriticalJudge.
func filledValues() Values {
	return Values{
		ReviewedFiles:         "`/abs/one.md` and `/abs/two.md`",
		SpawnPoint:            "SPAWN-POINT-SENTENCE",
		RejectionRule:         "REJECTION-RULE-SENTENCE",
		SeverityRule:          "SEVERITY-RULE-PHRASE",
		RubricPath:            "/abs/rubric.md",
		Lenses:                []Lens{{Name: "alpha", Text: "Look at A.\nThen at B.\n"}, {Name: "beta", Text: "Look at C."}},
		FormatStep:            "STEP-FORMAT",
		SelfCheckStep:         "STEP-CHECK",
		SelfCheckCommand:      "lyx check --now",
		FinalSelfCheckCommand: "lyx check --final",
		DispositionPlace:      "the PLACE section",
	}
}

// TestDirective_RendersEveryValueVerbatim covers the seeded stencil: each value lands, a lens's inner newlines collapse to spaces, and the text has no marker, leading banner or trailing newline.
func TestDirective_RendersEveryValueVerbatim(t *testing.T) {
	t.Parallel()

	withWriterFork := filledValues()
	withWriterFork.WriterFork = filledWriterFork()
	withWriterFork.CriticalJudge = filledCriticalJudge()
	tests := []struct {
		name   string
		values Values
		wants  []string
		// wantNots are strings the text must lack.
		wantNots []string
		// inOrder are strings that must appear in this order.
		inOrder []string
	}{
		{
			name:   "without a writer fork",
			values: filledValues(),
			wants: []string{
				"runs `lyx check --now` per `STEP-CHECK` until it passes",
				"STEP-FORMAT",
				"died before its Agent call still spawns once",
				"   REJECTION-RULE-SENTENCE\n5. Record the lens review at `the PLACE section`",
			},
			wantNots: []string{"fork0", "GOVERNING-INPUT", "DISPUTE-EVIDENCE"},
		},
		{
			name:   "with a writer fork",
			values: withWriterFork,
			wants: []string{
				"EXPLORE-STEP",
				"INPUT-RECORD",
				"LAST-FILE last",
				"every file in FILES-DIR",
				"Spawn one fork, fork0",
				"against the code and GOVERNING-INPUT",
				"opened in this session, DISPUTE-EVIDENCE, and what it shows",
			},
			wantNots: []string{"died before its Agent call", "REJECTION-RULE-SENTENCE"},
			inOrder:  []string{"SPAWN-POINT-SENTENCE", "Spawn one fork, fork0", "1. Read the rubric"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)

			got, err := Directive(dir, tt.values)
			if err != nil {
				t.Fatalf("Directive = _, %v; want nil error", err)
			}
			wants := append([]string{
				"`/abs/one.md` and `/abs/two.md` were written and passed the self-check before you were spawned",
				"SPAWN-POINT-SENTENCE\n",
				"forks are outstanding is allowed.\n4. Judge every finding",
				"returned no report.\n6. Run `lyx check --final` again",
				"each with SEVERITY-RULE-PHRASE, the section or decision it concerns",
				"/abs/rubric.md",
				"- alpha: Look at A. Then at B.\n- beta: Look at C.",
				"6. Run `lyx check --final` again",
				"the PLACE section",
			}, tt.wants...)
			for _, want := range wants {
				if !strings.Contains(got, want) {
					t.Errorf("Directive = %q; want it to contain %q", got, want)
				}
			}
			for _, wantNot := range tt.wantNots {
				if strings.Contains(got, wantNot) {
					t.Errorf("Directive = %q; want it to lack %q", got, wantNot)
				}
			}
			at := 0
			for _, want := range tt.inOrder {
				next := strings.Index(got[at:], want)
				if next < 0 {
					t.Fatalf("Directive lacks %q after offset %d; want %q in order", want, at, tt.inOrder)
				}
				at += next
			}
			if strings.Contains(got, "{{") {
				t.Errorf("Directive = %q; want no marker", got)
			}
			if strings.HasPrefix(got, "<!--") {
				t.Errorf("Directive = %q; want no leading comment", got)
			}
			if strings.HasSuffix(got, "\n") {
				t.Errorf("Directive = %q; want no trailing newline", got)
			}
			if strings.Contains(got, "\n\n\n") {
				t.Errorf("Directive = %q; want no run of blank lines", got)
			}
		})
	}
}

// TestDirective_MissingValueNamesTheMarker covers a host value or the lens list left empty: Directive errors and the error names that value's marker.
func TestDirective_MissingValueNamesTheMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		empty  func(v *Values)
		marker string
	}{
		{"ReviewedFiles", func(v *Values) { v.ReviewedFiles = "" }, "reviewed_files"},
		{"SpawnPoint", func(v *Values) { v.SpawnPoint = "" }, "spawn_point"},
		{"GoverningInput", func(v *Values) { v.CriticalJudge = filledCriticalJudge(); v.CriticalJudge.GoverningInput = "" }, "governing_input"},
		{"DisputeEvidence", func(v *Values) { v.CriticalJudge = filledCriticalJudge(); v.CriticalJudge.DisputeEvidence = "" }, "dispute_evidence"},
		{"RejectionRule without a critical judge", func(v *Values) { v.RejectionRule = "" }, "rejection_rule"},
		{"SeverityRule", func(v *Values) { v.SeverityRule = "" }, "severity_rule"},
		{"FinalSelfCheckCommand", func(v *Values) { v.FinalSelfCheckCommand = "" }, "final_self_check_command"},
		{"Lenses", func(v *Values) { v.Lenses = nil }, "lens_list"},
		{"ExploreStep", func(v *Values) { v.WriterFork = filledWriterFork(); v.WriterFork.ExploreStep = "" }, "explore_step"},
		{"InputRecord", func(v *Values) { v.WriterFork = filledWriterFork(); v.WriterFork.InputRecord = "" }, "input_record"},
		{"LastFile", func(v *Values) { v.WriterFork = filledWriterFork(); v.WriterFork.LastFile = "" }, "last_file"},
		{"FilesDir", func(v *Values) { v.WriterFork = filledWriterFork(); v.WriterFork.FilesDir = "" }, "files_dir"},
		{"FormatStep with a writer fork", func(v *Values) { v.WriterFork = filledWriterFork(); v.FormatStep = "" }, "format_step"},
		{"SelfCheckCommand without a writer fork", func(v *Values) { v.SelfCheckCommand = "" }, "self_check_command"},
		{"SelfCheckStep without a writer fork", func(v *Values) { v.SelfCheckStep = "" }, "self_check_step"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			values := filledValues()
			tt.empty(&values)

			got, err := Directive(dir, values)
			if err == nil {
				t.Fatalf("Directive = %q, nil; want an error", got)
			}
			if !strings.Contains(err.Error(), tt.marker) {
				t.Errorf("error = %q; want it to name %q", err, tt.marker)
			}
		})
	}
}

// TestDirective_MissingStencilNamesIt covers a stencil removed from the directory: Directive's error names it when the host reads it, and a stencil of the part the host does not ask for is never read.
func TestDirective_MissingStencilNamesIt(t *testing.T) {
	t.Parallel()

	withWriterFork := filledValues()
	withWriterFork.WriterFork = filledWriterFork()
	withCriticalJudge := filledValues()
	withCriticalJudge.CriticalJudge = filledCriticalJudge()
	tests := []struct {
		name    string
		values  Values
		removed string
		// wantRendered is true when the removed stencil is not one the host reads, so Directive renders cleanly.
		wantRendered bool
	}{
		{"the directive", filledValues(), directiveStencil, false},
		{"the writer-fork part with a writer fork", withWriterFork, writerForkStencil, false},
		{"the relaunch part without a writer fork", filledValues(), relaunchStencil, false},
		{"the relaunch part with a writer fork", withWriterFork, relaunchStencil, true},
		{"the writer-fork part without a writer fork", filledValues(), writerForkStencil, true},
		{"the critical-judge part with a critical judge", withCriticalJudge, criticalJudgeStencil, false},
		{"the plain-judge part without a critical judge", filledValues(), plainJudgeStencil, false},
		{"the plain-judge part with a critical judge", withCriticalJudge, plainJudgeStencil, true},
		{"the critical-judge part without a critical judge", filledValues(), criticalJudgeStencil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			stencilkit.Remove(t, dir, tt.removed)

			got, err := Directive(dir, tt.values)
			if tt.wantRendered {
				if err != nil {
					t.Fatalf("Directive = _, %v; want nil error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Directive = %q, nil; want an error", got)
			}
			if !strings.Contains(err.Error(), tt.removed) {
				t.Errorf("error = %q; want it to name %q", err, tt.removed)
			}
		})
	}
}
