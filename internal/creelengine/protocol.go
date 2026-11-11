// protocol.go declares the JSON-lines protocol between a client and the daemon.
// It holds the verbs, the Request and Response of one connection, and the line framing.

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
	// VerbWaitReply opens a fresh connection that waits for the reply to a message the client posted.
	VerbWaitReply = "wait-reply"
	VerbReply     = "reply"
	VerbRead      = "read"
	VerbInbox     = "inbox"
	VerbForward   = "forward"
	VerbLog       = "log"
	VerbInject    = "inject"
	// VerbWaitInject opens a fresh connection that waits for the outcome of an inject the client queued.
	VerbWaitInject = "wait-inject"
	VerbInterrupt  = "interrupt"
	VerbHold       = "hold"
	VerbRelease    = "release"
	// VerbWithdraw removes a program's own queued injects and unread mail to one receiver; only the Go client sends it.
	VerbWithdraw = "withdraw"
	// VerbClaim hands a strand's tool-boundary hook its unread mail, which the client acknowledges on the same connection.
	VerbClaim = "claim"
	// VerbHookWait holds a strand's idle waiter until the daemon rings it or a newer waiter supersedes it.
	// A ring to a waiter whose request set takes_mail carries the claimed messages, which the client acknowledges.
	VerbHookWait = "hook-wait"
)

// CodeStrandClaimOutsidePane is the Response.Code of a refused strand claim for a listed strand, from no strand pane.
const CodeStrandClaimOutsidePane = "strand-claim-outside-pane"

// CodeProgramClaimInStrandPane is the Response.Code of a program claim refused from a non-orch strand's pane.
const CodeProgramClaimInStrandPane = "program-claim-in-strand-pane"

// maxRequestBytes bounds one request line: a 64 KiB body whose every byte is escaped fits.
const maxRequestBytes = 1 << 20

// Request is the one line a client sends on a connection.
// Which fields a verb reads is part of the verb: the daemon ignores the rest.
type Request struct {
	Verb  string `json:"verb"`
	Claim Claim  `json:"claim"`
	// Receiver is the receiver of post, forward, inject, interrupt, hold, release and withdraw, and the To filter of log.
	Receiver string `json:"receiver,omitempty"`
	// Text is the body of post and reply, and the text of inject.
	Text string `json:"text,omitempty"`
	// Command and Argument make inject a command inject; Text is then empty.
	Command  CommandName `json:"command,omitempty"`
	Argument string      `json:"argument,omitempty"`
	// ID is the message or inject a verb concerns: a reply or forward target, a wait's id, or the log id filter.
	ID int64 `json:"id,omitempty"`
	// From is the sender filter of read and log.
	From string `json:"from,omitempty"`
	// Wait makes the verb wait: post for its reply, inject for its outcome, read for unread mail, hook-wait for a ring.
	Wait bool `json:"wait,omitempty"`
	// Interrupt makes inject and post interrupt the receiver's running turn first.
	Interrupt bool `json:"interrupt,omitempty"`
	// ExpectReply marks a post as expecting a reply, which Wait implies.
	ExpectReply bool `json:"expect_reply,omitempty"`
	// TTLSeconds is the TTL of an inject, 0 for the default.
	TTLSeconds int64 `json:"ttl_s,omitempty"`
	// ForSeconds is how long a hold lasts.
	ForSeconds int64 `json:"for_s,omitempty"`
	// Since is the lower time bound of log.
	Since time.Time `json:"since,omitzero"`
	// Stop marks a claim as the turn-end Stop hook's; absent, the claim is a tool boundary's.
	Stop bool `json:"stop,omitempty"`
	// StopHookActive is the Stop hook input's stop_hook_active, read by the claim verb alone.
	StopHookActive bool `json:"stop_hook_active,omitempty"`
	// TakesMail marks a hook-wait request's waiter as taking mail: its ring carries the claimed messages.
	// The client acknowledges them; a daemon that predates the key ignores it and rings with no mail.
	TakesMail bool `json:"takes_mail,omitempty"`
	// Mail is the message a deliver offers and the tuple a confirm asks about; no other verb reads it.
	Mail *HubMail `json:"mail,omitempty"`
}

// Response is what the daemon answers on a connection; every one carries the hub path.
// A waiting verb first gets a Pending response carrying the id it waits on, and its outcome as a later response.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Code names the kind of a refusal a Go caller tells apart, empty for every other refusal.
	Code    string `json:"code,omitempty"`
	HubPath string `json:"hub_path"`
	// Pending marks the interim response of a waiting verb: the request is accepted and the outcome follows.
	Pending bool `json:"pending,omitempty"`
	// Progress marks an interim response.
	// An inject's carries the Stall its receiver's delivery waits on, read at StallAt.
	// A waiting post's carries its Warning.
	Progress bool `json:"progress,omitempty"`
	// Stall and StallAt also ride the expired answer of an inject, as the stall that stood when its TTL ran out.
	Stall   Stall     `json:"stall,omitempty"`
	StallAt time.Time `json:"stall_at,omitzero"`
	// Sender is the sender the daemon resolved for the request.
	Sender string `json:"sender,omitempty"`
	// ID is the id of the message or inject the verb created or waits on.
	ID int64 `json:"id,omitempty"`
	// Messages are what a read, a claim or a marked hook ring hands out; AckRequired asks the client to ack them.
	Messages []Message `json:"messages,omitempty"`
	// Echo holds ring-mail lines a claim shows the operator before its messages; AckRequired covers them too.
	Echo        []string     `json:"echo,omitempty"`
	AckRequired bool         `json:"ack_required,omitempty"`
	Inbox       *InboxResult `json:"inbox,omitempty"`
	Log         []LogEntry   `json:"log,omitempty"`
	// Outcome is how a waited-on thing settled, with the reply or returned message in Message.
	Outcome OutcomeKind `json:"outcome,omitempty"`
	Message *Message    `json:"message,omitempty"`
	// Sent says an interrupt stopped a running turn, and Interrupted that an inject or a post did.
	Sent        bool `json:"sent,omitempty"`
	Interrupted bool `json:"interrupted,omitempty"`
	// ReceiverLive answers a post whether its receiver had a live pane, and is absent when creel cannot tell.
	ReceiverLive *bool `json:"receiver_live,omitempty"`
	// Warning names a post's receiver that is not live and the command that starts it.
	// A waiting post carries it in a progress response right after its pending one.
	Warning string `json:"warning,omitempty"`
	// Hold is the hold a hold verb set; Released says a release ended one.
	Hold     *HoldResult `json:"hold,omitempty"`
	Released bool        `json:"released,omitempty"`
	// Withdrawn is what a withdraw removed.
	Withdrawn *WithdrawResult `json:"withdrawn,omitempty"`
	// Delivery is a deliver's answer, accepted or refused; Reason is the refusal's cause, ID the receiver's id.
	Delivery string `json:"delivery,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Confirmed is a confirm's answer, yes or no.
	Confirmed string `json:"confirmed,omitempty"`
}

// ackLine is the client's acknowledgement that it wrote the messages a read, a claim or a marked hook ring handed out.
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
