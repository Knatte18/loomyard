// parentreview_test.go tables the parent-review answer and await verbs over a temp ask record and a fake clock.

package loomcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// parentReviewNow is the clock the parent-review verb tests judge deadlines against.
var parentReviewNow = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

// parentReviewParent is the run's parent orch as the fakes resolve it.
const parentReviewParent = "ly:orch"

const (
	// parentReviewSession is the strand name of the PR-Review session the fixture's pull request record names.
	parentReviewSession = "ly:task-a:pr-reviewer"
	// parentReviewHead is the task head the fixture reads.
	parentReviewHead = "head-1"
)

// parentReviewPRAsker returns the askPR seam that records through a real PRReviewAsker over files.
// It asks at now() and its post succeeds.
func parentReviewPRAsker(files burlerengine.ParentReviewFiles, now func() time.Time) func(pullrequest.Record, string, string) (burlerengine.ParentAsk, error) {
	return func(rec pullrequest.Record, head, _ string) (burlerengine.ParentAsk, error) {
		asker := landingshed.PRReviewAsker{
			Files:        files,
			Slug:         "task-a",
			URL:          rec.URL,
			TaskBranch:   rec.Head,
			ParentBranch: rec.Base,
			Wait:         time.Hour,
			Resolve:      func() (string, bool, error) { return parentReviewParent, true, nil },
			Post:         func(string, string) error { return nil },
		}
		return asker.Ask(head, now())
	}
}

// parentReviewFixture records asks into a fresh run's files and returns deps over them.
// The parent resolves to parentReviewParent and the caller is the operator's unnamed shell until the test sets them.
// The run's pull request record is a temp file recorded under autonomy for the session parentReviewSession,
// and the task head reads parentReviewHead.
func parentReviewFixture(t *testing.T, asks ...burlerengine.ParentAsk) parentReviewDeps {
	t.Helper()
	files := burlerengine.ParentReviewFilesOf(t.TempDir())
	for _, ask := range asks {
		if _, err := burlerengine.RecordParentAsk(files, ask); err != nil {
			t.Fatalf("RecordParentAsk() error = %v", err)
		}
	}
	dir := t.TempDir()
	recordPath, lockPath := filepath.Join(dir, "pull-request.json"), filepath.Join(dir, "pull-request.lock")
	autonomous := true
	record := pullrequest.Record{Number: 7, URL: "https://example.test/pull/7", Base: "main", Head: "task-a", Autonomy: &autonomous, StrandName: parentReviewSession}
	if err := pullrequest.WriteRecord(recordPath, lockPath, record); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	deps := parentReviewDeps{
		parent:   func() (hubgeom.Parent, error) { return hubgeom.Parent{Name: parentReviewParent}, nil },
		files:    files,
		now:      func() time.Time { return parentReviewNow },
		prRecord: func() (pullrequest.Record, bool, error) { return pullrequest.ReadRecord(recordPath, lockPath) },
		head:     func() (string, error) { return parentReviewHead, nil },
	}
	deps.askPR = parentReviewPRAsker(files, deps.now)
	return deps
}

// openParentAsk is an open ask whose deadline is d past parentReviewNow.
func openParentAsk(d time.Duration) burlerengine.ParentAsk {
	return burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Strand: "ly:orch-asked", Deadline: parentReviewNow.Add(d)}
}

// TestParentReviewAnswerVerb verifies the answer verb records a well-formed answer from an admitted caller,
// and that each refusal names its way forward and leaves the asks as stated.
func TestParentReviewAnswerVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const decisionWay = `"lym loom decision add task-a --by parent", run by the run's parent orch ly:orch`
	tests := []struct {
		name string
		asks []burlerengine.ParentAsk
		// findings, when set, is the content of the --findings-file the call passes.
		findings string
		// emptyFindingsFile writes the --findings-file even when findings is empty.
		emptyFindingsFile bool
		input             parentAnswerInput
		setup             func(*parentReviewDeps)
		// want lists text the refusal must hold; empty wants an answer.
		want []string
		// wantNot lists text the refusal must not hold.
		wantNot    []string
		wantStates []burlerengine.ParentAskState
		// wantReport, when set, is the findings the written report must hold.
		wantReport []burlerengine.ParentFinding
	}{
		{
			name:       "satisfied answers the open ask",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
		},
		{
			name:       "a findings file answers with its findings",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			findings:   "findings:\n  - change: name the lock in D5\n    severity: MEDIUM\n    class: design\n  - change: drop the duplicated bound\n",
			input:      parentAnswerInput{ask: 1},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
			wantReport: []burlerengine.ParentFinding{
				{ID: "P1", Severity: burlerengine.SeverityMedium, Class: burlerengine.ClassDesign, Change: "name the lock in D5"},
				{ID: "P2", Change: "drop the duplicated bound"},
			},
		},
		{
			name:       "the run's parent orch answers an open ask",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			setup:      func(d *parentReviewDeps) { d.strandName = parentReviewParent },
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
		},
		{
			name:       "the strand the ask went to answers a pending-post ask after the parent resolves to another orch",
			asks:       []burlerengine.ParentAsk{{State: burlerengine.ParentAskPendingPost, Strand: "ly:orch-asked", Deadline: parentReviewNow.Add(time.Minute)}},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			setup:      func(d *parentReviewDeps) { d.strandName = "ly:orch-asked" },
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
		},
		{name: "a missing --ask is refused", input: parentAnswerInput{satisfied: true}, want: []string{"--ask is required", parentReviewWayFlags}, wantStates: []burlerengine.ParentAskState{}},
		{name: "both answer forms are refused", findings: "findings: []\n", input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"exactly one of", parentReviewWayFlags}, wantStates: []burlerengine.ParentAskState{}},
		{name: "neither answer form is refused", input: parentAnswerInput{ask: 1}, want: []string{"exactly one of", parentReviewWayFlags}, wantStates: []burlerengine.ParentAskState{}},
		{name: "a findings file with no item is refused", findings: "findings: []\n", input: parentAnswerInput{ask: 1}, want: []string{"holds no finding", parentReviewWayFindingsFile}, wantStates: []burlerengine.ParentAskState{}},
		{name: "a finding with no change is refused", findings: "findings:\n  - severity: LOW\n", input: parentAnswerInput{ask: 1}, want: []string{"finding 1", "has no change", parentReviewWayFindingsFile}, wantStates: []burlerengine.ParentAskState{}},
		{name: "a severity outside the review format is refused", findings: "findings:\n  - change: a\n  - change: b\n    severity: HIGH\n", input: parentAnswerInput{ask: 1}, want: []string{"finding 2", `"HIGH"`, parentReviewWayFindingsFile}, wantStates: []burlerengine.ParentAskState{}},
		{name: "a class outside the review format is refused", findings: "findings:\n  - change: a\n    class: style\n", input: parentAnswerInput{ask: 1}, want: []string{"finding 1", `"style"`, parentReviewWayFindingsFile}, wantStates: []burlerengine.ParentAskState{}},
		{
			name:  "an empty findings file is refused",
			asks:  []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input: parentAnswerInput{ask: 1}, emptyFindingsFile: true,
			want:       []string{"holds no finding", parentReviewWayFindingsFile},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:       "a findings file that is not a YAML findings list is refused",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			findings:   "[",
			input:      parentAnswerInput{ask: 1},
			want:       []string{"is not YAML holding a findings list", parentReviewWayFindingsFile},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:       "a findings file path that does not exist is refused",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input:      parentAnswerInput{ask: 1, findingsFile: "/nonexistent/parent-review/findings.yaml"},
			want:       []string{"read the --findings-file", parentReviewWayFindingsFile},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:       "a finding whose change is only whitespace is refused",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			findings:   "findings:\n  - change: \"  \"\n",
			input:      parentAnswerInput{ask: 1},
			want:       []string{"finding 1", "has no change", parentReviewWayFindingsFile},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:  "a caller is refused when the run's parent orch cannot be resolved",
			asks:  []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input: parentAnswerInput{ask: 1, satisfied: true},
			setup: func(d *parentReviewDeps) {
				d.strandName = "ly:task-a:darn"
				d.parent = func() (hubgeom.Parent, error) { return hubgeom.Parent{}, errors.New("no orch is live") }
			},
			want:       []string{`"ly:task-a:darn"`, "could not be resolved", parentReviewWayParentAnswers},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:       "a caller that is neither the parent nor the ask's strand is refused",
			asks:       []burlerengine.ParentAsk{openParentAsk(time.Minute)},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			setup:      func(d *parentReviewDeps) { d.strandName = "ly:task-a:darn" },
			want:       []string{`"ly:task-a:darn"`, "neither the run's parent orch nor the strand the ask went to", parentReviewWayParentAnswers},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{name: "a run with no ask is refused", input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"no parent-review ask", decisionWay}, wantStates: []burlerengine.ParentAskState{}},
		{
			name:  "a run with no ask and no parent orch names the operator's shell",
			input: parentAnswerInput{ask: 1, satisfied: true},
			setup: func(d *parentReviewDeps) {
				d.parent = func() (hubgeom.Parent, error) { return hubgeom.Parent{}, errors.New("no orch is live") }
			},
			want:       []string{"no parent-review ask", `"lym loom decision add task-a --by parent", run by the operator's own unnamed shell`},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "an older ask while the latest awaits an answer is sent to review what the newer ask names",
			asks:       []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}, openParentAsk(time.Minute)},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"a newer ask, ask 2, awaits an answer", "review again what ask 2's own request names", "--ask 2"},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskOpen},
		},
		{
			name:       "an older ask while the latest has ended names the decision verb",
			asks:       []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}, {State: burlerengine.ParentAskNoParent}},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"which is ask 2, already no-parent", decisionWay},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskNoParent},
		},
		{name: "a done ask is refused", asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskDone}}, input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"already done", decisionWay}, wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone}},
		{name: "a timed-out ask is refused", asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}}, input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"already timeout", decisionWay}, wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout}},
		{name: "a no-parent ask is refused", asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskNoParent}}, input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"already no-parent", decisionWay}, wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskNoParent}},
		{name: "a not-sent ask is refused", asks: []burlerengine.ParentAsk{{State: burlerengine.ParentAskNotSent}}, input: parentAnswerInput{ask: 1, satisfied: true}, want: []string{"already not-sent", decisionWay}, wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskNotSent}},
		{
			name:       "an open ask at its deadline is set timeout and the answer refused",
			asks:       []burlerengine.ParentAsk{openParentAsk(0)},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"reached its deadline at 2026-11-11T12:00:00Z", "now timeout", decisionWay},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout},
		},
		{
			name: "the parent answers the newest PR-Review ask after a review ask and a judge ask",
			asks: []burlerengine.ParentAsk{
				{Tag: "[code-review request]", State: burlerengine.ParentAskTimeout},
				{Seed: "drift-judge-1", State: burlerengine.ParentAskTimeout},
				{Tag: landingshed.PRReviewAskLineTag, Digest: parentReviewHead, State: burlerengine.ParentAskOpen, Strand: "ly:orch-asked", Deadline: parentReviewNow.Add(time.Minute)},
			},
			input:      parentAnswerInput{ask: 3, satisfied: true},
			setup:      func(d *parentReviewDeps) { d.strandName = parentReviewParent },
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskTimeout, burlerengine.ParentAskDone},
		},
		{
			name: "the operator's unnamed shell answers the newest PR-Review ask",
			asks: []burlerengine.ParentAsk{
				{Tag: "[code-review request]", State: burlerengine.ParentAskTimeout},
				{Tag: landingshed.PRReviewAskLineTag, Digest: parentReviewHead, State: burlerengine.ParentAskOpen, Strand: "ly:orch-asked", Deadline: parentReviewNow.Add(time.Minute)},
			},
			input:      parentAnswerInput{ask: 2, satisfied: true},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskDone},
		},
		{
			name:       "an answer to a PR-Review ask past its deadline waits for the session's next request",
			asks:       []burlerengine.ParentAsk{{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskOpen, Deadline: parentReviewNow}},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"reached its deadline", landingshed.PRReviewAskLineTag},
			wantNot:    []string{"lym loom decision add"},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout},
		},
		{
			name: "an answer to a PR-Review ask a newer ended one replaced waits for the session's next request",
			asks: []burlerengine.ParentAsk{
				{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskOpen, Deadline: parentReviewNow.Add(time.Minute)},
				{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskTimeout},
			},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"which is ask 2, already timeout", landingshed.PRReviewAskLineTag},
			wantNot:    []string{"lym loom decision add"},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen, burlerengine.ParentAskTimeout},
		},
		{
			name:       "the parent answers a no-parent PR-Review ask at the head it named",
			asks:       []burlerengine.ParentAsk{{Tag: landingshed.PRReviewAskLineTag, Digest: parentReviewHead, State: burlerengine.ParentAskNoParent}},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			setup:      func(d *parentReviewDeps) { d.strandName = parentReviewParent },
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
		},
		{
			name:       "the parent answers an undelivered PR-Review ask with findings",
			asks:       []burlerengine.ParentAsk{{Tag: landingshed.PRReviewAskLineTag, Digest: parentReviewHead, State: burlerengine.ParentAskUndelivered, Strand: "ly:orch-asked", Deadline: parentReviewNow}},
			findings:   "findings:\n  - change: rename the helper\n",
			input:      parentAnswerInput{ask: 1},
			setup:      func(d *parentReviewDeps) { d.strandName = parentReviewParent },
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskDone},
			wantReport: []burlerengine.ParentFinding{{ID: "P1", Change: "rename the helper"}},
		},
		{
			name:       "a cleared no-parent PR-Review ask is refused",
			asks:       []burlerengine.ParentAsk{{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskNoParent, Cleared: true}},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"already no-parent", landingshed.PRReviewAskLineTag},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskNoParent},
		},
		{
			name: "an answer to a PR-Review ask a newer open one replaced names the newer ask",
			asks: []burlerengine.ParentAsk{
				{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskTimeout},
				{Tag: landingshed.PRReviewAskLineTag, State: burlerengine.ParentAskOpen, Deadline: parentReviewNow.Add(time.Minute)},
			},
			input:      parentAnswerInput{ask: 1, satisfied: true},
			want:       []string{"a newer ask, ask 2, awaits an answer"},
			wantNot:    []string{"lym loom decision add", landingshed.PRReviewAskLineTag},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskOpen},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := parentReviewFixture(t, tt.asks...)
			if tt.setup != nil {
				tt.setup(&deps)
			}
			input := tt.input
			if tt.findings != "" || tt.emptyFindingsFile {
				input.findingsFile = filepath.Join(t.TempDir(), "findings.yaml")
				if err := os.WriteFile(input.findingsFile, []byte(tt.findings), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			code := parentReviewAnswerVerb(&out, "task-a", deps, input)
			record, _, err := burlerengine.ReadParentAsks(deps.files)
			if err != nil {
				t.Fatalf("ReadParentAsks() error = %v", err)
			}
			states := []burlerengine.ParentAskState{}
			for _, ask := range record.Asks {
				states = append(states, ask.State)
			}
			if !reflect.DeepEqual(states, tt.wantStates) {
				t.Errorf("ask states = %v; want %v", states, tt.wantStates)
			}
			if len(tt.want) > 0 {
				env := envelope.RequireErr(t, out.String(), "loom: parent-review answer: ")
				for _, want := range tt.want {
					if !strings.Contains(env.Error, want) {
						t.Errorf("refusal = %q; want it to hold %q", env.Error, want)
					}
				}
				for _, unwanted := range tt.wantNot {
					if strings.Contains(env.Error, unwanted) {
						t.Errorf("refusal = %q; want it not to hold %q", env.Error, unwanted)
					}
				}
				if code == 0 {
					t.Errorf("parentReviewAnswerVerb() = 0; want a non-zero exit")
				}
				return
			}
			env := envelope.RequireOK(t, out.String())
			want := map[string]any{"slug": "task-a", "ask": float64(input.ask), "state": "done", "report": deps.files.ReportPath(input.ask), "findings": float64(len(tt.wantReport))}
			if !reflect.DeepEqual(env.Raw, want) {
				t.Errorf("envelope = %v; want %v", env.Raw, want)
			}
			if tt.wantReport != nil {
				data, err := os.ReadFile(deps.files.ReportPath(input.ask))
				if err != nil {
					t.Fatalf("read the report: %v", err)
				}
				var report burlerengine.ParentReport
				if err := yaml.Unmarshal(data, &report); err != nil {
					t.Fatalf("parse the report: %v", err)
				}
				if !reflect.DeepEqual(report.Findings, tt.wantReport) {
					t.Errorf("report findings = %+v; want %+v", report.Findings, tt.wantReport)
				}
			}
		})
	}
}

// TestParentReviewAskVerb verifies the ask verb records a PR-Review ask for the recorded session of an autonomous review,
// and that each refusal names its way forward and leaves the asks as stated.
func TestParentReviewAskVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	prAsk := func(state burlerengine.ParentAskState, d time.Duration) burlerengine.ParentAsk {
		return burlerengine.ParentAsk{Tag: landingshed.PRReviewAskLineTag, Digest: parentReviewHead, State: state, Strand: parentReviewParent, Deadline: parentReviewNow.Add(d)}
	}
	asSession := func(d *parentReviewDeps) { d.strandName = parentReviewSession }
	withRecord := func(rec pullrequest.Record, found bool) func(*parentReviewDeps) {
		return func(d *parentReviewDeps) {
			asSession(d)
			d.prRecord = func() (pullrequest.Record, bool, error) { return rec, found, nil }
		}
	}
	autonomyOff := false
	autonomyOn := true
	later := parentReviewNow.Add(2 * time.Hour)
	tests := []struct {
		name string
		asks []burlerengine.ParentAsk
		// satisfiedThen, when set, is recorded after the latest of asks is answered satisfied.
		satisfiedThen []burlerengine.ParentAsk
		setup         func(*parentReviewDeps)
		// want lists text the refusal must hold; empty wants an answer.
		want       []string
		wantStates []burlerengine.ParentAskState
		// wantOK is the envelope an answered call prints.
		wantOK map[string]any
	}{
		{
			name:       "a run with no pull request record is refused",
			setup:      withRecord(pullrequest.Record{}, false),
			want:       []string{"no pull request record", parentReviewWayAskNoRecord},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "a record holding no autonomy is refused",
			setup:      withRecord(pullrequest.Record{Number: 7, StrandName: parentReviewSession}, true),
			want:       []string{"not recorded under autonomy", parentReviewWayAskOperator},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "a record holding autonomy false is refused",
			setup:      withRecord(pullrequest.Record{Number: 7, StrandName: parentReviewSession, Autonomy: &autonomyOff}, true),
			want:       []string{"not recorded under autonomy", parentReviewWayAskOperator},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "a caller that is not the recorded session is refused",
			setup:      func(d *parentReviewDeps) { d.strandName = "ly:task-a:other" },
			want:       []string{`"ly:task-a:other"`, parentReviewWayAskPane},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "the unnamed shell is refused",
			want:       []string{`""`, parentReviewWayAskPane},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "a record that names no session refuses even the unnamed shell",
			setup:      withRecord(pullrequest.Record{Number: 7, Autonomy: &autonomyOn}, true),
			want:       []string{parentReviewWayAskPane},
			wantStates: []burlerengine.ParentAskState{},
		},
		{
			name:       "an ask awaiting its answer at the head is refused naming the await verb",
			asks:       []burlerengine.ParentAsk{prAsk(burlerengine.ParentAskOpen, time.Hour)},
			setup:      asSession,
			want:       []string{"ask 1", "lym loom parent-review await task-a"},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen},
		},
		{
			name:          "a satisfied answer at the head is refused naming pr-continue while a later ask is open",
			asks:          []burlerengine.ParentAsk{prAsk(burlerengine.ParentAskOpen, time.Hour)},
			satisfiedThen: []burlerengine.ParentAsk{prAsk(burlerengine.ParentAskOpen, time.Hour)},
			setup:         asSession,
			want:          []string{"a satisfied answer from your parent stands at this head", "lym loom pr-continue"},
			wantStates:    []burlerengine.ParentAskState{burlerengine.ParentAskDone, burlerengine.ParentAskOpen},
		},
		{
			name:       "the session records a PR-Review ask at the head after an earlier review ask",
			asks:       []burlerengine.ParentAsk{{Tag: "[code-review request]", State: burlerengine.ParentAskTimeout}},
			setup:      asSession,
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskTimeout, burlerengine.ParentAskOpen},
			wantOK: map[string]any{
				"slug": "task-a", "ask": float64(2), "state": "open", "head": parentReviewHead, "deadline": parentReviewNow.Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
		{
			name: "an ask unanswered past its deadline is followed by a new ask",
			asks: []burlerengine.ParentAsk{prAsk(burlerengine.ParentAskOpen, time.Hour)},
			setup: func(d *parentReviewDeps) {
				asSession(d)
				d.now = func() time.Time { return later }
				d.askPR = parentReviewPRAsker(d.files, d.now)
			},
			wantStates: []burlerengine.ParentAskState{burlerengine.ParentAskOpen, burlerengine.ParentAskOpen},
			wantOK: map[string]any{
				"slug": "task-a", "ask": float64(2), "state": "open", "head": parentReviewHead, "deadline": later.Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
		{
			name: "a failed ask is an error naming the cause and running the verb again",
			setup: func(d *parentReviewDeps) {
				asSession(d)
				d.askPR = func(pullrequest.Record, string, string) (burlerengine.ParentAsk, error) {
					return burlerengine.ParentAsk{}, errors.New("disk full")
				}
			},
			want:       []string{"record and send the ask", "disk full", parentReviewWayAskRetry},
			wantStates: []burlerengine.ParentAskState{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := parentReviewFixture(t, tt.asks...)
			if len(tt.satisfiedThen) > 0 {
				latest, _ := func() (burlerengine.ParentAsk, bool) {
					record, _, err := burlerengine.ReadParentAsks(deps.files)
					if err != nil {
						t.Fatalf("ReadParentAsks() error = %v", err)
					}
					return record.Latest()
				}()
				if _, err := burlerengine.AnswerParentAskOrUnreached(deps.files, latest.Number, "", parentReviewNow, nil); err != nil {
					t.Fatalf("AnswerParentAskOrUnreached() error = %v", err)
				}
				for _, ask := range tt.satisfiedThen {
					if _, err := burlerengine.RecordParentAsk(deps.files, ask); err != nil {
						t.Fatalf("RecordParentAsk() error = %v", err)
					}
				}
			}
			if tt.setup != nil {
				tt.setup(&deps)
			}
			var out bytes.Buffer
			var gotClaim string
			recordAsk := deps.askPR
			deps.askPR = func(rec pullrequest.Record, head, strandClaim string) (burlerengine.ParentAsk, error) {
				gotClaim = strandClaim
				return recordAsk(rec, head, strandClaim)
			}
			code := parentReviewAskVerb(&out, "task-a", deps)
			record, _, err := burlerengine.ReadParentAsks(deps.files)
			if err != nil {
				t.Fatalf("ReadParentAsks() error = %v", err)
			}
			states := []burlerengine.ParentAskState{}
			for _, ask := range record.Asks {
				states = append(states, ask.State)
			}
			if !reflect.DeepEqual(states, tt.wantStates) {
				t.Errorf("ask states = %v; want %v", states, tt.wantStates)
			}
			if len(tt.want) > 0 {
				env := envelope.RequireErr(t, out.String(), "loom: parent-review ask: ")
				for _, want := range tt.want {
					if !strings.Contains(env.Error, want) {
						t.Errorf("refusal = %q; want it to hold %q", env.Error, want)
					}
				}
				if code == 0 {
					t.Errorf("parentReviewAskVerb() = 0; want a non-zero exit")
				}
				return
			}
			env := envelope.RequireOK(t, out.String())
			if !reflect.DeepEqual(env.Raw, tt.wantOK) {
				t.Errorf("envelope = %v; want %v", env.Raw, tt.wantOK)
			}
			if gotClaim != parentReviewSession {
				t.Errorf("the ask was signed as %q; want the session's strand %q", gotClaim, parentReviewSession)
			}
			if latest, _ := record.Latest(); latest.Tag != landingshed.PRReviewAskLineTag || latest.Digest != parentReviewHead {
				t.Errorf("latest ask tag %q, digest %q; want the PR-Review tag and the head %q", latest.Tag, latest.Digest, parentReviewHead)
			}
		})
	}
}

// TestParentReviewPRAskFunc_ReturnsTheErrorOfAHubItCannotResolveTheParentIn drives the ask seam over a run's location.
// A hub with no parent orch resolvable returns that error and records no ask.
func TestParentReviewPRAskFunc_ReturnsTheErrorOfAHubItCannotResolveTheParentIn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task-a", AnchorRel: "."}

	ask, err := parentReviewPRAskFunc(location)(pullrequest.Record{URL: "https://example.test/pull/7", Head: "task-a", Base: "main"}, parentReviewHead, parentReviewSession)

	if err == nil || !strings.Contains(err.Error(), "not initialized here") {
		t.Errorf("ask seam error = %v; want the hub's not-initialized refusal", err)
	}
	if ask.Number != 0 {
		t.Errorf("ask = %+v; want no ask recorded", ask)
	}
}

// TestParentReviewAwaitVerb verifies the await answers ready once the latest ask has ended or reaches its deadline.
// The answer names the state, a not-sent or undelivered ask's reason and a done ask's report.
// The await answers not ready once its cap runs out first.
func TestParentReviewAwaitVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name     string
		asks     []burlerengine.ParentAsk
		awaitCap time.Duration
		// answerAfter, when above zero, answers the latest ask satisfied once the fake clock has run that long.
		answerAfter time.Duration
		want        map[string]any
		wantErr     []string
		wantSlept   time.Duration
		// sleepErr, when set, is what the fake sleep returns, as a cancelled context's sleep does.
		sleepErr error
	}{
		{
			name:        "an answer during the wait answers ready with the report",
			asks:        []burlerengine.ParentAsk{openParentAsk(time.Hour)},
			awaitCap:    time.Minute,
			answerAfter: 2 * time.Second,
			want:        map[string]any{"ready": true, "ask": float64(1), "state": "done", "report": "<report 1>"},
			wantSlept:   2 * time.Second,
		},
		{
			name:     "an ask already timed out answers ready at once",
			asks:     []burlerengine.ParentAsk{{State: burlerengine.ParentAskTimeout}},
			awaitCap: time.Minute,
			want:     map[string]any{"ready": true, "ask": float64(1), "state": "timeout"},
		},
		{
			name:     "a not-sent ask answers ready with its reason",
			asks:     []burlerengine.ParentAsk{{State: burlerengine.ParentAskNotSent, Reason: "raise review_timeout_min"}},
			awaitCap: time.Minute,
			want:     map[string]any{"ready": true, "ask": float64(1), "state": "not-sent", "reason": "raise review_timeout_min"},
		},
		{
			name:     "an undelivered ask answers ready with its reason",
			asks:     []burlerengine.ParentAsk{{State: burlerengine.ParentAskUndelivered, Reason: "the request was not delivered: refused"}},
			awaitCap: time.Minute,
			want:     map[string]any{"ready": true, "ask": float64(1), "state": "undelivered", "reason": "the request was not delivered: refused"},
		},
		{
			name:      "the deadline reached during the wait sets the ask timeout and answers ready",
			asks:      []burlerengine.ParentAsk{openParentAsk(3 * time.Second)},
			awaitCap:  time.Minute,
			want:      map[string]any{"ready": true, "ask": float64(1), "state": "timeout"},
			wantSlept: 3 * time.Second,
		},
		{
			name:     "a cap that runs out first answers not ready",
			asks:     []burlerengine.ParentAsk{openParentAsk(time.Hour)},
			awaitCap: 2500 * time.Millisecond,
			want: map[string]any{
				"ready": false, "ask": float64(1), "state": "open", "deadline": "2026-11-11T13:00:00Z",
				"message": "the parent has not answered and the ask's deadline has not passed; call this command again",
			},
			wantSlept: 2500 * time.Millisecond,
		},
		{name: "a run with no ask is refused", awaitCap: time.Minute, wantErr: []string{parentReviewWayAwaitNoAsk}},
		{
			name:     "a wait cut short is refused",
			asks:     []burlerengine.ParentAsk{openParentAsk(time.Hour)},
			awaitCap: time.Minute,
			sleepErr: context.Canceled,
			wantErr:  []string{"the wait was cut short", parentReviewWayAwaitRetry},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := parentReviewFixture(t, tt.asks...)
			clock := parentReviewNow
			deps.now = func() time.Time { return clock }
			deps.awaitCap = tt.awaitCap
			deps.sleep = func(_ context.Context, d time.Duration) error {
				if d > time.Second {
					t.Errorf("sleep(%s); want at most one second between reads", d)
				}
				if tt.sleepErr != nil {
					return tt.sleepErr
				}
				clock = clock.Add(d)
				if tt.answerAfter > 0 && !clock.Before(parentReviewNow.Add(tt.answerAfter)) {
					if _, err := burlerengine.AnswerParentAskOrUnreached(deps.files, len(tt.asks), "", clock, nil); err != nil {
						t.Errorf("AnswerParentAskOrUnreached() error = %v", err)
					}
				}
				return nil
			}
			var out bytes.Buffer
			parentReviewAwaitVerb(context.Background(), &out, deps)
			if len(tt.wantErr) > 0 {
				for _, want := range tt.wantErr {
					envelope.RequireErr(t, out.String(), want)
				}
				return
			}
			env := envelope.RequireOK(t, out.String())
			if report, ok := tt.want["report"]; ok && report == "<report 1>" {
				tt.want["report"] = deps.files.ReportPath(1)
			}
			if !reflect.DeepEqual(env.Raw, tt.want) {
				t.Errorf("envelope = %v; want %v", env.Raw, tt.want)
			}
			if slept := clock.Sub(parentReviewNow); slept != tt.wantSlept {
				t.Errorf("await slept %s; want %s", slept, tt.wantSlept)
			}
			if tt.want["state"] == "timeout" && len(tt.asks) > 0 && tt.asks[0].State == burlerengine.ParentAskOpen {
				record, _, _ := burlerengine.ReadParentAsks(deps.files)
				if latest, _ := record.Latest(); latest.State != burlerengine.ParentAskTimeout {
					t.Errorf("latest ask state = %s; want timeout written", latest.State)
				}
			}
		})
	}
}
