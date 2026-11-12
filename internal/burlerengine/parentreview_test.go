// parentreview_test.go tables the parent-review ask record.
// It covers numbering and the forward-only state moves.
// It also covers the parent's answer and its report, an answer racing the deadline,
// and a record write that fails at the deadline.

package burlerengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// parentAskBase is the clock every parent-review ask test judges deadlines against.
var parentAskBase = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

// recordParentAsks records asks in order into a fresh run's files and returns those files.
func recordParentAsks(t *testing.T, asks ...burlerengine.ParentAsk) burlerengine.ParentReviewFiles {
	t.Helper()
	files := burlerengine.ParentReviewFilesOf(t.TempDir())
	for _, ask := range asks {
		if _, err := burlerengine.RecordParentAsk(files, ask); err != nil {
			t.Fatalf("RecordParentAsk() error = %v", err)
		}
	}
	return files
}

// parentAskStates returns the state of every ask the run's record holds, oldest first.
func parentAskStates(t *testing.T, files burlerengine.ParentReviewFiles) []burlerengine.ParentAskState {
	t.Helper()
	record, _, err := burlerengine.ReadParentAsks(files)
	if err != nil {
		t.Fatalf("ReadParentAsks() error = %v", err)
	}
	states := make([]burlerengine.ParentAskState, len(record.Asks))
	for i, ask := range record.Asks {
		states[i] = ask.State
	}
	return states
}

// TestParentAsks_RecordAndMoveForward verifies asks are numbered in order and move only forward.
// MarkParentAskPosted opens only a pending-post ask;
// ExpireParentAsk sets an active latest ask timeout only at or past its deadline and never moves an ended one.
func TestParentAsks_RecordAndMoveForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	open := burlerengine.ParentAsk{State: burlerengine.ParentAskPendingPost, Deadline: parentAskBase.Add(time.Minute)}
	tests := []struct {
		name      string
		asks      []burlerengine.ParentAsk
		move      func(burlerengine.ParentReviewFiles) error
		want      []burlerengine.ParentAskState
		wantFound bool
	}{
		{
			name: "posting opens a pending-post ask",
			asks: []burlerengine.ParentAsk{open},
			move: func(f burlerengine.ParentReviewFiles) error { return burlerengine.MarkParentAskPosted(f, 1) },
			want: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name: "posting leaves an ended ask ended",
			asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}},
			move: func(f burlerengine.ParentReviewFiles) error { return burlerengine.MarkParentAskPosted(f, 1) },
			want: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout},
		},
		{
			name: "expiry before the deadline leaves the ask active",
			asks: []burlerengine.ParentAsk{open},
			move: expireAt(parentAskBase),
			want: []burlerengine.ParentAskState{burlerengine.ParentAskPendingPost},
		},
		{
			name: "expiry at the deadline sets the latest ask timeout",
			asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskDone}, open},
			move: expireAt(parentAskBase.Add(time.Minute)),
			want: []burlerengine.ParentAskState{burlerengine.ParentAskDone, burlerengine.ParentAskTimeout},
		},
		{
			name: "expiry past the deadline leaves a done ask done",
			asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskDone, Deadline: parentAskBase}},
			move: expireAt(parentAskBase.Add(time.Hour)),
			want: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
		},
		{
			name: "expiry past the deadline leaves an ask in an unknown state unmoved",
			asks: []burlerengine.ParentAsk{{State: "future-state", Deadline: parentAskBase}},
			move: expireAt(parentAskBase.Add(time.Hour)),
			want: []burlerengine.ParentAskState{"future-state"},
		},
		{
			name: "expiry of a run with no ask writes nothing",
			move: expireAt(parentAskBase),
			want: []burlerengine.ParentAskState{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := recordParentAsks(t, tt.asks...)
			if err := tt.move(files); err != nil {
				t.Fatalf("move error = %v", err)
			}
			if got := parentAskStates(t, files); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ask states = %v; want %v", got, tt.want)
			}
			record, found, _ := burlerengine.ReadParentAsks(files)
			if found != (len(tt.asks) > 0) {
				t.Errorf("ReadParentAsks() found = %v; want %v", found, len(tt.asks) > 0)
			}
			for i, ask := range record.Asks {
				if ask.Number != i+1 {
					t.Errorf("ask %d Number = %d; want %d", i, ask.Number, i+1)
				}
			}
		})
	}
}

// expireAt returns a move that runs ExpireParentAsk at now.
func expireAt(now time.Time) func(burlerengine.ParentReviewFiles) error {
	return func(files burlerengine.ParentReviewFiles) error {
		_, _, err := burlerengine.ExpireParentAsk(files, now)
		return err
	}
}

// TestAnswerParentAsk verifies an answer writes the report and sets the ask done,
// and that each refusal names its kind, writes no report and leaves the ask as the refusal says.
func TestAnswerParentAsk(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	active := burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: parentAskBase.Add(time.Minute)}
	findings := []burlerengine.ParentFinding{
		{Severity: burlerengine.SeverityMedium, Class: burlerengine.ClassDesign, Change: "name the lock in D5"},
		{Change: "drop the duplicated bound"},
	}
	tests := []struct {
		name        string
		asks        []burlerengine.ParentAsk
		number      int
		now         time.Time
		findings    []burlerengine.ParentFinding
		wantRefusal burlerengine.ParentAnswerRefusalKind
		wantStates  []burlerengine.ParentAskState
		wantReport  burlerengine.ParentReport
	}{
		{
			name:       "no findings make a satisfied report",
			asks:       []burlerengine.ParentAsk{active},
			number:     1,
			now:        parentAskBase,
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
			wantReport: burlerengine.ParentReport{Ask: 1, Satisfied: true, Findings: []burlerengine.ParentFinding{}},
		},
		{
			name:       "findings are numbered in order and keep their severity and class",
			asks:       []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}, {State: burlerengine.ParentAskPendingPost, Deadline: parentAskBase.Add(time.Minute)}},
			number:     2,
			now:        parentAskBase,
			findings:   findings,
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskDone},
			wantReport: burlerengine.ParentReport{Ask: 2, Findings: []burlerengine.ParentFinding{
				{ID: "P1", Severity: burlerengine.SeverityMedium, Class: burlerengine.ClassDesign, Change: "name the lock in D5"},
				{ID: "P2", Change: "drop the duplicated bound"},
			}},
		},
		{name: "a run with no ask is refused", number: 1, now: parentAskBase, wantRefusal: burlerengine.ParentAnswerNoAsk, wantStates: []burlerengine.ParentAskState{}},
		{
			name:        "an ask other than the latest is refused",
			asks:        []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}, active},
			number:      1,
			now:         parentAskBase,
			wantRefusal: burlerengine.ParentAnswerNotLatest,
			wantStates:  []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskOpen},
		},
		{
			name:        "an ended ask is refused",
			asks:        []burlerengine.ParentAsk{{State: burlerengine.ParentAskNotSent}},
			number:      1,
			now:         parentAskBase,
			wantRefusal: burlerengine.ParentAnswerEnded,
			wantStates:  []burlerengine.ParentAskState{burlerengine.ParentAskNotSent},
		},
		{
			name:        "an ask in an unknown state past its deadline is refused as ended and left unmoved",
			asks:        []burlerengine.ParentAsk{{State: "future-state", Deadline: parentAskBase}},
			number:      1,
			now:         parentAskBase.Add(time.Minute),
			wantRefusal: burlerengine.ParentAnswerEnded,
			wantStates:  []burlerengine.ParentAskState{"future-state"},
		},
		{
			name:        "an ask at its deadline is set timeout and the answer refused",
			asks:        []burlerengine.ParentAsk{active},
			number:      1,
			now:         parentAskBase.Add(time.Minute),
			wantRefusal: burlerengine.ParentAnswerPastDeadline,
			wantStates:  []burlerengine.ParentAskState{burlerengine.ParentAskTimeout},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := recordParentAsks(t, tt.asks...)
			ask, err := burlerengine.AnswerParentAsk(files, tt.number, tt.now, tt.findings)
			if got := parentAskStates(t, files); !reflect.DeepEqual(got, tt.wantStates) {
				t.Errorf("ask states = %v; want %v", got, tt.wantStates)
			}
			if tt.wantRefusal != "" {
				var refusal *burlerengine.ParentAnswerRefusal
				if !errors.As(err, &refusal) || refusal.Kind != tt.wantRefusal {
					t.Fatalf("AnswerParentAsk() error = %v; want a %s refusal", err, tt.wantRefusal)
				}
				if _, statErr := os.Stat(files.ReportPath(tt.number)); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("report at %s stat error = %v; want no report", files.ReportPath(tt.number), statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AnswerParentAsk() error = %v", err)
			}
			if ask.State != burlerengine.ParentAskDone || ask.Report != files.ReportPath(tt.number) || !ask.AnsweredAt.Equal(tt.now) {
				t.Errorf("AnswerParentAsk() = %+v; want done with report %s answered at %s", ask, files.ReportPath(tt.number), tt.now)
			}
			data, err := os.ReadFile(ask.Report)
			if err != nil {
				t.Fatalf("read the report: %v", err)
			}
			var report burlerengine.ParentReport
			if err := yaml.Unmarshal(data, &report); err != nil {
				t.Fatalf("parse the report: %v", err)
			}
			if len(report.Findings) == 0 {
				report.Findings = []burlerengine.ParentFinding{}
			}
			if !reflect.DeepEqual(report, tt.wantReport) {
				t.Errorf("report = %+v; want %+v", report, tt.wantReport)
			}
		})
	}
}

// TestAnswerParentAsk_FailedWriteAtDeadline verifies a failed record write returns a plain error, not a refusal.
// The ask at its deadline stays open on disk.
func TestAnswerParentAsk_FailedWriteAtDeadline(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory does not block writes on Windows or for root")
	}
	files := recordParentAsks(t, burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: parentAskBase})
	if err := os.Chmod(files.Dir, 0o500); err != nil {
		t.Fatalf("make the record directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(files.Dir, 0o700) })

	_, err := burlerengine.AnswerParentAsk(files, 1, parentAskBase.Add(time.Minute), nil)
	if err == nil {
		t.Fatal("AnswerParentAsk() error = nil; want the failed record write")
	}
	var refusal *burlerengine.ParentAnswerRefusal
	if errors.As(err, &refusal) {
		t.Errorf("AnswerParentAsk() error = %v; want a write error, not a refusal", err)
	}
	if got, want := parentAskStates(t, files), []burlerengine.ParentAskState{burlerengine.ParentAskOpen}; !reflect.DeepEqual(got, want) {
		t.Errorf("ask states = %v; want %v", got, want)
	}
}

// TestAnswerParentAsk_RacingTheDeadline verifies an answer and an expiry run at once leave exactly one end state.
// When the expiry wins the answer is refused; when the answer wins the expiry reads it done.
//
//testtiming:keep pins that a concurrent answer and expiry leave exactly one end state under the one lock
func TestAnswerParentAsk_RacingTheDeadline(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for range 20 {
		files := recordParentAsks(t, burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: parentAskBase})
		var answerErr, expireErr error
		var expired burlerengine.ParentAsk
		var wg sync.WaitGroup
		wg.Go(func() { _, answerErr = burlerengine.AnswerParentAsk(files, 1, parentAskBase.Add(-time.Second), nil) })
		wg.Go(func() { expired, _, expireErr = burlerengine.ExpireParentAsk(files, parentAskBase) })
		wg.Wait()
		if expireErr != nil {
			t.Fatalf("ExpireParentAsk() error = %v", expireErr)
		}
		switch states := parentAskStates(t, files); {
		case reflect.DeepEqual(states, []burlerengine.ParentAskState{burlerengine.ParentAskDone}):
			if answerErr != nil || expired.State != burlerengine.ParentAskDone {
				t.Errorf("done ask: answer error = %v, expiry read %s; want no error and done", answerErr, expired.State)
			}
		case reflect.DeepEqual(states, []burlerengine.ParentAskState{burlerengine.ParentAskTimeout}):
			var refusal *burlerengine.ParentAnswerRefusal
			if !errors.As(answerErr, &refusal) || refusal.Kind != burlerengine.ParentAnswerEnded {
				t.Errorf("timed-out ask: answer error = %v; want an ended refusal", answerErr)
			}
		default:
			t.Fatalf("ask states = %v; want exactly one of done or timeout", states)
		}
	}
}

// liveParentAsk is an open ask whose deadline lies a minute past parentAskBase.
var liveParentAsk = burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: parentAskBase.Add(time.Minute)}

// twoParentFindings is a parent report's worth of findings, which an answer numbers P1 and P2.
var twoParentFindings = []burlerengine.ParentFinding{
	{Severity: burlerengine.SeverityMedium, Class: burlerengine.ClassDesign, Change: "name the lock in D5"},
	{Change: "drop the duplicated bound"},
}

// answeredParentAsk records one open ask, answers it with findings before its deadline,
// and returns the run's files and the answered ask.
func answeredParentAsk(t *testing.T, findings []burlerengine.ParentFinding) (burlerengine.ParentReviewFiles, burlerengine.ParentAsk) {
	t.Helper()
	files := recordParentAsks(t, liveParentAsk)
	ask, err := burlerengine.AnswerParentAsk(files, 1, parentAskBase, findings)
	if err != nil {
		t.Fatalf("AnswerParentAsk() error = %v", err)
	}
	return files, ask
}

// TestParentAskRecord_RoundAsk verifies RoundAsk finds the latest ask of one seed and round.
// It reports false for any other seed or round.
func TestParentAskRecord_RoundAsk(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	record := burlerengine.ParentAskRecord{Asks: []burlerengine.ParentAsk{
		{Number: 1, Seed: "seed-a", Round: 1},
		{Number: 2, Seed: "seed-b", Round: 1},
		{Number: 3, Seed: "seed-a", Round: 2},
		{Number: 4, Seed: "seed-a", Round: 1},
	}}
	tests := []struct {
		name       string
		seed       string
		round      int
		wantNumber int
	}{
		{name: "the latest ask of the seed and round", seed: "seed-a", round: 1, wantNumber: 4},
		{name: "the same round of another seed", seed: "seed-b", round: 1, wantNumber: 2},
		{name: "another round of the seed", seed: "seed-a", round: 2, wantNumber: 3},
		{name: "a seed the run never asked under", seed: "seed-c", round: 1},
		{name: "a round the seed never asked for", seed: "seed-a", round: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ask, found := record.RoundAsk(tt.seed, tt.round)
			if found != (tt.wantNumber != 0) || ask.Number != tt.wantNumber {
				t.Errorf("RoundAsk(%q, %d) = ask %d, %v; want ask %d, found %v", tt.seed, tt.round, ask.Number, found, tt.wantNumber, tt.wantNumber != 0)
			}
		})
	}
}

// TestEndParentAsk verifies EndParentAsk moves a pending-post or open ask to the end state with its reason.
// It leaves an ended ask as it stands and still reports it.
// It reports an unknown number found false with nothing written.
func TestEndParentAsk(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		asks       []burlerengine.ParentAsk
		number     int
		wantFound  bool
		wantState  burlerengine.ParentAskState
		wantReason string
	}{
		{
			name:       "a pending-post ask moves to the end state with the reason",
			asks:       []burlerengine.ParentAsk{{State: burlerengine.ParentAskPendingPost}},
			number:     1,
			wantFound:  true,
			wantState:  burlerengine.ParentAskNotSent,
			wantReason: "no room",
		},
		{
			name:       "an open ask moves to the end state with the reason",
			asks:       []burlerengine.ParentAsk{liveParentAsk},
			number:     1,
			wantFound:  true,
			wantState:  burlerengine.ParentAskNotSent,
			wantReason: "no room",
		},
		{
			name:      "an ended ask is left as it stands and reported",
			asks:      []burlerengine.ParentAsk{{State: burlerengine.ParentAskDone}},
			number:    1,
			wantFound: true,
			wantState: burlerengine.ParentAskDone,
		},
		{
			name:      "an unknown number is not found and nothing is written",
			asks:      nil,
			number:    2,
			wantState: "",
		},
		{
			name:      "an unknown number leaves the asks it does not name",
			asks:      []burlerengine.ParentAsk{liveParentAsk},
			number:    2,
			wantState: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := recordParentAsks(t, tt.asks...)
			wantStates := parentAskStates(t, files)
			if tt.wantFound && !wantStates[tt.number-1].Ended() {
				wantStates[tt.number-1] = tt.wantState
			}

			ask, found, err := burlerengine.EndParentAsk(files, tt.number, burlerengine.ParentAskNotSent, "no room")

			if err != nil {
				t.Fatalf("EndParentAsk() error = %v", err)
			}
			if found != tt.wantFound || ask.State != tt.wantState || ask.Reason != tt.wantReason {
				t.Errorf("EndParentAsk() = state %q reason %q found %v; want %q, %q, %v", ask.State, ask.Reason, found, tt.wantState, tt.wantReason, tt.wantFound)
			}
			if got := parentAskStates(t, files); !reflect.DeepEqual(got, wantStates) {
				t.Errorf("ask states = %v; want %v", got, wantStates)
			}
			if _, recorded, _ := burlerengine.ReadParentAsks(files); recorded != (len(tt.asks) > 0) {
				t.Errorf("ReadParentAsks() found = %v; want %v, so nothing is written for an unknown number", recorded, len(tt.asks) > 0)
			}
		})
	}
}

// TestSettleParentAsk verifies settling an accepted round's ask.
// An open ask becomes timeout and copies nothing.
// A done ask's report is copied byte for byte beside the review once, and a second call copies nothing.
// A timeout or no-parent ask copies nothing.
func TestSettleParentAsk(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		ask       burlerengine.ParentAsk
		answer    bool
		wantCopy  bool
		wantState burlerengine.ParentAskState
	}{
		{name: "an open ask becomes timeout and copies nothing", ask: liveParentAsk, wantState: burlerengine.ParentAskTimeout},
		{name: "a done ask is copied once", ask: liveParentAsk, answer: true, wantCopy: true, wantState: burlerengine.ParentAskDone},
		{name: "a timeout ask copies nothing", ask: burlerengine.ParentAsk{State: burlerengine.ParentAskTimeout}, wantState: burlerengine.ParentAskTimeout},
		{name: "a no-parent ask copies nothing", ask: burlerengine.ParentAsk{State: burlerengine.ParentAskNoParent}, wantState: burlerengine.ParentAskNoParent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := recordParentAsks(t, tt.ask)
			var report string
			if tt.answer {
				answered, err := burlerengine.AnswerParentAsk(files, 1, parentAskBase, twoParentFindings)
				if err != nil {
					t.Fatalf("AnswerParentAsk() error = %v", err)
				}
				report = answered.Report
			}
			reviewPath := filepath.Join(t.TempDir(), "round-1-review.md")
			copyPath := burlerengine.ParentCopyPath(reviewPath)

			first, err := burlerengine.SettleParentAsk(files, 1, reviewPath)
			if err != nil {
				t.Fatalf("SettleParentAsk() error = %v", err)
			}
			second, err := burlerengine.SettleParentAsk(files, 1, reviewPath)
			if err != nil {
				t.Fatalf("SettleParentAsk() second call error = %v", err)
			}

			if second != "" {
				t.Errorf("second SettleParentAsk() = %q; want nothing copied", second)
			}
			if got := parentAskStates(t, files); !reflect.DeepEqual(got, []burlerengine.ParentAskState{tt.wantState}) {
				t.Errorf("ask states = %v; want [%s]", got, tt.wantState)
			}
			if !tt.wantCopy {
				if _, statErr := os.Stat(copyPath); first != "" || !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("SettleParentAsk() = %q, copy stat error = %v; want nothing copied", first, statErr)
				}
				return
			}
			if first != copyPath {
				t.Errorf("SettleParentAsk() = %q; want %q", first, copyPath)
			}
			want, err := os.ReadFile(report)
			if err != nil {
				t.Fatalf("read the report: %v", err)
			}
			if got, err := os.ReadFile(copyPath); err != nil || string(got) != string(want) {
				t.Errorf("copy = %q, %v; want the report's bytes %q", got, err, want)
			}
		})
	}
}

// TestReadParentReport verifies ReadParentReport returns what AnswerParentAsk wrote, with ids P1 up.
// It errors when the file is missing.
func TestReadParentReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	files, ask := answeredParentAsk(t, twoParentFindings)

	got, err := burlerengine.ReadParentReport(ask.Report)
	if err != nil {
		t.Fatalf("ReadParentReport() error = %v", err)
	}
	want := burlerengine.ParentReport{Ask: 1, Findings: []burlerengine.ParentFinding{
		{ID: "P1", Severity: burlerengine.SeverityMedium, Class: burlerengine.ClassDesign, Change: "name the lock in D5"},
		{ID: "P2", Change: "drop the duplicated bound"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadParentReport() = %+v; want %+v", got, want)
	}
	if _, err := burlerengine.ReadParentReport(files.ReportPath(2)); err == nil {
		t.Error("ReadParentReport(missing file) error = nil; want an error")
	}
}

// TestParentCopyPath verifies the copy of a parent report is named by the review's stem beside it.
//
//testtiming:keep pins the copy's file name suffix .parent.yaml, which the settle test derives from the function under test
func TestParentCopyPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := filepath.Join("some", "dir")
	if got, want := burlerengine.ParentCopyPath(filepath.Join(dir, "round-1-review.md")), filepath.Join(dir, "round-1-review.parent.yaml"); got != want {
		t.Errorf("ParentCopyPath() = %q; want %q", got, want)
	}
}

// parentReviewFinding returns the review-file frontmatter entry of one finding tagged origin with parentIDs.
func parentReviewFinding(id, origin, parentIDs string) string {
	return "  - id: " + id + "\n    severity: LOW\n    class: design\n    location: a.go:1\n    summary: s\n    fix: f\n" +
		"    origin: " + origin + "\n    parent_ids: " + parentIDs + "\n"
}

// parentReview returns an approved review file holding the finding entries.
func parentReview(entries ...string) string {
	return "---\nverdict: APPROVED\nfindings:\n" + strings.Join(entries, "") + "---\n"
}

// TestParentCoverageGateEntry_Gate verifies the gate closes an unanswered ask and holds the review to the report.
// An open ask is set timeout and passes; a satisfied report passes.
// A review fails naming each report id that no finding tagged origin parent lists in its parent_ids,
// and an unparseable or missing review fails with findings, not an error.
// A done ask whose report file is missing is a gate error.
func TestParentCoverageGateEntry_Gate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name         string
		findings     []burlerengine.ParentFinding
		answer       bool
		review       string
		dropReport   bool
		wantErr      bool
		wantPassed   bool
		wantState    burlerengine.ParentAskState
		wantFindings string
	}{
		{name: "an open ask is set timeout and passes", wantPassed: true, wantState: burlerengine.ParentAskTimeout},
		{name: "a satisfied report passes", answer: true, wantPassed: true, wantState: burlerengine.ParentAskDone},
		{
			name:         "a review covering one report id fails naming the other",
			findings:     twoParentFindings,
			answer:       true,
			review:       parentReview(parentReviewFinding("F1", "parent", "[P1]")),
			wantState:    burlerengine.ParentAskDone,
			wantFindings: "findings P2 that no finding",
		},
		{
			name:       "one merged finding listing both report ids covers them",
			findings:   twoParentFindings,
			answer:     true,
			review:     parentReview(parentReviewFinding("F1", "parent", "[P1, P2]")),
			wantPassed: true,
			wantState:  burlerengine.ParentAskDone,
		},
		{
			name:         "a finding with another origin does not cover its listed id",
			findings:     twoParentFindings,
			answer:       true,
			review:       parentReview(parentReviewFinding("F1", "parent", "[P1]"), parentReviewFinding("F2", "lens:style", "[P2]")),
			wantState:    burlerengine.ParentAskDone,
			wantFindings: "findings P2 that no finding",
		},
		{
			name:         "an unparseable review fails",
			findings:     twoParentFindings,
			answer:       true,
			review:       "not frontmatter at all\n",
			wantState:    burlerengine.ParentAskDone,
			wantFindings: "Rewrite the review file",
		},
		{
			name:         "a missing review fails",
			findings:     twoParentFindings,
			answer:       true,
			wantState:    burlerengine.ParentAskDone,
			wantFindings: "read review file",
		},
		{
			name:       "a done ask whose report is missing is a gate error",
			findings:   twoParentFindings,
			answer:     true,
			dropReport: true,
			wantErr:    true,
			wantState:  burlerengine.ParentAskDone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := recordParentAsks(t, liveParentAsk)
			if tt.answer {
				if _, err := burlerengine.AnswerParentAsk(files, 1, parentAskBase, tt.findings); err != nil {
					t.Fatalf("AnswerParentAsk() error = %v", err)
				}
			}
			if tt.dropReport {
				if err := os.Remove(files.ReportPath(1)); err != nil {
					t.Fatalf("remove the report: %v", err)
				}
			}
			reviewPath := filepath.Join(t.TempDir(), "review.md")
			if tt.review != "" {
				if err := os.WriteFile(reviewPath, []byte(tt.review), 0o644); err != nil {
					t.Fatalf("write the review: %v", err)
				}
			}

			result, err := burlerengine.ParentCoverageGateEntry(files, 1, reviewPath).Gate()

			if tt.wantErr {
				if err == nil {
					t.Error("Gate() error = nil; want an error for the missing report")
				}
				if got := parentAskStates(t, files); !reflect.DeepEqual(got, []burlerengine.ParentAskState{tt.wantState}) {
					t.Errorf("ask states = %v; want [%s]", got, tt.wantState)
				}
				return
			}
			if err != nil {
				t.Fatalf("Gate() error = %v; want a failed result, not an error", err)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("Gate() passed = %v; want %v (findings %q)", result.Passed, tt.wantPassed, result.Findings)
			}
			if tt.wantFindings != "" && !strings.Contains(result.Findings, tt.wantFindings) {
				t.Errorf("Gate() findings = %q; want them to contain %q", result.Findings, tt.wantFindings)
			}
			if got := parentAskStates(t, files); !reflect.DeepEqual(got, []burlerengine.ParentAskState{tt.wantState}) {
				t.Errorf("ask states = %v; want [%s]", got, tt.wantState)
			}
		})
	}
}
