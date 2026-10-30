//go:build tier0

// deliver_test.go pins delivery into panes through the fake seam: reminders, injects, the probe and sweep intervals, the removed-holder sweep and the chase clock.

package creelengine_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// isProbeWait, isSweepWait and isChaseWait tell the waits of the delivery workers (1 to 8 seconds), the sweep (up to its 50-second ceiling) and the chasers (one to eight minutes) apart on the shared fake clock.
func isProbeWait(d time.Duration) bool { return d <= 8*time.Second }

func isSweepWait(d time.Duration) bool { return d > 8*time.Second && d <= 50*time.Second }

func isChaseWait(d time.Duration) bool { return d >= time.Minute && d <= 8*time.Minute }

// start runs a scheduler over the env's store and stops it when the test ends.
func (e *env) start(t *testing.T) {
	t.Helper()
	scheduler := creelengine.NewScheduler(e.store, e.del, e.clock)
	scheduler.Start()
	e.scheduler = scheduler
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

// awaitTyped waits until the delivery has typed exactly want into the strand, for the mail whose delivery no clock wait of the test marks.
// The real-time deadline only bounds a failing test.
func awaitTyped(t *testing.T, e *env, strandName string, want ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !slices.Equal(e.typed(strandName), want) {
		if time.Now().After(deadline) {
			t.Fatalf("typed %q into %s; want %q", e.typed(strandName), strandName, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// awaitMarkedProbe waits until the receiver's worker has begun the probe after the first one that read the pause once the caller took mark, and returns the clock time it advanced and the passes that took.
// A caller takes seen, the probe-wait count, before the post that wakes the worker, and mark, the receiver's pause-read count, after that post returned.
// Each pass waits for one more probe wait than the pass before, then advances the clock by the probe ceiling,
// so a probe that comes only after a clock advance is reached without the caller advancing first.
// The real-time deadline only bounds a failing test.
func (e *env) awaitMarkedProbe(t *testing.T, receiver string, mark, seen int) (advanced time.Duration, passes int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		for e.del.PausedCalls(receiver) < mark+2 && len(e.probeWaits(0)) < seen+passes+1 {
			if time.Now().After(deadline) {
				t.Fatalf("no probe of %s read its pause after mark %d: %d pause reads, %d probe waits, %d passes", receiver, mark, e.del.PausedCalls(receiver), len(e.probeWaits(0)), passes)
			}
			time.Sleep(time.Millisecond)
		}
		if e.del.PausedCalls(receiver) >= mark+2 {
			return advanced, passes
		}
		e.clock.Advance(8 * time.Second)
		advanced += 8 * time.Second
		passes++
	}
}

// injectCommand queues a command inject from a program to receiver and fails the test when it is refused.
func injectCommand(t *testing.T, e *env, receiver string, command creelengine.Command) creelengine.InjectResult {
	t.Helper()
	result, err := e.store.InjectCommand(program(batten), receiver, command, 0)
	if err != nil {
		t.Fatalf("InjectCommand(%s): %v", command.Name, err)
	}
	return result
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

// reminderFor builds the reminder line for count unread messages listed as given; holdOwner is the owner of a hold on the receiver, empty for none.
func reminderFor(count int, holdOwner string, listed ...string) string {
	command := "lym creel read"
	if holdOwner != "" {
		command += " --from " + holdOwner
	}
	return fmt.Sprintf("[creel] %d unread: %s — run %s", count, strings.Join(listed, ", "), command)
}

// orchStrandIsRemindedAndTypedInto returns a row body that posts two messages and a text inject to the orch strand name, and expects the inject typed first and then one reminder of both messages.
func orchStrandIsRemindedAndTypedInto(name string) func(t *testing.T, e *env) {
	return func(t *testing.T, e *env) {
		e.post(t, strand(alpha), name, "one")
		e.post(t, strand(beta), name, "two")
		if _, err := e.store.Inject(program(batten), name, "/one", 0, false); err != nil {
			t.Fatalf("Inject: %v", err)
		}
		e.start(t)
		e.probeWaits(1)
		e.tick(time.Second)
		want := []string{"/one", reminderFor(2, "", "#1 from \""+alpha+"\"", "#2 from \""+beta+"\"")}
		if got := e.typed(name); !slices.Equal(got, want) {
			t.Errorf("typed %q into %s; want %q", got, name, want)
		}
	}
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
	// awaitReminded waits until the journal holds count reminded records to receiver; the real-time deadline only bounds a failing test.
	awaitReminded := func(t *testing.T, e *env, receiver string, count int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			reminded := 0
			for _, entry := range e.store.Log(creelengine.LogFilter{To: receiver}) {
				if entry.Event == "reminded" {
					reminded++
				}
			}
			if reminded >= count {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%d reminded records to %s; want %d", reminded, receiver, count)
			}
			time.Sleep(time.Millisecond)
		}
	}
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"PostToALiveIdleStrandIsRemindedOnce", func(t *testing.T, e *env) {
			e.start(t)
			e.post(t, strand(alpha), beta, "hello")
			e.probeWaits(1)
			e.tick(time.Second)
			want := []string{reminderFor(1, "", "#1 from \""+alpha+"\"")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"AHookStrandGetsNoNoticeOrReminderButItsInjectsAreTyped", func(t *testing.T, e *env) {
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.HookStrand(beta))
			e.start(t)
			e.post(t, program(batten), beta, "notice")
			e.post(t, strand(alpha), beta, "from a strand")
			e.driveProbeWaits(4)
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q into %s; want nothing for a strand marked CreelHooks", got, beta)
			}
			if _, err := e.store.Inject(program(batten), beta, "/one", 0, false); err != nil {
				t.Fatalf("Inject: %v", err)
			}
			awaitTyped(t, e, beta, "/one")
		}},
		{"ReplyToAStrandWithNoWorkerIsRemindedOnce", func(t *testing.T, e *env) {
			first := e.post(t, strand(beta), alpha, "question")
			e.readBodies(t, strand(alpha), "")
			e.start(t)
			if _, err := e.store.Reply(strand(alpha), first, "answer"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			e.probeWaits(1)
			want := []string{reminderFor(1, "", "#2 from \""+alpha+"\"")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"SchedulerStartedOverUnreadMailReminds", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "waiting")
			e.start(t)
			e.probeWaits(1)
			want := []string{reminderFor(1, "", "#1 from \""+alpha+"\"")}
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
			want := []string{reminderFor(1, "", "#1 from \""+alpha+"\"")}
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
			e.post(t, strand(alpha), beta, "fresh")
			awaitReminded(t, e, beta, sent+1)
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
				listed = append(listed, fmt.Sprintf("#%d from \"%s\"", i, alpha))
			}
			want := []string{reminderFor(7, "", append(listed, "+2 more")...)}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ReminderCarriesNoSubject", func(t *testing.T, e *env) {
			subject := "a\tb»c" + strings.Repeat("y", 100)
			e.post(t, strand(alpha), beta, subject+"\nsecond line")
			e.start(t)
			e.probeWaits(1)
			want := []string{reminderFor(1, "", "#1 from \""+alpha+"\"")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"ProgramNoticeIsTypedAsItsLineReadAndLeftOutOfTheReminder", func(t *testing.T, e *env) {
			notice := e.post(t, program(batten), beta, "task-x: landed; newly ready: task-y")
			e.post(t, strand(alpha), beta, "hello")
			if _, err := e.store.Inject(program(batten), beta, "/one", 0, false); err != nil {
				t.Fatalf("Inject: %v", err)
			}
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.tick(time.Second)
			want := []string{"/one", "[creel] #1 from \"" + batten + "\": task-x: landed; newly ready: task-y", reminderFor(1, "", "#2 from \""+alpha+"\"")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
			if got, want := eventsOf(e.store.Log(creelengine.LogFilter{ID: notice})), []string{"posted", "read"}; !slices.Equal(got, want) {
				t.Errorf("Log(#%d) events = %v; want %v", notice, got, want)
			}
			inbox, err := e.store.Inbox(strand(beta))
			if wantInbox := []creelengine.InboxEntry{{From: alpha, Unread: 1}}; err != nil || !reflect.DeepEqual(inbox.Senders, wantInbox) {
				t.Errorf("Inbox = %+v, %v; want senders %v", inbox, err, wantInbox)
			}
		}},
		{"MultiLineAndExternalMailIsCarriedAndOverCapAndStrandMailIsReminded", func(t *testing.T, e *env) {
			e.post(t, program(batten), beta, "first line\n\tsecond line\n")
			e.post(t, program(batten), beta, strings.Repeat("x", 4097))
			e.post(t, external(), beta, "from a terminal")
			e.post(t, strand(alpha), beta, "from a strand")
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.tick(time.Second)
			want := []string{
				"[creel] #1 from \"" + batten + "\": first line second line",
				"[creel] #3 from \"" + external().Name + "\": from a terminal",
				reminderFor(2, "", "#2 from \""+batten+"\"", "#4 from \""+alpha+"\""),
			}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want %q", got, want)
			}
		}},
		{"NoticeAReadHoldsReservedIsRemindedNotTypedAndJournaledReadOnce", func(t *testing.T, e *env) {
			notice := e.post(t, program(batten), beta, "reserved")
			reading, err := e.store.Read(strand(beta), "")
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			e.start(t)
			e.probeWaits(1)
			want := []string{reminderFor(1, "", "#1 from \""+batten+"\"")}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Fatalf("typed %q while a read held the notice; want %q", got, want)
			}
			if err := reading.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: notice})); !slices.Equal(got, []string{"posted", "reminded", "read"}) {
				t.Errorf("Log(#%d) events = %v; want posted, reminded and one read", notice, got)
			}
		}},
		{"NoticeBeingTypedIsNotClaimedAtABoundary", func(t *testing.T, e *env) {
			notice := e.post(t, program(batten), beta, "typing")
			claimed := make(chan []creelengine.Message, 1)
			e.del.SetSendFunc(func(string, string) error {
				reading, err := e.store.ClaimAtBoundary(strand(beta), creelengine.Boundary{})
				if err != nil {
					t.Errorf("ClaimAtBoundary during the send: %v", err)
					claimed <- nil
					return nil
				}
				claimed <- reading.Messages
				return nil
			})
			e.start(t)
			e.probeWaits(1)
			if got := <-claimed; len(got) != 0 {
				t.Errorf("a claim during the send handed out %v; want nothing", got)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: notice})); !slices.Equal(got, []string{"posted", "read"}) {
				t.Errorf("Log(#%d) events = %v; want posted and one read", notice, got)
			}
		}},
		{"BusySendReleasesTheNoticeToAClaimAndAnswersTheHoldersWait", func(t *testing.T, e *env) {
			notice := e.post(t, program(batten), beta, "busy")
			waiting := make(chan *creelengine.Waiter, 1)
			e.del.SetSendFunc(func(string, string) error {
				// Registered while the pane path holds the notice, so the wait is not answered until the busy send releases it.
				waiter, err := e.store.WaitMail(strand(beta))
				if err != nil {
					t.Errorf("WaitMail during the send: %v", err)
				}
				select {
				case waiting <- waiter:
				default:
				}
				return creelengine.ErrBusy
			})
			e.start(t)
			e.probeWaits(1)
			waiter := <-waiting
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeMail {
				t.Errorf("the holder's wait was answered %+v; want mail once the busy send released the notice", outcome)
			}
			reading, err := e.store.ClaimAtBoundary(strand(beta), creelengine.Boundary{})
			if err != nil {
				t.Fatalf("ClaimAtBoundary: %v", err)
			}
			if len(reading.Messages) != 1 || reading.Messages[0].ID != notice {
				t.Errorf("a claim after the busy send handed out %v; want the notice #%d", reading.Messages, notice)
			}
		}},
		{"NoticeClaimedBeforeTheSendIsNeverTyped", func(t *testing.T, e *env) {
			notice := e.post(t, program(batten), beta, "claimed first")
			claimed := make(chan *creelengine.Reading, 1)
			var once sync.Once
			// The claim lands between the probe's idle check and its reservation of the notice.
			e.del.SetIdleHook(func(string) {
				once.Do(func() {
					reading, err := e.store.ClaimAtBoundary(strand(beta), creelengine.Boundary{})
					if err != nil {
						t.Errorf("ClaimAtBoundary during the idle check: %v", err)
					}
					claimed <- reading
				})
			})
			e.start(t)
			e.probeWaits(1)
			reading := <-claimed
			if reading == nil || len(reading.Messages) != 1 || reading.Messages[0].ID != notice {
				t.Fatalf("the claim handed out %+v; want the notice #%d", reading, notice)
			}
			if got := e.typed(beta); len(got) != 0 {
				t.Errorf("typed %q after a claim took the notice first; want nothing", got)
			}
			if err := reading.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: notice})); !slices.Equal(got, []string{"posted", "read"}) {
				t.Errorf("Log(#%d) events = %v; want posted and the claim's one read", notice, got)
			}
		}},
		{"BusyNoticeStaysUnreadAndANotLandedOneIsRead", func(t *testing.T, e *env) {
			busy := e.post(t, program(batten), beta, "busy")
			e.del.SetSendError(creelengine.ErrBusy)
			e.start(t)
			e.probeWaits(1)
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: busy})); !slices.Equal(got, []string{"posted"}) {
				t.Fatalf("Log(#%d) events after a busy send = %v; want only posted", busy, got)
			}
			e.del.SetSendError(creelengine.ErrNotLanded)
			e.tick(time.Second)
			want := []string{"[creel] #1 from \"" + batten + "\": busy", "[creel] #1 from \"" + batten + "\": busy"}
			if got := e.typed(beta); !slices.Equal(got, want) {
				t.Errorf("typed %q; want the busy try and the retry", got)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: busy})); !slices.Equal(got, []string{"posted", "read"}) {
				t.Errorf("Log(#%d) events after a not-landed send = %v; want posted and read", busy, got)
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
		{"OrchStrandIsRemindedAndTypedInto", orchStrandIsRemindedAndTypedInto(orch)},
		{"NumberedOrchStrandIsRemindedAndTypedInto", orchStrandIsRemindedAndTypedInto(orch2)},
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
			seen := len(e.probeWaits(0))
			e.post(t, strand(alpha), beta, "muted")
			e.post(t, program(webster), beta, "muted notice")
			mark := e.del.PausedCalls(beta)
			if advanced, _ := e.awaitMarkedProbe(t, beta, mark, seen); advanced >= 30*time.Minute {
				t.Fatalf("the marked probe took %v of clock; want less than the 30-minute hold", advanced)
			}
			if got := e.typed(beta); len(got) != 0 {
				t.Fatalf("typed %q under another sender's hold", got)
			}
			notice := "[creel] #3 from \"" + batten + "\": from the owner"
			e.post(t, program(batten), beta, "from the owner")
			awaitTyped(t, e, beta, notice)
			e.post(t, program(batten), beta, strings.Repeat("y", 4097))
			want := []string{notice, reminderFor(1, batten, "#4 from \""+batten+"\"")}
			awaitTyped(t, e, beta, want...)
			seen = len(e.probeWaits(0))
			e.clock.Advance(31 * time.Minute)
			mark = e.del.PausedCalls(beta)
			e.awaitMarkedProbe(t, beta, mark, seen)
			want = append(want, "[creel] #2 from \""+webster+"\": muted notice", reminderFor(2, "", "#1 from \""+alpha+"\"", "#4 from \""+batten+"\""))
			awaitTyped(t, e, beta, want...)
		}},
		{"LogByIDListsTheRemindedEventsThatHoldTheID", func(t *testing.T, e *env) {
			e.start(t)
			first := e.post(t, strand(alpha), beta, "one")
			e.probeWaits(1)
			e.readBodies(t, strand(beta), "")
			second := e.post(t, strand(alpha), beta, "two")
			awaitReminded(t, e, beta, 2)
			logs := []struct {
				filter creelengine.LogFilter
				want   []string
			}{
				{creelengine.LogFilter{ID: first}, []string{"posted", "reminded", "read"}},
				{creelengine.LogFilter{ID: second}, []string{"posted", "reminded"}},
				{creelengine.LogFilter{ID: second, To: beta}, []string{"posted", "reminded"}},
				{creelengine.LogFilter{ID: second, From: alpha}, []string{"posted"}},
			}
			for _, l := range logs {
				if got := eventsOf(e.store.Log(l.filter)); !slices.Equal(got, l.want) {
					t.Errorf("Log(%+v) events = %v; want %v", l.filter, got, l.want)
				}
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
			want := []string{"/one", "/two", reminderFor(1, "", "#1 from \""+alpha+"\"")}
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
			inject(t, e, program(batten), alpha, "/paused", 3*time.Second, false)
			e.start(t)
			if got := e.driveProbeWaits(5); !slices.Equal(got, seconds(1, 2, 4, 8, 8)) {
				t.Fatalf("probe waits under the pause = %v; want 1, 2, 4, 8, 8 seconds, uncapped by the frozen TTL", got)
			}
			e.del.SetPaused(alpha, false)
			e.del.SetStrands(creelfake.DormantStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.tick(8 * time.Second)
			e.tick(8 * time.Second)
			if got := e.probeWaits(7)[5:]; !slices.Equal(got, seconds(8, 8)) {
				t.Errorf("probe waits while dormant = %v; want 8, 8 seconds, uncapped by the frozen TTL", got)
			}
			if queued := e.store.Queued(alpha); len(queued) != 1 {
				t.Fatalf("Queued = %+v after the pause and the dormancy; want the inject still waiting", queued)
			}
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.tick(8 * time.Second)
			if got := e.typed(alpha); !slices.Equal(got, []string{"/paused"}) {
				t.Errorf("typed %q; want the inject typed once live and unpaused, after its TTL stood still", got)
			}
		}},
		{"InjectExpiresWithinASecondOfItsTTLBetweenBackoffSteps", func(t *testing.T, _ *env) {
			for _, ttl := range seconds(10, 20, 37) {
				e := newEnv(t)
				e.del.SetIdle(alpha, false)
				posted := e.clock.Now()
				_, waiter, err := e.store.InjectWait(program(batten), alpha, "/slow", ttl, false)
				if err != nil {
					t.Fatalf("InjectWait: %v", err)
				}
				e.start(t)
				done := make(chan struct{})
				go e.clock.AdvanceByWaitsWhere(isProbeWait, done)
				outcome := awaitOutcome(t, waiter)
				close(done)
				if outcome.Kind != creelengine.OutcomeExpired {
					t.Errorf("TTL %v: wait answered %v; want expired", ttl, outcome.Kind)
				}
				if late := e.clock.Now().Sub(posted) - ttl; late < 0 || late > time.Second {
					t.Errorf("TTL %v: expired %v past the TTL; want within a second", ttl, late)
				}
				for _, wait := range e.clock.Waits() {
					if isProbeWait(wait) && wait < time.Second {
						t.Errorf("TTL %v: recorded probe wait %v is under the one-second floor", ttl, wait)
					}
				}
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
			before := len(e.probeWaits(0))
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/short", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.probeWaits(before + 1)
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
		{"WithdrawLeavesAnInjectAlreadySendingAndItSettlesInjected", func(t *testing.T, e *env) {
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
			sending, waiter, err := e.store.InjectWait(program(batten), alpha, "/first", time.Hour, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			<-entered
			queued := inject(t, e, program(batten), alpha, "/second", time.Hour, false)
			got, err := e.store.Withdraw(program(batten), alpha)
			if err != nil || !slices.Equal(got.Injects, []int64{queued}) {
				t.Fatalf("Withdraw = %+v, %v; want only the queued inject %d", got, err, queued)
			}
			close(release)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeInjected {
				t.Errorf("wait answered %v; want the sending inject to settle injected", outcome.Kind)
			}
			if events := eventsOf(e.store.Log(creelengine.LogFilter{ID: sending.ID})); !slices.Equal(events, []string{"posted", "sending", "injected"}) {
				t.Errorf("events of the sending inject = %v; want posted, sending, injected", events)
			}
		}},
		{"CommandInjectIsRunThroughCommandAndJournaledSendingThenInjected", func(t *testing.T, e *env) {
			command := creelengine.Command{Name: creelengine.CommandCompact, Argument: "keep the plan"}
			result := injectCommand(t, e, orch, command)
			e.start(t)
			e.probeWaits(1)
			want := []creelfake.CommandCall{{Strand: orch, Command: command}}
			if got := e.del.Commands(); !reflect.DeepEqual(got, want) {
				t.Errorf("Commands = %+v; want %+v", got, want)
			}
			if sends := e.del.Sends(); len(sends) != 0 {
				t.Errorf("a command inject also typed %v through Send", sends)
			}
			entries := e.store.Log(creelengine.LogFilter{ID: result.ID})
			if got := eventsOf(entries); !slices.Equal(got, []string{"posted", "sending", "injected"}) {
				t.Errorf("events = %v; want posted, sending, injected", got)
			}
			for _, entry := range entries {
				if entry.Command != creelengine.CommandCompact || entry.Argument != "keep the plan" {
					t.Errorf("log entry %+v lacks the command and argument", entry)
				}
			}
		}},
		{"CommandInjectUnconfirmedIsJournaledAndNeverRerun", func(t *testing.T, e *env) {
			e.del.SetCommandError(creelengine.ErrNotLanded)
			result := injectCommand(t, e, orch, creelengine.Command{Name: creelengine.CommandClear})
			e.start(t)
			e.probeWaits(1)
			e.clock.Advance(time.Hour)
			if got := e.del.Commands(); len(got) != 1 {
				t.Errorf("Commands = %+v; an unconfirmed command inject must never be rerun", got)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: result.ID})); !slices.Equal(got, []string{"posted", "sending", "unconfirmed"}) {
				t.Errorf("events = %v; want posted, sending, unconfirmed", got)
			}
		}},
		{"BusyCommandIsRequeuedAndRunOnceAtTheNextProbe", func(t *testing.T, e *env) {
			e.del.SetCommandError(creelengine.ErrBusy)
			result := injectCommand(t, e, orch, creelengine.Command{Name: creelengine.CommandReloadPlugins})
			e.start(t)
			e.probeWaits(1)
			e.del.SetCommandError(nil)
			e.tick(time.Second)
			if got := e.del.Commands(); len(got) != 2 {
				t.Errorf("Commands = %+v; want the busy attempt and one retry", got)
			}
			if events := eventsOf(e.store.Log(creelengine.LogFilter{ID: result.ID})); events[len(events)-1] != "injected" {
				t.Errorf("events = %v; want the inject to end injected", events)
			}
		}},
		{"CommandInjectWaitsOutAPauseAndExpiresAfterItsTTL", func(t *testing.T, e *env) {
			e.del.SetPaused(orch, true)
			injectCommand(t, e, orch, creelengine.Command{Name: creelengine.CommandClear})
			e.start(t)
			e.driveProbeWaits(3)
			if got := e.del.Commands(); len(got) != 0 {
				t.Fatalf("Commands = %+v under a covering pause; want none", got)
			}
			e.del.SetPaused(orch, false)
			e.tick(4 * time.Second)
			if got := e.del.Commands(); len(got) != 1 {
				t.Errorf("Commands = %+v after the pause lifted; want the command run once", got)
			}

			expiring := newEnv(t)
			expiring.del.SetIdle(orch, false)
			result, err := expiring.store.InjectCommand(program(batten), orch, creelengine.Command{Name: creelengine.CommandClear}, 10*time.Second)
			if err != nil {
				t.Fatalf("InjectCommand: %v", err)
			}
			expiring.start(t)
			done := make(chan struct{})
			go expiring.clock.AdvanceByWaitsWhere(isProbeWait, done)
			defer close(done)
			deadline := time.Now().Add(10 * time.Second)
			for {
				events := eventsOf(expiring.store.Log(creelengine.LogFilter{ID: result.ID}))
				if slices.Contains(events, "expired") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("events = %v; want the command inject expired", events)
				}
				time.Sleep(time.Millisecond)
			}
			if got := expiring.del.Commands(); len(got) != 0 {
				t.Errorf("Commands = %+v; an expired command inject runs nothing", got)
			}
		}},
		{"CommandInjectToADormantOrchStaysQueuedUntilItIsLive", func(t *testing.T, e *env) {
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.DormantStrand(orch), creelfake.LiveStrand(orch2))
			injectCommand(t, e, orch, creelengine.Command{Name: creelengine.CommandClear})
			e.start(t)
			e.probeWaits(1)
			if got := e.del.Commands(); len(got) != 0 {
				t.Fatalf("Commands = %+v for a dormant orch; want none", got)
			}
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.tick(time.Second)
			if got := e.del.Commands(); len(got) != 1 {
				t.Errorf("Commands = %+v once the orch is live; want the command run once", got)
			}
		}},
		{"TextAndCommandInjectsAreTypedInQueueOrder", func(t *testing.T, e *env) {
			first := inject(t, e, program(batten), orch, "/one", 0, false)
			second := injectCommand(t, e, orch, creelengine.Command{Name: creelengine.CommandClear}).ID
			third := inject(t, e, program(batten), orch, "/three", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			e.tick(time.Second)
			var sending []int64
			for _, entry := range e.store.Log(creelengine.LogFilter{To: orch}) {
				if entry.Event == "sending" {
					sending = append(sending, entry.ID)
				}
			}
			if want := []int64{first, second, third}; !slices.Equal(sending, want) {
				t.Errorf("sending events = %v; want the ids in queue order %v", sending, want)
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
			e.del.SetStall(alpha, creelengine.StallNoInputBox)
			_, waiter, err := e.store.InjectWait(program(batten), alpha, "/clear", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.start(t)
			done := make(chan struct{})
			go e.clock.AdvanceByWaitsWhere(isProbeWait, done)
			outcome := awaitOutcome(t, waiter)
			close(done)
			if outcome.Kind != creelengine.OutcomeExpired || outcome.Stall != creelengine.StallNoInputBox {
				t.Errorf("wait answered %v with stall %q; want expired at the TTL with stall no-input-box", outcome.Kind, outcome.Stall)
			}
			if got := stalledReasons(e, alpha); !slices.Equal(got, []string{"no-input-box"}) {
				t.Errorf("stalled reasons = %q; want one", got)
			}
			var expiredReasons []string
			for _, entry := range e.store.Log(creelengine.LogFilter{To: alpha}) {
				if entry.Event == "expired" {
					expiredReasons = append(expiredReasons, entry.Reason)
				}
			}
			if !slices.Equal(expiredReasons, []string{"no-input-box"}) {
				t.Errorf("expired log reasons = %q; want no-input-box", expiredReasons)
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
			before := len(e.probeWaits(0))
			muted, waiter, err := e.store.InjectWait(external(), alpha, "/muted", time.Minute, false)
			if err != nil {
				t.Fatalf("InjectWait: %v", err)
			}
			e.probeWaits(before + 1)
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
		{"PausedStrandTypesNothingAndIsNotClaimedByTheWorker", func(t *testing.T, e *env) {
			e.del.SetPaused(beta, true)
			inject(t, e, program(batten), beta, "/clear", 0, false)
			e.start(t)
			e.probeWaits(1)
			e.tick(time.Second)
			if got := e.del.Sends(); len(got) != 0 {
				t.Errorf("sent %v to a paused strand", got)
			}
			if got := e.del.Claims(); len(got) != 0 {
				t.Errorf("claims = %v; the delivery worker under a pause must record no strand", got)
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
		reading, err := e.store.Read(strand(as), "")
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
			e.start(t)
			e.post(t, strand(alpha), orch, "one")
			e.sweepWaits(1)
			e.clock.Advance(30 * time.Second)
			e.sweepWaits(2)
			e.clock.Advance(45 * time.Second)
			e.post(t, strand(beta), orch, "two")
			e.clock.Advance(5 * time.Second)
			if got := e.sweepWaits(3)[2]; got != 50*time.Second {
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
			_, first, err := e.store.PostWait(external(), orch, "one")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			without(e, orch)
			e.start(t)
			// The 5-second wait left to the confirm deadline is below the probe ceiling, so the row tells the sweep's waits from the workers' by the workers' own doubling steps.
			sweepOnly := func(d time.Duration) bool { return !slices.Contains(seconds(1, 2, 4, 8), d) }
			e.clock.AwaitWaitsWhere(1, sweepOnly)
			e.clock.Advance(30 * time.Second)
			if waits := e.clock.AwaitWaitsWhere(2, sweepOnly); waits[1] != 10*time.Second {
				t.Fatalf("wait after the first miss = %v; want the 10s confirm", waits[1])
			}
			e.clock.Advance(5 * time.Second)
			e.post(t, strand(alpha), orch2, "two")
			if waits := e.clock.AwaitWaitsWhere(3, sweepOnly); waits[2] != 5*time.Second {
				t.Fatalf("wait after a post inside the confirm = %v; want the 5s left to the confirm deadline", waits[2])
			}
			e.clock.Advance(5 * time.Second)
			if outcome := awaitOutcome(t, first); outcome.Kind != creelengine.OutcomeReturned {
				t.Errorf("wait answered %+v; want the message returned undelivered at the confirm deadline", outcome)
			}
		}},
		{"MailReturnsOneStepAtATimeThenIsUndeliveredAndAnswersItsWaiter", func(t *testing.T, e *env) {
			// Over the carried-line cap, so the returned mail stays unread behind a reminder and returns again once orch2 is gone.
			id, waiter, err := e.store.PostWait(external(), orch2, strings.Repeat("chain ", 700))
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
			// The returned mail's reminder may already be journaled, since the worker types it without the test advancing the clock.
			want := []string{"posted", "forwarded", "returned"}
			got := slices.DeleteFunc(eventsOf(e.store.Log(creelengine.LogFilter{ID: id})), func(event string) bool { return event == "reminded" })
			if !slices.Equal(got, want) {
				t.Errorf("events = %v; want %v", got, want)
			}
			awaitTyped(t, e, orch2, reminderFor(1, "", "#1 from \""+external().Name+"\""))
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
		{"HolderRemovedAfterReadingAMarkedMessageSettlesItUnansweredHolderGoneWithNoChase", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(strand(beta), alpha, "which schema?")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			e.readBodies(t, strand(alpha), "")
			without(e, alpha)
			e.sweepStep()
			e.sweepLast()
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnanswered || outcome.Message.ID != id {
				t.Errorf("wait answered %+v; want unanswered carrying message #%d", outcome, id)
			}
			var reasons []string
			for _, entry := range e.store.Log(creelengine.LogFilter{ID: id}) {
				if entry.Event == "unanswered" {
					reasons = append(reasons, entry.Reason)
				}
			}
			if want := []string{creelengine.UnansweredHolderGone}; !slices.Equal(reasons, want) {
				t.Errorf("unanswered reasons = %v; want %v", reasons, want)
			}
			if got := e.countEvents(id, "chased"); got != 0 {
				t.Errorf("chased events = %d; want no chase to a gone holder", got)
			}
			var notices []string
			for _, entry := range e.store.Log(creelengine.LogFilter{From: "ly:CREEL", To: beta}) {
				if entry.Event == "posted" {
					notices = append(notices, entry.Subject)
				}
			}
			if len(notices) != 1 || !strings.Contains(notices[0], "its holder is gone") {
				t.Errorf("notices to the sender = %q; want one that the holder is gone", notices)
			}
		}},
		{"HolderRemovedAfterReadingAMarkedMessageStillReturnsItsUnreadMailAlongItsPath", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(strand(beta), alpha, "which schema?")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			e.readBodies(t, strand(alpha), "")
			e.post(t, strand(orch), alpha, "unread when the holder goes")
			without(e, alpha)
			e.sweepStep()
			e.sweepLast()
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnanswered || outcome.Message.ID != id {
				t.Errorf("wait answered %+v; want unanswered carrying message #%d", outcome, id)
			}
			back := holderOf(t, e, orch)
			if len(back) != 1 || back[0].EndedUnreadBy != alpha {
				t.Errorf("orch's mail = %+v; want the unread message back, marked as ended unread by %s", back, alpha)
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

// TestScheduler_HookWake drives a store over the fake delivery and clock with hook strands, and pins when the daemon rings a strand's idle waiter and when it types the bell.
// A strand's waiter is answered once per claimable set, and only while no turn is in progress, whatever the input box holds.
// The bell follows a set no waiter reaches, once per set, after the grace of 5 seconds or after a ring unanswered for 30.
func TestScheduler_HookWake(t *testing.T) {
	t.Parallel()
	// bigBody is sized so two of its lines pass the claim's byte budget and each claim takes one message.
	bigBody := strings.Repeat("x", 5000)
	hookEnv := func(t *testing.T) *env {
		t.Helper()
		e := newEnv(t)
		e.del.SetStrands(creelfake.HookStrand(alpha), creelfake.HookStrand(beta), creelfake.LiveStrand(orch))
		return e
	}
	// settle runs trigger and waits until a worker has probed after it and gone back to sleep.
	// It counts one probe, so the trigger wakes alpha's worker at most once:
	// a second wake during that probe is buffered and runs a probe after settle returned.
	// A row that posts several messages at once posts them before the scheduler starts and settles the start.
	settle := func(e *env, trigger func()) {
		seen := len(e.probeWaits(0))
		trigger()
		e.probeWaits(seen + 1)
	}
	register := func(t *testing.T, e *env, name string) *creelengine.Waiter {
		t.Helper()
		waiter, err := e.store.HookWait(strand(name), false)
		if err != nil {
			t.Fatalf("HookWait(%s): %v", name, err)
		}
		return waiter
	}
	// registerTaking registers a waiter marked as taking mail, whose ring carries the claimed messages.
	registerTaking := func(t *testing.T, e *env, name string) *creelengine.Waiter {
		t.Helper()
		waiter, err := e.store.HookWait(strand(name), true)
		if err != nil {
			t.Fatalf("HookWait(%s, takes mail): %v", name, err)
		}
		return waiter
	}
	// ringed waits for the waiter's ring and returns its claim, which a marked waiter's ring always carries.
	ringed := func(t *testing.T, waiter *creelengine.Waiter) *creelengine.Reading {
		t.Helper()
		outcome := awaitOutcome(t, waiter)
		if outcome.Kind != creelengine.OutcomeRing || outcome.Reading == nil {
			t.Fatalf("the marked waiter was answered %+v; want a ring carrying a claim", outcome)
		}
		return outcome.Reading
	}
	// bodiesOf returns the bodies of the messages a reading holds.
	bodiesOf := func(reading *creelengine.Reading) []string {
		var bodies []string
		for _, msg := range reading.Messages {
			bodies = append(bodies, msg.Body)
		}
		return bodies
	}
	// boundaryClaim claims as alpha's tool-boundary hook and acknowledges, returning the bodies handed out.
	boundaryClaim := func(t *testing.T, e *env) []string {
		t.Helper()
		reading, err := e.store.ClaimAtBoundary(strand(alpha), creelengine.Boundary{})
		if err != nil {
			t.Fatalf("ClaimAtBoundary: %v", err)
		}
		if err := reading.Ack(); err != nil {
			t.Fatalf("Ack: %v", err)
		}
		return bodiesOf(reading)
	}
	// linesOf returns the lines the messages are handed out as.
	linesOf := func(messages []creelengine.Message) []string {
		var lines []string
		for _, msg := range messages {
			lines = append(lines, creelengine.MessageLine(msg))
		}
		return lines
	}
	// claimOnce claims as alpha's hook at the boundary and returns the reading without settling it.
	claimOnce := func(t *testing.T, e *env, boundary creelengine.Boundary) *creelengine.Reading {
		t.Helper()
		reading, err := e.store.ClaimAtBoundary(strand(alpha), boundary)
		if err != nil {
			t.Fatalf("ClaimAtBoundary: %v", err)
		}
		return reading
	}
	// ackedRing rings a marked waiter of alpha for a post of body, acknowledges the ring and returns the line it carried, which is then owed as an echo.
	ackedRing := func(t *testing.T, e *env, body string) []string {
		t.Helper()
		waiter := registerTaking(t, e, alpha)
		e.post(t, strand(beta), alpha, body)
		settle(e, func() { e.start(t) })
		ring := ringed(t, waiter)
		if err := ring.Ack(); err != nil {
			t.Fatalf("Ack of the ring: %v", err)
		}
		return linesOf(ring.Messages)
	}
	wantKind := func(t *testing.T, waiter *creelengine.Waiter, want creelengine.OutcomeKind) {
		t.Helper()
		if got := awaitOutcome(t, waiter).Kind; got != want {
			t.Fatalf("the waiter was answered %q; want %q", got, want)
		}
	}
	wantSilent := func(t *testing.T, waiter *creelengine.Waiter) {
		t.Helper()
		select {
		case outcome := <-waiter.C:
			t.Fatalf("the waiter was answered %q; want no answer", outcome.Kind)
		default:
		}
	}
	claim := func(t *testing.T, e *env, stopHookActive bool) []creelengine.Message {
		t.Helper()
		reading, err := e.store.ClaimAtBoundary(strand(alpha), creelengine.Boundary{Stop: true, StopHookActive: stopHookActive})
		if err != nil {
			t.Fatalf("ClaimAtBoundary: %v", err)
		}
		if err := reading.Ack(); err != nil {
			t.Fatalf("Ack: %v", err)
		}
		return reading.Messages
	}
	// step runs one probe of the worker at the wait it armed.
	step := func(e *env) {
		waits := e.probeWaits(1)
		e.tick(waits[len(waits)-1])
	}
	// untilBell steps probes until the bell was sent into alpha, and returns the clock time since from; the fake-time minute only bounds a failing test.
	untilBell := func(t *testing.T, e *env, from time.Time) time.Duration {
		t.Helper()
		for e.clock.Now().Sub(from) < time.Minute {
			step(e)
			if slices.Contains(e.typed(alpha), creelengine.BellLine) {
				return e.clock.Now().Sub(from)
			}
		}
		t.Fatal("no bell was typed within a minute of fake time")
		return 0
	}
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"AnIdleStrandWithADraftIsRungOnceForASetAndAgainForANewPost", func(t *testing.T, e *env) {
			e.del.SetTurn(alpha, false, time.Time{})
			e.del.SetStall(alpha, creelengine.StallDraft)
			e.start(t)
			first := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantKind(t, first, creelengine.OutcomeRing)
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q into a hook strand; want nothing", got)
			}
			var second *creelengine.Waiter
			settle(e, func() { second = register(t, e, alpha) })
			wantSilent(t, second)
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			wantKind(t, second, creelengine.OutcomeRing)
		}},
		{"ARunningTurnRingsNothingUntilItEnds", func(t *testing.T, e *env) {
			e.del.SetTurn(alpha, true, time.Time{})
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			var waiter *creelengine.Waiter
			settle(e, func() { waiter = register(t, e, alpha) })
			wantSilent(t, waiter)
			e.del.SetTurn(alpha, false, time.Time{})
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			wantKind(t, waiter, creelengine.OutcomeRing)
		}},
		{"AClaimableSetStandingBeforeTheWaiterRegistersRingsAtOnce", func(t *testing.T, e *env) {
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			var waiter *creelengine.Waiter
			settle(e, func() { waiter = register(t, e, alpha) })
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeRing || outcome.Reading != nil {
				t.Fatalf("the unmarked waiter was answered %+v; want a ring carrying no claim", outcome)
			}
			if got := boundaryClaim(t, e); !slices.Equal(got, []string{"one"}) {
				t.Errorf("a claim after the unmarked ring handed out %q; want the mail", got)
			}
		}},
		{"AMarkedWaiterIsRungWithTheBudgetedClaimAndItsAckJournalsItAndAnswersTheRing", func(t *testing.T, e *env) {
			ringAt := e.clock.Now()
			e.del.SetTurn(alpha, false, ringAt.Add(-time.Minute))
			first := registerTaking(t, e, alpha)
			e.post(t, strand(beta), alpha, bigBody)
			e.post(t, strand(beta), alpha, bigBody)
			settle(e, func() { e.start(t) })
			reading := ringed(t, first)
			if len(reading.Messages) != 1 || reading.Messages[0].ID != 1 {
				t.Fatalf("the ring carried %+v; want #1 alone, since #2 passes the byte budget", reading.Messages)
			}
			if err := reading.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{To: alpha})); !slices.Equal(got, []string{"posted", "posted", "read"}) {
				t.Errorf("Log(to alpha) events = %v; want two posts and the read of #1", got)
			}
			var second *creelengine.Waiter
			settle(e, func() { second = registerTaking(t, e, alpha) })
			for e.clock.Now().Sub(ringAt) < 45*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Errorf("typed %q after an acknowledged ring; want no bell, since the acknowledgement answered it", got)
			}
			wantSilent(t, second)
			if got := boundaryClaim(t, e); len(got) != 0 {
				t.Errorf("the tool-boundary claim after the ring handed out %d messages; want the echo of #1 alone, which fills the budget", len(got))
			}
			if got := boundaryClaim(t, e); len(got) != 1 || got[0] != bigBody {
				t.Errorf("a later tool-boundary claim handed out %d messages; want #2", len(got))
			}
		}},
		{"AMarkedRingStartsTheStopCountOver", func(t *testing.T, e *env) {
			waiter := registerTaking(t, e, alpha)
			e.post(t, strand(beta), alpha, bigBody)
			e.post(t, strand(beta), alpha, bigBody)
			settle(e, func() { e.start(t) })
			if err := ringed(t, waiter).Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if got := claim(t, e, true); len(got) != 0 {
				t.Fatalf("the first Stop claim after the marked ring handed out %+v; want the echo of #1 alone, which fills the budget", got)
			}
			if got := claim(t, e, true); len(got) != 1 || got[0].ID != 2 {
				t.Errorf("the Stop claim after the marked ring handed out %+v; want #2, which a count no claim of this daemon set would refuse", got)
			}
		}},
		{"AMarkedWaiterUnderAnotherSendersHoldIsRungWithTheOwnersMailOnly", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "not the owner's") })
			wantSilent(t, waiter)
			settle(e, func() { e.post(t, program(batten), alpha, "the owner's") })
			if got := bodiesOf(ringed(t, waiter)); !slices.Equal(got, []string{"the owner's"}) {
				t.Errorf("the ring carried %q; want the owner's mail only", got)
			}
		}},
		{"AMarkedWaiterIsRungWithoutALineOverTheHookFieldCap", func(t *testing.T, e *env) {
			waiter := registerTaking(t, e, alpha)
			if _, err := e.store.PostUnchecked(strand(beta), alpha, strings.Repeat("x", 10001)); err != nil {
				t.Fatalf("PostUnchecked: %v", err)
			}
			e.post(t, strand(beta), alpha, "short")
			settle(e, func() { e.start(t) })
			if got := bodiesOf(ringed(t, waiter)); !slices.Equal(got, []string{"short"}) {
				t.Errorf("the ring carried %q; want the short post alone", got)
			}
		}},
		{"APauseReadAtTheRingRingsNothingAndKeepsTheWaiter", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			e.del.SetIdleHook(func(string) { e.del.SetPaused(alpha, true) })
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantSilent(t, waiter)
			e.del.SetIdleHook(nil)
			e.del.SetPaused(alpha, false)
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			if got := bodiesOf(ringed(t, waiter)); !slices.Equal(got, []string{"one", "two"}) {
				t.Errorf("the ring carried %q; want both posts", got)
			}
		}},
		{"APauseReadErrorAtTheRingRingsNothingAndKeepsTheWaiter", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			e.del.SetIdleHook(func(string) { e.del.SetPausedError(errors.New("pause record unreadable")) })
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantSilent(t, waiter)
			e.del.SetIdleHook(nil)
			e.del.SetPausedError(nil)
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			if got := bodiesOf(ringed(t, waiter)); !slices.Equal(got, []string{"one", "two"}) {
				t.Errorf("the ring carried %q; want both posts", got)
			}
		}},
		{"AClaimTakingTheSetBeforeTheRingRingsNothingAndKeepsTheWaiter", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			var held atomic.Pointer[creelengine.Reading]
			e.del.SetIdleHook(func(string) {
				if held.Load() != nil {
					return
				}
				reading, err := e.store.ClaimAtBoundary(strand(alpha), creelengine.Boundary{})
				if err != nil {
					t.Errorf("ClaimAtBoundary during the probe: %v", err)
					return
				}
				held.Store(reading)
			})
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantSilent(t, waiter)
			e.del.SetIdleHook(nil)
			if reading := held.Load(); reading == nil || len(reading.Messages) != 1 {
				t.Fatalf("the claim during the probe held %+v; want the set", reading)
			} else {
				reading.Release()
			}
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			if got := bodiesOf(ringed(t, waiter)); !slices.Equal(got, []string{"one", "two"}) {
				t.Errorf("the ring carried %q; want both posts", got)
			}
		}},
		{"ARingOutcomeNeverTakenIsReleasedWhenItsWaitIsCancelled", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			ringAt := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			if got := len(waiter.C); got != 1 {
				t.Fatalf("the waiter's channel holds %d outcomes after the ring; want one", got)
			}
			waiter.Cancel()
			if elapsed := untilBell(t, e, ringAt); elapsed < 30*time.Second {
				t.Errorf("the bell came %v after the ring; want at least the 30s window", elapsed)
			}
			if got := boundaryClaim(t, e); !slices.Equal(got, []string{"one"}) {
				t.Errorf("a claim after the cancelled wait handed out %q; want the mail the ring reserved", got)
			}
		}},
		{"AReleasedRingClaimLeavesTheMailUnreadAndTheBellFollows", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			ringAt := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			ringed(t, waiter).Release()
			if elapsed := untilBell(t, e, ringAt); elapsed < 30*time.Second {
				t.Errorf("the bell came %v after the ring; want at least the 30s window", elapsed)
			}
			if got := boundaryClaim(t, e); !slices.Equal(got, []string{"one"}) {
				t.Errorf("a claim after the released ring handed out %q; want the mail", got)
			}
		}},
		{"AJournalFailureAtTheRingsAckLeavesTheRingUnanswered", func(t *testing.T, e *env) {
			e.start(t)
			waiter := registerTaking(t, e, alpha)
			ringAt := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			reading := ringed(t, waiter)
			e.store.FailJournalWrites(0)
			if err := reading.Ack(); err == nil {
				t.Fatal("Ack with the journal write failing returned nil; want an error")
			}
			e.store.FailJournalWrites(-1)
			if elapsed := untilBell(t, e, ringAt); elapsed < 30*time.Second {
				t.Errorf("the bell came %v after the ring; want at least the 30s window, since the failed ack answered nothing", elapsed)
			}
		}},
		{"AnAcknowledgedRingLeavesAnEchoTheNextClaimHandsOutAheadOfItsMail", func(t *testing.T, e *env) {
			ring := ackedRing(t, e, "one")
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			reading := claimOnce(t, e, creelengine.Boundary{})
			if !slices.Equal(reading.Echo, ring) || !slices.Equal(bodiesOf(reading), []string{"two"}) {
				t.Fatalf("the claim handed out echo %q and messages %q; want %q and the second post", reading.Echo, bodiesOf(reading), ring)
			}
			if err := reading.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			if again := claimOnce(t, e, creelengine.Boundary{}); len(again.Echo) != 0 || len(again.Messages) != 0 {
				t.Errorf("the claim after the acknowledged one handed out echo %q and messages %q; want nothing", again.Echo, bodiesOf(again))
			}
		}},
		{"ThePausedOrHeldStrandsClaimStillHandsOutTheEcho", func(t *testing.T, e *env) {
			ring := ackedRing(t, e, "one")
			e.del.SetPaused(alpha, true)
			paused := claimOnce(t, e, creelengine.Boundary{})
			if !slices.Equal(paused.Echo, ring) || len(paused.Messages) != 0 {
				t.Fatalf("the paused claim handed out echo %q and messages %q; want the echo alone", paused.Echo, bodiesOf(paused))
			}
			paused.Release()
			e.del.SetPaused(alpha, false)
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			settle(e, func() { e.post(t, strand(beta), alpha, "not the owner's") })
			held := claimOnce(t, e, creelengine.Boundary{})
			if !slices.Equal(held.Echo, ring) || len(held.Messages) != 0 {
				t.Errorf("the held claim handed out echo %q and messages %q; want the echo alone", held.Echo, bodiesOf(held))
			}
		}},
		{"TheStopCapKeepsTheMailBackButNotTheEcho", func(t *testing.T, e *env) {
			ring := ackedRing(t, e, "short")
			settle(e, func() { e.post(t, strand(beta), alpha, bigBody) })
			for i := range 6 {
				reading := claimOnce(t, e, creelengine.Boundary{Stop: true, StopHookActive: i > 0})
				if !slices.Equal(reading.Echo, ring) || !slices.Equal(bodiesOf(reading), []string{bigBody}) {
					t.Fatalf("Stop claim %d handed out echo %q and %d messages; want the echo and the big post", i+1, reading.Echo, len(reading.Messages))
				}
				reading.Release()
			}
			capped := claimOnce(t, e, creelengine.Boundary{Stop: true, StopHookActive: true})
			if !slices.Equal(capped.Echo, ring) || len(capped.Messages) != 0 {
				t.Errorf("the seventh Stop claim handed out echo %q and %d messages; want the echo alone", capped.Echo, len(capped.Messages))
			}
		}},
		{"TheEchoSharesTheBudgetAndAReleaseKeepsIt", func(t *testing.T, e *env) {
			ring := ackedRing(t, e, bigBody)
			settle(e, func() { e.post(t, strand(beta), alpha, bigBody) })
			for range 2 {
				reading := claimOnce(t, e, creelengine.Boundary{})
				if !slices.Equal(reading.Echo, ring) || len(reading.Messages) != 0 {
					t.Fatalf("the claim handed out echo %d lines and %d messages; want the echo alone, since the big post passes the shared budget", len(reading.Echo), len(reading.Messages))
				}
				reading.Release()
			}
		}},
		{"AJournalFailureAtTheRingsAckEchoesOnlyTheIdsBeforeTheFailingWrite", func(t *testing.T, e *env) {
			waiter := registerTaking(t, e, alpha)
			for _, body := range []string{"one", "two", "three"} {
				e.post(t, strand(beta), alpha, body)
			}
			settle(e, func() { e.start(t) })
			ring := ringed(t, waiter)
			e.store.FailJournalWrites(1)
			if err := ring.Ack(); err == nil {
				t.Fatal("Ack with the second journal write failing returned nil; want an error")
			}
			e.store.FailJournalWrites(-1)
			reading := claimOnce(t, e, creelengine.Boundary{})
			if want := linesOf(ring.Messages[:1]); !slices.Equal(reading.Echo, want) || !slices.Equal(bodiesOf(reading), []string{"two", "three"}) {
				t.Errorf("the claim handed out echo %q and messages %q; want the first line and the two released posts", reading.Echo, bodiesOf(reading))
			}
		}},
		{"AJournalFailureAtAClaimsAckDropsTheEchoItHandedOut", func(t *testing.T, e *env) {
			ring := ackedRing(t, e, "one")
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			reading := claimOnce(t, e, creelengine.Boundary{})
			if !slices.Equal(reading.Echo, ring) || !slices.Equal(bodiesOf(reading), []string{"two"}) {
				t.Fatalf("the claim handed out echo %q and messages %q; want the echo and the second post", reading.Echo, bodiesOf(reading))
			}
			e.store.FailJournalWrites(0)
			if err := reading.Ack(); err == nil {
				t.Fatal("Ack with the journal write failing returned nil; want an error")
			}
			e.store.FailJournalWrites(-1)
			after := claimOnce(t, e, creelengine.Boundary{})
			if len(after.Echo) != 0 || !slices.Equal(bodiesOf(after), []string{"two"}) {
				t.Errorf("the claim after the failed ack handed out echo %q and messages %q; want no echo and the released post", after.Echo, bodiesOf(after))
			}
		}},
		{"ARingAckEchoesAnIdThatLeftTheHolderBeforeTheAck", func(t *testing.T, e *env) {
			waiter := registerTaking(t, e, alpha)
			e.post(t, strand(beta), alpha, "one")
			e.post(t, strand(beta), alpha, "two")
			settle(e, func() { e.start(t) })
			ring := ringed(t, waiter)
			if err := e.store.Forward(strand(alpha), 2, beta); err != nil {
				t.Fatalf("Forward: %v", err)
			}
			if err := ring.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			reading := claimOnce(t, e, creelengine.Boundary{})
			if want := linesOf(ring.Messages); !slices.Equal(reading.Echo, want) || len(reading.Messages) != 0 {
				t.Errorf("the claim handed out echo %q and messages %q; want both lines of the ring and no mail", reading.Echo, bodiesOf(reading))
			}
		}},
		{"APauseRingsNothing", func(t *testing.T, e *env) {
			e.del.SetPaused(alpha, true)
			e.start(t)
			waiter := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantSilent(t, waiter)
		}},
		{"UnderAnotherSendersHoldOnlyTheOwnersMailMakesASet", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.start(t)
			waiter := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "not the owner's") })
			wantSilent(t, waiter)
			settle(e, func() { e.post(t, program(batten), alpha, "the owner's") })
			wantKind(t, waiter, creelengine.OutcomeRing)
		}},
		{"ANewerWaiterOfTheStrandSupersedesTheOlder", func(t *testing.T, e *env) {
			older := register(t, e, alpha)
			newer := register(t, e, alpha)
			wantKind(t, older, creelengine.OutcomeSuperseded)
			wantSilent(t, newer)
		}},
		{"ATurnEndBeforeTheRingHoldsTheReadingFalseUntilOneIsRecordedAfterIt", func(t *testing.T, e *env) {
			ringAt := e.clock.Now()
			e.del.SetTurn(alpha, false, ringAt.Add(-time.Minute))
			e.start(t)
			first := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantKind(t, first, creelengine.OutcomeRing)
			var second *creelengine.Waiter
			settle(e, func() { second = register(t, e, alpha) })
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			wantSilent(t, second)
			e.del.SetTurn(alpha, false, ringAt.Add(time.Second))
			var third *creelengine.Waiter
			settle(e, func() { third = register(t, e, alpha) })
			wantKind(t, second, creelengine.OutcomeSuperseded)
			wantKind(t, third, creelengine.OutcomeRing)
		}},
		{"MailArrivingInAWokenTurnsBlockedChainRingsNothingUntilANewTurnEnd", func(t *testing.T, e *env) {
			ringAt := e.clock.Now()
			e.del.SetTurn(alpha, false, ringAt.Add(-time.Minute))
			first := register(t, e, alpha)
			e.post(t, strand(beta), alpha, bigBody)
			e.post(t, strand(beta), alpha, bigBody)
			settle(e, func() { e.start(t) })
			wantKind(t, first, creelengine.OutcomeRing)
			if got := claim(t, e, false); len(got) != 1 {
				t.Fatalf("the Stop claim took %d messages; want part of the set", len(got))
			}
			var blocked *creelengine.Waiter
			settle(e, func() { blocked = register(t, e, alpha) })
			settle(e, func() { e.post(t, strand(beta), alpha, bigBody) })
			wantSilent(t, blocked)
			e.del.SetTurn(alpha, false, ringAt.Add(time.Second))
			var woken *creelengine.Waiter
			settle(e, func() { woken = register(t, e, alpha) })
			wantKind(t, woken, creelengine.OutcomeRing)
		}},
		{"ARingStartsTheStopCountOverWhereTheDaemonStartCapRefusedMail", func(t *testing.T, e *env) {
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			if got := claim(t, e, true); len(got) != 0 {
				t.Fatalf("a Stop claim on a count no claim of this daemon set took %d messages; want none", len(got))
			}
			var waiter *creelengine.Waiter
			settle(e, func() { waiter = register(t, e, alpha) })
			wantKind(t, waiter, creelengine.OutcomeRing)
			if got := claim(t, e, true); len(got) != 1 {
				t.Fatalf("a Stop claim after the ring took %d messages; want the mail", len(got))
			}
		}},
		{"ARingAfterTheStopCapStartsAFreshCount", func(t *testing.T, e *env) {
			for range 8 {
				e.post(t, strand(beta), alpha, bigBody)
			}
			settle(e, func() { e.start(t) })
			for i := range 6 {
				if got := claim(t, e, i > 0); len(got) != 1 {
					t.Fatalf("Stop claim %d took %d messages; want one", i+1, len(got))
				}
			}
			if got := claim(t, e, true); len(got) != 0 {
				t.Fatalf("the seventh Stop claim took %d messages; want none past the cap", len(got))
			}
			var waiter *creelengine.Waiter
			settle(e, func() { waiter = register(t, e, alpha) })
			wantKind(t, waiter, creelengine.OutcomeRing)
			if got := claim(t, e, true); len(got) != 1 {
				t.Fatalf("the Stop claim after the ring took %d messages; want one", len(got))
			}
		}},
		{"OnlyAStrandRegistersAWaiter", func(t *testing.T, e *env) {
			for _, sender := range []struct {
				name    creelengine.Sender
				wantErr string
			}{
				{program(batten), "lym creel read"},
				{external(), "lym creel post --wait-reply"},
			} {
				_, err := e.store.HookWait(sender.name, false)
				if err == nil || !strings.Contains(err.Error(), "is no strand") || !strings.Contains(err.Error(), sender.wantErr) {
					t.Errorf("HookWait(%s) = %v; want a refusal naming %q", sender.name.Name, err, sender.wantErr)
				}
			}
		}},
		{"APaneMailStrandIsNeverRung", func(t *testing.T, e *env) {
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta))
			e.start(t)
			waiter := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			wantSilent(t, waiter)
			if got := e.typed(alpha); len(got) != 1 {
				t.Errorf("typed %q into a pane-mail strand; want its reminder", got)
			}
		}},
		{"AnIdleStrandWithNoWaiterGetsTheBellOnceAfterTheGraceAndAgainForANewPost", func(t *testing.T, e *env) {
			e.start(t)
			opened := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			for e.clock.Now().Sub(opened) < 3*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q before the grace; want nothing", got)
			}
			if elapsed := untilBell(t, e, opened); elapsed < 5*time.Second {
				t.Errorf("the bell came %v after the store opened; want at least the 5s grace", elapsed)
			}
			for range 4 {
				step(e)
			}
			if got := e.typed(alpha); !slices.Equal(got, []string{creelengine.BellLine}) {
				t.Fatalf("typed %q over later probes of an unchanged set; want the bell once", got)
			}
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			if got := e.typed(alpha); !slices.Equal(got, []string{creelengine.BellLine, creelengine.BellLine}) {
				t.Errorf("typed %q after a new post; want the bell once more", got)
			}
		}},
		{"ADraftHoldsTheBellAndABusySendTypesItOnTheNextProbe", func(t *testing.T, e *env) {
			e.del.SetStall(alpha, creelengine.StallDraft)
			e.start(t)
			opened := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			for e.clock.Now().Sub(opened) < 20*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q into a pane holding a draft; want nothing", got)
			}
			e.del.SetStall(alpha, "")
			e.del.SetSendError(creelengine.ErrBusy)
			untilBell(t, e, e.clock.Now())
			attempts := len(e.typed(alpha))
			e.del.SetSendError(nil)
			step(e)
			for range 3 {
				step(e)
			}
			if got := e.typed(alpha); len(got) != attempts+1 {
				t.Errorf("typed %d bell lines after the busy one; want one retry and no more", len(got)-attempts)
			}
		}},
		{"ASendThatDidNotLandRecordsTheBellForTheSet", func(t *testing.T, e *env) {
			e.del.SetSendError(creelengine.ErrNotLanded)
			e.start(t)
			opened := e.clock.Now()
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			untilBell(t, e, opened)
			for range 4 {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 1 {
				t.Errorf("typed %q over later probes after a send that did not land; want the one attempt", got)
			}
		}},
		{"ARingUnansweredForTheWindowIsFollowedByTheBell", func(t *testing.T, e *env) {
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			var waiter *creelengine.Waiter
			settle(e, func() { waiter = register(t, e, alpha) })
			wantKind(t, waiter, creelengine.OutcomeRing)
			ringAt := e.clock.Now()
			if elapsed := untilBell(t, e, ringAt); elapsed < 30*time.Second {
				t.Errorf("the bell came %v after the ring; want it only after the 30s window", elapsed)
			}
		}},
		{"ARingAnsweredByAClaimIsNotFollowedByTheBell", func(t *testing.T, e *env) {
			e.del.SetTurn(alpha, false, e.clock.Now().Add(-time.Minute))
			e.post(t, strand(beta), alpha, bigBody)
			e.post(t, strand(beta), alpha, bigBody)
			settle(e, func() { e.start(t) })
			var first *creelengine.Waiter
			settle(e, func() { first = register(t, e, alpha) })
			wantKind(t, first, creelengine.OutcomeRing)
			ringAt := e.clock.Now()
			for e.clock.Now().Sub(ringAt) < 20*time.Second {
				step(e)
			}
			if got := claim(t, e, false); len(got) != 1 {
				t.Fatalf("the Stop claim took %d messages; want part of the set", len(got))
			}
			var second *creelengine.Waiter
			settle(e, func() { second = register(t, e, alpha) })
			wantSilent(t, second)
			for e.clock.Now().Sub(ringAt) < 45*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Errorf("typed %q after a claim answered the ring; want no bell", got)
			}
		}},
		{"ARingAnsweredByARunningTurnIsNotFollowedByTheBell", func(t *testing.T, e *env) {
			e.del.SetTurn(alpha, false, e.clock.Now().Add(-time.Minute))
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			var first *creelengine.Waiter
			settle(e, func() { first = register(t, e, alpha) })
			wantKind(t, first, creelengine.OutcomeRing)
			ringAt := e.clock.Now()
			e.del.SetTurn(alpha, true, time.Time{})
			step(e)
			e.del.SetTurn(alpha, false, ringAt.Add(-time.Minute))
			var second *creelengine.Waiter
			settle(e, func() { second = register(t, e, alpha) })
			wantSilent(t, second)
			for e.clock.Now().Sub(ringAt) < 45*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Errorf("typed %q after a running turn answered the ring; want no bell", got)
			}
		}},
		{"AWaiterThatEndsWithoutARingStartsTheGraceAgainFromItsEnd", func(t *testing.T, e *env) {
			e.del.SetTurn(alpha, true, time.Time{})
			e.start(t)
			opened := e.clock.Now()
			waiter := register(t, e, alpha)
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
			for e.clock.Now().Sub(opened) < 20*time.Second {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q while a waiter was live; want nothing", got)
			}
			endedAt := e.clock.Now()
			waiter.Cancel()
			if elapsed := untilBell(t, e, endedAt); elapsed < 5*time.Second {
				t.Errorf("the bell came %v after the waiter ended; want at least the 5s grace", elapsed)
			}
		}},
		{"UnderAnotherSendersHoldOnlyTheOwnersMailGetsTheBell", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.start(t)
			settle(e, func() { e.post(t, strand(beta), alpha, "not the owner's") })
			for range 3 {
				step(e)
			}
			if got := e.typed(alpha); len(got) != 0 {
				t.Fatalf("typed %q for mail the hold mutes; want nothing", got)
			}
			settle(e, func() { e.post(t, program(batten), alpha, "the owner's") })
			if got := e.typed(alpha); !slices.Equal(got, []string{creelengine.BellLine}) {
				t.Errorf("typed %q for the owner's mail; want the bell", got)
			}
		}},
		{"OnlyAStrandMarkedCreelHooksOnAnUnpausedPaneGetsTheBell", func(t *testing.T, e *env) {
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.HookStrand(orch))
			e.del.SetPaused(orch, true)
			e.start(t)
			settle(e, func() {
				e.post(t, strand(orch), alpha, "one")
				e.post(t, strand(orch), beta, "one")
				e.post(t, strand(alpha), orch, "one")
			})
			for range 4 {
				step(e)
			}
			for _, name := range []string{alpha, beta, orch} {
				if slices.Contains(e.typed(name), creelengine.BellLine) {
					t.Errorf("typed the bell into %s; want it only for an unpaused strand marked CreelHooks", name)
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := hookEnv(t)
			tt.run(t, e)
		})
	}
}

// chaseTick advances the clock by the chaser's current wait and waits until the chaser has rearmed, so the tick's charge, steps and journal records are done when it returns.
func (e *env) chaseTick() {
	waits := e.clock.AwaitWaitsWhere(1, isChaseWait)
	e.clock.Advance(waits[len(waits)-1])
	e.clock.AwaitWaitsWhere(len(waits)+1, isChaseWait)
}

// chaseTicks runs n chase ticks.
func (e *env) chaseTicks(n int) {
	for range n {
		e.chaseTick()
	}
}

// chaseTickLast advances the clock by the chaser's current wait without waiting for a rearm, for the tick after which the chaser ends.
func (e *env) chaseTickLast() {
	waits := e.clock.AwaitWaitsWhere(1, isChaseWait)
	e.clock.Advance(waits[len(waits)-1])
}

// awaitEvent waits until the log of message id holds an event of the given name; the real-time deadline only bounds a failing test.
func (e *env) awaitEvent(t *testing.T, id int64, event string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !slices.Contains(eventsOf(e.store.Log(creelengine.LogFilter{ID: id})), event) {
		if time.Now().After(deadline) {
			t.Fatalf("message #%d never logged %q; events %v", id, event, eventsOf(e.store.Log(creelengine.LogFilter{ID: id})))
		}
		time.Sleep(time.Millisecond)
	}
}

// countEvents counts the entries of message id's log with the given event name.
func (e *env) countEvents(id int64, event string) int {
	count := 0
	for _, name := range eventsOf(e.store.Log(creelengine.LogFilter{ID: id})) {
		if name == event {
			count++
		}
	}
	return count
}

// tickToUnanswered runs the 45 ticks of two chases and the step after them, and waits for the unanswered record of message id.
// The last tick ends the chaser, so it waits for no rearm.
func (e *env) tickToUnanswered(t *testing.T, id int64) {
	t.Helper()
	e.chaseTicks(44)
	e.chaseTickLast()
	e.awaitEvent(t, id, "unanswered")
}

// chaseAgedSeconds returns the consumed seconds of every chase-aged record of message id in the journal, oldest first.
func chaseAgedSeconds(t *testing.T, e *env, id int64) []int64 {
	t.Helper()
	var seconds []int64
	for _, line := range strings.Split(journalText(t, e.geom), "\n") {
		var rec struct {
			Event           string `json:"event"`
			ID              int64  `json:"id"`
			ConsumedSeconds int64  `json:"consumed_s"`
		}
		if line == "" || json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.Event == "chase-aged" && rec.ID == id {
			seconds = append(seconds, rec.ConsumedSeconds)
		}
	}
	return seconds
}

// writeJournal writes the lines as the journal file of the given day under the env's journal directory, for a store opened after it.
func (e *env) writeJournal(t *testing.T, day string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(e.geom.JournalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.geom.JournalDir, day+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// reopen stops the env's scheduler, closes its store and opens a new store and scheduler over the same journal.
func (e *env) reopen(t *testing.T) {
	t.Helper()
	if e.scheduler != nil {
		e.scheduler.Stop()
	}
	if err := e.store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	store, err := creelengine.OpenStore(e.geom, e.del, e.clock)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	e.store = store
	e.start(t)
}

// awaitChaseLines waits until n chase lines were typed into the strand and returns them; the real-time deadline only bounds a failing test.
func awaitChaseLines(t *testing.T, e *env, strandName string, n int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var lines []string
		for _, line := range e.typed(strandName) {
			if strings.Contains(line, "Chase ") {
				lines = append(lines, line)
			}
		}
		if len(lines) >= n {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("typed %d chase lines into %s; want %d", len(lines), strandName, n)
		}
		time.Sleep(time.Millisecond)
	}
}

// awaitTypedWith waits until a line containing part was typed into the strand; the real-time deadline only bounds a failing test.
func awaitTypedWith(t *testing.T, e *env, strandName, part string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !slices.ContainsFunc(e.typed(strandName), func(line string) bool { return strings.Contains(line, part) }) {
		if time.Now().After(deadline) {
			t.Fatalf("typed %q into %s; want a line containing %q", e.typed(strandName), strandName, part)
		}
		time.Sleep(time.Millisecond)
	}
}

// creelPostTo returns the id of the first message sender posted to receiver, 0 when there is none.
func creelPostTo(e *env, sender, receiver string) int64 {
	for _, entry := range e.store.Log(creelengine.LogFilter{From: sender}) {
		if entry.Event == "posted" && entry.To == receiver {
			return entry.ID
		}
	}
	return 0
}

// postMarked posts text from sender to receiver expecting a reply and returns the id.
func (e *env) postMarked(t *testing.T, sender creelengine.Sender, receiver, text string) int64 {
	t.Helper()
	id, _, _, err := e.store.PostWith(sender, receiver, text, creelengine.PostOptions{ExpectReply: true})
	if err != nil {
		t.Fatalf("PostWith(%s -> %s): %v", sender.Name, receiver, err)
	}
	return id
}

func TestScheduler_Chases(t *testing.T) {
	t.Parallel()
	const creel = "ly:CREEL"
	// seeded is a marked message from beta to alpha that alpha read, as journal lines a store folds at open.
	seeded := func(extra ...string) []string {
		return append([]string{
			`{"event":"posted","at":"2026-10-01T12:00:00Z","id":1,"from":"` + beta + `","to":"` + alpha + `","kind":"mail","body":"question","expect_reply":true}`,
			`{"event":"read","at":"2026-10-01T12:00:00Z","id":1,"by":"` + alpha + `"}`,
		}, extra...)
	}
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"ProgramSendersMessageIsChasedTwiceThenSettledUnansweredAndTheSenderIsTold", func(t *testing.T, e *env) {
			// A read makes batten's mailbox known, so it gets the notice.
			e.readBodies(t, program(batten), "")
			id, waiter, err := e.store.PostWait(program(batten), alpha, "which schema?")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			awaitTyped(t, e, alpha, "[creel] #1 from \""+batten+"\", expects a reply (lym creel post --reply 1 \"<text>\"): which schema?")

			e.chaseTicks(14)
			if got := e.countEvents(id, "chased"); got != 0 {
				t.Errorf("chased events after 14 ticks = %d; want none", got)
			}
			e.chaseTick()
			if got := e.countEvents(id, "chased"); got != 1 {
				t.Errorf("chased events after 15 ticks = %d; want chase 1", got)
			}
			e.chaseTicks(14)
			if got := e.countEvents(id, "chased"); got != 1 {
				t.Errorf("chased events after 29 ticks = %d; want only chase 1", got)
			}
			e.chaseTick()
			if got := e.countEvents(id, "chased"); got != 2 {
				t.Errorf("chased events after 30 ticks = %d; want chase 2", got)
			}
			e.chaseTicks(14)
			if got := e.countEvents(id, "unanswered"); got != 0 {
				t.Errorf("unanswered events after 44 ticks = %d; want none", got)
			}
			e.chaseTickLast()
			outcome := awaitOutcome(t, waiter)
			if outcome.Kind != creelengine.OutcomeUnanswered || outcome.Message.ID != id {
				t.Errorf("outcome = %+v; want unanswered carrying message #%d", outcome, id)
			}
			for _, entry := range e.store.Log(creelengine.LogFilter{ID: id}) {
				if entry.Event == "unanswered" && entry.Reason != creelengine.UnansweredNoReply {
					t.Errorf("unanswered reason = %q; want %q", entry.Reason, creelengine.UnansweredNoReply)
				}
			}
			for _, chase := range awaitChaseLines(t, e, alpha, 2) {
				for _, want := range []string{"from \"" + creel + "\"", "#1 from " + batten, "lym creel post --reply 1 "} {
					if !strings.Contains(chase, want) {
						t.Errorf("chase line %q lacks %q", chase, want)
					}
				}
				if strings.Contains(chase, "which schema?") {
					t.Errorf("chase line %q carries the sender's text", chase)
				}
			}
			notice := e.readBodies(t, program(batten), "")
			if len(notice) != 1 || !strings.Contains(notice[0], "#1") || !strings.Contains(notice[0], alpha) || !strings.Contains(notice[0], "lym creel log --id 1") {
				t.Errorf("sender notice = %q; want one naming message #1, the receiver and lym creel log --id 1", notice)
			}
		}},
		{"StrandSendersMessageReadThroughReadAndAckIsChasedTheSameWay", func(t *testing.T, e *env) {
			e.start(t)
			id := e.postMarked(t, strand(beta), alpha, "which schema?")
			e.readBodies(t, strand(alpha), "")
			e.tickToUnanswered(t, id)
			if got := e.countEvents(id, "chased"); got != 2 {
				t.Errorf("chased events = %d; want 2", got)
			}
			// The notice is a program's line, so the pane path types it into the pane-mail strand.
			awaitTypedWith(t, e, beta, "lym creel log --id 1")
		}},
		{"ARepliesStopTheStepsAfterItAndALateReplyIsStillDelivered", func(t *testing.T, e *env) {
			e.start(t)
			beforeFirst := e.postMarked(t, strand(beta), alpha, "answered before the first chase")
			betweenChases := e.postMarked(t, strand(beta), alpha, "answered after the first chase")
			never := e.postMarked(t, strand(beta), alpha, "never answered in time")
			e.readBodies(t, strand(alpha), "")
			e.chaseTicks(5)
			if _, err := e.store.Reply(strand(alpha), beforeFirst, "early"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			e.chaseTicks(11)
			if _, err := e.store.Reply(strand(alpha), betweenChases, "middle"); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			e.chaseTicks(28)
			e.chaseTickLast()
			e.awaitEvent(t, never, "unanswered")

			for _, tt := range []struct {
				name                   string
				id                     int64
				chases, unansweredWant int
			}{{"before the first chase", beforeFirst, 0, 0}, {"after the first chase", betweenChases, 1, 0}, {"never", never, 2, 1}} {
				if got := e.countEvents(tt.id, "chased"); got != tt.chases {
					t.Errorf("%s: chased events = %d; want %d", tt.name, got, tt.chases)
				}
				if got := e.countEvents(tt.id, "unanswered"); got != tt.unansweredWant {
					t.Errorf("%s: unanswered events = %d; want %d", tt.name, got, tt.unansweredWant)
				}
			}

			waiter, err := e.store.WaitReply(strand(beta), never)
			if err != nil {
				t.Fatalf("WaitReply: %v", err)
			}
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnanswered || outcome.Message.ID != never {
				t.Errorf("a wait registered after the outcome = %+v; want unanswered carrying message #%d", outcome, never)
			}
			if _, err := e.store.Reply(strand(alpha), never, "late"); err != nil {
				t.Fatalf("Reply after the outcome: %v", err)
			}
			if bodies := e.readBodies(t, strand(beta), ""); !slices.Contains(bodies, "late") {
				t.Errorf("beta's mail = %q; want the late reply still delivered", bodies)
			}
			again, err := e.store.WaitReply(strand(beta), never)
			if err != nil {
				t.Fatalf("WaitReply: %v", err)
			}
			if outcome := awaitOutcome(t, again); outcome.Kind != creelengine.OutcomeReply {
				t.Errorf("a wait registered after the late reply = %+v; want the reply", outcome)
			}
		}},
		{"AnUnreadMarkedMessageAndOneAProgramHoldsAreNeverChased", func(t *testing.T, e *env) {
			e.start(t)
			e.readBodies(t, program(batten), "")
			e.postMarked(t, strand(alpha), beta, "never read")
			e.postMarked(t, strand(alpha), batten, "held by a program")
			e.readBodies(t, program(batten), "")
			// Stopping waits for every goroutine, so a chaser a wake had started has armed its first wait by now.
			e.scheduler.Stop()
			var chaseWaits []time.Duration
			for _, wait := range e.clock.Waits() {
				if isChaseWait(wait) {
					chaseWaits = append(chaseWaits, wait)
				}
			}
			if len(chaseWaits) != 0 {
				t.Errorf("chase waits = %v; want no chaser", chaseWaits)
			}
		}},
		{"AChaseAgedRecordFollowsEachRunningTickAndAPauseStopsTheClockAndBacksTheWaitOff", func(t *testing.T, e *env) {
			e.start(t)
			id := e.postMarked(t, strand(beta), alpha, "question")
			e.readBodies(t, strand(alpha), "")
			e.chaseTick()
			if got, want := chaseAgedSeconds(t, e, id), []int64{60}; !slices.Equal(got, want) {
				t.Fatalf("chase-aged seconds after one running tick = %v; want %v", got, want)
			}
			e.del.SetPaused(alpha, true)
			e.chaseTicks(4)
			e.del.SetPaused(alpha, false)
			// The first running tick finds the clock stopped at the previous reading, so it charges nothing.
			e.chaseTick()
			if got := chaseAgedSeconds(t, e, id); !slices.Equal(got, []int64{60}) {
				t.Errorf("chase-aged seconds after the pause = %v; want no time charged while paused", got)
			}
			e.chaseTick()
			if got, want := chaseAgedSeconds(t, e, id), []int64{60, 120}; !slices.Equal(got, want) {
				t.Errorf("chase-aged seconds once the clock runs again = %v; want %v", got, want)
			}
			minutes := func(n ...int) []time.Duration {
				waits := make([]time.Duration, len(n))
				for i, v := range n {
					waits[i] = time.Duration(v) * time.Minute
				}
				return waits
			}
			if got, want := e.clock.AwaitWaitsWhere(0, isChaseWait), minutes(1, 1, 2, 4, 8, 8, 1, 1); !slices.Equal(got, want) {
				t.Errorf("chase waits = %v; want %v", got, want)
			}
		}},
		{"ADormantHolderAndAnotherSendersHoldStopTheClockWhileTheMessagesOwnSendersHoldDoesNot", func(t *testing.T, e *env) {
			e.start(t)
			own := e.postMarked(t, strand(orch), alpha, "from the holder of the hold")
			other := e.postMarked(t, strand(beta), alpha, "from another sender")
			e.readBodies(t, strand(alpha), "")
			if _, err := e.store.Hold(strand(orch), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.chaseTick()
			if got := chaseAgedSeconds(t, e, own); !slices.Equal(got, []int64{60}) {
				t.Errorf("chase-aged seconds of the hold owner's message = %v; want the clock to run", got)
			}
			if got := chaseAgedSeconds(t, e, other); len(got) != 0 {
				t.Errorf("chase-aged seconds of the other sender's message = %v; want the hold to stop its clock", got)
			}
			e.del.SetStrands(creelfake.DormantStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.chaseTicks(2)
			if got := chaseAgedSeconds(t, e, own); !slices.Equal(got, []int64{60}) {
				t.Errorf("chase-aged seconds of the hold owner's message while the holder is dormant = %v; want no time charged", got)
			}
		}},
		{"AReopenAfterAPauseCountsNoPausedTimeAndNeitherRepeatsNorSkipsAChase", func(t *testing.T, e *env) {
			e.start(t)
			id := e.postMarked(t, strand(beta), alpha, "question")
			e.readBodies(t, strand(alpha), "")
			e.chaseTicks(14)
			e.del.SetPaused(alpha, true)
			e.chaseTick()
			armed := len(e.clock.AwaitWaitsWhere(0, isChaseWait))
			e.reopen(t)
			// The restarted chaser has read the pause once its first wait is armed.
			e.clock.AwaitWaitsWhere(armed+1, isChaseWait)
			e.del.SetPaused(alpha, false)
			e.chaseTick()
			if got := e.countEvents(id, "chased"); got != 0 {
				t.Errorf("chased events after the first running tick of the restarted clock = %d; want none, since that tick charges nothing", got)
			}
			e.chaseTick()
			if got := e.countEvents(id, "chased"); got != 1 {
				t.Errorf("chased events after the second tick = %d; want chase 1 once, 840 seconds of running time plus 60", got)
			}
			e.chaseTicks(3)
			if got := e.countEvents(id, "chased"); got != 1 {
				t.Errorf("chased events after more ticks = %d; want the chase not repeated", got)
			}
		}},
		{"ASeededChasePostWithNoChasedRecordGetsItAtOpenAndYieldsNoThirdChase", func(t *testing.T, e *env) {
			e.writeJournal(t, "2026-10-01", seeded(
				`{"event":"posted","at":"2026-10-01T12:00:00Z","id":2,"from":"`+creel+`","to":"`+alpha+`","kind":"mail","body":"chase 1","chase":1,"chased":1,"consumed_s":900}`,
				`{"event":"chased","at":"2026-10-01T12:00:00Z","id":1,"to":"`+alpha+`","chase":1}`,
				`{"event":"posted","at":"2026-10-01T12:00:00Z","id":3,"from":"`+creel+`","to":"`+alpha+`","kind":"mail","body":"chase 2","chase":2,"chased":1,"consumed_s":1800}`,
				`{"event":"chase-aged","at":"2026-10-01T12:00:00Z","id":1,"consumed_s":1800}`,
			)...)
			e.reopen(t)
			var numbers []int
			for _, entry := range e.store.Log(creelengine.LogFilter{ID: 1}) {
				if entry.Event == "chased" {
					numbers = append(numbers, entry.Chase)
				}
			}
			if !slices.Equal(numbers, []int{1, 2}) {
				t.Errorf("chased entries after open = %v; want chase 2's record written", numbers)
			}
			e.chaseTicks(14)
			e.chaseTickLast()
			e.awaitEvent(t, 1, "unanswered")
			if got := e.countEvents(1, "chased"); got != 2 {
				t.Errorf("chased events = %d; want no third chase", got)
			}
		}},
		{"ASeededClockPastTheNextStepTakesThatStepAtTheFifteenthTickAfterTheRestartAndTheNextAtTheThirtieth", func(t *testing.T, e *env) {
			e.writeJournal(t, "2026-10-01", seeded(`{"event":"chase-aged","at":"2026-10-01T12:00:00Z","id":1,"consumed_s":2400}`)...)
			e.reopen(t)
			e.chaseTicks(14)
			if got := e.countEvents(1, "chased"); got != 0 {
				t.Errorf("chased events after 14 ticks = %d; want none", got)
			}
			e.chaseTick()
			if got := e.countEvents(1, "chased"); got != 1 {
				t.Errorf("chased events after 15 ticks = %d; want chase 1", got)
			}
			e.chaseTicks(14)
			if got := e.countEvents(1, "chased"); got != 1 {
				t.Errorf("chased events after 29 ticks = %d; want only chase 1", got)
			}
			e.chaseTick()
			if got := e.countEvents(1, "chased"); got != 2 {
				t.Errorf("chased events after 30 ticks = %d; want chase 2", got)
			}
		}},
		{"NoNoticeGoesToAProgramThatNeverReadOrToAStrandRemovedBeforeTheOutcomeAndTheirWaitsAreStillAnswered", func(t *testing.T, e *env) {
			// batten posted but never read, so its mailbox is known and still no notice is owed.
			programID, programWaiter, err := e.store.PostWait(program(batten), alpha, "from a program that never reads")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			strandID, strandWaiter, err := e.store.PostWait(strand(beta), alpha, "from a strand that leaves")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			e.readBodies(t, strand(alpha), "")
			e.chaseTicks(44)
			e.del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			e.chaseTickLast()
			for _, tt := range []struct {
				name   string
				id     int64
				waiter *creelengine.Waiter
			}{{"program", programID, programWaiter}, {"strand", strandID, strandWaiter}} {
				e.awaitEvent(t, tt.id, "unanswered")
				if outcome := awaitOutcome(t, tt.waiter); outcome.Kind != creelengine.OutcomeUnanswered {
					t.Errorf("%s: outcome = %+v; want unanswered", tt.name, outcome)
				}
			}
			if bodies := e.readBodies(t, program(batten), ""); len(bodies) != 0 {
				t.Errorf("batten's mail = %q; want no notice", bodies)
			}
			if bodies := e.readBodies(t, strand(beta), ""); len(bodies) != 0 {
				t.Errorf("beta's mail = %q; want no notice", bodies)
			}
		}},
		{"AnExternalSendersMessageIsChasedByItsBareNameWithNoSenderTextAndItsOutcomePostsNoNotice", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(external(), alpha, "SECRET question text")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			e.start(t)
			e.readBodies(t, strand(alpha), "")
			e.tickToUnanswered(t, id)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeUnanswered {
				t.Errorf("outcome = %+v; want unanswered", outcome)
			}
			chases := 0
			for _, entry := range e.store.Log(creelengine.LogFilter{From: creel}) {
				if entry.Event != "posted" {
					continue
				}
				chases++
				if entry.To != alpha {
					t.Errorf("a %s post went to %s; want only the two chases to %s and no notice", creel, entry.To, alpha)
				}
			}
			if chases != 2 {
				t.Errorf("%s posts = %d; want the two chases", creel, chases)
			}
			for _, line := range awaitChaseLines(t, e, alpha, 2) {
				if !strings.Contains(line, "from ly:EXTERNAL") {
					t.Errorf("chase line %q lacks the sender's bare name", line)
				}
				for _, leaked := range []string{"SECRET", "claude", "pts/4", "/home/me"} {
					if strings.Contains(line, leaked) {
						t.Errorf("chase line %q carries %q of the sender", line, leaked)
					}
				}
			}
		}},
		{"AReplyToAChaseAnswersTheChasedMessageAndStopsItsChasesWhileAnotherUnansweredOneRunsOn", func(t *testing.T, e *env) {
			e.start(t)
			answered := e.postMarked(t, strand(beta), alpha, "answered through its chase")
			never := e.postMarked(t, strand(beta), alpha, "never answered")
			e.readBodies(t, strand(alpha), "")
			e.chaseTicks(15)
			chase := creelPostTo(e, creel, alpha)
			replyID, err := e.store.Reply(strand(alpha), chase, "the answer")
			if err != nil {
				t.Fatalf("Reply to the chase: %v", err)
			}
			var reply *creelengine.LogEntry
			for _, entry := range e.store.Log(creelengine.LogFilter{ID: replyID}) {
				if entry.Event == "posted" {
					reply = &entry
				}
			}
			if reply == nil || reply.To != beta || reply.From != alpha || reply.ReplyTo != answered {
				t.Errorf("reply entry = %+v; want a post from %s to %s answering #%d", reply, alpha, beta, answered)
			}
			e.chaseTicks(14)
			e.chaseTick()
			if got := e.countEvents(answered, "chased"); got != 1 {
				t.Errorf("chased events of the answered message = %d; want only chase 1", got)
			}
			if got := e.countEvents(never, "chased"); got != 2 {
				t.Errorf("chased events of the unanswered message = %d; want both chases", got)
			}
			e.chaseTicks(14)
			e.chaseTickLast()
			e.awaitEvent(t, never, "unanswered")
			if got := e.countEvents(answered, "unanswered"); got != 0 {
				t.Errorf("unanswered events of the answered message = %d; want none", got)
			}
		}},
		{"AChaseTheHolderStillHoldsUnreadIsRefusedForwardingAndStaysWithItsHolder", func(t *testing.T, e *env) {
			e.start(t)
			e.postMarked(t, strand(orch), alpha, "question")
			e.readBodies(t, strand(alpha), "")
			// The question's sender holds alpha so its chase clock runs while the delivery worker leaves the chase unread.
			if _, err := e.store.Hold(strand(orch), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.chaseTicks(15)
			chase := creelPostTo(e, creel, alpha)
			before := len(e.store.Log(creelengine.LogFilter{}))
			wantErr(t, e.store.Forward(strand(alpha), chase, beta),
				fmt.Sprintf("message #%d is a chase and cannot be forwarded", chase),
				fmt.Sprintf("way forward: answer it with lym creel post --reply %d", chase), "which answers #1")
			if got := len(e.store.Log(creelengine.LogFilter{})); got != before {
				t.Errorf("journal entries after the refused forward = %d; want %d", got, before)
			}
			inbox, err := e.store.Inbox(strand(alpha))
			if err != nil {
				t.Fatalf("Inbox: %v", err)
			}
			if want := []creelengine.InboxEntry{{From: creel, Unread: 1}}; !slices.Equal(inbox.Senders, want) {
				t.Errorf("alpha's unread senders = %+v; want the chase still with its holder", inbox.Senders)
			}
		}},
		{"ASenderNoticeOfAnUnansweredOutcomeTakesNoReply", func(t *testing.T, e *env) {
			e.start(t)
			id := e.postMarked(t, strand(beta), alpha, "question")
			e.readBodies(t, strand(alpha), "")
			e.tickToUnanswered(t, id)
			notice := creelPostTo(e, creel, beta)
			before := len(e.store.Log(creelengine.LogFilter{}))
			_, err := e.store.Reply(strand(beta), notice, "thanks")
			wantErr(t, err, fmt.Sprintf("message #%d is a creel notice and takes no reply", notice), "way forward: post to the strand it names with lym creel post <strand>")
			if got := len(e.store.Log(creelengine.LogFilter{})); got != before {
				t.Errorf("journal entries after the refused reply = %d; want %d", got, before)
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

// TestStore_ClaimLog pins the daemon log line of each hook delivery attempt and each bell send: its level, path, ids, echo ids, outcome and reason.
// It is not parallel because it redirects the process-global logger.
func TestStore_ClaimLog(t *testing.T) {
	logs := logcapture.Capture(t)
	logger.SetVerbosity(2)
	bigBody := strings.Repeat("x", 5000)
	hookEnv := func(t *testing.T) *env {
		t.Helper()
		e := newEnv(t)
		e.del.SetStrands(creelfake.HookStrand(alpha), creelfake.HookStrand(beta), creelfake.LiveStrand(orch))
		return e
	}
	// settle runs trigger and waits until a worker has probed after it and gone back to sleep.
	settle := func(e *env, trigger func()) {
		seen := len(e.probeWaits(0))
		trigger()
		e.probeWaits(seen + 1)
	}
	registerTaking := func(t *testing.T, e *env) *creelengine.Waiter {
		t.Helper()
		waiter, err := e.store.HookWait(strand(alpha), true)
		if err != nil {
			t.Fatalf("HookWait: %v", err)
		}
		return waiter
	}
	// ring registers a marked waiter of alpha, posts one short message and returns the ring's claim.
	ring := func(t *testing.T, e *env) *creelengine.Reading {
		t.Helper()
		waiter := registerTaking(t, e)
		e.post(t, strand(beta), alpha, "one")
		settle(e, func() { e.start(t) })
		outcome := awaitOutcome(t, waiter)
		if outcome.Reading == nil {
			t.Fatalf("the marked waiter was answered %+v; want a ring carrying a claim", outcome)
		}
		return outcome.Reading
	}
	claim := func(t *testing.T, e *env, boundary creelengine.Boundary) *creelengine.Reading {
		t.Helper()
		reading, err := e.store.ClaimAtBoundary(strand(alpha), boundary)
		if err != nil {
			t.Fatalf("ClaimAtBoundary: %v", err)
		}
		return reading
	}
	// owedEcho acknowledges a ring of one message, leaving its line owed as an echo, and clears the log of the ring's own line.
	owedEcho := func(t *testing.T, e *env) {
		t.Helper()
		if err := ring(t, e).Ack(); err != nil {
			t.Fatalf("Ack of the ring: %v", err)
		}
		logs.Reset()
	}
	// untilBellAttempt steps probes until a bell was sent into alpha; the fake-time minute only bounds a failing test.
	untilBellAttempt := func(t *testing.T, e *env) {
		t.Helper()
		start := e.clock.Now()
		for e.clock.Now().Sub(start) < time.Minute {
			waits := e.probeWaits(1)
			e.tick(waits[len(waits)-1])
			if slices.Contains(e.typed(alpha), creelengine.BellLine) {
				return
			}
		}
		t.Fatal("no bell was sent within a minute of fake time")
	}
	postBellSet := func(t *testing.T, e *env) {
		t.Helper()
		e.start(t)
		settle(e, func() { e.post(t, strand(beta), alpha, "one") })
		untilBellAttempt(t, e)
	}
	failure := errors.New("pane exploded")
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
		// msg is the one log message the row expects exactly once, and fields what its line carries, the level included.
		msg    string
		fields map[string]string
	}{
		{"AToolBoundaryClaimAcknowledged", func(t *testing.T, e *env) {
			e.post(t, strand(beta), alpha, "one")
			if err := claim(t, e, creelengine.Boundary{}).Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
		}, "creel claim", map[string]string{"level": "INFO", "strand": alpha, "path": "tool", "ids": "[1]", "echo_ids": "[]", "outcome": "acknowledged"}},
		{"AStopClaimAcknowledged", func(t *testing.T, e *env) {
			e.post(t, strand(beta), alpha, "one")
			if err := claim(t, e, creelengine.Boundary{Stop: true}).Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
		}, "creel claim", map[string]string{"level": "INFO", "path": "stop", "ids": "[1]", "echo_ids": "[]", "outcome": "acknowledged"}},
		{"AClaimReleased", func(t *testing.T, e *env) {
			e.post(t, strand(beta), alpha, "one")
			claim(t, e, creelengine.Boundary{}).Release()
		}, "creel claim", map[string]string{"level": "INFO", "path": "tool", "ids": "[1]", "outcome": "released"}},
		{"AnEmptyClaimReleasedLogsTheNothingLineAlone", func(t *testing.T, e *env) {
			claim(t, e, creelengine.Boundary{}).Release()
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "tool", "outcome": "nothing", "reason": "nothing-claimable"}},
		{"AClaimWhoseAckFailsNamesTheReleasedIdsAndTheDroppedEcho", func(t *testing.T, e *env) {
			owedEcho(t, e)
			settle(e, func() { e.post(t, strand(beta), alpha, "two") })
			e.post(t, strand(beta), alpha, "three")
			reading := claim(t, e, creelengine.Boundary{})
			e.store.FailJournalWrites(0)
			if err := reading.Ack(); err == nil {
				t.Fatal("Ack with the journal write failing returned nil; want an error")
			}
		}, "creel claim", map[string]string{
			"level": "WARN", "path": "tool", "ids": "[2 3]", "echo_ids": "[1]", "outcome": "ack-failed",
			"released_ids": "[2 3]", "dropped_echo_ids": "[1]", "error": "creel journal: injected write fault",
		}},
		{"ARingWhoseAckFailsNamesTheReleasedIdsAndNoEcho", func(t *testing.T, e *env) {
			reading := ring(t, e)
			e.store.FailJournalWrites(0)
			if err := reading.Ack(); err == nil {
				t.Fatal("Ack with the journal write failing returned nil; want an error")
			}
		}, "creel claim", map[string]string{
			"level": "WARN", "path": "ring", "ids": "[1]", "echo_ids": "[]", "outcome": "ack-failed",
			"released_ids": "[1]", "dropped_echo_ids": "[]",
		}},
		{"AClaimHandingOutAnEchoIsAcknowledgedWithItsEchoIds", func(t *testing.T, e *env) {
			owedEcho(t, e)
			if err := claim(t, e, creelengine.Boundary{}).Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
		}, "creel claim", map[string]string{"level": "INFO", "path": "tool", "ids": "[]", "echo_ids": "[1]", "outcome": "acknowledged"}},
		{"ARingAcknowledged", func(t *testing.T, e *env) {
			if err := ring(t, e).Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
		}, "creel claim", map[string]string{"level": "INFO", "path": "ring", "ids": "[1]", "echo_ids": "[]", "outcome": "acknowledged"}},
		{"ARingReleased", func(t *testing.T, e *env) {
			ring(t, e).Release()
		}, "creel claim", map[string]string{"level": "INFO", "path": "ring", "ids": "[1]", "outcome": "released"}},
		{"APausedRingLogsTheNothingLineWithThePathRing", func(t *testing.T, e *env) {
			e.start(t)
			registerTaking(t, e)
			e.del.SetIdleHook(func(string) { e.del.SetPaused(alpha, true) })
			settle(e, func() { e.post(t, strand(beta), alpha, "one") })
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "ring", "outcome": "nothing", "reason": "paused"}},
		{"APausedStrandsClaimHandsOutNothing", func(t *testing.T, e *env) {
			e.post(t, strand(beta), alpha, "one")
			e.del.SetPaused(alpha, true)
			claim(t, e, creelengine.Boundary{})
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "tool", "outcome": "nothing", "reason": "paused"}},
		{"AnotherSendersHoldKeepsTheClaimFromMailThatIsNotTheOwners", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.post(t, strand(beta), alpha, "not the owner's")
			claim(t, e, creelengine.Boundary{})
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "tool", "outcome": "nothing", "reason": "held"}},
		{"TheStopCapHandsOutNothing", func(t *testing.T, e *env) {
			for range 8 {
				e.post(t, strand(beta), alpha, bigBody)
			}
			for i := range 6 {
				if err := claim(t, e, creelengine.Boundary{Stop: true, StopHookActive: i > 0}).Ack(); err != nil {
					t.Fatalf("Ack of Stop claim %d: %v", i+1, err)
				}
			}
			logs.Reset()
			claim(t, e, creelengine.Boundary{Stop: true, StopHookActive: true})
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "stop", "outcome": "nothing", "reason": "stop-cap"}},
		{"AStopCountNoClaimSetHandsOutNothing", func(t *testing.T, e *env) {
			e.post(t, strand(beta), alpha, "one")
			claim(t, e, creelengine.Boundary{Stop: true, StopHookActive: true})
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "stop", "outcome": "nothing", "reason": "stop-count-unknown"}},
		{"NoMailHandsOutNothing", func(t *testing.T, e *env) {
			claim(t, e, creelengine.Boundary{})
		}, "creel claim", map[string]string{"level": "DEBUG", "path": "tool", "outcome": "nothing", "reason": "nothing-claimable"}},
		{"ABellSentAfterTheGraceWithNoWaiter", func(t *testing.T, e *env) {
			postBellSet(t, e)
		}, "creel bell", map[string]string{"level": "INFO", "strand": alpha, "ids": "[1]", "outcome": "sent"}},
		{"ABellWhoseSendDidNotLand", func(t *testing.T, e *env) {
			e.del.SetSendError(creelengine.ErrNotLanded)
			postBellSet(t, e)
		}, "creel bell", map[string]string{"level": "INFO", "ids": "[1]", "outcome": "not-landed"}},
		{"ABusyBell", func(t *testing.T, e *env) {
			e.del.SetSendError(creelengine.ErrBusy)
			postBellSet(t, e)
		}, "creel bell", map[string]string{"level": "INFO", "ids": "[1]", "outcome": "busy"}},
		{"AFailedBellNamesItsError", func(t *testing.T, e *env) {
			e.del.SetSendError(failure)
			postBellSet(t, e)
		}, "creel bell", map[string]string{"level": "INFO", "ids": "[1]", "outcome": "failed", "error": failure.Error()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			tt.run(t, hookEnv(t))
			lines := logLinesWithMessage(logs.String(), tt.msg)
			if len(lines) != 1 {
				t.Fatalf("%d %q lines; want exactly one:\n%s", len(lines), tt.msg, strings.Join(lines, "\n"))
			}
			for key, want := range tt.fields {
				if got := logField(lines[0], key); got != want {
					t.Errorf("%s of %q = %q; want %q", key, lines[0], got, want)
				}
			}
		})
	}
}

// logLinesWithMessage returns the lines of text, logger output in its text form, whose message is msg.
func logLinesWithMessage(text, msg string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, `msg="`+msg+`"`) {
			lines = append(lines, line)
		}
	}
	return lines
}

// logField returns the value of key in a logger text line, unquoted, and empty when the line carries no such key.
func logField(line, key string) string {
	_, rest, found := strings.Cut(" "+line, " "+key+"=")
	if !found {
		return ""
	}
	if strings.HasPrefix(rest, `"`) {
		quoted, _, _ := strings.Cut(rest[1:], `"`)
		return quoted
	}
	value, _, _ := strings.Cut(rest, " ")
	return value
}
