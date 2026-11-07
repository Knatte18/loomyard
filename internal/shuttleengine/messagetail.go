// messagetail.go declares the fixed tail a sender appends to a message typed into a run's pane.
// It also holds WithMessageTail, which appends the tail.

package shuttleengine

import "strings"

// MessageTail is the tail a sender writes to end a message typed into a run's pane.
// No creel post appends it unless the sender writes it.
// It tells the receiving agent the message is an interjection to answer, not a new task.
const MessageTail = "Answer briefly, then continue your task."

// WithMessageTail returns text with MessageTail appended after one space.
// A text that already ends with MessageTail, ignoring trailing whitespace, is returned unchanged,
// so the tail goes out once.
func WithMessageTail(text string) string {
	if strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), MessageTail) {
		return text
	}
	return text + " " + MessageTail
}
