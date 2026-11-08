// presence.go holds the machine's presence record: whether the operator is at the terminal or in Slack.

package slackengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// Presence says where the operator is: at the terminal or in Slack.
// It is the machine's, so it holds for every hub and every orch on the machine.
type Presence string

const (
	// PresenceTerminal means the operator works at the terminal; it is the presence of a machine with no record.
	PresenceTerminal Presence = "terminal"
	// PresenceSlack means the operator is away and talks to the orchs through Slack.
	PresenceSlack Presence = "slack"
)

// HookCommand is the command an orch session's prompt and turn-end hooks run, given the engine's name as its argument.
// It sets the presence back to terminal on a line the operator typed, and forwards a turn's final reply while away.
const HookCommand = "lym slack hook"

// presenceRecord is the presence file's content.
type presenceRecord struct {
	Presence  Presence  `json:"presence"`
	ChangedAt time.Time `json:"changed_at"`
}

// valid reports whether presence is one of the two values the record may hold.
func (presence Presence) valid() bool {
	return presence == PresenceTerminal || presence == PresenceSlack
}

// unknownPresenceError is the error of a presence file holding a value that is neither presence.
func unknownPresenceError(path string, presence Presence) error {
	return fmt.Errorf("slack: the presence file %s holds %q; want %q or %q", path, presence, PresenceTerminal, PresenceSlack)
}

// ReadPresence returns the presence stored at path, PresenceTerminal when no file exists.
// A stored value that is neither presence is an error naming the file.
func ReadPresence(path string) (Presence, error) {
	record, found, err := state.ReadJSON[presenceRecord](path, recordLockPath(path))
	if err != nil {
		return "", fmt.Errorf("slack: read the presence file %s: %w", path, err)
	}
	if !found {
		return PresenceTerminal, nil
	}
	if !record.Presence.valid() {
		return "", unknownPresenceError(path, record.Presence)
	}
	return record.Presence, nil
}

// WritePresence stores presence at path with now as its change time.
// It reports whether the stored value changed.
// A missing file counts as PresenceTerminal; a stored value that is neither presence is an error naming the file.
// A stored value equal to presence is left unwritten, so a repeat wakes no relay that watches the file.
// The writer sends no notice: every running relay of the machine sees the change and mails its own hub's orchs.
func WritePresence(path string, presence Presence, now time.Time) (bool, error) {
	if record, found, err := state.ReadJSON[presenceRecord](path, recordLockPath(path)); err == nil && found && record.Presence == presence {
		return false, nil
	}
	changed := false
	err := state.UpdateJSON(path, recordLockPath(path), func(current presenceRecord, found bool) (presenceRecord, error) {
		stored := PresenceTerminal
		if found {
			if !current.Presence.valid() {
				return current, unknownPresenceError(path, current.Presence)
			}
			stored = current.Presence
		}
		if found && stored == presence {
			return current, nil
		}
		changed = stored != presence
		return presenceRecord{Presence: presence, ChangedAt: now}, nil
	})
	if err != nil {
		return false, fmt.Errorf("slack: write the presence file %s: %w", path, err)
	}
	return changed, nil
}
