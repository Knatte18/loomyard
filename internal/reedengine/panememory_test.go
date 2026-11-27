// panememory_test.go pins the pane memory cap's size and that a pane the cap cannot reach still launches with a warning.

package reedengine

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPaneMemoryMax(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		mib  int
		want int64
	}{
		{"AHubsOwnValue", 1024, 1073741824},
		{"ZeroTakesTheDefault", 0, 8589934592},
		{"NegativeTakesTheDefault", -1, 8589934592},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := paneMemoryMax(Config{PaneMemoryMaxMiB: tt.mib}); got != tt.want {
				t.Errorf("paneMemoryMax(pane_memory_max_mib %d) = %d; want %d", tt.mib, got, tt.want)
			}
		})
	}
}

// TestLaunchStrandLocked_APaneThatCannotBeCappedStillLaunchesWithAWarning pins the degraded path.
// It captures the process-global logger and empties PATH in one row, so it does not run in parallel.
// With PATH empty the cap's busctl lookup fails on Linux, and Windows has no cap at all;
// either way the cap returns an error wrapping proc.ErrMemoryCap.
func TestLaunchStrandLocked_APaneThatCannotBeCappedStillLaunchesWithAWarning(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name      string
		answer    string
		err       error
		emptyPath bool
		wantLog   string
		wantCause string
	}{
		{"TheShellPidCannotBeRead", "", errors.New("no such pane"), false, "its shell's pid could not be read", ""},
		{"TheShellPidIsNotANumber", "not-a-pid\n", nil, false, "named no shell pid", ""},
		{"TheShellPidIsZero", "0\n", nil, false, "named no shell pid", ""},
		{"TheUserManagerCannotCapThePane", strconv.Itoa(os.Getpid()) + "\n", nil, true, "cause=", proc.ErrMemoryCap.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.emptyPath {
				t.Setenv("PATH", "")
			}
			e := newTestEngine(t)
			fake := launchFake(t, e, nil)
			fake.answerFormat("#{pane_pid}", tt.answer, tt.err)
			buf := logcapture.CaptureVerbose(t)

			st := &ReedState{SelvagePaneID: "%selvage", Strands: []Strand{{GUID: "guid-c"}}}
			if err := e.launchStrandLocked(st, &st.Strands[0], "claude"); err != nil {
				t.Fatalf("launchStrandLocked: %v", err)
			}
			log := buf.String()
			if !strings.Contains(log, "the pane runs with no memory cap") || !strings.Contains(log, tt.wantLog) {
				t.Errorf("log %q; want the no-cap warning naming %q", log, tt.wantLog)
			}
			if tt.err != nil && !strings.Contains(log, tt.err.Error()) {
				t.Errorf("log %q; want it to name the cause %q", log, tt.err)
			}
			if !strings.Contains(log, tt.wantCause) {
				t.Errorf("log %q; want it to name the cap's cause %q", log, tt.wantCause)
			}
			sends := fake.ArgvFor("send-keys")
			if len(sends) == 0 || sends[len(sends)-1][len(sends[len(sends)-1])-1] != "Enter" {
				t.Errorf("send-keys calls %v; want the launch to still submit its command", sends)
			}
		})
	}
}
