// reminder.go composes the lines typed to a receiver with unread mail, a program's notice and the reminder of the rest, and decides when the next reminder is due.

package creelengine

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	// reminderBackoffBase is how long a receiver that ended a reminder turn with the same unread set waits for its next reminder.
	reminderBackoffBase = 5 * time.Minute
	// reminderBackoffCeiling is the longest the reminder backoff grows to.
	reminderBackoffCeiling = time.Hour
	// maxReminderMessages is how many unread messages a reminder lists; the rest are counted.
	maxReminderMessages = 5
	// maxNoticeBytes is the longest body a program's message may have and still be typed as a notice line.
	maxNoticeBytes = 2048
)

// isNotice reports whether msg is typed into its receiver's pane as a notice line instead of being reminded: a program's message, which takes no reply, whose body is one line without control characters and at most maxNoticeBytes long.
// A program's name is accepted only from an orch pane or the Go client, so its text cannot be an agent posing as an instruction.
func isNotice(msg *Message) bool {
	return isProgramMailbox(msg.From) && len(msg.Body) <= maxNoticeBytes && strings.IndexFunc(msg.Body, unicode.IsControl) < 0
}

// composeNotice renders the line that carries a notice: its sender and its body.
func composeNotice(msg Message) string {
	return fmt.Sprintf("[creel] %s: %s", msg.From, msg.Body)
}

// reminderBackoff returns the wait after the reminder at the given step of an unchanged unread set: 5 minutes, doubling to the 1-hour ceiling.
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
// Under a hold, holdOwner names the one sender whose mail may be read, and the command it tells the receiver to run reads only that sender's mail.
func composeReminder(unread []Message, holdOwner string) string {
	var listed []string
	for i, msg := range unread {
		if i == maxReminderMessages {
			break
		}
		listed = append(listed, fmt.Sprintf("#%d from %s", msg.ID, msg.From))
	}
	if rest := len(unread) - len(listed); rest > 0 {
		listed = append(listed, fmt.Sprintf("+%d more", rest))
	}
	command := "lyx creel read"
	if holdOwner != "" {
		command += " --from " + holdOwner
	}
	return fmt.Sprintf("[creel] %d unread: %s — run %s", len(unread), strings.Join(listed, ", "), command)
}
