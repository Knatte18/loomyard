// watchdogline_test.go covers the CPU watchdog's quit line and the tier timeout line: their rendering and the search a verify gate runs over a log.

package gateslot

import (
	"strings"
	"testing"
	"time"
)

func TestWatchdogLine(t *testing.T) {
	t.Parallel()

	line := WatchdogLine("/tmp/x/a.test", 42, 2*time.Minute)
	tests := []struct {
		name       string
		log        string
		wantWindow string
		wantOK     bool
	}{
		{"the line alone", line, "2m0s", true},
		{"the line amid test output with a SIGQUIT header after it", "=== RUN   TestX\n" + line + "\nSIGQUIT: quit\nPC=0x1 m=0 sigcode=0\n\ngoroutine 1 [running]:\n", "2m0s", true},
		{"test output with no watchdog line", "=== RUN   TestX\n--- PASS: TestX (0.00s)\nPASS\nok  \texample.com/x\t0.01s\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			window, ok := FindWatchdogQuit(tt.log)
			if window != tt.wantWindow || ok != tt.wantOK {
				t.Errorf("FindWatchdogQuit() = %q, %v; want %q, %v", window, ok, tt.wantWindow, tt.wantOK)
			}
		})
	}

	timeoutLine := TierTimeoutLine("tier2", 30*time.Minute)
	timeoutTests := []struct {
		name     string
		log      string
		wantTier string
		wantOK   bool
	}{
		{"the timeout line amid test output", "=== RUN   TestX\n" + timeoutLine + "\nSIGQUIT: quit\n", "tier2", true},
		{"test output with no timeout line", "=== RUN   TestX\n--- PASS: TestX (0.00s)\nPASS\n", "", false},
	}
	for _, tt := range timeoutTests {
		t.Run("TierTimeout_"+tt.name, func(t *testing.T) {
			t.Parallel()

			tier, ok := FindTierTimeout(tt.log)
			if tier != tt.wantTier || ok != tt.wantOK {
				t.Errorf("FindTierTimeout() = %q, %v; want %q, %v", tier, ok, tt.wantTier, tt.wantOK)
			}
		})
	}
	if want := "--set 'tiers.tier2={...}'"; !strings.Contains(timeoutLine, want) {
		t.Errorf("TierTimeoutLine() = %q; want it to name the whole-tier step %q", timeoutLine, want)
	}
}
