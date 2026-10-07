// waitmark.go declares the wait marker and pane mark shuttle keeps while its Go side waits on a gate entry or on background shells,
// and ReadWaitMarker, the reader `lyx loom status` uses.
// Both are display and status only: no shuttle decision reads them,
// and a failure to write, remove, set or clear either is logged and changes nothing else.

package shuttleengine

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// waitMarkerFileName is the wait marker's file name inside a per-run directory.
	waitMarkerFileName = "wait.yaml"
	// gateWaitLabelPrefix leads the label of a gate-entry wait, followed by the entry's name.
	gateWaitLabelPrefix = "gate "
	// shellWaitLabel is the label of the background-shell wait.
	shellWaitLabel = "background shells"
)

// isAlive is the liveness seam over proc.IsAlive, so the marker tests stay tier 1.
var isAlive = proc.IsAlive

// WaitMarker is the file `lyx loom status` reads to see what a shuttle run waits on:
// the wait's label (`gate <entry name>` or `background shells`), when it started, and the pid of the process running Wait.
type WaitMarker struct {
	Kind    string    `yaml:"kind"`
	Started time.Time `yaml:"started"`
	PID     int       `yaml:"pid"`
}

// waitState is the run's record of the wait it has on show.
type waitState struct {
	// gateLabel is the label of the gate entry whose closure is running, empty between closures.
	gateLabel string
	// shellStart is when the background-shell wait began, the zero time while it is off.
	shellStart time.Time
	// shown is the label currently on disk and on screen, empty when nothing is.
	shown string
}

// ReadWaitMarker returns the first wait marker with a live pid among the run directories under the run-directory root.
// A marker whose pid is dead reads as absent and the scan goes on,
// and an unreadable or undecodable marker is skipped with a logged warning,
// so one torn file cannot hide another run's wait.
func ReadWaitMarker(cfg Config, anchorPath string) (WaitMarker, bool, error) {
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return WaitMarker{}, false, nil
	}
	if err != nil {
		return WaitMarker{}, false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), waitMarkerFileName)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		var marker WaitMarker
		if err == nil {
			err = yaml.Unmarshal(data, &marker)
		}
		if err != nil {
			logger.Warn("shuttle: skipping an unreadable wait marker", "path", path, "cause", err)
			continue
		}
		if isAlive(marker.PID) {
			return marker, true, nil
		}
	}
	return WaitMarker{}, false, nil
}

// waitMarkerPath returns the wait marker's path inside the run directory.
func (run *Run) waitMarkerPath() string {
	return filepath.Join(run.runDir, waitMarkerFileName)
}

// clearWait removes any wait marker file of the run and clears the strand's pane mark, whatever this process last showed.
// Wait calls it on entry, so a mark a crashed step left behind is gone at the next touch.
func (run *Run) clearWait() {
	run.wait = waitState{}
	run.removeWaitMarker()
	if err := run.runner.reed.SetWaitMark(run.state.StrandGUID, "", time.Time{}); err != nil {
		logger.Warn("shuttle: could not clear the wait mark", "strandGUID", run.state.StrandGUID, "cause", err)
	}
}

// endWait removes the wait marker file and clears the pane mark when this process has one on show.
// Wait defers it, so every return leaves neither behind.
func (run *Run) endWait() {
	run.removeWaitMarker()
	if run.wait.shown == "" {
		return
	}
	run.wait = waitState{}
	if err := run.runner.reed.SetWaitMark(run.state.StrandGUID, "", time.Time{}); err != nil {
		logger.Warn("shuttle: could not clear the wait mark", "strandGUID", run.state.StrandGUID, "cause", err)
	}
}

// removeWaitMarker removes the run's wait marker file, logging a failure.
func (run *Run) removeWaitMarker() {
	if err := os.Remove(run.waitMarkerPath()); err != nil && !os.IsNotExist(err) {
		logger.Warn("shuttle: could not remove the wait marker", "path", run.waitMarkerPath(), "cause", err)
	}
}

// beginGateWait shows `gate <entry name>` for the closure about to run.
func (run *Run) beginGateWait(entryName string) {
	run.wait.gateLabel = gateWaitLabelPrefix + entryName
	run.showWait()
}

// endGateWait takes the gate label off, which brings back the background-shell wait when one is on.
func (run *Run) endGateWait() {
	run.wait.gateLabel = ""
	run.showWait()
}

// syncShellWait turns the background-shell wait on or off to match the recorded waiting turn end:
// it is on exactly while every outstanding task is a non-awaited background shell and at least one has not been waited out.
func (run *Run) syncShellWait() {
	active := run.shellWaitActive()
	switch {
	case active && run.wait.shellStart.IsZero():
		run.wait.shellStart = run.clock.Now()
	case !active && !run.wait.shellStart.IsZero():
		run.wait.shellStart = time.Time{}
	default:
		return
	}
	run.showWait()
}

// shellWaitActive reports whether the recorded waiting turn end holds only non-awaited background shells, not all waited out.
func (run *Run) shellWaitActive() bool {
	if len(run.waitingTasks) == 0 {
		return false
	}
	pending := false
	for _, task := range run.waitingTasks {
		if task.Kind != BackgroundShell || run.awaitedShell(task) {
			return false
		}
		if !run.expiredShells[task.ID] {
			pending = true
		}
	}
	return pending
}

// showWait brings the marker file and the pane mark in line with the wait that should be on show:
// a running gate entry wins over the background-shell wait,
// and neither shows nothing.
func (run *Run) showWait() {
	label, started := "", time.Time{}
	switch {
	case run.wait.gateLabel != "":
		label, started = run.wait.gateLabel, run.clock.Now()
	case !run.wait.shellStart.IsZero():
		label, started = shellWaitLabel, run.wait.shellStart
	}
	if label == run.wait.shown {
		return
	}
	run.wait.shown = label
	if label == "" {
		run.removeWaitMarker()
		if err := run.runner.reed.SetWaitMark(run.state.StrandGUID, "", time.Time{}); err != nil {
			logger.Warn("shuttle: could not clear the wait mark", "strandGUID", run.state.StrandGUID, "cause", err)
		}
		return
	}
	run.writeWaitMarker(WaitMarker{Kind: label, Started: started, PID: os.Getpid()})
	if err := run.runner.reed.SetWaitMark(run.state.StrandGUID, label, started); err != nil {
		logger.Warn("shuttle: could not set the wait mark", "strandGUID", run.state.StrandGUID, "label", label, "cause", err)
	}
}

// writeWaitMarker writes marker as the run's wait marker, logging a failure.
func (run *Run) writeWaitMarker(marker WaitMarker) {
	data, err := yaml.Marshal(marker)
	if err == nil {
		err = os.WriteFile(run.waitMarkerPath(), data, 0o644)
	}
	if err != nil {
		logger.Warn("shuttle: could not write the wait marker", "path", run.waitMarkerPath(), "cause", err)
	}
}
