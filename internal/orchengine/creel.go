// creel.go — the seam between the watcher and creel, the hub's message and inject daemon, and the plain values it exchanges.
//
// The watcher addresses creel by receiver name and reads nothing of creel's own types, so this package imports no creel package (Told-Geometry Invariant).
// The production implementation lives in orchcli and the tests use a recording fake.

package orchengine

import "time"

// Creel is the watcher's view of creel.
// Every call signs as the watcher's own program and addresses the receiver by its full strand name.
type Creel interface {
	// Post leaves body as mail for receiver and returns the message's id.
	Post(receiver, body string) (int64, error)
	// ReadMailbox hands every unread message of the watcher's mailbox to handle without waiting; they are marked read only once handle returns nil.
	// With nothing unread handle is not called.
	ReadMailbox(handle func([]CreelMessage) error) error
	// Inject queues step to be typed into receiver at its next idle and returns the inject's id; a zero ttl takes creel's default.
	Inject(receiver string, step CreelStep, ttl time.Duration) (int64, error)
	// InjectOutcome returns where the inject id stands.
	InjectOutcome(id int64) (CreelOutcome, error)
	// Sent returns the posts and injects the watcher sent to receiver at or after since, oldest first.
	Sent(receiver string, since time.Time) ([]CreelSent, error)
	// Hold holds receiver's deliveries to the watcher's own for duration.
	Hold(receiver string, duration time.Duration) error
	// Release ends the watcher's hold on receiver; a hold already gone is not an error.
	Release(receiver string) error
	// Withdraw removes the watcher's own queued injects to receiver and its own unread mail held by receiver, and answers the mail it removed.
	// It acts on receiver by name whether or not creel still lists that strand.
	Withdraw(receiver string) ([]CreelWithdrawn, error)
}

// CreelMessage is one message of the watcher's mailbox.
type CreelMessage struct {
	ID int64
	// From is the sender's full name.
	From string
	// ReplyTo is the id of the message this one answers, 0 for mail.
	ReplyTo int64
	Body    string
}

// CreelStep is what an inject types: a command by its name and argument, or free text when Command is empty.
type CreelStep struct {
	Command  string
	Argument string
	Text     string
}

// CreelState is where an inject stands.
type CreelState string

// The states of an inject.
const (
	// CreelQueued means the inject waits for the receiver's next idle.
	CreelQueued CreelState = "queued"
	// CreelSending means creel is typing the inject.
	CreelSending CreelState = "sending"
	// CreelInjected means the inject was typed and the session took it as a turn.
	CreelInjected CreelState = "injected"
	// CreelUnconfirmed means the inject was typed but the session was not seen to take it.
	CreelUnconfirmed CreelState = "unconfirmed"
	// CreelExpired means the inject's time to live ran out before it was typed.
	CreelExpired CreelState = "expired"
	// CreelWithdrawnState means the inject was withdrawn by its sender before it was typed.
	CreelWithdrawnState CreelState = "withdrawn"
)

// CreelOutcome is where one inject stands, as creel's journal records it.
type CreelOutcome struct {
	State CreelState
	// SendingAt is the time of the inject's newest sending record, zero while it has none.
	// Creel journals one for every typing attempt, so the newest is the attempt that was typed.
	SendingAt time.Time
	// Stall is the reason an expired inject names for the stall it ended in, empty when it names none.
	Stall string
}

// CreelSent is one post or inject the watcher sent.
type CreelSent struct {
	ID int64
	At time.Time
	// Mail is true for a post and false for an inject.
	Mail bool
	// Step is the command and argument of a command inject, empty otherwise.
	Step CreelStep
	// Subject is a post's first line, or a text inject's text.
	Subject string
}

// CreelWithdrawn is one message a withdraw returned undelivered.
type CreelWithdrawn struct {
	ID      int64
	Subject string
}
