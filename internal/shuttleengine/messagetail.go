// messagetail.go declares the fixed tail every operator or agent message sent through
// `lyx shuttle send` ends with, and the function that appends it.

package shuttleengine

import "strings"

// MessageTail ends every message `lyx shuttle send` types into a run's pane.
// It tells the receiving agent the message is an interjection to answer, not a new task.
const MessageTail = "Answer briefly, then continue your task."

// WithMessageTail returns text with MessageTail appended after one space.
// A text that, trimmed of trailing whitespace, already ends with MessageTail is returned unchanged,
// so the tail goes out once.
func WithMessageTail(text string) string {
	if strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), MessageTail) {
		return text
	}
	return text + " " + MessageTail
}
