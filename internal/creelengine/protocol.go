// protocol.go declares the JSON-lines protocol between a client and the daemon: the verbs, the Request and Response of one connection, and the line framing.

package creelengine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The verbs a Request names.
const (
	VerbPost = "post"
	// VerbWaitReply waits again, on a fresh connection, for the reply to a message the client posted.
	VerbWaitReply = "wait-reply"
	VerbReply     = "reply"
	VerbRead      = "read"
	VerbInbox     = "inbox"
	VerbForward   = "forward"
	VerbLog       = "log"
	VerbInject    = "inject"
	// VerbWaitInject waits again, on a fresh connection, for the outcome of an inject the client queued.
	VerbWaitInject = "wait-inject"
	VerbInterrupt  = "interrupt"
	VerbHold       = "hold"
	VerbRelease    = "release"
)

// maxRequestBytes bounds one request line: a 64 KiB body whose every byte is escaped fits.
const maxRequestBytes = 1 << 20

// Request is the one line a client sends on a connection.
// Which fields a verb reads is part of the verb: the daemon ignores the rest.
type Request struct {
	Verb  string `json:"verb"`
	Claim Claim  `json:"claim"`
	// Receiver is the receiver of post, forward, inject, interrupt, hold and release, and the To filter of log.
	Receiver string `json:"receiver,omitempty"`
	// Text is the body of post and reply, and the text of inject.
	Text string `json:"text,omitempty"`
	// ID is the message or inject a verb concerns: the message to reply to or forward, the id a wait resumes, the id filter of log.
	ID int64 `json:"id,omitempty"`
	// From is the sender filter of read and log.
	From string `json:"from,omitempty"`
	// All makes read include the read messages still retained.
	All bool `json:"all,omitempty"`
	// Wait makes post wait for its reply, inject wait for its outcome, and read wait for unread mail.
	Wait bool `json:"wait,omitempty"`
	// Interrupt makes inject interrupt the receiver's running turn first.
	Interrupt bool `json:"interrupt,omitempty"`
	// TTLSeconds is the TTL of an inject, 0 for the default.
	TTLSeconds int64 `json:"ttl_s,omitempty"`
	// ForSeconds is how long a hold lasts.
	ForSeconds int64 `json:"for_s,omitempty"`
	// Since is the lower time bound of log.
	Since time.Time `json:"since,omitzero"`
}

// Response is what the daemon answers on a connection; every one carries the hub path.
// A waiting verb first gets a Pending response carrying the id it waits on, and its outcome as a later response.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	HubPath string `json:"hub_path"`
	// Pending marks the interim response of a waiting verb: the request is accepted and the outcome follows.
	Pending bool `json:"pending,omitempty"`
	// Sender is the sender the daemon resolved for the request.
	Sender string `json:"sender,omitempty"`
	// ID is the id of the message or inject the verb created or waits on.
	ID int64 `json:"id,omitempty"`
	// Messages are the messages a read hands out; AckRequired says the client must ack them on the same connection.
	Messages    []Message    `json:"messages,omitempty"`
	AckRequired bool         `json:"ack_required,omitempty"`
	Inbox       *InboxResult `json:"inbox,omitempty"`
	Log         []LogEntry   `json:"log,omitempty"`
	// Outcome is how a waited-on thing settled, with the reply or returned message in Message.
	Outcome OutcomeKind `json:"outcome,omitempty"`
	Message *Message    `json:"message,omitempty"`
	// Sent says an interrupt stopped a running turn, and Interrupted that an inject did.
	Sent        bool `json:"sent,omitempty"`
	Interrupted bool `json:"interrupted,omitempty"`
	// Hold is the hold a hold verb set; Released says a release ended one.
	Hold     *HoldResult `json:"hold,omitempty"`
	Released bool        `json:"released,omitempty"`
}

// ackLine is the client's acknowledgement that it wrote the messages of a read.
type ackLine struct {
	Ack bool `json:"ack"`
}

// ErrDaemonRunning means another daemon holds the hub.
var ErrDaemonRunning = errors.New("creel: another daemon holds this hub")

// ErrWaitTimeout means a waiting call's timeout passed with no outcome.
var ErrWaitTimeout = errors.New("creel: no outcome arrived before the timeout")

// readLine reads one line of at most max bytes, without its newline.
func readLine(reader *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > max+1 {
			return nil, fmt.Errorf("creel: a line over the %d-byte limit", max)
		}
		if err == nil {
			return line[:len(line)-1], nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// encodeLine renders v as one JSON line.
func encodeLine(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
