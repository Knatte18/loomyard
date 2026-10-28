//go:build tier0

package loomshed

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestVerifyFailureFindings pins how a failed verify result renders: the exit code, the could-not-start cause or the timeout sentence, then the full log path and either the failing tests the log names with the gate's closing line, or the log tail when it names none.
func TestVerifyFailureFindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result verifytree.Result
		// log is the verify log's content, two plain lines when empty.
		log     string
		want    []string
		wantNot []string
		// failures marks a log that names failing tests, so the findings quote those instead of the tail.
		failures bool
	}{
		{
			name:    "a non-zero exit names its code",
			result:  verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3},
			want:    []string{"failed with exit code 3."},
			wantNot: []string{"did not finish", "could not start"},
		},
		{
			name:   "a shell that could not start names the cause",
			result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "exec: sh not found"},
			want:   []string{"exit code -1.", "Its shell could not start: exec: sh not found."},
		},
		{
			name:    "a timeout names the timeout instead of an exit code",
			result:  verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, TimedOut: true, Detail: "gate tier2 exceeded timeout_min 30m0s and was killed"},
			want:    []string{"The verify command timed out: gate tier2 exceeded timeout_min 30m0s and was killed."},
			wantNot: []string{"exit code", "could not start"},
		},
		{
			name:    "a log with the watchdog line names the window and the way forward",
			result:  verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:     "ok  \texample.com/mod/b\t0.1s\n" + gateslot.WatchdogLine("/tmp/x/a.test", 42, 2*time.Minute) + "\nSIGQUIT: quit\nlast line\n",
			want:    []string{"failed with exit code 1.", "2m0s", "busy_quit_sec", `"lym config gate" from the prime`},
			wantNot: []string{"did not finish", "could not start"},
		},
		{
			name:     "failing tests early in a log that ends in passes are quoted with their output above or under the header and the closing line instead of the tail",
			result:   verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:      "=== RUN   TestA\n    a_test.go:9: boom\n--- FAIL: TestA (0.00s)\nFAIL\nFAIL\texample.com/mod/a\t0.1s\n" + "--- FAIL: TestC (0.00s)\n    c_test.go:3: nested\nFAIL\nFAIL\texample.com/mod/c\t0.1s\n" + strings.Repeat("--- PASS: TestB (0.00s)\nok  \texample.com/mod/b\t0.1s\n", 300) + gateslot.FailedRunsPrefix + "tier0 (exit 1); the exit code is tier0's\nlast passing line\n",
			failures: true,
			want:     []string{"Failures:", "example.com/mod/a.TestA:", "a_test.go:9: boom", "example.com/mod/c.TestC:\nc_test.go:3: nested", gateslot.FailedRunsPrefix + "tier0 (exit 1)"},
			wantNot:  []string{"Log tail:", "last passing line", "TestB"},
		},
		{
			name:     "two packages failing a test of the same name each get their own output, never the other's or a closing line",
			result:   verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:      "=== RUN   TestX\n    a_test.go:1: from a\n--- FAIL: TestX (0.00s)\nFAIL\nFAIL\texample.com/mod/a\t0.1s\n" + "ok  \texample.com/mod/b\t0.1s\nstray line\n--- FAIL: TestX (0.00s)\nFAIL\nFAIL\texample.com/mod/c\t0.1s\n",
			failures: true,
			want:     []string{"example.com/mod/a.TestX:\na_test.go:1: from a", "example.com/mod/c.TestX:\nstray line"},
			wantNot:  []string{"ok  \texample.com/mod/b", "FAIL\texample.com/mod/a"},
		},
		{
			name:     "a package repeated in the log gets the output of the block that holds the failing test",
			result:   verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:      "=== RUN   TestOne\n    first\n--- FAIL: TestOne (0.00s)\nFAIL\nFAIL\texample.com/mod/a\t0.1s\n=== RUN   TestTwo\n    second\n--- FAIL: TestTwo (0.00s)\nFAIL\nFAIL\texample.com/mod/a\t0.1s\n",
			failures: true,
			want:     []string{"example.com/mod/a.TestOne:\nfirst", "example.com/mod/a.TestTwo:\nsecond"},
		},
		{
			name:     "an oversized failure is cut on a rune boundary and the failures after it are left to the full log",
			result:   verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:      "=== RUN   TestBig\n" + strings.Repeat("é", 20000) + "\n--- FAIL: TestBig (0.00s)\nFAIL\nFAIL\texample.com/mod/a\t0.1s\n=== RUN   TestSmall\n    small\n--- FAIL: TestSmall (0.00s)\nFAIL\nFAIL\texample.com/mod/b\t0.1s\n",
			failures: true,
			want:     []string{"example.com/mod/a.TestBig:\n" + strings.Repeat("é", 100), verifyFailureTruncated, "More failures are in the full log."},
			wantNot:  []string{"TestSmall", strings.Repeat("é", 5000)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			log := tt.log
			if log == "" {
				log = "first line\nlast line\n"
			}
			logPath := filepath.Join(t.TempDir(), "verify.log")
			if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
				t.Fatal(err)
			}

			got := verifyFailureFindings(tt.result, logPath)

			want := append(tt.want, "Full log: "+logPath)
			if !tt.failures {
				want = append(want, "Log tail:", "last line")
			}
			if !utf8.ValidString(got) {
				t.Errorf("findings are not valid UTF-8: %q", got)
			}
			for _, want := range want {
				if !strings.Contains(got, want) {
					t.Errorf("findings %q lack %q", got, want)
				}
			}
			// Only a log with the watchdog line names the key.
			if !slices.Contains(tt.want, "busy_quit_sec") {
				tt.wantNot = append(tt.wantNot, "busy_quit_sec")
			}
			for _, unwanted := range tt.wantNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("findings %q contain %q", got, unwanted)
				}
			}
		})
	}
}
