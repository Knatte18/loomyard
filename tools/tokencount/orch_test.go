// orch_test.go drives the orchestrator count and the cost estimate over fixture transcripts: the charge by time, a fork naming one run, the window cut-off, the list prices and an unpriced model.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// timedAssistant is an assistant transcript line at ts with no input and output tokens of usage.
func timedAssistant(id, model, ts string, output int) string {
	return `{"type":"assistant","timestamp":"` + ts + `","message":{"id":"` + id + `","model":"` + model +
		`","usage":{"input_tokens":0,"output_tokens":` + strconv.Itoa(output) + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`
}

// userPrompt is a user transcript line at ts whose content is the text item prompt, the shape of a fork's first prompt.
func userPrompt(t *testing.T, ts, prompt string) string {
	t.Helper()
	return `{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"Fork started"},{"type":"text","text":` + jsonString(t, prompt) + `}]}}`
}

func TestCountOrch(t *testing.T) {
	t.Parallel()
	const model = "claude-opus-5-5"
	projects := t.TempDir()

	// run-a is active 10:00-12:00, run-b 11:00-12:00 and run-c 13:00-14:00, each with one message of 1M output tokens.
	windows := map[string][2]string{"run-a": {"10:00", "12:00"}, "run-b": {"11:00", "12:00"}, "run-c": {"13:00", "14:00"}}
	var runs []RunTally
	for _, slug := range []string{"run-a", "run-b", "run-c"} {
		dir := projectDir(projects, "/hub/"+slug)
		writeLines(t, filepath.Join(dir, "s.jsonl"),
			`{"type":"custom-title","customTitle":"ly:`+slug+`:burler"}`,
			`{"type":"user","timestamp":"2026-01-02T`+windows[slug][0]+`:00Z","message":{"role":"user","content":"go"}}`,
			timedAssistant(slug+"-m", model, "2026-01-02T"+windows[slug][1]+":00Z", 1_000_000),
		)
		run, err := CountRun(dir, slug)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}

	prime := projectDir(projects, "/hub/prime")
	writeLines(t, filepath.Join(prime, "a.jsonl"),
		`{"type":"custom-title","customTitle":"ly:orch"}`,
		// Before the earliest window: cut off.
		timedAssistant("before", model, "2026-01-02T09:00:00Z", 9_000_000),
		// run-a alone in flight.
		timedAssistant("one", model, "2026-01-02T10:30:00Z", 1_000_000),
		// run-a and run-b in flight: split equally.
		timedAssistant("two", model, "2026-01-02T11:30:00Z", 2_000_000),
		// After the latest window: cut off.
		timedAssistant("after", model, "2026-01-02T15:00:00Z", 9_000_000),
	)
	writeLines(t, filepath.Join(prime, "a", "subagents", "agent-b.jsonl"),
		userPrompt(t, "2026-01-02T13:29:00Z", "You are the parent reviewer for run `run-b`, in pair /hub/run-b-old and /hub/run-b."),
		// In run-c's window, yet the fork names run-b alone.
		timedAssistant("fork-b", model, "2026-01-02T13:30:00Z", 4_000_000),
	)
	writeLines(t, filepath.Join(prime, "a", "subagents", "agent-ac.jsonl"),
		userPrompt(t, "2026-01-02T13:29:00Z", "Compare run-a with run-c."),
		// Names two runs, so charged by time: run-c alone is in flight.
		timedAssistant("fork-ac", model, "2026-01-02T13:30:00Z", 5_000_000),
	)
	writeLines(t, filepath.Join(prime, "a", "subagents", "agent-dup.jsonl"),
		userPrompt(t, "2026-01-02T10:29:00Z", "About run-a."),
		// The parent's message repeated in the fork's context.
		timedAssistant("one", model, "2026-01-02T10:30:00Z", 1_000_000),
	)
	writeLines(t, filepath.Join(prime, "b.jsonl"),
		`{"type":"custom-title","customTitle":"lyxhub:orch-2"}`,
		// Inside the span, between windows: no run in flight.
		timedAssistant("none", model, "2026-01-02T12:30:00Z", 3_000_000),
	)
	writeLines(t, filepath.Join(prime, "c.jsonl"),
		`{"type":"custom-title","customTitle":"ly:other"}`,
		timedAssistant("other", model, "2026-01-02T10:30:00Z", 7_000_000),
	)

	orch, err := CountOrch(prime, runs)
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
		{"NoRunInFlight", orch.Unattributed, 3_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.charge.Tokens != tt.want {
				t.Errorf("charge tokens = %v; want %v", tt.charge.Tokens, tt.want)
			}
			// Opus 5.5 output is $20 per million tokens.
			if wantUSD := 20 * tt.want / 1e6; math.Abs(tt.charge.Cost.USD-wantUSD) > 1e-9 {
				t.Errorf("charge cost = %v; want $%.2f", tt.charge.Cost.USD, wantUSD)
			}
		})
	}
	if orch.Main.Sessions != 2 || orch.Main.Messages != 3 || orch.Sub.Sessions != 2 || orch.Sub.Messages != 2 {
		t.Errorf("parts = main %+v, sub %+v; want main 2 sessions 3 messages, sub 2 sessions 2 messages", orch.Main, orch.Sub)
	}

	var buf bytes.Buffer
	if err := (Report{Runs: runs, Orch: &orch}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Orchestrator\n",
		"| orch | 2 | 3 | 6.0M | $120.00 |",
		"| orch+sub | 2 | 2 | 9.0M | $180.00 |",
		"| total | 4 | 5 | 15.0M | $300.00 |",
		"| run-a | 2.0M | $40.00 | $20.00 | 66.7% |",
		"| run-b | 5.0M | $100.00 | $20.00 | 83.3% |",
		"| unattributed | 3.0M | $60.00 | | |",
		orchAttributionRule,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
}

func TestEstimatedCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		model    string
		usage    Usage
		wantUSD  float64
		wantCell string
	}{
		{
			"PricedByCacheLifetime", "claude-opus-5-5",
			Usage{Input: 1e6, CacheCreate: 3e6, CacheCreation: CacheCreation{OneHour: 1e6}, CacheRead: 1e6, Output: 1e6},
			4 + 2*5 + 8 + 0.2 + 20, "$42.20",
		},
		{
			"PromptAtLongPromptThreshold_LowerCard", "claude-haiku-5-5",
			Usage{Input: 10_000, CacheRead: 90_000, Output: 1e6},
			(10_000*0.10 + 90_000*0.01 + 1e6*0.50) / 1e6, "$0.50",
		},
		{
			"PromptOverLongPromptThreshold_HigherCard", "claude-haiku-5-5",
			Usage{Input: 10_000, CacheCreate: 1, CacheRead: 90_000, Output: 1e6},
			(10_000*0.50 + 1*0.625 + 90_000*0.05 + 1e6*2.50) / 1e6, "$2.51",
		},
		{"UnpricedModel", "claude-future-9", Usage{Output: 1}, 0, "unpriced"},
		{"PlaceholderWithoutUsage", "<synthetic>", Usage{}, 0, "$0.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := messageCost(tt.model, tt.usage)
			if math.Abs(got.USD-tt.wantUSD) > 1e-9 || got.String() != tt.wantCell {
				t.Errorf("messageCost(%s) = %v (%s); want %v (%s)", tt.model, got.USD, got, tt.wantUSD, tt.wantCell)
			}
		})
	}

	dir := projectDir(t.TempDir(), "/hub/mixed")
	writeLines(t, filepath.Join(dir, "s.jsonl"),
		`{"type":"custom-title","customTitle":"ly:mixed:burler"}`,
		timedAssistant("priced", "claude-sonnet-5-5", "2026-01-02T10:00:00Z", 1_000_000),
		timedAssistant("unpriced", "claude-future-9", "2026-01-02T10:01:00Z", 1_000_000),
	)
	run, err := CountRun(dir, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| $10.00 + unpriced | claude-future-9: 1, claude-sonnet-5-5: 1 |",
		"Unpriced models (messages), in no cost figure: claude-future-9: 1.",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
}
