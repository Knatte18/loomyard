// holder.go holds the machine's one Socket Mode connection.
// It turns each Slack event into a wake hint in the inbox of the hub the event belongs to.
// The holder takes the machine's socket lock, so exactly one relay of the machine dials.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// resyncHint is the name of the hint that asks a hub's relay to read every channel of its map.
	resyncHint = "resync"
	// pingInterval is how often the holder pings the connection.
	pingInterval = 30 * time.Second
	// pongTimeout is how long a ping may wait for its pong before the connection counts as dead.
	pongTimeout = 10 * time.Second
	// envelopeLimit is how long routing one event may take before the event is acknowledged and its hub owed a resync.
	envelopeLimit = 10 * time.Second
	// reconnectFloor is the shortest wait before the holder dials again.
	reconnectFloor = time.Second
	// stableConnection is how long a connection must serve to reset the reconnect wait and end a notice episode.
	stableConnection = time.Minute
	// hintTempPattern is the name pattern of the temp file a hint is written through.
	hintTempPattern = ".hint-*"
	// linkDisabledReason is the reason of a `disconnect` that says the app's Socket Mode link was turned off.
	linkDisabledReason = "link_disabled"
	// disconnectType and eventsType are the types of the envelopes the holder acts on.
	disconnectType = "disconnect"
	eventsType     = "events_api"
	// dialNoticeKey and liveHubsNoticeKey name the notice episodes of a refused connection and an unreadable live-hub list.
	dialNoticeKey     = "dial"
	liveHubsNoticeKey = "live-hubs"
)

// errPongTimeout is why a connection ends when a ping gets no pong in time.
var errPongTimeout = errors.New("slack: no pong arrived within the pong timeout")

// socketHolder holds the machine's one Socket Mode connection on behalf of the relay that took the machine's socket lock.
// It is safe for concurrent use by its own goroutines only.
type socketHolder struct {
	geom     Geometry
	dial     Dialer
	liveHubs LiveHubs
	mailbox  Mailbox
	slack    SlackClient
	clock    Clock
	// write writes one hint into a hub's inbox; tests replace it.
	write func(geom Geometry, name string) error
	// ctx is the run's context, which the goroutines off the serving loop use.
	ctx      context.Context
	wg       sync.WaitGroup
	episodes logger.EpisodeSet

	mu sync.Mutex
	// owed holds the hubs owed a resync hint, keyed by hub path.
	owed map[string]Geometry
	// lastLive is the live-hub list of the last read that stood, valid while liveKnown.
	lastLive  []Geometry
	liveKnown bool
	// liveFailing is set while the live-hub read fails.
	liveFailing bool
	// routed holds, per envelope id, the hub a route in flight found.
	routed map[string]Geometry
	// noticed holds the keys of the notice episodes that have mailed their notice.
	noticed map[string]bool
	// upkeepRunning, upkeepWanted and resyncWanted schedule the one goroutine that reads the live hubs and pays owed hints.
	upkeepRunning bool
	upkeepWanted  bool
	resyncWanted  bool
}

// newSocketHolder returns a holder over the seams.
func newSocketHolder(geom Geometry, dial Dialer, liveHubs LiveHubs, mailbox Mailbox, slack SlackClient, clock Clock) *socketHolder {
	return &socketHolder{
		geom:     geom,
		dial:     dial,
		liveHubs: liveHubs,
		mailbox:  mailbox,
		slack:    slack,
		clock:    clock,
		write:    writeHint,
		owed:     map[string]Geometry{},
		routed:   map[string]Geometry{},
		noticed:  map[string]bool{},
	}
}

// run holds the machine's socket lock and serves Socket Mode connections until ctx ends.
// The lock wait blocks without waking until the holder of the lock ends.
// A ctx that ends during the wait returns at once, and the lock is released as soon as it is taken.
// A connection that fails is dialed again after a wait that starts at one second and doubles to waitCeiling;
// a connection that served at least stableConnection resets the wait.
// A refused app, or a disconnect that says the link is disabled, mails the hub's orch one notice per episode.
func (h *socketHolder) run(ctx context.Context) {
	h.ctx = ctx
	defer h.wg.Wait()
	held, ok := h.takeSocketLock(ctx)
	if !ok {
		return
	}
	defer releaseSocketLock(h.geom.SocketLockPath, held)

	wait := reconnectFloor
	for ctx.Err() == nil {
		socket, _, err := h.dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			h.dialFailed(ctx, err)
			if !waitFor(ctx, h.clock, wait) {
				return
			}
			wait = min(wait*2, waitCeiling)
			continue
		}
		h.episodes.Recover("dial", "slackengine: the Socket Mode dial succeeds again")
		started := h.clock.Now()
		serveErr := h.serve(ctx, socket)
		if closeErr := socket.Close(); closeErr != nil {
			logger.Debug("slackengine: could not close the Socket Mode connection", "error", closeErr)
		}
		if ctx.Err() != nil {
			return
		}
		h.episodes.Fail("connection", "slackengine: the Socket Mode connection ended", "error", serveErr)
		if h.clock.Now().Sub(started) >= stableConnection {
			wait = reconnectFloor
			h.endNotice(dialNoticeKey)
			h.episodes.Recover("connection", "slackengine: the Socket Mode connection served a stable minute")
		}
		if !waitFor(ctx, h.clock, wait) {
			return
		}
		wait = min(wait*2, waitCeiling)
	}
}

// takeSocketLock takes the machine's socket lock and reports whether the holder holds it.
func (h *socketHolder) takeSocketLock(ctx context.Context) (*lock.FileLock, bool) {
	path := h.geom.SocketLockPath
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		logger.Warn("slackengine: could not create the directory of the socket lock", "path", path, "error", err)
		return nil, false
	}
	type acquisition struct {
		held *lock.FileLock
		err  error
	}
	acquired := make(chan acquisition, 1)
	go func() {
		held, err := lock.AcquireWriteLock(path)
		acquired <- acquisition{held: held, err: err}
	}()
	select {
	case result := <-acquired:
		if result.err != nil {
			logger.Warn("slackengine: could not take the socket lock", "path", path, "error", result.err)
			return nil, false
		}
		return result.held, true
	case <-ctx.Done():
		go func() {
			if result := <-acquired; result.err == nil {
				releaseSocketLock(path, result.held)
			}
		}()
		return nil, false
	}
}

// releaseSocketLock gives up the socket lock.
func releaseSocketLock(path string, held *lock.FileLock) {
	if err := held.Release(); err != nil {
		logger.Warn("slackengine: could not release the socket lock", "path", path, "error", err)
	}
}

// dialFailed logs a failed dial and mails the notice a refused app calls for.
func (h *socketHolder) dialFailed(ctx context.Context, err error) {
	h.episodes.Fail("dial", "slackengine: the Socket Mode dial failed", "error", err)
	if refusal, refused := isAPIError(err); refused {
		h.notice(ctx, dialNoticeKey, fmt.Sprintf(
			"Slack refused the Socket Mode connection with %s; way forward: run `lym slack login` from the operator's terminal to give both tokens again",
			refusal.Code))
	}
}

// serve serves one connection until it fails, a disconnect arrives or ctx ends, and returns why.
// One goroutine reads the envelopes; the loop waits on them, on the ping interval and on ctx.
// A new connection owes every live hub a resync hint, which the upkeep goroutine writes.
// Each pong stamps the relay's last-healthy time, and each ping starts the upkeep goroutine again.
func (h *socketHolder) serve(ctx context.Context, socket Socket) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	envelopes := make(chan Envelope)
	readFailed := make(chan error, 1)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			envelope, err := socket.Read(connCtx)
			if err != nil {
				readFailed <- err
				return
			}
			select {
			case envelopes <- envelope:
			case <-connCtx.Done():
				return
			}
		}
	}()

	h.requestUpkeep(true)
	pingTimer := h.clock.After(pingInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readFailed:
			return fmt.Errorf("slack: read the Socket Mode connection: %w", err)
		case envelope := <-envelopes:
			switch envelope.Type {
			case disconnectType:
				if envelope.Reason == linkDisabledReason {
					h.notice(ctx, dialNoticeKey,
						"Slack disabled the app's Socket Mode link; way forward: run `lym slack login` from the operator's terminal once the app is enabled")
				}
				return fmt.Errorf("slack: Slack closed the Socket Mode connection: %s", envelope.Reason)
			case eventsType:
				h.wg.Add(1)
				go func() {
					defer h.wg.Done()
					h.handleEnvelope(connCtx, socket, envelope)
				}()
			}
		case <-pingTimer:
			if err := h.ping(connCtx, socket); err != nil {
				return err
			}
			h.requestUpkeep(false)
			pingTimer = h.clock.After(pingInterval)
		}
	}
}

// ping pings the connection and races the pong against the clock's pong timeout.
// A pong stamps the relay's last-healthy time.
func (h *socketHolder) ping(ctx context.Context, socket Socket) error {
	answered := make(chan error, 1)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		answered <- socket.Ping(ctx)
	}()
	select {
	case err := <-answered:
		if err != nil {
			return err
		}
	case <-h.clock.After(pongTimeout):
		return errPongTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
	now := h.clock.Now()
	if err := updateRelayState(h.geom, func(relayState *RelayState) { relayState.LastSlackOK = now }); err != nil {
		h.episodes.Fail("relay-state", "slackengine: could not stamp the last-healthy time", "error", err)
		return nil
	}
	h.episodes.Recover("relay-state", "slackengine: the last-healthy time is stamped again")
	return nil
}

// handleEnvelope routes one event off the serving loop and acknowledges it.
// The route runs in a goroutine of its own.
// When it has not answered within the clock's envelopeLimit, the event is acknowledged.
// The hub the route found, or every hub of the last live-hub read, is then owed a resync hint.
// A late answer of the stalled route is dropped.
// A route that answers false leaves the event unacknowledged, so Slack sends it again.
func (h *socketHolder) handleEnvelope(ctx context.Context, socket Socket, envelope Envelope) {
	answered := make(chan bool, 1)
	go func() { answered <- h.route(envelope) }()
	acknowledge := false
	select {
	case acknowledge = <-answered:
	case <-h.clock.After(envelopeLimit):
		h.oweStalled(envelope.EnvelopeID)
		acknowledge = true
	case <-ctx.Done():
		return
	}
	h.forgetRoute(envelope.EnvelopeID)
	if !acknowledge || envelope.EnvelopeID == "" {
		return
	}
	if err := socket.Ack(ctx, envelope.EnvelopeID); err != nil && ctx.Err() == nil {
		logger.Debug("slackengine: could not acknowledge a Socket Mode envelope", "error", err)
	}
}

// route writes the hint of an `events_api` envelope into the inbox of a hub.
// That hub is the one whose channel map holds the event's channel.
// It reports whether the event may be acknowledged.
// A failed hint write acknowledges the event and owes the hub a resync hint.
// An event no map holds is acknowledged and dropped while every live map reads;
// while any live map cannot be read, or the live-hub list fails, it is left unacknowledged for Slack's retry.
func (h *socketHolder) route(envelope Envelope) bool {
	live, ok := h.readLive(h.ctx)
	if !ok {
		return false
	}
	unreadable := false
	for _, geom := range live {
		channelMap, err := ReadChannelMap(geom.ChannelMapPath)
		if err != nil {
			unreadable = true
			logger.Debug("slackengine: could not read a hub's channel map to route an event", "path", geom.ChannelMapPath, "error", err)
			continue
		}
		if _, held := channelMap.OrchOf(envelope.Channel); !held {
			continue
		}
		h.mu.Lock()
		h.routed[envelope.EnvelopeID] = geom
		h.mu.Unlock()
		h.hint(geom, hintName(envelope))
		return true
	}
	if unreadable {
		return false
	}
	logger.Debug("slackengine: dropped an event no hub's channel map holds", "channel", envelope.Channel)
	return true
}

// forgetRoute drops the hub a finished route recorded for the envelope id.
func (h *socketHolder) forgetRoute(envelopeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.routed, envelopeID)
}

// oweStalled owes a resync hint to the hub the stalled route found.
// With none found, it owes the hint to every hub of the last live-hub read.
func (h *socketHolder) oweStalled(envelopeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if geom, found := h.routed[envelopeID]; found {
		h.owed[geom.HubPath] = geom
		return
	}
	for _, geom := range h.lastLive {
		h.owed[geom.HubPath] = geom
	}
}

// hint writes the named hint into the hub's inbox.
// A failed write mails the holder hub's orch one notice per episode and owes the hub a resync hint.
func (h *socketHolder) hint(geom Geometry, name string) {
	if err := h.write(geom, name); err != nil {
		h.hintFailed(geom, err)
		h.mu.Lock()
		h.owed[geom.HubPath] = geom
		h.mu.Unlock()
		return
	}
	h.endNotice(hintNoticeKey(geom))
}

// hintFailed mails the notice for a hint the hub's inbox refused.
func (h *socketHolder) hintFailed(geom Geometry, err error) {
	h.notice(h.ctx, hintNoticeKey(geom), fmt.Sprintf(
		"could not write a wake hint into the inbox %s: %v; the hub's relay reads its channels late until a write stands",
		geom.InboxDir, err))
}

// hintNoticeKey names the notice episode of a hub's failing hint writes.
func hintNoticeKey(geom Geometry) string {
	return "hint:" + geom.HubPath
}

// readLive reads the live-hub list and reports whether the read stood.
// A failed read mails the holder hub's orch one notice per episode carrying the seam's error as it is.
// A read that stands ends the episode and owes every hub it names a resync hint;
// so does each hub a read names that the previous one did not.
func (h *socketHolder) readLive(ctx context.Context) ([]Geometry, bool) {
	live, err := h.liveHubs()
	if err != nil {
		h.mu.Lock()
		h.liveFailing = true
		h.mu.Unlock()
		h.episodes.Fail(liveHubsNoticeKey, "slackengine: could not read the live hubs", "error", err)
		h.notice(ctx, liveHubsNoticeKey, fmt.Sprintf("could not read the machine's live hubs, so Slack events wait unacknowledged: %v", err))
		return nil, false
	}
	h.episodes.Recover(liveHubsNoticeKey, "slackengine: the live hubs read again")
	h.endNotice(liveHubsNoticeKey)

	h.mu.Lock()
	defer h.mu.Unlock()
	previous := map[string]bool{}
	for _, geom := range h.lastLive {
		previous[geom.HubPath] = true
	}
	listed := map[string]bool{}
	for _, geom := range live {
		listed[geom.HubPath] = true
		if h.liveFailing || (h.liveKnown && !previous[geom.HubPath]) {
			h.owed[geom.HubPath] = geom
		}
	}
	for hubPath := range h.owed {
		if !listed[hubPath] {
			delete(h.owed, hubPath)
		}
	}
	h.lastLive, h.liveKnown, h.liveFailing = live, true, false
	return live, true
}

// requestUpkeep asks for one run of the upkeep goroutine, which reads the live hubs and pays owed hints.
// A request made while the goroutine runs makes it run again, so none is lost.
// resync owes every live hub a resync hint, as a new connection does.
func (h *socketHolder) requestUpkeep(resync bool) {
	h.mu.Lock()
	h.upkeepWanted = true
	h.resyncWanted = h.resyncWanted || resync
	if h.upkeepRunning {
		h.mu.Unlock()
		return
	}
	h.upkeepRunning = true
	h.mu.Unlock()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for {
			h.mu.Lock()
			if !h.upkeepWanted || h.ctx.Err() != nil {
				h.upkeepWanted, h.upkeepRunning = false, false
				h.mu.Unlock()
				return
			}
			resyncAll := h.resyncWanted
			h.upkeepWanted, h.resyncWanted = false, false
			h.mu.Unlock()
			h.upkeep(resyncAll)
		}
	}()
}

// upkeep reads the live-hub list again and pays every owed resync hint.
func (h *socketHolder) upkeep(resyncAll bool) {
	live, ok := h.readLive(h.ctx)
	if ok && resyncAll {
		h.mu.Lock()
		for _, geom := range live {
			h.owed[geom.HubPath] = geom
		}
		h.mu.Unlock()
	}
	h.payOwed()
}

// payOwed writes a resync hint into the inbox of each hub owed one, and stops owing a hub once its write stands.
// A write that fails stays owed for the next ping.
func (h *socketHolder) payOwed() {
	h.mu.Lock()
	targets := make([]Geometry, 0, len(h.owed))
	for _, geom := range h.owed {
		targets = append(targets, geom)
	}
	h.mu.Unlock()
	for _, geom := range targets {
		if h.ctx.Err() != nil {
			return
		}
		if err := h.write(geom, resyncHint); err != nil {
			h.hintFailed(geom, err)
			continue
		}
		h.endNotice(hintNoticeKey(geom))
		h.mu.Lock()
		delete(h.owed, geom.HubPath)
		h.mu.Unlock()
	}
}

// notice mails the holder hub's orch one `[slack relay]` notice with line for the episode key.
// The notice is first posted to the hub's prime channel when the hub's map reads and holds it.
// Until endNotice ends the episode, a later notice of the same key does nothing.
func (h *socketHolder) notice(ctx context.Context, key, line string) {
	h.mu.Lock()
	if h.noticed[key] {
		h.mu.Unlock()
		return
	}
	h.noticed[key] = true
	h.mu.Unlock()

	receiver, err := orchName(h.geom.Shortname, "")
	if err != nil {
		logger.Warn("slackengine: could not name the orch to notify", "error", err)
		h.endNotice(key)
		return
	}
	h.postToPrimeChannel(ctx, receiver, line)
	if _, err := h.mailbox.Post(receiver, relayNoticePrefix+" "+line); err != nil {
		logger.Warn("slackengine: the relay notice was not posted", "receiver", receiver, "error", err)
		h.endNotice(key)
	}
}

// postToPrimeChannel posts line to the hub's prime channel.
// It posts only when the hub's channel map reads and holds an open entry for that channel.
// A post Slack refuses is logged and nothing more.
func (h *socketHolder) postToPrimeChannel(ctx context.Context, primeOrch, line string) {
	channelMap, err := ReadChannelMap(h.geom.ChannelMapPath)
	if err != nil {
		return
	}
	entry, held := channelMap.Channels[primeOrch]
	if !held || entry.Archived {
		return
	}
	if err := h.slack.PostAs(ctx, entry.ID, slackEscape.Replace(relayNoticePrefix+" "+line), Sender{}); err != nil {
		logger.Debug("slackengine: could not post a relay notice to the prime channel", "error", err)
	}
}

// endNotice ends the episode of the notice key.
func (h *socketHolder) endNotice(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.noticed, key)
}

// writeHint creates an empty file called name in the hub's inbox, creating the inbox when it is missing.
// The file is made in the hub's Slack directory, outside the inbox, and renamed into it,
// so a hint appears whole or not at all and a reader never sees a half-made one.
func writeHint(geom Geometry, name string) error {
	if err := os.MkdirAll(geom.InboxDir, privateDirMode); err != nil {
		return fmt.Errorf("slack: create the inbox %s: %w", geom.InboxDir, err)
	}
	temp, err := os.CreateTemp(geom.Dir, hintTempPattern)
	if err != nil {
		return fmt.Errorf("slack: create a hint file in %s: %w", geom.Dir, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temp.Name())
		return fmt.Errorf("slack: close a hint file in %s: %w", geom.Dir, err)
	}
	if err := os.Rename(temp.Name(), filepath.Join(geom.InboxDir, name)); err != nil {
		_ = os.Remove(temp.Name())
		return fmt.Errorf("slack: move the hint %s into the inbox %s: %w", name, geom.InboxDir, err)
	}
	return nil
}

// hintName returns the name of the hint for an event.
// The name is the event's channel.
// For a thread reply that Slack did not broadcast, it is the channel and the thread's timestamp joined by an underscore.
func hintName(envelope Envelope) string {
	isUnbroadcastReply := envelope.ThreadTS != "" && envelope.ThreadTS != envelope.TS && envelope.Subtype != "thread_broadcast"
	if isUnbroadcastReply {
		return envelope.Channel + "_" + envelope.ThreadTS
	}
	return envelope.Channel
}
