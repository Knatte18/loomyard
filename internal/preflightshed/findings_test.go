// findings_test.go asserts that the general Preflight producer surfaces the determined failures it
// used to discard.
// This row carries no OnStuck in any producer list that names it -- nothing in a list produces the
// git/filesystem state it gates -- so its Stuck halts the whole run for a human, and the driver log
// is the only place that human can read why.

package preflightshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/preflight"
)

// TestFormatFailures covers the rendering the Stuck log line carries: every determined failure, each
// as "check: reason", so no violation is silently dropped from the one account a human gets.
// A Failure whose Check is a CheckID this package does not itself declare is carried through
// verbatim rather than mapped, so a check added elsewhere still reaches the operator.
//
//testtiming:keep pins the exact "check: reason" rendering and its separators, which the broken-precondition test covering its blocks only reaches through one failure
func TestFormatFailures(t *testing.T) {
	t.Parallel()

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
				{Check: preflight.CheckWorktreeClean, Reason: "records side has 2 dirty paths"},
			}},
			want: "worktree-clean: records side has 2 dirty paths",
		},
		{
			name: "EveryFailureIsCarried",
			report: preflight.Report{Failures: []preflight.Failure{
				{Check: preflight.CheckGeometry, Reason: "no main worktree"},
				{Check: preflight.CheckWorktreeClean, Reason: "records side has 2 dirty paths"},
			}},
			want: "geometry: no main worktree; worktree-clean: records side has 2 dirty paths",
		},
		{
			name: "UnrecognisedCheckIDIsCarriedVerbatim",
			report: preflight.Report{Failures: []preflight.Failure{
				{Check: preflight.CheckID("some-future-check"), Reason: "whatever it found"},
			}},
			want: "some-future-check: whatever it found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatFailures(tt.report); got != tt.want {
				t.Errorf("formatFailures(%+v) = %q; want %q", tt.report, got, tt.want)
			}
		})
	}
}
