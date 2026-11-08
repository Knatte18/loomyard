//go:build !windows

// slacktoggle_test.go drives ShowToggleOutcome against a stand-in tmux executable that records its arguments.
// ShowToggleOutcome builds its own tmux command, so only a spawned binary shows the calls it makes.

package reedengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordingTmux writes a stand-in tmux that records every invocation.
// It returns the stand-in's path and a reader of the recorded calls.
// The stand-in appends one argument per line to a log, then a "--" line.
func recordingTmux(t *testing.T) (tmuxPath string, calls func() [][]string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	tmuxPath = filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> '" + logPath + "'\nprintf -- '--\\n' >> '" + logPath + "'\n"
	if err := os.WriteFile(tmuxPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile tmux stand-in: %v", err)
	}
	return tmuxPath, func() [][]string {
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ReadFile calls log: %v", err)
		}
		var recorded [][]string
		var current []string
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			if line == "--" {
				recorded = append(recorded, current)
				current = nil
				continue
			}
			current = append(current, line)
		}
		return recorded
	}
}

// TestShowToggleOutcome pins the tmux calls a Slack toggle's click ends with.
// A message is shown on the clicking client first, with every # and % doubled, and the bar redraw follows in every case.
// It sets the process-global version probe, so it does not run in parallel.
func TestShowToggleOutcome(t *testing.T) {
	tiergate.Require(t, 1)
	setTmuxVersionProbeForTest(t, func(string) (string, error) { return "tmux 3.6", nil })

	const socketPath, client = "/tmp/tmux-1000/sock", "/dev/pts/7"
	redraw := []string{"-S", socketPath, "refresh-client", "-S", "-t", client}
	tests := []struct {
		name    string
		message string
		want    func() [][]string
	}{
		{"EmptyMessageOnlyRedraws", "", func() [][]string { return [][]string{redraw} }},
		{"MessageIsShownThenRedrawn", "slack on", func() [][]string {
			return [][]string{{"-S", socketPath, "display-message", "-c", client, "slack on"}, redraw}
		}},
		{"HashAndPercentInTheMessageAreDoubled", "#3 is 50% done", func() [][]string {
			return [][]string{{"-S", socketPath, "display-message", "-c", client, "##3 is 50%% done"}, redraw}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmuxPath, calls := recordingTmux(t)

			ShowToggleOutcome(tmuxPath, socketPath, client, tt.message)

			if got, want := calls(), tt.want(); !slices.EqualFunc(got, want, slices.Equal[[]string]) {
				t.Errorf("ShowToggleOutcome(%q) tmux calls = %q; want %q", tt.message, got, want)
			}
		})
	}
}
