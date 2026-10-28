//go:build tier0

package claudeengine

import (
	"testing"
	"time"
)

func TestUsageLimitResetsAt(t *testing.T) {
	t.Parallel()
	// 12:03:42 in Oslo, which keeps UTC+1 after the October clock change.
	const written = "2026-10-28T11:03:42.000Z"
	tests := []struct {
		name      string
		text      string
		timestamp string
		want      time.Time
	}{
		{"An hour later the same day", "You've hit your weekly limit · resets 2pm (Europe/Oslo)", written, time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)},
		{"Hour and minute", "You've hit your session limit · resets 2:30pm (Europe/Oslo)", written, time.Date(2026, 10, 28, 13, 30, 0, 0, time.UTC)},
		{"A time already past rolls to the next day", "You've hit your session limit · resets 9am (Europe/Oslo)", written, time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)},
		{"Twelve am is midnight", "You've hit your session limit · resets 12am (Europe/Oslo)", written, time.Date(2026, 10, 28, 23, 0, 0, 0, time.UTC)},
		{"A month and day", "You've hit your weekly limit · resets Oct 30, 2pm (Europe/Oslo)", written, time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)},
		{"A date already past rolls to the next year", "You've hit your weekly limit · resets Jan 2, 2pm (Europe/Oslo)", written, time.Date(2027, 1, 2, 13, 0, 0, 0, time.UTC)},
		{"The older wording", "Claude usage limit reached. Your limit will reset at 2pm (Europe/Oslo).", written, time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)},
		{"An unknown zone", "You've hit your weekly limit · resets 2pm (Mars/Olympus)", written, time.Time{}},
		{"No reset clause", "You've hit your weekly limit", written, time.Time{}},
		{"An hour out of range", "You've hit your weekly limit · resets 13pm (Europe/Oslo)", written, time.Time{}},
		{"A day the month lacks", "You've hit your weekly limit · resets Feb 31, 2pm (Europe/Oslo)", written, time.Time{}},
		{"A timestamp that does not parse", "You've hit your weekly limit · resets 2pm (Europe/Oslo)", "yesterday", time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := usageLimitResetsAt(tt.text, tt.timestamp); !got.Equal(tt.want) {
				t.Errorf("usageLimitResetsAt(%q, %q) = %v; want %v", tt.text, tt.timestamp, got, tt.want)
			}
		})
	}
}
