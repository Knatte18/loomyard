// teardown_test.go drives the teardown sequence through fakes, so it spawns nothing.

package pairteardown

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// fakeSubstrates records the order of substrate calls and answers from scripted values.
type fakeSubstrates struct {
	calls      []string
	quiets     []quietState
	refusalErr error
	endErr     error
	sleeps     int
}

func (f *fakeSubstrates) teardown() *Teardown {
	return &Teardown{
		prime:    &lyxcwd.Location{HubPath: "/hub", AnchorRel: "."},
		interval: time.Second,
		gone:     func(string) (bool, error) { return false, nil },
		quiet: func(string) (quietState, error) {
			f.calls = append(f.calls, "quiet")
			st := f.quiets[0]
			if len(f.quiets) > 1 {
				f.quiets = f.quiets[1:]
			}
			return st, nil
		},
		refusal: func(string, bool) error {
			f.calls = append(f.calls, "refusal")
			return f.refusalErr
		},
		endSession: func(string, bool) (bool, string, error) {
			f.calls = append(f.calls, "end")
			return f.endErr == nil, "", f.endErr
		},
		remove: func(Request) (fabricengine.RemoveResult, error) {
			f.calls = append(f.calls, "remove")
			return fabricengine.RemoveResult{}, nil
		},
		sleep: func(context.Context, time.Duration) error {
			f.sleeps++
			return nil
		},
	}
}

var busy = quietState{driver: "tst:slug:driver"}

// TestTeardown_CallSequence drives EndSession and Run through the fake substrates and pins which substrate calls each makes, in what order, and what it returns.
func TestTeardown_CallSequence(t *testing.T) {
	t.Parallel()

	refusal := errors.New("worktree has uncommitted changes")
	tests := []struct {
		name string
		// run selects Teardown.Run over Teardown.EndSession.
		run        bool
		quiets     []quietState
		refusalErr error
		endErr     error
		// gone makes the task worktree read as gone.
		gone    bool
		request Request
		// wantErrIs is a sentinel the error must wrap; wantErrContains are substrings it must hold.
		wantErrIs       error
		wantErrContains []string
		// wantAnyError requires an error without naming it.
		wantAnyError bool
		// wantCalls is the comma-joined substrate call order.
		wantCalls  string
		wantSleeps int
		// wantSession, when set, is the Ended and DriverWasLive of the returned session result.
		wantSession *SessionResult
		// wantSawGone requires the session end to have been told the task worktree is gone.
		wantSawGone bool
	}{
		{
			name:            "BusyDriverAtBoundRefusesWithoutTouchingAnything",
			quiets:          []quietState{busy},
			request:         Request{Slug: "slug", QuietWait: 3 * time.Second, RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"tst:slug:driver", "lyx reed attach", "lyx reed down"},
			wantCalls:       "quiet,quiet,quiet,quiet",
			wantSleeps:      3,
		},
		{
			name:        "DriverTurningRetiringProceedsAfterCountedSleeps",
			quiets:      []quietState{busy, busy, {quiet: true}},
			request:     Request{Slug: "slug", QuietWait: time.Minute, RefuseWhenBusy: true},
			wantCalls:   "quiet,quiet,quiet,refusal,end",
			wantSleeps:  2,
			wantSession: &SessionResult{Ended: true, DriverWasLive: false},
		},
		{
			name:        "ZeroWaitWithoutRefuseProceedsAndReportsDriverWasLive",
			quiets:      []quietState{busy},
			request:     Request{Slug: "slug"},
			wantCalls:   "quiet,refusal,end",
			wantSession: &SessionResult{Ended: true, DriverWasLive: true},
		},
		{
			name:       "RefusalProbeErrorNeverEndsTheSession",
			quiets:     []quietState{{quiet: true}},
			refusalErr: refusal,
			request:    Request{Slug: "slug"},
			wantErrIs:  refusal,
			wantCalls:  "quiet,refusal",
		},
		{
			name:         "SessionEndErrorNeverRemoves",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			endErr:       errors.New("tmux down"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    "quiet,refusal,end",
		},
		{
			name:      "CallsQuietProbeEndAndRemoveInOrder",
			run:       true,
			quiets:    []quietState{{quiet: true}},
			request:   Request{Slug: "slug"},
			wantCalls: "quiet,refusal,end,remove",
		},
		{
			name:        "GonePairSkipsQuietWaitAndEndsByName",
			quiets:      []quietState{busy},
			gone:        true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls:   "refusal",
			wantSawGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSubstrates{quiets: tt.quiets, refusalErr: tt.refusalErr, endErr: tt.endErr}
			td := f.teardown()
			var sawGone bool
			if tt.gone {
				td.gone = func(string) (bool, error) { return true, nil }
				td.endSession = func(_ string, gone bool) (bool, string, error) {
					sawGone = gone
					return true, "", nil
				}
			}

			var session SessionResult
			var err error
			if tt.run {
				_, err = td.Run(context.Background(), tt.request)
			} else {
				session, err = td.EndSession(context.Background(), tt.request)
			}

			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want one wrapping %v", err, tt.wantErrIs)
				}
			case tt.wantAnyError:
				if err == nil {
					t.Fatal("error = nil, want one")
				}
			case err != nil:
				t.Fatalf("error = %v, want nil", err)
			}
			for _, want := range tt.wantErrContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err.Error(), want)
				}
			}
			if got := strings.Join(f.calls, ","); got != tt.wantCalls {
				t.Errorf("calls = %s, want %s", got, tt.wantCalls)
			}
			if f.sleeps != tt.wantSleeps {
				t.Errorf("sleeps = %d, want %d", f.sleeps, tt.wantSleeps)
			}
			if tt.wantSession != nil && (session.Ended != tt.wantSession.Ended || session.DriverWasLive != tt.wantSession.DriverWasLive) {
				t.Errorf("session = %+v, want Ended %v and DriverWasLive %v", session, tt.wantSession.Ended, tt.wantSession.DriverWasLive)
			}
			if sawGone != tt.wantSawGone {
				t.Errorf("session end told the task worktree is gone = %v, want %v", sawGone, tt.wantSawGone)
			}
		})
	}
}
