// entries_harden_test.go covers hardenFixEntry and the harden_round key of the Bouncer and BurlerReview rows.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// campaignRecorder records the seams a test Campaign was reached through.
type campaignRecorder struct {
	reaps    []string
	liveRuns []string
	commits  [][]string
}

// newTestCampaign returns a Campaign over env's worktree and run directory whose seams record into the returned recorder.
// Its live runs pass, and its reaps find nothing.
func newTestCampaign(t *testing.T, env Env) (*hardenshed.Campaign, *campaignRecorder) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(env.WorktreeRoot, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	recorder := &campaignRecorder{}
	return &hardenshed.Campaign{
		Config: hardenshed.Config{
			Packages: []string{"pkg"}, Reviewer: []string{"opus", "sonnet"}, Fixer: []string{"sonnet", "opus"},
			Concurrency: 1, LiveAttempts: 4, VerifyAttempts: 5,
		},
		WorktreeRoot: env.WorktreeRoot,
		AnchorPath:   env.AnchorPath,
		RunDir:       filepath.Join(env.RunRoot, "review-segment"),
		RunLive: func(key string, _ int, _ []string) (hardenshed.LiveRun, error) {
			recorder.liveRuns = append(recorder.liveRuns, key)
			return hardenshed.LiveRun{Passed: true, Output: gateslot.FormatStartedLine("tier2", 1) + "\n"}, nil
		},
		Reap: func(row string, point hardenshed.ReapPoint) ([]hardenshed.StrayEntry, error) {
			recorder.reaps = append(recorder.reaps, row+":"+string(point))
			return nil, nil
		},
		Head:    func() (string, error) { return "head0", nil },
		Commits: func(string) ([]string, error) { return []string{"c1"}, nil },
		Commit: func(relPaths []string, _ string) error {
			recorder.commits = append(recorder.commits, relPaths)
			return nil
		},
	}, recorder
}

// hardenTestEnv returns newTestEnv's Env with a test campaign set.
func hardenTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.Harden, _ = newTestCampaign(t, env)
	return env
}

// captureHardenFixGates builds a HardenFix row from cfg and env and drives one Call over an accepted round 1.
// It returns the gate spec env.Burler's fake recorded for the fix.
func captureHardenFixGates(t *testing.T, cfg Config, env Env) shuttleengine.GateSpec {
	t.Helper()
	producer, err := hardenFixEntry(reviewRow("fix-row"), cfg, env)
	if err != nil {
		t.Fatalf("hardenFixEntry() error = %v; want nil", err)
	}
	writeFixRound(t, filepath.Join(env.RunRoot, cfg["run_subdir"].(string)))
	_, _, _ = producer.Call(context.Background())

	fake := env.Burler.(*shedfake.BurlerHalfRunner)
	if len(fake.GotFixOpts) != 1 {
		t.Fatalf("BurlerHalfRunner recorded %d RunOpts for Fix; want 1", len(fake.GotFixOpts))
	}
	return fake.GotFixOpts[0].Gate
}

func TestHardenFixEntry(t *testing.T) {
	tiergate.Require(t, 0)

	t.Run("GatesAreCleanVerifyAndLiveWithTheCampaignsAttempts", func(t *testing.T) {
		env := newTestEnv(t)
		var recorder *campaignRecorder
		env.Harden, recorder = newTestCampaign(t, env)
		env.DarnVerifyAttempts = 9

		gate := captureHardenFixGates(t, minimalFixConfig(), env)

		var names []string
		var attempts []int
		var passOnCap []bool
		for _, entry := range gate {
			names = append(names, entry.Name)
			attempts = append(attempts, entry.Attempts)
			passOnCap = append(passOnCap, entry.PassOnCap)
		}
		if want := []string{"clean", "verify", "live"}; !slices.Equal(names, want) {
			t.Errorf("gate names = %v; want %v", names, want)
		}
		if want := []int{3, 5, 4}; !slices.Equal(attempts, want) {
			t.Errorf("gate attempts = %v; want %v, the verify attempts being the campaign's and not DarnVerifyAttempts", attempts, want)
		}
		if want := []bool{false, false, true}; !slices.Equal(passOnCap, want) {
			t.Errorf("gate PassOnCap = %v; want %v", passOnCap, want)
		}
		if want := []string{"fix-row:start", "fix-row:end"}; !slices.Equal(recorder.reaps, want) {
			t.Errorf("reaps = %v; want the row wrapped in the campaign's reaps %v", recorder.reaps, want)
		}

		result, err := gate[2].Gate()
		if err != nil || !result.Passed {
			t.Fatalf("live gate = %+v, %v; want a pass", result, err)
		}
		if want := []string{hardenshed.LiveGateKey}; !slices.Equal(recorder.liveRuns, want) {
			t.Errorf("live runs = %v; want %v", recorder.liveRuns, want)
		}
		report, found, err := hardenshed.ReadLiveReport(filepath.Join(env.RunRoot, "review-segment"), 1)
		if err != nil || !found || report.Mark != hardenshed.MarkGreen {
			t.Errorf("live report of round 1 = %+v, %v, %v; want a green one, the round the review row last wrote", report, found, err)
		}
	})

	refusals := []struct {
		name string
		edit func(cfg Config, env *Env)
		want []string
	}{
		{"GatesKey", func(cfg Config, _ *Env) { cfg["gates"] = gatesCfg("verify", 3)["gates"] }, []string{"gates are fixed to clean, verify and live", "way forward: remove the key from the recipe row"}},
		{"GateKeyKey", func(cfg Config, _ *Env) { cfg["gate_key"] = "custom" }, []string{"gates are fixed to clean, verify and live", "way forward: remove the key from the recipe row"}},
		{"NonSourceFixScope", func(cfg Config, _ *Env) { cfg["profile"].(map[string]any)["fix-scope"] = "overlay" }, []string{"fix-scope", "way forward"}},
		{"NilCampaign", func(_ Config, env *Env) { env.Harden = nil }, []string{"Env.Harden", "way forward"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name+"IsRefused", func(t *testing.T) {
			env := hardenTestEnv(t)
			cfg := minimalFixConfig()
			tt.edit(cfg, &env)
			_, err := hardenFixEntry(reviewRow("fix-row"), cfg, env)
			for _, want := range tt.want {
				assertErrContains(t, err, want)
			}
		})
	}
}

func TestHardenRoundKey(t *testing.T) {
	tiergate.Require(t, 0)

	t.Run("ReviewRowWithoutACampaignIsRefused", func(t *testing.T) {
		cfg := minimalReviewConfig()
		cfg["harden_round"] = true
		_, err := burlerReviewEntry(reviewRow("review-row"), cfg, newTestEnv(t))
		assertErrContains(t, err, "Env.Harden")
	})

	t.Run("ReviewRowReapsAtTheStartAndTheEndAndRecordsTheRoundModels", func(t *testing.T) {
		env := newTestEnv(t)
		var recorder *campaignRecorder
		env.Harden, recorder = newTestCampaign(t, env)
		cfg := minimalReviewConfig()
		cfg["harden_round"] = true

		producer, err := burlerReviewEntry(reviewRow("review-row"), cfg, env)
		if err != nil {
			t.Fatalf("burlerReviewEntry() error = %v; want nil", err)
		}
		_, _, _ = producer.Call(context.Background())

		if want := []string{"review-row:start", "review-row:end"}; !slices.Equal(recorder.reaps, want) {
			t.Errorf("reaps = %v; want %v", recorder.reaps, want)
		}
		models, err := os.ReadFile(filepath.Join(env.RunRoot, "review-segment", "harden-models-1.json"))
		if err != nil || !strings.Contains(string(models), `"review": "opus"`) || !strings.Contains(string(models), `"fix": "sonnet"`) {
			t.Errorf("models file = %q, %v; want the first entries of the campaign's lists", models, err)
		}
	})

	t.Run("BouncerRowWithoutACampaignIsRefused", func(t *testing.T) {
		env := newTestEnv(t)
		cfg := minimalBouncerConfig(t, env)
		cfg["harden_round"] = true
		_, err := bouncerEntry(reviewRow("review-bounce"), cfg, env)
		assertErrContains(t, err, "Env.Harden")
	})

	t.Run("BouncerRowSeedsFromTheNamedStencilFilledWithTheCampaignsBrief", func(t *testing.T) {
		env := newTestEnv(t)
		env.Harden, _ = newTestCampaign(t, env)
		writeStencil(t, env.StencilsDir, "harden-seed-stub", "{{.seed_brief}} {{.focus_path}}\n")
		cfg := minimalBouncerConfig(t, env)
		cfg["harden_round"] = true
		cfg["seed_stencil"] = "harden-seed-stub"

		producer, err := bouncerEntry(reviewRow("review-bounce"), cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		fake := env.Shuttle.(*shedfake.Shuttle)
		if len(fake.Specs) != 1 {
			t.Fatalf("len(fake.Specs) = %d; want 1, the seed spawn from the named stencil", len(fake.Specs))
		}
		brief, err := env.Harden.SeedBrief()
		if err != nil {
			t.Fatalf("SeedBrief() error = %v; want nil", err)
		}
		if !strings.Contains(fake.Specs[0].Prompt, brief) {
			t.Errorf("seed prompt = %q; want it to hold the campaign's brief %q", fake.Specs[0].Prompt, brief)
		}
	})

	t.Run("BouncerRowCarriesTheCampaignsFactsInItsFactsFile", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = judgeSeamShuttle()
		env.Harden, _ = newTestCampaign(t, env)
		writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
		cfg := minimalBouncerConfig(t, env)
		cfg["harden_round"] = true
		layoutBouncerRound1Report(t, env)

		producer, err := bouncerEntry(reviewRow("review-bounce"), cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		facts, err := os.ReadFile(filepath.Join(env.RunRoot, "review-segment", "round-1-facts.md"))
		if err != nil || !strings.Contains(string(facts), "## Campaign facts") || !strings.Contains(string(facts), "- Models: absent.") {
			t.Errorf("facts file = %q, %v; want the campaign's section", facts, err)
		}
	})

	t.Run("BouncerRowAutoAcceptStopsAtTheCampaignsRedLiveReport", func(t *testing.T) {
		tests := []struct {
			name        string
			liveReport  string
			wantOutcome shedengine.Outcome
			wantReason  string
		}{
			{name: "NoLiveReportAcceptsAtASpentBudget", wantOutcome: shedengine.Done},
			{name: "RedLiveReportEscalatesBudget", liveReport: `{"round":2,"mark":"red"}`, wantOutcome: shedengine.Awaiting, wantReason: "cause: budget"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				env := newTestEnv(t)
				env.Harden, _ = newTestCampaign(t, env)
				env.Setting = fakeSetting(map[string]any{"hardener.auto_accept": true})
				env.SegmentBounces = func(string) (int, int, bool, error) { return 5, 5, true, nil }
				env.Shuttle = continueSeamShuttle()
				writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
				cfg := minimalBouncerConfig(t, env)
				cfg["harden_round"] = true
				cfg["auto_accept_setting"] = "hardener.auto_accept"
				layoutBouncerCirclingRounds(t, env)
				if tt.liveReport != "" {
					if err := os.WriteFile(filepath.Join(env.RunRoot, "review-segment", "harden-live-2.json"), []byte(tt.liveReport), 0o644); err != nil {
						t.Fatal(err)
					}
				}

				producer, err := bouncerEntry(reviewRow("review-bounce"), cfg, env)
				if err != nil {
					t.Fatalf("bouncerEntry() error = %v; want nil", err)
				}
				outcome, pointer, err := producer.Call(context.Background())
				if err != nil {
					t.Fatalf("Call() error = %v; want nil", err)
				}
				if outcome != tt.wantOutcome || !strings.Contains(pointer.Reason, tt.wantReason) {
					t.Errorf("Call() = %v, Reason %q; want %v with %q", outcome, pointer.Reason, tt.wantOutcome, tt.wantReason)
				}
			})
		}
	})

	t.Run("BouncerRowReapsTheSandboxAtTheHaltPointWhenTheRunHalts", func(t *testing.T) {
		env := newTestEnv(t)
		var recorder *campaignRecorder
		env.Harden, recorder = newTestCampaign(t, env)
		env.Shuttle = circlingSeamShuttle()
		env.ReviewCirclingCheckpoint = 2
		writeStencil(t, env.StencilsDir, "bouncer-template-judge", "judge template, no markers\n")
		cfg := minimalBouncerConfig(t, env)
		cfg["harden_round"] = true
		layoutBouncerCirclingRounds(t, env)

		producer, err := bouncerEntry(reviewRow("review-bounce"), cfg, env)
		if err != nil {
			t.Fatalf("bouncerEntry() error = %v; want nil", err)
		}
		outcome, _, err := producer.Call(context.Background())
		if err != nil || outcome != shedengine.Awaiting {
			t.Fatalf("Call() = %v, %v; want an Awaiting circling halt", outcome, err)
		}
		if want := []string{"review-bounce:" + string(hardenshed.ReapHalt)}; !slices.Equal(recorder.reaps, want) {
			t.Errorf("reaps = %v; want %v, the campaign's reap at the halt point", recorder.reaps, want)
		}
	})
}

// continueSeamShuttle returns a shedfake.Shuttle whose Run writes round 2's CONTINUE verdict.
// The verdict goes to the spec's declared OutputFiles.
// It also writes the open ledger that layoutBouncerCirclingRounds expects.
// A bouncerEntry-built producer then settles on a judged CONTINUE within one Call.
func continueSeamShuttle() *shedfake.Shuttle {
	return &shedfake.Shuttle{
		RunFn: func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
			if len(spec.OutputFiles) == 3 {
				_ = os.WriteFile(spec.OutputFiles[0], []byte("---\nverdict: CONTINUE\nrationale: \"findings remain\"\n---\n"), 0o644)
				_ = os.WriteFile(spec.OutputFiles[1], []byte(circlingOpenLedger(2)), 0o644)
			}
			return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
		},
	}
}
