package shuttleengine

import (
	"reflect"
	"testing"
	"time"
)

// TestSessionFold covers how session signals and facts reduce to a session state.
// Each row folds its steps in order on one SessionFold and compares the final state and the whole history.
// The rows cover every state and cause, the precedence of the facts over the signals, each source of Since, the re-judging of the newest turn end once outputs appear, and the transcript markers.
func TestSessionFold(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	readAt := at(100)

	turnStart := func(seconds int) SessionSignal {
		return SessionSignal{Kind: SessionSignalTurnStart, At: at(seconds)}
	}
	turnEnd := func(seconds int, tasks ...BackgroundTask) SessionSignal {
		return SessionSignal{Kind: SessionSignalTurnEnd, At: at(seconds), Outstanding: tasks}
	}
	apiErrorTurnEnd := func(seconds int, text string) SessionSignal {
		return SessionSignal{Kind: SessionSignalAPIErrorTurnEnd, At: at(seconds), ErrorText: text}
	}
	ask := func(seconds int) SessionSignal { return SessionSignal{Kind: SessionSignalAsk, At: at(seconds)} }
	idleNotice := func(seconds int) SessionSignal {
		return SessionSignal{Kind: SessionSignalIdleNotice, At: at(seconds)}
	}
	sessionEnd := func(seconds int, reason string, endsProcess bool) SessionSignal {
		return SessionSignal{Kind: SessionSignalSessionEnd, At: at(seconds), Reason: reason, EndsProcess: endsProcess}
	}
	shell := BackgroundTask{Kind: BackgroundShell, ID: "bsh1", Label: "sleep 600", Signal: SignalPayload}

	// factsWith builds alive facts read at readAt, then applies each change.
	factsWith := func(changes ...func(*SessionFacts)) SessionFacts {
		facts := SessionFacts{Liveness: LivenessAlive, ReadAt: readAt}
		for _, change := range changes {
			change(&facts)
		}
		return facts
	}
	outputs := func(f *SessionFacts) { f.OutputsExist = true }
	interactive := func(f *SessionFacts) { f.Interactive = true }
	unproven := func(f *SessionFacts) { f.Liveness = LivenessUnproven }
	dead := func(f *SessionFacts) { f.Liveness = LivenessDead }
	unreadable := func(f *SessionFacts) { f.EventsUnreadable = true }

	state := func(name SessionStateName, cause string, since time.Time) SessionState {
		return SessionState{Name: name, Cause: cause, Since: since}
	}
	type step struct {
		signals []SessionSignal
		facts   SessionFacts
	}
	tests := []struct {
		name        string
		steps       []step
		wantState   SessionState
		wantHistory []SessionState
	}{
		{
			name:        "no signal at all is unknown, stamped with the reading",
			steps:       []step{{nil, factsWith()}},
			wantState:   state(SessionUnknown, SessionCauseNoSignal, readAt),
			wantHistory: []SessionState{state(SessionUnknown, SessionCauseNoSignal, readAt)},
		},
		{
			name:        "an unreadable events file is unknown, ahead of the no-signal reading",
			steps:       []step{{nil, factsWith(unreadable)}},
			wantState:   state(SessionUnknown, SessionCauseEventsUnreadable, readAt),
			wantHistory: []SessionState{state(SessionUnknown, SessionCauseEventsUnreadable, readAt)},
		},
		{
			name:        "an unproven liveness outranks the signals and an unreadable file",
			steps:       []step{{[]SessionSignal{turnStart(1)}, factsWith(unproven, unreadable)}},
			wantState:   state(SessionUnknown, SessionCauseLivenessUnproven, readAt),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), state(SessionUnknown, SessionCauseLivenessUnproven, readAt)},
		},
		{
			name:        "a dead process outranks the signals, stamped with the reading",
			steps:       []step{{[]SessionSignal{turnEnd(1)}, factsWith(dead, outputs)}},
			wantState:   state(SessionDead, SessionCauseProcessGone, readAt),
			wantHistory: []SessionState{state(SessionIdleDone, SessionCauseDone, at(1)), state(SessionDead, SessionCauseProcessGone, readAt)},
		},
		{
			name:        "a dead process outranks an unreadable events file",
			steps:       []step{{nil, factsWith(dead, unreadable)}},
			wantState:   state(SessionDead, SessionCauseProcessGone, readAt),
			wantHistory: []SessionState{state(SessionDead, SessionCauseProcessGone, readAt)},
		},
		{
			name:        "a process-ending session end outranks even an unproven liveness",
			steps:       []step{{[]SessionSignal{turnStart(1), sessionEnd(2, "logout", true)}, factsWith(unproven)}},
			wantState:   state(SessionDead, "logout", at(2)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), state(SessionDead, "logout", at(2))},
		},
		{
			name:        "a clear session end is unknown with its reason until the next signal",
			steps:       []step{{[]SessionSignal{turnEnd(1), sessionEnd(2, "clear", false)}, factsWith(outputs)}},
			wantState:   state(SessionUnknown, "clear", at(2)),
			wantHistory: []SessionState{state(SessionIdleDone, SessionCauseDone, at(1)), state(SessionUnknown, "clear", at(2))},
		},
		{
			name:        "a turn start after a clear session end reads busy again",
			steps:       []step{{[]SessionSignal{sessionEnd(1, "resume", false), turnStart(2)}, factsWith()}},
			wantState:   state(SessionBusy, SessionCauseTurn, at(2)),
			wantHistory: []SessionState{state(SessionUnknown, "resume", at(1)), state(SessionBusy, SessionCauseTurn, at(2))},
		},
		{
			name:        "a turn start is busy on the turn, and a second one records nothing",
			steps:       []step{{[]SessionSignal{turnStart(1), turnStart(2)}, factsWith()}},
			wantState:   state(SessionBusy, SessionCauseTurn, at(1)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1))},
		},
		{
			name:  "a turn end with outstanding tasks is busy on the background",
			steps: []step{{[]SessionSignal{turnEnd(1, shell)}, factsWith(outputs)}},
			wantState: SessionState{
				Name: SessionBusy, Cause: SessionCauseBackground, Outstanding: []BackgroundTask{shell}, Since: at(1),
			},
			wantHistory: []SessionState{{Name: SessionBusy, Cause: SessionCauseBackground, Outstanding: []BackgroundTask{shell}, Since: at(1)}},
		},
		{
			name:        "a turn end with the outputs present is idle-done",
			steps:       []step{{[]SessionSignal{turnStart(1), turnEnd(2)}, factsWith(outputs)}},
			wantState:   state(SessionIdleDone, SessionCauseDone, at(2)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), state(SessionIdleDone, SessionCauseDone, at(2))},
		},
		{
			name:        "an autonomous turn end without the outputs is idle-stalled",
			steps:       []step{{[]SessionSignal{turnEnd(1)}, factsWith()}},
			wantState:   state(SessionIdleStalled, SessionCauseNoOutput, at(1)),
			wantHistory: []SessionState{state(SessionIdleStalled, SessionCauseNoOutput, at(1))},
		},
		{
			name:        "an interactive turn end without the outputs is asking",
			steps:       []step{{[]SessionSignal{turnEnd(1)}, factsWith(interactive)}},
			wantState:   state(SessionAsking, SessionCauseAwaitingInput, at(1)),
			wantHistory: []SessionState{state(SessionAsking, SessionCauseAwaitingInput, at(1))},
		},
		{
			name:        "an API-error turn end is idle-stalled with the error's text, outputs or not",
			steps:       []step{{[]SessionSignal{apiErrorTurnEnd(1, "API Error: 529")}, factsWith(outputs)}},
			wantState:   SessionState{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: "API Error: 529", Since: at(1)},
			wantHistory: []SessionState{{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: "API Error: 529", Since: at(1)}},
		},
		{
			name:        "an ask is asking",
			steps:       []step{{[]SessionSignal{turnStart(1), ask(2)}, factsWith()}},
			wantState:   state(SessionAsking, SessionCauseAsk, at(2)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), state(SessionAsking, SessionCauseAsk, at(2))},
		},
		{
			name:        "an idle notice after a turn end changes nothing",
			steps:       []step{{[]SessionSignal{turnEnd(1), idleNotice(2)}, factsWith(outputs)}},
			wantState:   state(SessionIdleDone, SessionCauseDone, at(1)),
			wantHistory: []SessionState{state(SessionIdleDone, SessionCauseDone, at(1))},
		},
		{
			name:        "an idle notice right after a turn start is a contradiction",
			steps:       []step{{[]SessionSignal{turnStart(1), idleNotice(2)}, factsWith()}},
			wantState:   state(SessionUnknown, SessionCauseContradiction, at(2)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), state(SessionUnknown, SessionCauseContradiction, at(2))},
		},
		{
			name:  "an interrupt marker turns the busy turn into idle-stalled, stamped with the marker's time",
			steps: []step{{[]SessionSignal{turnStart(0)}, factsWith(func(f *SessionFacts) { f.Interrupted, f.InterruptAt = true, at(50) })}},
			// The turn start has no time here, and its later or missing At changes nothing.
			wantState:   state(SessionIdleStalled, SessionCauseInterrupt, at(50)),
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(0)), state(SessionIdleStalled, SessionCauseInterrupt, at(50))},
		},
		{
			name:        "an interrupt marker leaves a busy background turn end and a finished turn alone",
			steps:       []step{{[]SessionSignal{turnStart(1), turnEnd(2, shell)}, factsWith(func(f *SessionFacts) { f.Interrupted, f.InterruptAt = true, at(50) })}},
			wantState:   SessionState{Name: SessionBusy, Cause: SessionCauseBackground, Outstanding: []BackgroundTask{shell}, Since: at(2)},
			wantHistory: []SessionState{state(SessionBusy, SessionCauseTurn, at(1)), {Name: SessionBusy, Cause: SessionCauseBackground, Outstanding: []BackgroundTask{shell}, Since: at(2)}},
		},
		{
			name: "a transcript API-error marker turns the newest turn end into idle-stalled, stamped with the turn end",
			steps: []step{{[]SessionSignal{turnEnd(1)}, factsWith(outputs, func(f *SessionFacts) {
				f.APIError, f.APIErrorText = true, "API Error: 500"
			})}},
			wantState: SessionState{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: "API Error: 500", Since: at(1)},
			wantHistory: []SessionState{
				state(SessionIdleDone, SessionCauseDone, at(1)),
				{Name: SessionIdleStalled, Cause: SessionCauseAPIError, Detail: "API Error: 500", Since: at(1)},
			},
		},
		{
			name: "a transcript API-error marker does not reach a turn end that a turn start follows",
			steps: []step{{[]SessionSignal{turnEnd(1), turnStart(2)}, factsWith(func(f *SessionFacts) {
				f.APIError, f.APIErrorText = true, "API Error: 500"
			})}},
			wantState:   state(SessionBusy, SessionCauseTurn, at(2)),
			wantHistory: []SessionState{state(SessionIdleStalled, SessionCauseNoOutput, at(1)), state(SessionBusy, SessionCauseTurn, at(2))},
		},
		{
			name:      "every turn end of one fold is judged with that fold's outputs, and the history keeps each change",
			steps:     []step{{[]SessionSignal{turnStart(1), turnEnd(2, shell), turnEnd(3), turnStart(4), turnEnd(5)}, factsWith(outputs)}},
			wantState: state(SessionIdleDone, SessionCauseDone, at(5)),
			wantHistory: []SessionState{
				state(SessionBusy, SessionCauseTurn, at(1)),
				{Name: SessionBusy, Cause: SessionCauseBackground, Outstanding: []BackgroundTask{shell}, Since: at(2)},
				state(SessionIdleDone, SessionCauseDone, at(3)),
				state(SessionBusy, SessionCauseTurn, at(4)),
				state(SessionIdleDone, SessionCauseDone, at(5)),
			},
		},
		{
			name: "outputs that appear at a later fold with no new signal make the stalled turn end idle-done, stamped with the reading",
			steps: []step{
				{[]SessionSignal{turnStart(1), turnEnd(2)}, factsWith()},
				{nil, factsWith(outputs)},
			},
			wantState: state(SessionIdleDone, SessionCauseDone, readAt),
			wantHistory: []SessionState{
				state(SessionBusy, SessionCauseTurn, at(1)),
				state(SessionIdleStalled, SessionCauseNoOutput, at(2)),
				state(SessionIdleDone, SessionCauseDone, readAt),
			},
		},
		{
			name: "outputs that appear at a later fold make an interactive awaiting-input turn end idle-done",
			steps: []step{
				{[]SessionSignal{turnEnd(1)}, factsWith(interactive)},
				{nil, factsWith(interactive, outputs)},
			},
			wantState:   state(SessionIdleDone, SessionCauseDone, readAt),
			wantHistory: []SessionState{state(SessionAsking, SessionCauseAwaitingInput, at(1)), state(SessionIdleDone, SessionCauseDone, readAt)},
		},
		{
			name: "outputs that vanish at a later fold never turn idle-done negative",
			steps: []step{
				{[]SessionSignal{turnEnd(1)}, factsWith(outputs)},
				{nil, factsWith()},
			},
			wantState:   state(SessionIdleDone, SessionCauseDone, at(1)),
			wantHistory: []SessionState{state(SessionIdleDone, SessionCauseDone, at(1))},
		},
		{
			name: "a later fold re-judges only the newest turn end, so a newer turn start is left busy",
			steps: []step{
				{[]SessionSignal{turnEnd(1), turnStart(2)}, factsWith()},
				{nil, factsWith(outputs)},
			},
			wantState:   state(SessionBusy, SessionCauseTurn, at(2)),
			wantHistory: []SessionState{state(SessionIdleStalled, SessionCauseNoOutput, at(1)), state(SessionBusy, SessionCauseTurn, at(2))},
		},
		{
			name: "a state from the reading keeps its first Since across folds, and a signal moves it on",
			steps: []step{
				{nil, factsWith(dead)},
				{nil, func() SessionFacts { f := factsWith(dead); f.ReadAt = at(200); return f }()},
				{[]SessionSignal{turnStart(300)}, factsWith(func(f *SessionFacts) { f.ReadAt = at(300) })},
			},
			wantState:   state(SessionBusy, SessionCauseTurn, at(300)),
			wantHistory: []SessionState{state(SessionDead, SessionCauseProcessGone, readAt), state(SessionBusy, SessionCauseTurn, at(300))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fold SessionFold
			for _, s := range tt.steps {
				fold.Fold(s.signals, s.facts)
			}
			if got := fold.State(); !reflect.DeepEqual(got, tt.wantState) {
				t.Errorf("State() = %+v; want %+v", got, tt.wantState)
			}
			if got := fold.History(); !reflect.DeepEqual(got, tt.wantHistory) {
				t.Errorf("History() = %+v; want %+v", got, tt.wantHistory)
			}
		})
	}
}
