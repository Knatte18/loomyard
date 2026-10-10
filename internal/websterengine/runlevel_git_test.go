//go:build integration

// runlevel_git_test.go exercises Run's plan-level verify gate over a real scratch git repository and real shell commands:
// the gate asks git for the tree's dirty paths and for the commits since the first failure, and runs the plan's verify command in a subprocess.
// Every other Run behavior is tested over a fakeGit in runlevel_test.go.

package websterengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// gateScenario is the one scratch repository the verify-gate steps share.
// Its base commit holds base.txt alone.
type gateScenario struct {
	repo string
	base string
}

func newGateScenario(t *testing.T) *gateScenario {
	t.Helper()
	repo := newScratchRepo(t)
	base := gitkit.CommitFile(t, repo, "base.txt", "base", "base commit")
	return &gateScenario{repo: repo, base: base}
}

// restart puts the repository back on its base commit with a clean tree, and returns a one-card Run fixture over it whose plan carries verify.
// The fixture's Git is nil, so webster asks the repository itself.
// It adds one card commit touching internal/batch1 under a go.mod module, and returns that commit too; the worktree is clean afterwards, so the gate's own verify is the only thing that can fail.
func (s *gateScenario) restart(t *testing.T, verify string) (*runFixture, string) {
	t.Helper()
	gitkit.Git(t, s.repo, "reset", "--hard", s.base)
	gitkit.Git(t, s.repo, "clean", "-fdx")
	fx := newRunFixtureOver(t, 1, s.repo, nil)
	appendIntegrationVerify(t, fx.PlanDir, verify)
	gitkit.CommitFile(t, fx.Worktree, "go.mod", "module example.com/m\n", "go.mod")
	sha := gitkit.CommitFile(t, fx.Worktree, "internal/batch1/a.go", "package batch1\n", "card 1")
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha}},
		},
	})
	return fx, sha
}

// verifyGateOf returns the verify entry Run handed StartMaster.
func verifyGateOf(t *testing.T, fx *runFixture) shuttleengine.GateEntry {
	t.Helper()
	for _, e := range fx.Starter.gateCalls[0] {
		if e.Name == "verify" {
			return e
		}
	}
	t.Fatalf("StartMaster's gate %+v has no verify entry", fx.Starter.gateCalls[0])
	return shuttleengine.GateEntry{}
}

// TestRun_VerifyGate runs every verify-gate case over one scratch repository, one step per case.
// Each step starts by restarting the repository from its base commit, so no step relies on another's state.
func TestRun_VerifyGate(t *testing.T) {
	t.Parallel()
	s := newGateScenario(t)

	// A fake Merriam whose first done arrival fails verify and whose second passes ends done with one re-prompt,
	// and the findings name the failing identity and the card whose commit touched its package.
	t.Run("a failure then a pass ends done", func(t *testing.T) {
		okFile := filepath.Join(t.TempDir(), "ok")
		fx, sha := s.restart(t, "[ -f "+okFile+" ] || { printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1; }")

		var findings string
		var reprompts int
		fx.Starter.handle = &runFakeHandle{
			strandGUID: "master-strand-gatepass",
			result: shuttleengine.Result{
				Outcome:   shuttleengine.OutcomeDone,
				SessionID: "master-session-gatepass",
				RunDir:    "/run/dir/gatepass",
				ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
			},
			onWait: func() {
				writeDoneContract(t, fx)
				entry := verifyGateOf(t, fx)
				first, err := entry.Gate()
				if err != nil {
					t.Fatalf("first gate evaluation error = %v; want nil", err)
				}
				if first.Passed {
					t.Fatalf("first gate evaluation passed; want a verify failure")
				}
				reprompts++
				findings = first.Findings
				fixPromptPath, err := filepath.Abs(filepath.Join(fx.Deps.Geom.PromptsDir, "verify-fix.md"))
				if err != nil {
					t.Fatalf("resolve verify-fix prompt path: %v", err)
				}
				if entry.Reprompt == nil {
					t.Fatalf("verify entry has no Reprompt; want the renderer naming the fixer-fork way forward")
				}
				const findingsPath = "/scratch/gate-findings.md"
				line := entry.Reprompt(findingsPath)
				if strings.ContainsAny(line, "\n\r") || !strings.HasPrefix(line, "Gate findings recorded at "+findingsPath) || !strings.Contains(line, fixPromptPath) {
					t.Errorf("re-prompt line = %q; want one line opening with the findings path and naming %s", line, fixPromptPath)
				}
				// Merriam's fixer makes its fix.
				if err := os.WriteFile(okFile, nil, 0o644); err != nil {
					t.Fatalf("write ok file: %v", err)
				}
				second, err := entry.Gate()
				if err != nil || !second.Passed {
					t.Fatalf("second gate evaluation = %+v, %v; want a pass", second, err)
				}
			},
		}
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-gatepass", "master-session-gatepass")

		result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if result.Outcome != "done" {
			t.Errorf("Outcome = %q; want done", result.Outcome)
		}
		if reprompts != 1 {
			t.Errorf("re-prompts = %d; want 1", reprompts)
		}
		if !strings.Contains(findings, "example.com/m/internal/batch1") {
			t.Errorf("findings = %q; want the failing identity", findings)
		}
		if !strings.Contains(findings, "01-batch1") {
			t.Errorf("findings = %q; want the card whose commit %s touched the failing package", findings, sha)
		}
		if _, err := os.Stat(websterengine.VerifyGateReportPath(fx.Deps.Geom.ReportsDir)); err != nil {
			t.Errorf("verify-gate report: %v; want one written by the failed evaluation", err)
		}
	})

	// A gate that never passes ends the run stuck with a reason naming the failing identities.
	t.Run("a gate that never passes ends stuck", func(t *testing.T) {
		fx, _ := s.restart(t, "printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1")

		fx.Starter.handle = &runFakeHandle{
			strandGUID: "master-strand-gatestuck",
			result: shuttleengine.Result{
				Outcome:   shuttleengine.OutcomeDone,
				SessionID: "master-session-gatestuck",
				RunDir:    "/run/dir/gatestuck",
				ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
				Gate:      &shuttleengine.GateOutcome{Passed: false, Attempts: 3},
			},
			onWait: func() {
				writeDoneContract(t, fx)
				if res, err := verifyGateOf(t, fx).Gate(); err != nil || res.Passed {
					t.Fatalf("gate evaluation = %+v, %v; want a verify failure", res, err)
				}
			},
		}
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-gatestuck", "master-session-gatestuck")

		result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if result.Outcome != "stuck" {
			t.Errorf("Outcome = %q; want stuck", result.Outcome)
		}
		if !strings.Contains(result.StuckReason, "example.com/m/internal/batch1") {
			t.Errorf("StuckReason = %q; want the failing identity", result.StuckReason)
		}
	})

	// A verify failure that passes on rerun keeps the run done and surfaces the flaky warning and summary section.
	t.Run("a flaky verify keeps done with a warning", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "runs")
		// The first run records itself and fails.
		// The rerun sees the record and passes.
		fx, _ := s.restart(t, "if [ -f "+counter+" ]; then exit 0; fi; : > "+counter+"; printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1")

		fx.Starter.handle = &runFakeHandle{
			strandGUID: "master-strand-flaky",
			result: shuttleengine.Result{
				Outcome:   shuttleengine.OutcomeDone,
				SessionID: "master-session-flaky",
				RunDir:    "/run/dir/flaky",
				ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
				Gate:      &shuttleengine.GateOutcome{Passed: true},
			},
			onWait: func() {
				writeDoneContract(t, fx)
				if res, err := verifyGateOf(t, fx).Gate(); err != nil || !res.Passed {
					t.Fatalf("gate evaluation = %+v, %v; want a pass on rerun", res, err)
				}
			},
		}
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-flaky", "master-session-flaky")

		result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if result.Outcome != "done" {
			t.Errorf("Outcome = %q; want done", result.Outcome)
		}
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "flaky") {
			t.Errorf("Warnings = %v; want exactly the flaky warning", result.Warnings)
		}
		summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
		if err != nil {
			t.Fatalf("read summary.md: %v", err)
		}
		if !strings.Contains(string(summary), "example.com/m/internal/batch1") {
			t.Errorf("summary.md = %q; want the flaky identity in its triage section", summary)
		}
	})
}
