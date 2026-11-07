// inbound_test.go covers the inbound poll and the presence notices.
// It also declares the in-memory Slack, mailbox, hub and clock that the outbound and relay tests use.

package slackengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testChannel  = "C1"
	testOperator = "U1"
)

// fakeClock is a Clock whose time moves only when a test advances it or a wait is taken, and whose waits return at once.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
	// onWait runs after each wait is recorded, outside the clock's lock.
	onWait func(d time.Duration)
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// After records d, moves the clock forward by it and returns a channel that has already delivered.
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	now := c.now
	onWait := c.onWait
	c.mu.Unlock()
	if onWait != nil {
		onWait(d)
	}
	delivered := make(chan time.Time, 1)
	delivered <- now
	return delivered
}

// recordedWaits returns a copy of the waits taken so far.
func (c *fakeClock) recordedWaits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

// mail is one message a test posted to the fake mailbox.
type mail struct {
	Receiver string
	Body     string
}

// readStep scripts one answer of the fake mailbox's ReadAll or ReadWait.
type readStep struct {
	// messages is what the handler is handed; a step with none and no runHandler runs no handler.
	messages []creelengine.Message
	// runHandler runs the handler even with no messages.
	runHandler bool
	// err is returned without running the handler.
	err error
	// timeoutAfterHandler returns creelengine.ErrWaitTimeout once the handler returned nil, as a lost confirmation does.
	timeoutAfterHandler bool
	// ackErr is returned once the handler returned nil, as a refused acknowledgement is.
	ackErr error
}

// fakeMailbox is a Mailbox that records posts and acknowledgements and answers reads from a script.
type fakeMailbox struct {
	mu sync.Mutex
	// posts are the accepted posts, in order.
	posts []mail
	// postErr fails every post when set; postErrFor fails the posts to one receiver.
	postErr    error
	postErrFor map[string]error
	nextID     int64
	// readAllSteps and readWaitSteps are consumed one per call; an empty script reads as no mail.
	readAllSteps  []readStep
	readWaitSteps []readStep
	// blockReadWait, when set, makes ReadWait block until it is closed, as a long poll with no mail does.
	blockReadWait chan struct{}
	// acks holds the ids each acknowledged ReadWait covered, and readAllAcks the ids each acknowledged ReadAll covered.
	acks        [][]int64
	readAllAcks [][]int64
	// readAllCalls and readWaitCalls count the calls of each read.
	readAllCalls  int
	readWaitCalls int
	// releases counts handlers that returned an error and so released their read.
	releases int
}

func (m *fakeMailbox) Post(receiver, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.postErrFor[receiver]; err != nil {
		return 0, err
	}
	if m.postErr != nil {
		return 0, m.postErr
	}
	m.nextID++
	m.posts = append(m.posts, mail{Receiver: receiver, Body: body})
	return m.nextID, nil
}

func (m *fakeMailbox) ReadAll(handle func([]creelengine.Message) error) error {
	m.mu.Lock()
	m.readAllCalls++
	var step readStep
	if len(m.readAllSteps) > 0 {
		step, m.readAllSteps = m.readAllSteps[0], m.readAllSteps[1:]
	}
	m.mu.Unlock()
	if step.err != nil {
		return step.err
	}
	if len(step.messages) == 0 && !step.runHandler {
		return nil
	}
	if err := handle(step.messages); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readAllAcks = append(m.readAllAcks, messageIDs(step.messages))
	return nil
}

func (m *fakeMailbox) ReadWait(_ time.Duration, handle func([]creelengine.Message) error) error {
	m.mu.Lock()
	m.readWaitCalls++
	block := m.blockReadWait
	var step readStep
	if len(m.readWaitSteps) > 0 {
		step, m.readWaitSteps = m.readWaitSteps[0], m.readWaitSteps[1:]
	}
	m.mu.Unlock()
	if block != nil {
		<-block
		return creelengine.ErrWaitTimeout
	}
	if step.err != nil {
		return step.err
	}
	if len(step.messages) == 0 && !step.runHandler {
		return creelengine.ErrWaitTimeout
	}
	if err := handle(step.messages); err != nil {
		m.mu.Lock()
		m.releases++
		m.mu.Unlock()
		return err
	}
	if step.timeoutAfterHandler {
		return creelengine.ErrWaitTimeout
	}
	if step.ackErr != nil {
		return step.ackErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acks = append(m.acks, messageIDs(step.messages))
	return nil
}

func (m *fakeMailbox) setPostErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postErr = err
}

func (m *fakeMailbox) scriptReadWait(steps ...readStep) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readWaitSteps = append(m.readWaitSteps, steps...)
}

func (m *fakeMailbox) scriptReadAll(steps ...readStep) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readAllSteps = append(m.readAllSteps, steps...)
}

// accepted returns a copy of the accepted posts.
func (m *fakeMailbox) accepted() []mail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail(nil), m.posts...)
}

// acknowledged returns a copy of the ids each acknowledged ReadWait covered.
func (m *fakeMailbox) acknowledged() [][]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]int64(nil), m.acks...)
}

func messageIDs(messages []creelengine.Message) []int64 {
	ids := make([]int64, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

// postedText is one text a test posted to the fake Slack.
type postedText struct {
	Channel string
	Text    string
}

// fakeSlack is a SlackClient holding each channel's messages in memory.
type fakeSlack struct {
	mu       sync.Mutex
	channels map[string][]slackengine.Message
	// historyErr and latestErr fail the reads when set.
	historyErr error
	latestErr  error
	// postErrs is consumed one per PostMessage call, a nil entry succeeding; an empty script succeeds.
	postErrs []error
	posted   []postedText
	// onPost runs after each PostMessage call, outside the lock.
	onPost func()
	// calls counts every call of any method.
	calls int
	// onCall runs at the start of every call of any method, outside the lock.
	onCall func()
}

// called counts a call and runs the onCall hook.
func (s *fakeSlack) called() {
	s.mu.Lock()
	s.calls++
	onCall := s.onCall
	s.mu.Unlock()
	if onCall != nil {
		onCall()
	}
}

func newFakeSlack() *fakeSlack {
	return &fakeSlack{channels: map[string][]slackengine.Message{}}
}

// add appends message to channel, which a test does in timestamp order.
func (s *fakeSlack) add(channel string, message slackengine.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel] = append(s.channels[channel], message)
}

func (s *fakeSlack) History(_ context.Context, channel, oldest string) ([]slackengine.Message, error) {
	s.called()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyErr != nil {
		return nil, s.historyErr
	}
	var after []slackengine.Message
	for _, message := range s.channels[channel] {
		if tsValue(message.TS) > tsValue(oldest) {
			after = append(after, message)
		}
	}
	return after, nil
}

func (s *fakeSlack) Latest(_ context.Context, channel string) (string, bool, error) {
	s.called()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latestErr != nil {
		return "", false, s.latestErr
	}
	newest, found := "", false
	for _, message := range s.channels[channel] {
		if !found || tsValue(message.TS) > tsValue(newest) {
			newest, found = message.TS, true
		}
	}
	return newest, found, nil
}

func (s *fakeSlack) PostMessage(_ context.Context, channel, text string) error {
	s.called()
	s.mu.Lock()
	var err error
	if len(s.postErrs) > 0 {
		err, s.postErrs = s.postErrs[0], s.postErrs[1:]
	}
	if err == nil {
		s.posted = append(s.posted, postedText{Channel: channel, Text: text})
	}
	onPost := s.onPost
	s.mu.Unlock()
	if onPost != nil {
		onPost()
	}
	return err
}

func (s *fakeSlack) setHistoryErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyErr = err
}

func (s *fakeSlack) failPosts(errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postErrs = append(s.postErrs, errs...)
}

// texts returns the texts the fake accepted, oldest first.
func (s *fakeSlack) texts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var texts []string
	for _, post := range s.posted {
		texts = append(texts, post.Text)
	}
	return texts
}

func (s *fakeSlack) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// tsValue reads a Slack timestamp as a number, which orders it.
func tsValue(ts string) float64 {
	value, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0
	}
	return value
}

// fakeHub is the hub's beams and pauses, in memory.
type fakeHub struct {
	mu        sync.Mutex
	beams     []string
	beamsErr  error
	paused    map[string]bool
	pausedErr error
}

func (h *fakeHub) hub() slackengine.Hub {
	return slackengine.Hub{
		Beams: func() ([]string, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return append([]string(nil), h.beams...), h.beamsErr
		},
		Paused: func(receiver string) (bool, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.paused[receiver], h.pausedErr
		},
	}
}

func (h *fakeHub) set(update func(h *fakeHub)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	update(h)
}

// newGeometry returns the Geometry of a hub in a fresh temp directory, with no file written.
func newGeometry(t *testing.T) slackengine.Geometry {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "slack")
	return slackengine.Geometry{
		HubPath:        root,
		Shortname:      "ly",
		Dir:            dir,
		TokenPath:      filepath.Join(root, "credentials", slackengine.TokenFileName),
		SetupPath:      filepath.Join(dir, slackengine.SetupFileName),
		PresencePath:   filepath.Join(dir, slackengine.PresenceFileName),
		CursorPath:     filepath.Join(dir, slackengine.CursorFileName),
		OutboxPath:     filepath.Join(dir, slackengine.OutboxFileName),
		RelayStatePath: filepath.Join(dir, slackengine.RelayStateFileName),
		LockPath:       filepath.Join(dir, slackengine.RelayLockFileName),
		LogPath:        filepath.Join(dir, slackengine.RelayLogFileName),
		RunProcDir:     filepath.Join(root, "runproc"),
		LymExecutable:  filepath.Join(root, "lym"),
	}
}

// harness is an inbound poll over the in-memory seams of a configured hub.
type harness struct {
	t       *testing.T
	geom    slackengine.Geometry
	clock   *fakeClock
	mailbox *fakeMailbox
	slack   *fakeSlack
	hub     *fakeHub
	in      *slackengine.Inbound
}

// newHarness returns a harness whose hub has the token and the setup for testChannel and testOperator.
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, geom: newGeometry(t), clock: newFakeClock(), mailbox: &fakeMailbox{}, slack: newFakeSlack(), hub: &fakeHub{paused: map[string]bool{}}}
	if err := slackengine.WriteToken(h.geom.TokenPath, "xoxb-secret"); err != nil {
		t.Fatalf("WriteToken = %v", err)
	}
	h.setChannel(testChannel)
	h.in = slackengine.NewInbound(h.geom, h.mailbox, h.slack, h.hub.hub(), h.clock)
	return h
}

// setChannel writes the setup naming channel and testOperator.
func (h *harness) setChannel(channel string) {
	h.t.Helper()
	if err := slackengine.WriteSetup(h.geom.SetupPath, slackengine.Setup{ChannelID: channel, OperatorID: testOperator}); err != nil {
		h.t.Fatalf("WriteSetup = %v", err)
	}
}

// startAt writes the cursor for testChannel at ts.
func (h *harness) startAt(ts string) {
	h.t.Helper()
	if err := os.MkdirAll(h.geom.Dir, 0o700); err != nil {
		h.t.Fatalf("MkdirAll = %v", err)
	}
	content := fmt.Sprintf(`{"channel":%q,"ts":%q}`, testChannel, ts)
	if err := os.WriteFile(h.geom.CursorPath, []byte(content), 0o600); err != nil {
		h.t.Fatalf("write the cursor = %v", err)
	}
}

// cursor returns the channel and timestamp the cursor file holds, empty when none is written.
func (h *harness) cursor() (string, string) {
	h.t.Helper()
	data, err := os.ReadFile(h.geom.CursorPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", ""
	}
	if err != nil {
		h.t.Fatalf("read the cursor = %v", err)
	}
	var record struct{ Channel, TS string }
	if err := json.Unmarshal(data, &record); err != nil {
		h.t.Fatalf("decode the cursor = %v", err)
	}
	return record.Channel, record.TS
}

// say adds a top-level message of the operator to testChannel.
func (h *harness) say(ts, text string) {
	h.slack.add(testChannel, slackengine.Message{TS: ts, User: testOperator, Text: text})
}

func (h *harness) poll() (bool, error) {
	h.t.Helper()
	return h.in.Poll(context.Background())
}

// mustPoll polls and fails the test on an error.
func (h *harness) mustPoll() bool {
	h.t.Helper()
	activity, err := h.poll()
	if err != nil {
		h.t.Fatalf("Poll = %v", err)
	}
	return activity
}

func (h *harness) wantCursorTS(want string) {
	h.t.Helper()
	if _, ts := h.cursor(); ts != want {
		h.t.Errorf("cursor ts = %q; want %q", ts, want)
	}
}

func (h *harness) wantPosts(want ...mail) {
	h.t.Helper()
	if got := h.mailbox.accepted(); !cmp.Equal(got, append([]mail(nil), want...)) && !(len(got) == 0 && len(want) == 0) {
		h.t.Errorf("mailbox posts mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

func (h *harness) wantAnswers(want ...string) {
	h.t.Helper()
	if got := h.slack.texts(); !cmp.Equal(got, append([]string(nil), want...)) && !(len(got) == 0 && len(want) == 0) {
		h.t.Errorf("Slack answers mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

func TestInboundPoll(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("OperatorMessageReachesThePrimeOrch", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "deploy now")
		if !h.mustPoll() {
			t.Errorf("Poll reported no activity for a forwarded message")
		}
		h.wantPosts(mail{"ly:orch", "[slack] deploy now"})
		h.wantCursorTS("101.000001")
		if h.mustPoll() {
			t.Errorf("a second Poll reported activity")
		}
		h.wantPosts(mail{"ly:orch", "[slack] deploy now"})
	})

	t.Run("SlackMarkupReachesTheOrchAsPlainText", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "see <https://x.io/a|label> <https://x.io/a> <#C1|general> <!here> <@U1> &amp; &lt;c&gt;")
		h.mustPoll()
		h.wantPosts(mail{"ly:orch", "[slack] see https://x.io/a https://x.io/a <#C1|general> <!here> <@U1> & <c>"})
	})

	t.Run("WhatIsNotTheOperatorsTopLevelTextMovesTheCursorOnly", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.slack.add(testChannel, slackengine.Message{TS: "101.000001", User: "U2", Text: "someone else"})
		h.slack.add(testChannel, slackengine.Message{TS: "102.000001", User: testOperator, BotID: "B1", Text: "a bot"})
		h.slack.add(testChannel, slackengine.Message{TS: "103.000001", ThreadTS: "99.000001", User: testOperator, Text: "a thread reply"})
		h.slack.add(testChannel, slackengine.Message{TS: "104.000001", User: testOperator, Subtype: "channel_join", Text: "joined"})
		if h.mustPoll() {
			t.Errorf("Poll reported activity for messages it skipped")
		}
		h.wantPosts()
		h.wantAnswers()
		h.wantCursorTS("104.000001")
	})

	t.Run("AtSlugReachesThatBeamsOrch", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beams = []string{"payments"} })
		h.startAt("100.000000")
		h.say("101.000001", "@payments do x")
		h.mustPoll()
		h.wantPosts(mail{"ly:payments:orch", "[slack] do x"})
	})

	t.Run("UnknownBeamIsAnsweredWithTheBeamsAndPostsNothing", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beams = []string{"payments", "orders"} })
		h.startAt("100.000000")
		h.say("101.000001", "@nope x")
		h.mustPoll()
		h.wantPosts()
		h.wantAnswers(`no beam "nope"; the hub's beams are: payments, orders`)
		h.wantCursorTS("101.000001")
	})

	t.Run("BeamsErrorRetriesAndForwardsOnce", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beams, h.beamsErr = []string{"payments"}, errors.New("board unreadable") })
		h.startAt("100.000000")
		h.say("101.000001", "@payments x")
		if _, err := h.poll(); err == nil {
			t.Fatalf("Poll = nil; want the Beams error")
		}
		h.wantPosts()
		h.wantAnswers()
		h.wantCursorTS("100.000000")
		h.hub.set(func(h *fakeHub) { h.beamsErr = nil })
		h.mustPoll()
		h.mustPoll()
		h.wantPosts(mail{"ly:payments:orch", "[slack] x"})
	})

	t.Run("FirstPollWithNoCursorForwardsNoHistory", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.say("90.000001", "old one")
		h.say("91.000001", "old two")
		if h.mustPoll() {
			t.Errorf("the first Poll reported activity")
		}
		h.wantPosts()
		h.wantCursorTS("91.000001")
		h.say("92.000001", "new")
		h.mustPoll()
		h.mustPoll()
		h.wantPosts(mail{"ly:orch", "[slack] new"})
	})

	t.Run("EmptyChannelFirstPollStartsAtTheClock", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.mustPoll()
		if channel, ts := h.cursor(); channel != testChannel || ts != fmt.Sprintf("%d.000000", h.clock.Now().Unix()) {
			t.Errorf("cursor = %q, %q; want the channel and the clock's now", channel, ts)
		}
	})

	t.Run("ChangedChannelForwardsNoHistoryOfThatChannel", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "first")
		h.mustPoll()
		h.slack.add("C2", slackengine.Message{TS: "50.000001", User: testOperator, Text: "in the other channel"})
		h.setChannel("C2")
		h.mustPoll()
		if channel, ts := h.cursor(); channel != "C2" || ts != "50.000001" {
			t.Errorf("cursor after the change = %q, %q; want C2 at its newest message", channel, ts)
		}
		h.say("102.000001", "written while away from C1")
		h.setChannel(testChannel)
		h.mustPoll()
		if channel, ts := h.cursor(); channel != testChannel || ts != "102.000001" {
			t.Errorf("cursor after the change back = %q, %q; want C1 at its newest message", channel, ts)
		}
		h.wantPosts(mail{"ly:orch", "[slack] first"})
	})

	t.Run("RefusedPostHoldsLaterMessagesUntilAcceptedOrFiveMinutes", func(t *testing.T) {
		t.Parallel()

		t.Run("AcceptedWithinFiveMinutes", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.startAt("100.000000")
			h.say("101.000001", "m1")
			h.say("102.000001", "away")
			h.mailbox.setPostErr(errors.New("creel down"))
			if _, err := h.poll(); err == nil {
				t.Fatalf("Poll = nil; want the post error")
			}
			h.wantCursorTS("100.000000")
			h.wantAnswers()
			h.mailbox.setPostErr(nil)
			h.clock.Advance(time.Minute)
			h.mustPoll()
			h.wantCursorTS("102.000001")
			h.wantPosts(mail{"ly:orch", "[slack] m1"}, mail{"ly:orch", "[slack presence] presence is now slack"})
		})

		t.Run("PastFiveMinutesIsAnsweredOnceAndPassed", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.startAt("100.000000")
			h.say("101.000001", "m1")
			h.mailbox.setPostErr(errors.New("creel down"))
			if _, err := h.poll(); err == nil {
				t.Fatalf("Poll = nil; want the post error")
			}
			h.clock.Advance(5 * time.Minute)
			h.mustPoll()
			h.mustPoll()
			h.wantAnswers("your message was not delivered; send it again: creel down")
			h.wantCursorTS("101.000001")
			h.wantPosts()
		})
	})

	t.Run("BodyOverCreelsCapIsAnsweredNotPosted", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			textBytes  int
			wantPosted bool
		}{
			{"AtTheCap", 4096 - len("[slack] "), true},
			{"OverTheCap", 4097 - len("[slack] "), false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.startAt("100.000000")
				h.say("101.000001", strings.Repeat("x", tt.textBytes))
				h.mustPoll()
				if posted := len(h.mailbox.accepted()) == 1; posted != tt.wantPosted {
					t.Errorf("posted = %v; want %v", posted, tt.wantPosted)
				}
				if answers := h.slack.texts(); (len(answers) == 1) == tt.wantPosted {
					t.Errorf("answers = %d; want one exactly when the body is refused", len(answers))
				}
				h.wantCursorTS("101.000001")
			})
		}
	})

	t.Run("PausedReceiverGetsThePostAndOneAnswer", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.paused["ly:orch"] = true })
		h.startAt("100.000000")
		h.say("101.000001", "hello")
		h.mustPoll()
		h.wantPosts(mail{"ly:orch", "[slack] hello"})
		h.wantAnswers("the hub is paused; your message waits for the resume")
	})

	t.Run("PausedErrorAfterAnAcceptedPostStillMovesTheCursor", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.pausedErr = errors.New("pause record unreadable") })
		h.startAt("100.000000")
		h.say("101.000001", "hello")
		h.mustPoll()
		h.mustPoll()
		h.wantPosts(mail{"ly:orch", "[slack] hello"})
		h.wantAnswers()
		h.wantCursorTS("101.000001")
	})

	t.Run("PresenceCommandsSetAndReadThePresenceAndAreAnswered", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beams = []string{"payments"} })
		h.startAt("100.000000")
		h.say("101.000001", "away")
		h.say("102.000001", " Back ")
		h.say("103.000001", "status")
		h.mustPoll()
		h.wantAnswers("presence is now slack", "presence is now terminal", "presence is terminal")
		h.wantPosts(
			mail{"ly:orch", "[slack presence] presence is now slack"},
			mail{"ly:payments:orch", "[slack presence] presence is now slack"},
			mail{"ly:orch", "[slack presence] presence is now terminal"},
			mail{"ly:payments:orch", "[slack presence] presence is now terminal"},
		)
	})

	t.Run("AwayWhoseBeamsReadFailsAfterTheWriteIsAnsweredAndPassed", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beamsErr = errors.New("board unreadable") })
		h.startAt("100.000000")
		h.say("101.000001", "away")
		h.mustPoll()
		h.mustPoll()
		if presence, err := slackengine.ReadPresence(h.geom.PresencePath); err != nil || presence != slackengine.PresenceSlack {
			t.Errorf("ReadPresence = %q, %v; want slack", presence, err)
		}
		h.wantAnswers("presence is now slack")
		h.wantCursorTS("101.000001")
	})

	t.Run("AwayWhoseWriteFailsIsRetriedUntilTheWriteWorks", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "away")
		if err := os.MkdirAll(h.geom.PresencePath, 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if _, err := h.poll(); err == nil {
			t.Fatalf("Poll = nil; want the presence write error")
		}
		h.wantAnswers()
		h.wantCursorTS("100.000000")
		if err := os.Remove(h.geom.PresencePath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		h.mustPoll()
		h.mustPoll()
		h.wantAnswers("presence is now slack")
		h.wantCursorTS("101.000001")
	})

	t.Run("StatusOverAnUnknownPresenceIsRetriedThenAnsweredWithTheError", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		if err := os.WriteFile(h.geom.PresencePath, []byte(`{"presence":"away"}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		h.say("101.000001", "status")
		if _, err := h.poll(); err == nil {
			t.Fatalf("Poll = nil; want the presence read error")
		}
		h.wantAnswers()
		h.wantCursorTS("100.000000")
		h.clock.Advance(5 * time.Minute)
		h.mustPoll()
		h.mustPoll()
		answers := h.slack.texts()
		if len(answers) != 1 || !strings.Contains(answers[0], h.geom.PresencePath) {
			t.Errorf("answers = %q; want one quoting the error that names %s", answers, h.geom.PresencePath)
		}
		h.wantCursorTS("101.000001")
	})

	t.Run("AnswerIsEscapedForSlack", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "hello")
		h.mailbox.setPostErr(errors.New("a & <b>"))
		if _, err := h.poll(); err == nil {
			t.Fatalf("Poll = nil; want the post error")
		}
		h.clock.Advance(5 * time.Minute)
		h.mustPoll()
		h.wantAnswers("your message was not delivered; send it again: a &amp; &lt;b&gt;")
	})

	t.Run("AnswerSlackRefusesStillMovesTheCursor", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "status")
		h.slack.failPosts(errors.New("slack down"))
		h.mustPoll()
		h.wantCursorTS("101.000001")
		h.wantAnswers()
	})

	t.Run("PresenceChangedByAnotherWriterIsActivity", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		if h.mustPoll() {
			t.Errorf("the first Poll reported activity")
		}
		if _, err := slackengine.WritePresence(h.geom.PresencePath, slackengine.PresenceSlack, h.clock.Now()); err != nil {
			t.Fatalf("WritePresence = %v", err)
		}
		if !h.mustPoll() {
			t.Errorf("Poll after another writer changed the presence reported no activity")
		}
		if h.mustPoll() {
			t.Errorf("Poll over an unchanged presence reported activity")
		}
	})

	t.Run("SlackRefusalMailsOneNoticePerEpisode", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		refusal := &slackengine.APIError{Method: "conversations.history", Code: "invalid_auth"}
		h.slack.setHistoryErr(refusal)
		for range 2 {
			if _, err := h.poll(); !errors.Is(err, refusal) {
				t.Fatalf("Poll = %v; want the refusal", err)
			}
		}
		posts := h.mailbox.accepted()
		if len(posts) != 1 || posts[0].Receiver != "ly:orch" || !strings.HasPrefix(posts[0].Body, "[slack relay]") ||
			!strings.Contains(posts[0].Body, "invalid_auth") || !strings.Contains(posts[0].Body, "lym slack login") {
			t.Fatalf("notices = %+v; want one [slack relay] notice to ly:orch naming invalid_auth and lym slack login", posts)
		}
		h.slack.setHistoryErr(nil)
		h.mustPoll()
		h.slack.setHistoryErr(refusal)
		if _, err := h.poll(); !errors.Is(err, refusal) {
			t.Fatalf("Poll = %v; want the refusal", err)
		}
		if got := len(h.mailbox.accepted()); got != 2 {
			t.Errorf("notices after a recovered episode = %d; want 2", got)
		}
	})
}

func TestSetPresence(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const notice = "[slack presence] presence is now slack"

	beams := func(slugs ...string) func() ([]string, error) {
		return func() ([]string, error) { return slugs, nil }
	}

	t.Run("ChangeMailsTheOrchAndEachBeam", func(t *testing.T) {
		t.Parallel()
		geom, mailbox := newGeometry(t), &fakeMailbox{}
		changed, err := slackengine.SetPresence(geom, mailbox, beams("payments", "orders"), slackengine.PresenceSlack, time.Now())
		if err != nil || !changed {
			t.Fatalf("SetPresence = %v, %v; want a change", changed, err)
		}
		want := []mail{{"ly:orch", notice}, {"ly:payments:orch", notice}, {"ly:orders:orch", notice}}
		if got := mailbox.accepted(); !cmp.Equal(got, want) {
			t.Errorf("notices mismatch (-want +got):\n%s", cmp.Diff(want, got))
		}
	})

	t.Run("SameStateMailsNothing", func(t *testing.T) {
		t.Parallel()
		geom, mailbox := newGeometry(t), &fakeMailbox{}
		if _, err := slackengine.SetPresence(geom, mailbox, beams(), slackengine.PresenceSlack, time.Now()); err != nil {
			t.Fatalf("SetPresence = %v", err)
		}
		changed, err := slackengine.SetPresence(geom, mailbox, beams(), slackengine.PresenceSlack, time.Now())
		if err != nil || changed {
			t.Errorf("SetPresence(same state) = %v, %v; want unchanged", changed, err)
		}
		if got := len(mailbox.accepted()); got != 1 {
			t.Errorf("notices = %d; want the one of the first change", got)
		}
	})

	t.Run("FailedNoticeReturnsAnErrorWithThePresenceWritten", func(t *testing.T) {
		t.Parallel()
		geom := newGeometry(t)
		mailbox := &fakeMailbox{postErrFor: map[string]error{"ly:payments:orch": errors.New("creel refused")}}
		changed, err := slackengine.SetPresence(geom, mailbox, beams("payments"), slackengine.PresenceSlack, time.Now())
		if !changed || err == nil || !strings.Contains(err.Error(), "ly:payments:orch") {
			t.Fatalf("SetPresence = %v, %v; want changed with an error naming the failed receiver", changed, err)
		}
		if presence, readErr := slackengine.ReadPresence(geom.PresencePath); readErr != nil || presence != slackengine.PresenceSlack {
			t.Errorf("ReadPresence = %q, %v; want slack", presence, readErr)
		}
		if got := mailbox.accepted(); len(got) != 1 || got[0].Receiver != "ly:orch" {
			t.Errorf("notices = %+v; want the one to ly:orch", got)
		}
	})

	t.Run("BeamsReadErrorAfterTheWriteReturnsChangedWithTheError", func(t *testing.T) {
		t.Parallel()
		geom, mailbox := newGeometry(t), &fakeMailbox{}
		failing := func() ([]string, error) { return nil, errors.New("board unreadable") }
		changed, err := slackengine.SetPresence(geom, mailbox, failing, slackengine.PresenceSlack, time.Now())
		if !changed || err == nil {
			t.Fatalf("SetPresence = %v, %v; want changed with an error", changed, err)
		}
		if presence, readErr := slackengine.ReadPresence(geom.PresencePath); readErr != nil || presence != slackengine.PresenceSlack {
			t.Errorf("ReadPresence = %q, %v; want slack", presence, readErr)
		}
	})
}
