// outbound_test.go covers the outbound read of the relay's mailbox over the in-memory seams of inbound_test.go.

package slackengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// outboundHarness is an outbound read over the harness's seams, whose hub lists the payments beam.
type outboundHarness struct {
	*harness
	out *slackengine.Outbound
}

// newOutboundHarness returns an outbound read whose keeper is the harness's.
// Its Slack directory holds both channels of the map under names the relay never makes.
// So a channel the keeper makes is a new one.
func newOutboundHarness(t *testing.T) *outboundHarness {
	t.Helper()
	h := newHarness(t)
	h.hub.set(func(h *fakeHub) { h.beams = []string{"payments"} })
	h.slack.seedChannel(slackengine.ChannelInfo{ID: testChannel, Name: "ly-neve-prime-held", Creator: "UBOT", IsMember: true})
	h.slack.seedChannel(slackengine.ChannelInfo{ID: paymentsChannel, Name: "ly-neve-payments-held", Creator: "UBOT", IsMember: true})
	return &outboundHarness{harness: h, out: slackengine.NewOutbound(h.geom, h.mailbox, h.slack, h.clock, h.keeper, h.hub.hub())}
}

// mailFrom returns a message of id from sender, posted a minute before the harness's clock.
func (h *outboundHarness) mailFrom(id int64, from, body string) creelengine.Message {
	return creelengine.Message{ID: id, From: from, Body: body, Kind: "mail", Posted: h.clock.Now().Add(-time.Minute)}
}

func (h *outboundHarness) read() (time.Duration, error) {
	h.t.Helper()
	return h.out.Read(context.Background())
}

func (h *outboundHarness) mustRead() time.Duration {
	h.t.Helper()
	wait, err := h.read()
	if err != nil {
		h.t.Fatalf("Read = %v", err)
	}
	return wait
}

func (h *outboundHarness) wantAcks(want ...[]int64) {
	h.t.Helper()
	if got := h.mailbox.acknowledged(); !cmp.Equal(got, append([][]int64(nil), want...)) && !(len(got) == 0 && len(want) == 0) {
		h.t.Errorf("acknowledgements mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

// wantPosted fails the test unless Slack accepted exactly the posts, each with its channel and sender.
func (h *outboundHarness) wantPosted(want ...postedText) {
	h.t.Helper()
	if got := h.slack.postedTexts(); !cmp.Equal(got, append([]postedText(nil), want...)) && !(len(got) == 0 && len(want) == 0) {
		h.t.Errorf("Slack posts mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

// dropEntry removes the orch's entry from the harness's channel map.
func (h *outboundHarness) dropEntry(orch string) {
	h.t.Helper()
	err := slackengine.UpdateChannelMap(h.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		delete(channelMap.Channels, orch)
		return nil
	})
	if err != nil {
		h.t.Fatalf("UpdateChannelMap = %v", err)
	}
}

// refusalCount returns how often the outbox record counts Slack's refusals of message id.
func (h *outboundHarness) refusalCount(id string) int {
	h.t.Helper()
	data, err := os.ReadFile(h.geom.OutboxPath)
	if err != nil {
		return 0
	}
	var record struct {
		Refusals map[string]int `json:"refusals"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		h.t.Fatalf("decode the outbox record %q = %v", data, err)
	}
	return record.Refusals[id]
}

// postedTexts returns the posts the fake accepted, oldest first, with their channels and senders.
func (s *fakeSlack) postedTexts() []postedText {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]postedText(nil), s.posted...)
}

// orchSender is the sender of a post for the orch strand called name: the creel name behind the machine's name.
func orchSender(name string) slackengine.Sender {
	return slackengine.Sender{Username: testMachine + "::" + name, IconEmoji: ":robot_face:"}
}

// programSender is the sender of a post for the program called name, or of the relay's own line.
// The machine's name comes first.
func programSender(name string) slackengine.Sender {
	return slackengine.Sender{Username: testMachine + "::" + name, IconEmoji: ":gear:"}
}

func TestOutboundRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const logCommand = "lym creel log --to ly:SLACK"

	t.Run("MailFromOrchProgramAndReplyIsPostedToTheOrchsChannelOnceAndAcknowledged", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		reply := h.mailFrom(3, primeOrch, "answer")
		reply.Kind, reply.ReplyTo = "reply", 9
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, primeOrch, "hello"), h.mailFrom(2, "ly:BATTEN", "done"), reply,
		}})
		if wait := h.mustRead(); wait != slackengine.Floor {
			t.Errorf("Read wait = %s; want the floor %s", wait, slackengine.Floor)
		}
		h.wantPosted(
			postedText{testChannel, "hello", orchSender(primeOrch)},
			postedText{testChannel, "done", programSender("ly:BATTEN")},
			postedText{testChannel, "answer", orchSender(primeOrch)},
		)
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("BeamMailIsPostedEscapedToItsBeamsChannelWithNoAnswerHint", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, primeOrch, "a & <b>"),
			h.mailFrom(2, paymentsOrch, "x"),
			h.mailFrom(3, "ly:payments:DOBBY", "y"),
		}})
		h.mustRead()
		h.wantPosted(
			postedText{testChannel, "a &amp; &lt;b&gt;", orchSender(primeOrch)},
			postedText{paymentsChannel, "x", orchSender(paymentsOrch)},
			postedText{paymentsChannel, "y", programSender("ly:payments:DOBBY")},
		)
	})

	t.Run("NonOrchStrandAndExternalMailIsDroppedAndAcknowledged", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, "ly:payments:worker", "from a strand"),
			h.mailFrom(2, "ly:EXTERNAL", "from outside"),
		}})
		h.mustRead()
		h.wantAnswers()
		h.wantAcks([]int64{1, 2})
	})

	t.Run("MailRoutedToAnOrchWhoseButtonIsOffIsDroppedAndAcknowledgedUnposted", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			prepare func(h *outboundHarness)
			mail    func(h *outboundHarness) []creelengine.Message
			// wantTexts are the texts Slack accepts.
			wantTexts []string
			wantAcks  []int64
		}{
			{"OrchWithAnOpenChannel", func(h *outboundHarness) { h.setButton(paymentsOrch, false) },
				func(h *outboundHarness) []creelengine.Message {
					return []creelengine.Message{h.mailFrom(1, primeOrch, "kept"), h.mailFrom(2, paymentsOrch, "dropped")}
				}, []string{"kept"}, []int64{1, 2}},
			{"GoneBeamRedirectedToAPrimeThatHasNoChannelAndNoButton", func(h *outboundHarness) {
				h.setButton("ly:gone:orch", true)
				h.setButton(primeOrch, false)
				h.dropEntry(primeOrch)
			}, func(h *outboundHarness) []creelengine.Message {
				return []creelengine.Message{h.mailFrom(1, "ly:gone:orch", "bye")}
			}, nil, []int64{1}},
			{"StaleLineOfAPrimeThatHasNoChannelAndNoButton", func(h *outboundHarness) {
				h.setButton(primeOrch, false)
				h.dropEntry(primeOrch)
			}, func(h *outboundHarness) []creelengine.Message {
				old := h.mailFrom(1, paymentsOrch, "old")
				old.Posted = h.clock.Now().Add(-25 * time.Hour)
				return []creelengine.Message{old, h.mailFrom(2, paymentsOrch, "fresh")}
			}, []string{"fresh"}, []int64{1, 2}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				tt.prepare(h)
				h.mailbox.scriptReadWait(readStep{messages: tt.mail(h)})
				h.mustRead()
				if diff := cmp.Diff(tt.wantTexts, h.slack.texts()); diff != "" {
					t.Errorf("Slack texts mismatch (-want +got):\n%s", diff)
				}
				h.wantAcks(tt.wantAcks)
				if created := len(h.slack.logged("CreateChannel")); created != 0 {
					t.Errorf("CreateChannel calls = %d; want 0 for a button that is off", created)
				}
				if record, err := os.ReadFile(h.geom.OutboxPath); err == nil && strings.Contains(string(record), "stale") {
					t.Errorf("outbox record = %q; want the stale count cleared", record)
				}
			})
		}
	})

	t.Run("MailFromAListedBeamWithNoEntryCreatesItsChannelAtThatMail", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.dropEntry(paymentsOrch)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, paymentsOrch, "first")}})
		h.mustRead()
		if created := h.slack.logged("CreateChannel"); len(created) != 1 || created[0].Arg != paymentsChannelName {
			t.Errorf("CreateChannel calls = %+v; want one for %s", created, paymentsChannelName)
		}
		entry := h.entryOf(paymentsOrch)
		if entry.ID == "" || entry.ID == testChannel || entry.ID == paymentsChannel {
			t.Errorf("payments entry = %+v; want a new channel", entry)
		}
		h.wantPosted(postedText{entry.ID, "first", orchSender(paymentsOrch)})
	})

	t.Run("MailFromABeamTheHubDoesNotListIsPostedToThePrimesChannelUnderItsName", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.setButton("ly:gone:orch", true)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, "ly:gone:orch", "bye")}})
		h.mustRead()
		h.wantPosted(postedText{testChannel, "bye", orchSender("ly:gone:orch")})
		if created := len(h.slack.logged("CreateChannel")); created != 0 {
			t.Errorf("CreateChannel calls = %d; want a gone beam to reopen nothing", created)
		}
		h.wantAcks([]int64{1})
	})

	t.Run("BeamWithNoOpenChannelWhileTheListingFailsHoldsOnlyItsOwnMail", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.hub.set(func(h *fakeHub) { h.beamsErr = errors.New("board unreadable") })
		h.setButton("ly:other:orch", true)
		batch := []creelengine.Message{
			h.mailFrom(1, "ly:other:orch", "first"), h.mailFrom(2, primeOrch, "second"), h.mailFrom(3, paymentsOrch, "third"),
		}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch}, readStep{messages: batch})
		for i := range 2 {
			if _, err := h.read(); err == nil || !strings.Contains(err.Error(), "no open channel") {
				t.Fatalf("Read #%d = %v; want the error that the beam has no open channel", i+1, err)
			}
		}
		want := []postedText{
			{testChannel, "second", orchSender(primeOrch)},
			{paymentsChannel, "third", orchSender(paymentsOrch)},
		}
		h.wantPosted(want...)
		h.wantAcks()
		h.hub.set(func(h *fakeHub) { h.beamsErr = nil })
		h.mustRead()
		h.wantPosted(append(want, postedText{testChannel, "first", orchSender("ly:other:orch")})...)
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("PostSlackRefusesRecoversOnceAndIsAcknowledged", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			code        string
			wantJoin    bool
			wantCreated int
			// wantChannel is the channel the message ends up in, which is the entry's after the recovery.
			wantChannel string
		}{
			{"IsArchivedCreatesANewChannel", "is_archived", false, 1, "C3"},
			{"NotInChannelJoinsTheBot", "not_in_channel", true, 0, paymentsChannel},
			{"ChannelNotFoundDropsTheEntryAndCreatesTheChannelAnew", "channel_not_found", false, 1, "C3"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				h.slack.failPosts(&slackengine.APIError{Method: "chat.postMessage", Code: tt.code})
				h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, paymentsOrch, "x")}})
				h.mustRead()
				h.wantPosted(postedText{tt.wantChannel, "x", orchSender(paymentsOrch)})
				h.wantAcks([]int64{1})
				if entry := h.entryOf(paymentsOrch); entry.ID != tt.wantChannel || entry.Archived {
					t.Errorf("payments entry = %+v; want it open on %s", entry, tt.wantChannel)
				}
				if joins := len(h.slack.logged("Join")); (joins == 1) != tt.wantJoin || joins > 1 {
					t.Errorf("Join calls = %d; want a join exactly when %v", joins, tt.wantJoin)
				}
				if created := len(h.slack.logged("CreateChannel")); created != tt.wantCreated {
					t.Errorf("CreateChannel calls = %d; want %d", created, tt.wantCreated)
				}
			})
		}
	})

	t.Run("RefusalFollowedByARefusedJoinCountsOnceTowardTheCap", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.slack.failPosts(&slackengine.APIError{Method: "chat.postMessage", Code: "not_in_channel"})
		h.slack.failNext("Join", &slackengine.APIError{Method: "conversations.join", Code: "missing_scope"})
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, primeOrch, "x")}})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the refused post")
		}
		if got := h.refusalCount("1"); got != 1 {
			t.Errorf("refusals counted = %d; want 1", got)
		}
	})

	t.Run("SlackFailureMidReadPostsEachMessageExactlyOnce", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "one"), h.mailFrom(2, primeOrch, "two"), h.mailFrom(3, primeOrch, "three")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.slack.failPosts(nil, errors.New("slack down"))
		if wait, err := h.read(); err == nil || wait != slackengine.Floor {
			t.Fatalf("Read = %s, %v; want the post error and the floor", wait, err)
		}
		h.wantAcks()
		h.mustRead()
		h.wantAnswers("one", "two", "three")
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("MessageSlackRefusesFiveTimesIsDroppedWhetherOrNotTheReplacementLinePosts", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// replacementFailure is what the post of the replacement line itself answers; nil posts it.
			replacementFailure error
			wantLine           bool
		}{
			{"ReplacementLinePosts", nil, true},
			{"ReplacementLinePostFails", errors.New("slack down"), false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				refusal := &slackengine.APIError{Method: "chat.postMessage", Code: "msg_too_long"}
				h.slack.failPosts(refusal, refusal, refusal, refusal, refusal, tt.replacementFailure)
				for range 5 {
					h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(7, primeOrch, "huge")}})
				}
				for i := range 4 {
					if _, err := h.read(); !errors.Is(err, refusal) {
						t.Fatalf("Read #%d = %v; want the refusal", i+1, err)
					}
				}
				h.wantAcks()
				h.mustRead()
				posted := h.slack.postedTexts()
				switch {
				case tt.wantLine && (len(posted) != 1 || posted[0].Channel != testChannel || posted[0].Sender != programSender("ly:SLACK") ||
					!strings.Contains(posted[0].Text, "message 7") || !strings.Contains(posted[0].Text, primeOrch) || !strings.Contains(posted[0].Text, logCommand)):
					t.Errorf("posts = %+v; want one line to %s from ly:SLACK naming message 7, its sender and the log command", posted, testChannel)
				case !tt.wantLine && len(posted) != 0:
					t.Errorf("posts = %+v; want none, the replacement line failed", posted)
				}
				h.wantAcks([]int64{7})
				if calls := len(h.slack.logged("PostAs")); calls != 6 {
					t.Errorf("chat.postMessage calls = %d; want 6, five refusals and one try of the replacement line, so the message is not retried", calls)
				}
				record, err := os.ReadFile(h.geom.OutboxPath)
				if err != nil || strings.Contains(string(record), "refusals") {
					t.Errorf("outbox record = %q, %v; want no refusal count after the acknowledgement", record, err)
				}
			})
		}
	})

	t.Run("ListedBeamWhoseChannelIsRefusedCountsPerReadAndDropsAtTheCap", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			primeRefused bool
		}{
			{"LineReachesThePrimesChannel", false},
			{"PrimeRefusedTooSoTheLineReachesTheOrchAsANotice", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				h.dropEntry(paymentsOrch)
				h.slack.seedChannel(slackengine.ChannelInfo{ID: "CX", Name: paymentsChannelName, Creator: "UOTHER", IsMember: true})
				if tt.primeRefused {
					h.dropEntry(primeOrch)
					h.slack.seedChannel(slackengine.ChannelInfo{ID: "CY", Name: primeChannelName, Creator: "UOTHER", IsMember: true})
				}
				for range 5 {
					h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(7, paymentsOrch, "x")}})
				}
				for i := range 4 {
					if _, err := h.read(); !errors.Is(err, slackengine.ErrChannelRefused) {
						t.Fatalf("Read #%d = %v; want the refused channel", i+1, err)
					}
					if got := h.refusalCount("7"); got != i+1 {
						t.Errorf("refusals counted after read #%d = %d; want %d", i+1, got, i+1)
					}
				}
				h.mustRead()
				h.wantAcks([]int64{7})
				posted := h.slack.postedTexts()
				if tt.primeRefused {
					if len(posted) != 0 {
						t.Errorf("posts = %+v; want none while both channels are refused", posted)
					}
					found := false
					for _, notice := range h.relayNotices() {
						found = found || (strings.Contains(notice, "message 7") && strings.Contains(notice, logCommand))
					}
					if !found {
						t.Errorf("notices = %q; want one carrying the line about message 7", h.relayNotices())
					}
					return
				}
				if len(posted) != 1 || posted[0].Channel != testChannel || !strings.Contains(posted[0].Text, "message 7") {
					t.Errorf("posts = %+v; want the line about message 7 in %s", posted, testChannel)
				}
			})
		}
	})

	t.Run("ListedBeamWhoseEnsureFailsOnATransportErrorReleasesTheReadWithoutACount", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.dropEntry(paymentsOrch)
		down := errors.New("slack: call conversations.create: connection reset")
		h.slack.failNext("CreateChannel", down)
		batch := []creelengine.Message{h.mailFrom(5, paymentsOrch, "x")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		if _, err := h.read(); !errors.Is(err, down) {
			t.Fatalf("Read = %v; want the transport error", err)
		}
		if got := h.refusalCount("5"); got != 0 {
			t.Errorf("refusals counted = %d; want none for a transport failure", got)
		}
		h.wantPosted()
		h.mustRead()
		h.wantPosted(postedText{h.entryOf(paymentsOrch).ID, "x", orchSender(paymentsOrch)})
		h.wantAcks([]int64{5})
	})

	t.Run("TransportFailureAndServerErrorNeverCapOutAMessage", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			err  error
		}{
			{"TransportFailure", errors.New("slack: call chat.postMessage: connection reset")},
			{"ServerError", errors.New("slack: chat.postMessage answered HTTP 500")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				for range 7 {
					h.slack.failPosts(tt.err)
					h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, primeOrch, "keep me")}})
					if _, err := h.read(); !errors.Is(err, tt.err) {
						t.Fatalf("Read = %v; want the failure", err)
					}
				}
				h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, primeOrch, "keep me")}})
				h.mustRead()
				h.wantAnswers("keep me")
				h.wantAcks([]int64{1})
			})
		}
	})

	t.Run("BacklogPastThePostingBudgetIsReleasedAndPostedOnceAcrossReads", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.slack.onPost = func() { h.clock.Advance(15 * time.Second) }
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "one"), h.mailFrom(2, primeOrch, "two"), h.mailFrom(3, primeOrch, "three")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.mustRead()
		h.wantAnswers("one", "two")
		h.wantAcks()
		h.mustRead()
		h.wantAnswers("one", "two", "three")
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("MailOlderThanADayIsAcknowledgedUnpostedWithOneCountLineInThePrimesChannel", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		old1, old2 := h.mailFrom(1, primeOrch, "old"), h.mailFrom(2, paymentsOrch, "older")
		old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{old1, old2, h.mailFrom(3, primeOrch, "fresh")}})
		h.mustRead()
		posted := h.slack.postedTexts()
		if len(posted) != 2 || posted[0].Text != "fresh" || posted[1].Channel != testChannel || posted[1].Sender != programSender("ly:SLACK") ||
			!strings.Contains(posted[1].Text, "2 messages") || !strings.Contains(posted[1].Text, logCommand) {
			t.Errorf("posts = %+v; want the fresh mail and one line to %s naming the count and the log command", posted, testChannel)
		}
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("StaleLineRefusedWithNotInChannelJoinsThePrimesChannelAndPostsAndClearsTheCount", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		old := h.mailFrom(1, primeOrch, "old")
		old.Posted = h.clock.Now().Add(-25 * time.Hour)
		h.slack.failPosts(&slackengine.APIError{Method: "chat.postMessage", Code: "not_in_channel"})
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{old}})
		h.mustRead()
		if joins := h.slack.logged("Join"); len(joins) != 1 || joins[0].Channel != testChannel {
			t.Errorf("Join calls = %+v; want one for %s", joins, testChannel)
		}
		if posted := h.slack.postedTexts(); len(posted) != 1 || posted[0].Channel != testChannel || !strings.Contains(posted[0].Text, "1 messages") {
			t.Errorf("posts = %+v; want the stale line in %s", posted, testChannel)
		}
		if record, err := os.ReadFile(h.geom.OutboxPath); err != nil || strings.Contains(string(record), "stale") {
			t.Errorf("outbox record = %q, %v; want the stale count cleared", record, err)
		}
		h.wantAcks([]int64{1})
	})

	t.Run("StaleMailCountLineSurvivesAFailedReadAndPostsOnce", func(t *testing.T) {
		t.Parallel()
		slackDown := errors.New("slack down")
		tests := []struct {
			name string
			// failures are the answers of the first posts: the fresh message's, then the count line's.
			failures []error
			// wantLineIndex is where the count line stands among the texts Slack accepted.
			wantLineIndex int
		}{
			{"LinePostsBeforeTheFailedReadReleases", []error{slackDown}, 0},
			{"LineThatFailsToPostWaitsForTheNextRead", []error{slackDown, slackDown}, 1},
			{"LineWaitsOutARateLimitInsteadOfPostingIntoIt", []error{&slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: time.Second}}, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				old1, old2 := h.mailFrom(1, primeOrch, "old"), h.mailFrom(2, primeOrch, "older")
				old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
				batch := []creelengine.Message{old1, old2, h.mailFrom(3, primeOrch, "fresh")}
				h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
				h.slack.failPosts(tt.failures...)
				if _, err := h.read(); !errors.Is(err, tt.failures[0]) {
					t.Fatalf("Read = %v; want the failed post", err)
				}
				h.wantAcks()
				h.mustRead()
				answers := h.slack.texts()
				if len(answers) != 2 || answers[1-tt.wantLineIndex] != "fresh" ||
					!strings.Contains(answers[tt.wantLineIndex], "2 messages") || !strings.Contains(answers[tt.wantLineIndex], logCommand) {
					t.Errorf("answers = %q; want the fresh mail once and one count line naming 2 messages at index %d", answers, tt.wantLineIndex)
				}
				h.wantAcks([]int64{1, 2, 3})
			})
		}
	})

	t.Run("OutboxWriteFailingAfterAnAcceptedPostAcknowledgesTheMailAndNeverRepostsIt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// ackRefused makes creel refuse the first read's acknowledgement, so the batch is delivered again.
			ackRefused bool
		}{
			{"AcknowledgementConfirmed", false},
			{"AcknowledgementRefusedAndTheBatchRedelivered", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				// A directory in the outbox file's place makes every write of the record fail once Slack accepted the post.
				h.slack.onPost = func() {
					if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
						t.Errorf("MkdirAll = %v", err)
					}
				}
				batch := []creelengine.Message{h.mailFrom(1, primeOrch, "once")}
				steps := []readStep{{messages: batch}}
				if tt.ackRefused {
					steps = []readStep{{messages: batch, ackErr: errors.New("daemon refused")}, {messages: batch}}
				}
				h.mailbox.scriptReadWait(steps...)
				for i := range steps {
					if _, err := h.read(); err == nil || !strings.Contains(err.Error(), "outbox file") {
						t.Errorf("Read #%d = %v; want the failed write of the outbox file", i+1, err)
					}
				}
				h.wantAnswers("once")
				h.wantAcks([]int64{1})
			})
		}
	})

	t.Run("StaleMailLineIsPostedOnceWhenTheOutboxWriteFails", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		// The directory appears at the first post, before any write, so every write of the record fails.
		h.slack.onPost = func() {
			if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
				t.Errorf("MkdirAll = %v", err)
			}
		}
		old1, old2 := h.mailFrom(2, primeOrch, "old"), h.mailFrom(3, primeOrch, "older")
		old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "fresh"), old1, old2}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")}, readStep{messages: batch})
		for i := range 2 {
			if _, err := h.read(); err == nil || !strings.Contains(err.Error(), "outbox file") {
				t.Errorf("Read #%d = %v; want the failed write of the outbox file", i+1, err)
			}
		}
		lines := 0
		for _, text := range h.slack.texts() {
			if strings.Contains(text, "2 messages") && strings.Contains(text, logCommand) {
				lines++
			}
		}
		if lines != 1 {
			t.Errorf("count lines = %d in %q; want 1", lines, h.slack.texts())
		}
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("OutboxWriteFailureRidesAlongWithAFailedPostAndWithTheEndedBudget", func(t *testing.T) {
		t.Parallel()
		slackDown := errors.New("slack down")
		tests := []struct {
			name string
			// advance is how far each post moves the clock.
			advance time.Duration
			// failures are the answers of the first posts.
			failures []error
			// wantPostFailure is whether the read's error must also carry the failed post.
			wantPostFailure bool
		}{
			{"FailedPost", 0, []error{nil, slackDown}, true},
			{"PostingBudgetEnded", 15 * time.Second, nil, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				h.slack.onPost = func() {
					h.clock.Advance(tt.advance)
					if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
						t.Errorf("MkdirAll = %v", err)
					}
				}
				h.slack.failPosts(tt.failures...)
				batch := []creelengine.Message{h.mailFrom(1, primeOrch, "one"), h.mailFrom(2, primeOrch, "two"), h.mailFrom(3, primeOrch, "three")}
				h.mailbox.scriptReadWait(readStep{messages: batch})
				_, err := h.read()
				if err == nil || !strings.Contains(err.Error(), "outbox file") {
					t.Fatalf("Read = %v; want the failed write of the outbox file", err)
				}
				if tt.wantPostFailure && !errors.Is(err, slackDown) {
					t.Errorf("Read = %v; want it to carry the failed post too", err)
				}
			})
		}
	})

	t.Run("OutboxFileWritableAgainStandsInPlaceOfTheRecordHeldInMemory", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.slack.onPost = func() {
			if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
				t.Errorf("MkdirAll = %v", err)
			}
		}
		rest := []creelengine.Message{h.mailFrom(2, primeOrch, "two"), h.mailFrom(3, primeOrch, "three"), h.mailFrom(4, primeOrch, "four")}
		h.mailbox.scriptReadWait(
			readStep{messages: []creelengine.Message{h.mailFrom(1, primeOrch, "one")}},
			readStep{messages: rest},
			readStep{messages: rest},
		)
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the failed write of the outbox file")
		}
		if err := os.Remove(h.geom.OutboxPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		// The second read ends on the posting budget, so it leaves the record in the file instead of clearing it.
		h.slack.onPost = func() { h.clock.Advance(15 * time.Second) }
		if _, err := h.read(); err != nil {
			t.Fatalf("Read = %v; want nil once the file is writable again", err)
		}
		record, err := os.ReadFile(h.geom.OutboxPath)
		if err != nil || !strings.Contains(string(record), "handled") {
			t.Errorf("outbox record = %q, %v; want the file to hold the handled messages", record, err)
		}
		// With the file standing in for memory again, the third read skips the messages the second one handled.
		h.mustRead()
		h.wantAnswers("one", "two", "three", "four")
	})

	t.Run("RefusedAcknowledgementKeepsTheRecordSoNothingIsRepostedTwice", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "once")}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")}, readStep{messages: batch})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the refused acknowledgement")
		}
		h.mustRead()
		h.wantAnswers("once")
		h.wantAcks([]int64{1})
	})

	t.Run("RateLimitReturnsItsRetryAfterAndRaisesTheFloor", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "limited")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.slack.failPosts(&slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: 7 * time.Second})
		var limit *slackengine.RateLimitError
		if wait, err := h.read(); !errors.As(err, &limit) || wait != 7*time.Second {
			t.Fatalf("Read = %s, %v; want the 7-second retry wait", wait, err)
		}
		if wait := h.mustRead(); wait != 2*slackengine.Floor {
			t.Errorf("Read wait after the limit = %s; want the doubled floor %s", wait, 2*slackengine.Floor)
		}
	})

	t.Run("EmptyWaitBacksOffAndKeepsTheRecord", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "once")}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the refused acknowledgement")
		}
		var waits []time.Duration
		for range 6 {
			waits = append(waits, h.mustRead())
		}
		if want := []time.Duration{2 * slackengine.Floor, 4 * slackengine.Floor, 8 * slackengine.Floor, slackengine.Ceiling, slackengine.Ceiling, slackengine.Ceiling}; !cmp.Equal(waits, want) {
			t.Errorf("idle waits = %v; want %v", waits, want)
		}
		h.mailbox.scriptReadWait(readStep{messages: batch})
		h.mustRead()
		h.wantAnswers("once")
	})

	t.Run("EmptyPollThatWaitedOutTheFloorIsFollowedAtOnceByTheNext", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.mailbox.onReadWait = func() { h.clock.Advance(slackengine.Ceiling) }
		for i := range 3 {
			if wait := h.mustRead(); wait != 0 {
				t.Errorf("Read #%d wait = %s; want none after a poll that lasted its timeout", i+1, wait)
			}
		}
	})

	t.Run("EmptyPollsBackOffAndAPollThatWaitedItOutResetsTheWaitToTheFloor", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		var advance time.Duration
		h.mailbox.onReadWait = func() { h.clock.Advance(advance) }
		var waits []time.Duration
		for _, elapsed := range []time.Duration{0, 0, slackengine.Ceiling, 0} {
			advance = elapsed
			waits = append(waits, h.mustRead())
		}
		want := []time.Duration{2 * slackengine.Floor, 4 * slackengine.Floor, 0, 2 * slackengine.Floor}
		if !cmp.Equal(waits, want) {
			t.Errorf("waits = %v; want %v", waits, want)
		}
	})

	t.Run("EmptyPollOfExactlyTheRaisedFloorIsFollowedAtOnceByTheNext", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "limited")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.slack.failPosts(&slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: time.Second})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the rate limit")
		}
		h.mustRead()
		h.mailbox.onReadWait = func() { h.clock.Advance(2 * slackengine.Floor) }
		if wait := h.mustRead(); wait != 0 {
			t.Errorf("Read wait after a poll of the raised floor = %s; want none", wait)
		}
		h.mailbox.onReadWait = func() { h.clock.Advance(2*slackengine.Floor - time.Nanosecond) }
		if wait := h.mustRead(); wait != 4*slackengine.Floor {
			t.Errorf("Read wait after a poll just under the raised floor = %s; want %s", wait, 4*slackengine.Floor)
		}
	})

	t.Run("TimeoutAfterTheHandlerRanKeepsTheRecordAndReturnsTheFloor", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, primeOrch, "once")}
		h.mailbox.scriptReadWait(
			readStep{messages: batch, timeoutAfterHandler: true},
			readStep{messages: batch, timeoutAfterHandler: true},
		)
		for i := range 2 {
			if wait, err := h.read(); wait != slackengine.Floor || !errors.Is(err, creelengine.ErrWaitTimeout) {
				t.Fatalf("Read #%d = %s, %v; want the floor with the lost confirmation", i+1, wait, err)
			}
		}
		h.wantAnswers("once")
	})

	t.Run("ReadWithTheSetupFileGoneReturnsAnErrorNamingTheWayForwardAndPostsNothing", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		if err := os.Remove(h.geom.SetupPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, primeOrch, "x")}})
		_, err := h.read()
		if err == nil || !strings.Contains(err.Error(), h.geom.SetupPath) || !strings.Contains(err.Error(), "lym slack setup --operator <user-id>") {
			t.Errorf("Read = %v; want an error naming %s and lym slack setup --operator <user-id>", err, h.geom.SetupPath)
		}
		h.wantPosted()
	})
}
