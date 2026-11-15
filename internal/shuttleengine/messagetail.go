// messagetail.go declares the fixed tail a sender appends to a message typed into a run's pane.

package shuttleengine

// MessageTail is the tail a sender writes to end a message typed into a run's pane.
// No creel post appends it unless the sender writes it.
// It tells the receiving agent the message is an interjection to answer, not a new task.
const MessageTail = "Answer briefly, then continue your task."
