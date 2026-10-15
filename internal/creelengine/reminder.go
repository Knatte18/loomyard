// reminder.go composes the one-line reminder typed to a receiver with unread mail and decides when the next one is due.

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
	// maxReminderSubjectRunes is where a subject is cut in a reminder.
	maxReminderSubjectRunes = 80
)

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
// Each subject sits in « » as the sender's words and has control and delimiter characters replaced, so it cannot pose as an instruction.
func composeReminder(unread []Message) string {
	var listed []string
	for i, msg := range unread {
		if i == maxReminderMessages {
			break
		}
		listed = append(listed, fmt.Sprintf("#%d from %s «%s»", msg.ID, msg.From, reminderSubject(msg.Subject)))
	}
	if rest := len(unread) - len(listed); rest > 0 {
		listed = append(listed, fmt.Sprintf("+%d more", rest))
	}
	return fmt.Sprintf("[creel] %d unread: %s — run lyx creel read", len(unread), strings.Join(listed, ", "))
}

// reminderSubject replaces control characters and the « » delimiters with spaces, then cuts the subject to its rune limit.
func reminderSubject(subject string) string {
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '«' || r == '»' {
			return ' '
		}
		return r
	}, subject))
	if len(runes) > maxReminderSubjectRunes {
		runes = runes[:maxReminderSubjectRunes]
	}
	return string(runes)
}
