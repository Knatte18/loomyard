// inbound_test.go covers the inbound read of the hub's channels and the presence notices.
// It also declares the in-memory Slack, mailbox, hub and clock that the outbound and relay tests use.

package slackengine_test

import (
	"context"
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
	// paymentsChannel and paymentsOrch are the second channel of the harness's map and the orch it belongs to.
	paymentsChannel = "C2"
	paymentsOrch    = "ly:payments:orch"
	// primeOrch is the orch of testChannel.
	primeOrch = "ly:orch"
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
	// readWaitEntered, when set, is closed by the first ReadWait call before it blocks.
	readWaitEntered chan struct{}
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
	if m.readWaitCalls == 1 && m.readWaitEntered != nil {
		close(m.readWaitEntered)
	}
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
	Sender  slackengine.Sender
}

// slackCall is one call the fake Slack received.
// Arg is the call's argument besides the channel and the sender.
// It is the oldest timestamp of a read, the thread of Replies, the text of PostAs, or the user of Invite.
// For CreateChannel, FindChannel and Rename it is the name.
type slackCall struct {
	Method  string
	Channel string
	Arg     string
	Sender  slackengine.Sender
}

// fakeSlack is a SlackClient holding each channel's messages in memory, with a directory of the channels that exist.
type fakeSlack struct {
	mu       sync.Mutex
	channels map[string][]slackengine.Message
	// directory holds the channels that exist, created or seeded, in the order they came.
	directory []slackengine.ChannelInfo
	// invited maps a channel to the users Invite added to it.
	invited map[string][]string
	// historyErr and latestErr fail the reads when set.
	historyErr error
	latestErr  error
	// channelErr fails the reads of one channel, History and Replies, when set for it.
	channelErr map[string]error
	// failures holds, per method, the errors consumed one per call of that method, a nil entry succeeding.
	failures map[string][]error
	posted   []postedText
	// log holds every call received, in order.
	log []slackCall
	// onPost runs after each PostAs call, outside the lock.
	onPost func()
	// calls counts every call of any method.
	calls int
	// onCall runs at the start of every call of any method, outside the lock.
	onCall func()
	// onMethod runs at the start of every call with its method name, outside the lock.
	onMethod func(method string)
}

// begin counts and logs a call, runs the hooks and returns the next error queued for the call's method.
func (s *fakeSlack) begin(call slackCall) error {
	s.mu.Lock()
	s.calls++
	s.log = append(s.log, call)
	onCall, onMethod := s.onCall, s.onMethod
	s.mu.Unlock()
	if onCall != nil {
		onCall()
	}
	if onMethod != nil {
		onMethod(call.Method)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if queue := s.failures[call.Method]; len(queue) > 0 {
		err, s.failures[call.Method] = queue[0], queue[1:]
	}
	return err
}

func newFakeSlack() *fakeSlack {
	return &fakeSlack{
		channels: map[string][]slackengine.Message{}, invited: map[string][]string{},
		failures: map[string][]error{}, channelErr: map[string]error{},
	}
}

// add appends message to channel, which a test does in timestamp order.
func (s *fakeSlack) add(channel string, message slackengine.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel] = append(s.channels[channel], message)
}

// seedChannel adds a channel that exists in Slack, as another user, another bot or an archive left it.
func (s *fakeSlack) seedChannel(info slackengine.ChannelInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directory = append(s.directory, info)
}

// isTopLevel reports whether History returns the message: it starts no thread, starts one, or is broadcast from one.
func isTopLevel(message slackengine.Message) bool {
	return message.ThreadTS == "" || message.ThreadTS == message.TS || message.Subtype == "thread_broadcast"
}

func (s *fakeSlack) History(_ context.Context, channel, oldest string) ([]slackengine.Message, error) {
	if err := s.begin(slackCall{Method: "History", Channel: channel, Arg: oldest}); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyErr != nil {
		return nil, s.historyErr
	}
	if err := s.channelErr[channel]; err != nil {
		return nil, err
	}
	var after []slackengine.Message
	for _, message := range s.channels[channel] {
		if isTopLevel(message) && tsValue(message.TS) > tsValue(oldest) {
			after = append(after, message)
		}
	}
	return after, nil
}

// Replies returns the messages of one thread after oldest, the parent included, as Slack does.
func (s *fakeSlack) Replies(_ context.Context, channel, thread, oldest string) ([]slackengine.Message, error) {
	if err := s.begin(slackCall{Method: "Replies", Channel: channel, Arg: thread}); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.channelErr[channel]; err != nil {
		return nil, err
	}
	var replies []slackengine.Message
	for _, message := range s.channels[channel] {
		isParent := message.TS == thread
		if isParent || (message.ThreadTS == thread && tsValue(message.TS) > tsValue(oldest)) {
			replies = append(replies, message)
		}
	}
	return replies, nil
}

func (s *fakeSlack) Latest(_ context.Context, channel string) (string, bool, error) {
	if err := s.begin(slackCall{Method: "Latest", Channel: channel}); err != nil {
		return "", false, err
	}
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

func (s *fakeSlack) PostAs(_ context.Context, channel, text string, sender slackengine.Sender) error {
	err := s.begin(slackCall{Method: "PostAs", Channel: channel, Arg: text, Sender: sender})
	s.mu.Lock()
	if err == nil {
		s.posted = append(s.posted, postedText{Channel: channel, Text: text, Sender: sender})
	}
	onPost := s.onPost
	s.mu.Unlock()
	if onPost != nil {
		onPost()
	}
	return err
}

// CreateChannel adds a channel with a fresh id under name, created by UBOT with the bot a member.
// It answers name_taken for a name any existing channel holds, an archived one included.
func (s *fakeSlack) CreateChannel(_ context.Context, name string) (slackengine.ChannelInfo, error) {
	if err := s.begin(slackCall{Method: "CreateChannel", Arg: name}); err != nil {
		return slackengine.ChannelInfo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.indexByName(name) >= 0 {
		return slackengine.ChannelInfo{}, &slackengine.APIError{Method: "conversations.create", Code: "name_taken"}
	}
	info := slackengine.ChannelInfo{ID: s.freshID(), Name: name, Creator: "UBOT", IsMember: true}
	s.directory = append(s.directory, info)
	return info, nil
}

// FindChannel finds a created or seeded channel by name, archived ones included.
func (s *fakeSlack) FindChannel(_ context.Context, name string) (slackengine.ChannelInfo, bool, error) {
	if err := s.begin(slackCall{Method: "FindChannel", Arg: name}); err != nil {
		return slackengine.ChannelInfo{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexByName(name); i >= 0 {
		return s.directory[i], true, nil
	}
	return slackengine.ChannelInfo{}, false, nil
}

// Join marks the bot a member of the channel.
func (s *fakeSlack) Join(_ context.Context, channel string) error {
	if err := s.begin(slackCall{Method: "Join", Channel: channel}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexByID(channel); i >= 0 {
		s.directory[i].IsMember = true
	}
	return nil
}

// Invite records the user as invited to the channel.
func (s *fakeSlack) Invite(_ context.Context, channel, user string) error {
	if err := s.begin(slackCall{Method: "Invite", Channel: channel, Arg: user}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invited[channel] = append(s.invited[channel], user)
	return nil
}

// Rename changes the channel's name, and answers name_taken for a name another channel holds.
func (s *fakeSlack) Rename(_ context.Context, channel, name string) error {
	if err := s.begin(slackCall{Method: "Rename", Channel: channel, Arg: name}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexByID(channel)
	if i < 0 {
		return &slackengine.APIError{Method: "conversations.rename", Code: "channel_not_found"}
	}
	if taken := s.indexByName(name); taken >= 0 && taken != i {
		return &slackengine.APIError{Method: "conversations.rename", Code: "name_taken"}
	}
	s.directory[i].Name = name
	return nil
}

// Archive marks the channel archived.
func (s *fakeSlack) Archive(_ context.Context, channel string) error {
	if err := s.begin(slackCall{Method: "Archive", Channel: channel}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexByID(channel)
	if i < 0 {
		return &slackengine.APIError{Method: "conversations.archive", Code: "channel_not_found"}
	}
	s.directory[i].IsArchived = true
	return nil
}

// AuthTest answers the bot UBOT of the bot B1.
func (s *fakeSlack) AuthTest(context.Context) (slackengine.Identity, error) {
	if err := s.begin(slackCall{Method: "AuthTest"}); err != nil {
		return slackengine.Identity{}, err
	}
	return slackengine.Identity{UserID: "UBOT", BotID: "B1"}, nil
}

// indexByName returns the directory index of the channel called name, or -1; the caller holds the lock.
func (s *fakeSlack) indexByName(name string) int {
	for i, info := range s.directory {
		if info.Name == name {
			return i
		}
	}
	return -1
}

// indexByID returns the directory index of the channel with id, or -1; the caller holds the lock.
func (s *fakeSlack) indexByID(id string) int {
	for i, info := range s.directory {
		if info.ID == id {
			return i
		}
	}
	return -1
}

// freshID returns the first id of the form C<n> that no channel of the directory or of the messages holds.
// The caller holds the lock.
func (s *fakeSlack) freshID() string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("C%d", n)
		if _, held := s.channels[id]; !held && s.indexByID(id) < 0 {
			return id
		}
	}
}

func (s *fakeSlack) setHistoryErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historyErr = err
}

// failChannel makes the reads of channel fail with err; a nil err ends the failure.
func (s *fakeSlack) failChannel(channel string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelErr[channel] = err
}

// failNext queues errs for the next calls of method, one per call, a nil entry succeeding.
func (s *fakeSlack) failNext(method string, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[method] = append(s.failures[method], errs...)
}

// failPosts queues errs for the next PostAs calls.
func (s *fakeSlack) failPosts(errs ...error) {
	s.failNext("PostAs", errs...)
}

// logged returns the calls received of method, in order.
func (s *fakeSlack) logged(method string) []slackCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var calls []slackCall
	for _, call := range s.log {
		if call.Method == method {
			calls = append(calls, call)
		}
	}
	return calls
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
		Machine:        "neve",
		Dir:            dir,
		TokenPath:      filepath.Join(root, "credentials", slackengine.TokenFileName),
		ChannelMapPath: filepath.Join(dir, slackengine.ChannelMapFileName),
		InboxDir:       filepath.Join(dir, slackengine.InboxDirName),
		SocketLockPath: filepath.Join(root, "credentials", slackengine.SocketLockFileName),
		SetupPath:      filepath.Join(dir, slackengine.SetupFileName),
		PresencePath:   filepath.Join(dir, slackengine.PresenceFileName),
		OutboxPath:     filepath.Join(dir, slackengine.OutboxFileName),
		RelayStatePath: filepath.Join(dir, slackengine.RelayStateFileName),
		LockPath:       filepath.Join(dir, slackengine.RelayLockFileName),
		LogPath:        filepath.Join(dir, slackengine.RelayLogFileName),
		RunProcDir:     filepath.Join(root, "runproc"),
		LymExecutable:  filepath.Join(root, "lym"),
	}
}

// harness is an inbound read over the in-memory seams of a configured hub.
type harness struct {
	t       *testing.T
	geom    slackengine.Geometry
	clock   *fakeClock
	mailbox *fakeMailbox
	slack   *fakeSlack
	hub     *fakeHub
	keeper  *slackengine.ChannelKeeper
	in      *slackengine.Inbound
}

// newBareHarness returns a harness whose hub has the tokens and the setup for testOperator and an empty channel map.
func newBareHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, geom: newGeometry(t), clock: newFakeClock(), mailbox: &fakeMailbox{}, slack: newFakeSlack(), hub: &fakeHub{paused: map[string]bool{}}}
	if err := slackengine.WriteCredentials(h.geom.TokenPath, slackengine.Credentials{BotToken: "xoxb-secret", AppToken: "xapp-secret", AppID: "A1"}); err != nil {
		t.Fatalf("WriteCredentials = %v", err)
	}
	if err := slackengine.WriteSetup(h.geom.SetupPath, slackengine.Setup{OperatorID: testOperator}); err != nil {
		t.Fatalf("WriteSetup = %v", err)
	}
	liveHubs := func() ([]slackengine.Geometry, error) { return []slackengine.Geometry{h.geom}, nil }
	h.keeper = slackengine.NewChannelKeeper(h.geom, h.slack, liveHubs, h.mailbox, h.clock)
	h.in = slackengine.NewInbound(h.geom, h.mailbox, h.slack, h.hub.hub(), h.clock, h.keeper)
	return h
}

// newHarness returns a bare harness whose channel map holds the prime orch on testChannel.
// The map also holds the payments orch on paymentsChannel.
// Each entry records the operator, so no read or post invites anyone.
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := newBareHarness(t)
	err := slackengine.UpdateChannelMap(h.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		channelMap.Channels[primeOrch] = slackengine.ChannelEntry{ID: testChannel, Name: "ly-neve-prime", Operator: testOperator}
		channelMap.Channels[paymentsOrch] = slackengine.ChannelEntry{ID: paymentsChannel, Name: "ly-neve-payments", Operator: testOperator}
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateChannelMap = %v", err)
	}
	return h
}

// updateEntry applies change to the orch's entry in the harness's channel map.
func (h *harness) updateEntry(orch string, change func(*slackengine.ChannelEntry)) {
	h.t.Helper()
	err := slackengine.UpdateChannelMap(h.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		entry := channelMap.Channels[orch]
		change(&entry)
		channelMap.Channels[orch] = entry
		return nil
	})
	if err != nil {
		h.t.Fatalf("UpdateChannelMap = %v", err)
	}
}

// startAt writes the prime entry's cursor at ts.
func (h *harness) startAt(ts string) {
	h.t.Helper()
	h.updateEntry(primeOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = ts })
}

// entryOf returns the orch's entry in the harness's channel map.
func (h *harness) entryOf(orch string) slackengine.ChannelEntry {
	h.t.Helper()
	channelMap, err := slackengine.ReadChannelMap(h.geom.ChannelMapPath)
	if err != nil {
		h.t.Fatalf("ReadChannelMap = %v", err)
	}
	return channelMap.Channels[orch]
}

// ago returns the Slack timestamp d before the harness's clock.
func (h *harness) ago(d time.Duration) string {
	return fmt.Sprintf("%d.000001", h.clock.Now().Add(-d).Unix())
}

// say adds a top-level message of the operator to testChannel.
func (h *harness) say(ts, text string) {
	h.sayIn(testChannel, ts, text)
}

// sayIn adds a top-level message of the operator to channel.
func (h *harness) sayIn(channel, ts, text string) {
	h.slack.add(channel, slackengine.Message{TS: ts, User: testOperator, Text: text})
}

// hint writes an empty hint called name into the hub's inbox.
func (h *harness) hint(name string) {
	h.t.Helper()
	if err := slackengine.WriteHint(h.geom, name); err != nil {
		h.t.Fatalf("WriteHint(%s) = %v", name, err)
	}
}

func (h *harness) drain() (bool, error) {
	h.t.Helper()
	return h.in.Drain(context.Background())
}

// mustDrain drains and fails the test on an error.
func (h *harness) mustDrain() bool {
	h.t.Helper()
	activity, err := h.drain()
	if err != nil {
		h.t.Fatalf("Drain = %v", err)
	}
	return activity
}

// readC1 writes the hint of testChannel and drains.
func (h *harness) readC1() (bool, error) {
	h.t.Helper()
	h.hint(testChannel)
	return h.drain()
}

// mustReadC1 writes the hint of testChannel, drains and fails the test on an error.
func (h *harness) mustReadC1() bool {
	h.t.Helper()
	h.hint(testChannel)
	return h.mustDrain()
}

// retry makes the owed reads that are due.
func (h *harness) retry() bool {
	h.t.Helper()
	return h.in.RetryOwed(context.Background())
}

func (h *harness) wantCursorTS(want string) {
	h.t.Helper()
	if got := h.entryOf(primeOrch).Cursor; got != want {
		h.t.Errorf("cursor = %q; want %q", got, want)
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

// relayNotices returns the bodies of the `[slack relay]` notices mailed to the hub's orch.
func (h *harness) relayNotices() []string {
	var bodies []string
	for _, posted := range h.mailbox.accepted() {
		if posted.Receiver == primeOrch && strings.HasPrefix(posted.Body, "[slack relay]") {
			bodies = append(bodies, posted.Body)
		}
	}
	return bodies
}

// threadScenario prepares the prime channel for a read with a thread scan.
// The cursor stands 30 minutes back, a thread started two hours back has a reply an hour back past its cursor,
// and a top-level message "one" came 10 minutes back.
func (h *harness) threadScenario() {
	h.t.Helper()
	parent, reply := h.ago(2*time.Hour), h.ago(time.Hour)
	h.updateEntry(primeOrch, func(entry *slackengine.ChannelEntry) {
		entry.Cursor = h.ago(30 * time.Minute)
		entry.Threads = map[string]string{parent: h.ago(90 * time.Minute)}
	})
	h.slack.add(testChannel, slackengine.Message{TS: parent, User: testOperator, Text: "parent", LatestReply: reply})
	h.slack.add(testChannel, slackengine.Message{TS: reply, ThreadTS: parent, User: testOperator, Text: "reply"})
	h.slack.add(testChannel, slackengine.Message{TS: h.ago(10 * time.Minute), User: testOperator, Text: "one"})
}

func TestInboundRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// fires are the ways a read of every open channel with its thread scan is asked for.
	fires := []struct {
		name string
		fire func(h *harness)
	}{
		{"ResyncHint", func(h *harness) {
			h.hint(slackengine.ResyncHint)
			_, _ = h.drain()
		}},
		{"Start", func(h *harness) {
			h.in.Start()
			h.retry()
		}},
	}
	hintC1 := func(h *harness) {
		h.hint(testChannel)
		_, _ = h.drain()
	}
	notInChannel := &slackengine.APIError{Method: "conversations.history", Code: "not_in_channel"}

	t.Run("MessageReachesTheOrchOfTheChannelItWasWrittenIn", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		h.sayIn(paymentsChannel, "101.000001", "deploy prod")
		h.say("102.000001", "@payments x")
		h.hint(paymentsChannel)
		if !h.mustDrain() {
			t.Errorf("Drain reported no activity for a forwarded message")
		}
		h.wantPosts(mail{paymentsOrch, "deploy prod"})
		if got := h.entryOf(paymentsOrch).Cursor; got != "101.000001" {
			t.Errorf("payments cursor = %q; want 101.000001", got)
		}
		h.mustReadC1()
		h.wantPosts(mail{paymentsOrch, "deploy prod"}, mail{primeOrch, "@payments x"})
		h.wantCursorTS("102.000001")
		h.hint(paymentsChannel)
		if h.mustDrain() {
			t.Errorf("a second Drain over read messages reported activity")
		}
		h.wantPosts(mail{paymentsOrch, "deploy prod"}, mail{primeOrch, "@payments x"})
	})

	t.Run("HintFileContentIsNeverForwarded", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		if err := os.MkdirAll(h.geom.InboxDir, 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(filepath.Join(h.geom.InboxDir, paymentsChannel), []byte("deploy prod"), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		if h.mustDrain() {
			t.Errorf("Drain over a hint holding text and a channel with no new message reported activity")
		}
		h.wantPosts()
		h.sayIn(paymentsChannel, "101.000001", "what Slack returns")
		h.hint(paymentsChannel)
		h.mustDrain()
		h.wantPosts(mail{paymentsOrch, "what Slack returns"})
	})

	t.Run("ResyncAndStartReadEveryChannelAfterItsCursorAndEveryThreadPastItsCursor", func(t *testing.T) {
		t.Parallel()
		for _, tt := range fires {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.threadScenario()
				h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
				h.sayIn(paymentsChannel, "101.000001", "two")
				tt.fire(h)
				h.wantPosts(mail{primeOrch, "one"}, mail{primeOrch, "reply"}, mail{paymentsOrch, "two"})
				entry := h.entryOf(primeOrch)
				if got := entry.Threads[h.ago(2*time.Hour)]; got != h.ago(time.Hour) {
					t.Errorf("thread cursor = %q; want the reply's %q", got, h.ago(time.Hour))
				}
			})
		}
	})

	t.Run("ReadRefusedWithNotInChannelJoinsTheBotAndReadsAgain", func(t *testing.T) {
		t.Parallel()
		for _, tt := range fires {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.threadScenario()
				h.slack.failNext("History", notInChannel)
				tt.fire(h)
				if joins := h.slack.logged("Join"); len(joins) != 1 || joins[0].Channel != testChannel {
					t.Errorf("Join calls = %+v; want one for %s", joins, testChannel)
				}
				h.wantPosts(mail{primeOrch, "one"}, mail{primeOrch, "reply"})
			})
		}
	})

	t.Run("FailedReadStaysOwedAndIsMadeAgainOnceItStands", func(t *testing.T) {
		t.Parallel()
		rateLimit := &slackengine.RateLimitError{Method: "conversations.history", RetryAfter: 7 * time.Second}
		tests := []struct {
			name string
			fire func(h *harness)
			err  error
			// wait is the time the read stays owed before it is due.
			wait time.Duration
			// wantReply is whether the read carries the thread scan.
			wantReply bool
		}{
			{"HintedTransportError", hintC1, errors.New("slack: call conversations.history: connection reset"), 0, false},
			{"HintedServerError", hintC1, errors.New("slack: conversations.history answered HTTP 500"), 0, false},
			{"HintedRateLimit", hintC1, rateLimit, 7 * time.Second, false},
			{"ResyncReadWithItsScan", fires[0].fire, errors.New("slack: conversations.history answered HTTP 500"), 0, true},
			{"StartReadWithItsScan", fires[1].fire, errors.New("slack: call conversations.history: connection reset"), 0, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.threadScenario()
				h.slack.failNext("History", tt.err)
				tt.fire(h)
				h.wantPosts()
				if tt.wait > 0 {
					if h.retry() {
						t.Errorf("RetryOwed before the retry wait reported activity")
					}
					h.wantPosts()
					h.clock.Advance(tt.wait)
				}
				if !h.retry() {
					t.Errorf("RetryOwed reported no activity for the owed read")
				}
				want := []mail{{primeOrch, "one"}}
				if tt.wantReply {
					want = append(want, mail{primeOrch, "reply"})
				}
				h.wantPosts(want...)
				if h.retry() {
					t.Errorf("a second RetryOwed reported activity")
				}
				h.wantPosts(want...)
			})
		}
	})

	t.Run("DrainWithoutTheSetupFileReturnsAnErrorNamingTheWayForward", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		if err := os.Remove(h.geom.SetupPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		h.hint(testChannel)
		_, err := h.drain()
		if err == nil || !strings.Contains(err.Error(), h.geom.SetupPath) || !strings.Contains(err.Error(), "lym slack setup --operator <user-id>") {
			t.Errorf("Drain = %v; want an error naming %s and lym slack setup --operator <user-id>", err, h.geom.SetupPath)
		}
	})

	t.Run("RefusedChannelMailsOneNoticePerEpisodeWhileAnotherChannelReads", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		refusal := &slackengine.APIError{Method: "conversations.history", Code: "invalid_auth"}
		h.slack.failChannel(testChannel, refusal)
		for i := range 2 {
			h.sayIn(paymentsChannel, fmt.Sprintf("10%d.000001", i+1), fmt.Sprintf("ok%d", i+1))
			h.hint(testChannel)
			h.hint(paymentsChannel)
			if _, err := h.drain(); !errors.Is(err, refusal) {
				t.Fatalf("Drain #%d = %v; want the refusal", i+1, err)
			}
		}
		notices := h.relayNotices()
		if len(notices) != 1 || !strings.Contains(notices[0], "invalid_auth") || !strings.Contains(notices[0], "lym slack login") ||
			!strings.Contains(notices[0], "docs/slack-app.md") || strings.Contains(notices[0], "lym slack setup") {
			t.Fatalf("notices = %q; want one naming invalid_auth, lym slack login and docs/slack-app.md", notices)
		}
		h.wantPosts(mail{primeOrch, notices[0]}, mail{paymentsOrch, "ok1"}, mail{paymentsOrch, "ok2"})
		h.slack.failChannel(testChannel, nil)
		h.mustReadC1()
		h.slack.failChannel(testChannel, refusal)
		if _, err := h.readC1(); !errors.Is(err, refusal) {
			t.Fatalf("Drain = %v; want the refusal", err)
		}
		if got := len(h.relayNotices()); got != 2 {
			t.Errorf("notices after a recovered episode = %d; want 2", got)
		}
	})

	t.Run("HintsThatNameNoOpenChannelAreRemovedUnreadAndADotHintIsLeftAlone", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hint("C9")
		h.hint("junk_")
		if err := os.WriteFile(filepath.Join(h.geom.InboxDir, ".tmp"), nil, 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		h.mustDrain()
		if calls := h.slack.callCount(); calls != 0 {
			t.Errorf("Slack calls = %d; want none for hints that name no open channel", calls)
		}
		for _, name := range []string{"C9", "junk_"} {
			if _, err := os.Stat(filepath.Join(h.geom.InboxDir, name)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stat(hint %s) = %v; want it removed", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(h.geom.InboxDir, ".tmp")); err != nil {
			t.Errorf("Stat(.tmp) = %v; want the hint being written left in place", err)
		}
	})

	t.Run("HintWrittenDuringAReadStaysInTheInboxForTheNextDrain", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		h.sayIn(paymentsChannel, "101.000001", "two")
		var once sync.Once
		h.slack.onCall = func() { once.Do(func() { h.hint(paymentsChannel) }) }
		h.mustReadC1()
		if _, err := os.Stat(filepath.Join(h.geom.InboxDir, paymentsChannel)); err != nil {
			t.Errorf("Stat(hint written during the read) = %v; want it left in the inbox", err)
		}
		h.wantPosts()
		h.mustDrain()
		h.wantPosts(mail{paymentsOrch, "two"})
	})

	t.Run("CursorForwardsEachMessageOnceWhicheverReadFindsItFirst", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "read by Start")
		h.in.Start()
		h.retry()
		h.say("102.000001", "after a lost hint")
		h.say("103.000001", "hinted")
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "read by Start"}, mail{primeOrch, "after a lost hint"}, mail{primeOrch, "hinted"})
	})

	t.Run("ThreadCursorOlderThanADayIsPrunedAndTheFloorRaised", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		parent, older, newer := h.ago(26*time.Hour), h.ago(24*time.Hour+30*time.Minute), h.ago(time.Hour)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) {
			entry.Cursor = newer
			entry.Threads = map[string]string{parent: h.ago(25 * time.Hour)}
		})
		h.slack.add(paymentsChannel, slackengine.Message{TS: parent, User: testOperator, Text: "parent"})
		h.slack.add(paymentsChannel, slackengine.Message{TS: older, ThreadTS: parent, User: testOperator, Text: "earlier reply"})
		h.slack.add(paymentsChannel, slackengine.Message{TS: newer, ThreadTS: parent, User: testOperator, Text: "later reply"})
		h.hint(paymentsChannel + "_" + parent)
		h.mustDrain()
		h.wantPosts(mail{paymentsOrch, "later reply"})
		entry := h.entryOf(paymentsOrch)
		if wantFloor := fmt.Sprintf("%d.000000", h.clock.Now().Add(-24*time.Hour).Unix()); entry.ThreadFloor != wantFloor {
			t.Errorf("thread floor = %q; want the cut-off %q", entry.ThreadFloor, wantFloor)
		}
		if entry.Threads[parent] != newer {
			t.Errorf("thread cursor = %q; want the forwarded reply's %q", entry.Threads[parent], newer)
		}
	})

	t.Run("EntryWhoseFloorStandsAtItsNewestMessageForwardsNoEarlierReplyAtStart", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		parent, reply, newest := h.ago(2*time.Hour), h.ago(time.Hour), h.ago(10*time.Minute)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) {
			entry.Cursor, entry.ThreadFloor = newest, newest
		})
		h.slack.add(paymentsChannel, slackengine.Message{TS: parent, User: testOperator, Text: "parent", LatestReply: reply})
		h.slack.add(paymentsChannel, slackengine.Message{TS: reply, ThreadTS: parent, User: testOperator, Text: "earlier reply"})
		h.in.Start()
		h.retry()
		h.wantPosts()
	})

	t.Run("OperatorThreadReplyAndBroadcastReachTheChannelsOrchOnce", func(t *testing.T) {
		t.Parallel()
		parent, broadcast, reply := func(h *harness) string { return h.ago(2 * time.Hour) }, func(h *harness) string { return h.ago(time.Hour) }, func(h *harness) string { return h.ago(30 * time.Minute) }
		prepare := func(t *testing.T) *harness {
			h := newHarness(t)
			h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = parent(h) })
			h.slack.add(paymentsChannel, slackengine.Message{TS: parent(h), User: testOperator, Text: "parent", LatestReply: reply(h)})
			h.slack.add(paymentsChannel, slackengine.Message{TS: broadcast(h), ThreadTS: parent(h), Subtype: "thread_broadcast", User: testOperator, Text: "both"})
			h.slack.add(paymentsChannel, slackengine.Message{TS: reply(h), ThreadTS: parent(h), User: testOperator, Text: "reply"})
			return h
		}

		t.Run("ThreadHintThenChannelHint", func(t *testing.T) {
			t.Parallel()
			h := prepare(t)
			h.hint(paymentsChannel + "_" + parent(h))
			h.mustDrain()
			h.wantPosts(mail{paymentsOrch, "reply"})
			h.hint(paymentsChannel)
			h.mustDrain()
			h.hint(paymentsChannel + "_" + parent(h))
			h.mustDrain()
			h.wantPosts(mail{paymentsOrch, "reply"}, mail{paymentsOrch, "both"})
		})

		t.Run("StartReadsTheChannelAndTheThread", func(t *testing.T) {
			t.Parallel()
			h := prepare(t)
			h.in.Start()
			h.retry()
			h.wantPosts(mail{paymentsOrch, "both"}, mail{paymentsOrch, "reply"})
		})
	})

	t.Run("FileShareForwardsItsTextAndAFileLinePerFileAndOtherMessagesAreSkipped", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.slack.add(testChannel, slackengine.Message{TS: "101.000001", User: "U2", Text: "someone else"})
		h.slack.add(testChannel, slackengine.Message{TS: "102.000001", User: testOperator, BotID: "B1", Text: "a bot"})
		h.slack.add(testChannel, slackengine.Message{TS: "103.000001", User: testOperator, Subtype: "channel_join", Text: "joined"})
		h.slack.add(testChannel, slackengine.Message{
			TS: "104.000001", User: testOperator, Subtype: "file_share", Text: "see <https://x.io/a|doc>",
			Files: []slackengine.File{{Name: "a.png"}, {Name: "b.pdf"}},
		})
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "see https://x.io/a\n(file not relayed: a.png)\n(file not relayed: b.pdf)"})
		h.wantAnswers()
		h.wantCursorTS("104.000001")
	})

	t.Run("MessageWithNoTextAndNoFilesPostsNothingAndAdvancesTheCursor", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.slack.add(testChannel, slackengine.Message{TS: "101.000001", User: testOperator, Text: ""})
		h.mustReadC1()
		h.wantPosts()
		h.wantAnswers()
		h.wantCursorTS("101.000001")
	})

	t.Run("OperatorChangedInTheSetupIsTheOneForwardedAtTheNextDrain", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "from the first operator")
		h.mustReadC1()
		if err := slackengine.WriteSetup(h.geom.SetupPath, slackengine.Setup{OperatorID: "U2"}); err != nil {
			t.Fatalf("WriteSetup = %v", err)
		}
		h.say("102.000001", "from the first operator again")
		h.slack.add(testChannel, slackengine.Message{TS: "103.000001", User: "U2", Text: "from the second operator"})
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "from the first operator"}, mail{primeOrch, "from the second operator"})
	})

	t.Run("SlackMarkupReachesTheOrchAsPlainText", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "see <https://x.io/a|label> <https://x.io/a> <#C1|general> <!here> <@U1> &amp; &lt;c&gt;")
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "see https://x.io/a https://x.io/a <#C1|general> <!here> <@U1> & <c>"})
	})

	t.Run("PresenceCommandsInAnyChannelSetAndReadThePresenceAndAreAnsweredThere", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beams = []string{"payments"} })
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		h.sayIn(paymentsChannel, "101.000001", "away")
		h.sayIn(paymentsChannel, "102.000001", " Back ")
		h.sayIn(paymentsChannel, "103.000001", "status")
		h.hint(paymentsChannel)
		h.mustDrain()
		h.wantAnswers("presence is now slack", "presence is now terminal", "presence is terminal")
		for _, call := range h.slack.logged("PostAs") {
			if call.Channel != paymentsChannel {
				t.Errorf("an answer was posted to %s; want %s", call.Channel, paymentsChannel)
			}
		}
		h.wantPosts(
			mail{primeOrch, "[slack presence] presence is now slack"},
			mail{paymentsOrch, "[slack presence] presence is now slack"},
			mail{primeOrch, "[slack presence] presence is now terminal"},
			mail{paymentsOrch, "[slack presence] presence is now terminal"},
		)
	})

	t.Run("AnswerRefusedWithNotInChannelJoinsTheChannelAndPostsAgain", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.updateEntry(paymentsOrch, func(entry *slackengine.ChannelEntry) { entry.Cursor = "100.000000" })
		h.sayIn(paymentsChannel, "101.000001", "status")
		h.slack.failPosts(&slackengine.APIError{Method: "chat.postMessage", Code: "not_in_channel"})
		h.hint(paymentsChannel)
		h.mustDrain()
		if joins := h.slack.logged("Join"); len(joins) != 1 || joins[0].Channel != paymentsChannel {
			t.Errorf("Join calls = %+v; want one for %s", joins, paymentsChannel)
		}
		h.wantAnswers("presence is terminal")
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
			if _, err := h.readC1(); err == nil {
				t.Fatalf("Drain = nil; want the post error")
			}
			h.wantCursorTS("100.000000")
			h.wantAnswers()
			h.mailbox.setPostErr(nil)
			h.clock.Advance(time.Minute)
			if !h.retry() {
				t.Errorf("RetryOwed reported no activity for the held messages")
			}
			h.wantCursorTS("102.000001")
			h.wantPosts(mail{primeOrch, "m1"}, mail{primeOrch, "[slack presence] presence is now slack"})
		})

		t.Run("PastFiveMinutesIsAnsweredOnceAndPassed", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.startAt("100.000000")
			h.say("101.000001", "m1")
			h.mailbox.setPostErr(errors.New("creel down"))
			if _, err := h.readC1(); err == nil {
				t.Fatalf("Drain = nil; want the post error")
			}
			h.clock.Advance(5 * time.Minute)
			h.retry()
			h.retry()
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
			{"AtTheCap", 4096, true},
			{"OverTheCap", 4097, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				h.startAt("100.000000")
				h.say("101.000001", strings.Repeat("x", tt.textBytes))
				h.mustReadC1()
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
		h.hub.set(func(h *fakeHub) { h.paused[primeOrch] = true })
		h.startAt("100.000000")
		h.say("101.000001", "hello")
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "hello"})
		h.wantAnswers("the hub is paused; your message waits for the resume")
	})

	t.Run("PausedErrorAfterAnAcceptedPostStillMovesTheCursor", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.pausedErr = errors.New("pause record unreadable") })
		h.startAt("100.000000")
		h.say("101.000001", "hello")
		h.mustReadC1()
		h.mustReadC1()
		h.wantPosts(mail{primeOrch, "hello"})
		h.wantAnswers()
		h.wantCursorTS("101.000001")
	})

	t.Run("AwayWhoseBeamsReadFailsAfterTheWriteIsAnsweredAndPassed", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hub.set(func(h *fakeHub) { h.beamsErr = errors.New("board unreadable") })
		h.startAt("100.000000")
		h.say("101.000001", "away")
		h.mustReadC1()
		h.mustReadC1()
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
		if _, err := h.readC1(); err == nil {
			t.Fatalf("Drain = nil; want the presence write error")
		}
		h.wantAnswers()
		h.wantCursorTS("100.000000")
		if err := os.Remove(h.geom.PresencePath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		if !h.retry() {
			t.Errorf("RetryOwed reported no activity once the write works")
		}
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
		if _, err := h.readC1(); err == nil {
			t.Fatalf("Drain = nil; want the presence read error")
		}
		h.wantAnswers()
		h.wantCursorTS("100.000000")
		h.clock.Advance(5 * time.Minute)
		h.retry()
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
		if _, err := h.readC1(); err == nil {
			t.Fatalf("Drain = nil; want the post error")
		}
		h.clock.Advance(5 * time.Minute)
		h.retry()
		h.wantAnswers("your message was not delivered; send it again: a &amp; &lt;b&gt;")
	})

	t.Run("AnswerSlackRefusesStillMovesTheCursor", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.startAt("100.000000")
		h.say("101.000001", "status")
		h.slack.failPosts(errors.New("slack down"))
		h.mustReadC1()
		h.wantCursorTS("101.000001")
		h.wantAnswers()
	})

	t.Run("PresenceChangedByAnotherWriterIsActivity", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		if h.mustDrain() {
			t.Errorf("the first Drain reported activity")
		}
		if _, err := slackengine.WritePresence(h.geom.PresencePath, slackengine.PresenceSlack, h.clock.Now()); err != nil {
			t.Fatalf("WritePresence = %v", err)
		}
		if !h.mustDrain() {
			t.Errorf("Drain after another writer changed the presence reported no activity")
		}
		if h.mustDrain() {
			t.Errorf("Drain over an unchanged presence reported activity")
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
