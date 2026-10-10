// marker.go declares the running marker Verify writes while a command runs and the reader loom status uses.

package verifytree

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/proc"
)

// isAlive is the liveness seam, so marker tests stay tier 1.
var isAlive = proc.IsAlive

// The two states of a Marker.
const (
	// MarkerStateWaiting means the run waits for a gate slot and has not started its command.
	MarkerStateWaiting = "waiting"
	// MarkerStateRunning means the run holds its slot, or runs unslotted, and its command runs.
	MarkerStateRunning = "running"
)

// Marker is the running marker: which site is verifying which command, since when, and the pid that holds it.
// State says whether the run still waits for a gate slot; WaitStarted is when that wait began, zero for an unslotted run.
type Marker struct {
	Site        string    `yaml:"site"`
	Attempt     int       `yaml:"attempt"`
	Command     string    `yaml:"command"`
	Started     time.Time `yaml:"started"`
	PID         int       `yaml:"pid"`
	State       string    `yaml:"state,omitempty"`
	WaitStarted time.Time `yaml:"wait_started,omitempty"`
}

// ReadMarker reads the marker at path.
// It reports false for an absent marker and for one whose pid is no longer alive, which a crashed run leaves behind.
// A marker written before it had a state reads as running.
// A malformed marker is an error.
func ReadMarker(path string) (Marker, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Marker{}, false, nil
	}
	if err != nil {
		return Marker{}, false, fmt.Errorf("verifytree: read marker %s: %w", path, err)
	}
	var m Marker
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Marker{}, false, fmt.Errorf("verifytree: decode marker %s: %w", path, err)
	}
	if !isAlive(m.PID) {
		return Marker{}, false, nil
	}
	if m.State == "" {
		m.State = MarkerStateRunning
	}
	return m, true, nil
}

// writeMarker writes m to path.
func writeMarker(path string, m Marker) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("verifytree: encode marker: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("verifytree: write marker %s: %w", path, err)
	}
	return nil
}
