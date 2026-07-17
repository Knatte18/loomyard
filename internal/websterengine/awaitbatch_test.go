// awaitbatch_test.go covers AwaitBatch's bounded report-file watch: an
// already-present report returns immediately, a report appearing mid-wait
// returns the moment a tick sees it (never sleeping out the rest of the
// window), an absent report returns ReportPresent: false only once the wait
// window elapses, and an unknown batch number is refused — all against a
// scriptable clock, so no test ever blocks for real.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// awaitFakeClock is a scriptable clock whose Sleep advances virtual time and
// invokes an optional per-sleep hook — the hook is how a test materializes
// the report file "while" AwaitBatch is mid-wait.
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

// awaitTestPlan returns a minimal one-batch plan and the batch's report path
// inside dir.
func awaitTestPlan(dir string) (*builderengine.Plan, string) {
	plan := &builderengine.Plan{
		Batches: []builderengine.PlanBatch{
			{Number: 1, Slug: "json-flag", File: "01-json-flag.md"},
		},
	}
	return plan, filepath.Join(dir, builderengine.BatchReportFileName(1, "json-flag"))
}

func TestAwaitBatch_ReportAlreadyPresentReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	plan, reportPath := awaitTestPlan(dir)
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\n"), 0o644); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	clk := &awaitFakeClock{now: time.Unix(1000, 0)}
	result, err := websterengine.AwaitBatch(plan, dir, 1, time.Minute, clk)
	if err != nil {
		t.Fatalf("AwaitBatch() error: %v", err)
	}
	if !result.ReportPresent {
		t.Error("ReportPresent = false; want true for a pre-existing report")
	}
	if result.BatchName != "01-json-flag" {
		t.Errorf("BatchName = %q; want %q", result.BatchName, "01-json-flag")
	}
	if clk.sleeps != 0 {
		t.Errorf("clock slept %d time(s); want 0 for a pre-existing report", clk.sleeps)
	}
}

func TestAwaitBatch_ReportAppearingMidWaitReturnsWithoutSleepingOutWindow(t *testing.T) {
	dir := t.TempDir()
	plan, reportPath := awaitTestPlan(dir)

	// The report lands after the third tick — AwaitBatch must return on the
	// very next existence check, long before the one-hour window elapses.
	clk := &awaitFakeClock{now: time.Unix(1000, 0)}
	clk.onSleep = func(sleepCount int) {
		if sleepCount == 3 {
			if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\n"), 0o644); err != nil {
				t.Fatalf("write report mid-wait: %v", err)
			}
		}
	}

	result, err := websterengine.AwaitBatch(plan, dir, 1, time.Hour, clk)
	if err != nil {
		t.Fatalf("AwaitBatch() error: %v", err)
	}
	if !result.ReportPresent {
		t.Error("ReportPresent = false; want true once the report appeared mid-wait")
	}
	if clk.sleeps != 3 {
		t.Errorf("clock slept %d time(s); want exactly 3 (return on the tick that saw the report)", clk.sleeps)
	}
}

func TestAwaitBatch_AbsentReportReturnsFalseOnceWindowElapses(t *testing.T) {
	dir := t.TempDir()
	plan, _ := awaitTestPlan(dir)

	clk := &awaitFakeClock{now: time.Unix(1000, 0)}
	result, err := websterengine.AwaitBatch(plan, dir, 1, 5*time.Second, clk)
	if err != nil {
		t.Fatalf("AwaitBatch() error: %v", err)
	}
	if result.ReportPresent {
		t.Error("ReportPresent = true; want false when no report ever lands")
	}
	if result.ElapsedS < 5 {
		t.Errorf("ElapsedS = %d; want >= 5 (the full window was waited out)", result.ElapsedS)
	}
}

func TestAwaitBatch_UnknownBatchNumberRefused(t *testing.T) {
	dir := t.TempDir()
	plan, _ := awaitTestPlan(dir)

	if _, err := websterengine.AwaitBatch(plan, dir, 7, time.Second, &awaitFakeClock{now: time.Unix(1000, 0)}); err == nil {
		t.Fatal("AwaitBatch(unknown batch) = nil error; want the findBatch refusal")
	}
}
