// deliver_test.go pins delivery into panes through the fake seam: reminders, injects, the probe and sweep intervals, and the removed-holder sweep.

package creelengine_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// isProbeWait and isSweepWait tell the waits of the delivery workers (1 to 8 seconds) from the sweep's (longer than the probe ceiling) on the shared fake clock.
func isProbeWait(d time.Duration) bool { return d <= 8*time.Second }

func isSweepWait(d time.Duration) bool { return d > 8*time.Second }

// start runs a scheduler over the env's store and stops it when the test ends.
func (e *env) start(t *testing.T) {
	t.Helper()
	scheduler := creelengine.NewScheduler(e.store, e.del, e.clock)
	scheduler.Start()
	t.Cleanup(scheduler.Stop)
}

func (e *env) probeWaits(n int) []time.Duration { return e.clock.AwaitWaitsWhere(n, isProbeWait) }

func (e *env) sweepWaits(n int) []time.Duration { return e.clock.AwaitWaitsWhere(n, isSweepWait) }

// tick moves the clock by d and waits until one worker has probed and gone back to sleep.
func (e *env) tick(d time.Duration) {
	before := len(e.probeWaits(0))
	e.clock.Advance(d)
	e.probeWaits(before + 1)
}

// sweepStep moves the clock by the sweep's current wait and waits until the sweep has run and rearmed.
func (e *env) sweepStep() {
	waits := e.sweepWaits(1)
	e.clock.Advance(waits[len(waits)-1])
	e.sweepWaits(len(waits) + 1)
}

// sweepLast moves the clock by the sweep's current wait for the sweep that ends the work, after which the sweep rearms nothing.
func (e *env) sweepLast() {
	waits := e.sweepWaits(1)
	e.clock.Advance(waits[len(waits)-1])
}

// awaitOutcome waits for the answer to a registered wait; the real-time deadline only bounds a failing test.
func awaitOutcome(t *testing.T, waiter *creelengine.Waiter) creelengine.Outcome {
	t.Helper()
	select {
	case outcome := <-waiter.C:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("the wait was not answered")
		return creelengine.Outcome{}
	}
}

// driveProbeWaits ticks by each wait in turn until count waits are recorded, and returns them.
func (e *env) driveProbeWaits(count int) []time.Duration {
	waits := e.probeWaits(1)
	for len(waits) < count {
		e.tick(waits[len(waits)-1])
		waits = e.probeWaits(len(waits) + 1)
	}
	return waits[:count]
}

// typed returns the texts sent to the strand, in order.
func (e *env) typed(strandName string) []string {
	var texts []string
	for _, sent := range e.del.Sends() {
		if sent.Strand == strandName {
			texts = append(texts, sent.Text)
		}
	}
	return texts
}

// stalledReasons returns the reason of every stalled record to receiver, in order.
func stalledReasons(e *env, receiver string) []string {
	var reasons []string
	for _, entry := range e.store.Log(creelengine.LogFilter{To: receiver}) {
		if entry.Event == "stalled" {
			reasons = append(reasons, entry.Reason)
		}
	}
	return reasons
}

// holdAfterAReminderProbeReadTheDraft leaves a draft standing in alpha's pane, read once by the probe of an unread message's reminder, and then holds alpha for batten.
// From then on the probe skips the pane read, since only the hold owner's mail and injects are delivered.
func holdAfterAReminderProbeReadTheDraft(t *testing.T, e *env) {
	t.Helper()
	e.del.SetStall(alpha, creelengine.StallDraft)
	e.post(t, strand(beta), alpha, "hi")
	e.start(t)
	e.probeWaits(1)
	if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
		t.Fatalf("Hold: %v", err)
	}
}

// tickPastTheStallFreshness moves the clock 24 seconds in probe steps and checks that no probe read the pane meanwhile.
func (e *env) tickPastTheStallFreshness(t *testing.T) {
	t.Helper()
	idleCalls := e.del.IdleCalls(alpha)
	for range 3 {
		e.tick(8 * time.Second)
	}
	if got := e.del.IdleCalls(alpha); got != idleCalls {
		t.Fatalf("Idle was asked %d more times under the hold; want the probe to skip the pane read", got-idleCalls)
	}
}

func reminderFor(count int, listed ...string) string {
	return fmt.Sprintf("[creel] %d unread: %s — run lyx creel read", count, strings.Join(listed, ", "))
}

func seconds(n ...int) []time.Duration {
	waits := make([]time.Duration, len(n))
	for i, v := range n {
		waits[i] = time.Duration(v) * time.Second
	}
	return waits
}

func TestScheduler_Reminders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"PostToALiveIdleStrandIsRemindedOnce", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, strand(alpha), beta, "hello")
			e.probeWaits(1)
			e.tick(time.Second)
			want := []string{reminderFor(1, "#1 from "+alpha+" «hello»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ReplyToAStrandWithNoWorkerIsRemindedOnce", func(t *testing.T, e *env) {
			first := e.post(t, strand(beta), alpha, "question")
			e.readBodies(t, strand(alpha), "", false)
			e.start(t)
			if _, err := e.store.Reply(strand(alpha), first, "answer"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			e.probeWaits(1)
			want := []string{reminderFor(1, "#2 from "+alpha+" «answer»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"SchedulerStartedOverUnreadMailReminds", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "waiting")
			e.start(t)
			e.probeWaits(1)
			want := []string{reminderFor(1, "#1 from "+alpha+" «waiting»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ProbeIntervalOfABusyReceiverBacksOffAndResets", func(t *testing.T, e *env) {
			e.del.SetIdle(beta, false)
			e.start(t)
			e.post(t, strand(alpha), beta, "one")
			if got := e.driveProbeWaits(5); !slices.Equal(got, seconds(1, 2, 4, 8, 8)) {
				t.Fatalf("probe waits = %v; want 1, 2, 4, 8, 8 seconds", got)
			}
			e.post(t, strand(alpha), beta, "two")
			if got := e.probeWaits(6)[5]; got != time.Second {
				t.Errorf("wait after a new post = %v; want 1s", got)
			}
			e.tick(time.Second)
			e.tick(2 * time.Second)
			e.del.SetIdle(beta, true)
			e.tick(4 * time.Second)
			waits := e.probeWaits(9)
			if waits[6] != 2*time.Second || waits[7] != 4*time.Second || waits[8] != time.Second {
				t.Errorf("waits after the post = %v; want 2s, 4s, then 1s once a wake delivered", waits[6:])
			}
			if got := e.typed(beta); len(got) != 1 {
				t.Errorf("typed %q; want one reminder when the receiver turned idle", got)
			}
		}},
		{"ProbeIntervalOfADormantReceiverBacksOff", func(t *testing.T, e *env) {
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.DormantStrand(beta))
			e.start(t)
			e.post(t, strand(alpha), beta, "kept")
			if got := e.driveProbeWaits(5); !slices.Equal(got, seconds(1, 2, 4, 8, 8)) {
				t.Fatalf("probe waits = %v; want 1, 2, 4, 8, 8 seconds", got)
			}
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q into a dormant strand", got)
			}
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta))
			e.tick(8 * time.Second)
			want := []string{reminderFor(1, "#1 from "+alpha+" «kept»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q once the strand was live; want %q", got, want)
			}
		}},
		{"ProbeIntervalInsideTheReminderBackoffBacksOff", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, strand(alpha), beta, "unanswered")
			if got := e.driveProbeWaits(5); !slices.Equal(got, seconds(1, 2, 4, 8, 8)) {
				t.Fatalf("probe waits = %v; want 1, 2, 4, 8, 8 seconds", got)
			}
			if got := e.typed(beta); len(got) != 1 {
				t.Errorf("typed %q; want only the first reminder inside the backoff", got)
			}
		}},
		{"ReceiverWithNothingPendingIsNeverProbed", func(t *testing.T, e *env) {
			e.start(t)
			if _, err := e.store.Hold(program(batten), alpha, time.Minute); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			if _, err := e.store.Release(program(batten), alpha); err != nil {
				t.Fatalf("Release: %v", err)
			}
			e.clock.Advance(time.Hour)
			if calls := e.del.IdleCalls(alpha); calls != 0 {
				t.Errorf("Idle asked %d times about a receiver with nothing pending", calls)
			}
		}},
		{"BusyReceiverIsRemindedAtItsNextIdleEdge", func(t *testing.T, e *env) {
			e.del.SetIdle(beta, false)
			e.start(t)
			e.post(t, strand(alpha), beta, "later")
			e.probeWaits(1)
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q into a busy strand", got)
			}
			e.del.SetIdle(beta, true)
			e.tick(time.Second)
			if got := e.typed(beta); len(got) != 1 {
				t.Errorf("typed %q at the idle edge; want one reminder", got)
			}
		}},
		{"UnansweredReminderBacksOffToTheCeilingAndANewMessageResets", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, strand(alpha), beta, "unanswered")
			e.probeWaits(1)
			sent := 1
			for _, backoff := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, time.Hour, time.Hour} {
				e.tick(backoff - time.Minute)
				if got := len(e.typed(beta)); got != sent {
					t.Fatalf("a reminder went out %v short of the %v backoff: %d sent; want %d", time.Minute, backoff, got, sent)
				}
				e.tick(time.Minute)
				sent++
				if got := len(e.typed(beta)); got != sent {
					t.Fatalf("no reminder after the %v backoff: %d sent; want %d", backoff, got, sent)
				}
			}
			before := len(e.probeWaits(0))
			e.post(t, strand(alpha), beta, "fresh")
			e.probeWaits(before + 1)
			if got := len(e.typed(beta)); got != sent+1 {
				t.Errorf("a new message was not reminded at once: %d sent; want %d", got, sent+1)
			}
			var steps []int
			for _, entry := range e.store.Log(creelengine.LogFilter{To: beta}) {
				if entry.Event != "reminded" {
					continue
				}
				if len(entry.Unread) == 0 {
					t.Errorf("reminded entry %+v lists no unread ids", entry)
				}
				steps = append(steps, entry.Step)
			}
			if want := []int{0, 1, 2, 3, 4, 5, 6, 0}; !slices.Equal(steps, want) {
				t.Errorf("reminded steps = %v; want %v", steps, want)
			}
		}},
		{"TypedReminderClearsTheStall", func(t *testing.T, e *env) {
			e.del.SetStall(beta, creelengine.StallDraft)
			e.post(t, strand(alpha), beta, "hi")
			e.start(t)
			e.probeWaits(1)
			e.del.SetStall(beta, "")
			e.tick(time.Second)
			if got := len(e.typed(beta)); got != 1 {
				t.Fatalf("%d reminders typed once the draft was gone; want 1", got)
			}
			e.del.SetStall(beta, creelengine.StallDraft)
			before := len(e.probeWaits(0))
			e.post(t, strand(alpha), beta, "fresh")
			e.probeWaits(before + 1)
			if got := stalledReasons(e, beta); !slices.Equal(got, []string{"draft", "draft"}) {
				t.Errorf("stalled reasons = %q; want one before the reminder and one after it", got)
			}
		}},
		{"ReminderListsFiveMessagesAndCountsTheRest", func(t *testing.T, e *env) {
			for i := 1; i <= 7; i++ {
				e.post(t, strand(alpha), beta, fmt.Sprintf("m%d", i))
			}
			e.start(t)
			e.probeWaits(1)
			var listed []string
			for i := 1; i <= 5; i++ {
				listed = append(listed, fmt.Sprintf("#%d from %s «m%d»", i, alpha, i))
			}
			want := []string{reminderFor(7, append(listed, "+2 more")...)}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ReminderSubjectIsCutAndItsDelimitersReplaced", func(t *testing.T, e *env) {
			subject := "a\tb»c" + strings.Repeat("y", 100)
			e.post(t, strand(alpha), beta, subject+"\nsecond line")
			e.start(t)
			e.probeWaits(1)
			want := []string{reminderFor(1, "#1 from "+alpha+" «a b c"+strings.Repeat("y", 75)+"»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ProgramMailboxIsNeverReminded", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, program(batten), alpha, "hello")
			e.probeWaits(1)
			e.post(t, strand(alpha), batten, "reply")
			e.clock.Advance(time.Hour)
			if got := e.typed(batten); len(got) != 0 {
				t.Errorf("typed %q to a program", got)
			}
			if calls := e.del.IdleCalls(batten); calls != 0 {
				t.Errorf("Idle asked %d times about a program", calls)
			}
		}},
		{"OrchStrandsAreNeverTypedIntoAndTheirMailIsWithheld", func(t *testing.T, e *env) {
			e.start(t)
			for _, name := range []string{orch, orch2} {
				e.post(t, strand(alpha), name, "one")
				e.post(t, strand(beta), name, "two")
				e.clock.Advance(time.Hour)
				withheld := 0
				for _, entry := range e.store.Log(creelengine.LogFilter{To: name}) {
					if entry.Event == "withheld" && entry.Notice == "creel-notices" {
						withheld++
					}
				}
				if withheld != 2 {
					t.Errorf("%s: %d withheld lines; want one per new message", name, withheld)
				}
				if calls := e.del.IdleCalls(name); calls != 0 || len(e.typed(name)) != 0 {
					t.Errorf("%s: %d Idle calls and %q typed; want none", name, calls, e.typed(name))
				}
			}
		}},
		{"PauseTypesNothingUntilLifted", func(t *testing.T, e *env) {
			e.del.SetPaused(beta, true)
			e.start(t)
			e.post(t, strand(alpha), beta, "held back")
			e.probeWaits(1)
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q under a pause", got)
			}
			if inbox, err := e.store.Inbox(strand(beta)); err != nil || len(inbox.Senders) != 1 {
				t.Fatalf("a post under a pause was not stored: inbox %+v, %v", inbox, err)
			}
			e.del.SetPaused(beta, false)
			e.tick(time.Second)
			if got := e.typed(beta); len(got) != 1 {
				t.Errorf("typed %q after the pause lifted; want one reminder", got)
			}
		}},
		{"HoldMutesOtherSendersLetsTheOwnerPassAndEndsAtExpiry", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), beta, 30*time.Minute); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.start(t)
			e.post(t, strand(alpha), beta, "muted")
			e.probeWaits(1)
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q under another sender's hold", got)
			}
			before := len(e.probeWaits(0))
			e.post(t, program(batten), beta, "from the owner")
			e.probeWaits(before + 1)
			want := []string{reminderFor(1, "#2 from "+batten+" «from the owner»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Fatalf("typed %q; want only the owner's mail reminded", got)
			}
			e.tick(31 * time.Minute)
			want = append(want, reminderFor(2, "#1 from "+alpha+" «muted»", "#2 from "+batten+" «from the owner»"))
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q after the hold ended; want %q", got, want)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newEnv(t))
		})
	}
}

// TestScheduler_Injects captures the process-global logger, so neither it nor its rows run in parallel.
func TestScheduler_Injects(t *testing.T) {
	inject := func(t *testing.T, e *env, sender creelengine.Sender, to, text string, ttl time.Duration, interrupt bool) int64 {
		t.Helper()
		result, err := e.store.Inject(sender, to, text, ttl, interrupt)
		if err != nil {
			t.Fatalf("Inject(%s): %v", text, err)
		}
		return result.ID
	}
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"InjectsAreTypedBeforeRemindersInQueueOrder", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "hi")
			inject(t, e, program(batten), beta, "/one", 0, false)
			inject(t, e, program(batten), beta, "/two", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.tick(time.Second)
			want := []string{"/one", "/two", reminderFor(1, "#1 from "+alpha+" «hi»")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"BusySendIsRetriedAtTheNextProbe", func(t *testing.T, e *env) {
			e.del.SetSendError(creelengine.ErrBusy)
			id := inject(t, e, program(batten), alpha, "/clear", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.del.SetSendError(nil)
			e.tick(time.Second)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/clear", "/clear"}) {
				t.Errorf("typed %q; want the busy attempt and the retry", got)
			}
			if queued := e.store.Queued(alpha); len(queued) != 0 {
				t.Errorf("Queued = %+v after the retry landed", queued)
			}
			if events := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); events[len(events)-1] != "injected" {
				t.Errorf("events = %v; want the inject to end injected", events)
			}
		}},
		{"UnconfirmedSendIsJournaledAndNeverRetyped", func(t *testing.T, e *env) {
			e.del.SetSendError(creelengine.ErrNotLanded)
			result, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.start(t)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnconfirmed {
				t.Errorf("wait answered %v; want unconfirmed", outcome.Kind)
			}
			e.clock.Advance(time.Hour)
			if got := e.typed(alpha); len(got) != 1 {
				t.Errorf("typed %q; an unconfirmed inject must never be retyped", got)
			}
			if queued := e.store.Queued(alpha); len(queued) != 0 {
				t.Errorf("Queued = %+v; an unconfirmed inject leaves the queue", queued)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: result.ID})); !slices.Equal(got, []string{"posted", "sending", "unconfirmed"}) {
				t.Errorf("events = %v; want posted, sending, unconfirmed", got)
			}
		}},
		{"TTLIsFrozenWhilePausedAndWhileDormant", func(t *testing.T, e *env) {
			e.del.SetPaused(alpha, true)
			inject(t, e, program(batten), alpha, "/paused", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Hour)
			e.del.SetPaused(alpha, false)
			e.del.SetStrands(creelfake.DormantStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.tick(time.Hour)
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.tick(time.Hour)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/paused"}) {
				t.Errorf("typed %q; want the inject typed once live and unpaused, after hours of freeze", got)
			}
		}},
		{"TTLRunsUnderAnotherSendersHoldAndExpiresUntyped", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			muted, waiter, err := e.store.InjectWait(external(), alpha, "/muted", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			inject(t, e, program(batten), alpha, "/mine", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(5 * time.Minute)
			if queued := e.store.Queued(alpha); len(queued) != 1 {
				t.Fatalf("Queued = %+v after 5 of 10 minutes; want the muted inject still waiting", queued)
			}
			e.clock.Advance(6 * time.Minute)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeExpired {
				t.Errorf("wait answered %v; want expired", outcome.Kind)
			}
			if got := e.typed(alpha); !slices.Equal(got, []string{"/mine"}) {
				t.Errorf("typed %q; want only the hold owner's inject", got)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: muted.ID})); !slices.Contains(got, "expired") {
				t.Errorf("events = %v; want the muted inject expired", got)
			}
		}},
		{"InjectPostedMidWaitIsChargedFromItsPostingNotFromThePreviousProbe", func(t *testing.T, e *env) {
			e.del.SetIdle(alpha, false)
			inject(t, e, program(batten), alpha, "/keepalive", time.Hour, false)
			e.start(t)
			e.driveProbeWaits(4)
			e.clock.Advance(7 * time.Second)
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/short", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.tick(time.Second)
			e.tick(58 * time.Second)
			if queued := e.store.Queued(alpha); len(queued) != 2 {
				t.Fatalf("Queued = %+v 59 s after the posting; want both injects still waiting", queued)
			}
			e.tick(8 * time.Second)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeExpired {
				t.Errorf("wait answered %v; want expired at the first probe past 60 s since the posting", outcome.Kind)
			}
		}},
		{"StopChargesAnInjectPostedDuringASendOnlyFromItsPosting", func(t *testing.T, e *env) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			e.del.SetSendFunc(func(string, string) error {
				once.Do(func() { close(entered) })
				<-release
				return nil
			})
			scheduler := creelengine.NewScheduler(e.store, e.del, e.clock)
			scheduler.Start()
			t.Cleanup(func() {
				once.Do(func() { close(entered) })
				select {
				case <-release:
				default:
					close(release)
				}
				scheduler.Stop()
			})
			inject(t, e, program(batten), alpha, "/first", time.Hour, false)
			<-entered
			e.clock.Advance(30 * time.Second)
			second := inject(t, e, program(batten), alpha, "/second", time.Minute, false)
			e.clock.Advance(5 * time.Second)
			// A worker that probes once more before it sees the stop finds the receiver busy, so the second inject is never typed.
			e.del.SetIdle(alpha, false)
			stopped := make(chan struct{})
			go func() {
				scheduler.Stop()
				close(stopped)
			}()
			close(release)
			<-stopped
			want := fmt.Sprintf(`"id":%d,"consumed_s":5`, second)
			if text := journalText(t, e.geom); !strings.Contains(text, want) {
				t.Errorf("journal lacks %s; want the second inject's aged record counting 5 s since its posting:\n%s", want, text)
			}
		}},
		{"ChangingStallIsJournaledOncePerChangeAndStopsOnceTyped", func(t *testing.T, e *env) {
			e.del.SetStall(alpha, creelengine.StallDraft)
			inject(t, e, program(batten), alpha, "/clear", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.del.SetStall(alpha, creelengine.StallTooShort)
			e.tick(2 * time.Second)
			e.tick(4 * time.Second)
			e.del.SetStall(alpha, "")
			e.tick(8 * time.Second)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/clear"}) {
				t.Fatalf("typed %q; want the inject typed once the stall cleared", got)
			}
			if got := stalledReasons(e, alpha); !slices.Equal(got, []string{"draft", "too-short"}) {
				t.Errorf("stalled reasons = %q; want draft then too-short, repeats and the typed inject adding none", got)
			}
		}},
		{"ReadingWithNoReasonAndAnErrorEachClearTheStall", func(t *testing.T, e *env) {
			e.del.SetStall(alpha, creelengine.StallDraft)
			inject(t, e, program(batten), alpha, "/clear", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.del.SetStall(alpha, "")
			e.del.SetIdle(alpha, false)
			e.tick(time.Second)
			e.del.SetStall(alpha, creelengine.StallDraft)
			e.tick(2 * time.Second)
			e.del.SetStall(alpha, "")
			e.del.SetIdleError(errors.New("pane capture failed"))
			e.tick(4 * time.Second)
			e.del.SetIdleError(nil)
			e.del.SetStall(alpha, creelengine.StallDraft)
			e.tick(8 * time.Second)
			if got := stalledReasons(e, alpha); !slices.Equal(got, []string{"draft", "draft", "draft"}) {
				t.Errorf("stalled reasons = %q; want three, one per episode", got)
			}
		}},
		{"InjectStalledForItsWholeTTLStillExpiresWithOneStalledRecord", func(t *testing.T, e *env) {
			e.del.SetStall(alpha, creelengine.StallDraft)
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.start(t)
			e.probeWaits(1)
			e.clock.Advance(2 * time.Minute)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeExpired {
				t.Errorf("wait answered %v; want expired at the TTL", outcome.Kind)
			}
			if got := stalledReasons(e, alpha); !slices.Equal(got, []string{"draft"}) {
				t.Errorf("stalled reasons = %q; want one", got)
			}
		}},
		{"WaiterProgressKeepsOnlyTheNewestUnreadStall", func(t *testing.T, e *env) {
			e.del.SetStall(alpha, creelengine.StallDraft)
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.del.SetStall(alpha, creelengine.StallTooShort)
			e.tick(2 * time.Second)
			select {
			case note := <-waiter.Progress:
				if note.Stall != creelengine.StallTooShort || note.At.IsZero() {
					t.Errorf("progress = %+v; want the newer too-short note with its time", note)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the waiter heard no stall")
			}
			select {
			case note := <-waiter.Progress:
				t.Errorf("a second progress note %+v; the older unread one must have been replaced", note)
			default:
			}
		}},
		{"StallClearedUnderAHoldIsNotSeededIntoALaterWaiter", func(t *testing.T, e *env) {
			holdAfterAReminderProbeReadTheDraft(t, e)
			e.tickPastTheStallFreshness(t)
			e.del.SetStall(alpha, "")
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			select {
			case note := <-waiter.Progress:
				t.Errorf("seeded with %+v; want no note for a stall no probe read within 16 s", note)
			default:
			}
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeInjected {
				t.Errorf("wait answered %v; want injected", outcome.Kind)
			}
			select {
			case note := <-waiter.Progress:
				t.Errorf("heard %+v; want no stall line once the draft is gone", note)
			default:
			}
		}},
		{"PersistingStallIsToldOnceToALaterWaiterAndEachChangeOnce", func(t *testing.T, e *env) {
			holdAfterAReminderProbeReadTheDraft(t, e)
			e.tickPastTheStallFreshness(t)
			before := len(e.probeWaits(0))
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			select {
			case note := <-waiter.Progress:
				t.Fatalf("seeded with %+v; want no note for a stall no probe read within 16 s", note)
			default:
			}
			e.probeWaits(before + 1)
			// told drains what the last probe told the waiter: nothing for an empty want, else exactly that stall as it began at the probe.
			told := func(want creelengine.Stall) {
				t.Helper()
				select {
				case note := <-waiter.Progress:
					if want == "" {
						t.Errorf("told %+v; want nothing", note)
					} else if note.Stall != want || !note.At.Equal(e.clock.Now()) {
						t.Errorf("told %+v; want %s from the probe that read it, at %v", note, want, e.clock.Now())
					}
				default:
					if want != "" {
						t.Errorf("told nothing; want %s", want)
					}
				}
			}
			told(creelengine.StallDraft)
			e.tick(time.Second)
			told("")
			for _, change := range []creelengine.Stall{creelengine.StallTooShort, creelengine.StallDraft} {
				e.del.SetStall(alpha, change)
				e.tick(8 * time.Second)
				told(change)
			}
			e.del.SetStall(alpha, "")
			e.del.SetIdle(alpha, false)
			e.tick(8 * time.Second)
			told("")
			e.del.SetStall(alpha, creelengine.StallDraft)
			e.tick(8 * time.Second)
			told(creelengine.StallDraft)
		}},
		{"ExpiryNamesNoStallLastReadBeforeTheBound", func(t *testing.T, e *env) {
			holdAfterAReminderProbeReadTheDraft(t, e)
			muted, waiter, err := e.store.InjectWait(external(), alpha, "/muted", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.tickPastTheStallFreshness(t)
			e.clock.Advance(time.Minute)
			outcome := awaitOutcome(t, waiter)
			if outcome.Kind != creelengine.OutcomeExpired || outcome.Stall != "" || !outcome.StallAt.IsZero() {
				t.Errorf("wait answered %+v; want expired with no stall", outcome)
			}
			for _, entry := range e.store.Log(creelengine.LogFilter{ID: muted.ID}) {
				if entry.Event == "expired" && entry.Reason != "" {
					t.Errorf("expired entry carries reason %q; want none", entry.Reason)
				}
			}
		}},
		{"RunningInterruptHoldsBackEveryDeliveryToTheReceiver", func(t *testing.T, e *env) {
			e.del.SetIdle(alpha, false)
			inject(t, e, program(batten), alpha, "/queued", 0, false)
			e.post(t, strand(beta), alpha, "hi")
			e.start(t)
			e.probeWaits(1)
			entered, release := make(chan struct{}), make(chan struct{})
			e.del.SetInterruptFunc(func(string) error {
				close(entered)
				<-release
				return nil
			})
			interrupted := make(chan error, 1)
			go func() {
				_, err := e.store.Interrupt(program(batten), alpha)
				interrupted <- err
			}()
			<-entered
			idleBefore := e.del.IdleCalls(alpha)
			e.del.SetIdle(alpha, true)
			for range 3 {
				e.clock.Advance(10 * time.Second)
			}
			if got := e.del.IdleCalls(alpha); got != idleBefore {
				t.Errorf("Idle was asked %d more times while the interrupt ran; want none", got-idleBefore)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Errorf("typed %q while the interrupt ran", got)
			}
			close(release)
			if err := <-interrupted; err != nil {
				t.Fatalf("Interrupt: %v", err)
			}
			e.probeWaits(2)
			if got := e.typed(alpha); len(got) == 0 || got[0] != "/queued" {
				t.Errorf("typed %q; want the queued inject typed first once the interrupt ended", got)
			}
		}},
		{"EndOfAnInterruptWakesABackedOffWorker", func(t *testing.T, e *env) {
			e.del.SetIdle(alpha, false)
			inject(t, e, program(batten), alpha, "/queued", 0, false)
			e.start(t)
			e.driveProbeWaits(4)
			e.del.SetIdle(alpha, true)
			before := len(e.probeWaits(0))
			if sent, err := e.store.Interrupt(program(batten), alpha); err != nil || !sent {
				t.Fatalf("Interrupt = %v, %v; want sent", sent, err)
			}
			e.probeWaits(before + 1)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/queued"}) {
				t.Errorf("typed %q; want the inject typed at the interrupt's end with no clock move", got)
			}
		}},
		{"InterruptInjectInterruptsThenTypesItsTextFirstAtTheNextIdle", func(t *testing.T, e *env) {
			e.del.SetIdle(alpha, false)
			inject(t, e, program(batten), alpha, "/first", 0, false)
			inject(t, e, program(batten), alpha, "/urgent", 0, true)
			if got := e.del.Interrupts(); !slices.Equal(got, []string{alpha}) {
				t.Fatalf("interrupts = %v; want one at the time of the inject", got)
			}
			e.start(t)
			e.probeWaits(1)
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q into a busy strand", got)
			}
			e.del.SetIdle(alpha, true)
			e.tick(time.Second)
			e.tick(time.Second)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/urgent", "/first"}) {
				t.Errorf("typed %q; want the interrupting inject first", got)
			}
		}},
		{"ReceiverWhoseSendsStayBusyDelaysNoOther", func(t *testing.T, e *env) {
			release := make(chan struct{})
			var once sync.Once
			e.del.SetSendFunc(func(strandName, text string) error {
				if strandName == alpha {
					<-release
				}
				return nil
			})
			inject(t, e, program(batten), alpha, "/slow", 0, false)
			inject(t, e, program(batten), beta, "/fast", 0, false)
			e.start(t)
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			e.probeWaits(1)
			if got := e.typed(beta); !slices.Equal(got, []string{"/fast"}) {
				t.Errorf("typed %q to beta while alpha's send was stuck", got)
			}
			once.Do(func() { close(release) })
			e.probeWaits(2)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/slow"}) {
				t.Errorf("typed %q to alpha; want its inject once the send returned", got)
			}
		}},
		{"RepeatedSendErrorLogsOnceUntilItChanges", func(t *testing.T, e *env) {
			logs := logcapture.Capture(t)
			e.del.SetSendError(errors.New("pane exploded"))
			inject(t, e, program(batten), alpha, "/clear", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.tick(2 * time.Second)
			e.del.SetSendError(errors.New("pane vanished"))
			e.tick(4 * time.Second)
			text := logs.String()
			if got := strings.Count(text, "pane exploded"); got != 1 {
				t.Errorf("%d warnings for the repeated error; want one", got)
			}
			if got := strings.Count(text, "pane vanished"); got != 1 {
				t.Errorf("%d warnings for the changed error; want one", got)
			}
			if got := e.typed(alpha); len(got) != 4 {
				t.Errorf("typed %q; want the inject retried at every probe", got)
			}
		}},
		{"PausedErrorTypesNothingAndLogsOneWarning", func(t *testing.T, e *env) {
			logs := logcapture.Capture(t)
			e.del.SetPausedError(errors.New("pause record is corrupt"))
			inject(t, e, program(batten), beta, "/clear", 0, false)
			e.post(t, strand(alpha), beta, "hi")
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			if got := e.del.Sends(); len(got) != 0 {
				t.Errorf("sent %v while the pause record was unreadable", got)
			}
			if got := strings.Count(logs.String(), "pause record is corrupt"); got != 1 {
				t.Errorf("%d warnings; want one", got)
			}
		}},
		{"IdleErrorTypesNothingAndLogsOneWarning", func(t *testing.T, e *env) {
			logs := logcapture.Capture(t)
			e.del.SetIdleError(errors.New("pane capture failed"))
			inject(t, e, program(batten), beta, "/clear", 0, false)
			e.post(t, strand(alpha), beta, "hi")
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			if got := e.del.Sends(); len(got) != 0 {
				t.Errorf("sent %v while the idle check failed", got)
			}
			if got := strings.Count(logs.String(), "pane capture failed"); got != 1 {
				t.Errorf("%d warnings; want one", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newEnv(t))
		})
	}
}

func TestSweep_RemovedHolders(t *testing.T) {
	t.Parallel()
	without := func(e *env, removed ...string) {
		var kept []creelengine.Strand
		for _, name := range []string{alpha, beta, orch, orch2} {
			if !slices.Contains(removed, name) {
				kept = append(kept, creelfake.LiveStrand(name))
			}
		}
		e.del.SetStrands(kept...)
	}
	// holder reads the message as its holder without consuming it, so a test can see where it ended up.
	holderOf := func(t *testing.T, e *env, as string) []creelengine.Message {
		t.Helper()
		reading, err := e.store.Read(strand(as), "", true)
		if err != nil {
			t.Fatalf("Read(%s): %v", as, err)
		}
		reading.Release()
		return reading.Messages
	}
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"WaitsGrowToTheCeilingWhileTheStrandSetHoldsAndResetOnAPost", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, strand(alpha), orch, "one")
			waits := e.sweepWaits(1)
			for len(waits) < 3 {
				e.clock.Advance(waits[len(waits)-1])
				waits = e.sweepWaits(len(waits) + 1)
			}
			if !slices.Equal(waits, seconds(30, 50, 50)) {
				t.Fatalf("sweep waits = %v; want 30s doubling to a 50-second ceiling", waits)
			}
			e.post(t, strand(beta), orch, "two")
			if got := e.sweepWaits(4)[3]; got != 30*time.Second {
				t.Errorf("sweep wait after a post with the full ceiling left = %v; want 30s", got)
			}
		}},
		{"PostWithLessThanTheBaseLeftDoesNotDelayTheSweep", func(t *testing.T, e *env) {
			// Only orch-role mail exists, which has no delivery worker, so every wait on the clock is the sweep's.
			e.start(t)
			e.post(t, strand(alpha), orch, "one")
			e.clock.AwaitWaits(1)
			e.clock.Advance(30 * time.Second)
			e.clock.AwaitWaits(2)
			e.clock.Advance(45 * time.Second)
			e.post(t, strand(beta), orch, "two")
			e.clock.Advance(5 * time.Second)
			if got := e.clock.AwaitWaits(3)[2]; got != 50*time.Second {
				t.Errorf("wait after the sweep that the post left at 5s = %v; want the 50s ceiling", got)
			}
		}},
		{"RemovedHolderIsSettledWithin60sAtTheCeiling", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), orch, "orch mail")
			e.post(t, strand(alpha), beta, "beta mail")
			e.start(t)
			e.sweepStep()
			e.sweepStep()
			if waits := e.sweepWaits(3); !slices.Equal(waits, seconds(30, 50, 50)) {
				t.Fatalf("sweep waits = %v; want the ceiling reached at 30, 50, 50", waits)
			}
			without(e, orch, beta)
			e.sweepStep()
			if waits := e.sweepWaits(4); waits[3] != 10*time.Second {
				t.Fatalf("wait after the first miss = %v; want the 10s confirm", waits[3])
			}
			e.sweepLast()
			waits := e.sweepWaits(6)
			if waits[4] != 50*time.Second || waits[5] != 30*time.Second {
				t.Errorf("waits after the release = %v; want 50s armed by the releasing sweep and 30s re-armed by the returns' wake", waits[4:])
			}
			var endedBy []string
			for _, msg := range holderOf(t, e, alpha) {
				endedBy = append(endedBy, msg.EndedUnreadBy)
			}
			slices.Sort(endedBy)
			if want := []string{beta, orch}; !slices.Equal(endedBy, want) {
				t.Errorf("messages back with %s are marked ended unread by %v; want %v", alpha, endedBy, want)
			}
		}},
		{"PostInsideTheConfirmIntervalKeepsTheConfirmDeadline", func(t *testing.T, e *env) {
			// The removed holder is orch, whose mail has no worker, so the row reads every wait on the clock.
			_, first, err := e.store.PostWait(external(), orch, "one")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			without(e, orch)
			e.start(t)
			e.clock.AwaitWaits(1)
			e.clock.Advance(30 * time.Second)
			if waits := e.clock.AwaitWaits(2); waits[1] != 10*time.Second {
				t.Fatalf("wait after the first miss = %v; want the 10s confirm", waits[1])
			}
			e.clock.Advance(5 * time.Second)
			e.post(t, strand(alpha), orch2, "two")
			if waits := e.clock.AwaitWaits(3); waits[2] != 5*time.Second {
				t.Fatalf("wait after a post inside the confirm = %v; want the 5s left to the confirm deadline", waits[2])
			}
			e.clock.Advance(5 * time.Second)
			if outcome := awaitOutcome(t, first); outcome.Kind != creelengine.OutcomeReturned {
				t.Errorf("wait answered %+v; want the message returned undelivered at the confirm deadline", outcome)
			}
		}},
		{"MailReturnsOneStepAtATimeThenIsUndeliveredAndAnswersItsWaiter", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(external(), orch2, "chain")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			if err := e.store.Forward(strand(orch2), id, alpha); err != nil {
				t.Fatalf("Forward: %v", err)
			}
			without(e, alpha)
			e.start(t)
			e.sweepStep()
			e.sweepStep()
			held := holderOf(t, e, orch2)
			if len(held) != 1 || held[0].Holder != orch2 || held[0].EndedUnreadBy != alpha || len(held[0].Path) != 0 {
				t.Fatalf("after two sweeps the message is %+v; want it back with %s, marked as ended unread by %s", held, orch2, alpha)
			}
			want := []string{"posted", "withheld", "forwarded", "returned", "withheld"}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, want) {
				t.Errorf("events = %v; want %v", got, want)
			}
			if calls, typed := e.del.IdleCalls(orch2), e.typed(orch2); calls != 0 || len(typed) != 0 {
				t.Errorf("orch-2: %d Idle calls and %q typed; want none", calls, typed)
			}
			without(e, alpha, orch2)
			e.sweepStep()
			e.sweepLast()
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeReturned || outcome.Message.ID != id {
				t.Errorf("wait answered %+v; want the message returned undelivered", outcome)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); got[len(got)-1] != "undelivered" {
				t.Errorf("events = %v; want the message to end undelivered", got)
			}
		}},
		{"StrandRemovedAndReAddedKeepsItsMail", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "kept")
			e.post(t, strand(alpha), orch, "orch kept")
			without(e, beta)
			e.start(t)
			// The confirm sweep 10 seconds after the miss finds beta back, and ends the confirm at the 30-second base since the strand-name set changed.
			e.sweepStep()
			without(e)
			e.sweepStep()
			if waits := e.sweepWaits(3); !slices.Equal(waits, seconds(30, 10, 30)) {
				t.Fatalf("sweep waits = %v; want the 10s confirm after the miss and the 30s base after it", waits)
			}
			if held := holderOf(t, e, beta); len(held) != 1 || held[0].Holder != beta || held[0].EndedUnreadBy != "" {
				t.Errorf("the message is %+v; want it still with %s", held, beta)
			}
			// Beta is removed again for a first miss; at the confirm sweep beta is back and orch is gone, which is orch's first miss and a new confirm.
			without(e, beta)
			e.sweepStep()
			without(e, orch)
			e.sweepStep()
			if waits := e.sweepWaits(5); !slices.Equal(waits[3:], seconds(10, 10)) {
				t.Errorf("sweep waits after the second removal = %v; want a 10s confirm after each first miss", waits[3:])
			}
			if held := holderOf(t, e, orch); len(held) != 1 || held[0].Holder != orch {
				t.Errorf("orch's mail is %+v; want it still with %s after one miss", held, orch)
			}
		}},
		{"FailedListingMovesNothingAndCountsTowardNoStreak", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "kept")
			e.del.SetStrandsError(errors.New("reed state unreadable"))
			e.start(t)
			e.sweepStep()
			if waits := e.sweepWaits(2); waits[1] != 30*time.Second {
				t.Errorf("wait after a failed sweep with no miss pending = %v; want 30s", waits[1])
			}
			e.del.SetStrandsError(nil)
			without(e, beta)
			e.sweepStep()
			e.del.SetStrandsError(errors.New("reed state unreadable"))
			e.sweepStep()
			if waits := e.sweepWaits(4); waits[2] != 10*time.Second || waits[3] != 10*time.Second {
				t.Errorf("waits after the first miss and a failed sweep = %v; want 10s and 10s", waits[2:])
			}
			if held := holderOf(t, e, beta); len(held) != 1 {
				t.Fatalf("a failed sweep moved the mail: %+v", held)
			}
			e.del.SetStrandsError(nil)
			e.sweepStep()
			if held := holderOf(t, e, alpha); len(held) != 1 || held[0].EndedUnreadBy != beta {
				t.Errorf("after the second successful sweep the message is %+v; want it returned to %s", held, alpha)
			}
		}},
		{"InjectQueuedForARemovedStrandExpiresAndLeavesTheCarry", func(t *testing.T, e *env) {
			result, waiter, err := e.store.InjectWait(program(batten), beta, "/clear", 0, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			without(e, beta)
			e.start(t)
			e.sweepStep()
			e.sweepLast()
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeExpired {
				t.Errorf("wait answered %v; want expired", outcome.Kind)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: result.ID})); got[len(got)-1] != "expired" {
				t.Errorf("events = %v; want the inject to end expired", got)
			}
			if got := e.typed(beta); len(got) != 0 {
				t.Errorf("typed %q into a removed strand", got)
			}
			rebuilt, err := creelengine.OpenStore(e.geom, e.del, e.clock)
			if err != nil {
				t.Fatalf("OpenStore over the journal: %v", err)
			}
			defer func() { _ = rebuilt.Close() }()
			if queued := rebuilt.Queued(beta); len(queued) != 0 {
				t.Errorf("a rebuilt pool queues %+v; the expired inject must not come back", queued)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newEnv(t))
		})
	}
}
