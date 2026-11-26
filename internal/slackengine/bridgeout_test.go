// bridgeout_test.go covers the relay's sending of machine mail to the bridge channels and the bridge poll.
// It runs over the in-memory Slack, mailbox and hub of inbound_test.go, with the relay's files in temp directories.

package slackengine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// bridgeName is the name of the bridge channel of primeOrch and the peer's prime orch.
	bridgeName = "bridge-neve-ly-orch--osl-ly-orch"
	// peerOrch is the full address of the orch the test messages are for.
	peerOrch = "osl::ly:orch"
	// peerBot is the Slack user id of the approved peer's bot.
	peerBot = "UOSL"
	// bridgeCeiling is the longest wait of the bridge poll, which is also the least time between two tries of a held pair.
	bridgeCeiling = 60 * time.Second
	// firstTimestamp is the timestamp the fake Slack gives its first timestamped post.
	firstTimestamp = "1760000000.000001"
)

// unarchive marks the channel called name no longer archived, as the operator does in Slack.
func (s *fakeSlack) unarchive(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexByName(name); i >= 0 {
		s.directory[i].IsArchived = false
	}
}

// bridgeRun is a bridge sender over a configured harness.
type bridgeRun struct {
	*harness
	sender *slackengine.BridgeSender
}

func newBridgeRun(t *testing.T) *bridgeRun {
	t.Helper()
	h := newBareHarness(t)
	return &bridgeRun{harness: h, sender: slackengine.NewBridgeSender(h.geom, h.mailbox, h.slack, h.clock, h.keeper)}
}

// restart replaces the sender and the keeper, as a restarted relay does, over the same files.
func (b *bridgeRun) restart() {
	liveHubs := func() ([]slackengine.Geometry, error) { return []slackengine.Geometry{b.geom}, nil }
	b.keeper = slackengine.NewChannelKeeper(b.geom, b.slack, b.hub.hub(), liveHubs, b.mailbox, b.clock)
	b.sender = slackengine.NewBridgeSender(b.geom, b.mailbox, b.slack, b.clock, b.keeper)
}

// approvePeer records the approval of machine osl, whose bot is peerBot.
func approvePeer(t *testing.T, geom slackengine.Geometry, now time.Time) {
	t.Helper()
	err := slackengine.UpdatePeers(geom.PeersPath, func(peers *slackengine.Peers) error {
		peers.Machines["osl"] = slackengine.Peer{AppID: "A0OSL", BotUserID: peerBot, ApprovedAt: now}
		return nil
	})
	if err != nil {
		t.Fatalf("UpdatePeers = %v", err)
	}
}

func (b *bridgeRun) approve() {
	b.t.Helper()
	approvePeer(b.t, b.geom, b.clock.Now())
}

// message returns the machine message id from primeOrch to the peer's orch.
func (b *bridgeRun) message(id int64, body string) creelengine.MachineOutbound {
	return creelengine.MachineOutbound{ID: id, From: primeOrch, To: peerOrch, ReceivingMachine: "osl", Body: body, Posted: b.clock.Now()}
}

func (b *bridgeRun) send() (bool, error) {
	b.t.Helper()
	return b.sender.Send(context.Background())
}

// mustSend sends and fails the test on an error.
func (b *bridgeRun) mustSend() bool {
	b.t.Helper()
	posted, err := b.send()
	if err != nil {
		b.t.Fatalf("Send = %v", err)
	}
	return posted
}

// posts returns the timestamped posts the fake Slack received, in order.
func (b *bridgeRun) posts() []slackCall {
	return b.slack.logged("PostTimestamped")
}

// bridgeMap returns the hub's bridge map.
func (b *bridgeRun) bridgeMap() slackengine.BridgeMap {
	b.t.Helper()
	bridgeMap, err := slackengine.ReadBridgeMap(b.geom.BridgeMapPath)
	if err != nil {
		b.t.Fatalf("ReadBridgeMap = %v", err)
	}
	return bridgeMap
}

// wantReports fails the test unless the reports creel received are exactly reasons, each for message id.
func (b *bridgeRun) wantReports(id int64, reasons ...string) {
	b.t.Helper()
	var want []machineCall
	for _, reason := range reasons {
		want = append(want, machineCall{Verb: "Report", ID: id, Arg: reason})
	}
	got := b.mailbox.machineLog("Report")
	if len(got) != len(want) || (len(want) > 0 && !cmp.Equal(want, got)) {
		b.t.Errorf("reports mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

// wantNotices fails the test unless the relay mailed the orch exactly n notices.
func (b *bridgeRun) wantNotices(n int) []string {
	b.t.Helper()
	notices := b.relayNotices()
	if len(notices) != n {
		b.t.Errorf("relay notices = %q; want %d", notices, n)
	}
	return notices
}

// envelopeText is the text the relay posts for message id from primeOrch with body.
func envelopeText(id int64, body string) string {
	return fmt.Sprintf("[lym-bridge] from=%s::%s to=%s id=%d\n%s", testMachine, primeOrch, peerOrch, id, body)
}

// fileHolds reports whether the file at path holds substring.
func fileHolds(t *testing.T, path, substring string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", path, err)
	}
	return strings.Contains(string(data), substring)
}

func TestBridgeSend(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	lostConnection := errors.New("connection lost")
	creelRefusal := fmt.Errorf("creel: no such message: %w", creelengine.ErrDaemonRefusal)

	t.Run("MessageToAnUnapprovedPeerIsHeldWithOneNoticeAndPostedOnceThePeerIsPinned", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.mailbox.setPending(b.message(1, "hello"))
		for range 3 {
			if b.mustSend() {
				t.Errorf("Send reported a post for an unapproved peer")
			}
		}
		if posts := b.posts(); len(posts) != 0 {
			t.Errorf("posts = %+v; want none", posts)
		}
		b.wantReports(1, "peer-unapproved", "peer-unapproved", "peer-unapproved")
		notices := b.wantNotices(1)
		if len(notices) == 1 && !strings.Contains(notices[0], "type lym approve osl") {
			t.Errorf("notice = %q; want it to name lym approve osl", notices[0])
		}
		answers := b.slack.logged("PostAs")
		if len(answers) != 1 || answers[0].Channel != b.entryOf(primeOrch).ID || !strings.Contains(answers[0].Arg, "type lym approve osl") {
			t.Errorf("channel notices = %+v; want one in the orch's channel naming lym approve osl", answers)
		}

		b.approve()
		if !b.mustSend() {
			t.Fatalf("Send reported no post once the peer is approved")
		}
		posts := b.posts()
		if len(posts) != 1 || posts[0].Arg != envelopeText(1, "hello") {
			t.Errorf("posts = %+v; want one carrying %q", posts, envelopeText(1, "hello"))
		}
		wantSettled := []machineCall{{Verb: "SettleDelivered", ID: 1, Arg: firstTimestamp}}
		if got := b.mailbox.machineLog("SettleDelivered"); !cmp.Equal(wantSettled, got) {
			t.Errorf("settles mismatch (-want +got):\n%s", cmp.Diff(wantSettled, got))
		}
	})

	t.Run("HoldWhoseMessageIsNoLongerTakenEndsItsEpisodeSoTheNextHoldPostsANoticeAgain", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.mailbox.setPending(b.message(1, "first"))
		b.mustSend()
		b.wantNotices(1)
		b.mailbox.setPending()
		b.mustSend()
		b.mailbox.setPending(b.message(2, "second"))
		b.mustSend()
		b.wantNotices(2)
	})

	t.Run("MessageReturnedByCreelAtItsHourIsNotPostedWhenTheHoldClears", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.mailbox.setPending(b.message(1, "hello"))
		b.mustSend()
		b.mailbox.setPending()
		b.approve()
		if b.mustSend() {
			t.Errorf("Send reported a post for a message creel no longer holds")
		}
		if posts := b.posts(); len(posts) != 0 {
			t.Errorf("posts = %+v; want none", posts)
		}
	})

	t.Run("OrchWhoseButtonIsOffIsReportedButtonOffAndAnotherOrchsMailStillPosts", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.setButton(primeOrch, false)
		other := creelengine.MachineOutbound{ID: 2, From: paymentsOrch, To: peerOrch, ReceivingMachine: "osl", Body: "x", Posted: b.clock.Now()}
		b.mailbox.setPending(b.message(1, "a"), other)
		if !b.mustSend() {
			t.Fatalf("Send reported no post for the orch whose button is on")
		}
		b.wantReports(1, "button-off")
		want := fmt.Sprintf("[lym-bridge] from=%s::%s to=%s id=2\nx", testMachine, paymentsOrch, peerOrch)
		if posts := b.posts(); len(posts) != 1 || posts[0].Arg != want {
			t.Errorf("posts = %+v; want only %q", posts, want)
		}
		b.wantNotices(0)
	})

	t.Run("FirstSendCreatesTheChannelInvitesTheOperatorAndThePeerBotRecordsTheEntryAndPosts", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.mailbox.setPending(b.message(1, "a <b> & c"))
		if !b.mustSend() {
			t.Fatalf("Send reported no post")
		}
		created := b.slack.logged("CreateChannel")
		if len(created) != 1 || created[0].Arg != bridgeName {
			t.Fatalf("CreateChannel calls = %+v; want one for %s", created, bridgeName)
		}
		if got, want := invitedTo(b.slack, "C1"), []string{testOperator, peerBot}; !cmp.Equal(got, want) {
			t.Errorf("invited = %v; want %v", got, want)
		}
		wantEntry := slackengine.BridgeEntry{Name: bridgeName, LocalOrch: primeOrch, Peer: peerOrch, Operator: testOperator, PeerInvited: true}
		if diff := cmp.Diff(map[string]slackengine.BridgeEntry{"C1": wantEntry}, b.bridgeMap().Channels); diff != "" {
			t.Errorf("bridge map mismatch (-want +got):\n%s", diff)
		}
		wantPost := slackCall{
			Method: "PostTimestamped", Channel: "C1", Arg: envelopeText(1, "a &lt;b&gt; &amp; c"),
			Sender: slackengine.Sender{Username: testMachine + "::" + primeOrch, IconEmoji: ":robot_face:"},
		}
		if posts := b.posts(); len(posts) != 1 || posts[0] != wantPost {
			t.Errorf("posts = %+v; want %+v", posts, wantPost)
		}
	})

	t.Run("TakenNameIsAdoptedWhenThisBotOrThePeerBotMadeTheChannel", func(t *testing.T) {
		t.Parallel()
		for _, creator := range []string{"UBOT", peerBot} {
			t.Run(creator, func(t *testing.T) {
				t.Parallel()
				b := newBridgeRun(t)
				b.approve()
				b.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: bridgeName, Creator: creator})
				b.slack.add("CX", slackengine.Message{TS: "50.000001", User: peerBot, Text: "older"})
				b.mailbox.setPending(b.message(1, "hello"))
				if !b.mustSend() {
					t.Fatalf("Send reported no post")
				}
				if joins := b.slack.logged("Join"); len(joins) != 1 || joins[0].Channel != "CX" {
					t.Errorf("Join calls = %+v; want one for CX", joins)
				}
				wantEntry := slackengine.BridgeEntry{
					Name: bridgeName, LocalOrch: primeOrch, Peer: peerOrch, Operator: testOperator, PeerInvited: true,
					OperatorCursor: "50.000001", ThreadFloor: "50.000001",
				}
				if diff := cmp.Diff(map[string]slackengine.BridgeEntry{"CX": wantEntry}, b.bridgeMap().Channels); diff != "" {
					t.Errorf("bridge map mismatch (-want +got):\n%s", diff)
				}
				if posts := b.posts(); len(posts) != 1 || posts[0].Channel != "CX" {
					t.Errorf("posts = %+v; want one in CX", posts)
				}
			})
		}
	})

	t.Run("ChannelMadeByAnyoneElseHoldsBridgeNameTakenWithOneNoticeAndIsTriedOncePerCeiling", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: bridgeName, Creator: "UOTHER"})
		b.mailbox.setPending(b.message(1, "hello"))
		for range 3 {
			if b.mustSend() {
				t.Errorf("Send reported a post for a held pair")
			}
		}
		b.wantReports(1, "bridge-name-taken", "bridge-name-taken", "bridge-name-taken")
		notices := b.wantNotices(1)
		if len(notices) == 1 && !strings.Contains(notices[0], "rename that channel") {
			t.Errorf("notice = %q; want it to name the rename", notices[0])
		}
		if creates, finds := len(b.slack.logged("CreateChannel")), len(b.slack.logged("FindChannel")); creates != 2 || finds != 1 {
			// One create of the prime's own channel for the notice, one of the bridge channel.
			t.Errorf("CreateChannel and FindChannel calls = %d, %d; want 2, 1", creates, finds)
		}
		b.clock.Advance(bridgeCeiling)
		b.mustSend()
		if creates, finds := len(b.slack.logged("CreateChannel")), len(b.slack.logged("FindChannel")); creates != 3 || finds != 2 {
			t.Errorf("CreateChannel and FindChannel calls after the ceiling = %d, %d; want 3, 2", creates, finds)
		}
		b.wantNotices(1)

		if err := b.slack.Rename(context.Background(), "CX", "renamed"); err != nil {
			t.Fatalf("Rename = %v", err)
		}
		b.clock.Advance(bridgeCeiling)
		if !b.mustSend() {
			t.Fatalf("Send reported no post once the name was freed")
		}
		if posts := b.posts(); len(posts) != 1 || posts[0].Channel == "CX" {
			t.Errorf("posts = %+v; want one in a channel the relay made", posts)
		}
	})

	t.Run("RefusedOperatorInviteIntoANewBridgeChannelMailsOneNoticeNamingTheCode", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.slack.failNext("Invite", &slackengine.APIError{Method: "conversations.invite", Code: "user_not_found"})
		b.mailbox.setPending(b.message(1, "first"))
		if !b.mustSend() {
			t.Fatalf("Send reported no post")
		}
		notices := b.wantNotices(1)
		if len(notices) == 1 && (!strings.Contains(notices[0], "user_not_found") || !strings.Contains(notices[0], "lym slack setup --operator")) {
			t.Errorf("notice = %q; want the code and the setup command", notices[0])
		}
		b.mailbox.setPending(b.message(2, "second"))
		b.mustSend()
		b.wantNotices(1)
	})

	t.Run("ArchivedChannelHoldsBridgeArchivedWithOneNoticeCountedTowardNoCapAndIsAdoptedOnceUnarchived", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.slack.seedChannel(slackengine.ChannelInfo{ID: "CA", Name: bridgeName, Creator: "UBOT", IsArchived: true})
		b.mailbox.setPending(b.message(1, "hello"))
		for range 6 {
			if b.mustSend() {
				t.Errorf("Send reported a post for an archived channel")
			}
			b.clock.Advance(bridgeCeiling)
		}
		b.wantReports(1, "bridge-archived", "bridge-archived", "bridge-archived", "bridge-archived", "bridge-archived", "bridge-archived")
		notices := b.wantNotices(1)
		if len(notices) == 1 && !strings.Contains(notices[0], "unarchive") {
			t.Errorf("notice = %q; want the way forward to unarchive", notices[0])
		}
		if returned := b.mailbox.machineLog("SettleReturned"); len(returned) != 0 {
			t.Errorf("returned settles = %+v; want none, a hold counts toward no cap", returned)
		}
		b.slack.unarchive(bridgeName)
		if !b.mustSend() {
			t.Fatalf("Send reported no post once the channel was unarchived")
		}
		if posts := b.posts(); len(posts) != 1 || posts[0].Channel != "CA" {
			t.Errorf("posts = %+v; want one in CA", posts)
		}
	})

	t.Run("ArchivedChannelAnyoneElseMadeHoldsNameTakenOnceUnarchivedAndIsReplacedOnceRenamed", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.slack.seedChannel(slackengine.ChannelInfo{ID: "CA", Name: bridgeName, Creator: "UOTHER", IsArchived: true})
		b.mailbox.setPending(b.message(1, "hello"))
		b.mustSend()
		b.slack.unarchive(bridgeName)
		b.clock.Advance(bridgeCeiling)
		b.mustSend()
		b.wantReports(1, "bridge-archived", "bridge-name-taken")
		if err := b.slack.Rename(context.Background(), "CA", "freed"); err != nil {
			t.Fatalf("Rename = %v", err)
		}
		b.clock.Advance(bridgeCeiling)
		if !b.mustSend() {
			t.Fatalf("Send reported no post once the channel was renamed")
		}
		if posts := b.posts(); len(posts) != 1 || posts[0].Channel == "CA" {
			t.Errorf("posts = %+v; want one in a channel the relay made", posts)
		}
	})

	t.Run("RefusedPeerInviteHoldsWithSlacksCodeIsRetriedAtEachPollAndClearsOnceInvited", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.slack.failNext("Invite", nil, &slackengine.APIError{Method: "conversations.invite", Code: "cant_invite"})
		b.mailbox.setPending(b.message(1, "hello"))
		if b.mustSend() {
			t.Errorf("Send reported a post while the peer's bot is not invited")
		}
		b.wantReports(1, "cant_invite")
		notices := b.wantNotices(1)
		if len(notices) == 1 && !strings.Contains(notices[0], "@lym-osl") {
			t.Errorf("notice = %q; want it to name @lym-osl", notices[0])
		}
		if !b.mustSend() {
			t.Fatalf("Send reported no post once the invite stood")
		}
		var bridgeInvites []slackCall
		for _, invite := range b.slack.logged("Invite") {
			if invite.Channel == "C1" {
				bridgeInvites = append(bridgeInvites, invite)
			}
		}
		if len(bridgeInvites) != 3 {
			t.Errorf("Invite calls into C1 = %+v; want the operator's, the refused one and the retry", bridgeInvites)
		}
		if !b.bridgeMap().Channels["C1"].PeerInvited {
			t.Errorf("bridge map = %+v; want the peer invited", b.bridgeMap())
		}
	})

	t.Run("RelayRestartedDuringAHoldPostsNoSecondNotice", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			prepare func(b *bridgeRun)
		}{
			{"PeerUnapproved", func(*bridgeRun) {}},
			{"BridgeNameTaken", func(b *bridgeRun) {
				b.approve()
				b.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: bridgeName, Creator: "UOTHER"})
			}},
			{"BridgeArchived", func(b *bridgeRun) {
				b.approve()
				b.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: bridgeName, Creator: "UBOT", IsArchived: true})
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b := newBridgeRun(t)
				tt.prepare(b)
				b.mailbox.setPending(b.message(1, "hello"))
				b.mustSend()
				b.restart()
				b.mustSend()
				b.wantNotices(1)
			})
		}
	})

	t.Run("SettleLostOnADroppedConnectionIsMadeAgainAtTheNextPollNotByASecondPost", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.mailbox.setPending(b.message(1, "hello"))
		b.mailbox.failMachine("SettleDelivered", lostConnection)
		posted, err := b.send()
		if !posted || !errors.Is(err, lostConnection) {
			t.Fatalf("Send = %v, %v; want a post and the lost connection", posted, err)
		}
		if b.mustSend() {
			t.Errorf("the next Send reported a post; want the settle alone")
		}
		if posts := b.posts(); len(posts) != 1 {
			t.Errorf("posts = %+v; want one", posts)
		}
		wantSettles := []machineCall{{Verb: "SettleDelivered", ID: 1, Arg: firstTimestamp}, {Verb: "SettleDelivered", ID: 1, Arg: firstTimestamp}}
		if got := b.mailbox.machineLog("SettleDelivered"); !cmp.Equal(wantSettles, got) {
			t.Errorf("settles mismatch (-want +got):\n%s", cmp.Diff(wantSettles, got))
		}
	})

	t.Run("AcceptedPostWhoseSettleWasLostIsSettledByTheNextSendEvenWhenCreelReturnedTheMessageAtItsHour", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.mailbox.setPending(b.message(1, "hello"))
		b.mailbox.failMachine("SettleDelivered", lostConnection)
		if _, err := b.send(); !errors.Is(err, lostConnection) {
			t.Fatalf("Send = %v; want the lost connection", err)
		}
		b.mailbox.setPending()
		if b.mustSend() {
			t.Errorf("the next Send reported a post; want the settle alone")
		}
		wantSettles := []machineCall{{Verb: "SettleDelivered", ID: 1, Arg: firstTimestamp}, {Verb: "SettleDelivered", ID: 1, Arg: firstTimestamp}}
		if got := b.mailbox.machineLog("SettleDelivered"); !cmp.Equal(wantSettles, got) {
			t.Errorf("settles mismatch (-want +got):\n%s", cmp.Diff(wantSettles, got))
		}
		if fileHolds(t, b.geom.BridgePostsPath, `"accepted"`) {
			t.Errorf("the entry stayed although the next send settled it")
		}
	})

	t.Run("CreateAnsweringNameTakenWithNoListedChannelListsOncePerCeiling", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		nameTaken := &slackengine.APIError{Method: "conversations.create", Code: "name_taken"}
		b.slack.failNext("CreateChannel", nameTaken, nameTaken)
		b.mailbox.setPending(b.message(1, "hello"))
		for range 3 {
			if posted, err := b.send(); posted || err == nil {
				t.Errorf("Send = %v, %v; want no post and the not-listed error", posted, err)
			}
		}
		if creates, finds := len(b.slack.logged("CreateChannel")), len(b.slack.logged("FindChannel")); creates != 1 || finds != 1 {
			t.Errorf("CreateChannel and FindChannel calls = %d, %d; want 1, 1", creates, finds)
		}
		b.clock.Advance(bridgeCeiling)
		if _, err := b.send(); err == nil {
			t.Errorf("Send after the ceiling = nil; want the not-listed error")
		}
		if creates, finds := len(b.slack.logged("CreateChannel")), len(b.slack.logged("FindChannel")); creates != 2 || finds != 2 {
			t.Errorf("CreateChannel and FindChannel calls after the ceiling = %d, %d; want 2, 2", creates, finds)
		}
	})

	t.Run("AcceptedMessageIsNotPostedAgainAfterARestartNorAnOutboundReadAndItsEntryGoesOnceCreelAnswers", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.mailbox.setPending(b.message(1, "hello"))
		b.mailbox.failMachine("SettleDelivered", lostConnection)
		if _, err := b.send(); err == nil {
			t.Fatalf("Send = nil; want the lost connection")
		}

		b.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{{ID: 5, From: primeOrch, Body: "orch mail", Posted: b.clock.Now()}}})
		out := slackengine.NewOutbound(b.geom, b.mailbox, b.slack, b.clock, b.keeper, b.hub.hub())
		if _, err := out.Read(context.Background()); err != nil {
			t.Fatalf("outbound Read = %v", err)
		}
		if !fileHolds(t, b.geom.BridgePostsPath, `"accepted"`) {
			t.Errorf("the post record lost its accepted message to an outbound read of orch mail")
		}

		b.restart()
		b.mailbox.failMachine("SettleDelivered", lostConnection)
		if err := b.sender.SettleAccepted(); !errors.Is(err, lostConnection) {
			t.Errorf("SettleAccepted = %v; want the lost connection", err)
		}
		if !fileHolds(t, b.geom.BridgePostsPath, `"accepted"`) {
			t.Errorf("the entry went although creel did not answer")
		}
		b.mailbox.failMachine("SettleDelivered", creelRefusal)
		if err := b.sender.SettleAccepted(); err != nil {
			t.Errorf("SettleAccepted = %v; want nil once creel answered with a refusal", err)
		}
		if fileHolds(t, b.geom.BridgePostsPath, `"accepted"`) {
			t.Errorf("the entry stayed although creel answered")
		}
		if posts := b.posts(); len(posts) != 1 {
			t.Errorf("posts = %+v; want the one before the restart", posts)
		}
		for _, call := range b.mailbox.machineLog("SettleDelivered") {
			if call.Arg != firstTimestamp {
				t.Errorf("settle %+v; want the recorded timestamp %s", call, firstTimestamp)
			}
		}
	})

	t.Run("MessageSlackRefusesFiveTimesIsSettledReturnedWithTheCodeAndTransportAndRateLimitCountToNothing", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		b.approve()
		b.mailbox.setPending(b.message(1, "hello"))
		refusal := &slackengine.APIError{Method: "chat.postMessage", Code: "msg_too_long"}
		b.slack.failNext("PostTimestamped",
			refusal, errors.New("transport down"), refusal, &slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: time.Second},
			refusal, refusal, refusal)
		for i := range 7 {
			if posted, err := b.send(); posted || (err == nil) != (i == 6) {
				t.Errorf("Send %d = %v, %v; want no post and an error except for the fifth refusal", i+1, posted, err)
			}
			if i == 1 {
				b.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{{ID: 5, From: primeOrch, Body: "orch mail", Posted: b.clock.Now()}}})
				out := slackengine.NewOutbound(b.geom, b.mailbox, b.slack, b.clock, b.keeper, b.hub.hub())
				if _, err := out.Read(context.Background()); err != nil {
					t.Fatalf("outbound Read = %v", err)
				}
			}
		}
		wantReturned := []machineCall{{Verb: "SettleReturned", ID: 1, Arg: "msg_too_long"}}
		if got := b.mailbox.machineLog("SettleReturned"); !cmp.Equal(wantReturned, got) {
			t.Errorf("returned settles mismatch (-want +got):\n%s", cmp.Diff(wantReturned, got))
		}
		if posts := b.posts(); len(posts) != 7 {
			t.Errorf("post attempts = %d; want 7", len(posts))
		}
		b.mustSend()
		if fileHolds(t, b.geom.BridgePostsPath, `"refusals"`) {
			t.Errorf("the refusal count stayed once take no longer returned the message")
		}
	})

	t.Run("PostRefusedIsArchivedOrChannelNotFoundDropsTheEntryAndTheNextSendFindsTheChannelAgain", func(t *testing.T) {
		t.Parallel()
		for _, code := range []string{"is_archived", "channel_not_found"} {
			t.Run(code, func(t *testing.T) {
				t.Parallel()
				b := newBridgeRun(t)
				b.approve()
				b.mailbox.setPending(b.message(1, "hello"))
				b.slack.failNext("PostTimestamped", &slackengine.APIError{Method: "chat.postMessage", Code: code})
				if posted, err := b.send(); posted || err == nil {
					t.Fatalf("Send = %v, %v; want no post and the refusal", posted, err)
				}
				if channels := b.bridgeMap().Channels; len(channels) != 0 {
					t.Errorf("bridge map = %+v; want the entry dropped", channels)
				}
				if !b.mustSend() {
					t.Fatalf("the next Send reported no post")
				}
				if creates := len(b.slack.logged("CreateChannel")); creates != 2 {
					t.Errorf("CreateChannel calls = %d; want the first make and the make again", creates)
				}
				if _, held := b.bridgeMap().Channels["C1"]; !held {
					t.Errorf("bridge map = %+v; want the channel recorded again", b.bridgeMap())
				}
			})
		}
	})

	t.Run("RecordsKeepAKeyTheyDoNotKnowAcrossAWrite", func(t *testing.T) {
		t.Parallel()
		b := newBridgeRun(t)
		if err := os.MkdirAll(b.geom.Dir, 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(b.geom.BridgePostsPath, []byte(`{"accepted":{"7":"1.000001"},"future_posts_key":true}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if err := os.WriteFile(b.geom.BridgeNoticesPath, []byte(`{"marks":{},"future_notices_key":1}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if err := b.sender.SettleAccepted(); err != nil {
			t.Fatalf("SettleAccepted = %v", err)
		}
		b.mailbox.setPending(b.message(1, "hello"))
		b.mustSend()
		if !fileHolds(t, b.geom.BridgePostsPath, "future_posts_key") {
			t.Errorf("the post record dropped a key it does not know")
		}
		if !fileHolds(t, b.geom.BridgeNoticesPath, "future_notices_key") {
			t.Errorf("the notice record dropped a key it does not know")
		}
	})

	t.Run("PollWaitsDoubleFromFiveToSixtySecondsWhileNothingIsNewAndReturnToFiveAfterAPost", func(t *testing.T) {
		t.Parallel()
		clock := newManualClock()
		poll := slackengine.NewBridgePoll(clock)
		var rounds atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			poll.Run(ctx, func(context.Context) bool { return rounds.Add(1) == 7 })
		}()
		t.Cleanup(func() {
			cancel()
			awaitClosed(t, done)
		})
		for _, wait := range []time.Duration{5, 10, 20, 40, 60, 60, 5, 5} {
			wait *= time.Second
			clock.awaitWait(t, wait)
			clock.Advance(wait)
		}
		clock.awaitWait(t, 10*time.Second)
	})

	t.Run("WakeEndsThePollsWaitAtOnceButNotWithinTheWakeFloorOfTheRoundsStart", func(t *testing.T) {
		t.Parallel()
		clock := newManualClock()
		poll := slackengine.NewBridgePoll(clock)
		var rounds atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			poll.Run(ctx, func(context.Context) bool { rounds.Add(1); return false })
		}()
		t.Cleanup(func() {
			cancel()
			awaitClosed(t, done)
		})
		clock.awaitWait(t, 5*time.Second)
		poll.Wake()
		clock.awaitWait(t, slackengine.WakeFloor)
		if got := rounds.Load(); got != 1 {
			t.Errorf("rounds before the floor passed = %d; want 1", got)
		}
		clock.Advance(slackengine.WakeFloor)
		eventually(t, "the woken round to run", func() bool { return rounds.Load() == 2 })
	})

	t.Run("RelaySettlesAcceptedMessagesBeforeAnyTakeAndPostsTheRest", func(t *testing.T) {
		t.Parallel()
		r := newRelayRun(t)
		approvePeer(t, r.geom, r.harness.clock.Now())
		if err := os.MkdirAll(r.geom.Dir, 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(r.geom.BridgePostsPath, []byte(`{"accepted":{"9":"1.000009"}}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		accepted := creelengine.MachineOutbound{ID: 9, From: primeOrch, To: peerOrch, ReceivingMachine: "osl", Body: "old", Posted: r.harness.clock.Now()}
		fresh := creelengine.MachineOutbound{ID: 10, From: primeOrch, To: peerOrch, ReceivingMachine: "osl", Body: "new", Posted: r.harness.clock.Now()}
		r.mailbox.setPending(accepted, fresh)
		r.start()
		eventually(t, "the relay to post the fresh message", func() bool { return len(r.slack.logged("PostTimestamped")) == 1 })
		eventually(t, "both messages to be settled", func() bool { return len(r.mailbox.machineLog("SettleDelivered")) == 2 })
		settles := r.mailbox.machineLog("SettleDelivered")
		if settles[0] != (machineCall{Verb: "SettleDelivered", ID: 9, Arg: "1.000009"}) {
			t.Errorf("first settle = %+v; want the accepted message settled with its recorded timestamp", settles[0])
		}
		if posts := r.slack.logged("PostTimestamped"); posts[0].Arg != envelopeText(10, "new") {
			t.Errorf("post = %q; want the fresh message", posts[0].Arg)
		}
	})
}
