// cli_test.go covers the flag refusals and defaults of both report verbs, which come before any cwd resolution or read, the prime-only refusal decision, and the sleep note of the timing verb.

package reportcli

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestReportVerbs_RefuseBadFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		verb string
		args []string
		want []string
	}{
		{"convergence: a since date with no such month", "convergence", []string{"--since", "2026-13-01"}, []string{"--since", "2026-13-01", "YYYY-MM-DD"}},
		{"convergence: a since that is no date", "convergence", []string{"--since", "yesterday"}, []string{"--since", "yesterday", "YYYY-MM-DD"}},
		{"convergence: a fresh-days of zero", "convergence", []string{"--fresh-days", "0"}, []string{"--fresh-days", "0", "at least 1"}},
		{"convergence: a format that is neither json nor markdown", "convergence", []string{"--format", "yaml"}, []string{"--format", "yaml", "json", "markdown"}},
		{"timing: a since date with no such month", "timing", []string{"--since", "2026-13-01"}, []string{"--since", "2026-13-01", "YYYY-MM-DD"}},
		{"timing: a bucket that is neither day nor week", "timing", []string{"--by", "month"}, []string{"--by", "month", "day", "week"}},
		{"timing: a format that is neither json nor markdown", "timing", []string{"--format", "yaml"}, []string{"--format", "yaml", "json", "markdown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A temp directory is no git checkout, so only a refusal that precedes cwd resolution can name the flag.
			var out bytes.Buffer
			code := RunCLIIn(t.TempDir(), &out, append([]string{tt.verb}, tt.args...))

			if code != 1 {
				t.Errorf("exit code = %d; want 1", code)
			}
			for _, text := range tt.want {
				envelope.RequireErr(t, out.String(), text)
			}
		})
	}
}

func TestParseTimingOptions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 15, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		since, by string
		wantSince time.Time
		wantBy    string
	}{
		{name: "no flags open seven days before today's UTC date and bucket by day", by: reportengine.BucketDay, wantSince: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), wantBy: reportengine.BucketDay},
		{name: "a since after today is accepted as that date", since: "2999-01-01", by: reportengine.BucketDay, wantSince: time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC), wantBy: reportengine.BucketDay},
		{name: "a week bucket is kept", by: reportengine.BucketWeek, wantSince: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), wantBy: reportengine.BucketWeek},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTimingOptions(tt.since, tt.by, formatJSON, now)

			if err != nil {
				t.Fatalf("parseTimingOptions error = %v", err)
			}
			if !got.Since.Equal(tt.wantSince) || got.By != tt.wantBy {
				t.Errorf("parseTimingOptions() = since %v by %q; want since %v by %q", got.Since, got.By, tt.wantSince, tt.wantBy)
			}
		})
	}
}

func TestTimingSleepNote(t *testing.T) {
	t.Parallel()

	slept := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	woke := slept.Add(90 * time.Minute)
	journal := fmt.Sprintf(
		`{"__REALTIME_TIMESTAMP":"%d","_BOOT_ID":"boot","MESSAGE":"Performing sleep operation 'suspend'..."}`+"\n"+
			`{"__REALTIME_TIMESTAMP":"%d","_BOOT_ID":"boot","MESSAGE":"System returned from sleep operation 'suspend'."}`+"\n",
		slept.UnixMicro(), woke.UnixMicro())

	tests := []struct {
		name       string
		out        string
		runErr     error
		wantNote   string
		wantSleeps []reportengine.Interval
	}{
		{name: "a journalctl missing from PATH is named", runErr: fmt.Errorf("run: %w", exec.ErrNotFound), wantNote: "No machine sleep is subtracted: journalctl is not on PATH."},
		{name: "a failing journalctl is named", runErr: errors.New("exit status 1"), wantNote: "No machine sleep is subtracted: journalctl failed: exit status 1."},
		{name: "a recorded sleep is converted and left out", out: journal, wantNote: "Machine sleep, read from the systemd journal, is left out of every interval.", wantSleeps: []reportengine.Interval{{From: slept, To: woke}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sleeps, note := sleepsFromJournal([]byte(tt.out), tt.runErr)

			if note != tt.wantNote {
				t.Errorf("sleepsFromJournal() note = %q; want %q", note, tt.wantNote)
			}
			if len(sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleepsFromJournal() sleeps = %v; want %v", sleeps, tt.wantSleeps)
			}
			for i, want := range tt.wantSleeps {
				if !sleeps[i].From.Equal(want.From) || !sleeps[i].To.Equal(want.To) {
					t.Errorf("sleep %d = %v-%v; want %v-%v", i, sleeps[i].From, sleeps[i].To, want.From, want.To)
				}
			}
		})
	}
}

func TestRefuseNonPrime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		verb          string
		worktree      string
		prime         string
		primeNameErr  error
		wantErr       bool
		wantInMessage []string
	}{
		{name: "the prime's own name passes", verb: "timing", worktree: "main", prime: "main"},
		{name: "another name is refused naming the convergence verb and the prime", verb: "convergence", worktree: "task", prime: "main", wantErr: true, wantInMessage: []string{`"task"`, `"main"`, "lyx report convergence", "prime worktree only"}},
		{name: "another name is refused naming the timing verb", verb: "timing", worktree: "task", prime: "main", wantErr: true, wantInMessage: []string{"lyx report timing", "prime worktree only"}},
		{name: "an unresolvable prime name is refused wrapping its cause", verb: "timing", worktree: "main", primeNameErr: errors.New("boom"), wantErr: true, wantInMessage: []string{"boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := refuseNonPrime(tt.verb, tt.worktree, tt.prime, tt.primeNameErr)

			if (err != nil) != tt.wantErr {
				t.Fatalf("refuseNonPrime error = %v; want error %v", err, tt.wantErr)
			}
			for _, text := range tt.wantInMessage {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("error = %q; want it to contain %q", err, text)
				}
			}
			if tt.primeNameErr != nil && !errors.Is(err, tt.primeNameErr) {
				t.Errorf("error = %v; want it to wrap %v", err, tt.primeNameErr)
			}
		})
	}
}
