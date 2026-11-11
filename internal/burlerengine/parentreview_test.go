// parentreview_test.go tables the parent-review ask record.
// It covers numbering and the forward-only state moves.
// It also covers the parent's answer and its report, an answer racing the deadline,
// and a record write that fails at the deadline.

package burlerengine_test

import (
	"errors"
	"os"
	"reflect"
	"runtime"
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
