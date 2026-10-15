// durfmt.go holds the two duration formats the status panes and the timing report print.

package durfmt

import (
	"fmt"
	"time"
)

// FormatMinutes renders d in whole minutes as `12m`, `1h05m` or `2d03h`, so a pane line changes at most once a minute.
func FormatMinutes(d time.Duration) string {
	minutes := max(int(d/time.Minute), 0)
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes < 24*60:
		return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
	default:
		return fmt.Sprintf("%dd%02dh", minutes/(24*60), minutes/60%24)
	}
}

// FormatSeconds renders a closed span in whole seconds as `45s`, `12m05s` or `1h05m12s`, and from one day on as FormatMinutes does.
// A negative d renders as `0s`.
func FormatSeconds(d time.Duration) string {
	seconds := max(int(d/time.Second), 0)
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 60*60:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	case seconds < 24*60*60:
		return fmt.Sprintf("%dh%02dm%02ds", seconds/3600, seconds/60%60, seconds%60)
	default:
		return FormatMinutes(d)
	}
}
