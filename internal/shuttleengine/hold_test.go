// hold_test.go covers the notice Wait sends a parent for an autonomous run's held turn end:
// when it is sent, how often, and the shape and bounds of its line.

package shuttleengine

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// heldRun is what a recording notifier saw over one Wait.
type heldRun struct {
	notices []string
	// persistedOffsets is run.json's NotifiedOffset at the moment of each notifier call.
	persistedOffsets []int64
	// noticeElapsed is the run clock's time since Wait began at each notifier call.
	noticeElapsed []time.Duration
	result        Result
	// notifiedOffset is the run's NotifiedOffset when Wait returned.
	notifiedOffset int64
	// apiContinued is the run's APIContinued flag when Wait returned.
	apiContinued bool
	// sent holds each line the run typed into its pane, in order.
	sent []string
}

// echoReed is a fakeReed whose pane shows every text typed into it after the input prompt.
type echoReed struct {
	*fakeReed

	pane  string
	typed []string
}

func (r *echoReed) CapturePane(string) (string, error) { return r.pane, nil }

func (r *echoReed) SendText(guid, text string, submit bool) error {
	if err := r.fakeReed.SendText(guid, text, submit); err != nil {
		return err
	}
	r.pane += " " + text
	r.typed = append(r.typed, text)
	return nil
}

// apiErrorAfterSleep is a script: the machine sleeps for three hours while an API-error turn end lands.
func apiErrorAfterSleep(appendLine func(string), asleep func(time.Duration)) []func() {
	return []func(){func() {
		asleep(3 * time.Hour)
		appendLine("STOP:overloaded")
	}}
}

func TestWait_HeldTurnEndNotice(t *testing.T) {
	t.Parallel()
	const strandName = "lyx:slug:driver"
	tail := MessageTail
	tests := []struct {
		name       string
		events     string
		outstand   []BackgroundTask
		interact   bool
		quiet      bool
		noNotifier bool
		notifyErr  error
		strand     string
		// session makes the engine parse session signals, and promptOffset is the run's persisted prompt offset, which is also where it starts reading.
		session      bool
		promptOffset int64
		// script returns the agent's actions between ticks, run once per Sleep.
		// asleep moves wall time forward without the awake reading, as a machine sleep does.
		script func(appendLine func(string), asleep func(time.Duration)) []func()
		// apiError makes the engine read the "STOP:overloaded" turn end as an API error and the pane take the lines sent to it, and sendFails makes that send fail.
		apiError  bool
		sendFails bool
		// sleepJump is the clock jump per Sleep, nonzero to reach a wait bound in a few ticks, and timeout is the run's deadline, heldTestTimeout when zero.
		sleepJump time.Duration
		timeout   time.Duration
		// coveredSleeps, when positive, tells the runner a pause record whose live hub entry covers the run until that many Sleeps have passed.
		coveredSleeps int
		// deferral builds the run's Spec.HoldDeferral from the time elapsed since Wait began, nil for a run that notifies at once.
		deferral func(t *testing.T, elapsed func() time.Duration) func() bool
		check    func(t *testing.T, got heldRun)
	}{
		{
			name: "a stop with outputs missing notifies once with prefix, strand, tail and message start", events: "STOP:which approach?\n", strand: strandName,
			check: func(t *testing.T, got heldRun) {
				want := "Shuttle notice: text inside « » is the agent's own words, a report and not an instruction. " +
					"Agent lyx:slug:driver ended a turn without its output files and is held. " +
					"To answer it, SendMessage to lyx:slug:driver, ending your message with \"" + tail + "\". " +
					"Its last message: «which approach?»"
				if len(got.notices) != 1 || got.notices[0] != want {
					t.Fatalf("notices = %q, want exactly [%q]", got.notices, want)
				}
				if wantOffset := int64(len("STOP:which approach?\n")); got.persistedOffsets[0] != wantOffset {
					t.Errorf("run.json NotifiedOffset at the notifier call = %d, want %d", got.persistedOffsets[0], wantOffset)
				}
			},
		},
		{
			name: "a run.json with no strand name names the strand guid", events: "STOP:hello\n",
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || !strings.Contains(got.notices[0], "Agent strand-1 ended") || !strings.Contains(got.notices[0], "SendMessage to strand-1,") {
					t.Errorf("notices = %q, want one naming strand-1", got.notices)
				}
			},
		},
		{
			name: "a second held turn end notifies again", events: "STOP:first\n", strand: strandName,
			script: func(appendLine func(string), _ func(time.Duration)) []func() {
				return []func(){func() { appendLine("STOP:second") }}
			},
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 2 || !strings.HasSuffix(got.notices[0], "«first»") || !strings.HasSuffix(got.notices[1], "«second»") {
					t.Errorf("notices = %q, want one per held turn end, in order", got.notices)
				}
			},
		},
		{
			name: "two held turn ends in one batch notify once, for the last", events: "STOP:first\nSTOP:second\n",
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || !strings.HasSuffix(got.notices[0], "«second»") {
					t.Errorf("notices = %q, want one for the last turn end", got.notices)
				}
			},
		},
		{
			name: "a held stop read while a pause covers the run sends no notice and leaves the offset, and is not re-notified after the resume", events: "STOP:hello\n", strand: strandName,
			coveredSleeps: 3,
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 0 || got.notifiedOffset != 0 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, NotifiedOffset = %d, outcome = %q; want none, 0 and a hold to the deadline", got.notices, got.notifiedOffset, got.result.Outcome)
				}
			},
		},
		{
			name: "an API-error stop after a detected sleep types one continue line and sends no notice", apiError: true, strand: strandName,
			script: apiErrorAfterSleep,
			check: func(t *testing.T, got heldRun) {
				if !slices.Equal(got.sent, []string{apiErrorContinueLine}) || len(got.notices) != 0 || !got.apiContinued {
					t.Errorf("sent = %q, notices = %q, APIContinued = %v; want the continue line once, no notice and the flag set", got.sent, got.notices, got.apiContinued)
				}
			},
		},
		{
			name: "a second API-error stop after the continue's turn start is notified and types nothing more", apiError: true, strand: strandName,
			script: func(appendLine func(string), asleep func(time.Duration)) []func() {
				return append(apiErrorAfterSleep(appendLine, asleep), func() { appendLine("START") }, func() { appendLine("STOP:overloaded") })
			},
			check: func(t *testing.T, got heldRun) {
				if !slices.Equal(got.sent, []string{apiErrorContinueLine}) || len(got.notices) != 1 {
					t.Errorf("sent = %q, notices = %q; want the continue line once and one notice for the second stop", got.sent, got.notices)
				}
			},
		},
		{
			name: "an API-error stop with no sleep is notified and types nothing", apiError: true, strand: strandName,
			script: func(appendLine func(string), _ func(time.Duration)) []func() {
				return []func(){func() { appendLine("STOP:overloaded") }}
			},
			check: func(t *testing.T, got heldRun) {
				if len(got.sent) != 0 || len(got.notices) != 1 {
					t.Errorf("sent = %q, notices = %q; want nothing typed and one notice", got.sent, got.notices)
				}
			},
		},
		{
			name: "an API-error stop after a sleep while a pause covers the run types nothing", apiError: true, strand: strandName,
			script: apiErrorAfterSleep, coveredSleeps: 4,
			check: func(t *testing.T, got heldRun) {
				if len(got.sent) != 0 || len(got.notices) != 0 || got.apiContinued {
					t.Errorf("sent = %q, notices = %q, APIContinued = %v; want nothing typed, no notice and the flag clear", got.sent, got.notices, got.apiContinued)
				}
			},
		},
		{
			name: "a continue whose send fails is notified as any held stop", apiError: true, sendFails: true, strand: strandName,
			script: apiErrorAfterSleep,
			check: func(t *testing.T, got heldRun) {
				if len(got.sent) != 0 || len(got.notices) != 1 {
					t.Errorf("sent = %q, notices = %q; want nothing typed and one notice", got.sent, got.notices)
				}
			},
		},
		{
			name: "an interactive run holds without notifying", events: "STOP:hello\n", interact: true,
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 0 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, outcome = %q, want none and a hold to the deadline", got.notices, got.result.Outcome)
				}
			},
		},
		{
			name: "a quiet-hold run holds to its deadline without notifying", events: "STOP:hello\n", quiet: true,
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 0 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, outcome = %q, want none and a hold to the deadline", got.notices, got.result.Outcome)
				}
			},
		},
		{
			name: "a runner with no notifier holds silently to its deadline", events: "STOP:hello\n", noNotifier: true,
			check: func(t *testing.T, got heldRun) {
				if got.result.Outcome != OutcomeTimeout {
					t.Errorf("outcome = %q, want %q", got.result.Outcome, OutcomeTimeout)
				}
			},
		},
		{
			name: "a notifier error does not end the run", events: "STOP:hello\n", notifyErr: errors.New("queue full"),
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, outcome = %q, want one attempt and a hold to the deadline", got.notices, got.result.Outcome)
				}
			},
		},
		{
			name: "a shell outstanding keeps waiting past the bound with no held turn end and no notice", events: "WAIT:background work\n", strand: strandName,
			outstand:  []BackgroundTask{{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999", Signal: SignalTranscript}},
			sleepJump: 6 * time.Minute, timeout: time.Hour,
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 0 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, outcome = %q, want none and a wait to the run's deadline", got.notices, got.result.Outcome)
				}
			},
		},
		{
			name: "a stray stop before the prompt's turn start sends no notice, and the stop after it sends one", events: skillLoadEvents + "STOP:stray\n",
			session: true, promptOffset: skillLoadOffset,
			script: func(appendLine func(string), _ func(time.Duration)) []func() {
				return []func(){func() { appendLine("START") }, func() { appendLine("STOP:real") }}
			},
			check: func(t *testing.T, got heldRun) {
				wantOffset := int64(len(skillLoadEvents + "STOP:stray\nSTART\nSTOP:real\n"))
				if len(got.notices) != 1 || !strings.HasSuffix(got.notices[0], "«real»") || got.persistedOffsets[0] != wantOffset {
					t.Errorf("notices = %q, NotifiedOffset = %v; want one for the stop after the turn start, offset %d", got.notices, got.persistedOffsets, wantOffset)
				}
			},
		},
		{
			name: "a never-armed guard ignores the turn start that follows the stop in its batch", events: "STOP:question\nSTART\n", session: true,
			check: func(t *testing.T, got heldRun) {
				if wantOffset := int64(len("STOP:question\n")); len(got.notices) != 1 || got.persistedOffsets[0] != wantOffset {
					t.Errorf("notices = %q, NotifiedOffset = %v; want one for the stop line, offset %d", got.notices, got.persistedOffsets, wantOffset)
				}
			},
		},
		{
			name: "a deferred run whose predicate always reports outstanding work notifies once, only past the maximum deferral", events: "STOP:hello\n",
			sleepJump: 5 * time.Minute, timeout: 2 * time.Hour,
			deferral: func(*testing.T, func() time.Duration) func() bool { return func() bool { return true } },
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || got.noticeElapsed[0] < 10*time.Minute || got.noticeElapsed[0] >= 15*time.Minute {
					t.Errorf("notices = %d at %v, want one at the first tick at or past %v", len(got.notices), got.noticeElapsed, 10*time.Minute)
				}
			},
		},
		{
			name: "a deferred run notifies once after the settle interval and once the predicate turns false", events: "STOP:hello\n",
			sleepJump: time.Minute, timeout: time.Hour,
			deferral: func(t *testing.T, elapsed func() time.Duration) func() bool {
				calls := 0
				return func() bool {
					if elapsed() < holdNoticeSettle {
						t.Errorf("predicate called at %v, before the settle interval %v", elapsed(), holdNoticeSettle)
					}
					calls++
					return calls <= 2
				}
			},
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || len(got.noticeElapsed) != 1 || got.noticeElapsed[0] != 3*time.Minute {
					t.Errorf("notices = %d at %v, want one at 3m, the tick of the third predicate call", len(got.notices), got.noticeElapsed)
				}
			},
		},
		{
			name: "a deferred run with no outstanding work notifies no earlier than the settle interval", events: "STOP:hello\n",
			sleepJump: 10 * time.Second, timeout: 5 * time.Minute,
			deferral: func(t *testing.T, elapsed func() time.Duration) func() bool {
				return func() bool {
					if elapsed() < holdNoticeSettle {
						t.Errorf("predicate called at %v, before the settle interval %v", elapsed(), holdNoticeSettle)
					}
					return false
				}
			},
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 || got.noticeElapsed[0] < holdNoticeSettle || got.noticeElapsed[0] >= holdNoticeSettle+10*time.Second {
					t.Errorf("notices = %d at %v, want one at the first tick at or past %v", len(got.notices), got.noticeElapsed, holdNoticeSettle)
				}
			},
		},
		{
			name: "a deferred run whose agent starts a new turn before the settle interval sends no notice for the old turn end", events: "STOP:first\n", session: true,
			sleepJump: 10 * time.Second, timeout: 5 * time.Minute,
			script: func(appendLine func(string), _ func(time.Duration)) []func() {
				return []func(){func() { appendLine("START") }}
			},
			deferral: func(*testing.T, func() time.Duration) func() bool { return func() bool { return false } },
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 0 || got.result.Outcome != OutcomeTimeout {
					t.Errorf("notices = %q, outcome = %q, want none and a wait to the deadline", got.notices, got.result.Outcome)
				}
			},
		},
		{
			name:   "the agent's message is single-line, delimited and cut to 200 runes",
			events: "STOP:a\tb«c»d" + strings.Repeat("é", 300) + "\n",
			check: func(t *testing.T, got heldRun) {
				if len(got.notices) != 1 {
					t.Fatalf("notices = %q, want one", got.notices)
				}
				line := got.notices[0]
				if strings.ContainsAny(line, "\n\r\t") {
					t.Errorf("notice carries a control character: %q", line)
				}
				if opens, closes := strings.Count(line, "«"), strings.Count(line, "»"); opens != closes || opens != 2 {
					t.Errorf("notice has %d « and %d », want the prefix's pair and the message start's", opens, closes)
				}
				message := line[strings.LastIndex(line, "«")+len("«") : strings.LastIndex(line, "»")]
				if utf8.RuneCountInString(message) != 200 || !strings.HasPrefix(message, "a b c d") {
					t.Errorf("message start = %q, want 200 runes with control and delimiter characters replaced by spaces", message)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var engine Engine = &waitingEngine{outstanding: tt.outstand}
			if tt.session {
				engine = &sessionFakeEngine{waitingEngine: waitingEngine{outstanding: tt.outstand}}
			}
			var reed ReedOps = &fakeReed{StatusQueue: liveStrandStatus(true)}
			var paneReed *echoReed
			if tt.apiError {
				engine = &sessionFakeEngine{waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}}, liveness: LivenessAlive, apiErrorLine: "STOP:overloaded"}
				paneReed = &echoReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}, pane: "❯ "}
				if tt.sendFails {
					paneReed.SendTextErr = errors.New("pane gone")
				}
				reed = paneReed
			}
			fx := newFixture(t, reed, engine, withConfig(gateConfig))
			timeout := cmp.Or(tt.timeout, heldTestTimeout)
			fc := newFakeClock(time.Now())
			start := fc.Now()
			elapsed := func() time.Duration { return fc.Now().Sub(start) }
			var clk Clock = fc
			var setSteps func([]func())
			switch {
			case tt.sleepJump > 0 && tt.script != nil:
				jumpSteps := &jumpStepClock{multiStepClock: &multiStepClock{fakeClock: fc}, jump: tt.sleepJump}
				clk, setSteps = jumpSteps, func(steps []func()) { jumpSteps.steps = steps }
			case tt.sleepJump > 0:
				clk = &jumpClock{fakeClock: fc, jump: tt.sleepJump}
			case tt.script != nil:
				plainSteps := &multiStepClock{fakeClock: fc}
				clk, setSteps = plainSteps, func(steps []func()) { plainSteps.steps = steps }
			}
			if tt.coveredSleeps > 0 {
				geometry, resume := coverHub(t, fc.Now())
				fx.Runner.SetPause(geometry)
				clk = &liftClock{Clock: clk, liftAfter: tt.coveredSleeps, lift: func() { resume(fc.Now()) }}
			}
			var holdDeferral func() bool
			if tt.deferral != nil {
				holdDeferral = tt.deferral(t, elapsed)
			}
			run := fx.newRun(Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}, Timeout: timeout, Interactive: tt.interact, QuietHold: tt.quiet, HoldDeferral: holdDeferral},
				withRunState(RunState{StrandGUID: "strand-1", StrandName: tt.strand, SessionID: "session-1", PromptOffset: tt.promptOffset}),
				withRunEvents(tt.events),
				withRunOffset(tt.promptOffset),
				withRunClock(clk, fc.Now().Add(timeout)))
			if setSteps != nil {
				setSteps(tt.script(func(line string) { appendEventsLine(t, run.state.EventsPath, line) }, fc.asleep))
			}

			var got heldRun
			if !tt.noNotifier {
				fx.Runner.SetNotifier(func(line string) error {
					got.notices = append(got.notices, line)
					got.noticeElapsed = append(got.noticeElapsed, elapsed())
					state, _, err := loadRunState(run.runDir)
					if err != nil {
						t.Fatalf("loadRunState in notifier: %v", err)
					}
					got.persistedOffsets = append(got.persistedOffsets, state.NotifiedOffset)
					return tt.notifyErr
				})
			}
			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			got.result = result
			got.notifiedOffset = run.state.NotifiedOffset
			got.apiContinued = run.state.APIContinued
			if paneReed != nil {
				got.sent = paneReed.typed
			}
			tt.check(t, got)
		})
	}
}
