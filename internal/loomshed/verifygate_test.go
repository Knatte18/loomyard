package loomshed

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestVerifyFailureFindings pins how a failed verify result renders: the exit code, the could-not-start cause or the timeout sentence, then the full log path and the log tail.
func TestVerifyFailureFindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result verifytree.Result
		// log is the verify log's content, two plain lines when empty.
		log     string
		want    []string
		wantNot []string
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
			result:  verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, TimedOut: true, Detail: "the verify command did not finish within 1h0m0s and was killed"},
			want:    []string{"The verify command did not finish within 1h0m0s and was killed."},
			wantNot: []string{"exit code", "could not start"},
		},
		{
			name:    "a log with the watchdog line names the window and the way forward",
			result:  verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1},
			log:     "ok  \texample.com/mod/b\t0.1s\n" + gateslot.WatchdogLine("/tmp/x/a.test", 42, 2*time.Minute) + "\nSIGQUIT: quit\nlast line\n",
			want:    []string{"failed with exit code 1.", "2m0s", "busy_quit_sec", `"lyx config gate" from the prime`},
			wantNot: []string{"did not finish", "could not start"},
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

			for _, want := range append(tt.want, "Full log: "+logPath, "last line") {
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
