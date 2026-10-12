// agentwait.go declares the files-only reading of whether each live run is waiting or busy, the reading the parent lock's idle rule folds.
// Like ReadAgentActivity it reads files only, so any process may call it, and it writes nothing.

package shuttleengine

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/logger"
)

// AgentWait is one live run's waiting and busy reading.
type AgentWait struct {
	// StrandName is the run's strand name, the strand guid for a record that predates the name.
	StrandName string
	// Waiting is true when the run waits on something outside its own work: its held turn end, a question or a stall.
	Waiting bool
	// Busy is true when the run's session is working and is not waiting.
	Busy bool
}

// ReadAgentWaits returns one reading per run under the run-directory root of anchorPath whose record reads running and whose waiting process is alive.
// A run is Waiting when its own wait marker is live and is the held turn end, or when its session state is asking or idle-stalled.
// A run is Busy when its session state is busy and it is not waiting; every other state is neither.
// The session state is the fold ReadSessionStates reads, at now.
// A directory that is not a run directory, a run with no record, a run that is not live and an unreadable record are skipped, and an unreadable record is logged.
// An absent root returns no readings and no error.
func ReadAgentWaits(cfg Config, anchorPath string, engine Engine, now time.Time) ([]AgentWait, error) {
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var waits []AgentWait
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil {
			logger.Warn("shuttle: skipping an unreadable run record", "runDir", runDir, "cause", err)
			continue
		}
		if !found || !runIsLive(rs) {
			continue
		}
		session := readRunSessionState(rs, engine, now)
		waiting := heldMarkerLive(runDir) || session.State.Name == SessionAsking || session.State.Name == SessionIdleStalled
		waits = append(waits, AgentWait{
			StrandName: session.StrandName,
			Waiting:    waiting,
			Busy:       session.State.Name == SessionBusy && !waiting,
		})
	}
	return waits, nil
}

// heldMarkerLive reports whether the run directory holds a wait marker for the held turn end whose pid is alive.
// An absent or unreadable marker reads as no wait.
func heldMarkerLive(runDir string) bool {
	path := filepath.Join(runDir, waitMarkerFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	var marker WaitMarker
	if err == nil {
		err = yaml.Unmarshal(data, &marker)
	}
	if err != nil {
		logger.Warn("shuttle: skipping an unreadable wait marker", "path", path, "cause", err)
		return false
	}
	return marker.Held() && isAlive(marker.PID)
}
