// machineinbound.go implements the receiving half of mail across machines: the relay's deliver verb.
// It checks the sender and the receiver.
// It answers a repeated delivery with the id it already gave.
// It stores the mail as unread, stamped with its sending machine.

package creelengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/machinename"
)

// VerbMachineDeliver offers creel one message from an orch of another machine; only the relay's program sends it.
const VerbMachineDeliver = "machine-deliver"

// MachineInboundRetention is how long creel recognizes a delivery it already stored.
// It is the journal's retention, so the relay never skips a message within that window.
const MachineInboundRetention = journalRetention

// MachineMail is the mail of one machine-deliver request.
type MachineMail struct {
	// Sender is the full <machine>::<name> address of the orch that posted the message, as the envelope names it.
	Sender string `json:"sender"`
	// Receiver is the full name of the orch of this hub the message is for.
	Receiver string `json:"receiver"`
	// SendingID is the id the sending machine gave the message.
	SendingID int64 `json:"sending_id"`
	// ReplyTo is the id of the message of this hub the message answers, 0 for mail.
	ReplyTo int64  `json:"reply_to,omitempty"`
	Body    string `json:"body"`
	// Posted is when the sending machine posted the message.
	Posted time.Time `json:"posted"`
}

// DeliverMachine stores one message from an orch of another machine as unread mail for an orch of this hub.
// It answers the id the message has here.
// A message the sender and sending id already delivered is answered with its id and stored again never.
// It refuses a sender other than the relay's program.
// It refuses a sender address that does not parse or names this machine.
// It refuses a receiver that is no orch of this hub, an empty body and a line over the cap.
// A reply-to must name a message this hub sent to the sender address from the receiver.
// A reply then ends the wait on that message.
// The mail is stamped trusted when the sender is an orch.
func (s *Store) DeliverMachine(sender Sender, mail MachineMail) (int64, error) {
	if err := s.requireBridgeProgram(sender, "deliver"); err != nil {
		return 0, err
	}
	bridge := s.bridge
	if bridge == nil {
		return 0, errNoBridge()
	}
	current, former, err := bridge.Names()
	if err != nil {
		return 0, err
	}
	address, err := agentname.ParseMachineAddress(mail.Sender, append([]string{current}, former...), machinename.CheckName)
	if err != nil {
		return 0, err
	}
	if !s.isHubOrchName(mail.Receiver) {
		return 0, fmt.Errorf("creel: %s is no orch of this hub, and only an orch takes mail from another machine; way forward: post anew with lym creel post %s \"<text>\"",
			mail.Receiver, agentname.FormatMachineAddress(current, s.geom.Shortname+":orch"))
	}
	if err := validateMachineLine(address.Name.String(), mail.Receiver, mail.Body); err != nil {
		return 0, err
	}
	var id int64
	err = s.do(func() error {
		for _, held := range s.state.messages {
			if held.SendingMachine != "" && held.From == mail.Sender && held.SendingID == mail.SendingID {
				id = held.ID
				return nil
			}
		}
		kind := kindMail
		if mail.ReplyTo != 0 {
			kind = kindReply
			if sent := s.state.machineOutbound[mail.ReplyTo]; sent == nil || sent.To != mail.Sender || sent.From != mail.Receiver {
				return fmt.Errorf("creel: the reply names message #%d, which %s never sent to %s or which is no longer kept; way forward: post anew with lym creel post %s \"<text>\"",
					mail.ReplyTo, mail.Receiver, mail.Sender, mail.Sender)
			}
		}
		id = s.state.newID()
		if err := s.writer.append(record{
			Event: eventMachineInbound, ID: id, From: mail.Sender, To: mail.Receiver, Kind: kind, Body: mail.Body, ReplyTo: mail.ReplyTo,
			SendingMachine: address.Machine, SendingID: mail.SendingID, Trusted: agentname.MatchesRole(address.Name.Role, agentname.RoleOrch),
			RemotePosted: mail.Posted, DeliveredTo: mail.Receiver,
		}); err != nil {
			return err
		}
		if mail.ReplyTo != 0 {
			s.answerLocked(waitKey{kind: waitReply, id: mail.ReplyTo}, Outcome{Kind: OutcomeReply, Message: *s.state.messages[id]})
		}
		return s.deliverMailLocked(id, mail.Receiver)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}
