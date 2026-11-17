// bridgein_test.go covers the relay's receiving half of the bridge.
// It covers the read of the bridge channels, the delivery to creel, and the operator's text in them.
// It also covers the resync that finds the channels the bridge map lacks.
// The poll round that runs both is covered too.

package slackengine_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// bridgeChannelID is the id of the bridge channel the receive harness records for primeOrch and the peer's prime orch.
	bridgeChannelID = "CB1"
	// foundChannelID is the id of a bridge channel the bridge map lacks.
	foundChannelID = "CN"
	// peerBotID and peerAppID are the bot and the app of the approved peer machine osl, whose bot user is peerBot.
	peerBotID = "B2"
	peerAppID = "A0OSL"
	// homeOrch is the full address of primeOrch on the machine the fake hub names testHubMachine.
	homeOrch = testHubMachine + "::" + primeOrch
	// bridgeBase is the first wait of the bridge poll, and its wait after a round that delivered.
	bridgeBase = 5 * time.Second
	// retryFor is how long a command that cannot be carried out is retried before it is answered.
	retryFor = 5 * time.Minute
)

// bridgeText returns the text of a bridge message from one address to another.
func bridgeText(from, to string, id, replyTo int64, body string) string {
	line := fmt.Sprintf("[lym-bridge] from=%s to=%s id=%d", from, to, id)
	if replyTo != 0 {
		line += fmt.Sprintf(" reply-to=%d", replyTo)
	}
	return line + "\n" + body
}

// seedBridgeHub records the bridge channel bridgeChannelID of primeOrch and the peer's prime orch in the harness's hub.
// It pins the machine osl and makes the fake Slack know its bot.
func seedBridgeHub(t *testing.T, h *harness) {
	t.Helper()
	err := slackengine.UpdateBridgeMap(h.geom.BridgeMapPath, func(bridgeMap *slackengine.BridgeMap) error {
		bridgeMap.Channels[bridgeChannelID] = slackengine.BridgeEntry{Name: bridgeName, LocalOrch: primeOrch, Peer: peerOrch, Operator: testOperator, PeerInvited: true}
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateBridgeMap = %v", err)
	}
	approvePeer(t, h.geom, h.clock.Now())
	h.slack.mu.Lock()
	defer h.slack.mu.Unlock()
	h.slack.botApps[peerBotID] = peerAppID
	h.slack.bots = []fakeBot{{Name: "lym-osl", UserID: peerBot, AppID: peerAppID}}
}

// receiveRun is a bridge reader over a configured harness whose hub is named testHubMachine and once named old-box.
type receiveRun struct {
	*harness
	reader *slackengine.BridgeReader
}

// newReceiveRun returns a run whose reader's machine is named machine in its geometry.
func newReceiveRun(t *testing.T, machine string) *receiveRun {
	t.Helper()
	h := newHarness(t)
	h.geom.Machine = machine
	seedBridgeHub(t, h)
	r := &receiveRun{harness: h}
	r.reader = r.newReader()
	return r
}

// newResyncRun returns a run whose bridge map holds no channel, as a hub that lost its map does.
func newResyncRun(t *testing.T, machine string) *receiveRun {
	t.Helper()
	r := newReceiveRun(t, machine)
	err := slackengine.UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *slackengine.BridgeMap) error {
		delete(bridgeMap.Channels, bridgeChannelID)
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateBridgeMap = %v", err)
	}
	return r
}

// newReader returns a reader over the run's files, as a restarted relay builds one.
func (r *receiveRun) newReader() *slackengine.BridgeReader {
	liveHubs := func() ([]slackengine.Geometry, error) { return []slackengine.Geometry{r.geom}, nil }
	hub := r.hub.hub()
	hub.MachineNames = func() (string, []string, error) { return testHubMachine, []string{"old-box"}, nil }
	return slackengine.NewBridgeReader(r.geom, r.mailbox, r.slack, hub, liveHubs, r.clock, r.keeper, r.in)
}

// peerSays adds an envelope message of the peer's bot to the bridge channel.
func (r *receiveRun) peerSays(ts string, id, replyTo int64, body string) {
	r.slack.add(bridgeChannelID, slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, homeOrch, id, replyTo, body)})
}

// operatorSays adds a top-level message of the operator to the bridge channel.
func (r *receiveRun) operatorSays(ts, text string) {
	r.sayIn(bridgeChannelID, ts, text)
}

func (r *receiveRun) read() (bool, error) {
	r.t.Helper()
	return r.reader.Read(context.Background())
}

// mustRead reads and fails the test on an error.
func (r *receiveRun) mustRead() bool {
	r.t.Helper()
	progressed, err := r.read()
	if err != nil {
		r.t.Fatalf("Read = %v", err)
	}
	return progressed
}

// mustResync resyncs the run's reader and fails the test on an error.
func (r *receiveRun) mustResync() {
	r.t.Helper()
	if err := r.reader.Resync(context.Background()); err != nil {
		r.t.Fatalf("Resync = %v", err)
	}
}

// deliveredMail returns the machine mail creel took, in order.
func (r *receiveRun) deliveredMail() []creelengine.MachineMail {
	r.mailbox.mu.Lock()
	defer r.mailbox.mu.Unlock()
	return append([]creelengine.MachineMail(nil), r.mailbox.delivered...)
}

// deliveredIDs returns the sending ids of the machine mail creel took, in order.
func (r *receiveRun) deliveredIDs() []int64 {
	var ids []int64
	for _, mail := range r.deliveredMail() {
		ids = append(ids, mail.SendingID)
	}
	return ids
}

// bridgeEntryOf returns the entry of the bridge channel id in the hub's bridge map, and whether the map holds one.
func (r *receiveRun) bridgeEntryOf(id string) (slackengine.BridgeEntry, bool) {
	r.t.Helper()
	bridgeMap, err := slackengine.ReadBridgeMap(r.geom.BridgeMapPath)
	if err != nil {
		r.t.Fatalf("ReadBridgeMap = %v", err)
	}
	entry, held := bridgeMap.Channels[id]
	return entry, held
}

// entry returns the entry of the harness's bridge channel.
func (r *receiveRun) entry() slackengine.BridgeEntry {
	r.t.Helper()
	entry, _ := r.bridgeEntryOf(bridgeChannelID)
	return entry
}

// postsIn returns the texts the relay posted in the channel with PostAs, in order.
func (r *receiveRun) postsIn(channel string) []string {
	var texts []string
	for _, call := range r.slack.logged("PostAs") {
		if call.Channel == channel {
			texts = append(texts, call.Arg)
		}
	}
	return texts
}

// noticesIn returns the `[slack relay]` notices the relay posted in the channel.
func (r *receiveRun) noticesIn(channel string) []string {
	var notices []string
	for _, text := range r.postsIn(channel) {
		if strings.HasPrefix(text, "[slack relay]") {
			notices = append(notices, text)
		}
	}
	return notices
}

// countIn returns how many of the texts hold substring.
func countIn(texts []string, substring string) int {
	count := 0
	for _, text := range texts {
		if strings.Contains(text, substring) {
			count++
		}
	}
	return count
}

// unpin removes the approval of the machine osl.
func (r *receiveRun) unpin() {
	r.t.Helper()
	err := slackengine.UpdatePeers(r.geom.PeersPath, func(peers *slackengine.Peers) error {
		delete(peers.Machines, "osl")
		return nil
	})
	if err != nil {
		r.t.Fatalf("UpdatePeers = %v", err)
	}
}

// pinned reports whether the machine osl is approved.
func (r *receiveRun) pinned() bool {
	r.t.Helper()
	peers, err := slackengine.ReadPeers(r.geom.PeersPath)
	if err != nil {
		r.t.Fatalf("ReadPeers = %v", err)
	}
	_, pinned := peers.Machines["osl"]
	return pinned
}

// minutesAgo returns the Slack timestamp the given number of minutes before the harness's clock.
func (r *receiveRun) minutesAgo(minutes int) string {
	return r.ago(time.Duration(minutes) * time.Minute)
}

// wantNotices fails the test unless the relay posted exactly n notices in the bridge channel.
func (r *receiveRun) wantNotices(n int) []string {
	r.t.Helper()
	notices := r.noticesIn(bridgeChannelID)
	if len(notices) != n {
		r.t.Errorf("notices in the bridge channel = %q; want %d", notices, n)
	}
	return notices
}

func TestBridgeReceive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("AMessageOfThePinnedAppIsDeliveredOnceFromItsSenderAndARereadDeliversNothingNew", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		posted := r.minutesAgo(10)
		r.peerSays(posted, 7, 0, "hello &lt;there&gt;")
		if !r.mustRead() {
			t.Fatalf("Read reported no delivery")
		}
		postedAt := time.Unix(r.clock.Now().Add(-10*time.Minute).Unix(), int64(time.Microsecond)).UTC()
		want := []creelengine.MachineMail{{Sender: peerOrch, Receiver: primeOrch, SendingID: 7, Body: "hello <there>", Posted: postedAt}}
		if diff := cmp.Diff(want, r.deliveredMail()); diff != "" {
			t.Errorf("delivered mail mismatch (-want +got):\n%s", diff)
		}
		if r.mustRead() {
			t.Errorf("a reread reported a delivery")
		}
		if got := len(r.deliveredMail()); got != 1 {
			t.Errorf("deliveries after a reread = %d; want 1", got)
		}
		if got := r.entry().Cursor; got != posted {
			t.Errorf("delivery cursor = %q; want %q", got, posted)
		}
	})

	t.Run("MessagesThatAreNotForCreelAreSkippedWithNoDeliveryAndTheCursorMovesPastThem", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			message func(ts string) slackengine.Message
		}{
			{"ThisBotsOwnEnvelope", func(ts string) slackengine.Message {
				return slackengine.Message{TS: ts, User: "UBOT", BotID: "B1", Text: bridgeText(peerOrch, homeOrch, 1, 0, "x")}
			}},
			{"AnotherHumansEnvelope", func(ts string) slackengine.Message {
				return slackengine.Message{TS: ts, User: "U9", Text: bridgeText(peerOrch, homeOrch, 1, 0, "x")}
			}},
			{"ABotMessageWithNoBotID", func(ts string) slackengine.Message {
				return slackengine.Message{TS: ts, User: peerBot, Text: bridgeText(peerOrch, homeOrch, 1, 0, "x")}
			}},
			{"AFirstLineThatIsNoEnvelope", func(ts string) slackengine.Message {
				return slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: "just talking"}
			}},
			{"AnEnvelopeForAnotherOrch", func(ts string) slackengine.Message {
				return slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, testHubMachine+"::"+paymentsOrch, 1, 0, "x")}
			}},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				r := newReceiveRun(t, testMachine)
				ts := r.minutesAgo(10)
				r.slack.add(bridgeChannelID, row.message(ts))
				if r.mustRead() {
					t.Errorf("Read reported a delivery")
				}
				if ids := r.deliveredIDs(); len(ids) != 0 {
					t.Errorf("delivered ids = %v; want none", ids)
				}
				if posts := r.postsIn(bridgeChannelID); len(posts) != 0 {
					t.Errorf("posts in the bridge channel = %q; want none", posts)
				}
				if got := r.entry().Cursor; got != ts {
					t.Errorf("delivery cursor = %q; want %q", got, ts)
				}
			})
		}
	})

	t.Run("ADeliverLostOnADroppedConnectionHoldsTheCursorAndDeliversAtTheNextRead", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.mailbox.failMachine("Deliver", errors.New("connection lost"))
		ts := r.minutesAgo(10)
		r.peerSays(ts, 1, 0, "a")
		if _, err := r.read(); err == nil {
			t.Fatalf("Read = nil; want the lost connection")
		}
		if got := r.entry().Cursor; got != "" {
			t.Errorf("delivery cursor after the lost deliver = %q; want it held", got)
		}
		if !r.mustRead() {
			t.Fatalf("the next Read reported no delivery")
		}
		if got := r.entry().Cursor; got != ts {
			t.Errorf("delivery cursor after the next read = %q; want %q", got, ts)
		}
		if diff := cmp.Diff([]int64{1}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ADeliverWhoseReplyToCreelRefusesIsSkippedWithOneNoticeQuotingTheReasonAndTheNextMessageDelivers", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.mailbox.failMachine("Deliver", fmt.Errorf("creel: this hub never sent message 99 to that orch: %w", creelengine.ErrDaemonRefusal))
		r.peerSays(r.minutesAgo(10), 1, 99, "answers a message nobody sent")
		r.peerSays(r.minutesAgo(9), 2, 0, "plain")
		if !r.mustRead() {
			t.Fatalf("Read reported no delivery")
		}
		if diff := cmp.Diff([]int64{2}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
		if notices := r.wantNotices(1); len(notices) == 1 && !strings.Contains(notices[0], "never sent message 99") {
			t.Errorf("notice = %q; want it to quote creel's reason", notices[0])
		}
		if got := r.entry().Cursor; got != r.minutesAgo(9) {
			t.Errorf("delivery cursor = %q; want it past both messages", got)
		}
	})

	t.Run("AnApproveOrRevokeNamingThisMachineByAnyOfItsNamesPinsNothingAndAnswersNothing", func(t *testing.T) {
		t.Parallel()
		for _, text := range []string{"approve home", "revoke home", "approve OLD-BOX"} {
			r := newReceiveRun(t, testMachine)
			ts := r.minutesAgo(10)
			r.operatorSays(ts, text)
			r.mustRead()
			if !r.pinned() {
				t.Errorf("%q changed the approval of osl", text)
			}
			if posts := r.postsIn(bridgeChannelID); len(posts) != 0 {
				t.Errorf("%q was answered with %q; want it left to the peer's relay", text, posts)
			}
			if got := r.entry().OperatorCursor; got != ts {
				t.Errorf("operator cursor after %q = %q; want %q", text, got, ts)
			}
		}
	})

	t.Run("TwoHubsOfOneMachineThatGiveTheSameSendingIDToOneOrchBothDeliver", func(t *testing.T) {
		t.Parallel()
		for _, r := range []*receiveRun{newReceiveRun(t, testMachine), newReceiveRun(t, testMachine)} {
			r.peerSays(r.minutesAgo(10), 7, 0, "x")
			r.mustRead()
			if diff := cmp.Diff([]int64{7}, r.deliveredIDs()); diff != "" {
				t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
			}
		}
	})

	t.Run("AnotherAppsMessageIsSkippedWithOneNoticeAndLaterPinnedMessagesDeliver", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.slack.mu.Lock()
		r.slack.botApps["B9"] = "A0BAD"
		r.slack.mu.Unlock()
		for i, ts := range []string{r.minutesAgo(10), r.minutesAgo(9)} {
			r.slack.add(bridgeChannelID, slackengine.Message{TS: ts, User: "UBAD", BotID: "B9", Text: bridgeText(peerOrch, homeOrch, int64(i+1), 0, "x")})
		}
		if r.mustRead() {
			t.Errorf("Read reported a delivery for another app's messages")
		}
		notices := r.wantNotices(1)
		if len(notices) == 1 && (!strings.Contains(notices[0], "A0BAD") || !strings.Contains(notices[0], "approve osl A0BAD")) {
			t.Errorf("notice = %q; want it to name the app and the command that pins it", notices[0])
		}
		if got := r.entry().Cursor; got != r.minutesAgo(9) {
			t.Errorf("delivery cursor = %q; want it past both messages", got)
		}
		r.peerSays(r.minutesAgo(8), 3, 0, "from the pinned app")
		if !r.mustRead() {
			t.Fatalf("Read reported no delivery for the pinned app's message")
		}
		if diff := cmp.Diff([]int64{3}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AnUnpinnedMachineHoldsTheCursorWithOneNoticeAndAnApproveDeliversTheHeldMessagesInOrderSkippingOnePastRetention", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.unpin()
		old, approved := r.ago(8*24*time.Hour), r.minutesAgo(5)
		r.peerSays(old, 1, 0, "past retention")
		r.peerSays(r.minutesAgo(10), 2, 0, "first")
		r.peerSays(r.minutesAgo(9), 3, 0, "second")

		if r.mustRead() {
			t.Errorf("the first Read reported a delivery while the machine was unpinned")
		}
		if got := r.entry().Cursor; got != old {
			t.Errorf("delivery cursor while held = %q; want it past the message beyond retention only (%q)", got, old)
		}
		if notices := r.wantNotices(1); len(notices) == 1 && !strings.Contains(notices[0], "approve osl") {
			t.Errorf("notice = %q; want it to ask for approve osl", notices[0])
		}

		r.operatorSays(approved, "approve osl")
		if !r.mustRead() {
			t.Fatalf("the Read that handled the approve reported no delivery of the held messages")
		}
		if !r.pinned() {
			t.Fatalf("the operator's approve was not handled while a message was held")
		}
		if diff := cmp.Diff([]int64{2, 3}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
		if got := r.entry().Cursor; got != approved {
			t.Errorf("delivery cursor after the approval = %q; want %q", got, approved)
		}
		r.wantNotices(1)
		if got := countIn(r.postsIn(bridgeChannelID), "approved osl"); got != 1 {
			t.Errorf("answers naming the approval = %d; want 1", got)
		}
	})

	t.Run("AnApproveDeliversTheHeldMessageBeforeALaterRevokeInTheSameReadAndEachLineIsHandledOnce", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.unpin()
		r.peerSays(r.minutesAgo(20), 1, 0, "first")
		r.mustRead()
		r.wantNotices(1)

		r.operatorSays(r.minutesAgo(19), "approve osl")
		r.operatorSays(r.minutesAgo(18), "revoke osl")
		r.operatorSays(r.minutesAgo(17), "just a line")
		if !r.mustRead() {
			t.Errorf("the Read that handled the approve reported no delivery of the held message")
		}
		if r.pinned() {
			t.Errorf("the revoke did not stand")
		}
		r.mustRead()
		posts := r.postsIn(bridgeChannelID)
		for substring, want := range map[string]int{"approved osl": 1, "revoked osl": 1, "not delivered": 1} {
			if got := countIn(posts, substring); got != want {
				t.Errorf("answers holding %q = %d; want %d", substring, got, want)
			}
		}
		r.wantNotices(1)

		r.peerSays(r.minutesAgo(10), 2, 0, "after the revoke")
		for range 2 {
			if r.mustRead() {
				t.Errorf("a Read delivered a message of the revoked machine")
			}
		}
		r.wantNotices(2)
		if diff := cmp.Diff([]int64{1}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AnApproveAsAThreadReplyIsHandledOnceAndAnsweredInItsThread", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.unpin()
		parent, reply := r.minutesAgo(10), r.minutesAgo(9)
		r.slack.add(bridgeChannelID, slackengine.Message{TS: parent, User: peerBot, BotID: peerBotID, Text: "a thread", LatestReply: reply})
		r.slack.add(bridgeChannelID, slackengine.Message{TS: reply, ThreadTS: parent, User: testOperator, Text: "approve osl"})
		for range 2 {
			r.mustRead()
		}
		if !r.pinned() {
			t.Errorf("the approve in the thread was not handled")
		}
		replies := r.slack.logged("PostThreadReply")
		if len(replies) != 1 || replies[0].Channel != bridgeChannelID || !strings.Contains(replies[0].Arg, "approved osl") {
			t.Errorf("thread replies = %+v; want one in the bridge channel naming the approval", replies)
		}
		if got := r.entry().Threads[parent]; got != reply {
			t.Errorf("thread cursor = %q; want %q", got, reply)
		}
	})

	t.Run("AUsersListRefusalOnAnApproveHoldsTheOperatorCursorAndIsAnsweredOnceTheRetryWindowPasses", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.unpin()
		refused := &slackengine.APIError{Method: "users.list", Code: "not_authed"}
		r.slack.failNext("BotUsersNamed", refused, refused, refused)
		ts := r.minutesAgo(10)
		r.operatorSays(ts, "approve osl")

		for call := 1; call <= 2; call++ {
			if _, err := r.read(); !errors.Is(err, refused) {
				t.Errorf("Read #%d = %v; want the refusal", call, err)
			}
			if got := r.entry().OperatorCursor; got != "" {
				t.Errorf("operator cursor after read #%d = %q; want it held", call, got)
			}
		}
		r.clock.Advance(retryFor)
		if _, err := r.read(); err != nil {
			t.Fatalf("Read after the retry window = %v", err)
		}
		if got := countIn(r.postsIn(bridgeChannelID), "could not be carried out"); got != 1 {
			t.Errorf("answers naming the failure = %d; want 1", got)
		}
		if got := r.entry().OperatorCursor; got != ts {
			t.Errorf("operator cursor after the answer = %q; want %q", got, ts)
		}
		r.mustRead()
		if calls := len(r.slack.logged("BotUsersNamed")); calls != 3 {
			t.Errorf("BotUsersNamed calls = %d; want no retry once the message is answered", calls)
		}
	})

	t.Run("AHeldMessageStaysHeldPastFiveMinutes", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.unpin()
		r.peerSays(r.minutesAgo(10), 1, 0, "waiting")
		r.mustRead()
		r.clock.Advance(2 * retryFor)
		if r.mustRead() {
			t.Errorf("a held message was delivered or given up on")
		}
		if got := r.entry().Cursor; got != "" {
			t.Errorf("delivery cursor = %q; want it still held", got)
		}
		r.wantNotices(1)
	})

	t.Run("AMessageToAnOrchWhoseButtonIsOffIsHeldWithNoNoticeUntilTheButtonIsOn", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.setButton(primeOrch, false)
		ts := r.minutesAgo(10)
		r.peerSays(ts, 1, 0, "x")
		if r.mustRead() {
			t.Errorf("a message was delivered to an orch whose button is off")
		}
		if got := r.entry().Cursor; got != "" {
			t.Errorf("delivery cursor = %q; want it held", got)
		}
		r.wantNotices(0)
		r.setButton(primeOrch, true)
		if !r.mustRead() {
			t.Fatalf("Read reported no delivery once the button is on")
		}
		if got := r.entry().Cursor; got != ts {
			t.Errorf("delivery cursor = %q; want %q", got, ts)
		}
	})

	t.Run("AnEnvelopeNamingAFormerNameOfThisMachineAsToIsDeliveredAndAsFromIsSkippedWithOneNotice", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		r.slack.add(bridgeChannelID, slackengine.Message{TS: r.minutesAgo(10), User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, "old-box::"+primeOrch, 1, 0, "to the former name")})
		for i, ts := range []string{r.minutesAgo(9), r.minutesAgo(8)} {
			r.slack.add(bridgeChannelID, slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: bridgeText("old-box::"+primeOrch, homeOrch, int64(i+2), 0, "from the former name")})
		}
		r.mustRead()
		if diff := cmp.Diff([]int64{1}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
		if notices := r.wantNotices(1); len(notices) == 1 && !strings.Contains(notices[0], "old-box") {
			t.Errorf("notice = %q; want it to name the clashing name", notices[0])
		}
	})

	t.Run("AMessageToARetiredOrchIsSkippedWithOneNoticeNamingIt", func(t *testing.T) {
		t.Parallel()
		r := newReceiveRun(t, testMachine)
		err := slackengine.UpdateBridgeMap(r.geom.BridgeMapPath, func(bridgeMap *slackengine.BridgeMap) error {
			entry := bridgeMap.Channels[bridgeChannelID]
			entry.LocalOrch = paymentsOrch
			bridgeMap.Channels[bridgeChannelID] = entry
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateBridgeMap = %v", err)
		}
		for i, ts := range []string{r.minutesAgo(10), r.minutesAgo(9)} {
			r.slack.add(bridgeChannelID, slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, testHubMachine+"::"+paymentsOrch, int64(i+1), 0, "x")})
		}
		if r.mustRead() {
			t.Errorf("Read reported a delivery to a retired orch")
		}
		if notices := r.wantNotices(1); len(notices) == 1 && !strings.Contains(notices[0], paymentsOrch) {
			t.Errorf("notice = %q; want it to name %s", notices[0], paymentsOrch)
		}
		if got := r.entry().Cursor; got != r.minutesAgo(9) {
			t.Errorf("delivery cursor = %q; want it past both messages", got)
		}
	})

	t.Run("OtherOperatorTextIsAnsweredOnceByTheEndThatSortsFirstAndNotByTheOther", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			machine string
			want    int
		}{
			{"TheEndThatSortsFirstAnswers", testMachine, 1},
			{"TheEndThatSortsLastStaysSilent", "zed", 0},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				r := newReceiveRun(t, row.machine)
				r.operatorSays(r.minutesAgo(10), "just a line")
				r.mustRead()
				r.mustRead()
				posts := r.postsIn(bridgeChannelID)
				if got := countIn(posts, "not delivered"); got != row.want {
					t.Errorf("answers = %q; want %d naming not delivered", posts, row.want)
				}
				if row.want == 1 && countIn(posts, primeChannelName) != 1 {
					t.Errorf("answers = %q; want one pointing at %s", posts, primeChannelName)
				}
			})
		}
	})

	t.Run("AChannelMadeByAnApprovedBotNamingThisHubsOrchIsAddedAndOthersAreLeftOut", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			creator string
			// envelopes are the envelopes the channel's messages hold, each as from, to and the poster's user id, bot id.
			envelopes []struct{ from, to, user, bot string }
			wantOrch  string
			wantPeer  string
		}{
			{"MadeByThePinnedPeersBotIgnoringAHumanPostedEnvelope", peerBot, []struct{ from, to, user, bot string }{
				{peerOrch, homeOrch, peerBot, peerBotID},
				{"osl::ly:payments:orch", testHubMachine + "::" + paymentsOrch, "U9", ""},
			}, primeOrch, peerOrch},
			{"NamingAFormerNameOfThisMachine", peerBot, []struct{ from, to, user, bot string }{
				{peerOrch, "old-box::" + primeOrch, peerBot, peerBotID},
			}, primeOrch, peerOrch},
			{"MadeByThisMachinesBot", "UBOT", []struct{ from, to, user, bot string }{
				{homeOrch, peerOrch, "UBOT", "B1"},
			}, primeOrch, peerOrch},
			{"MadeByAnotherCreator", "UOTHER", []struct{ from, to, user, bot string }{
				{peerOrch, homeOrch, peerBot, peerBotID},
			}, "", ""},
			{"NamingAnotherHub", peerBot, []struct{ from, to, user, bot string }{
				{"osl::zz:orch", testHubMachine + "::zz:orch", peerBot, peerBotID},
			}, "", ""},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				r := newResyncRun(t, testMachine)
				r.slack.memberChannels = []slackengine.ChannelInfo{{ID: foundChannelID, Name: bridgeName, Creator: row.creator, IsMember: true}}
				var newest string
				for i, envelope := range row.envelopes {
					newest = r.minutesAgo(10 - i)
					r.slack.add(foundChannelID, slackengine.Message{TS: newest, User: envelope.user, BotID: envelope.bot, Text: bridgeText(envelope.from, envelope.to, int64(i+1), 0, "x")})
				}
				r.mustResync()
				got, added := r.bridgeEntryOf(foundChannelID)
				if row.wantOrch == "" {
					if added {
						t.Errorf("bridge entry = %+v; want the channel left unmapped", got)
					}
					if posts := r.postsIn(foundChannelID); len(posts) != 0 {
						t.Errorf("posts in the channel = %q; want none", posts)
					}
					return
				}
				want := slackengine.BridgeEntry{Name: bridgeName, LocalOrch: row.wantOrch, Peer: row.wantPeer, OperatorCursor: newest, ThreadFloor: newest}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("bridge entry mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("AResyncLeavesTheChannelOfARetiredOrchUnmappedWithOneNoticeAndARestartPostsNoSecondOne", func(t *testing.T) {
		t.Parallel()
		r := newResyncRun(t, testMachine)
		r.slack.memberChannels = []slackengine.ChannelInfo{{ID: foundChannelID, Name: bridgeName, Creator: peerBot, IsMember: true}}
		r.slack.add(foundChannelID, slackengine.Message{TS: r.minutesAgo(10), User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, testHubMachine+"::"+paymentsOrch, 1, 0, "x")})
		r.mustResync()
		r.reader = r.newReader()
		r.mustResync()
		if _, added := r.bridgeEntryOf(foundChannelID); added {
			t.Errorf("the channel of a retired orch was added")
		}
		notices := r.noticesIn(foundChannelID)
		if len(notices) != 1 || !strings.Contains(notices[0], paymentsOrch) {
			t.Errorf("notices in the channel = %q; want one naming %s", notices, paymentsOrch)
		}
	})

	t.Run("AChannelAnUnapprovedAppMadeGetsOneNoticeInThePrimeChannelAndIsAddedOnceTheAppIsApproved", func(t *testing.T) {
		t.Parallel()
		r := newResyncRun(t, testMachine)
		r.slack.mu.Lock()
		r.slack.bots = append(r.slack.bots, fakeBot{Name: "lym-new", UserID: "UNEW", AppID: "A0NEW"})
		r.slack.botApps["B3"] = "A0NEW"
		r.slack.mu.Unlock()
		const newName = "bridge-home-ly-orch--new-ly-orch"
		r.slack.memberChannels = []slackengine.ChannelInfo{{ID: foundChannelID, Name: newName, Creator: "UNEW", IsMember: true}}
		r.slack.add(foundChannelID, slackengine.Message{TS: r.minutesAgo(10), User: "UNEW", BotID: "B3", Text: bridgeText("new::"+primeOrch, homeOrch, 1, 0, "x")})

		r.mustResync()
		r.mustResync()
		notices := r.noticesIn(testChannel)
		if len(notices) != 1 || !strings.Contains(notices[0], "A0NEW") || !strings.Contains(notices[0], "approve new") {
			t.Errorf("notices in the prime channel = %q; want one naming the app and approve new", notices)
		}
		if got := countIn(r.relayNotices(), "A0NEW"); got != 1 {
			t.Errorf("mailed notices naming the app = %d; want 1", got)
		}
		if _, added := r.bridgeEntryOf(foundChannelID); added {
			t.Fatalf("the channel of an unapproved app was added")
		}

		err := slackengine.UpdatePeers(r.geom.PeersPath, func(peers *slackengine.Peers) error {
			peers.Machines["new"] = slackengine.Peer{AppID: "A0NEW", BotUserID: "UNEW", ApprovedAt: r.clock.Now()}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdatePeers = %v", err)
		}
		r.mustResync()
		got, added := r.bridgeEntryOf(foundChannelID)
		if !added || got.LocalOrch != primeOrch || got.Peer != "new::"+primeOrch {
			t.Errorf("bridge entry = %+v, held %v; want the channel added for %s and new::%s", got, added, primeOrch, primeOrch)
		}
	})

	t.Run("AChannelAnUnapprovedAppMadeIsReportedInEveryOpenChannelWhoseButtonIsOnWhenThePrimeHasNoOpenChannel", func(t *testing.T) {
		t.Parallel()
		r := newResyncRun(t, testMachine)
		err := slackengine.UpdateChannelMap(r.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
			delete(channelMap.Channels, primeOrch)
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateChannelMap = %v", err)
		}
		r.slack.mu.Lock()
		r.slack.bots = append(r.slack.bots, fakeBot{Name: "lym-new", UserID: "UNEW", AppID: "A0NEW"})
		r.slack.mu.Unlock()
		r.slack.memberChannels = []slackengine.ChannelInfo{{ID: foundChannelID, Name: "bridge-home-ly-orch--new-ly-orch", Creator: "UNEW", IsMember: true}}
		r.slack.add(foundChannelID, slackengine.Message{TS: r.minutesAgo(10), User: "UNEW", BotID: "B3", Text: bridgeText("new::"+primeOrch, homeOrch, 1, 0, "x")})

		r.mustResync()
		if got := len(r.noticesIn(paymentsChannel)); got != 1 {
			t.Errorf("notices in the open channel whose button is on = %d; want 1", got)
		}
		if got := len(r.noticesIn(testChannel)); got != 0 {
			t.Errorf("notices in the prime's channel = %d; want none as it is not open", got)
		}
		if got := countIn(r.relayNotices(), "A0NEW"); got != 1 {
			t.Errorf("mailed notices naming the app = %d; want 1", got)
		}
	})

	t.Run("AnAddedChannelDeliversItsMessagesWithinRetentionOnceAndHandlesNeitherAnOldOperatorLineNorAnEnvelopePastRetention", func(t *testing.T) {
		t.Parallel()
		r := newResyncRun(t, testMachine)
		r.slack.memberChannels = []slackengine.ChannelInfo{{ID: foundChannelID, Name: bridgeName, Creator: peerBot, IsMember: true}}
		peerEnvelope := func(ts string, id int64) {
			r.slack.add(foundChannelID, slackengine.Message{TS: ts, User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, homeOrch, id, 0, "x")})
		}
		peerEnvelope(r.ago(8*24*time.Hour), 1)
		r.slack.add(foundChannelID, slackengine.Message{TS: r.minutesAgo(120), User: testOperator, Text: "an old line"})
		peerEnvelope(r.minutesAgo(60), 2)
		r.mustResync()

		r.reader = r.newReader()
		for range 2 {
			if _, err := r.read(); err != nil {
				t.Fatalf("Read = %v", err)
			}
		}
		if diff := cmp.Diff([]int64{2}, r.deliveredIDs()); diff != "" {
			t.Errorf("delivered ids mismatch (-want +got):\n%s", diff)
		}
		if posts := r.postsIn(foundChannelID); len(posts) != 0 {
			t.Errorf("posts in the added channel = %q; want the old operator line left unhandled", posts)
		}
	})

	t.Run("OnAMachineNamedBridgeAResyncReadsNoneOfTheHubsOrchChannels", func(t *testing.T) {
		t.Parallel()
		r := newResyncRun(t, "bridge")
		name := slackengine.ChannelName("bridge", "ly", "p0", strings.Repeat("x", 90))
		const orchChannel, strayChannel = "CL", "CL2"
		err := slackengine.UpdateChannelMap(r.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
			channelMap.Channels["ly:long:orch"] = slackengine.ChannelEntry{ID: orchChannel, Name: name}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateChannelMap = %v", err)
		}
		r.slack.memberChannels = []slackengine.ChannelInfo{
			{ID: orchChannel, Name: name, Creator: "UBOT", IsMember: true},
			{ID: strayChannel, Name: name, Creator: "UBOT", IsMember: true},
		}
		r.mustResync()
		var read []string
		for _, call := range r.slack.logged("History") {
			read = append(read, call.Channel)
		}
		if diff := cmp.Diff([]string{strayChannel}, read); diff != "" {
			t.Errorf("channels the resync read mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ThePollRoundResyncsAtTheStartAndAgainOnceTheClockPassesTheCeilingAndNotBetween", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		seedBridgeHub(t, r.harness)
		resyncs := func() int { return len(r.slack.logged("MemberChannels")) }
		r.start()
		r.bridgeClock.awaitWait(t, bridgeBase)
		if got := resyncs(); got != 1 {
			t.Errorf("resyncs at the relay's start = %d; want 1", got)
		}
		r.bridgeClock.Advance(bridgeBase)
		r.bridgeClock.awaitWait(t, 2*bridgeBase)
		if got := resyncs(); got != 1 {
			t.Errorf("resyncs after a round inside the ceiling = %d; want 1", got)
		}
		r.clock.Advance(bridgeCeiling)
		r.bridgeClock.Advance(2 * bridgeBase)
		r.bridgeClock.awaitWait(t, 4*bridgeBase)
		if got := resyncs(); got != 2 {
			t.Errorf("resyncs after a round past the ceiling = %d; want 2", got)
		}
	})

	t.Run("ThePollsWaitsDoubleWhileAHeldMessageIsReReadAndReturnToBaseAfterANewDelivery", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		seedBridgeHub(t, r.harness)
		err := slackengine.UpdatePeers(r.geom.PeersPath, func(peers *slackengine.Peers) error {
			delete(peers.Machines, "osl")
			return nil
		})
		if err != nil {
			t.Fatalf("UpdatePeers = %v", err)
		}
		r.slack.add(bridgeChannelID, slackengine.Message{TS: r.ago(10 * time.Minute), User: peerBot, BotID: peerBotID, Text: bridgeText(peerOrch, homeOrch, 1, 0, "held")})

		r.start()
		r.bridgeClock.awaitWait(t, bridgeBase)
		r.bridgeClock.Advance(bridgeBase)
		r.bridgeClock.awaitWait(t, 2*bridgeBase)
		r.bridgeClock.Advance(2 * bridgeBase)
		r.bridgeClock.awaitWait(t, 4*bridgeBase)

		approvePeer(t, r.geom, r.clock.Now())
		r.bridgeClock.Advance(4 * bridgeBase)
		r.bridgeClock.awaitWait(t, bridgeBase)
		eventually(t, "the held message to be delivered", func() bool {
			r.mailbox.mu.Lock()
			defer r.mailbox.mu.Unlock()
			return len(r.mailbox.delivered) == 1
		})
	})

	t.Run("AHintForABridgeChannelOrOneOfItsThreadsWakesThePollAtOnce", func(t *testing.T) {
		t.Parallel()
		for _, hint := range []string{bridgeChannelID, bridgeChannelID + "_1760000000.000001"} {
			t.Run(hint, func(t *testing.T) {
				t.Parallel()
				r := newRelayRun(t)
				seedBridgeHub(t, r.harness)
				r.start()
				r.bridgeClock.awaitWait(t, bridgeBase)
				if r.bridgeClock.hasWait(2 * bridgeBase) {
					t.Fatalf("the poll ran a second round before any hint")
				}
				r.clock.Advance(slackengine.WakeFloor)
				r.hint(hint)
				r.driveUntil("the bridge poll to run a round on the hint", func() bool { return r.bridgeClock.hasWait(2 * bridgeBase) })
			})
		}
	})
}
