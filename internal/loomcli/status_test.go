// status_test.go is a table over renderStatusLine, pinning its exact rendered line for each shape of
// Activity the composed status file can carry, plus a table over printStatusLinesOnChange's
// suppress-an-unchanged-line rule.

package loomcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestRenderStatusLine covers renderStatusLine's three shapes: an empty last and wait, a populated
// last only, and both populated -- each asserting the exact expected line, since the format is pinned
// rather than left to judgment.
func TestRenderStatusLine(t *testing.T) {
	tests := []struct {
		name string
		st   shedengine.Status
		want string
	}{
		{
			name: "EmptyLastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Preflight", Last: "", Wait: ""},
			},
			want: "loom running | now Preflight",
		},
		{
			name: "LastOnly",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Discussion-Write", Last: "Preflight → done", Wait: ""},
			},
			want: "loom running | now Discussion-Write | last Preflight → done",
		},
		{
			name: "LastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateBlocked,
				Activity: shedengine.Activity{Now: "Plan-Validate", Last: "Plan-Validate → stuck", Wait: "plan validation failed"},
			},
			want: "loom blocked | now Plan-Validate | last Plan-Validate → stuck | wait plan validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderStatusLine(tt.st); got != tt.want {
				t.Errorf("renderStatusLine(%+v) = %q; want %q", tt.st, got, tt.want)
			}
		})
	}
}

// TestPrintStatusLinesOnChange covers the suppress-an-unchanged-line rule the watch tail rests on.
// The UnchangedRepeats row is the direct regression guard: before this rule the tail printed one
// line per poll, so a producer call that lasts minutes emitted hundreds of byte-identical lines into
// the strand pane and evicted its own scrollback.
func TestPrintStatusLinesOnChange(t *testing.T) {
	tests := []struct {
		name  string
		polls []string
		want  []string
	}{
		{
			name:  "UnchangedRepeats",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Write", "loom running | now Plan-Write"},
			want:  []string{"loom running | now Plan-Write"},
		},
		{
			name:  "PrintsEveryTransition",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Write", "loom running | now Plan-Validate", "loom blocked | now Plan-Validate"},
			want:  []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom blocked | now Plan-Validate"},
		},
		{
			name:  "ReprintsAfterReturningToAnEarlierLine",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom running | now Plan-Write"},
			want:  []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom running | now Plan-Write"},
		},
		{
			name:  "TransientUnavailableIsAlsoDeduped",
			polls: []string{statusUnavailableLine, statusUnavailableLine, "loom running | now Plan-Write"},
			want:  []string{statusUnavailableLine, "loom running | now Plan-Write"},
		},
		{
			name:  "FirstLineIsAlwaysPrinted",
			polls: []string{"loom running | now Preflight"},
			want:  []string{"loom running | now Preflight"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			next := 0
			poll := func() string {
				line := tt.polls[next]
				next++
				return line
			}
			slept := 0
			printStatusLinesOnChange(&out, poll, func() { slept++ }, len(tt.polls))

			got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if out.Len() == 0 {
				got = nil
			}
			if len(got) != len(tt.want) {
				t.Fatalf("printStatusLinesOnChange(%v) printed %d line(s) %q; want %d line(s) %q", tt.polls, len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("printStatusLinesOnChange(%v) line %d = %q; want %q", tt.polls, i, got[i], tt.want[i])
				}
			}
			if slept != len(tt.polls) {
				t.Errorf("printStatusLinesOnChange(%v) slept %d time(s); want %d -- the tail must keep polling at its interval even while suppressing output", tt.polls, slept, len(tt.polls))
			}
		})
	}
}
