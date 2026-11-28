// reach_test.go covers WithReachTurn over a scripted inner gate and a scripted pending report.
// The test is not parallel as a whole: one row captures the process-wide logger output.

package verifytree

import (
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// turnOutcome is what one arrival at the decorated gate answers.
type turnOutcome string

const (
	outcomePass         turnOutcome = "pass"
	outcomeInnerFailure turnOutcome = "inner failure"
	outcomeTurn         turnOutcome = "turn"
	outcomeError        turnOutcome = "error"
)

// turnStep is one arrival: what the inner gate and the pending source answer, and what the decorated gate must answer.
type turnStep struct {
	inner shuttleengine.GateResult
	// rerun runs the arrival inside the turn's RerunMark.
	rerun      bool
	pending    ReachPending
	found      bool
	pendingErr error
	want       turnOutcome
	// wantListed, when non-nil, is what the listed file holds after the arrival.
	wantListed map[string]string
}

// TestWithReachTurn pins the turn's outcomes over scripted arrivals, each scenario with its own gate and listed file.
func TestWithReachTurn(t *testing.T) {
	tiergate.Require(t, 0)

	pendingOf := func(id, hash string) ReachPending {
		return ReachPending{Note: "gate test: reach: no mapped test reaches " + id, Functions: map[string]string{id: hash}}
	}
	errPending := errors.New("reach report unreadable")
	pass := shuttleengine.GateResult{Passed: true}
	innerFailure := shuttleengine.GateResult{Findings: "inner broke"}

	tests := []struct {
		name     string
		attempts int
		steps    []turnStep
	}{
		{"AFunctionListedOnceBringsNoSecondTurnUntilItsHashChanges", 3, []turnStep{
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomeTurn, wantListed: map[string]string{"a.go#F": "h1"}},
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomePass},
			{inner: pass, pending: pendingOf("a.go#F", "h2"), found: true, want: outcomeTurn, wantListed: map[string]string{"a.go#F": "h2"}},
		}},
		{"ALaterTurnKeepsTheFunctionsAnEarlierTurnListed", 4, []turnStep{
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomeTurn, wantListed: map[string]string{"a.go#F": "h1"}},
			{inner: pass, pending: pendingOf("b.go#G", "h2"), found: true, want: outcomeTurn, wantListed: map[string]string{"a.go#F": "h1", "b.go#G": "h2"}},
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomePass},
		}},
		{"APendingOrTerminalInnerFailureAddsNothingToTheFailingCount", 2, []turnStep{
			{inner: shuttleengine.GateResult{Findings: "waiting", Pending: true}, want: outcomeInnerFailure},
			{inner: shuttleengine.GateResult{Findings: "stop", Terminal: true}, want: outcomeInnerFailure},
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomeTurn},
		}},
		{"AnArrivalOneFailureShortOfTheBudgetPassesWithoutTheTurn", 3, []turnStep{
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomePass},
		}},
		{"APassResetsTheFailureCount", 4, []turnStep{
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: pass, want: outcomePass},
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: pass, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomeTurn},
		}},
		{"AFailureAndATurnInsideTheReRunAddNothingToTheCount", 3, []turnStep{
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: innerFailure, rerun: true, want: outcomeInnerFailure},
			{inner: pass, rerun: true, pending: pendingOf("a.go#F", "h1"), found: true, want: outcomeTurn},
			{inner: innerFailure, want: outcomeInnerFailure},
			{inner: pass, pending: pendingOf("b.go#G", "h2"), found: true, want: outcomePass},
		}},
		{"APassWithNoPendingReportPasses", 3, []turnStep{
			{inner: pass, want: outcomePass},
		}},
		{"APendingErrorIsReturnedAsTheGatesError", 3, []turnStep{
			{inner: pass, pendingErr: errPending, want: outcomeError},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			listedPath := filepath.Join(t.TempDir(), "listed.json")
			mark := &shuttleengine.RerunMark{}
			var current turnStep
			gate := WithReachTurn(
				func() (shuttleengine.GateResult, error) { return current.inner, nil },
				ReachTurn{
					Pending: func() (ReachPending, bool, error) {
						return current.pending, current.found, current.pendingErr
					},
					Attempts:   tt.attempts,
					Rerun:      mark,
					ListedPath: listedPath,
				})
			for i, step := range tt.steps {
				current = step
				run := gate
				if step.rerun {
					run = func() (shuttleengine.GateResult, error) { return mark.Run(gate) }
				}
				got, err := run()
				switch step.want {
				case outcomePass:
					if err != nil || !got.Passed || got.WriterOnly {
						t.Errorf("step %d: gate() = %+v, %v; want a plain pass", i, got, err)
					}
				case outcomeInnerFailure:
					if err != nil || got != step.inner {
						t.Errorf("step %d: gate() = %+v, %v; want the inner failure %+v unchanged", i, got, err, step.inner)
					}
				case outcomeTurn:
					if err != nil || got.Passed || !got.WriterOnly ||
						!strings.HasPrefix(got.Findings, step.pending.Note) || !strings.HasSuffix(got.Findings, reachWayForward) {
						t.Errorf("step %d: gate() = %+v, %v; want a writer-only failure of the note and the way forward", i, got, err)
					}
				case outcomeError:
					if !errors.Is(err, step.pendingErr) {
						t.Errorf("step %d: gate() error = %v; want %v", i, err, step.pendingErr)
					}
				}
				if step.wantListed != nil {
					listed, _, err := state.ReadJSON[reachListed](listedPath, lock.TrackedFileLockPath(listedPath, ".lock"))
					if err != nil || !maps.Equal(listed.Functions, step.wantListed) {
						t.Errorf("step %d: listed file = %v, %v; want %v", i, listed.Functions, err, step.wantListed)
					}
				}
			}
		})
	}

	t.Run("AListedPathHeldByADirectoryPassesWithoutTheTurnAndWarns", func(t *testing.T) {
		logs := logcapture.Capture(t)
		listedPath := t.TempDir()
		gate := WithReachTurn(
			func() (shuttleengine.GateResult, error) { return pass, nil },
			ReachTurn{
				Pending:    func() (ReachPending, bool, error) { return pendingOf("a.go#F", "h1"), true, nil },
				Attempts:   3,
				ListedPath: listedPath,
			})

		got, err := gate()
		if err != nil || !got.Passed || got.WriterOnly {
			t.Errorf("gate() = %+v, %v; want a plain pass", got, err)
		}
		if !strings.Contains(logs.String(), listedPath) {
			t.Errorf("logged %q; want a Warn naming the listed path %q", logs.String(), listedPath)
		}
	})
}
