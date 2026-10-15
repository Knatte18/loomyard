// sleepjournal_test.go pins how journal entries become sleep intervals and which clause Read gives for each way a run can leave no sleep.

package sleepjournal

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	// entry is one journalctl -o json line at minutes past base in boot, holding message as a JSON string.
	entry := func(minutes int, boot, message string) string {
		return `{"__REALTIME_TIMESTAMP":"` + strconv.FormatInt(at(minutes).UnixMicro(), 10) + `","_BOOT_ID":"` + boot + `","MESSAGE":` + strconv.Quote(message) + `}`
	}
	enter := func(minutes int, boot, operation string) string {
		return entry(minutes, boot, "Performing sleep operation '"+operation+"'...")
	}
	leave := func(minutes int, boot, operation string) string {
		return entry(minutes, boot, "System returned from sleep operation '"+operation+"'.")
	}

	tests := []struct {
		name    string
		lines   []string
		want    []Interval
		wantErr string
	}{
		{
			name:  "a hibernate and a suspend amid other messages",
			lines: []string{entry(0, "b1", "Successfully froze unit 'user.slice'."), enter(1, "b1", "hibernate"), leave(600, "b1", "hibernate"), entry(600, "b1", "Successfully thawed unit 'user.slice'."), enter(700, "b1", "suspend"), leave(710, "b1", "suspend")},
			want:  []Interval{{From: at(1), To: at(600)}, {From: at(700), To: at(710)}},
		},
		{name: "a start with no return yields to the next start", lines: []string{enter(0, "b1", "hibernate"), enter(5, "b1", "hibernate"), leave(9, "b1", "hibernate")}, want: []Interval{{From: at(5), To: at(9)}}},
		{name: "a return with no start", lines: []string{leave(9, "b1", "suspend")}},
		{name: "a pair split across boots", lines: []string{enter(0, "b1", "hibernate"), leave(9, "b2", "hibernate")}},
		{name: "a return from another operation", lines: []string{enter(0, "b1", "suspend"), leave(9, "b1", "hibernate")}},
		{name: "a byte-array message is skipped", lines: []string{enter(0, "b1", "suspend"), `{"__REALTIME_TIMESTAMP":"1","_BOOT_ID":"b1","MESSAGE":[104,105]}`, leave(9, "b1", "suspend")}, want: []Interval{{From: at(0), To: at(9)}}},
		{name: "a line that does not decode", lines: []string{"{"}, wantErr: "does not decode"},
		{name: "a timestamp that does not parse", lines: []string{`{"__REALTIME_TIMESTAMP":"soon","MESSAGE":"x"}`}, wantErr: "does not parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(strings.Join(tt.lines, "\n") + "\n"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse error = %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, tt.want, func(a, b Interval) bool { return a.From.Equal(b.From) && a.To.Equal(b.To) }) {
				t.Errorf("Parse = %v; want %v", got, tt.want)
			}
		})
	}

	// Read gives the sleeps with an empty clause, or no sleeps and the clause saying why.
	reads := []struct {
		name       string
		out        []byte
		runErr     error
		wantClause string
		wantLen    int
	}{
		{name: "journalctl absent", runErr: fmt.Errorf("start: %w", exec.ErrNotFound), wantClause: "journalctl is not on PATH"},
		{name: "a failing journalctl", runErr: errors.New("exit status 1"), wantClause: "journalctl failed: exit status 1"},
		{name: "a line that does not decode", out: []byte("{\n"), wantClause: "journal line \"{\" does not decode: unexpected end of JSON input"},
		{name: "an empty journal", wantClause: "the journal records no systemd-sleep operation"},
		{name: "a recorded sleep", out: []byte(enter(1, "b1", "hibernate") + "\n" + leave(9, "b1", "hibernate") + "\n"), wantLen: 1},
	}
	for _, tt := range reads {
		t.Run("read "+tt.name, func(t *testing.T) {
			t.Parallel()
			sleeps, clause := Read(tt.out, tt.runErr)
			if clause != tt.wantClause || len(sleeps) != tt.wantLen {
				t.Errorf("Read = %d sleeps, %q; want %d sleeps, %q", len(sleeps), clause, tt.wantLen, tt.wantClause)
			}
		})
	}
}
