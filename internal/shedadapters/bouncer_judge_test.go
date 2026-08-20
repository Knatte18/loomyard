// bouncer_judge_test.go covers Bouncer.Call's judge mode: the happy paths (APPROVED and BLOCKING),
// the unconditional-OutputFiles guard against the conditional-output regression that would make
// shedengine.Done unreachable, the previous-ledger marker's three cases, every judge-call
// degradation, harvest, debris handling, and stale-output archival.
// The seed call, the re-bounce, replay, focus synthesis, pointer discipline, and cancellation are
// left to bouncer_seed_test.go (batch 3) and bouncer_replay_test.go (batch 4's own second file).

package shedadapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// judgeFakeShuttle returns a fakeShuttle whose duringRun writes verdict, ledger, and (unless
// omitted) a next-round focus file to the spec's declared OutputFiles, then reports OutcomeDone.
func judgeFakeShuttle(round int, verdictBody, ledgerBody string, writeFocus bool) *fakeShuttle {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.duringRun = func() {
		outputs := shuttle.gotSpec.OutputFiles
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

func TestBouncer_JudgeCall_Approved(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if _, err := os.Stat(ptr.Path); err != nil {
		t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
	}

	if len(shuttle.gotSpec.OutputFiles) != 3 {
		t.Errorf("recorded Spec.OutputFiles has %d entries; want 3", len(shuttle.gotSpec.OutputFiles))
	}

	nextFocus := focusPath(cfg.RunDir, 2)
	raw, err := os.ReadFile(nextFocus)
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
}

func TestBouncer_JudgeCall_Blocking(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("BLOCKING"), bouncerLedgerContent(1), true)
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}

	if len(shuttle.gotSpec.OutputFiles) != 3 {
		t.Errorf("recorded Spec.OutputFiles has %d entries; want 3", len(shuttle.gotSpec.OutputFiles))
	}

	for _, path := range []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1), focusPath(cfg.RunDir, 2)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("os.Stat(%q) = %v; want nil (all three files exist)", path, err)
		}
	}
}

func TestBouncer_JudgeCall_RoundThree_UsesRoundTwoLedger(t *testing.T) {
	shuttle := judgeFakeShuttle(3, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(3), true)
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1)},
		{round: 2, report: bouncerReport(2)},
		{round: 3, report: bouncerReport(3)},
	})
	// Ledgers for rounds 1 and 2 already exist on disk (a fully judged round 1 and round 2).
	if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
	}
	if err := os.WriteFile(ledgerPath(cfg.RunDir, 2), []byte(bouncerLedgerContent(2)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-2 ledger) = %v; want nil", err)
	}
	if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
		t.Fatalf("WriteFile(round-1 verdict) = %v; want nil", err)
	}
	if err := os.WriteFile(verdictPath(cfg.RunDir, 2), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
		t.Fatalf("WriteFile(round-2 verdict) = %v; want nil", err)
	}

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}

	wantReport := filepath.Join(cfg.RunDir, cfg.ReportName(3))
	if !strings.Contains(shuttle.gotSpec.Prompt, wantReport) {
		t.Errorf("recorded Spec.Prompt does not contain round 3's report path %q", wantReport)
	}
	wantPrevLedger := ledgerPath(cfg.RunDir, 2)
	if !strings.Contains(shuttle.gotSpec.Prompt, wantPrevLedger) {
		t.Errorf("recorded Spec.Prompt does not contain round 2's ledger path %q (previous_ledger)", wantPrevLedger)
	}
}

func TestBouncer_JudgeCall_PreviousLedgerHandling(t *testing.T) {
	t.Run("ValidPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(2, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(2), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
			{round: 1, report: bouncerReport(1)},
			{round: 2, report: bouncerReport(2)},
		})
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
		}
		if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 verdict) = %v; want nil", err)
		}

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantPrevLedger := ledgerPath(cfg.RunDir, 1)
		if !strings.Contains(shuttle.gotSpec.Prompt, wantPrevLedger) {
			t.Errorf("recorded Spec.Prompt does not contain the valid prior ledger's absolute path %q", wantPrevLedger)
		}
	})

	t.Run("MalformedPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(2, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(2), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
			{round: 1, report: bouncerReport(1)},
			{round: 2, report: bouncerReport(2)},
		})
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte("not frontmatter at all"), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
		}

		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q (the judge still runs)", outcome, shedengine.Done)
		}
		if !strings.Contains(shuttle.gotSpec.Prompt, "(none)") {
			t.Error("recorded Spec.Prompt does not contain the (none) literal for a malformed prior ledger")
		}
	})

	t.Run("NoPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if !strings.Contains(shuttle.gotSpec.Prompt, "(none)") {
			t.Error("recorded Spec.Prompt does not contain the (none) literal when no prior ledger exists")
		}
	})
}

// bouncerJudgeTestClock is the fixed instant newTestBouncer resolves cfg.Now to, reused here so
// archive-sibling filenames are computable rather than discovered by directory scan.
var bouncerJudgeTestClock = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// captureBouncerWarnings redirects logger's stderr sink into a buffer for the duration of t,
// restoring os.Stderr via t.Cleanup, matching the pattern internal/treadleengine/engine_test.go
// already establishes for asserting a Warn was logged.
func captureBouncerWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	return &buf
}

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
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
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
				b, cfg := newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
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
				b, cfg := newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
				if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(1)), []byte(""), 0o644); err != nil {
					t.Fatalf("WriteFile(empty report) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "WhitespaceOnlyReportFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: "   \n\t  \n"}})
				return b, cfg
			},
		},
		{
			name: "JudgeTemplateUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				runDir := t.TempDir()
				stencilsDir := newBouncerStencilsFixture(t, map[string]string{
					"bouncer-template-seed":   "# Seed\n\n{{.rubric}} {{.artifacts}} {{.round}} {{.focus_path}}\n",
					"bouncer-template-rubric": "# Rubric\n\nBe thorough.\n",
					// bouncer-template-judge deliberately absent.
				})
				cfg := BouncerConfig{
					Name:          "gate",
					RunDir:        runDir,
					ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
					ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
					StencilsDir:   stencilsDir,
					RubricStencil: "bouncer-template-rubric",
					Shuttle:       &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
					Now:           fixedClock(bouncerJudgeTestClock),
				}
				b, err := NewBouncer(cfg)
				if err != nil {
					t.Fatalf("NewBouncer(...) error = %v; want nil", err)
				}
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "RubricUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
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
				runDir := t.TempDir()
				stencilsDir := newBouncerStencilsFixture(t, map[string]string{
					"bouncer-template-seed": "# Seed\n\n{{.rubric}} {{.artifacts}} {{.round}} {{.focus_path}}\n",
					// Declares a marker the Go side does not supply.
					"bouncer-template-judge":  "# Judge\n\n{{.unknown_marker}}\n",
					"bouncer-template-rubric": "# Rubric\n\nBe thorough.\n",
				})
				cfg := BouncerConfig{
					Name:          "gate",
					RunDir:        runDir,
					ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
					ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
					StencilsDir:   stencilsDir,
					RubricStencil: "bouncer-template-rubric",
					Shuttle:       &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
					Now:           fixedClock(bouncerJudgeTestClock),
				}
				b, err := NewBouncer(cfg)
				if err != nil {
					t.Fatalf("NewBouncer(...) error = %v; want nil", err)
				}
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "RunError",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				b, cfg := newTestBouncer(t, &fakeShuttle{err: errors.New("judge run exploded")})
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnreadableVerdictFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
				shuttle.duringRun = func() {
					outputs := shuttle.gotSpec.OutputFiles
					// A directory in place of the verdict file makes os.ReadFile fail without
					// relying on filesystem permission bits, which are unreliable to flip
					// portably in a test.
					_ = os.Mkdir(outputs[0], 0o755)
					_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
				}
				b, cfg := newTestBouncer(t, shuttle)
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnparseableVerdictFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := judgeFakeShuttle(1, "garbage, not frontmatter", bouncerLedgerContent(1), true)
				b, cfg := newTestBouncer(t, shuttle)
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
		{
			name: "UnparseableLedgerFile",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), "garbage, not frontmatter", true)
				b, cfg := newTestBouncer(t, shuttle)
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
				return b, cfg
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuf := captureBouncerWarnings(t)
			b, _ := tt.buildBouncer(t)

			outcome, ptr, err := b.Call(context.Background())
			assertJudgeDegraded(t, outcome, ptr, err)
			if logBuf.Len() == 0 {
				t.Error("Call() did not log a warning on a degraded path")
			}
		})
	}
}

func TestBouncer_JudgeCall_NonCompletionOutcomesHarvestCannotRescue(t *testing.T) {
	outcomes := []shuttleengine.Outcome{
		shuttleengine.OutcomeAsking,
		shuttleengine.OutcomeDied,
		shuttleengine.OutcomeTimeout,
	}
	for _, oc := range outcomes {
		t.Run(string(oc), func(t *testing.T) {
			logBuf := captureBouncerWarnings(t)
			shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: oc}}
			b, cfg := newTestBouncer(t, shuttle)
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

			outcome, ptr, err := b.Call(context.Background())
			assertJudgeDegraded(t, outcome, ptr, err)
			if logBuf.Len() == 0 {
				t.Error("Call() did not log a warning on a degraded path")
			}
		})
	}
}

func TestBouncer_JudgeCall_Harvest(t *testing.T) {
	t.Run("NonCompletionOutcome_BLOCKING", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}}
		shuttle.duringRun = func() {
			outputs := shuttle.gotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("BLOCKING")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
			// Deliberately does not write the focus file: harvest is keyed on judged(n), not
			// on whether the run reported completion.
		}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
		if _, err := os.Stat(focusPath(cfg.RunDir, 2)); err != nil {
			t.Errorf("os.Stat(round-2-focus.md) = %v; want nil (synthesized on the BLOCKING branch)", err)
		}
	})

	t.Run("RunError_APPROVED", func(t *testing.T) {
		shuttle := &fakeShuttle{err: errors.New("run failed after write")}
		shuttle.duringRun = func() {
			outputs := shuttle.gotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("APPROVED")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
		}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
	})

	t.Run("Contrast_NothingWrittenDegrades", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		assertJudgeDegraded(t, outcome, ptr, err)
	})
}

func TestBouncer_JudgeCall_DebrisIsNotJudged(t *testing.T) {
	tests := []struct {
		name          string
		layoutDebris  func(t *testing.T, cfg BouncerConfig)
		archivedPaths func(cfg BouncerConfig) []string
	}{
		{
			name: "VerdictPresentLedgerAbsent",
			layoutDebris: func(t *testing.T, cfg BouncerConfig) {
				if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644); err != nil {
					t.Fatalf("WriteFile(debris verdict) = %v; want nil", err)
				}
			},
			archivedPaths: func(cfg BouncerConfig) []string {
				return []string{verdictPath(cfg.RunDir, 1)}
			},
		},
		{
			name: "LedgerPresentVerdictAbsent",
			layoutDebris: func(t *testing.T, cfg BouncerConfig) {
				if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
					t.Fatalf("WriteFile(debris ledger) = %v; want nil", err)
				}
			},
			archivedPaths: func(cfg BouncerConfig) []string {
				return []string{ledgerPath(cfg.RunDir, 1)}
			},
		},
		{
			name: "BothPresentVerdictUnparseable",
			layoutDebris: func(t *testing.T, cfg BouncerConfig) {
				if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte("garbage, not frontmatter"), 0o644); err != nil {
					t.Fatalf("WriteFile(debris verdict) = %v; want nil", err)
				}
				if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
					t.Fatalf("WriteFile(debris ledger) = %v; want nil", err)
				}
			},
			archivedPaths: func(cfg BouncerConfig) []string {
				return []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1)}
			},
		},
		{
			name: "BothPresentLedgerUnparseable",
			layoutDebris: func(t *testing.T, cfg BouncerConfig) {
				if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644); err != nil {
					t.Fatalf("WriteFile(debris verdict) = %v; want nil", err)
				}
				if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte("garbage, not frontmatter"), 0o644); err != nil {
					t.Fatalf("WriteFile(debris ledger) = %v; want nil", err)
				}
			},
			archivedPaths: func(cfg BouncerConfig) []string {
				return []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1)}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
			b, cfg := newTestBouncer(t, shuttle)
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			tt.layoutDebris(t, cfg)

			if _, _, err := b.Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if !shuttle.called {
				t.Error("Call() did not invoke the shuttle seam; debris must not be mistaken for judged(N)")
			}

			for _, debrisPath := range tt.archivedPaths(cfg) {
				archived := archivedSiblingPath(debrisPath, bouncerJudgeTestClock)
				if _, err := os.Stat(archived); err != nil {
					t.Errorf("os.Stat(archived sibling %q) = %v; want nil (debris archived on the way in)", archived, err)
				}
				if _, err := os.Stat(debrisPath); err != nil {
					t.Errorf("os.Stat(%q) = %v; want nil (debris path recreated by the spawn)", debrisPath, err)
				}
			}
		})
	}
}

func TestBouncer_JudgeCall_StaleOutputsArchivedBeforeSpawn(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	staleVerdict := "garbage, not frontmatter (stale verdict)"
	staleLedger := "garbage, not frontmatter (stale ledger)"
	staleFocus := "garbage, not frontmatter (stale focus)"
	if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(staleVerdict), 0o644); err != nil {
		t.Fatalf("WriteFile(stale verdict) = %v; want nil", err)
	}
	if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(staleLedger), 0o644); err != nil {
		t.Fatalf("WriteFile(stale ledger) = %v; want nil", err)
	}
	if err := os.WriteFile(focusPath(cfg.RunDir, 2), []byte(staleFocus), 0o644); err != nil {
		t.Fatalf("WriteFile(stale focus) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q (the spawn must succeed rather than trip over stale outputs)", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}

	tests := []struct {
		original string
		want     string
	}{
		{verdictPath(cfg.RunDir, 1), staleVerdict},
		{ledgerPath(cfg.RunDir, 1), staleLedger},
		{focusPath(cfg.RunDir, 2), staleFocus},
	}
	for _, tt := range tests {
		archived := archivedSiblingPath(tt.original, bouncerJudgeTestClock)
		got, err := os.ReadFile(archived)
		if err != nil {
			t.Fatalf("ReadFile(archived sibling %q) = %v; want nil", archived, err)
		}
		if string(got) != tt.want {
			t.Errorf("archived sibling %q content = %q; want %q (byte-identical to what stood there before)", archived, got, tt.want)
		}
	}
}
