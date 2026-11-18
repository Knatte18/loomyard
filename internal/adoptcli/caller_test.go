// caller_test.go drives the `lym adopt` call over fake seams.
// It covers the here call, the orch default and the end envelope.

package adoptcli

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunAdopt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	alphaRequest := request{Slugs: []string{"alpha"}}
	// failedCheck is the result of an adopt check whose alpha failed and filed the fix task fix1.
	failedCheck := holdCheckResult{
		State: checkFailed, Ran: true, States: map[string]string{"alpha": checkFailed}, FixTask: "fix1",
		Lines: []string{"the adopt check of alpha failed at tier0; fix task fix1 is filed"},
	}

	tests := []struct {
		name   string
		strand string
		req    request
		// here passes --here from the worktree named worktree; pullSkip is the detail of a Bolt pull that could not run.
		here      bool
		worktree  string
		pullSkip  string
		pullErr   error
		wantPulls int
		setup     func(w *world)
		wantExit  int
		wantOut   []string
		check     func(t *testing.T, w *world)
	}{
		{
			name: "HereInAPrimeThisMachineOwnsEndsOwnedHereAndWritesNothing", here: true, worktree: "prime",
			wantPulls: 1, wantOut: []string{`"outcome":"owned_here"`, `"subject":"root"`, `"ok":true`},
			check: func(t *testing.T, w *world) {
				if len(w.order) != 0 {
					t.Errorf("calls = %v; want no write and nothing started", w.order)
				}
			},
		},
		{
			name: "HereReadsTheLocalBoltWhenThePullCannotReachOrigin", here: true, worktree: "prime", pullSkip: "fetching origin failed",
			wantPulls: 1, wantOut: []string{`"outcome":"owned_here"`, "fetching origin failed"},
		},
		{
			name: "HereInAForeignOwnedPrimeAdoptsRoot", here: true, worktree: "prime",
			setup:     func(w *world) { w.owners.Prime = there },
			wantPulls: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, w *world) {
				if !w.writeReq.Prime || len(w.writeReq.Slugs) != 0 {
					t.Errorf("adopt request = %+v; want the prime alone", w.writeReq)
				}
			},
		},
		{
			name: "HereInAForeignOwnedBeamAdoptsTheBeam", here: true, worktree: "alpha",
			wantPulls: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, w *world) {
				if !slices.Equal(w.writeReq.Slugs, []string{"alpha"}) || w.writeReq.Prime {
					t.Errorf("adopt request = %+v; want the beam alone", w.writeReq)
				}
			},
		},
		{
			name: "HereInATaskWorktreeDoesNothing", here: true, worktree: "idle",
			wantOut: []string{`"outcome":"no_subject"`, `"ok":true`},
			check: func(t *testing.T, w *world) {
				if len(w.order) != 0 {
					t.Errorf("calls = %v; want none", w.order)
				}
			},
		},
		{name: "HereWithASubjectIsRefused", here: true, worktree: "prime", req: alphaRequest, wantExit: 1, wantOut: []string{"--here"}},
		{name: "HereWithRootIsRefused", here: true, worktree: "prime", req: request{Root: true}, wantExit: 1, wantOut: []string{"--here"}},
		{name: "HereWithAllIsRefused", here: true, worktree: "prime", req: request{All: true}, wantExit: 1, wantOut: []string{"--here"}},
		{
			name: "HereWhenThePullFailsIsRefusedWithNothingWritten", here: true, worktree: "prime", pullErr: errors.New("the board clone is broken"),
			wantPulls: 1, wantExit: 1, wantOut: []string{"pull Bolt", "the board clone is broken"},
			check: func(t *testing.T, w *world) {
				if len(w.order) != 0 {
					t.Errorf("calls = %v; want none", w.order)
				}
			},
		},
		{
			name: "HereCarriesTheTakeHoldFlagToTheAdopt", here: true, worktree: "alpha", req: request{TakeHold: true},
			wantPulls: 1, wantOut: []string{`"ok":true`},
			check: func(t *testing.T, w *world) {
				if !w.writeReq.TakeHold {
					t.Errorf("adopt request = %+v; want the take-hold flag carried", w.writeReq)
				}
			},
		},
		{
			name: "AGreenCheckEndsWithExitZeroAndTheEnvelope", req: alphaRequest,
			wantOut: []string{`"prime_recorded":false`, `"ok":true`},
		},
		{
			name: "AFailedCheckEndsNonZeroNamingTheFixTaskAndKeepingTheHold", req: alphaRequest,
			setup:    func(w *world) { w.check = failedCheck },
			wantExit: 1, wantOut: []string{"fix1", `"fix_task":"fix1"`, `"hold"`, `"ok":false`},
		},
		{name: "ARefusedRequestEndsNonZeroNamingTheRefusal", req: request{Slugs: []string{"nope"}}, wantExit: 1, wantOut: []string{"lym board list"}},
		{
			name: "OrchStrandAdoptsWithNoOrch", req: alphaRequest, strand: "kn:orch",
			wantOut: []string{`"ok":true`}, check: func(t *testing.T, w *world) { requireNoOrch(t, w, true) },
		},
		{
			name: "NumberedOrchStrandAdoptsWithNoOrch", req: alphaRequest, strand: "kn:orch-2",
			wantOut: []string{`"ok":true`}, check: func(t *testing.T, w *world) { requireNoOrch(t, w, true) },
		},
		{
			name: "ImplementerStrandAdoptsWithoutNoOrch", req: alphaRequest, strand: "kn:implementer-1",
			wantOut: []string{`"ok":true`}, check: func(t *testing.T, w *world) { requireNoOrch(t, w, false) },
		},
		{
			name: "EmptyStrandAdoptsWithoutNoOrch", req: alphaRequest,
			wantOut: []string{`"ok":true`}, check: func(t *testing.T, w *world) { requireNoOrch(t, w, false) },
		},
		{
			name: "UnparsableStrandAdoptsWithoutNoOrch", req: alphaRequest, strand: "not a name",
			wantOut: []string{`"ok":true`}, check: func(t *testing.T, w *world) { requireNoOrch(t, w, false) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld()
			if tt.setup != nil {
				tt.setup(w)
			}
			exempted, restored, pulls := 0, 0, 0
			deps := callerDeps{
				ExemptPause: func() func() {
					exempted++
					return func() { restored++ }
				},
				Strand:   tt.strand,
				Worktree: tt.worktree,
				PullBolt: func() (string, error) {
					if exempted == 0 {
						t.Errorf("Bolt pulled before the call passed every pause")
					}
					pulls++
					return tt.pullSkip, tt.pullErr
				},
				Steps:  w.deps(),
				Stderr: io.Discard,
			}

			var out bytes.Buffer
			exit := runAdopt(deps, tt.req, callFlags{Here: tt.here}, &out)

			if (exit != 0) != (tt.wantExit != 0) {
				t.Errorf("runAdopt = exit %d, want exit %d; output %s", exit, tt.wantExit, out.String())
			}
			if pulls != tt.wantPulls {
				t.Errorf("runAdopt pulled Bolt %d times, want %d", pulls, tt.wantPulls)
			}
			if exempted != 1 || restored != 1 {
				t.Errorf("runAdopt exempted the pause gate %d times and restored it %d times, want once each", exempted, restored)
			}
			for _, text := range tt.wantOut {
				if !strings.Contains(out.String(), text) {
					t.Errorf("output = %s, want it to contain %q", out.String(), text)
				}
			}
			if tt.check != nil {
				tt.check(t, w)
			}
		})
	}
}

// requireNoOrch fails the test unless the beam steps of the adopt left the orch down exactly when want is set.
func requireNoOrch(t *testing.T, w *world, want bool) {
	t.Helper()
	if len(w.noOrch) != 1 || w.noOrch[0] != want || w.checkNoOrch != want {
		t.Errorf("noOrch arguments = %v, check noOrch = %v; want one beam step and the check with no-orch %v", w.noOrch, w.checkNoOrch, want)
	}
}
