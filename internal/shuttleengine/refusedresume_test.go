// refusedresume_test.go covers a resume whose copied transcript the provider refused.
// Its first turn ends on an API error.
// Such a session starts fresh in the same run.

package shuttleengine

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// refusedAsk is one question to Runner.RefusedResume about a turn end of the harness's run.
type refusedAsk struct {
	// turnEnd is the events line of the turn end asked about.
	turnEnd string
	want    bool
}

// refusedRow is one case of TestWait_RefusedResumeStartsFresh.
// A row with an ask only seeds the run's record and asks Runner.RefusedResume; every other row waits on the run.
type refusedRow struct {
	waitRelaunchRow
	ask *refusedAsk
}

// TestWait_RefusedResumeStartsFresh pins when a resume whose first turn ends on an API error starts fresh in its run.
// It also pins that Runner.RefusedResume answers the same question for a run found by its strand GUID.
func TestWait_RefusedResumeStartsFresh(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		firstAPIError = "START\nAPIERR:overloaded\n"
		limitAPIError = "APIERR:limit"
	)
	// notRelaunched fails unless Wait ended on the API error with no relaunch and no re-send.
	notRelaunched := func(t *testing.T, h *waitRelaunchHarness) {
		t.Helper()
		if h.err != nil || h.result.Outcome != OutcomeAPIError {
			t.Errorf("Wait() = outcome %q, error %v; want the run ended on the API errors as it ends today", h.result.Outcome, h.err)
		}
		if callsOf(h.reed.CallLog, "EndStrandProcess")+callsOf(h.reed.CallLog, "RelaunchStrand") != 0 {
			t.Errorf("reed calls = %v; want no relaunch", h.reed.CallLog)
		}
	}
	// startedFresh fails unless the run relaunched once, fresh, in the same run.
	startedFresh := func(t *testing.T, h *waitRelaunchHarness) {
		t.Helper()
		h.checkRelaunched(t)
		if spec := h.engine.prepared[0]; spec.ResumeSessionID != "" || spec.Prompt != "do the task" || spec.SkipSkillLoadTurn {
			t.Errorf("the relaunch's Prepare spec = %+v; want a fresh launch of the run's own prompt", spec)
		}
		if got := h.reed.RelaunchStrandCalls[0].Spec.SessionID; got != "session-fresh" || h.record.SessionID != "session-fresh" || h.record.TranscriptCopied {
			t.Errorf("relaunched session %q, record %q, copied %v; want session-fresh in both and no copy", got, h.record.SessionID, h.record.TranscriptCopied)
		}
		if slices.ContainsFunc(h.reed.SendTextCalls, func(call struct {
			GUID   string
			Text   string
			Submit bool
		}) bool {
			return call.Text == apiErrorRetryLine
		}) {
			t.Errorf("texts typed = %v; want no API-error re-send", h.reed.SendTextCalls)
		}
	}

	tests := []refusedRow{
		{waitRelaunchRow: waitRelaunchRow{name: "ACopiedResumesFirstTurnEndOnAnAPIErrorStartsFreshInTheSameRun", events: firstAPIError, copied: true, retries: 2, finishes: true, check: startedFresh}},
		{waitRelaunchRow: waitRelaunchRow{name: "AResumeThatCopiedNothingReSendsAsToday", events: "START\nAPIERR:a\nAPIERR:b\n", retries: 1, check: notRelaunched}},
		{waitRelaunchRow: waitRelaunchRow{
			name: "ACopiedResumesFirstTurnEndOnTheUsageLimitIsNotRefused", events: "START\n" + limitAPIError + "\nAPIERR:b\n", copied: true, retries: 1,
			setup: func(h *waitRelaunchHarness) { h.engine.limitLine = limitAPIError }, check: notRelaunched,
		}},
		{waitRelaunchRow: waitRelaunchRow{
			name: "ACopiedResumesSecondTurnEndOnAnAPIErrorReSendsAsToday", events: "START\nSTOP:ok\nAPIERR:a\nAPIERR:b\n", copied: true, retries: 1, check: notRelaunched,
		}},
		{waitRelaunchRow: waitRelaunchRow{name: "ACopiedResumeOnASpecWithoutRetriesStartsFresh", events: firstAPIError, copied: true, finishes: true, check: startedFresh}},
		{waitRelaunchRow: waitRelaunchRow{
			name: "ACopiedResumeWhoseLoadTurnEndedReadsItsFirstTaskTurnsAPIErrorAsAnyOther", events: "START\nSTOP:ok\nAPIERR:a\nAPIERR:b\n",
			copied: true, promptOffset: int64(len("START\nSTOP:ok\n")), retries: 1, check: notRelaunched,
		}},
		{waitRelaunchRow: waitRelaunchRow{
			name: "AResumeWhoseFirstTurnEndPastItsLaunchOffsetIsAnAPIErrorStartsFresh", events: "START\nAPIERR:old\nSTART\nAPIERR:new\n",
			copied: true, launchOffset: int64(len("START\nAPIERR:old\n")), finishes: true, check: startedFresh,
		}},

		{
			waitRelaunchRow: waitRelaunchRow{name: "RefusedResumeAnswersTrueForAFirstAPIErrorTurnEndOfACopiedResume", events: firstAPIError, copied: true},
			ask:             &refusedAsk{turnEnd: "APIERR:overloaded", want: true},
		},
		{
			waitRelaunchRow: waitRelaunchRow{name: "RefusedResumeAnswersFalseWhenNothingWasCopied", events: firstAPIError},
			ask:             &refusedAsk{turnEnd: "APIERR:overloaded"},
		},
		{
			waitRelaunchRow: waitRelaunchRow{name: "RefusedResumeAnswersFalseForASecondTurnEnd", events: "START\nSTOP:ok\nAPIERR:a\n", copied: true},
			ask:             &refusedAsk{turnEnd: "APIERR:a"},
		},
		{
			waitRelaunchRow: waitRelaunchRow{
				name: "RefusedResumeAnswersFalseForTheUsageLimit", events: "START\n" + limitAPIError + "\n", copied: true,
				setup: func(h *waitRelaunchHarness) { h.engine.limitLine = limitAPIError },
			},
			ask: &refusedAsk{turnEnd: limitAPIError},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWaitRelaunchHarness(t, tt.waitRelaunchRow)
			if tt.ask != nil {
				if err := saveRunState(h.runDir, h.run.state); err != nil {
					t.Fatalf("seed the run record: %v", err)
				}
				got, err := h.fx.Runner.RefusedResume("strand-1", Event{Raw: []byte(tt.ask.turnEnd)})
				if err != nil || got != tt.ask.want {
					t.Errorf("RefusedResume(strand-1, %q) = %v, %v; want %v", tt.ask.turnEnd, got, err, tt.ask.want)
				}
				if _, err := h.fx.Runner.RefusedResume("nope", Event{}); err == nil {
					t.Error("RefusedResume(nope) error = nil; want an unknown strand refused")
				}
				return
			}
			h.wait()
			tt.check(t, h)
		})
	}
}

// TestStart_RefusedResumeOfALoadTurnStartsFresh pins the fresh start of a refused copied resume.
// The refusal is an API error ending the first skill-load turn.
// The fresh launch loads the skills again, and the prompt is typed once.
func TestStart_RefusedResumeOfALoadTurnStartsFresh(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := startRelaunch(t, startRelaunchRow{resumes: true, turnEnd: "APIERR:overloaded"})
	if h.err != nil || h.run == nil {
		t.Fatalf("Start() = run %v, error %v; want the relaunched run", h.run, h.err)
	}
	if len(h.engine.prepared) != 2 || h.engine.prepared[1].ResumeSessionID != "" || !slices.Equal(h.engine.prepared[1].Skills, []string{"a"}) {
		t.Errorf("Prepare specs = %+v; want a second, fresh one that keeps the skills", h.engine.prepared)
	}
	record, _, err := loadRunState(h.runDir)
	if err != nil || record.SessionID != "session-fresh" || record.TranscriptCopied || record.RelaunchingAt != "" {
		t.Errorf("record = %+v, error %v; want session-fresh, no copy and no relaunching mark", record, err)
	}
	if got := strings.Join(typedTexts(h.reed.skillReed), "|"); got != "do the task" || h.reed.skillReed.loads != 2 {
		t.Errorf("typed = %q, load turns = %d; want the prompt once and one load turn for each launch", got, h.reed.skillReed.loads)
	}
	if len(h.reed.RelaunchStrandCalls) != 1 || h.reed.RelaunchStrandCalls[0].Spec.SessionID != "session-fresh" {
		t.Errorf("relaunches = %+v; want one, naming session-fresh", h.reed.RelaunchStrandCalls)
	}
}
