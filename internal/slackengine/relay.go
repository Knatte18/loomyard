// relay.go runs the relay process.
// It waits to be reparented, takes the relay lock and makes the first creel read.
// It then runs the socket holder, the outbound loop and the wake loop.
// It also re-execs onto a replaced binary and writes the relay log.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// wakeFloor is the shortest time between the starts of two wakes of the wake loop.
const wakeFloor = time.Second

// removeEventOp is the operation of a file event that wakes nothing: the relay's own removal of a hint.
const removeEventOp = "remove"

// ErrRelayInStrandPane reports that creel refused the relay's program claim.
// The relay runs in a strand's pane that is no orch's; the relay releases its lock before it returns this error.
var ErrRelayInStrandPane = errors.New("slack: the relay runs in a strand pane that is no orch's")

// errFirstReadAccepted is what the first read's handler returns to accept the read.
// The mail the read saw stays unread for the outbound read.
var errFirstReadAccepted = errors.New("slack: the first read was accepted and its mail left for the outbound read")

// RelayDeps holds the seams a relay process runs against.
// They are its creel mailbox, Slack client, Socket Mode dial, hub, clock, file watch, and process operations.
type RelayDeps struct {
	// Mailbox is the relay's creel mailbox.
	Mailbox Mailbox
	// Slack is the Slack client the relay's loops call.
	Slack SlackClient
	// Dial opens a Socket Mode connection for the machine's socket holder.
	Dial Dialer
	// Hub reads the hub's beams and pauses.
	Hub Hub
	// LiveHubs lists the machine's live hubs, which the relay's channel keeper reads to judge a channel.
	LiveHubs LiveHubs
	// Clock is the relay's source of time.
	Clock Clock
	// Watch opens a file watch on dir for the entries called names, or for every entry when names is empty.
	// A nil Watch means fswatch.Watch.
	Watch func(dir string, names ...string) (*fswatch.Watcher, error)
	// PID returns the relay's process id.
	PID func() int
	// ReexecLockWait is how long a re-exec waits for the lock its old image released; zero waits only lock.ProbeGrace.
	ReexecLockWait time.Duration
	// CheckBinary re-execs onto a replaced lym binary and returns when the binary is unchanged.
	// It calls beforeExec before each exec and failed with the error of each exec that fails.
	CheckBinary func(beforeExec func(), failed func(error))
	// ReapInherited waits in the background on the children the replaced image started.
	ReapInherited func() error
	// Register records the relay process in the hub's run-process owner records.
	Register func() error
}

// relayLock is the lock a live relay holds, which a re-exec gives up and takes back.
type relayLock struct {
	path string
	held *lock.FileLock
}

// release gives up the lock; releasing a lock not held does nothing.
func (l *relayLock) release() {
	if l.held == nil {
		return
	}
	if err := l.held.Release(); err != nil {
		logger.Warn("slackengine: could not release the relay lock", "path", l.path, "error", err)
	}
	l.held = nil
}

// retake takes the lock again, waiting lock.ProbeGrace for a probe that holds it for a moment.
// It reports whether it took the lock.
func (l *relayLock) retake() bool {
	held, acquired, err := lock.AcquireWriteLockWithin(l.path, lock.ProbeGrace)
	if err != nil || !acquired {
		return false
	}
	l.held = held
	return true
}

// relayLog writes the relay's failures to its log file, and to the process log once per episode.
type relayLog struct {
	geom     Geometry
	clock    Clock
	episodes logger.EpisodeSet
}

// fail appends a line for err under key to the relay log and counts it in key's episode.
func (l *relayLog) fail(key string, err error) {
	l.episodes.Fail(key, "slackengine: the relay hit a failure", "kind", key, "error", err)
	l.appendLine(fmt.Sprintf("%s %s: %v", l.clock.Now().UTC().Format(time.RFC3339), key, err))
}

// recovered ends key's episode.
func (l *relayLog) recovered(key string) {
	l.episodes.Recover(key, "slackengine: the relay recovered", "kind", key)
}

// appendLine appends line to the relay log; a log that cannot be written is logged at Debug.
func (l *relayLog) appendLine(line string) {
	if err := os.MkdirAll(filepath.Dir(l.geom.LogPath), privateDirMode); err != nil {
		logger.Debug("slackengine: could not create the relay log directory", "path", l.geom.LogPath, "error", err)
		return
	}
	file, err := os.OpenFile(l.geom.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		logger.Debug("slackengine: could not open the relay log", "path", l.geom.LogPath, "error", err)
		return
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, line); err != nil {
		logger.Debug("slackengine: could not write the relay log", "path", l.geom.LogPath, "error", err)
	}
}

// updateRelayState applies mutate to the relay state under the record's lock,
// so the relay's writes and the starter's keep each other's fields.
func updateRelayState(geom Geometry, mutate func(*RelayState)) error {
	err := state.UpdateJSON(geom.RelayStatePath, recordLockPath(geom.RelayStatePath), func(current RelayState, _ bool) (RelayState, error) {
		mutate(&current)
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: write the relay state %s: %w", geom.RelayStatePath, err)
	}
	return nil
}

// waitFor waits d on clock and reports false when ctx ended first.
func waitFor(ctx context.Context, clock Clock, d time.Duration) bool {
	select {
	case <-clock.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// RunRelay runs the hub's relay until ctx ends, and returns nil when it ends or another relay holds the lock.
// It takes the relay lock, registers itself, records its pid, and makes the first creel read before any Slack call.
// It reads the machine's presence before that read.
// So a presence a start sets once the read is accepted is a change it notices.
// Only then does it refuse a machine missing either token and a hub with no setup file, through missingTokenExit.
// It ensures the hub's channels and owes the start read of each, then runs three loops:
// the machine's socket holder, which turns Slack events into hints in the hub's inbox;
// the outbound read of the relay's mailbox; and the wake loop, which reads the channels on a hint or a presence change.
// At the end of each outbound read it stamps the relay's last-healthy time in its state,
// and the wake loop then does the retried work and checks whether the lym binary was replaced.
// A creel refusal of the program claim from a strand's pane returns ErrRelayInStrandPane with the lock released.
// Any other first-read refusal is retried while the lock is held.
// A relay that cannot re-exec retakes its lock and its socket holder, or exits when another relay holds the lock.
func RunRelay(ctx context.Context, geom Geometry, deps RelayDeps) error {
	if err := deps.ReapInherited(); err != nil {
		logger.Warn("slackengine: could not wait on the children the replaced image started", "error", err)
	}
	heldLock, err := takeRelayLock(geom, deps)
	if err != nil {
		return err
	}
	if heldLock == nil {
		logger.Info("slackengine: another relay holds the lock; this one exits", "lock", geom.LockPath)
		return nil
	}
	defer heldLock.release()

	if err := deps.Register(); err != nil {
		return fmt.Errorf("slack: register the relay: %w", err)
	}
	pid := deps.PID()
	startTime, _ := proc.StartTime(pid)
	if err := updateRelayState(geom, func(relayState *RelayState) {
		relayState.PID, relayState.StartTime = pid, startTime
		relayState.FirstReadAt, relayState.FirstReadRefusal = time.Time{}, ""
	}); err != nil {
		return err
	}

	log := &relayLog{geom: geom, clock: deps.Clock}
	floor := newSharedFloor()
	startPresence, err := ReadPresence(geom.PresencePath)
	if err != nil {
		log.fail("presence", err)
	}
	if err := firstRead(ctx, geom, deps, heldLock, log, floor); err != nil {
		return err
	}
	if _, _, err := RequireConfigured(geom); err != nil {
		return missingTokenExit(ctx, geom, deps, err)
	}

	loopCtx, stopLoops := context.WithCancel(ctx)
	defer stopLoops()
	keeper := newChannelKeeper(geom, deps.Slack, deps.LiveHubs, deps.Mailbox, deps.Clock)
	reader := newInbound(geom, deps.Mailbox, deps.Slack, deps.Hub, deps.Clock, keeper)
	primePending := relayStart(loopCtx, geom, keeper, reader, deps.Hub)

	holder := &holderRun{geom: geom, deps: deps}
	holder.start(loopCtx)
	defer holder.stop()

	watchOpen := deps.Watch
	if watchOpen == nil {
		watchOpen = fswatch.Watch
	}
	wake := &wakeLoop{
		geom: geom, hub: deps.Hub, mailbox: deps.Mailbox, clock: deps.Clock, keeper: keeper, in: reader, log: log, floor: floor,
		watchOpen: watchOpen, checkBinary: deps.CheckBinary, relayLock: heldLock, holder: holder,
		outbound:     make(chan chan struct{}),
		primePending: primePending,
		noticed:      startPresence,
	}
	stampLastHealthy := func() {
		now := deps.Clock.Now()
		if err := updateRelayState(geom, func(relayState *RelayState) { relayState.LastSlackOK = now }); err != nil {
			log.fail("relay-state", err)
			return
		}
		log.recovered("relay-state")
	}
	afterRead := func(readCtx context.Context) {
		if readCtx.Err() != nil {
			return
		}
		stampLastHealthy()
		wake.askOutboundWake(readCtx)
	}
	go runOutbound(loopCtx, newOutbound(geom, deps.Mailbox, deps.Slack, deps.Clock, floor, keeper, deps.Hub), deps.Clock, log, afterRead)
	return wake.run(loopCtx)
}

// takeRelayLock takes the relay lock, and returns nil when another relay holds it.
// A relay whose recorded pid is its own is a re-exec.
// A re-exec waits up to deps.ReexecLockWait for the lock its old image released.
// Any other start waits only lock.ProbeGrace,
// so a liveness probe that holds the lock for a moment never makes a starting relay exit.
func takeRelayLock(geom Geometry, deps RelayDeps) (*relayLock, error) {
	previous, found, err := ReadRelayState(geom)
	if err != nil {
		return nil, err
	}
	wait := lock.ProbeGrace
	if found && previous.PID == deps.PID() {
		wait = max(wait, deps.ReexecLockWait)
	}
	if err := os.MkdirAll(filepath.Dir(geom.LockPath), privateDirMode); err != nil {
		return nil, fmt.Errorf("slack: create the directory of the relay lock %s: %w", geom.LockPath, err)
	}
	held, acquired, err := lock.AcquireWriteLockWithin(geom.LockPath, wait)
	if err != nil {
		return nil, fmt.Errorf("slack: take the relay lock %s: %w", geom.LockPath, err)
	}
	if !acquired {
		return nil, nil
	}
	return &relayLock{path: geom.LockPath, held: held}, nil
}

// firstRead makes the relay's first creel read as the program, before any Slack call.
// It records the outcome in the relay state.
// The read is accepted with or without mail, the mail staying unread.
// A refusal of a program claim from a strand's pane returns ErrRelayInStrandPane with the lock released.
// Any other refusal is retried with the poll's backoff while the lock is held.
// A stop of ctx ends the wait and returns nil.
func firstRead(ctx context.Context, geom Geometry, deps RelayDeps, heldLock *relayLock, log *relayLog, floor *sharedFloor) error {
	retryWait := newLoopWait(floor)
	for {
		err := deps.Mailbox.ReadAll(func([]creelengine.Message) error { return errFirstReadAccepted })
		if err == nil || errors.Is(err, errFirstReadAccepted) {
			now := deps.Clock.Now()
			log.recovered("first-read")
			return updateRelayState(geom, func(relayState *RelayState) {
				relayState.FirstReadAt, relayState.FirstReadRefusal = now, ""
			})
		}
		refusal := err.Error()
		if writeErr := updateRelayState(geom, func(relayState *RelayState) { relayState.FirstReadRefusal = refusal }); writeErr != nil {
			return writeErr
		}
		log.fail("first-read", err)
		if errors.Is(err, creelengine.ErrProgramClaimInStrandPane) {
			heldLock.release()
			return fmt.Errorf("%w: %w; way forward: start the relay with `lym slack start` from the operator's terminal or an orch's pane", ErrRelayInStrandPane, err)
		}
		if !waitFor(ctx, deps.Clock, retryWait.afterIdle()) {
			return nil
		}
	}
}

// missingTokenExit tells the operator why the relay cannot run, and returns the refusal so the relay exits with it.
// The hub's orch is mailed a `[slack relay]` notice with the refusal and the `lym slack start` to run after the fix.
// Neither the login nor a dobby tick while presence is `terminal` starts a relay that has exited.
// The same line is first posted to the hub's prime channel.
// That needs a bot token on the machine and the channel in the channel map.
// With no prime entry it is posted to the channel id an older binary's setup file holds, the only read of that key.
func missingTokenExit(ctx context.Context, geom Geometry, deps RelayDeps, refusal error) error {
	line := fmt.Sprintf("%v, then `lym slack start`", refusal)
	credentials, credentialsErr := ReadCredentials(geom.TokenPath)
	if credentialsErr == nil && credentials.BotToken != "" {
		if channel, found := missingTokenChannel(geom); found {
			err := deps.Slack.PostAs(ctx, channel, slackEscape.Replace(relayNoticePrefix+" "+line), Sender{})
			if err != nil {
				logger.Debug("slackengine: could not post the refusal to the hub's channel", "channel", channel, "error", err)
			}
		}
	}
	receiver, err := orchName(geom.Shortname, "")
	if err != nil {
		logger.Warn("slackengine: could not name the orch to notify of the refusal", "error", err)
		return refusal
	}
	if _, err := deps.Mailbox.Post(receiver, relayNoticePrefix+" "+line); err != nil {
		logger.Warn("slackengine: the relay notice for the refusal was not posted", "receiver", receiver, "error", err)
	}
	return refusal
}

// missingTokenChannel returns the channel a refused relay posts its line to and reports whether there is one.
// It is the prime's open channel in the hub's map, otherwise the channel id of an older binary's setup file.
func missingTokenChannel(geom Geometry) (string, bool) {
	channelMap, err := ReadChannelMap(geom.ChannelMapPath)
	if err != nil {
		logger.Debug("slackengine: the refusal's post needs the channel map", "error", err)
		return "", false
	}
	prime, err := orchName(geom.Shortname, "")
	if err != nil {
		return "", false
	}
	if entry, held := channelMap.Channels[prime]; held {
		return entry.ID, !entry.Archived
	}
	setup, found, err := ReadSetup(geom.SetupPath)
	if err != nil || !found || setup.ChannelID == "" {
		return "", false
	}
	return setup.ChannelID, true
}

// relayStart ensures the hub's prime channel.
// When presence is slack it also ensures every listed beam's channel.
// It then owes every open channel its start read and makes the reads.
// It reports whether the prime's channel is still to be ensured, which the outbound wakes retry.
func relayStart(ctx context.Context, geom Geometry, keeper *channelKeeper, in *inbound, hub Hub) bool {
	prime, err := orchName(geom.Shortname, "")
	if err != nil {
		logger.Warn("slackengine: could not name the hub's orch at the relay's start", "error", err)
		return false
	}
	_, primeErr := keeper.ensure(ctx, prime)
	if primeErr != nil {
		logger.Warn("slackengine: could not ensure the hub's prime channel at the start; the outbound wakes retry", "error", primeErr)
	}
	if presence, err := ReadPresence(geom.PresencePath); err == nil && presence == PresenceSlack {
		beams, err := hub.Beams()
		if err == nil {
			err = ensureBeamChannels(ctx, geom, keeper, beams)
		}
		if err != nil {
			logger.Warn("slackengine: could not ensure the beams' channels at the start", "error", err)
		}
	}
	in.start()
	in.retryOwed(ctx)
	return primeErr != nil
}

// ensureBeamChannels ensures the channel of each listed beam and returns the joined errors of the ones that failed.
func ensureBeamChannels(ctx context.Context, geom Geometry, keeper *channelKeeper, beams []string) error {
	var failures []error
	for _, slug := range beams {
		orch, err := orchName(geom.Shortname, slug)
		if err == nil {
			_, err = keeper.ensure(ctx, orch)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// holderRun runs the machine's socket holder in a goroutine of its own, which a re-exec stops and starts again.
// It is used from one goroutine at a time.
type holderRun struct {
	geom   Geometry
	deps   RelayDeps
	cancel context.CancelFunc
	done   chan struct{}
}

// start runs a new socket holder under a context of its own, derived from ctx.
func (r *holderRun) start(ctx context.Context) {
	socket := newSocketHolder(r.geom, r.deps.Dial, r.deps.LiveHubs, r.deps.Mailbox, r.deps.Slack, r.deps.Clock)
	holderCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.cancel, r.done = cancel, done
	go func() {
		defer close(done)
		socket.run(holderCtx)
	}()
}

// stop ends the running holder and waits until its connection is closed and the machine's socket lock is released.
// Stopping a holder that is not running does nothing.
func (r *holderRun) stop() {
	if r.cancel == nil {
		return
	}
	r.cancel()
	<-r.done
	r.cancel, r.done = nil, nil
}

// runOutbound reads the relay's mailbox until ctx ends, waiting between reads as each read asks.
// It calls afterRead after each read, whatever the read found, before the wait.
func runOutbound(ctx context.Context, out *outbound, clock Clock, log *relayLog, afterRead func(ctx context.Context)) {
	for ctx.Err() == nil {
		wait, err := out.read(ctx)
		if err != nil {
			log.fail("outbound", err)
		} else {
			log.recovered("outbound")
		}
		afterRead(ctx)
		if !waitFor(ctx, clock, wait) {
			return
		}
	}
}

// wakeLoop does the relay's channel work when a hint or a presence change arrives, and once per outbound read.
// One goroutine runs it, so one goroutine reads the channels and holds the inbound's owed reads and episodes.
type wakeLoop struct {
	geom      Geometry
	hub       Hub
	mailbox   Mailbox
	clock     Clock
	keeper    *channelKeeper
	in        *inbound
	log       *relayLog
	floor     *sharedFloor
	watchOpen func(dir string, names ...string) (*fswatch.Watcher, error)
	// checkBinary, relayLock and holder carry the re-exec: a replaced binary stops the holder and frees the relay lock.
	checkBinary func(beforeExec func(), failed func(error))
	relayLock   *relayLock
	holder      *holderRun
	// outbound carries an outbound read's request for a wake; the loop answers by closing the channel it was sent.
	outbound chan chan struct{}

	// The fields below belong to the loop's own goroutine.
	// presence is the presence the previous wake read, empty before the first.
	presence Presence
	// noticed is the presence the hub's orchs were last told of, or the one the relay read at its start;
	// empty while neither is known.
	noticed Presence
	// primePending is set while the prime's channel is still to be ensured.
	primePending bool
	// lastWake is the start of the previous wake, valid once woke is set.
	lastWake time.Time
	woke     bool
	// exiting is set when a failed re-exec found the relay lock kept by another relay.
	exiting bool
}

// askOutboundWake asks the loop for an outbound wake and waits until it is done or ctx ends.
func (w *wakeLoop) askOutboundWake(ctx context.Context) {
	done := make(chan struct{})
	select {
	case w.outbound <- done:
	case <-ctx.Done():
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// run waits for wakes until ctx ends or a failed re-exec leaves the relay lock with another relay.
// A file event in the inbox, or on the presence file alone, wakes it.
// A wake never starts within wakeFloor of the previous wake's start:
// an event inside that second waits the rest of it on the clock.
// The inbox is created first, and watched whole.
// When the watch cannot be opened, the loop looks at the inbox and the presence file on disk instead.
// It waits from wakeFloor, doubling to waitCeiling while it finds nothing, and returns to wakeFloor after a wake.
func (w *wakeLoop) run(ctx context.Context) error {
	if err := os.MkdirAll(w.geom.InboxDir, privateDirMode); err != nil {
		return fmt.Errorf("slack: create the inbox %s: %w", w.geom.InboxDir, err)
	}
	var events <-chan struct{}
	var timer <-chan time.Time
	wait := wakeFloor
	signal, closeWatch, err := w.watch()
	if err != nil {
		w.log.fail("watch", err)
		timer = w.clock.After(wait)
	} else {
		events = signal
		defer closeWatch()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case done := <-w.outbound:
			w.wake(ctx, true)
			close(done)
		case <-events:
			if !w.awaitFloor(ctx) {
				return nil
			}
			w.wake(ctx, false)
		case <-timer:
			if w.diskCheck() {
				wait = wakeFloor
				w.wake(ctx, false)
			} else {
				wait = min(wait*2, waitCeiling)
			}
			timer = w.clock.After(wait)
		}
		if w.exiting {
			return nil
		}
	}
}

// awaitFloor waits out what is left of wakeFloor since the previous wake's start, and reports false when ctx ended first.
func (w *wakeLoop) awaitFloor(ctx context.Context) bool {
	if !w.woke {
		return true
	}
	rest := wakeFloor - w.clock.Now().Sub(w.lastWake)
	if rest <= 0 {
		return true
	}
	return waitFor(ctx, w.clock, rest)
}

// watch watches the inbox whole and the machine presence file's directory for that file alone,
// so the relay's records, outbox, map, log and lock wake nothing.
// It returns one channel that holds a signal after any event but a removal, and the function that closes both watches.
func (w *wakeLoop) watch() (<-chan struct{}, func(), error) {
	inbox, err := w.watchOpen(w.geom.InboxDir)
	if err != nil {
		return nil, nil, fmt.Errorf("slack: watch the inbox %s: %w", w.geom.InboxDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(w.geom.PresencePath), privateDirMode); err != nil {
		_ = inbox.Close()
		return nil, nil, fmt.Errorf("slack: create the directory of the presence file %s: %w", w.geom.PresencePath, err)
	}
	presence, err := w.watchOpen(filepath.Dir(w.geom.PresencePath), filepath.Base(w.geom.PresencePath))
	if err != nil {
		_ = inbox.Close()
		return nil, nil, fmt.Errorf("slack: watch the presence file %s: %w", w.geom.PresencePath, err)
	}
	signal := make(chan struct{}, 1)
	var forwarders sync.WaitGroup
	for _, watcher := range []*fswatch.Watcher{inbox, presence} {
		forwarders.Add(1)
		go func() {
			defer forwarders.Done()
			for event := range watcher.Events() {
				if event.Op == removeEventOp {
					continue
				}
				select {
				case signal <- struct{}{}:
				default:
				}
			}
		}()
	}
	closeAll := func() {
		_ = inbox.Close()
		_ = presence.Close()
		forwarders.Wait()
	}
	return signal, closeAll, nil
}

// diskCheck reports whether the inbox holds a hint or the presence differs from the previous wake's or the noticed one.
// It is the watch's stand-in when the watch cannot be opened.
// Before the first wake, and with no presence noticed, there is nothing to differ from.
func (w *wakeLoop) diskCheck() bool {
	if entries, err := os.ReadDir(w.geom.InboxDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				return true
			}
		}
	}
	presence, err := ReadPresence(w.geom.PresencePath)
	if err != nil {
		return false
	}
	return (w.presence != "" && presence != w.presence) || (w.noticed != "" && presence != w.noticed)
}

// wake does the channel work of one wake.
// It drains the inbox and reads the presence and the hub's beams.
// It mails the hub's orchs the presence notice when the presence changed.
// It ensures every listed beam's channel when presence turned to slack.
// It retires the channels of beams that a successful beams read no longer lists.
// A failed beams read retires nothing.
// An outbound wake also makes the owed reads again.
// It invites the setup's operator where an entry records another or none.
// It retries a refused retire and a prime ensure that did not stand.
// It then checks whether the lym binary was replaced.
func (w *wakeLoop) wake(ctx context.Context, fromOutbound bool) {
	w.lastWake, w.woke = w.clock.Now(), true

	if _, err := w.in.drain(ctx); err != nil {
		w.log.fail("inbound", err)
		if _, limited := isRateLimit(err); limited {
			w.floor.rateLimited()
		}
	} else {
		w.log.recovered("inbound")
	}

	beams, beamsErr := w.hub.Beams()
	if beamsErr != nil {
		w.log.fail("beams", beamsErr)
		beams = nil
	} else {
		w.log.recovered("beams")
	}
	presence, presenceErr := ReadPresence(w.geom.PresencePath)
	if presenceErr != nil {
		w.log.fail("presence", presenceErr)
	} else {
		w.log.recovered("presence")
		w.noticePresenceChange(presence, beams)
		w.followPresence(ctx, presence, beams, beamsErr)
	}
	if beamsErr == nil {
		if err := w.keeper.retire(ctx, beams, fromOutbound); err != nil {
			w.log.fail("retire", err)
		} else {
			w.log.recovered("retire")
		}
	}

	if !fromOutbound {
		return
	}
	w.in.retryOwed(ctx)
	if err := w.keeper.inviteOperators(ctx); err != nil {
		w.log.fail("invite", err)
	} else {
		w.log.recovered("invite")
	}
	w.retryPrime(ctx)
	w.runBinaryCheck(ctx)
}

// noticePresenceChange mails the hub's orchs a `[slack presence]` notice when presence differs from the noticed one.
// With no presence noticed yet it takes presence as noticed and mails nothing.
// The notice goes to the hub's orch and to the orch of each beam in beams, which is empty when the beams read failed.
// A failed notice is logged and not sent again.
func (w *wakeLoop) noticePresenceChange(presence Presence, beams []string) {
	if presence == w.noticed {
		return
	}
	known := w.noticed != ""
	w.noticed = presence
	if !known {
		return
	}
	if err := noticePresence(w.geom, w.mailbox, beams, presence); err != nil {
		w.log.fail("presence-notice", err)
		return
	}
	w.log.recovered("presence-notice")
}

// followPresence records presence and, when it turned to slack, ensures every listed beam's channel.
// A turn it could not act on, for a beams read that failed or an ensure that failed, is acted on again at the next wake.
func (w *wakeLoop) followPresence(ctx context.Context, presence Presence, beams []string, beamsErr error) {
	if presence != PresenceSlack || w.presence == PresenceSlack {
		w.presence = presence
		return
	}
	if beamsErr != nil {
		return
	}
	if err := ensureBeamChannels(ctx, w.geom, w.keeper, beams); err != nil {
		w.log.fail("beam-channels", err)
		return
	}
	w.log.recovered("beam-channels")
	w.presence = presence
}

// retryPrime ensures the prime's channel again when the start's ensure did not stand.
func (w *wakeLoop) retryPrime(ctx context.Context) {
	if !w.primePending {
		return
	}
	prime, err := orchName(w.geom.Shortname, "")
	if err == nil {
		_, err = w.keeper.ensure(ctx, prime)
	}
	if err != nil {
		w.log.fail("prime-channel", err)
		return
	}
	w.log.recovered("prime-channel")
	w.primePending = false
}

// runBinaryCheck re-execs onto a replaced lym binary.
// Before an exec it stops the socket holder and releases the relay lock.
// Stopping the holder closes its connection and releases the machine's socket lock.
// The exec's image waits its turn for both again.
// A failed exec retakes the relay lock and starts the holder again,
// and a relay lock kept by another relay ends this relay.
func (w *wakeLoop) runBinaryCheck(ctx context.Context) {
	w.checkBinary(
		func() {
			w.holder.stop()
			w.relayLock.release()
		},
		func(execErr error) {
			w.log.appendLine(fmt.Sprintf("%s re-exec failed: %v", w.clock.Now().UTC().Format(time.RFC3339), execErr))
			if !w.relayLock.retake() {
				w.log.appendLine("another relay holds the lock; this one exits")
				w.exiting = true
				return
			}
			w.holder.start(ctx)
		},
	)
}

// AwaitReparent waits until the process is no longer a child of intermediate, polling its parent once a second.
// It returns an error once bound has passed.
// A relay started through a double fork calls it before it takes its lock,
// so it never runs in its starter's tree.
func AwaitReparent(intermediate int, parentPID func() int, clock Clock, bound time.Duration) error {
	started := clock.Now()
	for parentPID() == intermediate {
		if clock.Now().Sub(started) >= bound {
			return fmt.Errorf("slack: the relay is still a child of its start process %d after %s; way forward: run `lym slack start` again from the operator's terminal", intermediate, bound)
		}
		<-clock.After(time.Second)
	}
	return nil
}
