//go:build smoke

// smoke_dotfill_measure_test.go is the measurement gate for the repaint candidates named in reed's
// repaint-mechanism decision: it installs each candidate's hook body into reed's own window-resized
// array directly from this file, as a literal string, rather than from any production repaint code —
// no such code exists yet, and the whole point of this file is to measure a body before one is
// written. A candidate is accepted only when it clears the dot-fill artifact this package's control
// scenarios reproduce AND satisfies both criteria in the repaint-must-not-self-retrigger decision: no
// repeated hook fire, no resize storm.

package reedcli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
)

// tmuxHookQuote mirrors reedengine's unexported tmuxQuoteValue: it wraps s in tmux double quotes,
// backslash-escaping \, ", and $ first. tmux parses a hook's value as a tmux command line with its
// own word splitting, so a shell fragment inside it has to survive as one tmux word; $ is escaped
// because tmux expands it inside double quotes.
func tmuxHookQuote(s string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
	).Replace(s)
	return `"` + escaped + `"`
}

// fireCounterEntry returns a window-resized array entry body that appends one line to countPath
// every time the hook fires.
// It carries "-b" — without it the tmux server blocks while the command runs — and it is the
// cheapest available instrument: it is appended to the array for the duration of one measurement
// only, and it is never part of any shipped array.
func fireCounterEntry(t *testing.T, countPath string) string {
	t.Helper()
	fragment := "echo fire >> " + shell.Posix().Quote(countPath)
	return "run-shell -b " + tmuxHookQuote(fragment)
}

// fireCount reads countPath and returns its line count, treating an absent file as zero.
func fireCount(t *testing.T, countPath string) int {
	t.Helper()
	data, err := os.ReadFile(countPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0
		}
		t.Fatalf("read fire count file %s: %v", countPath, err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// sampleWindowSize polls `display-message -p -t "=<session>:" "#{window_width} #{window_height}"`
// every 100 ms for the whole of window and returns every answer in order.
func sampleWindowSize(t *testing.T, tmuxPath, socket, session string, window time.Duration) []string {
	t.Helper()
	target := "=" + session + ":"
	deadline := time.Now().Add(window)
	var samples []string
	for time.Now().Before(deadline) {
		out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", target, "#{window_width} #{window_height}").Output()
		if err != nil {
			t.Fatalf("display-message window size: %v", err)
		}
		samples = append(samples, strings.TrimSpace(string(out)))
		time.Sleep(100 * time.Millisecond)
	}
	return samples
}

// assertWindowSizeSettles fails unless the final third of samples is a single repeated value.
//
// This is the no resize storm criterion: the window's size must be observably stable after the
// trigger settles, rather than oscillating between two clients' sizes.
func assertWindowSizeSettles(t *testing.T, samples []string) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("no window-size samples collected")
	}
	n := len(samples) / 3
	if n == 0 {
		n = len(samples)
	}
	tail := samples[len(samples)-n:]
	want := tail[0]
	for _, got := range tail {
		if got != want {
			t.Fatalf("window size did not settle within the measurement window — this is the no-resize-storm criterion: final third of samples = %v, want a single repeated value", tail)
		}
	}
}

// assertSingleHookFire fails unless got is exactly 1.
//
// This is the no repeated hook fire criterion: one settled resize must yield the documented single
// fire, not a growing series. A growing series is the resize-storm feedback path where a
// server-issued repaint would move the most-recently-used client pointer that window-size latest
// keys on.
func assertSingleHookFire(t *testing.T, got int) {
	t.Helper()
	if got != 1 {
		t.Fatalf("hook fired %d times; want exactly 1 — this is the no-repeated-hook-fire criterion", got)
	}
}
