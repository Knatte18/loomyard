// relay.go runs the relay process.
// It waits to be reparented, takes the relay lock, makes the first creel read, and runs the inbound and outbound loops.
// It also re-execs onto a replaced binary and writes the relay log.

package slackengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

// ErrRelayInStrandPane reports that creel refused the relay's program claim.
// The relay runs in a strand's pane that is no orch's; the relay releases its lock before it returns this error.
var ErrRelayInStrandPane = errors.New("slack: the relay runs in a strand pane that is no orch's")

// errFirstReadAccepted is what the first read's handler returns to accept the read.
// The mail the read saw stays unread for the outbound read.
var errFirstReadAccepted = errors.New("slack: the first read was accepted and its mail left for the outbound read")

// RelayDeps holds the seams a relay process runs against.
// They are its creel mailbox, Slack client, hub, clock, and process operations.
type RelayDeps struct {
	// Mailbox is the relay's creel mailbox.
	Mailbox Mailbox
	// Slack is the Slack client, whose successful calls the relay records in its state.
	Slack SlackClient
	// Hub reads the hub's beams and pauses.
	Hub Hub
	// Clock is the relay's source of time.
	Clock Clock
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

// recordingSlack is a SlackClient that stamps LastSlackOK in the relay state after each call that succeeds.
type recordingSlack struct {
	inner SlackClient
	geom  Geometry
	clock Clock
	log   *relayLog
}

func (s *recordingSlack) stamp(err error) {
	if err != nil {
		return
	}
	now := s.clock.Now()
	if writeErr := updateRelayState(s.geom, func(relayState *RelayState) { relayState.LastSlackOK = now }); writeErr != nil {
		s.log.fail("relay-state", writeErr)
	}
}

func (s *recordingSlack) History(ctx context.Context, channel, oldest string) ([]Message, error) {
	messages, err := s.inner.History(ctx, channel, oldest)
	s.stamp(err)
	return messages, err
}

func (s *recordingSlack) Latest(ctx context.Context, channel string) (string, bool, error) {
	ts, found, err := s.inner.Latest(ctx, channel)
	s.stamp(err)
	return ts, found, err
}

func (s *recordingSlack) PostMessage(ctx context.Context, channel, text string) error {
	err := s.inner.PostMessage(ctx, channel, text)
	s.stamp(err)
	return err
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
// It refuses a hub with no token or no setup file.
// It takes the relay lock, registers itself, and records its pid.
// It makes the first creel read before any Slack call, then polls the channel and reads its own mailbox.
// Each loop waits on its own schedule.
// A creel refusal of the program claim from a strand's pane returns ErrRelayInStrandPane with the lock released.
// Any other first-read refusal is retried while the lock is held.
// At each poll it checks whether the lym binary was replaced and re-execs onto it.
// A relay that cannot re-exec retakes its lock, or exits when another relay holds it.
func RunRelay(ctx context.Context, geom Geometry, deps RelayDeps) error {
	if _, _, err := RequireConfigured(geom); err != nil {
		return err
	}
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
	if err := firstRead(ctx, geom, deps, heldLock, log, floor); err != nil {
		return err
	}

	loopCtx, stopLoops := context.WithCancel(ctx)
	defer stopLoops()
	slack := &recordingSlack{inner: deps.Slack, geom: geom, clock: deps.Clock, log: log}
	go runOutbound(loopCtx, newOutbound(geom, deps.Mailbox, slack, deps.Clock, floor), deps.Clock, log)
	poller := &inbound{geom: geom, mailbox: deps.Mailbox, slack: slack, hub: deps.Hub, clock: deps.Clock}
	return runInbound(loopCtx, poller, deps, heldLock, log, floor)
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

// runOutbound reads the relay's mailbox until ctx ends, waiting between reads as each read asks.
func runOutbound(ctx context.Context, out *outbound, clock Clock, log *relayLog) {
	for ctx.Err() == nil {
		wait, err := out.read(ctx)
		if err != nil {
			log.fail("outbound", err)
		} else {
			log.recovered("outbound")
		}
		if !waitFor(ctx, clock, wait) {
			return
		}
	}
}

// runInbound polls the channel until ctx ends.
// After each poll it checks for a replaced binary and then waits.
// The wait is the shared floor after activity, doubled while polls find nothing or fail, and a rate limit's retry wait.
func runInbound(ctx context.Context, in *inbound, deps RelayDeps, heldLock *relayLock, log *relayLog, floor *sharedFloor) error {
	wait := newLoopWait(floor)
	exiting := false
	for ctx.Err() == nil {
		activity, err := in.poll(ctx)
		var pause time.Duration
		limit, rateLimited := isRateLimit(err)
		switch {
		case rateLimited:
			log.fail("inbound", err)
			pause = wait.afterRateLimit(limit)
		case err != nil:
			log.fail("inbound", err)
			pause = wait.afterIdle()
			if activity {
				pause = wait.afterActivity()
			}
		case activity:
			log.recovered("inbound")
			pause = wait.afterActivity()
		default:
			log.recovered("inbound")
			pause = wait.afterIdle()
		}

		deps.CheckBinary(
			heldLock.release,
			func(execErr error) {
				log.appendLine(fmt.Sprintf("%s re-exec failed: %v", deps.Clock.Now().UTC().Format(time.RFC3339), execErr))
				if !heldLock.retake() {
					log.appendLine("another relay holds the lock; this one exits")
					exiting = true
				}
			},
		)
		if exiting {
			return nil
		}
		if !waitFor(ctx, deps.Clock, pause) {
			return nil
		}
	}
	return nil
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
