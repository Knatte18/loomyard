// bouncer_judge_test.go covers Bouncer.Call's judge mode: the happy paths (CONVERGED and CONTINUE), the unconditional-OutputFiles guard against the conditional-output regression that would make shedengine.Done unreachable, the previous-ledger marker's three cases, every judge-call degradation, harvest, debris handling, and stale-output archival.
// The seed call, the re-bounce, replay, focus synthesis, pointer discipline, and cancellation are
// left to bouncer_seed_test.go (batch 3) and bouncer_replay_test.go (batch 4's own second file).

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// bouncerJudgeFixture is one round's worth of on-disk state a judge-call test lays out before
// calling b.Call.
type bouncerJudgeFixture struct {
	round   int
	report  string // report content for this round; empty means "do not write"
	verdict string // verdict file content for this round; empty means "do not write"
	ledger  string // ledger file content for this round; empty means "do not write"
	focus   string // focus file content for this round; empty means "do not write"
}

// layoutBouncerRun writes cfg.RunDir's on-disk state for rounds 1..len(fixtures), each entry
// describing that round's report, verdict, ledger, and (existing, pre-judge) focus files.
func layoutBouncerRun(t *testing.T, cfg BouncerConfig, fixtures []bouncerJudgeFixture) {
	t.Helper()
	for _, f := range fixtures {
		if f.report != "" {
			path := filepath.Join(cfg.RunDir, cfg.ReportName(f.round))
			if err := os.WriteFile(path, []byte(f.report), 0o644); err != nil {
				t.Fatalf("WriteFile(report round %d) = %v; want nil", f.round, err)
			}
		}
		if f.verdict != "" {
			if err := os.WriteFile(verdictPath(cfg.RunDir, f.round), []byte(f.verdict), 0o644); err != nil {
				t.Fatalf("WriteFile(verdict round %d) = %v; want nil", f.round, err)
			}
		}
		if f.ledger != "" {
			if err := os.WriteFile(ledgerPath(cfg.RunDir, f.round), []byte(f.ledger), 0o644); err != nil {
				t.Fatalf("WriteFile(ledger round %d) = %v; want nil", f.round, err)
			}
		}
		if f.focus != "" {
			if err := os.WriteFile(focusPath(cfg.RunDir, f.round), []byte(f.focus), 0o644); err != nil {
				t.Fatalf("WriteFile(focus round %d) = %v; want nil", f.round, err)
			}
		}
	}
}

// bouncerReport, bouncerVerdictContent, and bouncerLedgerContent are minimal well-formed file
// bodies for the three file contracts, reused across this batch's fixtures.
func bouncerReport(round int) string {
	return fmt.Sprintf("---\nround: %d\n---\nfindings for round %d\n", round, round)
}

func bouncerVerdictContent(verdict string) string {
	return fmt.Sprintf("---\nverdict: %s\nrationale: because reasons\n---\n", verdict)
}

func bouncerLedgerContent(round int) string {
	return fmt.Sprintf("---\nround: %d\nledger: []\n---\nno open findings\n", round)
}

// openGatingLedgerContent is a well-formed ledger for round holding one gating key open since round 1,
// so a CIRCLING verdict over it is earned once round is 2 or later and the other rounds' ledgers carry the same key.
func openGatingLedgerContent(round int) string {
	return fmt.Sprintf("---\nround: %d\nledger:\n  - key: alpha\n    status: open\n    rounds: [1]\n    class: design\n    severity: MEDIUM\n---\nprose\n", round)
}

// judgeFakeShuttle returns a shedfake.Shuttle whose DuringRun writes verdict, ledger, and (unless
// omitted) a next-round focus file to the spec's declared OutputFiles, then reports OutcomeDone.
func judgeFakeShuttle(round int, verdictBody, ledgerBody string, writeFocus bool) *shedfake.Shuttle {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.DuringRun = func() {
		outputs := shuttle.GotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		_ = os.WriteFile(outputs[0], []byte(verdictBody), 0o644)
		_ = os.WriteFile(outputs[1], []byte(ledgerBody), 0o644)
		if writeFocus {
			_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
		}
	}
	return shuttle
}

// TestBouncer_PromptComposition covers what both the seed and the judge call put into the spec they
// spawn: the parent directive, the one skill, and the composed prompt's told specs directory --
// where the rubric is interpolated as a marker VALUE, never run through the fill itself, so the
// {{.specs_dir}} marker inside the rubric value is invisible to a rubric-bytes-only assertion
// and only a check of the composed prompt can catch a literal marker surviving.
func TestBouncer_PromptComposition(t *testing.T) {
	specsDir := t.TempDir()
	if !filepath.IsAbs(specsDir) {
		t.Fatalf("t.TempDir() = %q; want an absolute path", specsDir)
	}
	tests := []struct {
		name       string
		parentName string
		withSpecs  bool
		wantPrompt string
	}{
		{name: "with a parent", parentName: "hub:parent", wantPrompt: "`hub:parent`"},
		{name: "with no parent", wantPrompt: "No parent is recorded"},
		{name: "with a told specs directory", withSpecs: true, wantPrompt: specsDir},
	}
	for _, role := range []string{"seed", "judge"} {
		for _, tt := range tests {
			t.Run(role+" "+tt.name, func(t *testing.T) {
				var shuttle *shedfake.Shuttle
				if role == "judge" {
					shuttle = judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
				} else {
					shuttle = &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
					shuttle.DuringRun = func() {
						path := shuttle.GotSpec.OutputFiles[0]
						content := "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"
						if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
							t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
						}
					}
				}
				opts := []bouncerFixtureOpt{withShuttle(shuttle)}
				if tt.withSpecs {
					opts = append(opts, withSpecsMarker(specsDir))
				}
				fx := newBouncerFixture(t, opts...)
				fx.Config.ParentName = tt.parentName
				b, cfg := fx.Build()
				if role == "judge" {
					layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				}

				shedfake.CallOK(t, b)

				prompt := shuttle.GotSpec.Prompt
				if !strings.Contains(prompt, tt.wantPrompt) {
					t.Errorf("%s prompt does not contain %q", role, tt.wantPrompt)
				}
				if strings.Contains(prompt, "{{.specs_dir}}") {
					t.Errorf("%s prompt contains a literal \"{{.specs_dir}}\" marker; want it rendered", role)
				}
				if got := shuttle.GotSpec.Skills; len(got) != 1 || got[0] != "scribe:prose" {
					t.Errorf("%s spec.Skills = %v; want [scribe:prose]", role, got)
				}
			})
		}
	}
}

// patternMarkerLine is a line of the told PATTERN.md overview that the judge prompt must carry verbatim when the directive is active.
const patternMarkerLine = "- PATTERN-judge-probe: a probe entry"

// writeJudgePattern writes a PATTERN.md holding patternMarkerLine into a fresh worktree root and returns the root.
func writeJudgePattern(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "PATTERN.md"), []byte("# PATTERN\n\n"+patternMarkerLine+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
	}
	return root
}

// TestBouncer_JudgeCall_PatternDirective asserts the judge prompt carries the RoleJudge directive, ahead of the rubric, when PATTERN.md exists at the configured worktree root, and omits it when the file or the root is absent.
func TestBouncer_JudgeCall_PatternDirective(t *testing.T) {
	tests := []struct {
		name string
		root func(t *testing.T) string
		want bool
	}{
		{"PatternAtRoot", writeJudgePattern, true},
		{"RootWithoutPattern", func(t *testing.T) string { return t.TempDir() }, false},
		{"EmptyRoot", func(t *testing.T) string { return "" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
			fx := newBouncerFixture(t, withShuttle(shuttle))
			fx.Config.WorktreeRoot = tt.root(t)
			b, cfg := fx.Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

			shedfake.CallOK(t, b)

			prompt := shuttle.GotSpec.Prompt
			if got := strings.Contains(prompt, patternMarkerLine); got != tt.want {
				t.Errorf("judge prompt contains the PATTERN overview = %v; want %v", got, tt.want)
			}
			if got := strings.Contains(prompt, "## Constraints"); got != tt.want {
				t.Errorf("judge prompt contains the directive heading = %v; want %v", got, tt.want)
			}
			if tt.want && strings.Index(prompt, patternMarkerLine) > strings.Index(prompt, "## Rubric") {
				t.Error("judge prompt carries the directive after the rubric; want it before the judge's first work instruction")
			}
			if strings.Contains(prompt, "{{.") {
				t.Errorf("judge prompt contains an unrendered marker: %q", prompt)
			}
		})
	}
}

// TestBouncer_JudgeCall_PatternDirectiveFailureDegrades asserts an active PATTERN whose directive stencil cannot be read degrades the judge call rather than spawning a judge without the directive.
func TestBouncer_JudgeCall_PatternDirectiveFailureDegrades(t *testing.T) {
	logBuf := logcapture.Capture(t)
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	fx := newBouncerFixture(t, withShuttle(shuttle), withoutStencils("pattern-directive-judge"))
	fx.Config.WorktreeRoot = writeJudgePattern(t)
	b, cfg := fx.Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	assertJudgeDegraded(t, outcome, ptr, err)
	if !strings.Contains(logBuf.String(), "pattern directive unreadable") {
		t.Errorf("Call() log = %q; want the pattern directive degrade message", logBuf.String())
	}
	if shuttle.GotSpec.Prompt != "" {
		t.Error("Call() spawned a judge after a failed directive read; want no spawn")
	}
}

// TestBouncer_JudgeCall_Verdicts covers the judge call's outcome for each verdict it writes: the
// ledger as pointer, the three declared outputs, and the next round's focus file, which is
// declared and written whatever the verdict. A converged judge's focus file carries no targeting.
//
//testtiming:keep pins the judge call's outcome and ledger pointer per verdict, the three declared outputs and the well-formed empty next-round focus file
func TestBouncer_JudgeCall_Verdicts(t *testing.T) {
	tests := []struct {
		name        string
		verdict     string
		wantOutcome shedengine.Outcome
	}{
		{"CONVERGED is Done", "CONVERGED", shedengine.Done},
		{"CONTINUE is Stuck", "CONTINUE", shedengine.Stuck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent(tt.verdict), bouncerLedgerContent(1), true)
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

			ptr := shedfake.RequireOutcome(t, b, tt.wantOutcome)
			wantPointer := ledgerPath(cfg.RunDir, 1)
			if ptr.Path != wantPointer {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
			}

			if len(shuttle.GotSpec.OutputFiles) != 3 {
				t.Errorf("recorded Spec.OutputFiles has %d entries; want 3", len(shuttle.GotSpec.OutputFiles))
			}
			for _, path := range []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1), focusPath(cfg.RunDir, 2)} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("os.Stat(%q) = %v; want nil (all three files exist)", path, err)
				}
			}

			raw, err := os.ReadFile(focusPath(cfg.RunDir, 2))
			if err != nil {
				t.Fatalf("ReadFile(round-2-focus.md) = %v; want nil", err)
			}
			parsed, err := parseFocus(raw)
			if err != nil {
				t.Fatalf("parseFocus(...) error = %v; want nil", err)
			}
			if len(parsed.ExcludeLenses) != 0 {
				t.Errorf("parsed focus ExcludeLenses = %v; want empty", parsed.ExcludeLenses)
			}
			if len(parsed.Focus) != 0 {
				t.Errorf("parsed focus Focus = %v; want empty", parsed.Focus)
			}
		})
	}
}

//testtiming:keep pins the judge prompt's inputs: the facts and review paths, never the artifacts or the fixer report, and the two guard sentences
func TestBouncer_JudgeCall_PromptReadsFactsNotArtifacts(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	shedfake.RequireOutcome(t, b, shedengine.Done)

	prompt := shuttle.GotSpec.Prompt
	for name, want := range map[string]string{
		"facts path":      factsPath(cfg.RunDir, 1),
		"review path":     filepath.Join(cfg.RunDir, cfg.ReportName(1)),
		"previous ledger": "(none)",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("judge prompt does not name the %s %q", name, want)
		}
	}
	for _, artifact := range cfg.ArtifactPaths {
		if strings.Contains(prompt, artifact) {
			t.Errorf("judge prompt names the artifact path %q; want the judge never pointed at the artifacts", artifact)
		}
	}
	if fixer := roundFixerReportPath(cfg.RunDir, 1); strings.Contains(prompt, fixer) {
		t.Errorf("judge prompt names the fixer-report path %q; want it absent", fixer)
	}
	for _, sentence := range []string{
		"A parse-error row for the latest round means `CONTINUE`",
		"Never relabel a BLOCKING finding downward",
	} {
		if !strings.Contains(prompt, sentence) {
			t.Errorf("judge prompt lacks the sentence %q", sentence)
		}
	}
	if _, err := os.Stat(factsPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("os.Stat(facts file) = %v; want nil after a judge call", err)
	}
	for _, out := range shuttle.GotSpec.OutputFiles {
		if out == factsPath(cfg.RunDir, 1) {
			t.Error("the facts file is a declared output file; want it an input only")
		}
	}
}

// TestBouncer_JudgeCall_PreviousLedgerHandling covers the previous ledger the judge prompt names:
// round N's judge reads round N-1's ledger when it parses, and the literal "(none)" when it is
// malformed or absent. The judged round's own report path is named alongside.
func TestBouncer_JudgeCall_PreviousLedgerHandling(t *testing.T) {
	tests := []struct {
		name string
		// round is the round being judged; rounds below it are laid out with a report, and with a
		// CONTINUE verdict and the ledger body in priorLedger.
		round       int
		priorLedger func(round int) string
		// wantLedgerRound is the round whose ledger path the prompt names; zero wants "(none)".
		wantLedgerRound int
	}{
		{
			name:            "a valid prior ledger is named",
			round:           2,
			priorLedger:     bouncerLedgerContent,
			wantLedgerRound: 1,
		},
		{
			name:            "round three names round two's ledger",
			round:           3,
			priorLedger:     bouncerLedgerContent,
			wantLedgerRound: 2,
		},
		{
			name:        "a malformed prior ledger reads as none",
			round:       2,
			priorLedger: func(int) string { return "not frontmatter at all" },
		},
		{
			name:  "no prior ledger reads as none",
			round: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := judgeFakeShuttle(tt.round, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(tt.round), true)
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			var fixtures []bouncerJudgeFixture
			for round := 1; round <= tt.round; round++ {
				fixtures = append(fixtures, bouncerJudgeFixture{round: round, report: bouncerReport(round)})
			}
			layoutBouncerRun(t, cfg, fixtures)
			for round := 1; round < tt.round; round++ {
				if err := os.WriteFile(ledgerPath(cfg.RunDir, round), []byte(tt.priorLedger(round)), 0o644); err != nil {
					t.Fatalf("WriteFile(round-%d ledger) = %v; want nil", round, err)
				}
				if err := os.WriteFile(verdictPath(cfg.RunDir, round), []byte(bouncerVerdictContent("CONTINUE")), 0o644); err != nil {
					t.Fatalf("WriteFile(round-%d verdict) = %v; want nil", round, err)
				}
			}

			shedfake.RequireOutcome(t, b, shedengine.Done)

			prompt := shuttle.GotSpec.Prompt
			if wantReport := filepath.Join(cfg.RunDir, cfg.ReportName(tt.round)); !strings.Contains(prompt, wantReport) {
				t.Errorf("recorded Spec.Prompt does not contain round %d's report path %q", tt.round, wantReport)
			}
			if tt.wantLedgerRound == 0 {
				if !strings.Contains(prompt, "(none)") {
					t.Error("recorded Spec.Prompt does not contain the (none) literal for the previous ledger")
				}
				return
			}
			if want := ledgerPath(cfg.RunDir, tt.wantLedgerRound); !strings.Contains(prompt, want) {
				t.Errorf("recorded Spec.Prompt does not contain round %d's ledger path %q (previous_ledger)", tt.wantLedgerRound, want)
			}
		})
	}
}

// bouncerJudgeTestClock is the fixed instant newBouncerFixture resolves cfg.Now to, so
// archive-sibling filenames are computable rather than discovered by directory scan.
var bouncerJudgeTestClock = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// archivedSiblingPath returns the path archiveStaleOutputs would rename original to when now
// resolves to instant and no same-second collision exists, mirroring archive.go's own naming
// scheme.
func archivedSiblingPath(original string, instant time.Time) string {
	dir := filepath.Dir(original)
	ext := filepath.Ext(original)
	base := strings.TrimSuffix(filepath.Base(original), ext)
	stamp := instant.UTC().Format(archiveTimestampFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, stamp, ext))
}

// assertJudgeDegraded asserts the three properties every judge-call degradation shares: an empty
// pointer, a nil error, and shedengine.Stuck (never shedengine.Done) as the outcome.
func assertJudgeDegraded(t *testing.T, outcome shedengine.Outcome, ptr shedengine.OutputPointer, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome == shedengine.Done {
		t.Fatalf("Call() outcome = %q; want anything but %q on a degraded path", outcome, shedengine.Done)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr.Path != "" || ptr.GateAttempts != nil {
		t.Errorf("Call() pointer = %+v; want empty Path and no GateAttempts", ptr)
	}
	if ptr.Reason == "" {
		t.Errorf("Call() pointer = %+v; want the degrade cause on Reason", ptr)
	}
}

func TestBouncer_JudgeCall_Degradations(t *testing.T) {
	tests := []struct {
		name         string
		buildBouncer func(t *testing.T) (*Bouncer, BouncerConfig)
	}{
		{
			name: "UnreadableReportFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})).Build()
				reportPath := filepath.Join(cfg.RunDir, cfg.ReportName(1))
				if err := os.Mkdir(reportPath, 0o755); err != nil {
					t.Fatalf("Mkdir(report path) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "EmptyReportFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})).Build()
				if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(1)), []byte(""), 0o644); err != nil {
					t.Fatalf("WriteFile(empty report) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "WhitespaceOnlyReportFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: "   \n\t  \n"}})
				return b, cfg
			},
		},
		{
			name: "JudgeTemplateUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t,
					withBareConfig(),
					withoutStencils("bouncer-template-judge"),
					withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}),
					withClock(fixedClock(bouncerJudgeTestClock)),
				).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "RubricUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				if err := os.Remove(filepath.Join(cfg.StencilsDir, "bouncer", "bouncer-template-rubric.md")); err != nil {
					t.Fatalf("Remove(rubric) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "FillFailure",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t,
					withBareConfig(),
					withStencilOverrides(map[string]string{
						// Declares a marker the Go side does not supply.
						"bouncer-template-judge": "# Judge\n\n{{.unknown_marker}}\n",
					}),
					withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}),
					withClock(fixedClock(bouncerJudgeTestClock)),
				).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "RunError",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Err: errors.New("judge run exploded")})).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnreadableVerdictFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
				shuttle.DuringRun = func() {
					outputs := shuttle.GotSpec.OutputFiles
					// A directory in place of the verdict file makes os.ReadFile fail without
					// relying on filesystem permission bits, which are unreliable to flip
					// portably in a test.
					_ = os.Mkdir(outputs[0], 0o755)
					_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
				}
				b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnparseableVerdictFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := judgeFakeShuttle(1, "garbage, not frontmatter", bouncerLedgerContent(1), true)
				b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnparseableLedgerFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), "garbage, not frontmatter", true)
				b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
	}

	// A non-completion outcome that wrote nothing is one the harvest cannot rescue.
	for _, oc := range []shuttleengine.Outcome{shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout} {
		tests = append(tests, struct {
			name         string
			buildBouncer func(t *testing.T) (*Bouncer, BouncerConfig)
		}{
			name: "NonCompletionOutcome_" + string(oc),
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: oc}})).Build()
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuf := logcapture.Capture(t)
			b, _ := tt.buildBouncer(t)

			outcome, ptr, err := b.Call(context.Background())
			assertJudgeDegraded(t, outcome, ptr, err)
			if logBuf.String() == "" {
				t.Error("Call() did not log a warning on a degraded path")
			}
		})
	}
}

//testtiming:keep pins that the harvest is keyed on judged(N): a written verdict settles the round whatever the run reported, and a non-completion with nothing written degrades
func TestBouncer_JudgeCall_Harvest(t *testing.T) {
	t.Run("NonCompletionOutcome_CONTINUE", func(t *testing.T) {
		shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
		shuttle.DuringRun = func() {
			outputs := shuttle.GotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("CONTINUE")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
			// Deliberately does not write the focus file: harvest is keyed on judged(n), not
			// on whether the run reported completion.
		}
		b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
		if _, err := os.Stat(focusPath(cfg.RunDir, 2)); err != nil {
			t.Errorf("os.Stat(round-2-focus.md) = %v; want nil (synthesized on the CONTINUE branch)", err)
		}
	})

	t.Run("RunError_CONVERGED", func(t *testing.T) {
		shuttle := &shedfake.Shuttle{Err: errors.New("run failed after write")}
		shuttle.DuringRun = func() {
			outputs := shuttle.GotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("CONVERGED")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
		}
		b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
	})

	t.Run("Contrast_NothingWrittenDegrades", func(t *testing.T) {
		shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
		b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		assertJudgeDegraded(t, outcome, ptr, err)
	})
}

// TestBouncer_JudgeCall_DebrisIsNotJudged covers stale or half-written judge outputs standing in
// the run dir before the spawn: they are never mistaken for judged(N), they are archived
// byte-identical beside the original on the way in, and the spawn recreates each path.
//
//testtiming:keep pins that stale or half-written judge outputs are archived byte-identical before the spawn and never mistaken for a judged round
func TestBouncer_JudgeCall_DebrisIsNotJudged(t *testing.T) {
	tests := []struct {
		name string
		// debris maps each path standing in the run dir before the call to its content.
		debris func(cfg BouncerConfig) map[string]string
	}{
		{
			name: "VerdictPresentLedgerAbsent",
			debris: func(cfg BouncerConfig) map[string]string {
				return map[string]string{verdictPath(cfg.RunDir, 1): bouncerVerdictContent("CONVERGED")}
			},
		},
		{
			name: "LedgerPresentVerdictAbsent",
			debris: func(cfg BouncerConfig) map[string]string {
				return map[string]string{ledgerPath(cfg.RunDir, 1): bouncerLedgerContent(1)}
			},
		},
		{
			name: "BothPresentVerdictUnparseable",
			debris: func(cfg BouncerConfig) map[string]string {
				return map[string]string{
					verdictPath(cfg.RunDir, 1): "garbage, not frontmatter",
					ledgerPath(cfg.RunDir, 1):  bouncerLedgerContent(1),
				}
			},
		},
		{
			name: "BothPresentLedgerUnparseable",
			debris: func(cfg BouncerConfig) map[string]string {
				return map[string]string{
					verdictPath(cfg.RunDir, 1): bouncerVerdictContent("CONVERGED"),
					ledgerPath(cfg.RunDir, 1):  "garbage, not frontmatter",
				}
			},
		},
		{
			name: "AllThreeOutputsStale",
			debris: func(cfg BouncerConfig) map[string]string {
				return map[string]string{
					verdictPath(cfg.RunDir, 1): "garbage, not frontmatter (stale verdict)",
					ledgerPath(cfg.RunDir, 1):  "garbage, not frontmatter (stale ledger)",
					focusPath(cfg.RunDir, 2):   "garbage, not frontmatter (stale focus)",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			debris := tt.debris(cfg)
			for path, content := range debris {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(debris %q) = %v; want nil", path, err)
				}
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
			if !shuttle.Called {
				t.Error("Call() did not invoke the shuttle seam; debris must not be mistaken for judged(N)")
			}
			if wantPointer := ledgerPath(cfg.RunDir, 1); ptr.Path != wantPointer {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
			}

			for debrisPath, content := range debris {
				archived := archivedSiblingPath(debrisPath, bouncerJudgeTestClock)
				got, err := os.ReadFile(archived)
				if err != nil {
					t.Fatalf("ReadFile(archived sibling %q) = %v; want nil (debris archived on the way in)", archived, err)
				}
				if string(got) != content {
					t.Errorf("archived sibling %q content = %q; want %q (byte-identical to what stood there before)", archived, got, content)
				}
				if _, err := os.Stat(debrisPath); err != nil {
					t.Errorf("os.Stat(%q) = %v; want nil (debris path recreated by the spawn)", debrisPath, err)
				}
			}
		})
	}
}
