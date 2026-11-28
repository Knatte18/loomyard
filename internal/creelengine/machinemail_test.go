// machinemail_test.go pins mail across machines inside creel.
// It covers the relay's take, report and settle verbs with the machine mail hour.
// It covers the deliver verb with its dedupe, its reply rules and the replies and forwards of the mail it stores.
// It covers the post to the Slack relay's program, which asks the bridge whether the relay would post it.

package creelengine_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// slack is the relay's program on the hub of the tests.
	slack = "ly:SLACK"
	// remoteOrch is the orch of the machine osl the tests exchange mail with.
	remoteOrch = "osl::zz:orch"
)

// machineMail is mail from sender with the sending machine's id, addressed to the orch of the hub.
func machineMail(sender string, sendingID int64, body string) creelengine.MachineMail {
	return creelengine.MachineMail{Sender: sender, Receiver: orch, SendingID: sendingID, Body: body, Posted: epoch}
}

// deliverMachine delivers mail as the relay and fails the test when it is refused.
func (e *env) deliverMachine(t *testing.T, mail creelengine.MachineMail) int64 {
	t.Helper()
	id, err := e.store.DeliverMachine(program(slack), mail)
	if err != nil {
		t.Fatalf("DeliverMachine(%s #%d): %v", mail.Sender, mail.SendingID, err)
	}
	return id
}

// noticeSubjects returns the first lines of the CREEL notices posted to receiver, oldest first.
func (e *env) noticeSubjects(receiver string) []string {
	var subjects []string
	for _, entry := range e.store.Log(creelengine.LogFilter{To: receiver}) {
		if entry.Event == "posted" && strings.HasSuffix(entry.From, ":"+creelengine.ChaserProgram) {
			subjects = append(subjects, entry.Subject)
		}
	}
	return subjects
}

// returnedReason returns the reason of the machine-returned entry of message id, and false when it has none.
func (e *env) returnedReason(id int64) (string, bool) {
	for _, entry := range e.store.Log(creelengine.LogFilter{ID: id}) {
		if entry.Event == "machine-returned" {
			return entry.Reason, true
		}
	}
	return "", false
}

// unreadOf returns the unread mail of holder without consuming it.
func (e *env) unreadOf(t *testing.T, holder string) []creelengine.Message {
	t.Helper()
	reading, err := e.store.Read(strand(holder), "")
	if err != nil {
		t.Fatalf("Read(%s): %v", holder, err)
	}
	reading.Release()
	return reading.Messages
}

// isHourWait picks the waits of the machine hour loop on the shared fake clock: no other loop waits over ten minutes.
func isHourWait(d time.Duration) bool { return d > 10*time.Minute }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_MachineOutbound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	relay := program(slack)
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"TakeAnswersTwoPendingMessagesOldestFirstWithTheirReplyTo", func(t *testing.T, e *env) {
			e.withJournal(t, `{"event":"machine-outbound","at":"2026-10-01T12:00:00Z","id":5,"from":"ly:orch","to":"osl::zz:orch",`+
				`"receiving_machine":"osl","kind":"reply","body":"the answer","reply_to":9}`)
			e.withBridge(creelfake.NewBridge("neve"))
			later := e.post(t, strand(orch), remoteOrch, "a question")
			taken, err := e.store.TakeMachineOutbound(relay)
			must(t, err)
			var ids, replyTos []int64
			for _, message := range taken {
				ids, replyTos = append(ids, message.ID), append(replyTos, message.ReplyTo)
				if message.From != orch || message.To != remoteOrch || message.ReceivingMachine != "osl" || !message.Posted.Equal(epoch) {
					t.Errorf("taken message %+v; want from %s to %s on osl posted at the epoch", message, orch, remoteOrch)
				}
			}
			if !slices.Equal(ids, []int64{5, later}) || !slices.Equal(replyTos, []int64{9, 0}) {
				t.Errorf("take answered ids %v with reply-tos %v; want [5 %d] with [9 0]", ids, replyTos, later)
			}
			if taken[1].Body != "a question" || taken[1].ExpectReply {
				t.Errorf("taken mail %+v; want the plain question", taken[1])
			}
		}},
		{"AnUnchangedHoldReasonReportedThreeTimesWritesOneRecordAndAChangedOneASecond", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), remoteOrch, "hold me")
			for range 3 {
				must(t, e.store.ReportMachineHold(relay, id, "peer-unapproved"))
			}
			if got := e.countEvents(id, "machine-held"); got != 1 {
				t.Errorf("machine-held records after three equal reports = %d; want 1", got)
			}
			must(t, e.store.ReportMachineHold(relay, id, "button-off"))
			if got := e.countEvents(id, "machine-held"); got != 2 {
				t.Errorf("machine-held records after a changed report = %d; want 2", got)
			}
			must(t, e.store.SettleMachineDelivered(relay, id, "1760000000.000100"))
			must(t, e.store.ReportMachineHold(relay, id, "peer-unapproved"))
			if got := e.countEvents(id, "machine-held"); got != 2 {
				t.Errorf("machine-held records after a report for a settled message = %d; want still 2", got)
			}
		}},
		{"ADeliveredMessageIsNotReturnedAtItsHourAndAReturnSettleForItWritesNothing", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), remoteOrch, "sent")
			must(t, e.store.SettleMachineDelivered(relay, id, "1760000000.000100"))
			must(t, e.store.SettleMachineReturned(relay, id, "channel_not_found"))
			e.clock.Advance(2 * time.Hour)
			taken, err := e.store.TakeMachineOutbound(relay)
			must(t, err)
			if len(taken) != 0 {
				t.Errorf("take after the hour answered %+v; want nothing for a delivered message", taken)
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, []string{"machine-outbound", "machine-delivered"}) {
				t.Errorf("events of the delivered message = %v; want only its post and its delivery", got)
			}
		}},
		{"WithTheSchedulerStartedAndNoTakeAMessageIsReturnedAtItsHourWithItsLastReportedReasonOrBridgeUnsettled", func(t *testing.T, e *env) {
			// Both posts come before the start, so the loop's first pass sees both and arms the one hour wait the test advances past.
			// A post after the start wakes another pass, which can straddle the advance and arm its floor wait past the hour.
			held, waiter, err := e.store.PostWait(strand(orch), remoteOrch, "first")
			must(t, err)
			must(t, e.store.ReportMachineHold(relay, held, "peer-unapproved"))
			bare := e.post(t, strand(orch), remoteOrch, "second")
			e.start(t)
			e.clock.AwaitWaitsWhere(1, isHourWait)
			e.clock.Advance(time.Hour)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeReturned {
				t.Errorf("the wait on the first message ended %q; want %q", outcome.Kind, creelengine.OutcomeReturned)
			}
			e.awaitEvent(t, bare, "machine-returned")
			if reason, _ := e.returnedReason(held); reason != "peer-unapproved" {
				t.Errorf("the held message returned with %q; want its last reported reason", reason)
			}
			if reason, _ := e.returnedReason(bare); reason != creelengine.MachineUnsettled {
				t.Errorf("the unreported message returned with %q; want %q", reason, creelengine.MachineUnsettled)
			}
			notices := e.noticeSubjects(orch)
			if len(notices) != 2 || !strings.Contains(notices[0], "peer-unapproved") || !strings.Contains(notices[0], "`lym approve osl`") ||
				!strings.Contains(notices[1], creelengine.MachineUnsettled) || !strings.Contains(notices[1], "slack relay runs") {
				t.Errorf("notices to the sender = %q; want one per message, the first naming `lym approve osl` and the second the slack button", notices)
			}
		}},
		{"APostToAnotherMachineStartsNoHubWorkerAtTheSchedulersStartOrOnThePost", func(t *testing.T, e *env) {
			hubs := e.withHubs()
			e.start(t)
			id := e.post(t, strand(orch), remoteOrch, "mail across machines")
			e.clock.AwaitWaitsWhere(1, isHourWait)
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, []string{"machine-outbound"}) {
				t.Errorf("events of the post = %v; want only its machine-outbound record, since a per-hub worker needs an outbound one", got)
			}
			if spawns := hubs.Spawns(zzHub); spawns != 0 {
				t.Errorf("the hub %s was started %d times by a post to another machine; want none", zzHub, spawns)
			}
		}},
		{"TwoMessagesDueTogetherAreReturnedInOnePassAndTheLoopNeverWaitsUnderASecond", func(t *testing.T, e *env) {
			e.start(t)
			first := e.post(t, strand(orch), remoteOrch, "first")
			e.clock.Advance(500 * time.Millisecond)
			second := e.post(t, strand(orch), remoteOrch, "second")
			e.clock.AwaitWaitsWhere(2, isHourWait)
			e.clock.Advance(time.Hour + time.Second)
			e.awaitEvent(t, first, "machine-returned")
			e.awaitEvent(t, second, "machine-returned")
			for _, wait := range e.clock.Waits() {
				if wait < time.Second {
					t.Errorf("a recorded wait of %v; want none under a second", wait)
				}
			}
		}},
		{"ATakeAfterTheHourReturnsTheMessageAndNeverAnswersIt", func(t *testing.T, e *env) {
			id, waiter, err := e.store.PostWait(strand(orch), remoteOrch, "too late")
			must(t, err)
			e.clock.Advance(time.Hour)
			taken, err := e.store.TakeMachineOutbound(relay)
			must(t, err)
			if len(taken) != 0 {
				t.Errorf("take at the hour answered %+v; want nothing", taken)
			}
			if reason, returned := e.returnedReason(id); !returned || reason != creelengine.MachineUnsettled {
				t.Errorf("the message returned = (%q, %v); want %q", reason, returned, creelengine.MachineUnsettled)
			}
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeReturned {
				t.Errorf("the wait ended %q; want %q", outcome.Kind, creelengine.OutcomeReturned)
			}
		}},
		{"ASettleDeliveredAfterTheReturnJournalsDeliveredAndTellsTheSenderOnce", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), remoteOrch, "late")
			e.clock.Advance(time.Hour)
			_, err := e.store.TakeMachineOutbound(relay)
			must(t, err)
			for range 2 {
				must(t, e.store.SettleMachineDelivered(relay, id, "1760000000.000100"))
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, []string{"machine-outbound", "machine-returned", "machine-delivered"}) {
				t.Errorf("events of the late message = %v; want its return and one delivery", got)
			}
			notices := e.noticeSubjects(orch)
			if len(notices) != 2 || !strings.Contains(notices[1], "did reach the other machine") {
				t.Errorf("notices to the sender = %q; want the return notice and one late notice", notices)
			}
		}},
		{"ASettleReturnedWithChannelNotFoundReturnsTheMessageWithThatReason", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), remoteOrch, "nowhere")
			must(t, e.store.SettleMachineReturned(relay, id, "channel_not_found"))
			if reason, returned := e.returnedReason(id); !returned || reason != "channel_not_found" {
				t.Errorf("the message returned = (%q, %v); want channel_not_found", reason, returned)
			}
		}},
		{"TakeReportAndSettleAreRefusedToEveryoneButTheBridgeProgram", func(t *testing.T, e *env) {
			id := e.post(t, strand(orch), remoteOrch, "mine")
			verbs := []func(creelengine.Sender) error{
				func(from creelengine.Sender) error { _, err := e.store.TakeMachineOutbound(from); return err },
				func(from creelengine.Sender) error { return e.store.ReportMachineHold(from, id, "peer-unapproved") },
				func(from creelengine.Sender) error { return e.store.SettleMachineDelivered(from, id, "1.1") },
				func(from creelengine.Sender) error {
					return e.store.SettleMachineReturned(from, id, "channel_not_found")
				},
			}
			for _, from := range []creelengine.Sender{strand(orch), program(batten), program("ly:zz:SLACK"), external()} {
				for _, verb := range verbs {
					wantErr(t, verb(from), "lym creel post", slack, from.Name)
				}
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{ID: id})); !slices.Equal(got, []string{"machine-outbound"}) {
				t.Errorf("events of the message = %v; want only its post, since every refused verb wrote nothing", got)
			}
		}},
		{"ReportAndSettleNamingAnUnknownIdAreRefusedNamingIt", func(t *testing.T, e *env) {
			wantErr(t, e.store.ReportMachineHold(relay, 999, "peer-unapproved"), "#999", "take again")
			wantErr(t, e.store.SettleMachineDelivered(relay, 999, "1.1"), "#999")
			wantErr(t, e.store.SettleMachineReturned(relay, 999, "channel_not_found"), "#999")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newEnv(t).withBridge(creelfake.NewBridge("neve")))
		})
	}
}

func TestStore_PostToTheSlackRelay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	refusal := errors.New("creel: the slack button of ly:orch is off")
	const (
		relayDown = slack + " is not running, so the message waits in its mail, and the relay posts it only if it starts within a day, " +
			"since it drops older mail; way forward: start it with lym slack start"
		// beamSlack is the relay program of a beam, which is no program of this hub's relay.
		beamSlack = "ly:zz:SLACK"
	)
	shown := func(live *bool) any {
		if live == nil {
			return nil
		}
		return *live
	}
	tests := []struct {
		name   string
		sender creelengine.Sender
		// receiver is the receiver of the post, or the sender of the mail the reply answers.
		receiver string
		// reply answers mail the receiver posted to the sender instead of posting to the receiver.
		reply bool
		// bridge tells the store a bridge; a store without one asks nothing.
		bridge    bool
		relayLive bool
		// refused makes the bridge refuse the sender.
		refused       bool
		wantErr       error
		wantLive      *bool
		wantDelivered *bool
		wantWarning   string
	}{
		{"AnOrchsPostWhileTheRelayRunsIsStoredAndLive", strand(orch), slack, false, true, true, false, nil, new(true), new(true), ""},
		{"AnOrchsPostWhileTheRelayIsDownIsStoredNotDeliveredWithAWarningNamingSlackStart", strand(orch), slack, false, true, false, false, nil, new(false), new(false), relayDown},
		{"ASenderTheBridgeRefusesIsRefusedWithItsWayForwardAndNothingIsStored", strand(orch), slack, false, true, true, true, refusal, nil, nil, ""},
		{"AnExternalSenderTheBridgeRefusesIsRefusedAndNothingIsStored", external(), slack, false, true, true, true, refusal, nil, nil, ""},
		{"AProgramsPostAsksTheBridgeNothing", program(batten), slack, false, true, false, true, nil, nil, nil, ""},
		{"AStoreWithoutTheBridgeStoresThePostAndAnswersNoLiveness", strand(orch), slack, false, false, false, false, nil, nil, nil, ""},
		{"APostToAStrandSkipsTheBridgeAndAnswersTheStrandsOwnLiveness", strand(orch), alpha, false, true, false, true, nil, new(true), nil, ""},
		{"APostToABeamsRelayProgramSkipsTheBridgeAndAnswersNoLiveness", strand(orch), beamSlack, false, true, false, true, nil, nil, nil, ""},
		{"AReplyToRelayMailWhileTheRelayRunsIsStoredAndLive", strand(orch), slack, true, true, true, false, nil, new(true), new(true), ""},
		{"AReplyToRelayMailWhileTheRelayIsDownIsStoredNotDeliveredWithAWarningNamingSlackStart", strand(orch), slack, true, true, false, false, nil, new(false), new(false), relayDown},
		{"AReplyToRelayMailTheBridgeRefusesIsRefusedAndNothingIsStored", strand(orch), slack, true, true, true, true, refusal, nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			for _, name := range []string{slack, beamSlack} {
				reading, err := e.store.Read(program(name), "")
				must(t, err)
				reading.Release()
			}
			if tt.bridge {
				bridge := creelfake.NewBridge("neve")
				bridge.SetRelayLive(tt.relayLive)
				if tt.refused {
					bridge.Refuse(tt.sender.Name, refusal)
				}
				e.withBridge(bridge)
			}

			var posted creelengine.PostResult
			var err error
			if tt.reply {
				asked := e.post(t, program(tt.receiver), tt.sender.Name, "the operator's words")
				posted, err = e.store.ReplyWith(tt.sender, asked, "the result")
			} else {
				posted, err = e.store.PostWith(tt.sender, tt.receiver, "the result", creelengine.PostOptions{})
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("post from %s to %s error = %v; want %v", tt.sender.Name, tt.receiver, err, tt.wantErr)
			}
			if !reflect.DeepEqual(posted.ReceiverLive, tt.wantLive) || !reflect.DeepEqual(posted.Delivered, tt.wantDelivered) || posted.Warning != tt.wantWarning {
				t.Errorf("post from %s to %s = live %v, delivered %v, warning %q; want live %v, delivered %v, warning %q",
					tt.sender.Name, tt.receiver, shown(posted.ReceiverLive), shown(posted.Delivered), posted.Warning,
					shown(tt.wantLive), shown(tt.wantDelivered), tt.wantWarning)
			}
			stored := 0
			for _, entry := range e.store.Log(creelengine.LogFilter{To: tt.receiver}) {
				if entry.Event == "posted" && entry.From == tt.sender.Name {
					stored++
				}
			}
			wantStored := 1
			if tt.wantErr != nil {
				wantStored = 0
			}
			if stored != wantStored {
				t.Errorf("posts journaled from %s to %s = %d; want %d", tt.sender.Name, tt.receiver, stored, wantStored)
			}
		})
	}
}

func TestStore_MachineInbound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"ADeliverFromAnOrchStoresTrustedMailAndASecondOfTheSameIdAnswersTheSameIdStoringNothing", func(t *testing.T, e *env) {
			first := e.deliverMachine(t, machineMail(remoteOrch, 7, "hello"))
			if again := e.deliverMachine(t, machineMail(remoteOrch, 7, "hello")); again != first {
				t.Errorf("a repeated deliver answered #%d; want #%d", again, first)
			}
			unread := e.unreadOf(t, orch)
			if len(unread) != 1 || unread[0].ID != first || unread[0].From != remoteOrch || !unread[0].Trusted ||
				unread[0].SendingMachine != "osl" || unread[0].SendingID != 7 || unread[0].Body != "hello" {
				t.Errorf("unread mail = %+v; want one trusted message from %s with sending id 7 on osl", unread, remoteOrch)
			}
		}},
		{"DeliversOfTheSameSendingIdFromTwoSendersBothStore", func(t *testing.T, e *env) {
			first := e.deliverMachine(t, machineMail(remoteOrch, 7, "from zz"))
			second := e.deliverMachine(t, machineMail("osl::yy:orch", 7, "from yy"))
			if first == second || len(e.unreadOf(t, orch)) != 2 {
				t.Errorf("delivers of id 7 from two orchs answered #%d and #%d with %d unread; want two distinct messages", first, second, len(e.unreadOf(t, orch)))
			}
		}},
		{"ADeliverFromAnotherRoleStoresUntrustedMail", func(t *testing.T, e *env) {
			e.deliverMachine(t, machineMail("osl::zz:driver", 8, "a task"))
			if unread := e.unreadOf(t, orch); len(unread) != 1 || unread[0].Trusted {
				t.Errorf("unread mail = %+v; want one untrusted message", unread)
			}
		}},
		{"AReplyToANeverSentIdIsRefusedAndAValidOneEndsTheWaitAndStillArrivesAsMail", func(t *testing.T, e *env) {
			asked, waiter, err := e.store.PostWait(strand(orch), remoteOrch, "question")
			must(t, err)
			bad := machineMail(remoteOrch, 3, "stray")
			bad.ReplyTo = 999
			_, err = e.store.DeliverMachine(program(slack), bad)
			wantErr(t, err, "#999", "lym creel post")
			stranger := machineMail("osl::yy:orch", 4, "not the one asked")
			stranger.ReplyTo = asked
			_, err = e.store.DeliverMachine(program(slack), stranger)
			wantErr(t, err, "never sent to osl::yy:orch")
			reply := machineMail(remoteOrch, 5, "the answer")
			reply.ReplyTo = asked
			replyID := e.deliverMachine(t, reply)
			if outcome := awaitOutcome(t, waiter); outcome.Kind != creelengine.OutcomeReply || outcome.Message.ID != replyID {
				t.Errorf("the wait ended with %+v; want the reply #%d", outcome, replyID)
			}
			late, err := e.store.WaitReply(strand(orch), asked)
			must(t, err)
			if outcome := awaitOutcome(t, late); outcome.Kind != creelengine.OutcomeReply || outcome.Message.Body != "the answer" {
				t.Errorf("a wait registered after the reply ended with %+v; want the reply at once", outcome)
			}
			if unread := e.unreadOf(t, orch); len(unread) != 1 || unread[0].Kind != "reply" || unread[0].ReplyTo != asked {
				t.Errorf("unread mail = %+v; want the reply to #%d as mail", unread, asked)
			}
		}},
		{"AReplyToMachineMailJournalsMachineOutboundWhoseTakeAnswersTheSendingIdAndStartsNoHub", func(t *testing.T, e *env) {
			hubs := e.withHubs()
			id := e.deliverMachine(t, machineMail(remoteOrch, 7, "question"))
			replyID, err := e.store.Reply(strand(orch), id, "answer")
			must(t, err)
			taken, err := e.store.TakeMachineOutbound(program(slack))
			must(t, err)
			if len(taken) != 1 || taken[0].ID != replyID || taken[0].ReplyTo != 7 || taken[0].To != remoteOrch || taken[0].ReceivingMachine != "osl" {
				t.Errorf("take answered %+v; want the reply #%d to %s answering sending id 7", taken, replyID, remoteOrch)
			}
			if slices.Contains(eventsOf(e.store.Log(creelengine.LogFilter{})), "outbound") {
				t.Error("the reply wrote a per-hub outbound record; want a machine-outbound one alone")
			}
			if spawns := hubs.Spawns(zzHub); spawns != 0 {
				t.Errorf("the hub %s was started %d times; want none", zzHub, spawns)
			}
		}},
		{"ARefusedSendReadyAndANonOrchReplierEachRefuseTheReplyJournalingNothing", func(t *testing.T, e *env) {
			id := e.deliverMachine(t, machineMail(remoteOrch, 7, "question"))
			bridge := creelfake.NewBridge("neve")
			bridge.Refuse(orch, errors.New("the slack button is off; way forward: turn it on"))
			e.withBridge(bridge)
			_, err := e.store.Reply(strand(orch), id, "answer")
			wantErr(t, err, "the slack button is off; way forward: turn it on")
			e.withBridge(creelfake.NewBridge("neve"))
			must(t, e.store.Forward(strand(orch), id, alpha))
			_, err = e.store.Reply(strand(alpha), id, "answer")
			wantErr(t, err, "only an orch strand posts to another machine", alpha)
			taken, err := e.store.TakeMachineOutbound(program(slack))
			must(t, err)
			if len(taken) != 0 {
				t.Errorf("take answered %+v; want nothing, since both replies were refused", taken)
			}
		}},
		{"AForwardToAProgramOrToAnotherHubIsRefusedNamingANewPostAndToALocalStrandWorks", func(t *testing.T, e *env) {
			id := e.deliverMachine(t, machineMail(remoteOrch, 7, "question"))
			wantErr(t, e.store.Forward(strand(orch), id, batten), "another machine", "lym creel post "+batten)
			wantErr(t, e.store.Forward(strand(orch), id, "zz:orch"), "another machine", "lym creel post zz:orch")
			must(t, e.store.Forward(strand(orch), id, alpha))
		}},
		{"ADeliverFromAnOrchStrandAnotherProgramOrExternalIsRefused", func(t *testing.T, e *env) {
			for _, from := range []creelengine.Sender{strand(orch), program(batten), external()} {
				_, err := e.store.DeliverMachine(from, machineMail(remoteOrch, 7, "forged"))
				wantErr(t, err, "lym creel post", slack)
			}
			if unread := e.unreadOf(t, orch); len(unread) != 0 {
				t.Errorf("unread mail = %+v; want nothing stored", unread)
			}
		}},
		{"ABadSenderOrReceiverOrBodyIsRefusedStoringNothing", func(t *testing.T, e *env) {
			overTheCap := machineMail(remoteOrch, 7, strings.Repeat("x", 8<<10))
			toADriver := machineMail(remoteOrch, 7, "hi")
			toADriver.Receiver = alpha
			tests := []struct {
				name string
				mail creelengine.MachineMail
				want []string
			}{
				{"ThisMachine", machineMail("neve::zz:orch", 7, "hi"), []string{"names this machine (neve)"}},
				{"ABadMachinePart", machineMail("Bad_Name::zz:orch", 7, "hi"), []string{"is invalid"}},
				{"ANamePartThatIsAProgramName", machineMail("osl::zz:SLACK", 7, "hi"), []string{"osl::zz:SLACK"}},
				{"AReceiverThatIsNoOrch", toADriver, []string{alpha, "lym creel post"}},
				{"ALineOverTheCap", overTheCap, []string{"with the bridge envelope and stamp"}},
				{"AnEmptyBody", machineMail(remoteOrch, 7, " "), []string{"empty"}},
			}
			for _, tt := range tests {
				_, err := e.store.DeliverMachine(program(slack), tt.mail)
				if err == nil {
					t.Errorf("%s: DeliverMachine returned nil; want a refusal", tt.name)
					continue
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("%s: refusal %q lacks %q", tt.name, err, want)
					}
				}
			}
			if got := eventsOf(e.store.Log(creelengine.LogFilter{})); len(got) != 0 {
				t.Errorf("the refused delivers journaled %v; want nothing", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newEnv(t).withBridge(creelfake.NewBridge("neve")))
		})
	}
}
