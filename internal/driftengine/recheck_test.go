// recheck_test.go — tier 0 tests for the Drift-Recheck producer over the fake Deps the baseline tests use and scripted compares.

package driftengine

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recheckFixture is the fake world one DriftRecheck pass runs in.
type recheckFixture struct {
	*baselineFixture

	// baseline and accepted are what the compare against the baseline and the one against the accepted profile answer.
	baseline, accepted CompareResult

	baselineCalls int
	acceptedCalls int
}

const (
	// recheckHead is the fixture's HEAD, a commit after the compare the seeded scope record holds.
	recheckHead = "ccc"
	// recheckComparedHead is the commit of the last compare the seeded scope record holds.
	recheckComparedHead = "bbb"
)

// comparedScope returns a scope record whose compare ran at recheckComparedHead against the baseline taken at base.
func comparedScope() ScopeRecord {
	return ScopeRecord{
		Scope:         ScopePartial,
		Skip:          []string{"slow"},
		AwaitedAt:     "2026-01-01T00:00:00Z",
		BaselineHead:  "base",
		ComparedHead:  recheckComparedHead,
		ComparedAt:    "2026-01-01T00:00:00Z",
		ComparePassed: true,
	}
}

// acceptingScope returns comparedScope with t1 and t2 accepted and the accepted profile captured at profileHead.
func acceptingScope(profileHead string) ScopeRecord {
	scope := comparedScope()
	scope.Accepted = []Acceptance{{Test: "t1", By: AcceptedByAgent}, {Test: "t2", By: AcceptedByAgent}}
	scope.AcceptedProfileHead = profileHead
	return scope
}

// newRecheckFixture returns a fixture at recheckHead with a declared compare and scope as its scope record, which nil leaves unwritten.
func newRecheckFixture(t *testing.T, scope *ScopeRecord) *recheckFixture {
	t.Helper()
	f := &recheckFixture{baselineFixture: newBaselineFixture(t), baseline: passingCompare, accepted: passingCompare}
	f.head = recheckHead
	if scope != nil {
		f.seedScopeRecord(*scope, false)
	}
	d := &f.deps
	d.Config = func() (Config, error) {
		return Config{Enabled: true, Compare: CommandStep{Command: "compare {baseline_dir}", Dir: ".", TimeoutMin: 1}}, nil
	}
	d.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 700_000_000, time.UTC) }
	d.Compare = func(context.Context) (CompareResult, error) {
		f.baselineCalls++
		return f.baseline, nil
	}
	d.CompareAccepted = func(context.Context) (CompareResult, error) {
		f.acceptedCalls++
		return f.accepted, nil
	}
	return f
}

// call runs one DriftRecheck pass.
func (f *recheckFixture) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return NewDriftRecheck("Drift-Recheck", f.deps).Call(context.Background())
}

// scope reads the scope record.
func (f *recheckFixture) scope() ScopeRecord {
	f.t.Helper()
	scope, _ := mustReadScope(f.t, f.paths)
	return scope
}

// requireOutcome runs one pass and checks it ended in want with a reason naming every fragment, and that a Stuck reason opens with the recheck opening.
func (f *recheckFixture) requireOutcome(want shedengine.Outcome, fragments ...string) shedengine.OutputPointer {
	f.t.Helper()
	outcome, pointer, err := f.call()
	if err != nil || outcome != want {
		f.t.Fatalf("Call() = %q, %v (%s); want %q", outcome, err, pointer.Reason, want)
	}
	if got := strings.HasPrefix(pointer.Reason, recheckOpening); got != (want == shedengine.Stuck) {
		f.t.Errorf("reason %q opens with %q = %v; want %v", pointer.Reason, recheckOpening, got, want == shedengine.Stuck)
	}
	for _, fragment := range fragments {
		if !strings.Contains(pointer.Reason, fragment) {
			f.t.Errorf("reason %q does not name %q", pointer.Reason, fragment)
		}
	}
	return pointer
}

// requireCommittedOnce checks the pass committed the durable drift directory once.
func (f *recheckFixture) requireCommittedOnce() {
	f.t.Helper()
	if len(f.commits) != 1 || !slices.Equal(f.commits[0], []string{DirRel()}) {
		f.t.Errorf("commits = %v; want the durable directory once", f.commits)
	}
}

// requireCompares checks how many compares against the baseline and against the accepted profile ran.
func (f *recheckFixture) requireCompares(baseline, accepted int) {
	f.t.Helper()
	if f.baselineCalls != baseline || f.acceptedCalls != accepted {
		f.t.Errorf("compares against the baseline %d and the accepted profile %d; want %d and %d", f.baselineCalls, f.acceptedCalls, baseline, accepted)
	}
}

// TestDriftRecheck covers the passes that need no compare, the Stuck of a pass that cannot compare, the compare against the baseline and the one against the accepted profile, and the lapse of acceptances no profile backs.
func TestDriftRecheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const compareLog = "compare-1.log"
	drifted := CompareResult{ExitCode: 1, Log: compareLog, Reason: "exit 1; log " + compareLog}

	t.Run("a pass that needs no compare is Done", func(t *testing.T) {
		t.Parallel()
		atComparedHead := comparedScope()
		atComparedHead.ComparedHead = recheckHead
		off := comparedScope()
		off.Scope = ScopeOff
		waived := comparedScope()
		waived.WaivedAt, waived.WaivedReason = "2026-01-02T03:04:05Z", "no baseline possible"
		rows := []struct {
			name  string
			scope *ScopeRecord
		}{
			{"no scope record", nil},
			{"scope off", &off},
			{"a waived scope", &waived},
			{"HEAD equal to the compared head", &atComparedHead},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				f := newRecheckFixture(t, row.scope)
				f.requireOutcome(shedengine.Done)
				f.requireCompares(0, 0)
				f.requireCommittedOnce()
			})
		}
	})

	t.Run("drift switched off is Done with no compare and no commit", func(t *testing.T) {
		t.Parallel()
		scope := comparedScope()
		f := newRecheckFixture(t, &scope)
		f.deps.Config = func() (Config, error) { return Config{}, nil }
		outcome, pointer, err := f.call()
		if err != nil || outcome != shedengine.Done || !strings.Contains(pointer.Reason, "drift.enabled = false") {
			t.Fatalf("Call() = %q, %v (%s); want Done naming the switch", outcome, err, pointer.Reason)
		}
		f.requireCompares(0, 0)
		if len(f.commits) != 0 {
			t.Errorf("commits = %v; want none", f.commits)
		}
	})

	t.Run("a pass that cannot compare is Stuck with no compare", func(t *testing.T) {
		t.Parallel()
		neverCompared := comparedScope()
		neverCompared.ComparedAt = ""
		rows := []struct {
			name      string
			scope     ScopeRecord
			arrange   func(f *recheckFixture)
			fragments []string
		}{
			{"no compare recorded", neverCompared, func(*recheckFixture) {}, []string{"no compare is recorded", "Drift-Verify"}},
			{"a dirty tree", comparedScope(), func(f *recheckFixture) { f.dirty = []string{"a.go", "b.go"} }, []string{"dirty", "a.go, b.go"}},
			{"an undeclared compare", comparedScope(), func(f *recheckFixture) {
				f.deps.Config = func() (Config, error) { return Config{Enabled: true}, nil }
			}, []string{"lym config drift"}},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				f := newRecheckFixture(t, &row.scope)
				row.arrange(f)
				f.requireOutcome(shedengine.Stuck, row.fragments...)
				f.requireCompares(0, 0)
				f.requireCommittedOnce()
				if got := f.scope(); got.ComparedHead != recheckComparedHead || !got.ComparePassed {
					t.Errorf("scope compare fields = %q, %v; want the earlier compare untouched", got.ComparedHead, got.ComparePassed)
				}
			})
		}
	})

	t.Run("with no test accepted", func(t *testing.T) {
		t.Parallel()

		t.Run("a passing compare against the baseline is Done and records the compare at HEAD", func(t *testing.T) {
			t.Parallel()
			scope := comparedScope()
			f := newRecheckFixture(t, &scope)
			f.requireOutcome(shedengine.Done, "baseline", recheckHead)
			f.requireCompares(1, 0)
			f.requireCommittedOnce()
			got := f.scope()
			requireCompareRecorded(t, got, "base", recheckHead, true)
			if got.AcceptedProfileHead != "" {
				t.Errorf("accepted_profile_head = %q; want none", got.AcceptedProfileHead)
			}
		})

		t.Run("a failing compare against the baseline is Stuck and records the failure", func(t *testing.T) {
			t.Parallel()
			scope := comparedScope()
			f := newRecheckFixture(t, &scope)
			f.baseline = drifted
			f.requireOutcome(shedengine.Stuck, "baseline", drifted.Reason, recheckComparedHead)
			f.requireCompares(1, 0)
			f.requireCommittedOnce()
			requireCompareRecorded(t, f.scope(), "base", recheckHead, false)
		})
	})

	t.Run("with tests accepted and the profile captured at the compared head", func(t *testing.T) {
		t.Parallel()

		t.Run("a passing compare against the accepted profile is Done and moves both heads to HEAD", func(t *testing.T) {
			t.Parallel()
			scope := acceptingScope(recheckComparedHead)
			f := newRecheckFixture(t, &scope)
			f.requireOutcome(shedengine.Done, "accepted profile", recheckHead)
			f.requireCompares(0, 1)
			f.requireCommittedOnce()
			got := f.scope()
			requireCompareRecorded(t, got, "base", recheckHead, true)
			if got.AcceptedProfileHead != recheckHead {
				t.Errorf("accepted_profile_head = %q; want %q", got.AcceptedProfileHead, recheckHead)
			}
			if !slices.Equal(got.Accepted, scope.Accepted) {
				t.Errorf("accepted = %+v; want the acceptances kept as %+v", got.Accepted, scope.Accepted)
			}
		})

		t.Run("a failing compare is Stuck naming the accepted tests and lapses every acceptance", func(t *testing.T) {
			t.Parallel()
			scope := acceptingScope(recheckComparedHead)
			f := newRecheckFixture(t, &scope)
			f.accepted = drifted
			f.requireOutcome(shedengine.Stuck, "accepted profile", drifted.Reason, "`t1`", "`t2`", "lapse")
			f.requireCompares(0, 1)
			f.requireCommittedOnce()
			got := f.scope()
			requireCompareRecorded(t, got, "base", recheckHead, false)
			if len(got.Accepted) != 0 || got.AcceptedProfileHead != "" {
				t.Errorf("accepted = %+v, accepted_profile_head = %q; want every acceptance lapsed", got.Accepted, got.AcceptedProfileHead)
			}
		})
	})

	t.Run("tests accepted with the profile captured at another commit lapse with no compare", func(t *testing.T) {
		t.Parallel()
		for _, profileHead := range []string{"", "older"} {
			scope := acceptingScope(profileHead)
			f := newRecheckFixture(t, &scope)
			f.requireOutcome(shedengine.Stuck, "no accepted profile", recheckComparedHead, "`t1`", "`t2`", "lapse")
			f.requireCompares(0, 0)
			f.requireCommittedOnce()
			got := f.scope()
			if len(got.Accepted) != 0 || got.AcceptedProfileHead != "" {
				t.Errorf("profile head %q: accepted = %+v, accepted_profile_head = %q; want every acceptance lapsed", profileHead, got.Accepted, got.AcceptedProfileHead)
			}
			if got.ComparedHead != recheckComparedHead || !got.ComparePassed {
				t.Errorf("profile head %q: compared head %q, passed %v; want the earlier compare untouched", profileHead, got.ComparedHead, got.ComparePassed)
			}
		}
	})
}
