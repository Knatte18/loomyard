// holdcheck_test.go pins the hold check's flow over fakes of every seam:
// which seams a call reaches, in which order, and what each end leaves standing.

package adoptcli

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holdRead is what the fake Hold seam answers for one read.
type holdRead struct {
	hold  boardengine.Hold
	own   bool
	found bool
}

// holdCheckFixture fakes every seam of one hold check and records the calls in order.
// The Hold seam answers holds in turn and repeats the last; the pull answers pullErrs in turn, then nil.
type holdCheckFixture struct {
	deps holdCheckDeps

	calls        []string
	notices      []string
	limitNotices []string

	pullErrs   []error
	pulls      int
	holds      []holdRead
	holdReads  int
	records    map[string]CheckRecord
	notRan     map[string]bool
	fixTask    string
	fixLines   []string
	fixLost    *boardengine.HoldRefusal
	fixTries   []int
	cleared    bool
	clearErr   error
	dropErr    error
	beamUpErrs map[string]error
}

// holdOf returns a hold of this machine over subjects, each with the tries given beside it in tries.
func holdOf(tries map[string]int, subjects ...string) boardengine.Hold {
	hold := boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: "boxA"}
	for _, subject := range subjects {
		hold.Subjects = append(hold.Subjects, boardengine.HoldSubject{Subject: subject, Tries: tries[subject]})
	}
	return hold
}

// greenRecord returns the record of a check that ended green.
func greenRecord(subject string) CheckRecord { return CheckRecord{Subject: subject, State: checkGreen} }

// failedSubjectRecord returns the record of subject's check that failed at tier 0.
func failedSubjectRecord(subject string) CheckRecord {
	record := failedRecord()
	record.Subject = subject
	return record
}

// newHoldCheckFixture returns a fixture over a hold of this machine on `alpha` and `beta`, every check green.
func newHoldCheckFixture() *holdCheckFixture {
	f := &holdCheckFixture{
		holds:      []holdRead{{hold: holdOf(nil, "alpha", "beta"), own: true, found: true}},
		records:    map[string]CheckRecord{},
		notRan:     map[string]bool{},
		cleared:    true,
		beamUpErrs: map[string]error{},
	}
	f.deps = holdCheckDeps{
		PullBolt: func() error {
			f.calls = append(f.calls, "pull")
			defer func() { f.pulls++ }()
			if f.pulls < len(f.pullErrs) {
				return f.pullErrs[f.pulls]
			}
			return nil
		},
		Hold: func(string) (boardengine.Hold, bool, bool, error) {
			f.calls = append(f.calls, "hold")
			read := f.holds[min(f.holdReads, len(f.holds)-1)]
			f.holdReads++
			return read.hold, read.own, read.found, nil
		},
		Check: func(_ context.Context, subject, _ string) (CheckRecord, bool, error) {
			f.calls = append(f.calls, "check:"+subject)
			if record, set := f.records[subject]; set {
				return record, !f.notRan[subject], nil
			}
			return greenRecord(subject), !f.notRan[subject], nil
		},
		Fix: func(subject string, tries int, _ CheckRecord, _ string) (string, []string, *boardengine.HoldRefusal) {
			f.calls = append(f.calls, "fix:"+subject)
			f.fixTries = append(f.fixTries, tries)
			return f.fixTask, f.fixLines, f.fixLost
		},
		Green: func(subject, _ string) []string {
			f.calls = append(f.calls, "green:"+subject)
			return nil
		},
		ClearHold: func() (bool, error) {
			f.calls = append(f.calls, "clear")
			return f.cleared, f.clearErr
		},
		DropBoltHold: func() error {
			f.calls = append(f.calls, "drop")
			return f.dropErr
		},
		BeamUp: func(subject string) error {
			f.calls = append(f.calls, "beamup:"+subject)
			return f.beamUpErrs[subject]
		},
		Notice: func(_, line string) error {
			f.notices = append(f.notices, line)
			return nil
		},
		LimitNotice: func(_, line string) error {
			f.limitNotices = append(f.limitNotices, line)
			return nil
		},
	}
	return f
}

// reached reports whether the fixture recorded a call that starts with prefix.
func (f *holdCheckFixture) reached(prefix string) bool {
	return slices.ContainsFunc(f.calls, func(call string) bool { return strings.HasPrefix(call, prefix) })
}

func TestRunHoldCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const rerun = "lym adopt alpha beta"
	otherMachinesHold := holdRead{hold: boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: "otherbox", Subjects: []boardengine.HoldSubject{{Subject: "alpha"}, {Subject: "beta"}}}, found: true}

	t.Run("NoOwnHoldEndsQuietlyNamingTheAdopt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			holds []holdRead
			wants []string
		}{
			{"NoHoldStands", []holdRead{{}}, []string{"no hold of this machine stands for alpha", "nothing was checked", "run lym adopt alpha"}},
			{"AnotherMachinesHoldStands", []holdRead{otherMachinesHold}, []string{"no hold of this machine stands for alpha", "otherbox"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newHoldCheckFixture()
				f.holds = tt.holds

				result, err := runHoldCheck(context.Background(), f.deps, "alpha")

				if err != nil || result.State != holdStateNoHold || result.Green || result.Ran {
					t.Errorf("runHoldCheck() = %+v, %v; want no_hold, not green, nothing run and no error", result, err)
				}
				if !slices.Equal(f.calls, []string{"pull", "hold"}) {
					t.Errorf("calls = %q; want the pull and the hold read alone", f.calls)
				}
				if !mentions(result.Lines, tt.wants...) || !slices.Equal(f.notices, result.Lines) {
					t.Errorf("lines = %q, notices = %q; want one line naming %q, posted", result.Lines, f.notices, tt.wants)
				}
			})
		}
	})

	t.Run("APullThatFailsReturnsItsError", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.pullErrs = []error{errors.New("origin down")}

		_, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err == nil || !mentions([]string{err.Error()}, "origin down", "lym adopt alpha", "once origin answers") {
			t.Errorf("runHoldCheck() error = %v; want it to name the cause and the re-run once origin answers", err)
		}
		if !slices.Equal(f.calls, []string{"pull"}) {
			t.Errorf("calls = %q; want the pull alone", f.calls)
		}
	})

	t.Run("ASubjectOutsideTheHoldRefusesNamingTheHoldsSubjects", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()

		_, err := runHoldCheck(context.Background(), f.deps, "gamma")

		if err == nil || !mentions([]string{err.Error()}, "gamma", "alpha, beta", rerun) {
			t.Errorf("runHoldCheck() error = %v; want it to name gamma, the hold's subjects and %q", err, rerun)
		}
		if f.reached("check:") {
			t.Errorf("calls = %q; want no check run", f.calls)
		}
	})

	t.Run("AllGreenRemovesFixTasksClearsTheHoldBringsEachSubjectUpAndDropsBoltHoldEntriesLast", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()

		result, err := runHoldCheck(context.Background(), f.deps, "beta")

		if err != nil || !result.Green || !result.Ran || result.State != checkGreen || result.HoldLost {
			t.Errorf("runHoldCheck() = %+v, %v; want green with every check run", result, err)
		}
		want := []string{"pull", "hold", "check:beta", "check:alpha", "pull", "hold", "green:alpha", "green:beta", "clear", "beamup:alpha", "beamup:beta", "drop"}
		if !slices.Equal(f.calls, want) {
			t.Errorf("calls = %q; want %q", f.calls, want)
		}
		if result.States["alpha"] != checkGreen || result.States["beta"] != checkGreen {
			t.Errorf("states = %v; want each subject green", result.States)
		}
	})

	t.Run("AHoldTakenOverDuringAGreenCheckBringsNothingUp", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			cleared  bool
			clearErr error
			wants    []string
		}{
			{"ClearHoldRefusesNamingTheOtherMachine", true, &boardengine.HoldRefusal{Reason: "machine \"otherbox\" holds the hub", Machine: "otherbox"}, []string{"otherbox", "took it"}},
			{"ClearHoldReportsNoHoldStood", false, nil, []string{"cleared meanwhile"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newHoldCheckFixture()
				f.cleared, f.clearErr = tt.cleared, tt.clearErr

				result, err := runHoldCheck(context.Background(), f.deps, "alpha")

				if err != nil || result.Green || !result.HoldLost || result.State != holdStateLost || !mentions(result.Lines, tt.wants...) {
					t.Errorf("runHoldCheck() = %+v, %v; want the hold lost, not green, with a line naming %q", result, err, tt.wants)
				}
				if f.reached("beamup:") || f.reached("drop") {
					t.Errorf("calls = %q; want no beam up and no bolt-hold drop", f.calls)
				}
			})
		}
	})

	t.Run("AHoldTakenOverBeforeTheGreenEndLeavesTheFixTaskStanding", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.holds = []holdRead{f.holds[0], otherMachinesHold}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Green || !result.HoldLost || !mentions(result.Lines, "otherbox") {
			t.Errorf("runHoldCheck() = %+v, %v; want the hold lost to otherbox", result, err)
		}
		for _, prefix := range []string{"green:", "clear", "beamup:", "drop"} {
			if f.reached(prefix) {
				t.Errorf("calls = %q; want no %q call", f.calls, prefix)
			}
		}
	})

	t.Run("AHoldTakenOverDuringAFailingCheckBringsNothingUp", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.records["alpha"] = failedSubjectRecord("alpha")
		f.fixLost = &boardengine.HoldRefusal{Reason: "machine \"otherbox\" holds the hub", Machine: "otherbox"}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || !result.HoldLost || result.Green || result.State != holdStateLost {
			t.Errorf("runHoldCheck() = %+v, %v; want the hold lost", result, err)
		}
		if f.reached("beamup:") {
			t.Errorf("calls = %q; want no beam up", f.calls)
		}
	})

	t.Run("ARefusalOverThisMachinesChangedHoldIsNotLost", func(t *testing.T) {
		t.Parallel()
		t.Run("ClearHoldRefuses", func(t *testing.T) {
			t.Parallel()
			f := newHoldCheckFixture()
			f.clearErr = &boardengine.HoldRefusal{Reason: "this machine's hold still stands", OwnStands: true}

			result, err := runHoldCheck(context.Background(), f.deps, "alpha")

			if err != nil || result.Green || result.HoldLost || !mentions(result.Lines, rerun) {
				t.Errorf("runHoldCheck() = %+v, %v; want not green, the hold not lost and a line naming %q", result, err, rerun)
			}
			if f.reached("beamup:") {
				t.Errorf("calls = %q; want no beam up", f.calls)
			}
		})
		t.Run("FixRefuses", func(t *testing.T) {
			t.Parallel()
			f := newHoldCheckFixture()
			f.records["alpha"] = failedSubjectRecord("alpha")
			f.fixLost = &boardengine.HoldRefusal{Reason: "the hold does not stand as written", OwnStands: true}

			result, err := runHoldCheck(context.Background(), f.deps, "alpha")

			if err != nil || result.HoldLost {
				t.Errorf("runHoldCheck() = %+v, %v; want the hold not lost", result, err)
			}
			if !f.reached("beamup:alpha") || !f.reached("beamup:beta") {
				t.Errorf("calls = %q; want each subject brought up", f.calls)
			}
		})
	})

	t.Run("APullBeforeTheGreenEndThatFailsCallsNoLaterSeam", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.pullErrs = []error{nil, errors.New("origin down")}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Green || !mentions(result.Lines, "origin down", rerun) {
			t.Errorf("runHoldCheck() = %+v, %v; want not green with a line naming the cause and %q", result, err, rerun)
		}
		if want := []string{"pull", "hold", "check:alpha", "check:beta", "pull"}; !slices.Equal(f.calls, want) {
			t.Errorf("calls = %q; want %q", f.calls, want)
		}
	})

	t.Run("AFailedClearHoldNamesTheRerunAndIsNotGreen", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.clearErr = errors.New("push refused")

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Green || result.HoldLost || !mentions(result.Lines, "push refused", rerun) {
			t.Errorf("runHoldCheck() = %+v, %v; want not green, the hold not lost, and a line naming the cause and %q", result, err, rerun)
		}
		if f.reached("beamup:") || f.reached("drop") {
			t.Errorf("calls = %q; want no beam up and no bolt-hold drop", f.calls)
		}
	})

	t.Run("AFailedBeamUpNamesBeamStart", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.beamUpErrs["beta"] = errors.New("tmux missing")

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || !result.Green || !mentions(result.Lines, "tmux missing", "lym beam start beta") {
			t.Errorf("runHoldCheck() = %+v, %v; want green still, with a line naming the cause and lym beam start beta", result, err)
		}
		if !f.reached("drop") {
			t.Errorf("calls = %q; want the bolt-hold drop after the failed beam up", f.calls)
		}
	})

	t.Run("AFailedSubjectFilesOneTaskForTheFirstFailingSubjectInTheHoldsOrderAndBringsEachSubjectUp", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.records["alpha"] = failedSubjectRecord("alpha")
		f.records["beta"] = failedSubjectRecord("beta")
		f.fixTask = "adopt-fix-alpha-t1"
		f.fixLines = []string{"filed the fix task adopt-fix-alpha-t1"}

		result, err := runHoldCheck(context.Background(), f.deps, "beta")

		if err != nil || result.Green || result.State != checkFailed || result.FixTask != "adopt-fix-alpha-t1" || !result.Ran {
			t.Errorf("runHoldCheck() = %+v, %v; want failed with the filed task", result, err)
		}
		want := []string{"pull", "hold", "check:beta", "check:alpha", "fix:alpha", "beamup:alpha", "beamup:beta"}
		if !slices.Equal(f.calls, want) {
			t.Errorf("calls = %q; want %q, the one fix for alpha and no clear", f.calls, want)
		}
		if !slices.Equal(f.notices, f.fixLines) {
			t.Errorf("notices = %q; want the fix's lines posted", f.notices)
		}
	})

	t.Run("TheTryCountIsReadFromTheHold", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.holds = []holdRead{{hold: holdOf(map[string]int{"alpha": 2, "beta": 1}, "alpha", "beta"), own: true, found: true}}
		f.records["beta"] = failedSubjectRecord("beta")

		if _, err := runHoldCheck(context.Background(), f.deps, "alpha"); err != nil {
			t.Fatalf("runHoldCheck() error = %v", err)
		}

		if !slices.Equal(f.fixTries, []int{1}) {
			t.Errorf("fix tries = %v; want beta's 1 from the hold", f.fixTries)
		}
	})

	t.Run("AtTheLimitTheHoldStaysAndTheDobbyNoticeNamesTheFailingTestsTheTaskAndTheRerun", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.records["alpha"] = failedSubjectRecord("alpha")
		limit := fixLimitLineOpening + "alpha ran out (3 of 3): tier0 failed in example.com/a TestA; settle the failure by hand, then run " + rerun
		f.fixLines = []string{limit}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Green || !slices.Equal(f.limitNotices, []string{limit}) || len(f.notices) != 0 {
			t.Errorf("runHoldCheck() = %+v, %v, dobby notices = %q, orch notices = %q; want the limit line posted as the dobby's notice alone", result, err, f.limitNotices, f.notices)
		}
		if f.reached("clear") || f.reached("green:") {
			t.Errorf("calls = %q; want the hold left standing", f.calls)
		}
	})

	t.Run("AnOpenOwnFixTaskFilesNoSecondTask", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.records["alpha"] = failedSubjectRecord("alpha")
		f.fixTask = "adopt-fix-alpha-t1"
		f.fixLines = []string{"the fix task adopt-fix-alpha-t1 of alpha stands running, so no new one was filed"}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.FixTask != "adopt-fix-alpha-t1" || result.HoldLost {
			t.Errorf("runHoldCheck() = %+v, %v; want the standing task reported and the hold kept", result, err)
		}
		if got := len(f.fixTries); got != 1 {
			t.Errorf("fix calls = %d; want one", got)
		}
	})

	t.Run("AFailureBeforeAnyTierFilesNoTask", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.records["alpha"] = CheckRecord{Subject: "alpha", State: checkFailed, Error: "the gate exited 1 before any tier ended"}

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Green || result.State != checkFailed || !mentions(result.Lines, "before any tier ended", rerun) {
			t.Errorf("runHoldCheck() = %+v, %v; want failed with a line naming the re-run", result, err)
		}
		if f.reached("fix:") || f.reached("beamup:") {
			t.Errorf("calls = %q; want no fix and no beam up", f.calls)
		}
	})

	t.Run("ACheckThatDidNotRunEndsNothingAndSaysToWait", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.notRan["beta"] = true

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || result.Ran || result.Green || result.State != holdStateAlreadyRunning || !mentions(result.Lines, "beta is already running", "wait for it", rerun) {
			t.Errorf("runHoldCheck() = %+v, %v; want not run, with a line telling to wait and naming %q", result, err, rerun)
		}
		if want := []string{"pull", "hold", "check:alpha", "check:beta"}; !slices.Equal(f.calls, want) {
			t.Errorf("calls = %q; want %q", f.calls, want)
		}
	})

	t.Run("ANilBeamUpBringsNothingUp", func(t *testing.T) {
		t.Parallel()
		f := newHoldCheckFixture()
		f.deps.BeamUp = nil

		result, err := runHoldCheck(context.Background(), f.deps, "alpha")

		if err != nil || !result.Green || f.reached("beamup:") {
			t.Errorf("runHoldCheck() = %+v, %v, calls = %q; want green with nothing brought up", result, err, f.calls)
		}
	})
}
