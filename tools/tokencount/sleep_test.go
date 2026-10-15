package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestSleepsFromJournal(t *testing.T) {
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

	// The header line names why a sleep is or is not left out.
	notes := []struct {
		name     string
		out      []byte
		runErr   error
		wantNote string
		want     []Interval
	}{
		{name: "journalctl absent", runErr: fmt.Errorf("start: %w", exec.ErrNotFound), wantNote: "No machine sleep is left out of the time figures: journalctl is not on PATH.\n"},
		{name: "a failing journalctl", runErr: errors.New("exit status 1"), wantNote: "No machine sleep is left out of the time figures: journalctl failed: exit status 1.\n"},
		{name: "an empty journal", wantNote: "No machine sleep is left out of the time figures: the journal records no systemd-sleep operation.\n"},
		{name: "a line that does not decode", out: []byte("{\n"), wantNote: "No machine sleep is left out of the time figures: journal line \"{\" does not decode: unexpected end of JSON input.\n"},
		{name: "a recorded sleep", out: []byte(enter(1, "b1", "hibernate") + "\n" + leave(9, "b1", "hibernate") + "\n"), wantNote: "Machine sleep, read from the systemd journal, is left out of every time figure.\n", want: []Interval{{at(1), at(9)}}},
	}
	for _, tt := range notes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sleeps, note := sleepsFromJournal(tt.out, tt.runErr)
			if note != tt.wantNote {
				t.Errorf("sleepsFromJournal note = %q; want %q", note, tt.wantNote)
			}
			if len(sleeps) != len(tt.want) {
				t.Fatalf("sleepsFromJournal = %d sleeps; want %d", len(sleeps), len(tt.want))
			}
			for i, want := range tt.want {
				if !sleeps[i].From.Equal(want.From) || !sleeps[i].To.Equal(want.To) {
					t.Errorf("sleep %d = %v..%v; want %v..%v", i, sleeps[i].From, sleeps[i].To, want.From, want.To)
				}
			}
		})
	}
}
