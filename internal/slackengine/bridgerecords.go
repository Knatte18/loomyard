// bridgerecords.go holds the hub's two records of the relay's bridge sending.
// They are the posts Slack accepted and the notices sent.

package slackengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// bridgePosts is the record of the machine mail the relay posted to bridge channels.
// It is apart from the outbox record of orch mail, so a confirmed orch-mail read never clears it.
type bridgePosts struct {
	// Accepted maps the id of each message Slack accepted to the timestamp Slack gave it, until creel has settled it.
	Accepted map[int64]string `json:"accepted,omitempty"`
	// Refusals counts how often Slack refused each message with an error code.
	Refusals map[int64]int `json:"refusals,omitempty"`
}

// bridgeNotices is the record of the bridge notices already sent.
// It keeps a restarted relay from posting a second notice per episode.
type bridgeNotices struct {
	// Marks maps the key of each notice sent to when it was sent.
	Marks map[string]time.Time `json:"marks,omitempty"`
}

// readBridgePosts reads the bridge-post record at path, and reads a missing file as an empty record.
func readBridgePosts(path string) (bridgePosts, error) {
	posts, _, err := state.ReadJSON[bridgePosts](path, recordLockPath(path))
	if err != nil {
		return bridgePosts{}, fmt.Errorf("slack: read the bridge-post record %s: %w", path, err)
	}
	return posts, nil
}

// writeBridgePosts stores posts at path, keeping any key of the file that bridgePosts does not declare.
func writeBridgePosts(path string, posts bridgePosts) error {
	if err := state.WriteJSON(path, recordLockPath(path), posts); err != nil {
		return fmt.Errorf("slack: write the bridge-post record %s: %w", path, err)
	}
	return nil
}

// readBridgeNotices reads the bridge-notice record at path, and reads a missing file as an empty record.
func readBridgeNotices(path string) (bridgeNotices, error) {
	notices, _, err := state.ReadJSON[bridgeNotices](path, recordLockPath(path))
	if err != nil {
		return bridgeNotices{}, fmt.Errorf("slack: read the bridge-notice record %s: %w", path, err)
	}
	return notices, nil
}

// markBridgeNotice sets key in the bridge-notice record at path.
// It answers true only when the key was unset, so the caller that gets true sends the notice.
func markBridgeNotice(path, key string) (bool, error) {
	marked := false
	err := state.UpdateJSON(path, recordLockPath(path), func(current bridgeNotices, _ bool) (bridgeNotices, error) {
		if _, set := current.Marks[key]; set {
			return current, nil
		}
		if current.Marks == nil {
			current.Marks = map[string]time.Time{}
		}
		current.Marks[key] = time.Now().UTC()
		marked = true
		return current, nil
	})
	if err != nil {
		return false, fmt.Errorf("slack: write the bridge-notice record %s: %w", path, err)
	}
	return marked, nil
}

// clearBridgeNotice drops key from the bridge-notice record at path, so the next episode of the notice sends it again.
func clearBridgeNotice(path, key string) error {
	err := state.UpdateJSON(path, recordLockPath(path), func(current bridgeNotices, _ bool) (bridgeNotices, error) {
		delete(current.Marks, key)
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: write the bridge-notice record %s: %w", path, err)
	}
	return nil
}
