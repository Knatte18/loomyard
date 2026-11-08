// holder_test.go covers the socket holder over a manual clock, scripted sockets and dials,
// and the temp directories of two hubs.
// It also declares the fakes of the Socket Mode connection and the clock that only advances when a row says so.

package slackengine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// awaitTimeout bounds every wait of a row on the holder; a correct holder never reaches it.
const awaitTimeout = 10 * time.Second

// errPingBlocks is the scripted ping answer that makes a ping wait until its context ends.
var errPingBlocks = errors.New("the ping blocks")

// manualClock is a Clock whose time moves only when a row advances it.
// After records a pending wait and returns a channel that delivers only once Advance moves the time past its deadline.
type manualClock struct {
	mu      sync.Mutex
	now     time.Time
	waits   []*manualWait
	changed chan struct{}
}

// manualWait is one pending wait of a manualClock.
type manualWait struct {
	duration  time.Duration
	deadline  time.Time
	delivered chan time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), changed: make(chan struct{})}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := &manualWait{duration: d, deadline: c.now.Add(d), delivered: make(chan time.Time, 1)}
	c.waits = append(c.waits, wait)
	c.signalChange()
	return wait.delivered
}

// Advance moves the time forward and delivers every wait whose deadline has passed.
func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	remaining := c.waits[:0]
	for _, wait := range c.waits {
		if wait.deadline.After(c.now) {
			remaining = append(remaining, wait)
			continue
		}
		wait.delivered <- c.now
	}
	c.waits = remaining
	c.signalChange()
}

// pending lists the durations of the waits not yet delivered, in the order they were taken.
func (c *manualClock) pending() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	durations := make([]time.Duration, 0, len(c.waits))
	for _, wait := range c.waits {
		durations = append(durations, wait.duration)
	}
	return durations
}

// signalChange wakes the rows waiting on a change of the pending waits; the caller holds the lock.
func (c *manualClock) signalChange() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// awaitWait blocks until a wait of d is pending.
func (c *manualClock) awaitWait(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.After(awaitTimeout)
	for {
		c.mu.Lock()
		changed := c.changed
		found := false
		for _, wait := range c.waits {
			found = found || wait.duration == d
		}
		c.mu.Unlock()
		if found {
			return
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("no wait of %s became pending; pending waits are %v", d, c.pending())
		}
	}
}

// socketEvent is one thing a fakeSocket's Read answers: an envelope or an error.
type socketEvent struct {
	envelope slackengine.Envelope
	err      error
}

// fakeSocket is a Socket that delivers each envelope or error a row sends, and records acknowledgements and pings.
type fakeSocket struct {
	t        *testing.T
	incoming chan socketEvent
	pinged   chan struct{}
	closed   chan struct{}
	closing  sync.Once

	mu         sync.Mutex
	acked      map[string]chan struct{}
	pingScript []error
}

func newFakeSocket(t *testing.T) *fakeSocket {
	return &fakeSocket{
		t:        t,
		incoming: make(chan socketEvent),
		pinged:   make(chan struct{}, 64),
		closed:   make(chan struct{}),
		acked:    map[string]chan struct{}{},
	}
}

func (s *fakeSocket) Read(ctx context.Context) (slackengine.Envelope, error) {
	select {
	case event := <-s.incoming:
		return event.envelope, event.err
	case <-ctx.Done():
		return slackengine.Envelope{}, ctx.Err()
	case <-s.closed:
		return slackengine.Envelope{}, errors.New("the socket is closed")
	}
}

// ackChannel returns the channel that is closed once the envelope with id is acknowledged.
func (s *fakeSocket) ackChannel(id string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ack, ok := s.acked[id]
	if !ok {
		ack = make(chan struct{})
		s.acked[id] = ack
	}
	return ack
}

func (s *fakeSocket) Ack(_ context.Context, envelopeID string) error {
	ack := s.ackChannel(envelopeID)
	select {
	case <-ack:
	default:
		close(ack)
	}
	return nil
}

func (s *fakeSocket) Ping(ctx context.Context) error {
	s.mu.Lock()
	var answer error
	if len(s.pingScript) > 0 {
		answer, s.pingScript = s.pingScript[0], s.pingScript[1:]
	}
	s.mu.Unlock()
	select {
	case s.pinged <- struct{}{}:
	default:
	}
	if errors.Is(answer, errPingBlocks) {
		<-ctx.Done()
		return ctx.Err()
	}
	return answer
}

func (s *fakeSocket) Close() error {
	s.closing.Do(func() { close(s.closed) })
	return nil
}

// scriptPings queues the answers of the next pings, one per ping.
func (s *fakeSocket) scriptPings(answers ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingScript = append(s.pingScript, answers...)
}

func (s *fakeSocket) deliver(event socketEvent) {
	s.t.Helper()
	select {
	case s.incoming <- event:
	case <-time.After(awaitTimeout):
		s.t.Fatalf("the holder did not read from the socket")
	}
}

// send delivers an envelope to the holder's reader.
func (s *fakeSocket) send(envelope slackengine.Envelope) {
	s.t.Helper()
	s.deliver(socketEvent{envelope: envelope})
}

// fail makes the socket's Read return err.
func (s *fakeSocket) fail(err error) {
	s.t.Helper()
	s.deliver(socketEvent{err: err})
}

// sendEventAndAwaitAck delivers the envelope and waits for the holder to acknowledge it.
func (s *fakeSocket) sendEventAndAwaitAck(envelope slackengine.Envelope) {
	s.t.Helper()
	s.send(envelope)
	s.awaitAck(envelope.EnvelopeID)
}

func (s *fakeSocket) awaitAck(id string) {
	s.t.Helper()
	select {
	case <-s.ackChannel(id):
	case <-time.After(awaitTimeout):
		s.t.Fatalf("the envelope %s was not acknowledged", id)
	}
}

// isAcked reports whether the envelope with id has been acknowledged.
func (s *fakeSocket) isAcked(id string) bool {
	select {
	case <-s.ackChannel(id):
		return true
	default:
		return false
	}
}

func (s *fakeSocket) awaitPing() {
	s.t.Helper()
	select {
	case <-s.pinged:
	case <-time.After(awaitTimeout):
		s.t.Fatalf("the holder sent no ping")
	}
}

// eventsEnvelope returns an `events_api` envelope for a message in the channel.
func eventsEnvelope(id, channel, ts string) slackengine.Envelope {
	return slackengine.Envelope{Type: "events_api", EnvelopeID: id, Channel: channel, TS: ts}
}

// dialStep is one scripted answer of a fakeDialer.
type dialStep struct {
	socket *fakeSocket
	err    error
	dialed chan struct{}
}

// fakeDialer is a Dialer that answers a script and blocks until its context ends once the script is used up.
type fakeDialer struct {
	mu    sync.Mutex
	steps []dialStep
	dials int
}

// hand scripts a dial that hands out socket, and returns a channel closed when that dial happens.
func (d *fakeDialer) hand(socket *fakeSocket) <-chan struct{} {
	return d.script(dialStep{socket: socket})
}

// fail scripts a dial that fails with err, and returns a channel closed when that dial happens.
func (d *fakeDialer) fail(err error) <-chan struct{} {
	return d.script(dialStep{err: err})
}

func (d *fakeDialer) script(step dialStep) <-chan struct{} {
	step.dialed = make(chan struct{})
	d.mu.Lock()
	defer d.mu.Unlock()
	d.steps = append(d.steps, step)
	return step.dialed
}

func (d *fakeDialer) dial(ctx context.Context) (slackengine.Socket, string, error) {
	d.mu.Lock()
	if len(d.steps) == 0 {
		d.mu.Unlock()
		<-ctx.Done()
		return nil, "", ctx.Err()
	}
	step := d.steps[0]
	d.steps = d.steps[1:]
	d.dials++
	d.mu.Unlock()
	close(step.dialed)
	if step.err != nil {
		return nil, "", step.err
	}
	return step.socket, "A1", nil
}

func (d *fakeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

// fakeLive is the live-hub seam: a list of hubs, an error, and an optional block that holds a call until released.
type fakeLive struct {
	mu    sync.Mutex
	hubs  []slackengine.Geometry
	err   error
	block chan struct{}
}

func (l *fakeLive) list() ([]slackengine.Geometry, error) {
	l.mu.Lock()
	hubs, err, block := append([]slackengine.Geometry(nil), l.hubs...), l.err, l.block
	l.mu.Unlock()
	if block != nil {
		<-block
	}
	return hubs, err
}

func (l *fakeLive) setErr(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.err = err
}

func (l *fakeLive) setHubs(hubs ...slackengine.Geometry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hubs = hubs
}

// blockCalls makes every later call wait until the returned release runs, which the test's cleanup also runs.
func (l *fakeLive) blockCalls(t *testing.T) (release func()) {
	t.Helper()
	block := make(chan struct{})
	l.mu.Lock()
	l.block = block
	l.mu.Unlock()
	var once sync.Once
	release = func() {
		once.Do(func() {
			l.mu.Lock()
			l.block = nil
			l.mu.Unlock()
			close(block)
		})
	}
	t.Cleanup(release)
	return release
}

// hintWriter is the holder's hint writer with a failure and a block a row can switch on for one hub.
type hintWriter struct {
	mu         sync.Mutex
	failures   map[string]error
	block      chan struct{}
	blockedHub string
}

func (w *hintWriter) write(geom slackengine.Geometry, name string) error {
	w.mu.Lock()
	failure, block, blockedHub := w.failures[geom.HubPath], w.block, w.blockedHub
	w.mu.Unlock()
	if block != nil && geom.HubPath == blockedHub {
		<-block
	}
	if failure != nil {
		return failure
	}
	return slackengine.WriteHint(geom, name)
}

// failFor makes the writes into the hub's inbox fail with err; a nil err makes them stand again.
func (w *hintWriter) failFor(geom slackengine.Geometry, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failures == nil {
		w.failures = map[string]error{}
	}
	w.failures[geom.HubPath] = err
}

// blockFor makes the writes into the hub's inbox wait until the returned release runs.
// The test's cleanup also runs the release.
func (w *hintWriter) blockFor(t *testing.T, geom slackengine.Geometry) (release func()) {
	t.Helper()
	block := make(chan struct{})
	w.mu.Lock()
	w.block, w.blockedHub = block, geom.HubPath
	w.mu.Unlock()
	var once sync.Once
	release = func() { once.Do(func() { close(block) }) }
	t.Cleanup(release)
	return release
}

// runningHolder is a holder running in a goroutine of the test.
type runningHolder struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
}

// stop ends the holder's context and waits for Run to return.
func (r *runningHolder) stop() {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(awaitTimeout):
		r.t.Errorf("the holder's Run did not return")
	}
}

// holderRig is two live hubs that share one socket lock, with their seams.
type holderRig struct {
	t       *testing.T
	a, b    slackengine.Geometry
	clock   *manualClock
	mailbox *fakeMailbox
	slack   *fakeSlack
	live    *fakeLive
}

// newHolderRig returns a rig whose hubs each hold one prime channel in their channel map: CA for hub a and CB for hub b.
func newHolderRig(t *testing.T) *holderRig {
	t.Helper()
	a, b := newGeometry(t), newGeometry(t)
	b.SocketLockPath = a.SocketLockPath
	r := &holderRig{
		t: t, a: a, b: b,
		clock:   newManualClock(),
		mailbox: &fakeMailbox{},
		slack:   newFakeSlack(),
		live:    &fakeLive{hubs: []slackengine.Geometry{a, b}},
	}
	r.mapPrimeChannel(a, "CA")
	r.mapPrimeChannel(b, "CB")
	return r
}

// mapPrimeChannel records an open prime channel with the id in the hub's channel map.
func (r *holderRig) mapPrimeChannel(geom slackengine.Geometry, id string) {
	r.t.Helper()
	err := slackengine.UpdateChannelMap(geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
		channelMap.Channels["ly:orch"] = slackengine.ChannelEntry{ID: id, Name: "ly-neve-prime"}
		return nil
	})
	if err != nil {
		r.t.Fatalf("UpdateChannelMap = %v", err)
	}
}

// holder builds a holder for the hub over the dialer; a nil write keeps the holder's own hint writer.
func (r *holderRig) holder(geom slackengine.Geometry, dialer *fakeDialer, writer *hintWriter) *slackengine.SocketHolder {
	var write func(slackengine.Geometry, string) error
	if writer != nil {
		write = writer.write
	}
	return slackengine.NewSocketHolder(geom, dialer.dial, r.live.list, r.mailbox, r.slack, r.clock, write)
}

// start runs the holder in a goroutine and stops it when the test ends.
func (r *holderRig) start(holder *slackengine.SocketHolder) *runningHolder {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	running := &runningHolder{t: r.t, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(running.done)
		holder.Run(ctx)
	}()
	r.t.Cleanup(running.stop)
	return running
}

// notices returns the bodies of the `[slack relay]` notices mailed to the hub's orch.
func (r *holderRig) notices() []string {
	var bodies []string
	for _, posted := range r.mailbox.accepted() {
		if posted.Receiver == "ly:orch" && strings.HasPrefix(posted.Body, "[slack relay]") {
			bodies = append(bodies, posted.Body)
		}
	}
	return bodies
}

// awaitNotices blocks until the hub's orch holds want notices, and returns them.
func (r *holderRig) awaitNotices(want int) []string {
	r.t.Helper()
	eventually(r.t, fmt.Sprintf("%d notices", want), func() bool { return len(r.notices()) >= want })
	return r.notices()
}

// awaitResyncHints blocks until both hubs' inboxes hold a resync hint.
func (r *holderRig) awaitResyncHints() {
	r.t.Helper()
	awaitHint(r.t, r.a, slackengine.ResyncHint)
	awaitHint(r.t, r.b, slackengine.ResyncHint)
}

// removeResyncHints removes the resync hint from both hubs' inboxes, so a row sees it written again.
func (r *holderRig) removeResyncHints() {
	r.t.Helper()
	for _, geom := range []slackengine.Geometry{r.a, r.b} {
		if err := os.Remove(filepath.Join(geom.InboxDir, slackengine.ResyncHint)); err != nil {
			r.t.Fatalf("remove the resync hint of %s = %v", geom.InboxDir, err)
		}
	}
}

// eventually polls condition on a short tick until it holds, and fails the test at awaitTimeout.
func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.After(awaitTimeout)
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for !condition() {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// awaitClosed blocks until the channel is closed.
func awaitClosed(t *testing.T, closed <-chan struct{}) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(awaitTimeout):
		t.Fatalf("the awaited dial did not happen")
	}
}

// hintSize returns the size of the hint in the hub's inbox, and reports whether it exists.
func hintSize(geom slackengine.Geometry, name string) (int64, bool) {
	info, err := os.Stat(filepath.Join(geom.InboxDir, name))
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

// awaitHint blocks until the hint exists in the hub's inbox.
func awaitHint(t *testing.T, geom slackengine.Geometry, name string) {
	t.Helper()
	eventually(t, "the hint "+name+" in "+geom.InboxDir, func() bool {
		_, found := hintSize(geom, name)
		return found
	})
}

// TestSocketHolder covers the lock and the routing of events into hints.
// It also covers the owed resyncs, the notices and the reconnect waits.
func TestSocketHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const pingInterval, pongTimeout, envelopeLimit = slackengine.PingInterval, slackengine.PongTimeout, slackengine.EnvelopeLimit

	t.Run("OnlyTheLockHolderDialsAndTheNextOneDialsOnceItEnds", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		socketA := newFakeSocket(t)
		dialerA, dialerB := &fakeDialer{}, &fakeDialer{}
		dialedA := dialerA.hand(socketA)
		dialedB := dialerB.hand(newFakeSocket(t))

		runA := r.start(r.holder(r.a, dialerA, nil))
		awaitClosed(t, dialedA)
		r.start(r.holder(r.b, dialerB, nil))
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		socketA.awaitPing()
		if got := dialerB.count(); got != 0 {
			t.Errorf("the second holder dialed %d times while the first held the lock; want 0", got)
		}

		runA.stop()
		awaitClosed(t, dialedB)
	})

	t.Run("AHolderEndedWhileBlockedOnTheLockReleasesItAtOnceWhenItGetsIt", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialerA, dialerB, dialerC := &fakeDialer{}, &fakeDialer{}, &fakeDialer{}
		dialedA := dialerA.hand(newFakeSocket(t))
		dialedC := dialerC.hand(newFakeSocket(t))

		runA := r.start(r.holder(r.a, dialerA, nil))
		awaitClosed(t, dialedA)
		runB := r.start(r.holder(r.b, dialerB, nil))
		runB.stop()
		runA.stop()

		r.start(r.holder(r.a, dialerC, nil))
		awaitClosed(t, dialedC)
		if got := dialerB.count(); got != 0 {
			t.Errorf("the holder ended while blocked dialed %d times; want 0", got)
		}
	})

	t.Run("AnEventWritesItsHintIntoTheOwningHubsInboxBeforeTheAck", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))
		r.awaitResyncHints()

		rows := []struct {
			name     string
			envelope slackengine.Envelope
			hub      slackengine.Geometry
			wantHint string
		}{
			{"TopLevelMessageWritesTheChannel", eventsEnvelope("e1", "CB", "10.1"), r.b, "CB"},
			{"ThreadReplyWritesTheChannelAndThread", slackengine.Envelope{Type: "events_api", EnvelopeID: "e2", Channel: "CB", TS: "11.1", ThreadTS: "10.1"}, r.b, "CB_10.1"},
			{"ThreadBroadcastWritesTheChannel", slackengine.Envelope{Type: "events_api", EnvelopeID: "e3", Channel: "CA", TS: "12.1", ThreadTS: "10.1", Subtype: "thread_broadcast"}, r.a, "CA"},
		}
		for _, row := range rows {
			if _, found := hintSize(row.hub, row.wantHint); found {
				t.Fatalf("%s: the hint %s exists before the event", row.name, row.wantHint)
			}
			socket.sendEventAndAwaitAck(row.envelope)
			if size, found := hintSize(row.hub, row.wantHint); !found || size != 0 {
				t.Errorf("%s: hint %s in %s = size %d, found %v at the ack; want an empty hint", row.name, row.wantHint, row.hub.InboxDir, size, found)
			}
		}
	})

	t.Run("AFailingHintWriteAcksOwesAResyncAndMailsOneNoticePerEpisode", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		writer := &hintWriter{}
		diskFull := errors.New("disk full")
		writer.failFor(r.b, diskFull)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, writer))

		awaitHint(t, r.a, slackengine.ResyncHint)
		notices := r.awaitNotices(1)
		if !strings.Contains(notices[0], r.b.InboxDir) || !strings.Contains(notices[0], diskFull.Error()) {
			t.Errorf("notice = %q; want it to name %s and %q", notices[0], r.b.InboxDir, diskFull)
		}
		socket.sendEventAndAwaitAck(eventsEnvelope("e1", "CB", "10.1"))
		socket.sendEventAndAwaitAck(eventsEnvelope("e2", "CB", "10.2"))
		socket.sendEventAndAwaitAck(eventsEnvelope("e3", "CA", "10.3"))
		if _, found := hintSize(r.a, "CA"); !found {
			t.Errorf("the event for hub A wrote no hint while hub B's writes failed")
		}
		if got := len(r.notices()); got != 1 {
			t.Errorf("notices across three failures = %d; want 1 for the episode", got)
		}

		writer.failFor(r.b, nil)
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		awaitHint(t, r.b, slackengine.ResyncHint)

		writer.failFor(r.b, diskFull)
		socket.sendEventAndAwaitAck(eventsEnvelope("e4", "CB", "10.4"))
		r.awaitNotices(2)
	})

	t.Run("ABlockedHintWriterStallsNeitherTheAckNorThePing", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		writer := &hintWriter{}
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, writer))
		r.awaitResyncHints()

		release := writer.blockFor(t, r.b)
		socket.send(eventsEnvelope("e1", "CB", "10.1"))
		r.clock.awaitWait(t, envelopeLimit)
		r.clock.Advance(envelopeLimit)
		socket.awaitAck("e1")

		if err := os.Remove(filepath.Join(r.b.InboxDir, slackengine.ResyncHint)); err != nil {
			t.Fatalf("remove the resync hint = %v", err)
		}
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		socket.awaitPing()
		eventually(t, "the pong to stamp the last-healthy time", func() bool {
			relayState, found, err := slackengine.ReadRelayState(r.a)
			return err == nil && found && relayState.LastSlackOK.Equal(r.clock.Now())
		})

		release()
		awaitHint(t, r.b, slackengine.ResyncHint)
	})

	t.Run("ABlockedLiveHubsReadOwesEveryHubOfTheLastReadAResync", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))
		r.awaitResyncHints()

		release := r.live.blockCalls(t)
		socket.send(eventsEnvelope("e1", "CB", "10.1"))
		r.clock.awaitWait(t, envelopeLimit)
		r.clock.Advance(envelopeLimit)
		socket.awaitAck("e1")
		release()
		awaitHint(t, r.b, "CB")

		r.removeResyncHints()
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		r.awaitResyncHints()
	})

	t.Run("UnmappedEventsAreAckedOnlyWhileEveryMapReads", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))
		r.awaitResyncHints()

		if err := os.WriteFile(r.b.ChannelMapPath, []byte("{"), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		socket.send(eventsEnvelope("e1", "CX", "10.1"))
		socket.sendEventAndAwaitAck(eventsEnvelope("e2", "CA", "10.2"))
		if socket.isAcked("e1") {
			t.Errorf("an unmapped event was acked while hub B's map could not be read")
		}
		if err := os.WriteFile(r.b.ChannelMapPath, []byte(`{"channels":{"ly:orch":{"id":"CB"}}}`), 0o600); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		socket.sendEventAndAwaitAck(eventsEnvelope("e3", "CX", "10.3"))
	})

	t.Run("AFailingLiveHubsReadLeavesEventsUnackedAndNoticesOncePerEpisode", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		var mailsAtFirstPost atomic.Int64
		mailsAtFirstPost.Store(-1)
		r.slack.onPost = func() { mailsAtFirstPost.CompareAndSwap(-1, int64(len(r.mailbox.accepted()))) }
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))
		r.awaitResyncHints()

		const hubsRecord = "/machine/hubs.json"
		r.live.setErr(fmt.Errorf("read the hubs record %s: boom", hubsRecord))
		socket.send(eventsEnvelope("e1", "CA", "10.1"))
		notices := r.awaitNotices(1)
		socket.send(eventsEnvelope("e2", "CA", "10.2"))
		if !strings.Contains(notices[0], hubsRecord) {
			t.Errorf("notice = %q; want it to carry the error naming %s", notices[0], hubsRecord)
		}
		eventually(t, "the notice to reach hub A's prime channel", func() bool { return mailsAtFirstPost.Load() >= 0 })
		if got := mailsAtFirstPost.Load(); got != 0 {
			t.Errorf("mails at the post to the prime channel = %d; want the post first", got)
		}
		for _, text := range r.slack.texts() {
			if !strings.Contains(text, hubsRecord) {
				t.Errorf("post to the prime channel = %q; want it to carry the error naming %s", text, hubsRecord)
			}
		}
		if socket.isAcked("e1") || socket.isAcked("e2") {
			t.Errorf("an event was acked while the live-hub read failed")
		}
		if got := len(r.notices()); got != 1 {
			t.Errorf("notices across two failed reads = %d; want 1 for the episode", got)
		}

		r.removeResyncHints()
		r.live.setErr(nil)
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		r.awaitResyncHints()

		c := newGeometry(t)
		r.live.setHubs(r.a, r.b, c)
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		awaitHint(t, c, slackengine.ResyncHint)
	})

	t.Run("EveryEndOfAConnectionLeadsToANewDialAndAResyncOnTheNewConnection", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name string
			end  func(t *testing.T, r *holderRig, socket *fakeSocket)
		}{
			{"Disconnect", func(_ *testing.T, _ *holderRig, socket *fakeSocket) {
				socket.send(slackengine.Envelope{Type: "disconnect", Reason: "refresh_requested"})
			}},
			{"ReadError", func(_ *testing.T, _ *holderRig, socket *fakeSocket) {
				socket.fail(errors.New("connection reset"))
			}},
			{"FailedPing", func(t *testing.T, r *holderRig, socket *fakeSocket) {
				socket.scriptPings(errors.New("pong lost"))
				r.clock.awaitWait(t, pingInterval)
				r.clock.Advance(pingInterval)
			}},
			{"PingPastThePongTimeout", func(t *testing.T, r *holderRig, socket *fakeSocket) {
				socket.scriptPings(errPingBlocks)
				r.clock.awaitWait(t, pingInterval)
				r.clock.Advance(pingInterval)
				socket.awaitPing()
				r.clock.awaitWait(t, pongTimeout)
				r.clock.Advance(pongTimeout)
			}},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				r := newHolderRig(t)
				dialer, first := &fakeDialer{}, newFakeSocket(t)
				dialedFirst := dialer.hand(first)
				dialedSecond := dialer.hand(newFakeSocket(t))
				r.start(r.holder(r.a, dialer, nil))
				awaitClosed(t, dialedFirst)
				r.awaitResyncHints()
				r.removeResyncHints()

				row.end(t, r, first)
				r.clock.awaitWait(t, slackengine.ReconnectFloor)
				r.clock.Advance(slackengine.ReconnectFloor)
				awaitClosed(t, dialedSecond)
				r.awaitResyncHints()
			})
		}
	})

	t.Run("ReconnectWaitsDoubleToTheCeilingAndResetAfterAStableConnection", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		waits := []time.Duration{1, 2, 4, 8, 16, 32, 60}
		for range waits {
			dialer.fail(errors.New("dial refused"))
		}
		dialedSocket := dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))

		for _, seconds := range waits {
			r.clock.awaitWait(t, seconds*time.Second)
			r.clock.Advance(seconds * time.Second)
		}
		awaitClosed(t, dialedSocket)
		// The ping wait is pending only once the holder has stamped the connection's start,
		// so the clock is advanced after it and the connection reads as stable.
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(slackengine.StableConnection)
		socket.fail(errors.New("connection reset"))
		r.clock.awaitWait(t, slackengine.ReconnectFloor)
	})

	t.Run("ARefusedAppMailsOneNoticePerEpisode", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		refused := &slackengine.APIError{Method: "apps.connections.open", Code: "invalid_auth"}
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.fail(refused)
		dialer.fail(refused)
		dialer.fail(refused)
		dialedSocket := dialer.hand(socket)
		dialedRefusedAgain := dialer.fail(refused)
		r.start(r.holder(r.a, dialer, nil))

		r.clock.awaitWait(t, 1*time.Second)
		r.clock.Advance(1 * time.Second)
		r.clock.awaitWait(t, 2*time.Second)
		r.clock.Advance(2 * time.Second)
		r.clock.awaitWait(t, 4*time.Second)
		notices := r.notices()
		if len(notices) != 1 || !strings.Contains(notices[0], "lym slack login") || !strings.Contains(notices[0], "invalid_auth") {
			t.Fatalf("notices across three refusals = %q; want one naming lym slack login and invalid_auth", notices)
		}

		r.clock.Advance(4 * time.Second)
		awaitClosed(t, dialedSocket)
		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(slackengine.StableConnection)
		socket.fail(errors.New("connection reset"))
		r.clock.awaitWait(t, slackengine.ReconnectFloor)
		r.clock.Advance(slackengine.ReconnectFloor)
		awaitClosed(t, dialedRefusedAgain)
		r.awaitNotices(2)
	})

	t.Run("ADisconnectThatDisablesTheLinkMailsANotice", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))
		r.awaitResyncHints()

		socket.send(slackengine.Envelope{Type: "disconnect", Reason: "link_disabled"})
		notices := r.awaitNotices(1)
		if !strings.Contains(notices[0], "lym slack login") {
			t.Errorf("notice = %q; want it to name lym slack login", notices[0])
		}
	})

	t.Run("APongStampsTheLastHealthyTime", func(t *testing.T) {
		t.Parallel()
		r := newHolderRig(t)
		dialer, socket := &fakeDialer{}, newFakeSocket(t)
		dialer.hand(socket)
		r.start(r.holder(r.a, dialer, nil))

		r.clock.awaitWait(t, pingInterval)
		r.clock.Advance(pingInterval)
		socket.awaitPing()
		eventually(t, "the pong to stamp the last-healthy time", func() bool {
			relayState, found, err := slackengine.ReadRelayState(r.a)
			return err == nil && found && relayState.LastSlackOK.Equal(r.clock.Now())
		})
	})
}
