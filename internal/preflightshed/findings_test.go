// findings_test.go asserts that the general Preflight producer surfaces the determined failures it
// used to discard.
// This row carries no OnStuck in any producer list that names it -- nothing in a list produces the
// git/filesystem state it gates -- so its Stuck halts the whole run for a human, and the driver log
// is the only place that human can read why.

package preflightshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/preflight"
)

// TestFormatFailures covers the rendering the Stuck log line carries: every determined failure, each
// as "check: reason", so no violation is silently dropped from the one account a human gets.
func TestFormatFailures(t *testing.T) {
	tests := []struct {
		name   string
		report preflight.Report
		want   string
	}{
		{
			name:   "NoFailures",
			report: preflight.Report{OK: true},
			want:   "",
		},
		{
			name: "SingleFailure",
			report: preflight.Report{Failures: []preflight.Failure{
				{Check: preflight.CheckWorktreeClean, Reason: "weft has 2 dirty paths"},
			}},
			want: "worktree-clean: weft has 2 dirty paths",
		},
		{
			name: "EveryFailureIsCarried",
			report: preflight.Report{Failures: []preflight.Failure{
				{Check: preflight.CheckGeometry, Reason: "no main worktree"},
				{Check: preflight.CheckWorktreeClean, Reason: "weft has 2 dirty paths"},
			}},
			want: "geometry: no main worktree; worktree-clean: weft has 2 dirty paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatFailures(tt.report); got != tt.want {
				t.Errorf("formatFailures(%+v) = %q; want %q", tt.report, got, tt.want)
			}
		})
	}
}

// TestFormatFailures_NamesEveryCheckID guards the rendering against a Failure whose Check is a
// CheckID this package does not itself declare: the value is carried through verbatim rather than
// mapped, so a check added elsewhere still reaches the operator.
func TestFormatFailures_NamesEveryCheckID(t *testing.T) {
	got := formatFailures(preflight.Report{Failures: []preflight.Failure{
		{Check: preflight.CheckID("some-future-check"), Reason: "whatever it found"},
	}})
	if !strings.Contains(got, "some-future-check") {
		t.Errorf("formatFailures() = %q; want it to carry an unrecognised check ID verbatim", got)
	}
}
