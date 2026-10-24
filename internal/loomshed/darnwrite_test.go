//go:build tier0

// darnwrite_test.go exercises the Darn-Write row's producer at Tier 1: a fake session and in-memory seams recording the order of their calls.

package loomshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// darnFixture is one Darn-Write producer setup: seams that log their calls and a fake session reporting a fixed verdict.
type darnFixture struct {
	deps    DarnDeps
	events  []string
	told    []DarnTold
	outcome shedengine.Outcome
	pointer shedengine.OutputPointer
	sessErr error
}

func newDarnFixture() *darnFixture {
	f := &darnFixture{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "/work/description.md"}}
	f.deps = DarnDeps{
		Commit:        func() error { f.events = append(f.events, "commit"); return nil },
		LatestOutcome: func() (DarnOutcome, bool, error) { return DarnOutcome{}, false, nil },
	}
	return f
}

func (f *darnFixture) session(told DarnTold) shedengine.ShedProducer {
	f.told = append(f.told, told)
	return darnSession{f: f}
}

type darnSession struct{ f *darnFixture }

func (s darnSession) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	s.f.events = append(s.f.events, "session")
	return s.f.outcome, s.f.pointer, s.f.sessErr
}

func TestDarnWrite_Call(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		mutate     func(*darnFixture)
		session    bool
		wantResult shedengine.Outcome
		wantErr    string
		wantReason string
		// wantReasonHas lists words the reason must hold beyond wantReason.
		wantReasonHas []string
		// wantReasonLacks is a text the reason must not hold.
		wantReasonLacks string
		wantEvents      string
		wantTold        DarnTold
	}{
		{
			name:       "first spawn tells nothing and commits",
			session:    true,
			wantResult: shedengine.Done,
			wantEvents: "session,commit",
		},
		{
			name: "prior work names the halt reason",
			mutate: func(f *darnFixture) {
				f.deps.LatestOutcome = func() (DarnOutcome, bool, error) {
					return DarnOutcome{Outcome: shedengine.Stuck, Reason: "verify failed"}, true, nil
				}
			},
			session:    true,
			wantResult: shedengine.Done,
			wantEvents: "session,commit",
			wantTold: DarnTold{PriorWork: "An earlier spawn of this task already began the change, and its work is committed on the task branch.\n" +
				"Read that work first and continue it instead of starting over.\n" +
				"The run halted there with: verify failed"},
		},
		{
			name: "gate-failed Stuck commits, keeps the record and appends the way forward",
			mutate: func(f *darnFixture) {
				f.outcome = shedengine.Stuck
				f.pointer = shedengine.OutputPointer{Path: "/work/description.md", Reason: "verify gate failed: see /work/verify-1.log"}
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReason:      "verify gate failed: see /work/verify-1.log; " + darnResumeWayForward,
			wantReasonHas:   []string{"lyx loom resume"},
			wantReasonLacks: "Merge-In",
			wantEvents:      "session,commit",
		},
		{
			name: "Stuck without a pointer is neither committed nor amended",
			mutate: func(f *darnFixture) {
				f.outcome = shedengine.Stuck
				f.pointer = shedengine.OutputPointer{Reason: "no output"}
			},
			session:    true,
			wantResult: shedengine.Stuck,
			wantReason: "no output",
			wantEvents: "session",
		},
		{
			name: "latest outcome read failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.LatestOutcome = func() (DarnOutcome, bool, error) { return DarnOutcome{}, false, errors.New("status gone") }
			},
			wantErr: "status gone",
		},
		{
			name: "commit failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.Commit = func() error { return errors.New("git fault") }
			},
			session:    true,
			wantErr:    "git fault",
			wantEvents: "session",
		},
		{
			name:       "session error is returned without a commit",
			mutate:     func(f *darnFixture) { f.sessErr = errors.New("shuttle died") },
			session:    true,
			wantErr:    "shuttle died",
			wantEvents: "session",
		},
		{
			name:    "unwired Commit seam is named",
			mutate:  func(f *darnFixture) { f.deps.Commit = nil },
			wantErr: "no Commit seam wired",
		},
		{
			name:    "unwired LatestOutcome seam is named",
			mutate:  func(f *darnFixture) { f.deps.LatestOutcome = nil },
			wantErr: "no LatestOutcome seam wired",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDarnFixture()
			if tc.mutate != nil {
				tc.mutate(f)
			}
			outcome, pointer, err := NewDarnWrite(NameDarnWrite, f.session, f.deps).Call(context.Background())

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Call() error = %v; want it to contain %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			} else if outcome != tc.wantResult {
				t.Errorf("Call() outcome = %q; want %q", outcome, tc.wantResult)
			}
			if tc.wantReason != "" && !strings.Contains(pointer.Reason, tc.wantReason) {
				t.Errorf("Call() reason = %q; want it to contain %q", pointer.Reason, tc.wantReason)
			}
			for _, word := range tc.wantReasonHas {
				if !strings.Contains(pointer.Reason, word) {
					t.Errorf("Call() reason = %q; want it to contain %q", pointer.Reason, word)
				}
			}
			if tc.wantReasonLacks != "" && strings.Contains(pointer.Reason, tc.wantReasonLacks) {
				t.Errorf("Call() reason = %q; want it not to contain %q", pointer.Reason, tc.wantReasonLacks)
			}
			if pointer.Reroute {
				t.Errorf("Call() Reroute = true; want false")
			}
			if got := strings.Join(f.events, ","); got != tc.wantEvents {
				t.Errorf("events = %q; want %q", got, tc.wantEvents)
			}
			if !tc.session {
				if len(f.told) != 0 {
					t.Errorf("session built %d times; want none", len(f.told))
				}
				return
			}
			if len(f.told) != 1 || f.told[0] != tc.wantTold {
				t.Errorf("told = %+v; want one %+v", f.told, tc.wantTold)
			}
		})
	}
}

func TestDarnWrite_CancelledContextRefusesBeforeAnySeam(t *testing.T) {
	t.Parallel()
	f := newDarnFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewDarnWrite(NameDarnWrite, f.session, f.deps).Call(ctx); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v; want context.Canceled", err)
	}
	if len(f.events) != 0 {
		t.Errorf("events = %v; want none", f.events)
	}
}

func TestRenderPriorWork(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		latest DarnOutcome
		found  bool
		want   string
	}{
		{"nothing to name", DarnOutcome{}, false, ""},
		{"outcome without a reason", DarnOutcome{Outcome: shedengine.Done}, true, ""},
		{"reason of an entry the read did not find is ignored", DarnOutcome{Reason: "stale"}, false, ""},
		{"reason only", DarnOutcome{Reason: "halted"}, true, "An earlier spawn of this task already began the change, and its work is committed on the task branch.\nRead that work first and continue it instead of starting over.\nThe run halted there with: halted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := renderPriorWork(tc.latest, tc.found); got != tc.want {
				t.Errorf("renderPriorWork() = %q; want %q", got, tc.want)
			}
		})
	}
}
