// receive.go implements the receiving half of mail across hubs: the deliver and confirm verbs.
// It also asks the sending hub's daemon to confirm mail, stamps trust, and drops unconfirmed mail.

package creelengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// The verbs the daemons of two hubs call on each other.
// Neither resolves a sender: the calling daemon runs in no pane, and the confirmation alone settles who sent.
const (
	// VerbDeliver offers a receiving daemon one message from a strand of another hub.
	VerbDeliver = "deliver"
	// VerbConfirm asks a sending daemon whether it posted the message the tuple names.
	VerbConfirm = "confirm"
)

// The answers a deliver gives, and the words a confirm answers with.
const (
	deliveryAccepted = "accepted"
	deliveryRefused  = "refused"
	confirmedYes     = "yes"
	confirmedNo      = "no"
)

const (
	// confirmTimeout bounds one confirm to the sending hub's daemon, on the store's clock.
	confirmTimeout = 10 * time.Second
	// dropWindowLength is how long a hub's drops share one window of notices.
	dropWindowLength = time.Hour
	// dropNoticesPerWindow is how many drops from one hub post a notice to the orch in one window.
	dropNoticesPerWindow = 5
	// dropReason is the journaled reason of a message the sending hub's daemon did not confirm.
	dropReason = "the sending hub's daemon did not confirm that its strand posted it"
)

// HubMail is the mail of one deliver or confirm request.
// A confirm carries no body, only the body hash.
type HubMail struct {
	// SendingID is the id the sending hub gave the message.
	SendingID int64 `json:"sending_id"`
	// Sender and Receiver are the full names of the strand that posted the message and the strand it is for.
	Sender   string `json:"sender"`
	Receiver string `json:"receiver"`
	// ReplyTo is the id on the receiving hub the message answers, 0 for mail.
	ReplyTo int64 `json:"reply_to,omitempty"`
	// ExpectReply marks a message whose sender expects a reply.
	ExpectReply bool `json:"expect_reply,omitempty"`
	// Posted is when the sending hub posted the message.
	Posted time.Time `json:"posted"`
	// Body is the message body, and BodyHash its SHA-256 in hex.
	Body     string `json:"body,omitempty"`
	BodyHash string `json:"body_hash,omitempty"`
}

// DeliverAnswer is a receiving daemon's answer to a deliver:
// accepted with the id it gave the message, or refused with the reason.
type DeliverAnswer struct {
	Delivery string
	Reason   string
	ID       int64
}

// bodyHash returns the SHA-256 of body in hex.
func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// dropWindow is the notices posted for the drops of one sending hub in the current hour.
type dropWindow struct {
	start time.Time
	// notices counts the notices this window posted, and undisclosed the drops journaled since the last notice.
	notices     int
	undisclosed int
}

// serveHubCall answers a deliver or confirm before any sender is resolved.
// A request with no mail, and a daemon told no HubDirectory, are refused.
func (d *daemon) serveHubCall(conn net.Conn, req Request) error {
	if req.Mail == nil {
		return fmt.Errorf("creel: a %s request carries no mail", req.Verb)
	}
	if d.opts.Hubs == nil {
		return fmt.Errorf("creel: this daemon serves no other hub, so it cannot take a %s", req.Verb)
	}
	if req.Verb == VerbConfirm {
		answer := confirmedNo
		if d.store.Confirm(*req.Mail) {
			answer = confirmedYes
		}
		return d.write(conn, Response{OK: true, Confirmed: answer})
	}
	answer, err := d.store.Deliver(*req.Mail)
	if err != nil {
		return err
	}
	return d.write(conn, Response{OK: true, Delivery: answer.Delivery, Reason: answer.Reason, ID: answer.ID})
}

// Confirm reports whether this hub's journal holds a pending outbound message that the whole tuple of mail names.
// The body is compared by its hash, since a confirm carries no body.
func (s *Store) Confirm(mail HubMail) bool {
	confirmed := false
	_ = s.do(func() error {
		sent := s.state.outbound[mail.SendingID]
		confirmed = sent != nil && sent.State == outboundPending && sent.From == mail.Sender && sent.To == mail.Receiver &&
			sent.ReplyTo == mail.ReplyTo && sent.ExpectReply == mail.ExpectReply && sent.Posted.Equal(mail.Posted) && bodyHash(sent.Body) == mail.BodyHash
		return nil
	})
	return confirmed
}

// Deliver takes one message from a strand of another hub.
// Nothing reaches an agent until the sending hub's own daemon confirms the whole tuple.
// Mail it does not confirm is journaled as a drop.
// A refusal is an answer; an error is transient, so the sending daemon retries.
func (s *Store) Deliver(mail HubMail) (DeliverAnswer, error) {
	mail.BodyHash = bodyHash(mail.Body)
	senderName, reason := s.refuseDeliveredNames(mail)
	if reason != "" {
		return DeliverAnswer{Delivery: deliveryRefused, Reason: reason}, nil
	}
	sendingHub, err := s.hubs.Lookup(senderName.Shortname)
	if errors.Is(err, ErrHubUnresolved) {
		return DeliverAnswer{Delivery: deliveryRefused, Reason: err.Error()}, nil
	}
	if err != nil {
		return DeliverAnswer{}, err
	}
	strands, strandsErr := s.readStrands()

	var answer *DeliverAnswer
	err = s.do(func() error {
		held, clash := s.heldLocked(sendingHub, mail)
		switch {
		case held != nil:
			answer = &DeliverAnswer{Delivery: deliveryAccepted, ID: held.ID}
		case clash != "":
			answer = &DeliverAnswer{Delivery: deliveryRefused, Reason: clash}
		case strandsErr != nil:
			return strandsErr
		default:
			if err := s.checkReceiverLocked(strands, mail.Receiver); err != nil {
				answer = &DeliverAnswer{Delivery: deliveryRefused, Reason: err.Error()}
			} else if mail.ReplyTo != 0 {
				if sent := s.state.outbound[mail.ReplyTo]; sent == nil || sent.ReceivingHub != sendingHub || sent.From != mail.Receiver {
					answer = &DeliverAnswer{Delivery: deliveryRefused, Reason: fmt.Sprintf("creel: the reply names message #%d, which %s never sent to the sending hub or which is no longer kept; way forward: post anew with lym creel post %s \"<text>\"", mail.ReplyTo, mail.Receiver, mail.Sender)}
				}
			}
		}
		return nil
	})
	if err != nil {
		return DeliverAnswer{}, err
	}
	if answer != nil {
		return *answer, nil
	}

	confirmed, err := s.confirmWithSender(sendingHub, mail)
	if err != nil {
		return DeliverAnswer{}, err
	}
	var repository string
	if confirmed {
		repository = s.sendingRepository(sendingHub)
	}
	trusted := confirmed && s.trustedSender(mail.Sender, repository)
	err = s.do(func() error {
		held, clash := s.heldLocked(sendingHub, mail)
		switch {
		case confirmed && clash != "":
			answer = &DeliverAnswer{Delivery: deliveryRefused, Reason: clash}
		case held != nil:
			answer = &DeliverAnswer{Delivery: deliveryAccepted, ID: held.ID}
		case confirmed:
			id, err := s.storeInboundLocked(sendingHub, repository, mail, trusted)
			if err != nil {
				return err
			}
			answer = &DeliverAnswer{Delivery: deliveryAccepted, ID: id}
		default:
			if err := s.dropLocked(senderName.Shortname, sendingHub, mail, dropReason); err != nil {
				return err
			}
			answer = &DeliverAnswer{Delivery: deliveryRefused, Reason: dropReason}
		}
		return nil
	})
	if err != nil {
		return DeliverAnswer{}, err
	}
	return *answer, nil
}

// refuseDeliveredNames checks the names and the line of a deliver.
// It answers the sender's parsed name, or the reason for a refusal.
func (s *Store) refuseDeliveredNames(mail HubMail) (agentname.Name, string) {
	senderName, err := agentname.Parse(mail.Sender)
	if err != nil {
		return agentname.Name{}, fmt.Sprintf("creel: the sender %q is no strand name; way forward: post from a strand of another hub", mail.Sender)
	}
	if senderName.Shortname == s.geom.Shortname {
		return agentname.Name{}, fmt.Sprintf("creel: the sender %q is a strand of this hub, and only a strand of another hub delivers mail across hubs", mail.Sender)
	}
	if _, err := agentname.Parse(mail.Receiver); err != nil {
		return agentname.Name{}, fmt.Sprintf("creel: the receiver %q is no strand name; way forward: post to a strand by its full name", mail.Receiver)
	}
	if err := validateStampedLine(mail.Sender, mail.Body); err != nil {
		return agentname.Name{}, err.Error()
	}
	return senderName, ""
}

// heldLocked finds the message this hub holds from the hub at sendingHub under mail's sending id.
// It returns the held message when the other fields of mail match:
// sender, receiver, reply-to id, expect-reply flag, post time to the nanosecond, and body hash.
// Any difference returns the clash reason, which names the ids this hub holds from that hub.
func (s *Store) heldLocked(sendingHub string, mail HubMail) (held *Message, clash string) {
	var (
		found    *Message
		low, top int64
	)
	for _, msg := range s.state.messages {
		if msg.SendingHub != sendingHub {
			continue
		}
		if msg.SendingID == mail.SendingID {
			found = msg
		}
		if low == 0 || msg.SendingID < low {
			low = msg.SendingID
		}
		top = max(top, msg.SendingID)
	}
	if found == nil {
		return nil, ""
	}
	if found.From == mail.Sender && found.DeliveredTo == mail.Receiver && found.ReplyTo == mail.ReplyTo && found.ExpectReply == mail.ExpectReply &&
		found.RemotePosted.Equal(mail.Posted) && bodyHash(found.Body) == mail.BodyHash {
		return found, ""
	}
	return nil, fmt.Sprintf("creel: id clash: this hub holds ids %d-%d from that hub; a post succeeds once its ids pass %d or those messages pass the seven-day retention", low, top, top)
}

// confirmWithSender asks the daemon of the sending hub whether its strand posted the message.
// It does not start that daemon.
// A confirm that fails, times out or answers neither yes nor no is an error, so the sender retries.
func (s *Store) confirmWithSender(sendingHub string, mail HubMail) (bool, error) {
	geometry, err := s.hubs.Geometry(sendingHub)
	if err != nil {
		return false, fmt.Errorf("creel: cannot reach the sending hub to confirm the message: %w", err)
	}
	asked := mail
	asked.Body = ""
	resp, err := NewClient(geometry, Claim{}, nil, s.clock).Call(Request{Verb: VerbConfirm, Mail: &asked}, confirmTimeout, nil)
	if err != nil {
		return false, fmt.Errorf("creel: the sending hub's daemon did not answer the confirm: %w", err)
	}
	switch resp.Confirmed {
	case confirmedYes:
		return true, nil
	case confirmedNo:
		return false, nil
	}
	return false, fmt.Errorf("creel: the sending hub's daemon answered the confirm with %q, neither yes nor no", resp.Confirmed)
}

// sendingRepository answers the repository name of the hub at sendingHub, or "" when it has none.
// A hub with no repository name, such as one with no origin remote, is ordinary, so the reason is logged at Debug.
func (s *Store) sendingRepository(sendingHub string) string {
	repository, err := s.hubs.RepositoryName(sendingHub)
	if err != nil {
		logger.Debug("creel: the sending hub has no repository name, so its mail is stored untrusted", "hub", sendingHub, "error", err)
		return ""
	}
	return repository
}

// trustedSender reports whether sender is the orch of a hub whose repository this hub's creel.yaml lists.
// repository is the sending hub's repository name, and an empty one is never trusted.
// It answers only on Linux, where the sending hub's identity rests on the kernel.
// A trusted-hubs read that fails logs one warning and counts as untrusted.
func (s *Store) trustedSender(sender, repository string) bool {
	name, err := agentname.Parse(sender)
	if err != nil || name.Role != agentname.RoleOrch || s.goos != "linux" || repository == "" {
		return false
	}
	listed, err := s.hubs.TrustedHubs()
	if err != nil {
		logger.Warn("creel: could not read the trusted hubs, so mail from another hub is stored untrusted", "error", err)
		return false
	}
	return ListsRepository(listed, repository)
}

// storeInboundLocked journals mail from the hub at sendingHub as unread, stamped with its trust and its repository name.
// It then delivers the mail to its receiver.
// A reply also ends a wait on the outbound message it answers, and still reaches the receiver as mail.
func (s *Store) storeInboundLocked(sendingHub, sendingRepository string, mail HubMail, trusted bool) (int64, error) {
	kind := kindMail
	if mail.ReplyTo != 0 {
		kind = kindReply
	}
	id := s.state.newID()
	if err := s.writer.append(record{
		Event: eventInbound, ID: id, From: mail.Sender, To: mail.Receiver, Kind: kind, Body: mail.Body, ReplyTo: mail.ReplyTo, ExpectReply: mail.ExpectReply,
		SendingHub: sendingHub, SendingRepository: sendingRepository, SendingID: mail.SendingID, Trusted: trusted, RemotePosted: mail.Posted, DeliveredTo: mail.Receiver,
	}); err != nil {
		return 0, err
	}
	if mail.ReplyTo != 0 {
		s.answerLocked(waitKey{kind: waitReply, id: mail.ReplyTo}, Outcome{Kind: OutcomeReply, Message: *s.state.messages[id]})
	}
	return id, s.deliverMailLocked(id, mail.Receiver)
}

// dropLocked journals mail that was never delivered to any agent.
// It tells this hub's orch of the drop within the hub's notice window.
// A journal write that fails returns its error and posts no notice, so the drop is journaled on the sender's retry.
func (s *Store) dropLocked(shortname, sendingHub string, mail HubMail, reason string) error {
	if err := s.writer.append(record{
		Event: eventDropped, ID: s.state.newID(), From: mail.Sender, To: mail.Receiver,
		SendingHub: sendingHub, SendingShortname: shortname, Reason: reason,
	}); err != nil {
		return err
	}
	s.noticeDropLocked(shortname, sendingHub, mail, reason)
	return nil
}

// noticeDropLocked posts the notice of a drop to this hub's orch while the hub's window holds notices.
// A notice that fails to store is logged and spends no notice slot, so the next notice counts its drops.
// The window is kept in memory per sending hub path, so a flood of claimed strands from one hub still costs one entry.
func (s *Store) noticeDropLocked(shortname, sendingHub string, mail HubMail, reason string) {
	now := s.clock.Now()
	window := s.dropWindows[sendingHub]
	if window == nil || now.Sub(window.start) >= dropWindowLength {
		carried := 0
		if window != nil {
			carried = window.undisclosed
		}
		window = &dropWindow{start: now, undisclosed: carried}
		s.dropWindows[sendingHub] = window
	}
	window.undisclosed++
	if window.notices >= dropNoticesPerWindow {
		return
	}
	noticesBefore := window.notices
	window.notices++
	counted := window.undisclosed
	window.undisclosed = 0
	body := fmt.Sprintf("Mail from another hub was dropped and reached no agent: the hub %s (%s) claimed the strand %s and addressed %s; reason: %s; see: lym creel log --from %s",
		shortname, sendingHub, mail.Sender, mail.Receiver, reason, mail.Sender)
	if counted > 1 {
		body += fmt.Sprintf("; %d drops from that hub were journaled since the last notice", counted)
	}
	if window.notices == dropNoticesPerWindow {
		body += "; further drops from that hub this hour are journaled without a notice, listed by lym creel log --since 1h"
	}
	if err := s.postDropNoticeLocked(body); err != nil {
		// Restored so the slot is not spent and the next notice counts these drops too.
		window.notices = noticesBefore
		window.undisclosed = counted
		logger.Warn("creel: could not post the notice of a dropped message", "error", err)
	}
}

// postDropNoticeLocked posts body as a CREEL notice to this hub's orch.
func (s *Store) postDropNoticeLocked(body string) error {
	orch, err := agentname.Format(s.geom.Shortname, "", agentname.RoleOrch)
	if err != nil {
		return err
	}
	sender, err := s.chaserSender()
	if err != nil {
		return err
	}
	id, err := s.appendPostedLocked(sender, record{To: orch, Kind: kindMail, Body: body})
	if err != nil {
		return err
	}
	return s.deliverMailLocked(id, orch)
}
