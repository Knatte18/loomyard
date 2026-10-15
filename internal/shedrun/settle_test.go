// settle_test.go pins the tolerant decoding of the Go driver's settle-records file.

package shedrun_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedrun"
)

func TestParseSettleRecords(t *testing.T) {
	t.Parallel()
	const (
		first  = `{"stop_key":{"current_producer":"Plan-Write","history_length":2,"state":"blocked"},"settler":"go-rule","action":"restep","outcome":"ok","at":"2026-01-02T03:04:05Z"}`
		second = `{"stop_key":{"current_producer":"Publish","history_length":5,"state":"blocked"},"settler":"go-rule","action":"verify-retry","outcome":"spent","at":"2026-01-02T03:05:05Z","build":"abc123","target":"Publish"}`
		newer  = `{"stop_key":{"current_producer":"B","history_length":1,"state":"failed"},"settler":"tackler","action":"spawn","outcome":"ok","at":"2026-01-02T03:04:05Z","from_a_newer_binary":true}`
		torn   = `{"stop_key":{"current_prod`
	)
	firstWant := shedrun.SettleRecord{
		StopKey: shedrun.StopKey{CurrentProducer: "Plan-Write", HistoryLength: 2, State: "blocked"},
		Settler: shedrun.SettlerGoRule,
		Action:  shedrun.SettleRestep,
		Outcome: "ok",
		At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	secondWant := shedrun.SettleRecord{
		StopKey: shedrun.StopKey{CurrentProducer: "Publish", HistoryLength: 5, State: "blocked"},
		Settler: shedrun.SettlerGoRule,
		Action:  shedrun.SettleVerifyRetry,
		Outcome: "spent",
		At:      time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC),
		Build:   "abc123",
		Target:  "Publish",
	}

	tests := []struct {
		name         string
		lines        []string
		want         []shedrun.SettleRecord
		wantSkipLine []int
	}{
		{name: "two records in order", lines: []string{first, second}, want: []shedrun.SettleRecord{firstWant, secondWant}},
		{name: "a blank line is skipped", lines: []string{first, "", "  ", second}, want: []shedrun.SettleRecord{firstWant, secondWant}},
		{name: "an unknown key decodes", lines: []string{newer}, want: []shedrun.SettleRecord{{
			StopKey: shedrun.StopKey{CurrentProducer: "B", HistoryLength: 1, State: "failed"},
			Settler: shedrun.SettlerTackler,
			Action:  shedrun.SettleSpawn,
			Outcome: "ok",
			At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}}},
		{name: "a torn line is skipped with its line number", lines: []string{first, second, torn, newer}, want: []shedrun.SettleRecord{firstWant, secondWant, {
			StopKey: shedrun.StopKey{CurrentProducer: "B", HistoryLength: 1, State: "failed"},
			Settler: shedrun.SettlerTackler,
			Action:  shedrun.SettleSpawn,
			Outcome: "ok",
			At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}}, wantSkipLine: []int{3}},
		{name: "no bytes", lines: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, skipped := shedrun.ParseSettleRecords([]byte(strings.Join(tt.lines, "\n")))
			if len(got) != len(tt.want) {
				t.Fatalf("ParseSettleRecords = %d records %+v; want %d", len(got), got, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("record %d = %+v; want %+v", i, got[i], tt.want[i])
				}
			}
			if len(skipped) != len(tt.wantSkipLine) {
				t.Fatalf("ParseSettleRecords skipped %v; want %d errors", skipped, len(tt.wantSkipLine))
			}
			for i, line := range tt.wantSkipLine {
				if want := fmt.Sprintf("line %d ", line); !strings.Contains(skipped[i].Error(), want) {
					t.Errorf("skipped error %d = %q; want it to contain %q", i, skipped[i], want)
				}
			}
		})
	}
}
