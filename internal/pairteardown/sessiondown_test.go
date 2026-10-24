//go:build tier0

// sessiondown_test.go pins EndBeforeSessionDown, the local-state reading `lyx reed down` runs before it kills a pair's session.

package pairteardown

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/runproc"
)

// TestEndBeforeSessionDown pins which local readings let a session end through, the run-lock wait and owned-process end that follow, and the refusal of a running unpaused run.
func TestEndBeforeSessionDown(t *testing.T) {
	t.Parallel()

	done := "done"
	ended := []runproc.Ended{{PID: 11, Role: runproc.RoleDriver}}
	awaitRefusal := errors.New("the run lock is held; run `lyx reed down` again")
	tests := []struct {
		name string
		f    fakeSubstrates
		// wantCalls are the substrate calls in order.
		wantCalls string
		// wantAwait is what the run-lock wait saw, zero when it was not called.
		wantAwait awaitCall
		wantEnded []runproc.Ended
		// wantErr and wantErrIs describe the refusal; wantContains are substrings of its text.
		wantErr      bool
		wantErrIs    error
		wantContains []string
	}{
		{
			name:      "CoveringSlugPauseWaitsThenEnds",
			f:         fakeSubstrates{paused: true, endedProcs: ended},
			wantCalls: "awaitOrPass,endOwned",
			wantAwait: awaitCall{paused: true, retry: "`lyx reed down`"},
			wantEnded: ended,
		},
		{
			name:      "CoveringHubPauseWaitsThenEnds",
			f:         fakeSubstrates{paused: true, hubEntry: true, endedProcs: ended},
			wantCalls: "awaitOrPass,endOwned",
			wantAwait: awaitCall{paused: true, hubEntry: true, retry: "`lyx reed down`"},
			wantEnded: ended,
		},
		{
			name:      "LocalDoneEntryWaitsThenEnds",
			f:         fakeSubstrates{claim: &done, claimFound: true, endedProcs: ended},
			wantCalls: "readClaim,awaitOrPass,endOwned",
			wantAwait: awaitCall{retry: "`lyx reed down`"},
			wantEnded: ended,
		},
		{
			name:      "FinalizeDoneWithNoLiveBattenWaitsThenEnds",
			f:         fakeSubstrates{childLanded: true, endedProcs: ended},
			wantCalls: "readClaim,landed,awaitOrPass,endOwned",
			wantAwait: awaitCall{retry: "`lyx reed down`"},
			wantEnded: ended,
		},
		{
			name:      "NotRunningWaitsThenEnds",
			f:         fakeSubstrates{quiets: []quietState{{quiet: true}}, endedProcs: ended},
			wantCalls: "readClaim,landed,quiet,awaitOrPass,endOwned",
			wantAwait: awaitCall{retry: "`lyx reed down`"},
			wantEnded: ended,
		},
		{
			name:         "FinalizeDoneWithALiveBattenRefuses",
			f:            fakeSubstrates{childLanded: true, battenAlive: true},
			wantCalls:    "readClaim,landed",
			wantErr:      true,
			wantErrIs:    ErrRunRunning,
			wantContains: []string{"lyx tender pause slug", "lyx reed down"},
		},
		{
			name:         "LiveDriverStrandRefuses",
			f:            fakeSubstrates{quiets: []quietState{{driver: "tst:slug:driver"}}},
			wantCalls:    "readClaim,landed,quiet",
			wantErr:      true,
			wantErrIs:    ErrRunRunning,
			wantContains: []string{"lyx tender pause slug"},
		},
		{
			name:      "LiveLoopRefuses",
			f:         fakeSubstrates{quiets: []quietState{{loop: true}}},
			wantCalls: "readClaim,landed,quiet",
			wantErr:   true,
			wantErrIs: ErrRunRunning,
		},
		{
			name:      "HeldDriverLockRefuses",
			f:         fakeSubstrates{quiets: []quietState{{goDriver: true}}},
			wantCalls: "readClaim,landed,quiet",
			wantErr:   true,
			wantErrIs: ErrRunRunning,
		},
		{
			name:      "RefusedRunLockWaitIsReturnedAsItIsAndEndsNothing",
			f:         fakeSubstrates{paused: true, awaitOrPassErr: awaitRefusal},
			wantCalls: "awaitOrPass",
			wantAwait: awaitCall{paused: true, retry: "`lyx reed down`"},
			wantErr:   true,
			wantErrIs: awaitRefusal,
		},
		{
			name:         "SurvivorsAreReturnedWrappedNamingThePid",
			f:            fakeSubstrates{paused: true, endedProcs: ended, endOwnedErr: &runproc.SurvivorsError{Survivors: []runproc.Ended{{PID: 4242, Role: runproc.RoleLoop}}}},
			wantCalls:    "awaitOrPass,endOwned",
			wantAwait:    awaitCall{paused: true, retry: "`lyx reed down`"},
			wantEnded:    ended,
			wantErr:      true,
			wantContains: []string{"4242", "lyx reed down"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := tt.f
			got, err := f.teardown().EndBeforeSessionDown(context.Background(), "slug")

			if calls := strings.Join(f.calls, ","); calls != tt.wantCalls {
				t.Errorf("calls = %q, want %q", calls, tt.wantCalls)
			}
			if f.awaitSeen != tt.wantAwait {
				t.Errorf("run-lock wait saw %+v, want %+v", f.awaitSeen, tt.wantAwait)
			}
			if !reflect.DeepEqual(got, tt.wantEnded) {
				t.Errorf("ended = %v, want %v", got, tt.wantEnded)
			}
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("EndBeforeSessionDown() error = %v, want none", err)
				}
				return
			}
			if err == nil {
				t.Fatal("EndBeforeSessionDown() error = nil, want a refusal")
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("error = %v, want it to wrap %v", err, tt.wantErrIs)
			}
			for _, text := range tt.wantContains {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("error %q lacks %q", err, text)
				}
			}
		})
	}
}
