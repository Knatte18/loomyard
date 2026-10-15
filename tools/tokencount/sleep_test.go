package main

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSleepJournal(t *testing.T) {
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
			want:  []Interval{{at(1), at(600)}, {at(700), at(710)}},
		},
		{name: "a start with no return yields to the next start", lines: []string{enter(0, "b1", "hibernate"), enter(5, "b1", "hibernate"), leave(9, "b1", "hibernate")}, want: []Interval{{at(5), at(9)}}},
		{name: "a return with no start", lines: []string{leave(9, "b1", "suspend")}},
		{name: "a pair split across boots", lines: []string{enter(0, "b1", "hibernate"), leave(9, "b2", "hibernate")}},
		{name: "a return from another operation", lines: []string{enter(0, "b1", "suspend"), leave(9, "b1", "hibernate")}},
		{name: "a byte-array message is skipped", lines: []string{enter(0, "b1", "suspend"), `{"__REALTIME_TIMESTAMP":"1","_BOOT_ID":"b1","MESSAGE":[104,105]}`, leave(9, "b1", "suspend")}, want: []Interval{{at(0), at(9)}}},
		{name: "a line that does not decode", lines: []string{"{"}, wantErr: "does not decode"},
		{name: "a timestamp that does not parse", lines: []string{`{"__REALTIME_TIMESTAMP":"soon","MESSAGE":"x"}`}, wantErr: "does not parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSleepJournal([]byte(strings.Join(tt.lines, "\n") + "\n"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSleepJournal error = %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, tt.want, func(a, b Interval) bool { return a.From.Equal(b.From) && a.To.Equal(b.To) }) {
				t.Errorf("parseSleepJournal = %v; want %v", got, tt.want)
			}
		})
	}
}
