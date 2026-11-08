// channelmap.go holds the hub's channel map: the Slack channel of each orch and the forward-only cursors read in it.

package slackengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// threadRetention is how long a thread's cursor is kept, and how far behind now the thread floor stands after a prune.
const threadRetention = 24 * time.Hour

// ChannelMap is the hub's record of the Slack channel of each orch, keyed by the orch's creel name.
// The key is `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam.
type ChannelMap struct {
	// Channels holds one entry per orch.
	Channels map[string]ChannelEntry `json:"channels"`
}

// ChannelEntry is the Slack channel of one orch and how far the relay has read it.
type ChannelEntry struct {
	// ID is the channel's Slack id.
	ID string `json:"id"`
	// Name is the channel's name.
	Name string `json:"name"`
	// Cursor is the timestamp of the newest top-level message handled.
	Cursor string `json:"cursor"`
	// Threads maps a thread's timestamp to the timestamp of the newest reply handled.
	Threads map[string]string `json:"threads"`
	// ThreadFloor is the timestamp before which a thread with no cursor has nothing left to forward.
	ThreadFloor string `json:"thread_floor"`
	// Operator is the Slack user id of the operator invited to the channel, empty for none.
	Operator string `json:"operator"`
	// Archived reports whether the channel is archived.
	Archived bool `json:"archived"`
}

// ReadChannelMap reads the channel map at path, and reads a missing file as an empty map.
// A file that does not decode is an error that names it and wraps state.ErrDecode.
// The way forward is to move the file aside:
// the relay then ensures the hub's channels anew and adopts the channels it made on `name_taken`.
func ReadChannelMap(path string) (ChannelMap, error) {
	channelMap, _, err := state.ReadJSON[ChannelMap](path, recordLockPath(path))
	if err != nil {
		return ChannelMap{}, fmt.Errorf("slack: read the channel map %s: %w; way forward: move the file aside", path, err)
	}
	return channelMap, nil
}

// UpdateChannelMap is the one writer of the channel map at path.
// It reads the map, calls mutate on it and writes the result, all under the map's lock,
// so a key an older binary does not know survives and two updates never lose one another's.
// mutate makes no Slack call, since the lock is held while it runs.
func UpdateChannelMap(path string, mutate func(*ChannelMap) error) error {
	err := state.UpdateJSON(path, recordLockPath(path), func(current ChannelMap, _ bool) (ChannelMap, error) {
		if current.Channels == nil {
			current.Channels = map[string]ChannelEntry{}
		}
		if err := mutate(&current); err != nil {
			return ChannelMap{}, err
		}
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: update the channel map %s: %w", path, err)
	}
	return nil
}

// OrchOf returns the creel name of the orch whose entry holds the channel id, archived or not.
func (m ChannelMap) OrchOf(channelID string) (string, bool) {
	for orch, entry := range m.Channels {
		if entry.ID == channelID {
			return orch, true
		}
	}
	return "", false
}

// Advance moves the entry's cursor to ts when ts is later; a cursor never moves back.
func (e *ChannelEntry) Advance(ts string) {
	if compareTS(ts, e.Cursor) > 0 {
		e.Cursor = ts
	}
}

// AdvanceThread moves the cursor of the thread to ts when ts is later; a cursor never moves back.
func (e *ChannelEntry) AdvanceThread(thread, ts string) {
	if current, ok := e.Threads[thread]; ok && compareTS(ts, current) <= 0 {
		return
	}
	if e.Threads == nil {
		e.Threads = map[string]string{}
	}
	e.Threads[thread] = ts
}

// ThreadStart returns the timestamp after which a read of the thread starts.
// It is the thread's cursor, or with none the later of the thread's own timestamp and the thread floor,
// so a reply the channel handled before the floor is not forwarded again.
func (e ChannelEntry) ThreadStart(thread string) string {
	if cursor, ok := e.Threads[thread]; ok {
		return cursor
	}
	if compareTS(e.ThreadFloor, thread) > 0 {
		return e.ThreadFloor
	}
	return thread
}

// PruneThreads drops each thread cursor older than 24 hours before now and raises the thread floor to that time.
// A reply that no read caught before the prune is therefore not recovered.
func (e *ChannelEntry) PruneThreads(now time.Time) {
	cutoff := slackTimestamp(now.Add(-threadRetention))
	for thread, cursor := range e.Threads {
		if compareTS(cursor, cutoff) < 0 {
			delete(e.Threads, thread)
		}
	}
	if compareTS(cutoff, e.ThreadFloor) > 0 {
		e.ThreadFloor = cutoff
	}
}
