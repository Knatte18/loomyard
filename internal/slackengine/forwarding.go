// forwarding.go holds the hub's forwarding record: whether each orch's slack button is on.

package slackengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// forwardingRecord is the forwarding file's content, keyed by the orch's creel name.
type forwardingRecord struct {
	Orchs map[string]orchForwarding `json:"orchs"`
}

// orchForwarding is one orch's button state and when it last changed.
type orchForwarding struct {
	On        bool      `json:"on"`
	ChangedAt time.Time `json:"changed_at"`
}

// ReadForwarding returns the creel names of the hub's orchs whose slack button is on, sorted.
// A missing file holds none.
// An orch gets a channel, and its mail reaches Slack, only while its button is on.
func ReadForwarding(path string) ([]string, error) {
	record, _, err := state.ReadJSON[forwardingRecord](path, recordLockPath(path))
	if err != nil {
		return nil, fmt.Errorf("slack: read the forwarding file %s: %w", path, err)
	}
	orchs := make([]string, 0, len(record.Orchs))
	for orch, forwarding := range record.Orchs {
		if forwarding.On {
			orchs = append(orchs, orch)
		}
	}
	sort.Strings(orchs)
	return orchs, nil
}

// ForwardingOn reports whether the slack button of the orch with creel name orch is on.
func ForwardingOn(path, orch string) (bool, error) {
	orchs, err := ReadForwarding(path)
	if err != nil {
		return false, err
	}
	return sortedContains(orchs, orch), nil
}

// WriteForwarding records whether the slack button of the orch with creel name orch is on, with now as its change time.
// It reports whether the stored state changed; a repeat leaves the file unwritten, so it wakes no relay that watches it.
func WriteForwarding(path, orch string, on bool, now time.Time) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return false, fmt.Errorf("slack: create the directory of the forwarding file %s: %w", path, err)
	}
	changed := false
	err := state.UpdateJSON(path, recordLockPath(path), func(current forwardingRecord, _ bool) (forwardingRecord, error) {
		if current.Orchs[orch].On == on {
			return current, nil
		}
		if current.Orchs == nil {
			current.Orchs = map[string]orchForwarding{}
		}
		current.Orchs[orch] = orchForwarding{On: on, ChangedAt: now}
		changed = true
		return current, nil
	})
	if err != nil {
		return false, fmt.Errorf("slack: write the forwarding file %s: %w", path, err)
	}
	return changed, nil
}

// dropForwarding removes the orch's entry from the forwarding file.
// An orch that retired thereby leaves no button on behind it.
func dropForwarding(path, orch string) error {
	err := state.UpdateJSON(path, recordLockPath(path), func(current forwardingRecord, _ bool) (forwardingRecord, error) {
		delete(current.Orchs, orch)
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("slack: write the forwarding file %s: %w", path, err)
	}
	return nil
}

// sortedContains reports whether the sorted list holds name.
func sortedContains(sorted []string, name string) bool {
	i := sort.SearchStrings(sorted, name)
	return i < len(sorted) && sorted[i] == name
}
