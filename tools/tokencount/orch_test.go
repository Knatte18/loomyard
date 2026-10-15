// orch_test.go drives the orchestrator count and the cost estimate over fake reads: the charge by time, a fork naming one run, the window cut-off, the orch rows of the role tables, the section order, the fake engine's prices and an unpriced model.
// The prices are the fake pricer's, not any provider's.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// timedAssistant is an assistant entry at ts whose usage is output tokens alone.
func timedAssistant(id, model, ts string, output int) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryAssistant, MessageKey: id, Model: model,
		Usage: &shuttleengine.MessageUsage{Output: output},
	}
}

// userPrompt is a user entry at ts holding the text prompt beside a tool result, the shape of a fork's first prompt.
func userPrompt(ts, prompt string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryUser, UserTexts: []string{prompt},
		ToolResults: []shuttleengine.ToolResult{{CallID: "x", Text: "Fork started"}},
	}
}

func TestCountOrch(t *testing.T) {
	t.Parallel()
	// model-a is priced at $20 per million tokens by the fake.
	const model = "model-a"
	fake := newFakeTranscripts()

	// run-a is active 10:00-12:00, run-b 11:00-12:00 and run-c 13:00-14:00, each with one message of 1M output tokens;
	// each window is over the stall cap, so the run's time is all idle.
	windows := map[string][2]string{"run-a": {"10:00", "12:00"}, "run-b": {"11:00", "12:00"}, "run-c": {"13:00", "14:00"}}
	for _, slug := range []string{"run-a", "run-b", "run-c"} {
		fake.add(filepath.Join("/hub", slug), time.Time{}, sessionOf(readOf("s.jsonl", "ly:"+slug+":burler",
			userAt("2026-01-02T"+windows[slug][0]+":00Z", "go"),
			timedAssistant(slug+"-m", model, "2026-01-02T"+windows[slug][1]+":00Z", 1_000_000),
		)))
	}
	var runs []RunTally
	for _, slug := range []string{"run-a", "run-b", "run-c"} {
		run, err := CountRun(fakeReadSet(fake), filepath.Join("/hub", slug), slug, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}

	const prime = "/hub/prime"
	fake.add(prime, time.Time{}, sessionOf(
		readOf("a.jsonl", "ly:orch",
			// Before the earliest window: cut off.
			timedAssistant("before", model, "2026-01-02T09:00:00Z", 9_000_000),
			// run-a alone in flight.
			timedAssistant("one", model, "2026-01-02T10:30:00Z", 1_000_000),
			// run-a and run-b in flight: split equally.
			timedAssistant("two", model, "2026-01-02T11:30:00Z", 2_000_000),
			// After the latest window: cut off.
			timedAssistant("after", model, "2026-01-02T15:00:00Z", 9_000_000),
			// A message the engine collapsed to its final record is judged at that record's time, between windows.
			timedAssistant("straddle", model, "2026-01-02T12:30:00Z", 500_000),
		),
		readOf("agent-ac.jsonl", "",
			userPrompt("2026-01-02T13:29:00Z", "Compare run-a with run-c."),
			// Names two runs, so charged by time: run-c alone is in flight.
			timedAssistant("fork-ac", model, "2026-01-02T13:30:00Z", 5_000_000),
		),
		readOf("agent-b.jsonl", "",
			userPrompt("2026-01-02T13:29:00Z", "You are the parent reviewer for run `run-b`, in pair /hub/run-b-old and /hub/run-b."),
			// In run-c's window, yet the fork names run-b alone.
			timedAssistant("fork-b", model, "2026-01-02T13:30:00Z", 4_000_000),
		),
		readOf("agent-dup.jsonl", "",
			userPrompt("2026-01-02T10:29:00Z", "About run-a."),
			// The parent's message repeated in the fork's context.
			timedAssistant("one", model, "2026-01-02T10:30:00Z", 1_000_000),
		),
	))
	fake.add(prime, time.Time{}, sessionOf(readOf("b.jsonl", "lyxhub:orch-2",
		// Inside the span, between windows: no run in flight.
		timedAssistant("none", model, "2026-01-02T12:30:00Z", 3_000_000),
	)))
	fake.add(prime, time.Time{}, sessionOf(readOf("c.jsonl", "ly:other",
		timedAssistant("other", model, "2026-01-02T10:30:00Z", 7_000_000),
	)))

	orch, err := CountOrch(fakeReadSet(fake), prime, runs)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		charge OrchCharge
		want   float64
	}{
		{"OneRunInFlight_PlusHalfOfTwo", *orch.Charges["run-a"], 2_000_000},
		{"HalfOfTwo_PlusForkNamingIt", *orch.Charges["run-b"], 5_000_000},
		{"ForkNamingTwoRuns_ChargedByTime", *orch.Charges["run-c"], 5_000_000},
		{"NoRunInFlight_PlusCollapsedMessage", orch.Unattributed, 3_500_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.charge.Tokens != tt.want {
				t.Errorf("charge tokens = %v; want %v", tt.charge.Tokens, tt.want)
			}
			if tt.charge.Weight != 5*tt.want {
				t.Errorf("charge weight = %v; want %v, five per output token", tt.charge.Weight, 5*tt.want)
			}
			// The fake prices model-a at $20 per million tokens.
			if wantUSD := 20 * tt.want / 1e6; math.Abs(tt.charge.Cost.USD-wantUSD) > 1e-9 {
				t.Errorf("charge cost = %v; want $%.2f", tt.charge.Cost.USD, wantUSD)
			}
		})
	}
	if orch.Main.Sessions != 2 || orch.Main.Messages != 4 || orch.Sub.Sessions != 2 || orch.Sub.Messages != 2 {
		t.Errorf("parts = main %+v, sub %+v; want main 2 sessions 4 messages, sub 2 sessions 2 messages", orch.Main, orch.Sub)
	}

	var buf bytes.Buffer
	if err := (Report{Runs: runs, Orch: &orch}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	report := buf.String()
	// The orch section sits between the All runs table and the first run's table.
	all, orchSection, firstRun := strings.Index(report, "## All runs\n"), strings.Index(report, "## Orchestrator\n"), strings.Index(report, "## run-a\n")
	if all < 0 || orchSection < all || firstRun < orchSection {
		t.Errorf("sections at All runs %d, Orchestrator %d, run-a %d; want them in that order:\n%s", all, orchSection, firstRun, report)
	}
	// Each output token weighs 5, so the runs' burler rows weigh 5M each and the orch rows five times their charged tokens;
	// the shares divide by a total that holds the orch row.
	for _, want := range []string{
		"| burler | 3 | 3000000 | 0 | 0 | 0 | 15.0M | 20.0% | 0s | 0.0% | $60.00 | model-a: 3 |\n| idle | | | | | | | | 4h0m0s | 100.0% | | |\n| orch | | | | | | 60.0M | 80.0% | | | $240.00 | |\n",
		"Total weight 75.0M;",
		"## run-a\n\n| role |",
		"| burler | 1 | 1000000 | 0 | 0 | 0 | 5.0M | 33.3% | 0s | 0.0% | $20.00 | model-a: 1 |\n| idle | | | | | | | | 2h0m0s | 100.0% | | |\n| orch | | | | | | 10.0M | 66.7% | | | $40.00 | |\n",
		"| orch | 2 | 4 | 6.5M | $130.00 |",
		"| orch+sub | 2 | 2 | 9.0M | $180.00 |",
		"| total | 4 | 6 | 15.5M | $310.00 |",
		"| run-a | 2.0M | $40.00 | $20.00 | 66.7% |",
		"| run-b | 5.0M | $100.00 | $20.00 | 83.3% |",
		"| unattributed | 3.5M | $70.00 | | |",
		orchAttributionRule,
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestEstimatedCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pricer   shuttleengine.MessagePricer
		model    string
		usage    shuttleengine.MessageUsage
		wantUSD  float64
		wantCell string
	}{
		{"PricedByTheEngine", fakeRates, "model-a", shuttleengine.MessageUsage{Input: 1e6, CacheWrite: 1e6, CacheRead: 1e6, Output: 1e6}, 80, "$80.00"},
		{"ModelWithoutPrice", fakeRates, "model-x", shuttleengine.MessageUsage{Output: 1}, 0, "unpriced"},
		{"EngineWithoutPricing", nil, "model-a", shuttleengine.MessageUsage{Output: 1}, 0, "unpriced"},
		{"PlaceholderWithoutUsage", nil, "<synthetic>", shuttleengine.MessageUsage{}, 0, "$0.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := messageCost(tt.pricer, tt.model, tt.usage)
			if math.Abs(got.USD-tt.wantUSD) > 1e-9 || got.String() != tt.wantCell {
				t.Errorf("messageCost(%s) = %v (%s); want %v (%s)", tt.model, got.USD, got, tt.wantUSD, tt.wantCell)
			}
		})
	}

	const worktree = "/hub/mixed"
	fake := newFakeTranscripts()
	fake.add(worktree, time.Time{}, sessionOf(readOf("s.jsonl", "ly:mixed:burler",
		timedAssistant("priced", "model-b", "2026-01-02T10:00:00Z", 1_000_000),
		timedAssistant("unpriced", "model-x", "2026-01-02T10:01:00Z", 1_000_000),
	)))
	run, err := CountRun(fakeReadSet(fake), worktree, "mixed", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| $10.00 + unpriced | model-b: 1, model-x: 1 |",
		"Unpriced models (messages), in no cost figure: model-x: 1.",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}

	// An engine without pricing makes every message with usage unpriced.
	unpriced := ReadSet{fakeEngine: {Transcripts: fake}}
	run, err = CountRun(unpriced, worktree, "mixed", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	if want := "| unpriced | model-b: 1, model-x: 1 |"; !strings.Contains(buf.String(), want) {
		t.Errorf("report with no pricing lacks %q:\n%s", want, buf.String())
	}
}
