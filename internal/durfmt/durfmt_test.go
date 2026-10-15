// durfmt_test.go pins the format boundaries of FormatMinutes and FormatSeconds.

package durfmt

import (
	"testing"
	"time"
)

// TestFormat pins the format boundaries of both FormatMinutes and FormatSeconds.
func TestFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format func(time.Duration) string
		d      time.Duration
		want   string
	}{
		{"minutes below a minute", FormatMinutes, 30 * time.Second, "0m"},
		{"minutes", FormatMinutes, 12 * time.Minute, "12m"},
		{"minutes at 59", FormatMinutes, 59 * time.Minute, "59m"},
		{"minutes at an hour", FormatMinutes, 60 * time.Minute, "1h00m"},
		{"minutes in hours", FormatMinutes, 65 * time.Minute, "1h05m"},
		{"minutes below a day", FormatMinutes, 23*time.Hour + 59*time.Minute, "23h59m"},
		{"minutes at a day", FormatMinutes, 24 * time.Hour, "1d00h"},
		{"minutes in days", FormatMinutes, 51 * time.Hour, "2d03h"},
		{"seconds negative", FormatSeconds, -5 * time.Second, "0s"},
		{"seconds", FormatSeconds, 45 * time.Second, "45s"},
		{"seconds at a minute", FormatSeconds, time.Minute, "1m00s"},
		{"seconds in minutes", FormatSeconds, 12*time.Minute + 5*time.Second, "12m05s"},
		{"seconds below an hour", FormatSeconds, 59*time.Minute + 59*time.Second, "59m59s"},
		{"seconds at an hour", FormatSeconds, time.Hour, "1h00m00s"},
		{"seconds in hours", FormatSeconds, time.Hour + 5*time.Minute + 12*time.Second, "1h05m12s"},
		{"seconds below a day", FormatSeconds, 23*time.Hour + 59*time.Minute + 59*time.Second, "23h59m59s"},
		{"seconds at a day", FormatSeconds, 24 * time.Hour, "1d00h"},
		{"seconds in days", FormatSeconds, 51 * time.Hour, "2d03h"},
	}
	for _, tt := range tests {
		if got := tt.format(tt.d); got != tt.want {
			t.Errorf("%s: format(%v) = %q, want %q", tt.name, tt.d, got, tt.want)
		}
	}
}
