// peers.go holds the machine's peers record: the machines whose Slack app the operator approved.

package slackengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// Peers is the machine's record of the approved machines, keyed by the machine name lower-cased.
type Peers struct {
	// Machines holds one entry per approved machine.
	Machines map[string]Peer `json:"machines"`
}

// Peer is one approved machine and the Slack app and bot the operator approved for it.
type Peer struct {
	// AppID is the id of the Slack app of the machine.
	AppID string `json:"app_id"`
	// BotUserID is the Slack user id of the machine's bot.
	BotUserID string `json:"bot_user_id"`
	// ApprovedAt is when the operator approved the machine.
	ApprovedAt time.Time `json:"approved_at"`
}

// ReadPeers reads the peers record at path, and reads a missing file as no peers.
// A file that does not decode is an error that names it and wraps state.ErrDecode.
func ReadPeers(path string) (Peers, error) {
	peers, _, err := state.ReadJSON[Peers](path, recordLockPath(path))
	if err != nil {
		return Peers{}, fmt.Errorf("slack: read the peers record %s: %w", path, err)
	}
	return peers, nil
}

// UpdatePeers is the one writer of the peers record at path.
// It reads the record, calls mutate on it and writes the result, all under the record's lock,
// so a key an older binary does not know survives and two updates never lose one another's.
// mutate makes no Slack call, since the lock is held while it runs.
func UpdatePeers(path string, mutate func(*Peers) error) error {
	err := state.UpdateJSON(path, recordLockPath(path), func(current Peers, _ bool) (Peers, error) {
		if current.Machines == nil {
			current.Machines = map[string]Peer{}
		}
		if err := mutate(&current); err != nil {
			return Peers{}, err
		}
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: update the peers record %s: %w", path, err)
	}
	return nil
}
