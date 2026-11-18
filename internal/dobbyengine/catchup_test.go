// catchup_test.go pins the catch-up over fake seams: what it fetches, pushes, hands over and writes, and when it stops.

package dobbyengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// catchUpFake is a set of seams over in-memory runs that records each act in calls.
type catchUpFake struct {
	boltErr     error
	registerErr error
	// slugs is what the Runs seam lists, in order.
	slugs []string
	runs  map[string]CatchUpRun
	// fetchErrs, pushErrs and handOverErrs fail the call for the named slug.
	fetchErrs    map[string]error
	pushErrs     map[string]error
	handOverErrs map[string]error
	calls        []string
}

func (f *catchUpFake) deps() CatchUpDeps {
	return CatchUpDeps{
		PullBolt: func() error {
			f.calls = append(f.calls, "pull")
			return f.boltErr
		},
		Register: func() error {
			f.calls = append(f.calls, "register")
			return f.registerErr
		},
		Runs: func() ([]string, error) { return f.slugs, nil },
		FetchRun: func(slug string) error {
			f.calls = append(f.calls, "fetch "+slug)
			return f.fetchErrs[slug]
		},
		ReadRun: func(slug string) (CatchUpRun, error) { return f.runs[slug], nil },
		PushOwed: func(slug string) error {
			f.calls = append(f.calls, "push "+slug)
			return f.pushErrs[slug]
		},
		HandOver: func(slug, owner string) error {
			f.calls = append(f.calls, fmt.Sprintf("handover %s %s", slug, owner))
			return f.handOverErrs[slug]
		},
		ApplyPauseIntents: func(slugs []string) error {
			f.calls = append(f.calls, "pauses "+strings.Join(slugs, ","))
			return nil
		},
	}
}

func TestCatchUp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	errPush := errors.New("push refused")
	errHandOver := errors.New("teardown lock busy")
	errRefused := errors.New("registration refused")
	mixedRuns := map[string]CatchUpRun{
		"here-ahead": {Owner: "m1", Ahead: true},
		"here-level": {Owner: "m1"},
		"away":       {Owner: "m2", OwnedElsewhere: true, Ahead: true},
	}

	tests := []struct {
		name        string
		fake        catchUpFake
		wantCalls   []string
		wantReport  CatchUpReport
		wantErrIs   error
		wantErrText string
	}{
		{
			name:      "AnUnreachableBoltFetchActsOnNoRun",
			fake:      catchUpFake{boltErr: ErrOriginUnreachable, slugs: []string{"here-ahead"}, runs: mixedRuns},
			wantCalls: []string{"pull"},
			wantErrIs: ErrOriginUnreachable,
		},
		{
			name:      "ARefusedRegistrationStopsBeforeTheRuns",
			fake:      catchUpFake{registerErr: errRefused, slugs: []string{"here-ahead"}, runs: mixedRuns},
			wantCalls: []string{"pull", "register"},
			wantErrIs: errRefused,
		},
		{
			name:      "ARunAheadIsPushedAndALevelOneIsNot",
			fake:      catchUpFake{slugs: []string{"here-ahead", "here-level"}, runs: mixedRuns},
			wantCalls: []string{"pull", "register", "fetch here-ahead", "fetch here-level", "push here-ahead", "pauses here-ahead,here-level"},
			wantReport: CatchUpReport{
				Pushed:    []string{"here-ahead"},
				OwnedHere: []string{"here-ahead", "here-level"},
			},
		},
		{
			name: "ARunOwnedElsewhereIsHandedOverNamingItsOwnerAndGetsNoPush",
			fake: catchUpFake{slugs: []string{"away", "here-level"}, runs: mixedRuns},
			wantCalls: []string{
				"pull", "register", "fetch away", "fetch here-level", "handover away m2", "pauses here-level",
			},
			wantReport: CatchUpReport{HandedOver: []string{"away"}, OwnedHere: []string{"here-level"}},
		},
		{
			name: "AnUnreachablePairFetchStopsBeforeAnyPush",
			fake: catchUpFake{
				slugs: []string{"here-ahead", "away"}, runs: mixedRuns,
				fetchErrs: map[string]error{"away": ErrOriginUnreachable},
			},
			wantCalls: []string{"pull", "register", "fetch here-ahead", "fetch away"},
			wantErrIs: ErrOriginUnreachable,
		},
		{
			name: "AFailedHandOverGoesOnToTheNextRunAndIsReturned",
			fake: catchUpFake{
				slugs: []string{"away", "here-ahead"}, runs: mixedRuns,
				handOverErrs: map[string]error{"away": errHandOver},
			},
			wantCalls: []string{
				"pull", "register", "fetch away", "fetch here-ahead", "handover away m2", "push here-ahead", "pauses here-ahead",
			},
			wantReport:  CatchUpReport{Pushed: []string{"here-ahead"}, OwnedHere: []string{"here-ahead"}},
			wantErrIs:   errHandOver,
			wantErrText: "run away",
		},
		{
			name: "AFailedPushGoesOnToTheNextRunAndIsReturned",
			fake: catchUpFake{
				slugs: []string{"here-ahead", "away"}, runs: mixedRuns,
				pushErrs: map[string]error{"here-ahead": errPush},
			},
			wantCalls: []string{
				"pull", "register", "fetch here-ahead", "fetch away", "push here-ahead", "handover away m2", "pauses here-ahead",
			},
			wantReport:  CatchUpReport{HandedOver: []string{"away"}, OwnedHere: []string{"here-ahead"}},
			wantErrIs:   errPush,
			wantErrText: "run here-ahead",
		},
		{
			name:      "AScopeOfOneSlugFetchesOnlyThatSlug",
			fake:      catchUpFake{slugs: []string{"here-level"}, runs: mixedRuns},
			wantCalls: []string{"pull", "register", "fetch here-level", "pauses here-level"},
			wantReport: CatchUpReport{
				OwnedHere: []string{"here-level"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report, err := CatchUp(tt.fake.deps())
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("CatchUp() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if tt.wantErrIs == nil && err != nil {
				t.Errorf("CatchUp() error = %v; want none", err)
			}
			if tt.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrText)) {
				t.Errorf("CatchUp() error = %v; want it to contain %q", err, tt.wantErrText)
			}
			if !slices.Equal(tt.fake.calls, tt.wantCalls) {
				t.Errorf("CatchUp() calls = %q; want %q", tt.fake.calls, tt.wantCalls)
			}
			if !slices.Equal(report.Pushed, tt.wantReport.Pushed) ||
				!slices.Equal(report.HandedOver, tt.wantReport.HandedOver) ||
				!slices.Equal(report.OwnedHere, tt.wantReport.OwnedHere) {
				t.Errorf("CatchUp() report = %+v; want %+v", report, tt.wantReport)
			}
		})
	}

	t.Run("ANilSeamIsAWiringErrorActingOnNothing", func(t *testing.T) {
		t.Parallel()
		fake := catchUpFake{slugs: []string{"here-ahead"}, runs: mixedRuns}
		deps := fake.deps()
		deps.HandOver = nil
		if _, err := CatchUp(deps); err == nil || !strings.Contains(err.Error(), "HandOver") {
			t.Errorf("CatchUp() error = %v; want a wiring error naming HandOver", err)
		}
		if len(fake.calls) != 0 {
			t.Errorf("CatchUp() calls = %q; want none", fake.calls)
		}
	})
}
