// verbs_test.go pins the pool's verbs over the fake delivery seam: mail, replies, reads, forwards, the log, injects, interrupts and holds.

package creelengine_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
)

const (
	alpha  = "ly:alpha:driver"
	beta   = "ly:beta:driver"
	orch   = "ly:orch"
	orch2  = "ly:orch-2"
	batten = "ly:BATTEN"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// env is a store over a fake delivery of four strands and a fake clock.
type env struct {
	store *creelengine.Store
	del   *creelfake.Delivery
	clock *creelfake.Clock
	geom  creelengine.Geometry
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	geom := creelengine.Geometry{
		HubPath: filepath.Join(root, "hub"), Shortname: "ly", StateDir: filepath.Join(root, "state"),
		JournalDir: filepath.Join(root, "state", "journal"),
	}
	del := creelfake.NewDelivery()
	del.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
	clock := creelfake.NewClock(epoch)
	store, err := creelengine.OpenStore(geom, del, clock)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &env{store: store, del: del, clock: clock, geom: geom}
}

func strand(name string) creelengine.Sender { return creelengine.StrandSender(name) }

func program(name string) creelengine.Sender { return creelengine.ProgramSender(name) }

func external() creelengine.Sender {
	sender, err := creelengine.ExternalSender("ly", "pts/4", "claude", "/home/me/Code/x")
	if err != nil {
		panic(err)
	}
	return sender
}

func (e *env) post(t *testing.T, from creelengine.Sender, to, body string) int64 {
	t.Helper()
	id, err := e.store.Post(from, to, body)
	if err != nil {
		t.Fatalf("Post(%s -> %s): %v", from.Name, to, err)
	}
	return id
}

// readBodies reads as the sender, acks, and returns the bodies handed out.
func (e *env) readBodies(t *testing.T, as creelengine.Sender, from string, all bool) []string {
	t.Helper()
	reading, err := e.store.Read(as, from, all)
	if err != nil {
		t.Fatalf("Read(%s): %v", as.Name, err)
	}
	if err := reading.Ack(); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	var bodies []string
	for _, msg := range reading.Messages {
		bodies = append(bodies, msg.Body)
	}
	return bodies
}

func wantErr(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error; want one containing %q", parts)
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q lacks %q", err, part)
		}
	}
}

func wantBodies(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("bodies = %q; want %q", got, want)
	}
}

func eventsOf(entries []creelengine.LogEntry) []string {
	var events []string
	for _, entry := range entries {
		events = append(events, entry.Event)
	}
	return events
}

func TestStore_Mail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"Post_StoredWithIdAndWakesReceiver", func(t *testing.T, e *env) {
			var woken []string
			e.store.SetWakeHook(func(receiver string) { woken = append(woken, receiver) })
			first := e.post(t, strand(alpha), beta, "hello")
			second := e.post(t, strand(alpha), beta, "again")
			if first <= 0 || second <= first {
				t.Errorf("ids = %d, %d; want increasing positive ids", first, second)
			}
			if !reflect.DeepEqual(woken, []string{beta, beta}) {
				t.Errorf("wake hook saw %v; want beta twice", woken)
			}
			wantBodies(t, e.readBodies(t, strand(beta), "", false), "hello", "again")
		}},
		{"Post_RefusesEmptyAndOversizedBody", func(t *testing.T, e *env) {
			_, err := e.store.Post(strand(alpha), beta, "  ")
			wantErr(t, err, "empty")
			_, err = e.store.Post(strand(alpha), beta, strings.Repeat("x", 64<<10+1))
			wantErr(t, err, "write it to a file", "mail the file")
			if _, err := e.store.Post(strand(alpha), beta, strings.Repeat("x", 64<<10)); err != nil {
				t.Errorf("a body of exactly 64 KiB: %v", err)
			}
		}},
		{"Post_RefusesUnknownReceiverListingNames", func(t *testing.T, e *env) {
			_, err := e.store.Post(strand(alpha), "ly:nobody:driver", "hi")
			wantErr(t, err, "unknown receiver", alpha, beta, orch)
		}},
		{"Post_RefusesExternalReceiver", func(t *testing.T, e *env) {
			_, err := e.store.Post(strand(alpha), "ly:EXTERNAL", "hi")
			wantErr(t, err, "--wait-reply", "--reply")
		}},
		{"Post_FailsNamingUnreadableStrands", func(t *testing.T, e *env) {
			e.del.SetStrandsError(errors.New("reed state of /hub/x is unreadable"))
			_, err := e.store.Post(strand(alpha), beta, "hi")
			wantErr(t, err, "reed state of /hub/x is unreadable")
			if strings.Contains(err.Error(), "unknown receiver") {
				t.Errorf("error %q reports an unknown receiver instead of the unreadable state", err)
			}
		}},
		{"Post_ProgramMailboxKnownAfterFirstPost", func(t *testing.T, e *env) {
			_, err := e.store.Post(strand(alpha), batten, "early")
			wantErr(t, err, "unknown receiver")
			e.post(t, program(batten), alpha, "from the program")
			e.post(t, strand(alpha), batten, "reply by mail")
			wantBodies(t, e.readBodies(t, program(batten), "", false), "reply by mail")
		}},
		{"Read_ProgramMailboxKnownAfterFirstRead", func(t *testing.T, e *env) {
			wantBodies(t, e.readBodies(t, program(batten), "", false))
			e.post(t, strand(alpha), batten, "now known")
			wantBodies(t, e.readBodies(t, program(batten), "", false), "now known")
		}},
		{"Read_SetsAndInbox", func(t *testing.T, e *env) {
			e.post(t, program(batten), beta, "b1")
			e.post(t, strand(alpha), beta, "a1")
			e.post(t, program(batten), beta, "b2")
			e.post(t, strand(alpha), beta, "a2")
			e.post(t, strand(alpha), beta, "a3")
			inbox, err := e.store.Inbox(strand(beta))
			if err != nil {
				t.Fatalf("Inbox: %v", err)
			}
			wantInbox := []creelengine.InboxEntry{{From: batten, Unread: 2}, {From: alpha, Unread: 3}}
			if !reflect.DeepEqual(inbox.Senders, wantInbox) {
				t.Errorf("Inbox = %+v; want senders %v", inbox, wantInbox)
			}
			wantBodies(t, e.readBodies(t, strand(beta), "", false), "b1", "b2")
			inbox, err = e.store.Inbox(strand(beta))
			wantInbox = []creelengine.InboxEntry{{From: alpha, Unread: 3}}
			if err != nil || !reflect.DeepEqual(inbox.Senders, wantInbox) {
				t.Errorf("Inbox after the first read = %+v, %v; want senders %v", inbox, err, wantInbox)
			}
			wantBodies(t, e.readBodies(t, strand(beta), "", false), "a1", "a2", "a3")
			wantBodies(t, e.readBodies(t, strand(beta), "", false))
			e.post(t, strand(alpha), beta, "a4")
			e.post(t, program(batten), beta, "b3")
			e.post(t, strand(alpha), beta, "a5")
			wantBodies(t, e.readBodies(t, strand(beta), batten, false), "b3")
			e.post(t, program(batten), beta, "b4")
			wantBodies(t, e.readBodies(t, strand(beta), batten, true), "b4")
			e.post(t, program(batten), beta, "b5")
			wantBodies(t, e.readBodies(t, strand(beta), "", true), "a4", "a5", "b5")
			wantBodies(t, e.readBodies(t, strand(beta), "", true))
			inbox, err = e.store.Inbox(strand(beta))
			if err != nil || len(inbox.Senders) != 0 {
				t.Errorf("Inbox after reading everything = %+v, %v; want an empty set", inbox, err)
			}
		}},
		{"Inbox_OrchCallerSeesPlainCounts", func(t *testing.T, e *env) {
			for _, name := range []string{orch, orch2} {
				e.post(t, strand(alpha), name, "one")
				inbox, err := e.store.Inbox(strand(name))
				if want := []creelengine.InboxEntry{{From: alpha, Unread: 1}}; err != nil || !reflect.DeepEqual(inbox, creelengine.InboxResult{Senders: want}) {
					t.Errorf("Inbox(%s) = %+v, %v; want only the senders %v", name, inbox, err, want)
				}
			}
		}},
		{"Read_ReservesUntilAckedOrReleased", func(t *testing.T, e *env) {
			e.post(t, strand(alpha), beta, "one")
			first, err := e.store.Read(strand(beta), "", false)
			if err != nil || len(first.Messages) != 1 {
				t.Fatalf("first Read = %+v, %v; want one message", first, err)
			}
			second, err := e.store.Read(strand(beta), "", false)
			if err != nil || len(second.Messages) != 0 {
				t.Errorf("a read while the first is un-acked = %+v, %v; want none", second, err)
			}
			first.Release()
			again, err := e.store.Read(strand(beta), "", false)
			if err != nil || len(again.Messages) != 1 {
				t.Fatalf("Read after Release = %+v, %v; want the message back", again, err)
			}
			if err := again.Ack(); err != nil {
				t.Fatalf("Ack: %v", err)
			}
			wantBodies(t, e.readBodies(t, strand(beta), "", false))
		}},
		{"Read_RefusesExternalCaller", func(t *testing.T, e *env) {
			_, err := e.store.Read(external(), "", false)
			wantErr(t, err, "--wait-reply")
			_, err = e.store.Inbox(external())
			wantErr(t, err, "--wait-reply")
		}},
		{"Forward_ChangesHolderAndPath", func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "route me")
			if err := e.store.Forward(strand(beta), id, orch); err != nil {
				t.Fatalf("Forward: %v", err)
			}
			wantBodies(t, e.readBodies(t, strand(beta), "", false))
			reading, err := e.store.Read(strand(orch), "", false)
			if err != nil || len(reading.Messages) != 1 {
				t.Fatalf("Read as the new holder = %+v, %v; want the message", reading, err)
			}
			msg := reading.Messages[0]
			if msg.From != alpha || msg.Holder != orch || !reflect.DeepEqual(msg.Path, []string{beta}) {
				t.Errorf("forwarded message = from %q holder %q path %v; want from %q holder %q path [%s]", msg.From, msg.Holder, msg.Path, alpha, orch, beta)
			}
		}},
		{"Forward_RefusesWhatItShould", func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "mine")
			wantErr(t, e.store.Forward(strand(alpha), id, orch), "do not hold")
			wantErr(t, e.store.Forward(strand(beta), id, beta), "yourself")
			wantErr(t, e.store.Forward(strand(beta), id, "ly:EXTERNAL"), "--wait-reply")
			wantErr(t, e.store.Forward(strand(beta), id, "ly:nobody:driver"), "unknown receiver")
			wantErr(t, e.store.Forward(strand(beta), 999, orch), "no longer")
			e.readBodies(t, strand(beta), "", false)
			wantErr(t, e.store.Forward(strand(beta), id, orch), "already read")
		}},
		{"Reply_ToAStrandIsMail", func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "question")
			e.readBodies(t, strand(beta), "", false)
			replyID, err := e.store.Reply(strand(beta), id, "answer")
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			reading, err := e.store.Read(strand(alpha), "", false)
			if err != nil || len(reading.Messages) != 1 {
				t.Fatalf("Read as the original sender = %+v, %v; want the reply", reading, err)
			}
			msg := reading.Messages[0]
			if msg.ID != replyID || msg.Body != "answer" || msg.From != beta || msg.ReplyTo != id {
				t.Errorf("reply = %+v; want #%d \"answer\" from %s answering #%d", msg, replyID, beta, id)
			}
		}},
		{"Reply_RefusesUnknownIdAndNonHolder", func(t *testing.T, e *env) {
			id := e.post(t, strand(alpha), beta, "question")
			_, err := e.store.Reply(strand(beta), 999, "answer")
			wantErr(t, err, "no longer")
			_, err = e.store.Reply(strand(orch), id, "answer")
			wantErr(t, err, "neither hold nor have read")
		}},
		{"Reply_ToExternalGoesToTheRegisteredWaiter", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(external(), alpha, "question")
			if err != nil {
				t.Fatalf("PostWait: %v", err)
			}
			replyID, err := e.store.Reply(strand(alpha), id, "answer")
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			select {
			case outcome := <-waiter.C:
				if outcome.Kind != creelengine.OutcomeReply || outcome.Message.Body != "answer" || outcome.Message.ID != replyID {
					t.Errorf("outcome = %+v; want the reply #%d handed over", outcome, replyID)
				}
			default:
				t.Fatal("the waiting post was not answered")
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: replyID})); !slices.Equal(got, []string{"posted", "delivered"}) {
				t.Errorf("events of the reply = %v; want posted, delivered", got)
			}
		}},
		{"Reply_ToExternalWithNoWaiterIsUndelivered", func(t *testing.T, e *env) {
			id := e.post(t, external(), alpha, "question")
			replyID, err := e.store.Reply(strand(alpha), id, "answer")
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: replyID})); !slices.Equal(got, []string{"posted", "undelivered"}) {
				t.Errorf("events of the reply = %v; want posted, undelivered", got)
			}
		}},
		{"Reply_ToOrchJournalsPostedOnly", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), beta, "question")
			replyID, err := e.store.Reply(strand(beta), id, "answer")
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: replyID})); !slices.Equal(got, []string{"posted"}) {
				t.Errorf("events of the reply = %v; want posted only", got)
			}
		}},
		{"Post_ToOrchJournalsPostedOnly", func(t *testing.T, e *env) {
			for _, name := range []string{orch, orch2} {
				id := e.post(t, strand(alpha), name, "for the orch")
				if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, []string{"posted"}) {
					t.Errorf("events of the post to %s = %v; want posted only", name, got)
				}
			}
		}},
		{"Log_FiltersAndNeverShowsABody", func(t *testing.T, e *env) {
			first := e.post(t, strand(alpha), beta, "subject line\nsecret body")
			e.clock.Advance(time.Hour)
			cut := e.clock.Now()
			second := e.post(t, strand(beta), alpha, "later")
			third := e.post(t, program(batten), beta, "from the program")
			ids := func(entries []creelengine.LogEntry) []int64 {
				var got []int64
				for _, entry := range entries {
					got = append(got, entry.ID)
				}
				return got
			}
			filters := []struct {
				name   string
				filter creelengine.LogFilter
				want   []int64
			}{
				{"id", creelengine.LogFilter{ID: second}, []int64{second}},
				{"from", creelengine.LogFilter{From: alpha}, []int64{first}},
				{"to", creelengine.LogFilter{To: beta}, []int64{first, third}},
				{"since", creelengine.LogFilter{Since: cut}, []int64{second, third}},
				{"all", creelengine.LogFilter{}, []int64{first, second, third}},
			}
			for _, tt := range filters {
				if got := ids(e.store.Log(tt.filter)); !slices.Equal(got, tt.want) {
					t.Errorf("Log by %s = ids %v; want %v", tt.name, got, tt.want)
				}
			}
			entries := e.store.Log(creelengine.LogFilter{ID: first})
			if entries[0].Subject != "subject line" || strings.Contains(fmt.Sprintf("%+v", entries), "secret") {
				t.Errorf("log line = %+v; want the subject and no body", entries[0])
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

func TestStore_InjectAndHold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"Inject_PrivilegedIntoAnotherStrandIsQueued", func(t *testing.T, e *env) {
			for _, sender := range []creelengine.Sender{program(batten), external(), strand(orch)} {
				result, err := e.store.Inject(sender, alpha, "/clear", 0, false)
				if err != nil || result.ID == 0 || result.Interrupted {
					t.Errorf("Inject by %s = %+v, %v; want a queued inject", sender.Name, result, err)
				}
			}
			if queued := e.store.Queued(alpha); len(queued) != 3 || queued[0].Text != "/clear" {
				t.Errorf("Queued = %+v; want three injects of /clear", queued)
			}
			if sends := e.del.Sends(); len(sends) != 0 {
				t.Errorf("Inject typed %v; the store only queues", sends)
			}
		}},
		{"Inject_NonPrivilegedNeedsPost", func(t *testing.T, e *env) {
			_, err := e.store.Inject(strand(beta), alpha, "/clear", 0, false)
			wantErr(t, err, "lyx creel post")
			_, err = e.store.Interrupt(strand(beta), alpha)
			wantErr(t, err, "lyx creel post")
			_, err = e.store.Hold(strand(beta), alpha, time.Minute)
			wantErr(t, err, "lyx creel post")
			if queued := e.store.Queued(alpha); len(queued) != 0 || len(e.del.Interrupts()) != 0 {
				t.Errorf("a refusal left queued %v and interrupts %v", queued, e.del.Interrupts())
			}
		}},
		{"Inject_SelfInjectPasses", func(t *testing.T, e *env) {
			for _, name := range []string{beta, orch, orch2} {
				if _, err := e.store.Inject(strand(name), name, "/compact", 0, false); err != nil {
					t.Errorf("self-inject by %s: %v", name, err)
				}
			}
		}},
		{"Inject_AndInterruptIntoOrchSucceed", func(t *testing.T, e *env) {
			for _, name := range []string{orch, orch2} {
				for _, sender := range []creelengine.Sender{program(batten), external(), strand(orch2)} {
					if sender.Name == name {
						continue
					}
					if _, err := e.store.Inject(sender, name, "/clear", 0, false); err != nil {
						t.Errorf("Inject by %s into %s: %v", sender.Name, name, err)
					}
					if _, err := e.store.Interrupt(sender, name); err != nil {
						t.Errorf("Interrupt by %s into %s: %v", sender.Name, name, err)
					}
				}
			}
			if interrupts := e.del.Interrupts(); len(interrupts) == 0 {
				t.Errorf("no interrupt reached the orch strands")
			}
		}},
		{"Inject_RefusesControlCharactersAndBadTTL", func(t *testing.T, e *env) {
			_, err := e.store.Inject(program(batten), alpha, "two\nlines", 0, false)
			wantErr(t, err, "single line")
			_, err = e.store.Inject(program(batten), alpha, "tab\there", 0, false)
			wantErr(t, err, "single line")
			_, err = e.store.Inject(program(batten), alpha, "/clear", time.Hour+time.Second, false)
			wantErr(t, err, "at most")
			if _, err := e.store.Inject(program(batten), alpha, "/clear", time.Hour, false); err != nil {
				t.Errorf("a TTL of exactly 1 hour: %v", err)
			}
			if _, err := e.store.Inject(program(batten), alpha, "/clear", 0, false); err != nil {
				t.Fatalf("Inject with no TTL: %v", err)
			}
			queued := e.store.Queued(alpha)
			if len(queued) != 2 || queued[0].TTL != time.Hour || queued[1].TTL != 10*time.Minute {
				t.Errorf("queued TTLs = %+v; want 1 hour then the 10-minute default", queued)
			}
		}},
		{"Interrupt_RefusedUnderPauseOrAnUnreadablePauseRecord", func(t *testing.T, e *env) {
			e.del.SetPaused(alpha, true)
			_, err := e.store.Interrupt(program(batten), alpha)
			wantErr(t, err, "pause")
			_, err = e.store.Inject(program(batten), alpha, "/clear", 0, true)
			wantErr(t, err, "pause")
			e.del.SetPaused(alpha, false)
			e.del.SetPausedError(errors.New("pause record is corrupt"))
			_, err = e.store.Interrupt(program(batten), alpha)
			wantErr(t, err, "pause record is corrupt")
			_, err = e.store.Inject(program(batten), alpha, "/clear", 0, true)
			wantErr(t, err, "pause record is corrupt")
			if queued := e.store.Queued(alpha); len(queued) != 0 || len(e.del.Interrupts()) != 0 || len(e.del.Sends()) != 0 {
				t.Errorf("refusals left queued %v, interrupts %v, sends %v", queued, e.del.Interrupts(), e.del.Sends())
			}
		}},
		{"Interrupt_UnderAHoldOnlyTheOwnerPasses", func(t *testing.T, e *env) {
			held, err := e.store.Hold(program(batten), alpha, 30*time.Minute)
			if err != nil {
				t.Fatalf("Hold: %v", err)
			}
			expiry := held.Expires.UTC().Format(time.RFC3339)
			_, err = e.store.Interrupt(external(), alpha)
			wantErr(t, err, batten, expiry)
			_, err = e.store.Inject(external(), alpha, "/clear", 0, true)
			wantErr(t, err, batten, expiry)
			if queued := e.store.Queued(alpha); len(queued) != 0 || len(e.del.Interrupts()) != 0 {
				t.Errorf("refusals left queued %v and interrupts %v", queued, e.del.Interrupts())
			}
			sent, err := e.store.Interrupt(program(batten), alpha)
			if err != nil || !sent || !slices.Equal(e.del.Interrupts(), []string{alpha}) {
				t.Errorf("owner's Interrupt = %v, %v with interrupts %v; want sent", sent, err, e.del.Interrupts())
			}
		}},
		{"Interrupt_NoRunningTurnSendsNothing", func(t *testing.T, e *env) {
			e.del.SetInterruptError(creelengine.ErrNoRunningTurn)
			sent, err := e.store.Interrupt(program(batten), alpha)
			if err != nil || sent {
				t.Errorf("Interrupt with no running turn = %v, %v; want nothing sent and no error", sent, err)
			}
			result, err := e.store.Inject(program(batten), alpha, "/clear", 0, true)
			if err != nil || result.Interrupted {
				t.Fatalf("Inject --interrupt with no running turn = %+v, %v; want queued and nothing interrupted", result, err)
			}
			if queued := e.store.Queued(alpha); len(queued) != 1 || !queued[0].Head {
				t.Errorf("Queued = %+v; want the text at the head", queued)
			}
		}},
		{"Inject_InterruptQueuesAtTheHeadAndFailsOnOtherErrors", func(t *testing.T, e *env) {
			if _, err := e.store.Inject(program(batten), alpha, "first", 0, false); err != nil {
				t.Fatalf("Inject: %v", err)
			}
			result, err := e.store.Inject(program(batten), alpha, "urgent", 0, true)
			if err != nil || !result.Interrupted {
				t.Fatalf("Inject --interrupt = %+v, %v; want the running turn interrupted", result, err)
			}
			if queued := e.store.Queued(alpha); len(queued) != 2 || queued[0].Text != "urgent" || queued[1].Text != "first" {
				t.Errorf("Queued = %+v; want urgent ahead of first", queued)
			}
			boom := errors.New("pane gone")
			e.del.SetInterruptError(boom)
			if _, err := e.store.Inject(program(batten), alpha, "never queued", 0, true); !errors.Is(err, boom) {
				t.Errorf("Inject --interrupt on a failing interrupt = %v; want %v", err, boom)
			}
			if queued := e.store.Queued(alpha); len(queued) != 2 {
				t.Errorf("Queued = %+v; the failed inject must queue nothing", queued)
			}
			e.del.SetInterruptError(fmt.Errorf("clear: %w", creelengine.ErrClearFailed))
			sent, err := e.store.Interrupt(program(batten), alpha)
			if err != nil || !sent {
				t.Errorf("Interrupt whose clear failed = %v, %v; want sent with no error", sent, err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{To: alpha})); !slices.Contains(got, "interrupted") {
				t.Errorf("events = %v; want an interrupted record for the sent key", got)
			}
			result, err = e.store.Inject(program(batten), alpha, "after a failed clear", 0, true)
			if err != nil || !result.Interrupted {
				t.Errorf("Inject --interrupt whose clear failed = %+v, %v; want interrupted", result, err)
			}
			if queued := e.store.Queued(alpha); len(queued) != 3 || queued[0].Text != "after a failed clear" {
				t.Errorf("Queued = %+v; want the inject queued at the head", queued)
			}
		}},
		{"Hold_RefusesWhatItShould", func(t *testing.T, e *env) {
			_, err := e.store.Hold(program(batten), alpha, time.Hour+time.Second)
			wantErr(t, err, "at most")
			held, err := e.store.Hold(program(batten), alpha, time.Hour)
			if err != nil || held.Renewed {
				t.Fatalf("Hold = %+v, %v; want a new hold", held, err)
			}
			_, err = e.store.Hold(external(), alpha, time.Minute)
			wantErr(t, err, batten, held.Expires.UTC().Format(time.RFC3339))
			_, err = e.store.Release(external(), alpha)
			wantErr(t, err, batten, held.Expires.UTC().Format(time.RFC3339))
		}},
		{"Hold_OwnerRenewsAndReleases", func(t *testing.T, e *env) {
			first, err := e.store.Hold(program(batten), alpha, 10*time.Minute)
			if err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.clock.Advance(time.Minute)
			renewed, err := e.store.Hold(program(batten), alpha, 10*time.Minute)
			if err != nil || !renewed.Renewed || !renewed.Expires.After(first.Expires) {
				t.Errorf("renewing Hold = %+v, %v; want a renewal ending after %v", renewed, err, first.Expires)
			}
			released, err := e.store.Release(program(batten), alpha)
			if err != nil || !released {
				t.Errorf("owner's Release = %v, %v; want released", released, err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{To: alpha})); !slices.Equal(got, []string{"held", "held", "released"}) {
				t.Errorf("events = %v; want held, held, released", got)
			}
		}},
		{"Release_WithNoHoldChangesNothing", func(t *testing.T, e *env) {
			released, err := e.store.Release(program(batten), alpha)
			if err != nil || released {
				t.Errorf("Release with no hold = %v, %v; want nothing held", released, err)
			}
			if entries := e.store.Log(creelengine.LogFilter{}); len(entries) != 0 {
				t.Errorf("Release with no hold journaled %v", entries)
			}
		}},
		{"Release_OfAnUnknownNameIsRefusedNamingTheStrandsThatExist", func(t *testing.T, e *env) {
			_, err := e.store.Release(program(batten), "ly:ghost:driver")
			wantErr(t, err, "unknown strand", alpha, beta)
			if entries := e.store.Log(creelengine.LogFilter{}); len(entries) != 0 {
				t.Errorf("a refused Release journaled %v", entries)
			}
		}},
		{"Release_EndsTheOwnersHoldOnAStrandThatIsGone", func(t *testing.T, e *env) {
			if _, err := e.store.Hold(program(batten), alpha, time.Hour); err != nil {
				t.Fatalf("Hold: %v", err)
			}
			e.del.SetStrands(creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
			released, err := e.store.Release(program(batten), alpha)
			if err != nil || !released {
				t.Errorf("Release on a removed strand = %v, %v; want the hold released", released, err)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{To: alpha})); !slices.Equal(got, []string{"held", "released"}) {
				t.Errorf("events = %v; want held, released", got)
			}
		}},
		{"Release_WithNoHoldAndAnUnreadableStrandListingReturnsTheReadError", func(t *testing.T, e *env) {
			e.del.SetStrandsError(errors.New("reed state is corrupt"))
			released, err := e.store.Release(program(batten), alpha)
			wantErr(t, err, "could not read the hub's strands", "reed state is corrupt")
			if released {
				t.Errorf("Release = true with the strand listing unreadable; want false")
			}
			if entries := e.store.Log(creelengine.LogFilter{}); len(entries) != 0 {
				t.Errorf("a failed Release journaled %v", entries)
			}
		}},
		{"Hold_OnOrchIsAProgramsAlone", func(t *testing.T, e *env) {
			for _, name := range []string{orch, orch2} {
				for _, sender := range []creelengine.Sender{external(), strand(orch), strand(orch2)} {
					_, err := e.store.Hold(sender, name, time.Minute)
					want := fmt.Sprintf("creel: only a program may hold an orch strand, and %s is not one; send %s mail with `lyx creel post`, or cycle the orch with `lyx orch distill` or `lyx orch refresh`", sender.Name, name)
					if err == nil || err.Error() != want {
						t.Errorf("Hold by %s on %s = %v; want the refusal %q", sender.Name, name, err, want)
					}
				}
				if got := eventsOf(e.store.Log(creelengine.LogFilter{To: name})); len(got) != 0 {
					t.Errorf("refused holds on %s journaled %v", name, got)
				}
				if _, err := e.store.Hold(program(batten), name, time.Minute); err != nil {
					t.Errorf("Hold on %s: %v", name, err)
				}
				if released, err := e.store.Release(program(batten), name); err != nil || !released {
					t.Errorf("Release on %s = %v, %v; want released", name, released, err)
				}
				if got := eventsOf(e.store.Log(creelengine.LogFilter{To: name})); !slices.Equal(got, []string{"held", "released"}) {
					t.Errorf("events for %s = %v; want held, released", name, got)
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
