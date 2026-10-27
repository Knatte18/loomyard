//go:build tier0

// checkview_test.go pins the adopt check's view in `lym beam status`: what each state shows and which paths its lines name.

package adoptcli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckView(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("hub", "_board", ".lym", "adopt")
	logPath, outPath := filepath.Join(dir, suiteLogName), filepath.Join(dir, checkOutName)
	one, two := 1, 2
	ended := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	done := []CheckTier{{Tier: 0, Outcome: checkGreen}, {Tier: 1, Outcome: checkFailed}}

	tests := []struct {
		name      string
		record    CheckRecord
		alive     bool
		wantState string
		wantLines []string
		// wantRunning is the tier the view holds in flight, nil for none.
		wantRunning *int
	}{
		{
			name:      "RunningAliveShowsTheTierInFlightAndTheTiersDone",
			record:    CheckRecord{State: checkRunning, Running: &two, Tiers: done[:1]},
			alive:     true,
			wantState: "running", wantRunning: &two,
			wantLines: []string{"tier2 running", "done: tier0 green", logPath},
		},
		{
			name:      "GreenShowsItsEndTime",
			record:    CheckRecord{State: checkGreen, EndedAt: ended, Tiers: done[:1]},
			wantState: "green",
			wantLines: []string{"green at 2026-10-27T12:30:00Z", "done: tier0 green", logPath},
		},
		{
			name:      "FailedShowsItsEndTimeAndFailingTier",
			record:    CheckRecord{State: checkFailed, EndedAt: ended, Tiers: done, FailedTier: &one},
			wantState: "failed",
			wantLines: []string{"failed at 2026-10-27T12:30:00Z", "tier1 failed", logPath},
		},
		{
			name:      "RunningReadDeadShowsEndedWithoutAVerdict",
			record:    CheckRecord{State: checkRunning, Running: &one, Tiers: done[:1]},
			wantState: "ended without a verdict",
			wantLines: []string{outPath, logPath, "lym gate test --gate adopt", "next lym adopt with an owner write", "starts a fresh check"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkView(tt.record, tt.alive, dir)

			if got.State != tt.wantState || (got.Running == nil) != (tt.wantRunning == nil) || (tt.wantRunning != nil && *got.Running != *tt.wantRunning) {
				t.Errorf("checkView() state = %q, running = %v; want %q and %v", got.State, got.Running, tt.wantState, tt.wantRunning)
			}
			if got.SuiteLog != logPath || got.CheckOut != outPath {
				t.Errorf("checkView() paths = %q, %q; want %q, %q", got.SuiteLog, got.CheckOut, logPath, outPath)
			}
			text := strings.Join(got.Lines, "\n")
			for _, want := range tt.wantLines {
				if !strings.Contains(text, want) {
					t.Errorf("checkView() lines = %q; want them to hold %q", text, want)
				}
			}
		})
	}
}
