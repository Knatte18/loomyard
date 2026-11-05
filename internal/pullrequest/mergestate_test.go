// mergestate_test.go covers ReadMergeState against a scripted prkit.API and a fake clock: which pull requests merge now, what each other one names as its cause, and how long an uncomputed mergeability is waited for.

package pullrequest_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeClock is a clock whose sleeps advance it and add up, so a test never waits.
type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.slept += d
	c.now = c.now.Add(d)
	return nil
}

func TestReadMergeState(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		beamHead = "beam1"
		base     = "main"
	)
	open := func(mergeableState string) pullrequest.Status {
		return pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: beamHead, MergeableState: mergeableState}
	}
	with := func(status pullrequest.Status, change func(*pullrequest.Status)) pullrequest.Status {
		change(&status)
		return status
	}
	failedLint := []pullrequest.Check{{Name: "build", Conclusion: "success"}, {Name: "lint", Conclusion: "failure"}}
	statusErr := errors.New("github is down")

	tests := []struct {
		name string
		// statuses are the scripted Status answers, the last repeating.
		statuses []pullrequest.Status
		checks   []pullrequest.Check
		// statusErr is returned by every Status call.
		statusErr error
		wantNow   bool
		// wantCauses each appear in some cause; unwantedCauses appear in none.
		wantCauses     []string
		unwantedCauses []string
		wantSlept      time.Duration
	}{
		{name: "OpenCleanAtTheBeamHeadMerges", statuses: []pullrequest.Status{open("clean")}, wantNow: true},
		{name: "HasHooksMerges", statuses: []pullrequest.Status{open("has_hooks")}, wantNow: true},
		{name: "UnstableMerges", statuses: []pullrequest.Status{open("unstable")}, wantNow: true},
		{
			name:       "DraftNamesDraft",
			statuses:   []pullrequest.Status{with(open("draft"), func(s *pullrequest.Status) { s.Draft = true })},
			wantCauses: []string{"draft"},
		},
		{
			name:       "HeadMismatchNamesBothHeads",
			statuses:   []pullrequest.Status{with(open("clean"), func(s *pullrequest.Status) { s.HeadSHA = "other" })},
			wantCauses: []string{"head other is not the beam's head beam1"},
		},
		{name: "DirtyNamesTheConflict", statuses: []pullrequest.Status{open("dirty")}, wantCauses: []string{"dirty"}},
		{
			name:           "BlockedNamesTheFailingRequiredCheck",
			statuses:       []pullrequest.Status{open("blocked")},
			checks:         failedLint,
			wantCauses:     []string{"blocked", "required check lint is failure"},
			unwantedCauses: []string{"build"},
		},
		{
			name:           "BlockedNamesOnlyTheOwedCheckWhenAnotherSkipped",
			statuses:       []pullrequest.Status{open("blocked")},
			checks:         []pullrequest.Check{{Name: "build", Conclusion: "skipped"}, {Name: "lint", Conclusion: ""}, {Name: "deploy", Conclusion: pullrequest.CheckMissing}},
			wantCauses:     []string{"required check lint is not finished", "required check deploy is missing"},
			unwantedCauses: []string{"build"},
		},
		{name: "BehindNamesTheBase", statuses: []pullrequest.Status{open("behind")}, wantCauses: []string{"behind"}},
		{
			name:           "ClosedNamesClosedAlone",
			statuses:       []pullrequest.Status{{State: pullrequest.StateClosed, HeadSHA: beamHead, MergeableState: "unknown"}},
			wantCauses:     []string{"closed"},
			unwantedCauses: []string{"GitHub has not yet computed"},
		},
		{name: "MergedNamesMerged", statuses: []pullrequest.Status{{State: pullrequest.StateMerged, HeadSHA: beamHead, MergeableState: "clean"}}, wantCauses: []string{"merged"}},
		{
			name:      "UnknownTurningCleanWithinTheWindowMerges",
			statuses:  []pullrequest.Status{open("unknown"), open("unknown"), open("clean")},
			wantNow:   true,
			wantSlept: 3 * time.Second,
		},
		{
			name:       "UnknownPastTheWindowNamesTheUncomputedCauseAfterWaitingItOut",
			statuses:   []pullrequest.Status{open("unknown")},
			wantCauses: []string{"GitHub has not yet computed whether it can merge"},
			wantSlept:  pullrequest.MergeRetryWindow,
		},
		{name: "AFailingStatusCallIsReturned", statuses: []pullrequest.Status{open("clean")}, statusErr: statusErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &prkit.API{}
			answers := make([]prkit.Answer, len(tt.statuses))
			for i, status := range tt.statuses {
				answers[i] = prkit.Answer{Status: status, Err: tt.statusErr}
			}
			api.Script(prkit.MethodStatus, answers...)
			api.Script(prkit.MethodRequiredChecks, prkit.Answer{Checks: tt.checks})
			clock := &fakeClock{now: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}

			got, err := pullrequest.ReadMergeState(t.Context(), api, 7, beamHead, base, clock.Now, clock.Sleep)

			if tt.statusErr != nil {
				if !errors.Is(err, tt.statusErr) {
					t.Fatalf("ReadMergeState() error = %v; want it to wrap %v", err, tt.statusErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadMergeState() error = %v; want nil", err)
			}
			if got.MergeableNow != tt.wantNow || (len(got.Causes) == 0) != tt.wantNow {
				t.Errorf("MergeableNow, Causes = %t, %q; want %t with causes exactly when not mergeable", got.MergeableNow, got.Causes, tt.wantNow)
			}
			joined := strings.Join(got.Causes, "\n")
			for _, want := range tt.wantCauses {
				if !strings.Contains(joined, want) {
					t.Errorf("Causes = %q; want one containing %q", got.Causes, want)
				}
			}
			for _, unwanted := range tt.unwantedCauses {
				if strings.Contains(joined, unwanted) {
					t.Errorf("Causes = %q; want none containing %q", got.Causes, unwanted)
				}
			}
			last := tt.statuses[len(tt.statuses)-1]
			if got.State != last.State || got.Head != last.HeadSHA || got.MergeableState != last.MergeableState || got.Draft != last.Draft {
				t.Errorf("State, Head, MergeableState, Draft = %q, %q, %q, %t; want the last read %+v", got.State, got.Head, got.MergeableState, got.Draft, last)
			}
			if len(got.Checks) != len(tt.checks) {
				t.Errorf("Checks = %v; want %v", got.Checks, tt.checks)
			}
			if calls, want := api.Calls(prkit.MethodRequiredChecks), []string{base + "|" + last.HeadSHA}; len(calls) != 1 || calls[0] != want[0] {
				t.Errorf("RequiredChecks calls = %q; want %q", calls, want)
			}
			if clock.slept != tt.wantSlept {
				t.Errorf("slept %v; want %v", clock.slept, tt.wantSlept)
			}
		})
	}
}
