// watchdogline_test.go covers the CPU watchdog's quit line: its rendering and the search a verify gate runs over a log.

package gateslot

import (
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
}
