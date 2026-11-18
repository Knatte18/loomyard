// hubstop_test.go pins the order of the hub stop's sequence.
// It pins that the stop lifts only a pause entry it added.
// It pins that the stop goes past an unconfirmed pause and each failed step.
// It pins the command the caller starts detached.

package beamcli

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDetachedStopCommand_RunsDetachedAtThePrimesAnchor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	prime := &lymcwd.Location{HubPath: "/hub", WorktreeName: "prime", AnchorRel: "backend"}

	cmd := detachedStopCommand("/bin/lym", prime)

	wantArgs := []string{"/bin/lym", "beam", "stop", "--all", "--detached"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Errorf("detachedStopCommand() args = %v; want %v", cmd.Args, wantArgs)
	}
	if wantDir := filepath.Join("/hub", "prime", "backend"); cmd.Dir != wantDir {
		t.Errorf("detachedStopCommand() dir = %q; want the prime's anchor %q", cmd.Dir, wantDir)
	}
}

func TestRunHubStop(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owned := &tenderengine.EntryRef{Scope: pauserecord.ScopeHub, Caller: stopPauseCaller}
	order := []string{"pause", "down t1", "beam-down prime", "beam-down b1", "down b1", "reap", "lift", "down prime"}
	without := func(calls []string, dropped ...string) []string {
		return slices.DeleteFunc(slices.Clone(calls), func(call string) bool { return slices.Contains(dropped, call) })
	}
	reached := hibernateengine.HubPause{Owned: owned, Outcome: hibernateengine.ConfirmationReached}
	tests := []struct {
		name   string
		paused hibernateengine.HubPause
		// failCall names the one call, as the fakes record it, that returns an error.
		failCall        string
		wantCalls       []string
		wantLifted      bool
		wantUnconfirmed string
		wantFailures    []string
		// wantPairs is the number of task pairs the report holds, wantOrchs the orchs and wantReaped the reaped processes.
		wantPairs, wantOrchs, wantReaped int
	}{
		{
			name: "an owned pause is lifted after the reap, and the prime's session goes last", paused: reached,
			wantCalls: order, wantLifted: true, wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name:      "a hub entry already standing covers the stop and is not lifted",
			paused:    hibernateengine.HubPause{Outcome: hibernateengine.ConfirmationReached},
			wantCalls: without(order, "lift"), wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name:      "an unconfirmed pause names what was busy and the stop goes on",
			paused:    hibernateengine.HubPause{Owned: owned, Outcome: hibernateengine.ConfirmationTimedOut, Withheld: "a reads busy"},
			wantCalls: order, wantLifted: true, wantUnconfirmed: "a reads busy", wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name:      "a pause the operator lifted before its confirmation is not lifted again",
			paused:    hibernateengine.HubPause{Owned: owned, Outcome: hibernateengine.ConfirmationLifted},
			wantCalls: without(order, "lift"), wantUnconfirmed: "the hub pause was lifted before it was confirmed",
			wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name: "a failed pause is named and the stop goes on without a pause to lift", failCall: "pause", paused: reached,
			wantCalls: without(order, "lift"), wantFailures: []string{"pause the hub: refused"}, wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name: "a failed worktree list is named and the prime is still taken down", failCall: "worktrees", paused: reached,
			wantCalls:    []string{"pause", "beam-down prime", "reap", "lift", "down prime"},
			wantFailures: []string{"list the hub's worktrees: refused"}, wantLifted: true, wantOrchs: 1, wantReaped: 1,
		},
		{
			name: "a failed pair down is named and every later step still runs", failCall: "down t1", paused: reached,
			wantCalls: order, wantFailures: []string{"down the reed session of t1: refused"}, wantLifted: true, wantOrchs: 2, wantReaped: 1,
		},
		{
			name: "a failed beam down is named, its report is kept and every later step still runs", failCall: "beam-down b1", paused: reached,
			wantCalls: order, wantFailures: []string{"take down the orch and dobby of b1: refused"}, wantLifted: true,
			wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
		{
			name: "a failed reap is named and the pause is still lifted", failCall: "reap", paused: reached,
			wantCalls: order, wantFailures: []string{"end the run processes no run owns: refused"}, wantLifted: true,
			wantPairs: 1, wantOrchs: 2,
		},
		{
			name: "a failed lift is named and leaves the pause unlifted", failCall: "lift", paused: reached,
			wantCalls: order, wantFailures: []string{"lift the stop's own pause entry: refused"},
			wantPairs: 1, wantOrchs: 2, wantReaped: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := &lymcwd.Location{HubPath: "/hub", WorktreeName: "prime"}
			beam := &lymcwd.Location{HubPath: "/hub", WorktreeName: "b1"}
			task := &lymcwd.Location{HubPath: "/hub", WorktreeName: "t1"}
			var calls []string
			var lifted []tenderengine.EntryRef
			// record logs call and returns the error the row asks that call to fail with.
			record := func(call string) error {
				calls = append(calls, call)
				if call == tt.failCall {
					return errors.New("refused")
				}
				return nil
			}
			steps := hubStopSteps{
				pause: func() (hibernateengine.HubPause, error) {
					if err := record("pause"); err != nil {
						return hibernateengine.HubPause{}, err
					}
					return tt.paused, nil
				},
				worktrees: func() ([]*lymcwd.Location, []*lymcwd.Location, error) {
					if tt.failCall == "worktrees" {
						return nil, nil, errors.New("refused")
					}
					return []*lymcwd.Location{beam}, []*lymcwd.Location{task}, nil
				},
				downSession: func(location *lymcwd.Location) (reedcli.DownReport, error) {
					if err := record("down " + location.WorktreeName); err != nil {
						return reedcli.DownReport{}, err
					}
					return reedcli.DownReport{Session: location.WorktreeName}, nil
				},
				beamDown: func(location *lymcwd.Location) (orchcli.BeamDownReport, error) {
					return orchcli.BeamDownReport{Worktree: location.WorktreeName}, record("beam-down " + location.WorktreeName)
				},
				reap: func() (tenderengine.ReapReport, error) {
					if err := record("reap"); err != nil {
						return tenderengine.ReapReport{}, err
					}
					return tenderengine.ReapReport{Ended: []tenderengine.ReapedProcess{{PID: 9, Role: "loop"}}}, nil
				},
				lift: func(ref tenderengine.EntryRef) error {
					lifted = append(lifted, ref)
					return record("lift")
				},
			}

			report := runHubStop(prime, steps)

			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("runHubStop() calls = %v; want %v", calls, tt.wantCalls)
			}
			if report.PauseLifted != tt.wantLifted {
				t.Errorf("runHubStop() pause lifted = %v; want %v", report.PauseLifted, tt.wantLifted)
			}
			if tt.wantLifted && (len(lifted) != 1 || lifted[0] != *owned) {
				t.Errorf("runHubStop() lifted entries %+v; want only %+v", lifted, *owned)
			}
			if report.PauseOwned != (tt.paused.Owned != nil && tt.failCall != "pause") || report.Unconfirmed != tt.wantUnconfirmed {
				t.Errorf("runHubStop() pause owned %v, unconfirmed %q; want unconfirmed %q", report.PauseOwned, report.Unconfirmed, tt.wantUnconfirmed)
			}
			if !slices.Equal(report.Failures, tt.wantFailures) {
				t.Errorf("runHubStop() failures = %q; want %q", report.Failures, tt.wantFailures)
			}
			if len(report.Pairs) != tt.wantPairs || len(report.Orchs) != tt.wantOrchs || len(report.Reaped) != tt.wantReaped {
				t.Errorf("runHubStop() pairs %+v, orchs %+v, reaped %+v; want %d, %d and %d of them",
					report.Pairs, report.Orchs, report.Reaped, tt.wantPairs, tt.wantOrchs, tt.wantReaped)
			}
			if tt.failCall == "down t1" && slices.ContainsFunc(report.Pairs, func(down reedcli.DownReport) bool { return down.Session == "t1" }) {
				t.Errorf("runHubStop() pairs = %+v; want t1 absent after its down failed", report.Pairs)
			}
		})
	}
}
