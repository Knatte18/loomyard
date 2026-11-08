// relaystate.go holds the relay's state record, which the relay and its starter write and `lym slack` reads.

package slackengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// RelayState is what the relay and its starter record about the relay process.
type RelayState struct {
	// PID is the relay's process id.
	PID int `json:"pid"`
	// StartTime is the relay's process start time, as the proc package reports it.
	StartTime string `json:"start_time"`
	// FirstReadAt is when the relay's first creel read was accepted, zero until then.
	FirstReadAt time.Time `json:"first_read_at"`
	// FirstReadRefusal is the text of the refusal the relay's first creel read met, empty when it met none.
	FirstReadRefusal string `json:"first_read_refusal"`
	// LastSlackOK is the relay's last-healthy time: the end of its latest outbound read and its latest Socket Mode pong.
	// The key keeps its older name, so a state an older relay wrote still feeds the restart cap.
	LastSlackOK time.Time `json:"last_slack_ok"`
	// StartFailures counts the starts in a row whose relay did not live long enough to count as healthy.
	StartFailures int `json:"start_failures"`
	// NextStartAt is the earliest time the starter may spawn the relay again.
	NextStartAt time.Time `json:"next_start_at"`
	// LastSpawnAt is when the starter last spawned the relay.
	LastSpawnAt time.Time `json:"last_spawn_at"`
}

// ReadRelayState returns the relay state of the hub in geom, and reports found false when no relay has recorded one.
func ReadRelayState(geom Geometry) (RelayState, bool, error) {
	relayState, found, err := state.ReadJSON[RelayState](geom.RelayStatePath, recordLockPath(geom.RelayStatePath))
	if err != nil {
		return RelayState{}, false, fmt.Errorf("slack: read the relay state %s: %w", geom.RelayStatePath, err)
	}
	return relayState, found, nil
}
