// reminder.go composes the lines typed to a receiver with unread mail: a program's notice, and the reminder of the rest.
// It also decides when the next reminder is due.

package creelengine

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Knatte18/loomyard/internal/agentname"
)

const (
	// reminderBackoffBase is how long a receiver that ended a reminder turn with the same unread set waits for its next reminder.
	reminderBackoffBase = 5 * time.Minute
	// reminderBackoffCeiling is the longest the reminder backoff grows to.
	reminderBackoffCeiling = time.Hour
	// maxReminderMessages is how many unread messages a reminder lists; the rest are counted.
	maxReminderMessages = 5
	// maxNoticeBytes is the longest body a message may have and still be carried in full in its own line.
	maxNoticeBytes = 4096
)

// isNotice reports whether msg is carried in full on its own line instead of being reminded.
// A notice is a message from a program or EXTERNAL that is not an agent strand, with a body of at most maxNoticeBytes.
// Mail from an agent strand is never carried, so an agent's text cannot pose as an instruction in another agent's pane.
func isNotice(msg *Message) bool {
	return agentname.IsProgram(msg.From) && len(msg.Body) <= maxNoticeBytes
}

// LinePrefix opens every line creel types into a session or hands it at a hook:
// a message's line, a reminder, the bell and the echo heading.
const LinePrefix = "[creel]"

// MessageLine renders the one line that carries a message in full.
// It is typed as a notice into an idle pane, or handed over as mail at a tool boundary.
// The line starts with the message id, so the receiver can reply without a read, then gives the sender and the body.
// Every run of control characters and line breaks in the body becomes one space.
// A message that expects a reply also says so, with the command that answers it.
// A message from another hub carries its stamp after the sender.
func MessageLine(msg Message) string {
	body := strings.TrimSpace(strings.Join(strings.FieldsFunc(msg.Body, unicode.IsControl), " "))
	sender := fmt.Sprintf("\"%s\"", msg.From)
	if stamp := msg.HubStamp(); stamp != "" {
		sender += " " + stamp
	}
	if msg.ExpectReply {
		return fmt.Sprintf("[creel] #%d from %s, expects a reply (lym creel post --reply %d \"<text>\"): %s", msg.ID, sender, msg.ID, body)
	}
	return fmt.Sprintf("[creel] #%d from %s: %s", msg.ID, sender, body)
}

// reminderBackoff returns the wait after the reminder at the given step of an unchanged unread set.
// The wait doubles from the base up to the ceiling.
func reminderBackoff(step int) time.Duration {
	wait := reminderBackoffBase
	for i := 0; i < step && wait < reminderBackoffCeiling; i++ {
		wait *= 2
	}
	return min(wait, reminderBackoffCeiling)
}

// reminderDue decides whether a reminder for the unread ids goes out at now, and at which step.
// An unread set that differs from the last reminder's is reminded at once; an unchanged one waits out the backoff.
func reminderDue(last reminderEntry, hasLast bool, unread []int64, now time.Time) (due bool, step int) {
	if !hasLast || !slices.Equal(last.Unread, unread) {
		return true, 0
	}
	return !now.Before(last.At.Add(reminderBackoff(last.Step))), last.Step + 1
}

// composeReminder renders the reminder line for the unread messages, oldest first.
// The line names each message by id and sender and carries no sender-written text, so it cannot pose as an instruction.
// Under a hold, holdOwner names the one sender whose mail may be read,
// and the command in the line reads only that sender's mail.
func composeReminder(unread []Message, holdOwner string) string {
	var listed []string
	for i, msg := range unread {
		if i == maxReminderMessages {
			break
		}
		entry := fmt.Sprintf("#%d from \"%s\"", msg.ID, msg.From)
		if stamp := msg.HubStamp(); stamp != "" {
			entry += " " + stamp
		}
		listed = append(listed, entry)
	}
	if rest := len(unread) - len(listed); rest > 0 {
		listed = append(listed, fmt.Sprintf("+%d more", rest))
	}
	command := "lym creel read"
	if holdOwner != "" {
		command += " --from " + holdOwner
	}
	return fmt.Sprintf("[creel] %d unread: %s — run %s", len(unread), strings.Join(listed, ", "), command)
}
