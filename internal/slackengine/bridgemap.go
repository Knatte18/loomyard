// bridgemap.go holds the hub's bridge map: the bridge channel of each pair of orchs and the cursors read in it.

package slackengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/state"
)

// BridgeMap is the hub's record of its bridge channels, keyed by the channel's Slack id.
type BridgeMap struct {
	// Channels holds one entry per bridge channel.
	Channels map[string]BridgeEntry `json:"channels"`
}

// BridgeEntry is the bridge channel of one pair of orchs and how far the relay has read it.
type BridgeEntry struct {
	// Name is the channel's name.
	Name string `json:"name"`
	// LocalOrch is the creel name of this hub's orch of the pair.
	LocalOrch string `json:"local_orch"`
	// Peer is the full `<machine>::<name>` address of the other machine's orch of the pair.
	Peer string `json:"peer"`
	// Operator is the Slack user id of the operator invited to the channel, empty for none.
	Operator string `json:"operator"`
	// PeerInvited reports whether the peer's bot was invited to the channel.
	PeerInvited bool `json:"peer_invited"`
	// Cursor is the delivery cursor: the timestamp of the newest message of the channel the relay has handled.
	Cursor string `json:"cursor"`
	// OperatorCursor is the timestamp of the newest message of the operator the relay has handled.
	OperatorCursor string `json:"operator_cursor"`
	// Threads maps a thread's timestamp to the timestamp of the newest reply handled.
	Threads map[string]string `json:"threads"`
	// ThreadFloor is the timestamp before which a thread with no cursor has nothing left to read.
	ThreadFloor string `json:"thread_floor"`
	// Renamed reports whether the channel carries the dated name a retiring channel takes before its archive.
	Renamed bool `json:"renamed,omitempty"`
	// Archived reports whether the channel is archived.
	Archived bool `json:"archived"`
}

// ReadBridgeMap reads the bridge map at path, and reads a missing file as an empty map.
// A file that does not decode is an error that names it and wraps state.ErrDecode.
func ReadBridgeMap(path string) (BridgeMap, error) {
	bridgeMap, _, err := state.ReadJSON[BridgeMap](path, recordLockPath(path))
	if err != nil {
		return BridgeMap{}, fmt.Errorf("slack: read the bridge map %s: %w", path, err)
	}
	return bridgeMap, nil
}

// UpdateBridgeMap is the one writer of the bridge map at path.
// It reads the map, calls mutate on it and writes the result, all under the map's lock,
// so a key an older binary does not know survives and two updates never lose one another's.
// mutate makes no Slack call, since the lock is held while it runs.
func UpdateBridgeMap(path string, mutate func(*BridgeMap) error) error {
	err := state.UpdateJSON(path, recordLockPath(path), func(current BridgeMap, _ bool) (BridgeMap, error) {
		if current.Channels == nil {
			current.Channels = map[string]BridgeEntry{}
		}
		if err := mutate(&current); err != nil {
			return BridgeMap{}, err
		}
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: update the bridge map %s: %w", path, err)
	}
	return nil
}

// OpenChannelIDs returns the ids, sorted, of the open channels whose entry keep accepts; a nil keep accepts every entry.
func (m BridgeMap) OpenChannelIDs(keep func(BridgeEntry) bool) []string {
	ids := make([]string, 0, len(m.Channels))
	for id, entry := range m.Channels {
		if !entry.Archived && (keep == nil || keep(entry)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// ChannelOf returns the id of the open bridge channel of the local orch and the peer address, skipping archived ones.
// Several open channels of one pair answer the one with the lowest id.
func (m BridgeMap) ChannelOf(localOrch, peer string) (string, bool) {
	found := ""
	for id, entry := range m.Channels {
		if entry.Archived || entry.LocalOrch != localOrch || entry.Peer != peer {
			continue
		}
		if found == "" || id < found {
			found = id
		}
	}
	return found, found != ""
}
