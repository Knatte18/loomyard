// awaitbatch_test.go covers AwaitBatch's bounded report-file watch: an already-present report
// returns immediately, a report appearing mid-wait returns the moment a tick sees it (never
// sleeping out the rest of the window), an absent report returns ReportPresent: false only once the
// wait window elapses, and an unknown batch number is refused — all against a scriptable clock, so
// no test ever blocks for real.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// awaitFakeClock is a scriptable clock whose Sleep advances virtual time.
type awaitFakeClock struct {
	now     time.Time
	sleeps  int
	onSleep func(sleepCount int)
}

func (c *awaitFakeClock) Now() time.Time { return c.now }
func (c *awaitFakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
	c.sleeps++
	if c.onSleep != nil {
		c.onSleep(c.sleeps)
	}
}

var _ websterengine.Clock = (*awaitFakeClock)(nil)

// awaitTestBatches returns a minimal one-batch list and its report path.
func awaitTestBatches(dir string) ([]batcher.Batch, string) {
	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
	}
	return batches, filepath.Join(dir, websterengine.ReportFileName(1, "json-flag"))
}

func TestAwaitBatch(t *testing.T) {
	t.Parallel()

	const report = "status: OK\nhead_sha: deadbeef\n"
	tests := []struct {
		name   string
		number int
		window time.Duration
		// seedReport writes the report before the call; reportOnSleep writes it on that sleep tick instead.
		seedReport    bool
		reportOnSleep int
		wantErr       bool
		wantPresent   bool
		// wantSleeps is asserted exactly when wantSleepsSet; wantMinElapsedS when positive.
		wantSleepsSet   bool
		wantSleeps      int
		wantMinElapsedS int
		wantBatchName   string
	}{
		{
			name:          "a report already present returns immediately",
			number:        1,
			window:        time.Minute,
			seedReport:    true,
			wantPresent:   true,
			wantSleepsSet: true,
			wantSleeps:    0,
			wantBatchName: "01-json-flag",
		},
		{
			// The report lands after the third tick — AwaitBatch must return on the very next
			// existence check, long before the one-hour window elapses.
			name:          "a report appearing mid-wait returns without sleeping out the window",
			number:        1,
			window:        time.Hour,
			reportOnSleep: 3,
			wantPresent:   true,
			wantSleepsSet: true,
			wantSleeps:    3,
		},
		{
			name:            "an absent report returns false once the window elapses",
			number:          1,
			window:          5 * time.Second,
			wantMinElapsedS: 5,
		},
		{
			name:    "an unknown batch number is refused",
			number:  7,
			window:  time.Second,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			batches, reportPath := awaitTestBatches(dir)
			if tt.seedReport {
				if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
					t.Fatalf("seed report: %v", err)
				}
			}
			clk := &awaitFakeClock{now: time.Unix(1000, 0)}
			if tt.reportOnSleep > 0 {
				clk.onSleep = func(sleepCount int) {
					if sleepCount == tt.reportOnSleep {
						if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
							t.Fatalf("write report mid-wait: %v", err)
						}
					}
				}
			}

			result, err := websterengine.AwaitBatch(batches, dir, tt.number, tt.window, clk)
			if tt.wantErr {
				if err == nil {
					t.Fatal("AwaitBatch(unknown batch) = nil error; want the findBatch refusal")
				}
				return
			}
			if err != nil {
				t.Fatalf("AwaitBatch() error: %v", err)
			}
			if result.ReportPresent != tt.wantPresent {
				t.Errorf("ReportPresent = %v; want %v", result.ReportPresent, tt.wantPresent)
			}
			if tt.wantBatchName != "" && result.BatchName != tt.wantBatchName {
				t.Errorf("BatchName = %q; want %q", result.BatchName, tt.wantBatchName)
			}
			if tt.wantSleepsSet && clk.sleeps != tt.wantSleeps {
				t.Errorf("clock slept %d time(s); want exactly %d", clk.sleeps, tt.wantSleeps)
			}
			if result.ElapsedS < tt.wantMinElapsedS {
				t.Errorf("ElapsedS = %d; want >= %d (the full window was waited out)", result.ElapsedS, tt.wantMinElapsedS)
			}
		})
	}
}
