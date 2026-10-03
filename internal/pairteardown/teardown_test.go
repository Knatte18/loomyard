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

func TestEndSession_BusyDriverAtBoundRefusesWithoutTouchingAnything(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{busy}}
	_, err := f.teardown().EndSession(context.Background(), Request{Slug: "slug", QuietWait: 3 * time.Second, RefuseWhenBusy: true})

	if !errors.Is(err, ErrDriverBusy) {
		t.Fatalf("EndSession error = %v, want ErrDriverBusy", err)
	}
	for _, want := range []string{"tst:slug:driver", "lyx reed attach", "lyx reed down"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("busy error %q lacks %q", err.Error(), want)
		}
	}
	for _, call := range f.calls {
		if call != "quiet" {
			t.Errorf("busy refusal made call %q, want only quiet probes", call)
		}
	}
	if wantProbes := 4; len(f.calls) != wantProbes || f.sleeps != wantProbes-1 {
		t.Errorf("probes = %d, sleeps = %d, want %d probes and %d sleeps", len(f.calls), f.sleeps, wantProbes, wantProbes-1)
	}
}

func TestEndSession_DriverTurningRetiringProceedsAfterCountedSleeps(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{busy, busy, {quiet: true}}}
	res, err := f.teardown().EndSession(context.Background(), Request{Slug: "slug", QuietWait: time.Minute, RefuseWhenBusy: true})
	if err != nil {
		t.Fatalf("EndSession = %v, want nil", err)
	}
	if f.sleeps != 2 {
		t.Errorf("sleeps = %d, want 2", f.sleeps)
	}
	if res.DriverWasLive || !res.Ended {
		t.Errorf("result = %+v, want Ended and not DriverWasLive", res)
	}
}

func TestEndSession_ZeroWaitWithoutRefuseProceedsAndReportsDriverWasLive(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{busy}}
	res, err := f.teardown().EndSession(context.Background(), Request{Slug: "slug"})
	if err != nil {
		t.Fatalf("EndSession = %v, want nil", err)
	}
	if !res.DriverWasLive || !res.Ended {
		t.Errorf("result = %+v, want Ended and DriverWasLive", res)
	}
	if f.sleeps != 0 {
		t.Errorf("sleeps = %d, want 0", f.sleeps)
	}
}

func TestEndSession_RefusalProbeErrorNeverEndsTheSession(t *testing.T) {
	refusal := errors.New("worktree has uncommitted changes")
	f := &fakeSubstrates{quiets: []quietState{{quiet: true}}, refusalErr: refusal}
	_, err := f.teardown().EndSession(context.Background(), Request{Slug: "slug"})
	if !errors.Is(err, refusal) {
		t.Fatalf("EndSession error = %v, want the probe's refusal unchanged", err)
	}
	if got := strings.Join(f.calls, ","); got != "quiet,refusal" {
		t.Errorf("calls = %s, want quiet,refusal", got)
	}
}

func TestRun_SessionEndErrorNeverRemoves(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{{quiet: true}}, endErr: errors.New("tmux down")}
	if _, err := f.teardown().Run(context.Background(), Request{Slug: "slug"}); err == nil {
		t.Fatal("Run = nil, want the session-end error")
	}
	if got := strings.Join(f.calls, ","); got != "quiet,refusal,end" {
		t.Errorf("calls = %s, want quiet,refusal,end", got)
	}
}

func TestRun_CallsQuietProbeEndAndRemoveInOrder(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{{quiet: true}}}
	if _, err := f.teardown().Run(context.Background(), Request{Slug: "slug"}); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if got := strings.Join(f.calls, ","); got != "quiet,refusal,end,remove" {
		t.Errorf("calls = %s, want quiet,refusal,end,remove", got)
	}
}

func TestEndSession_GonePairSkipsQuietWaitAndEndsByName(t *testing.T) {
	f := &fakeSubstrates{quiets: []quietState{busy}}
	td := f.teardown()
	td.gone = func(string) (bool, error) { return true, nil }
	var sawGone bool
	td.endSession = func(_ string, gone bool) (bool, string, error) {
		sawGone = gone
		return true, "", nil
	}
	if _, err := td.EndSession(context.Background(), Request{Slug: "slug", RefuseWhenBusy: true}); err != nil {
		t.Fatalf("EndSession = %v, want nil", err)
	}
	if !sawGone {
		t.Error("session end was not told the task worktree is gone")
	}
	if got := strings.Join(f.calls, ","); got != "refusal" {
		t.Errorf("calls = %s, want only the refusal probe", got)
	}
}
