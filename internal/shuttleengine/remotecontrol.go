// remotecontrol.go reads whether a session's remote control connected.
// It waits a bounded time for the provider to record the connection.

package shuttleengine

import (
	"fmt"
	"time"
)

const (
	// remoteControlConnectWait bounds how long a check waits for a session to record its remote control connecting.
	// A connection lands about a second after the request; a refused one never lands.
	remoteControlConnectWait = 20 * time.Second
	// remoteControlPollTick is the interval between two reads of the provider's record during that wait.
	remoteControlPollTick = 500 * time.Millisecond
)

// RemoteControlCheckpoint returns how far the provider's record of the session sessionID on engineName reaches now.
// A later AwaitRemoteControl from it sees only a connection recorded after this call.
// An engine that cannot read remote control returns zero.
func (r *Runner) RemoteControlCheckpoint(engineName, sessionID string) (int64, error) {
	if r.toldErr != nil {
		return 0, r.toldErr
	}
	engine, err := r.engines.Lookup(engineName)
	if err != nil {
		return 0, err
	}
	return r.remoteControlCheckpoint(engine, sessionID)
}

// AwaitRemoteControl reports whether the session sessionID on engineName records a connection past checkpoint.
// It waits a bounded time for the record.
// It reports false when none comes: the provider refused the connection, or the engine cannot read remote control.
func (r *Runner) AwaitRemoteControl(engineName, sessionID string, checkpoint int64) (bool, error) {
	if r.toldErr != nil {
		return false, r.toldErr
	}
	engine, err := r.engines.Lookup(engineName)
	if err != nil {
		return false, err
	}
	return r.awaitRemoteControl(engine, sessionID, checkpoint)
}

// remoteControlCheckpoint returns how far engine's record of the session sessionID reaches now.
// An engine that reads no remote control returns zero.
func (r *Runner) remoteControlCheckpoint(engine Engine, sessionID string) (int64, error) {
	reader, ok := engine.(RemoteControlReader)
	if !ok || sessionID == "" {
		return 0, nil
	}
	checkpoint, err := reader.RemoteControlCheckpoint(sessionID, r.paneCwd)
	if err != nil {
		return 0, fmt.Errorf("shuttle: read where session %s's record ends before turning on remote control: %w", sessionID, err)
	}
	return checkpoint, nil
}

// awaitRemoteControl polls engine's record of the session sessionID for a remote-control connection past checkpoint.
// It reports false once remoteControlConnectWait has passed on the runner's clock with none recorded.
func (r *Runner) awaitRemoteControl(engine Engine, sessionID string, checkpoint int64) (bool, error) {
	reader, ok := engine.(RemoteControlReader)
	if !ok || sessionID == "" {
		return false, nil
	}
	deadline := r.clock.Now().Add(remoteControlConnectWait)
	for {
		connected, err := reader.RemoteControlConnected(sessionID, r.paneCwd, checkpoint)
		if err != nil {
			return false, fmt.Errorf("shuttle: read whether session %s's remote control connected: %w", sessionID, err)
		}
		if connected {
			return true, nil
		}
		if !r.clock.Now().Before(deadline) {
			return false, nil
		}
		r.clock.Sleep(remoteControlPollTick)
	}
}
