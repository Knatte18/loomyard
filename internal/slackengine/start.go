// start.go starts and stops the relay.
// It holds the double-fork spawn and the restart-capped starts the hub's records call for.
// It also holds the wait for a started relay's first read.

package slackengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// StartOutcome is how a start of the relay ended.
type StartOutcome string

const (
	// OutcomeStarted means the relay was down and a start spawned it.
	OutcomeStarted StartOutcome = "started"
	// OutcomeLive means the relay was already running.
	OutcomeLive StartOutcome = "live"
	// OutcomeNotConfigured means the machine has no token or the hub no setup, so no relay was started.
	OutcomeNotConfigured StartOutcome = "not-configured"
	// OutcomeTerminal means the presence is terminal, so a start made while the operator is away started nothing.
	OutcomeTerminal StartOutcome = "terminal"
	// OutcomeWaiting means a start made while the operator is away came before the next restart is due.
	OutcomeWaiting StartOutcome = "waiting"
	// OutcomeGaveUp means the relay died young too many starts in a row, so starts made while away spawn no more.
	OutcomeGaveUp StartOutcome = "gave-up"
)

const (
	// relayHealthyLifetime is how long a spawned relay must have lived for its spawn to count as healthy.
	relayHealthyLifetime = 10 * time.Minute
	// maxStartFailures is how many starts in a row may end in a relay that lived under relayHealthyLifetime.
	// Once that many have, starts made while the operator is away stop.
	maxStartFailures = 5
	// startBackoffBase is the wait before the next restart after a spawn, doubled for each counted failure.
	startBackoffBase = time.Minute
	// ReparentBound is how long a relay started with `--after` waits for its start process to exit.
	ReparentBound = 30 * time.Second
	// FirstReadBound is how long a start waits for the relay's first creel read.
	FirstReadBound = 30 * time.Second
)

// ErrSpawnUnderTest is what the spawn returns in a test binary, where starting the running binary would run the test suite again.
var ErrSpawnUnderTest = errors.New("slackengine: no relay spawned under test")

// starter starts the relay through its seams: how a process is spawned, the clock and the process's own id.
type starter struct {
	// spawn runs the lym binary with args from the hub's directory, detached in a new session;
	// with wait set it waits for the process to exit.
	spawn func(geom Geometry, args []string, wait bool) error
	clock Clock
	pid   func() int
}

// productionStarter returns the starter that spawns real processes.
func productionStarter() starter {
	return starter{spawn: spawnDetached, clock: SystemClock{}, pid: os.Getpid}
}

// relayArgs returns the arguments of a relay process for the hub and prime, followed by extra.
func relayArgs(geom Geometry, primeName string, extra ...string) []string {
	return append([]string{"slack", "relay", "--hub-path", geom.HubPath, "--prime", primeName}, extra...)
}

// spawnDetached starts the lym binary with args in a new session, with its output appended to the relay log.
// With wait set it waits for the process, the intermediate of a double fork, which exits at once;
// without it the process is released to run on as the relay.
// It logs the spawn and returns ErrSpawnUnderTest, starting nothing, in a test binary.
func spawnDetached(geom Geometry, args []string, wait bool) error {
	if testing.Testing() {
		return ErrSpawnUnderTest
	}
	if err := os.MkdirAll(filepath.Dir(geom.LogPath), privateDirMode); err != nil {
		return fmt.Errorf("slack: create the relay log directory: %w", err)
	}
	logFile, err := os.OpenFile(geom.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("slack: open the relay log %s: %w", geom.LogPath, err)
	}
	defer logFile.Close()
	cmd := exec.Command(geom.LymExecutable, args...)
	cmd.Dir = geom.HubPath
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("slackengine: spawn failed", "exe", geom.LymExecutable, "args", args, "dir", geom.HubPath, "error", err)
		return err
	}
	logger.Info("slackengine: spawned relay process", "exe", geom.LymExecutable, "args", args, "dir", geom.HubPath, "pid", cmd.Process.Pid, "wait", wait)
	if wait {
		return cmd.Wait()
	}
	return cmd.Process.Release()
}

// EnsureRelay starts the hub's relay when it is not running, and answers how that went.
// It starts nothing for a machine with no token or a hub with no setup file.
// It ignores the restart cap and resets it, so the operator's start and a prime's bring-up always start the relay.
// primeName is the prime worktree's name, which rides the relay's arguments.
func EnsureRelay(geom Geometry, primeName string) (StartOutcome, error) {
	return productionStarter().ensure(geom, primeName, false)
}

// EnsureRelayWhileAway starts the hub's relay on the prime dobby's tick, and only while the presence is slack.
// A relay that is down before its next restart is due answers OutcomeWaiting.
// A relay that died young too many starts in a row answers OutcomeGaveUp.
// It stays down until a start without the away gate resets the restart cap.
func EnsureRelayWhileAway(geom Geometry, primeName string) (StartOutcome, error) {
	return productionStarter().ensure(geom, primeName, true)
}

// RelayLive reports whether a relay runs for the hub: its lock is held.
// A free lock is released again at once.
func RelayLive(geom Geometry) (bool, error) {
	if _, err := os.Stat(filepath.Dir(geom.LockPath)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(geom.LockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// StopRelay ends the hub's relay and the processes below it through the run-process records.
// It spares callerPID and its ancestors.
func StopRelay(geom Geometry, callerPID int) ([]runproc.Ended, error) {
	return runproc.EndRole(geom.RunProcDir, runproc.HubOwner, runproc.RoleSlackRelay, callerPID)
}

// SpawnRelayFromIntermediate is the intermediate process's step of a double fork:
// it starts the relay detached in a new session and returns without waiting on it.
// The relay is then reparented out of the starter's tree, and out of any pane's, whoever started it.
func SpawnRelayFromIntermediate(geom Geometry, primeName string) error {
	return productionStarter().spawnRelay(geom, primeName)
}

// spawnRelay starts the relay, told its intermediate's pid, and does not wait on it.
func (s starter) spawnRelay(geom Geometry, primeName string) error {
	return s.spawn(geom, relayArgs(geom, primeName, "--after", strconv.Itoa(s.pid())), false)
}

// ensure starts the relay when it is down.
// A machine with no token or a hub with no setup file answers OutcomeNotConfigured, and a live relay OutcomeLive.
// With whileAway, any presence but slack answers OutcomeTerminal and starts nothing;
// a relay down before its next restart is due answers OutcomeWaiting.
// Each due start judges the relay its previous start spawned by how long it lived:
// the last successful Slack call minus the spawn time.
// A relay under relayHealthyLifetime counts one failure, and one at or over it resets the count;
// reaching maxStartFailures in a row answers OutcomeGaveUp and spawns nothing.
// Before it spawns, it records the spawn time and the next restart time.
// The restart comes one minute after the spawn, doubled for each counted failure.
// A spawn that fails to start still leaves both behind.
// A start without whileAway resets the count first.
func (s starter) ensure(geom Geometry, primeName string, whileAway bool) (StartOutcome, error) {
	_, tokenFound, err := ReadToken(geom.TokenPath)
	if err != nil {
		return "", err
	}
	_, setupFound, err := ReadSetup(geom.SetupPath)
	if err != nil {
		return "", err
	}
	if !tokenFound || !setupFound {
		return OutcomeNotConfigured, nil
	}
	if whileAway {
		presence, err := ReadPresence(geom.PresencePath)
		if err != nil {
			return "", err
		}
		if presence != PresenceSlack {
			return OutcomeTerminal, nil
		}
	}
	live, err := RelayLive(geom)
	if err != nil {
		return "", err
	}
	if live {
		return OutcomeLive, nil
	}

	now := s.clock.Now()
	failures := 0
	if whileAway {
		current, _, err := ReadRelayState(geom)
		if err != nil {
			return "", err
		}
		if current.StartFailures >= maxStartFailures {
			return OutcomeGaveUp, nil
		}
		if now.Before(current.NextStartAt) {
			return OutcomeWaiting, nil
		}
		failures = current.StartFailures
		if !current.LastSpawnAt.IsZero() {
			if lifetime(current) < relayHealthyLifetime {
				failures++
			} else {
				failures = 0
			}
		}
		if failures >= maxStartFailures {
			return OutcomeGaveUp, updateRelayState(geom, func(relayState *RelayState) { relayState.StartFailures = failures })
		}
	}
	if err := updateRelayState(geom, func(relayState *RelayState) {
		relayState.StartFailures = failures
		relayState.LastSpawnAt = now
		relayState.NextStartAt = now.Add(startBackoffBase << failures)
	}); err != nil {
		return "", err
	}
	if err := s.spawn(geom, relayArgs(geom, primeName, "--fork"), true); err != nil {
		return "", fmt.Errorf("slack: start the relay: %w", err)
	}
	return OutcomeStarted, nil
}

// lifetime returns how long the relay of the last spawn lived: its last successful Slack call minus the spawn time.
// A last call that is missing or not after the spawn reads as zero.
func lifetime(relayState RelayState) time.Duration {
	if !relayState.LastSlackOK.After(relayState.LastSpawnAt) {
		return 0
	}
	return relayState.LastSlackOK.Sub(relayState.LastSpawnAt)
}

// relayRunning reports whether the process the relay state names is running:
// its pid is alive and, when a start time is recorded, still has it.
func relayRunning(relayState RelayState) bool {
	if relayState.PID <= 0 || !proc.IsRunning(relayState.PID) {
		return false
	}
	if relayState.StartTime == "" {
		return true
	}
	startTime, ok := proc.StartTime(relayState.PID)
	return ok && startTime == relayState.StartTime
}

// AwaitFirstRead waits until the relay the state names is running and has made its first creel read.
// It reads the state once a second and returns an error once bound has passed.
// The refusal names the running relay's recorded first-read refusal, or the relay log when it recorded none.
// A state whose relay is gone counts as not yet written, whatever refusal it records, since it belongs to an earlier relay.
func AwaitFirstRead(geom Geometry, clock Clock, bound time.Duration) error {
	const wayForward = "way forward: fix what it names, then run `lym slack away` again; the relay keeps its lock and retries"
	started := clock.Now()
	for {
		relayState, found, err := ReadRelayState(geom)
		if err != nil {
			return err
		}
		running := found && relayRunning(relayState)
		if running && !relayState.FirstReadAt.IsZero() {
			return nil
		}
		if clock.Now().Sub(started) >= bound {
			if running && relayState.FirstReadRefusal != "" {
				return fmt.Errorf("slack: the relay's first creel read was refused: %s; %s", relayState.FirstReadRefusal, wayForward)
			}
			return fmt.Errorf("slack: the relay made no first creel read within %s; its log %s says why; %s", bound, geom.LogPath, wayForward)
		}
		<-clock.After(time.Second)
	}
}
