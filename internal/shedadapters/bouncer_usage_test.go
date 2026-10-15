// bouncer_usage_test.go covers the usage record the Bouncer writes beside the Burler's: one entry per seed or judge pass whose result reached the Call.

package shedadapters

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// requirePassEntry fails t unless entry holds model, the 90-second run and the known 1000-fresh, 5000-cache-read reading, with no fork fields.
func requirePassEntry(t *testing.T, entry halfUsage, model shuttleengine.ModelChoice, started time.Time) {
	t.Helper()
	if entry.Model != model.Model || entry.Effort != model.Effort {
		t.Errorf("entry model, effort = %q, %q; want %q, %q", entry.Model, entry.Effort, model.Model, model.Effort)
	}
	if !entry.Start.Equal(started) || !entry.End.Equal(started.Add(90*time.Second)) {
		t.Errorf("entry start, end = %v, %v; want the pass's times", entry.Start, entry.End)
	}
	if entry.WallSeconds == nil || *entry.WallSeconds != 90 {
		t.Errorf("entry wall seconds = %v; want 90", entry.WallSeconds)
	}
	if !entry.TokensKnown || entry.FreshTokens == nil || *entry.FreshTokens != 1000 || entry.CacheReadTokens == nil || *entry.CacheReadTokens != 5000 {
		t.Errorf("entry tokens = %+v; want known, 1000 fresh and 5000 cache-read", entry)
	}
	if entry.Forks != nil || entry.ForkFreshTokens != nil || entry.ForkCacheReadTokens != nil || entry.ForkFreshShare != nil {
		t.Errorf("entry fork fields = %+v; want none", entry)
	}
}

func TestBouncer_UsageRecord(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	known := shuttleengine.Result{
		Outcome:   shuttleengine.OutcomeDone,
		StartedAt: started,
		EndedAt:   started.Add(90 * time.Second),
		Usage:     shuttleengine.SessionUsage{Known: true, Fresh: 1000, CacheRead: 5000},
	}
	unreadable := known
	unreadable.Usage = shuttleengine.SessionUsage{}
	round1Focus := "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"
	blockingLedger := evidenceLedgerContent(t, 1, evidenceEntry{"alpha", "open", "scope", "BLOCKING"})
	layoutReport := func(t *testing.T, cfg BouncerConfig) {
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
	}
	judgeShuttle := func(verdict string) func() *shedfake.Shuttle {
		return func() *shedfake.Shuttle {
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent(verdict), bouncerLedgerContent(1), true)
			shuttle.Result = known
			return shuttle
		}
	}
	readUsage := func(t *testing.T, cfg BouncerConfig) roundUsage {
		t.Helper()
		u, err := readRoundUsage(cfg.RunDir, 1)
		if err != nil {
			t.Fatalf("readRoundUsage(1) = %v; want nil", err)
		}
		return u
	}

	tests := []struct {
		name         string
		layout       func(t *testing.T, cfg BouncerConfig)
		shuttle      func() *shedfake.Shuttle
		wantOutcomes []shedengine.Outcome
		check        func(t *testing.T, cfg BouncerConfig, ptr shedengine.OutputPointer)
	}{
		{
			name:         "a seed pass writes round 1's seed entry",
			shuttle:      func() *shedfake.Shuttle { return &shedfake.Shuttle{Result: known} },
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				u := readUsage(t, cfg)
				if len(u.Seed) != 1 || len(u.Judge) != 0 || u.Review != nil {
					t.Fatalf("record = %+v; want one seed entry and nothing else", u)
				}
				requirePassEntry(t, u.Seed[0], cfg.Models.Seed, started)
			},
		},
		{
			name: "a judge pass appends to its round's judge list beside the Burler's review half",
			layout: func(t *testing.T, cfg BouncerConfig) {
				layoutReport(t, cfg)
				if err := writeRoundUsage(cfg.RunDir, 1, roundUsage{Review: &halfUsage{Model: "reviewer"}}); err != nil {
					t.Fatalf("writeRoundUsage = %v; want nil", err)
				}
			},
			shuttle:      judgeShuttle("CONTINUE"),
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				u := readUsage(t, cfg)
				if len(u.Judge) != 1 || len(u.Seed) != 0 {
					t.Fatalf("record = %+v; want one judge entry", u)
				}
				requirePassEntry(t, u.Judge[0], cfg.Models.Judge, started)
				if u.Review == nil || u.Review.Model != "reviewer" {
					t.Errorf("review half = %+v; want the Burler's half left in place", u.Review)
				}
			},
		},
		{
			name:   "a refused CONVERGED followed by a re-judge leaves two judge entries",
			layout: layoutReport,
			shuttle: func() *shedfake.Shuttle {
				shuttle := judgeShuttle("CONVERGED")()
				judgments := 0
				shuttle.DuringRun = func() {
					judgments++
					ledger := bouncerLedgerContent(1)
					if judgments == 1 {
						ledger = blockingLedger
					}
					outputs := shuttle.GotSpec.OutputFiles
					_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("CONVERGED")), 0o644)
					_ = os.WriteFile(outputs[1], []byte(ledger), 0o644)
				}
				return shuttle
			},
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck, shedengine.Done},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				if u := readUsage(t, cfg); len(u.Judge) != 2 {
					t.Errorf("judge entries = %d; want 2 (the refused pass and its re-judge)", len(u.Judge))
				}
			},
		},
		{
			name: "a judge harvested by the entry-time attach records one judge entry",
			layout: func(t *testing.T, cfg BouncerConfig) {
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1)}})
			},
			shuttle:      func() *shedfake.Shuttle { return &shedfake.Shuttle{AttachFound: true, AttachResult: known} },
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				u := readUsage(t, cfg)
				if len(u.Judge) != 1 || len(u.Seed) != 0 {
					t.Fatalf("record = %+v; want one judge entry", u)
				}
				requirePassEntry(t, u.Judge[0], cfg.Models.Judge, started)
			},
		},
		{
			name: "a seed attached on the re-bounce records one seed entry",
			layout: func(t *testing.T, cfg BouncerConfig) {
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, focus: round1Focus}})
			},
			shuttle:      func() *shedfake.Shuttle { return &shedfake.Shuttle{AttachFound: true, AttachResult: known} },
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				u := readUsage(t, cfg)
				if len(u.Seed) != 1 || len(u.Judge) != 0 {
					t.Fatalf("record = %+v; want one seed entry", u)
				}
				requirePassEntry(t, u.Seed[0], cfg.Models.Seed, started)
			},
		},
		{
			name:         "a result the provider could not read records unknown tokens",
			shuttle:      func() *shedfake.Shuttle { return &shedfake.Shuttle{Result: unreadable} },
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				u := readUsage(t, cfg)
				if len(u.Seed) != 1 {
					t.Fatalf("seed entries = %d; want 1", len(u.Seed))
				}
				entry := u.Seed[0]
				if entry.TokensKnown || entry.FreshTokens != nil || entry.CacheReadTokens != nil {
					t.Errorf("entry = %+v; want tokens_known false and no token fields", entry)
				}
			},
		},
		{
			name:         "a Run error records nothing",
			shuttle:      func() *shedfake.Shuttle { return &shedfake.Shuttle{Err: errors.New("run failed")} },
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, _ shedengine.OutputPointer) {
				if _, err := os.Stat(roundUsagePath(cfg.RunDir, 1)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("os.Stat(usage record) = %v; want none written", err)
				}
			},
		},
		{
			name: "a record the Bouncer cannot write leaves the outcome and pointer unchanged",
			layout: func(t *testing.T, cfg BouncerConfig) {
				layoutReport(t, cfg)
				if err := os.MkdirAll(roundUsagePath(cfg.RunDir, 1), 0o755); err != nil {
					t.Fatalf("MkdirAll = %v; want nil", err)
				}
			},
			shuttle:      judgeShuttle("CONTINUE"),
			wantOutcomes: []shedengine.Outcome{shedengine.Stuck},
			check: func(t *testing.T, cfg BouncerConfig, ptr shedengine.OutputPointer) {
				if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want || ptr.Reason != "" {
					t.Errorf("pointer = %+v; want path %q and no reason", ptr, want)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, cfg := newBouncerFixture(t, withShuttle(tt.shuttle())).Build()
			if tt.layout != nil {
				tt.layout(t, cfg)
			}

			var ptr shedengine.OutputPointer
			for _, want := range tt.wantOutcomes {
				ptr = shedfake.RequireOutcome(t, b, want)
			}
			tt.check(t, cfg, ptr)
		})
	}
}
