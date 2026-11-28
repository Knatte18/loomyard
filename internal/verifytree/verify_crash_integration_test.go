// verify_crash_integration_test.go covers how Verify reads the gate's runner crash line and tier timeout line.
// It reads them from a failed command's log, over real shell processes.

package verifytree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestVerify_ReadsTheRunnerCrashAndTimeoutLines(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	crashLine := gateslot.RunnerCrashLine("tier1 (exit 134, SIGABRT)")
	timeoutLine := gateslot.TierTimeoutLine("tier2", 30*time.Minute)
	tests := []struct {
		name         string
		logLines     []string
		exit         string
		wantCrash    bool
		wantTimedOut bool
		wantDetail   string
	}{
		{name: "a crash line", logLines: []string{crashLine}, exit: "134", wantCrash: true, wantDetail: "tier1 (exit 134, SIGABRT)"},
		{name: "a timeout line beside a crash line is a timeout", logLines: []string{timeoutLine, crashLine}, exit: "1", wantTimedOut: true, wantDetail: "tier2"},
		{name: "neither line", logLines: []string{"FAIL"}, exit: "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logSource := filepath.Join(t.TempDir(), "gate.log")
			if err := os.WriteFile(logSource, []byte(strings.Join(tt.logLines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			p := newScratch(t)

			res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, "cat '"+logSource+"'; exit "+tt.exit)
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}

			if res.Status != StatusFailed || res.RunnerCrash != tt.wantCrash || res.TimedOut != tt.wantTimedOut {
				t.Errorf("Verify() = status %q, runner crash %v, timed out %v; want %q, %v, %v", res.Status, res.RunnerCrash, res.TimedOut, StatusFailed, tt.wantCrash, tt.wantTimedOut)
			}
			if !strings.Contains(res.Detail, tt.wantDetail) || (tt.wantDetail == "" && res.Detail != "") {
				t.Errorf("Verify() detail = %q; want it to name %q", res.Detail, tt.wantDetail)
			}
		})
	}
}
