// machinemail.go implements the Slack relay's verbs over mail to another machine, and creel's hour on that mail.
// The relay takes the pending messages, reports why it holds one and settles each as delivered or returned.
// A scheduler timer returns every message still unsettled an hour after its post, whether or not the relay runs.

package creelengine

import (
	"cmp"
	"fmt"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// BridgeProgram is the program name of the Slack relay, which the hub's shortname leads as `<shortname>:SLACK`.
// Only that program takes, reports on, settles and delivers mail across machines.
const BridgeProgram = "SLACK"

// The verbs the relay sends creel about mail to another machine.
const (
	// VerbMachineTake asks for the pending machine-outbound messages, oldest first.
	VerbMachineTake = "machine-take"
	// VerbMachineReport tells creel why the relay holds a message.
	VerbMachineReport = "machine-report"
	// VerbMachineSettle settles a message as delivered to Slack or as returned.
	VerbMachineSettle = "machine-settle"
)

// MachineUnsettled is the reason of a message that came back at its hour with no hold reason ever reported.
const MachineUnsettled = "bridge-unsettled"

// The hold reasons the relay reports whose return notice names what clears them.
const (
	holdPeerUnapproved  = "peer-unapproved"
	holdBridgeNameTaken = "bridge-name-taken"
	holdBridgeArchived  = "bridge-archived"
	holdButtonOff       = "button-off"
)

// machineHourFloor is the shortest wait the machine hour loop takes between passes.
const machineHourFloor = time.Second

// MachineOutbound is a pending message to an orch of another machine, as a take hands it to the relay.
type MachineOutbound struct {
	ID int64 `json:"id"`
	// From is the local orch and To the full <machine>::<name> address of the receiving orch.
	From             string `json:"from"`
	To               string `json:"to"`
	ReceivingMachine string `json:"receiving_machine"`
	Body             string `json:"body"`
	// ReplyTo is the id the other machine gave the message this one answers, 0 for mail.
	ReplyTo     int64     `json:"reply_to,omitempty"`
	ExpectReply bool      `json:"expect_reply,omitempty"`
	Posted      time.Time `json:"posted"`
}

// requireBridgeProgram refuses a sender other than this hub's relay program, `<shortname>:SLACK`.
// A scoped `<shortname>:<slug>:SLACK` is refused.
// verb names what the sender tried to do with bridge mail.
func (s *Store) requireBridgeProgram(sender Sender, verb string) error {
	want, err := agentname.FormatProgram(s.geom.Shortname, BridgeProgram)
	if err != nil {
		return fmt.Errorf("creel: cannot name the bridge program: %w", err)
	}
	if sender.Kind == KindProgram && sender.Name == want {
		return nil
	}
	return fmt.Errorf("creel: only %s may %s bridge mail, and %s is not it; way forward: post to another machine with lym creel post <machine>::<name> \"<text>\"",
		want, verb, sender.Name)
}

// machineEntryLocked returns the machine-outbound message id, or a refusal naming the id when creel holds none.
func (s *Store) machineEntryLocked(id int64) (*machineOutboundEntry, error) {
	entry := s.state.machineOutbound[id]
	if entry == nil {
		return nil, fmt.Errorf("creel: no machine-outbound message #%d is held; way forward: take again; creel no longer holds that message", id)
	}
	return entry, nil
}

// pendingMachineLocked returns the machine-outbound messages not yet settled, oldest first.
func (s *Store) pendingMachineLocked() []*machineOutboundEntry {
	var pending []*machineOutboundEntry
	for _, entry := range s.state.machineOutbound {
		if entry.State == outboundPending {
			pending = append(pending, entry)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return pending
}

// returnDueMachineLocked returns every pending message whose hour has passed.
// The return carries the message's last hold reason, or MachineUnsettled when it has none.
// It answers the messages still pending, oldest first.
func (s *Store) returnDueMachineLocked(listing strandListing) ([]*machineOutboundEntry, error) {
	now := s.clock.Now()
	var remaining []*machineOutboundEntry
	for _, entry := range s.pendingMachineLocked() {
		if now.Before(entry.Posted.Add(outboundDeadline)) {
			remaining = append(remaining, entry)
			continue
		}
		if err := s.returnMachineLocked(entry, cmp.Or(entry.HoldReason, MachineUnsettled), listing); err != nil {
			return nil, err
		}
	}
	return remaining, nil
}

// TakeMachineOutbound hands the relay the pending machine-outbound messages, oldest first.
// A message whose hour has passed is returned first and never handed out.
// Only the relay's program may take.
func (s *Store) TakeMachineOutbound(sender Sender) ([]MachineOutbound, error) {
	if err := s.requireBridgeProgram(sender, "take"); err != nil {
		return nil, err
	}
	listing := s.listStrands()
	var taken []MachineOutbound
	err := s.do(func() error {
		remaining, err := s.returnDueMachineLocked(listing)
		if err != nil {
			return err
		}
		for _, entry := range remaining {
			taken = append(taken, MachineOutbound{
				ID: entry.ID, From: entry.From, To: entry.To, ReceivingMachine: entry.ReceivingMachine, Body: entry.Body,
				ReplyTo: entry.ReplyTo, ExpectReply: entry.ExpectReply, Posted: entry.Posted,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return taken, nil
}

// ReportMachineHold records why the relay holds the pending machine-outbound message id.
// It journals the reason only when it differs from the last one, and writes nothing for a settled message.
// An id creel holds no machine-outbound message for is refused.
// Only the relay's program may report.
func (s *Store) ReportMachineHold(sender Sender, id int64, reason string) error {
	if err := s.requireBridgeProgram(sender, "report"); err != nil {
		return err
	}
	return s.do(func() error {
		entry, err := s.machineEntryLocked(id)
		if err != nil {
			return err
		}
		if entry.State != outboundPending || entry.HoldReason == reason {
			return nil
		}
		return s.writer.append(record{Event: eventMachineHeld, ID: id, Reason: reason})
	})
}

// SettleMachineDelivered journals that the relay posted the machine-outbound message id to Slack at slackTS.
// A message creel already returned is settled delivered all the same, and its sender is told in a late notice.
// A message already delivered changes nothing.
// An id creel holds no machine-outbound message for is refused.
// Only the relay's program may settle.
func (s *Store) SettleMachineDelivered(sender Sender, id int64, slackTS string) error {
	if err := s.requireBridgeProgram(sender, "settle"); err != nil {
		return err
	}
	listing := s.listStrands()
	return s.do(func() error {
		entry, err := s.machineEntryLocked(id)
		if err != nil {
			return err
		}
		if entry.State == outboundDelivered {
			return nil
		}
		wasReturned := entry.State == outboundReturned
		if err := s.writer.append(record{Event: eventMachineDelivered, ID: id, SlackTS: slackTS}); err != nil {
			return err
		}
		if wasReturned {
			body := fmt.Sprintf("Message #%d to %s, which came back at its hour, did reach the other machine; see: lym creel log --id %d", id, entry.To, id)
			if err := s.postNoticeLocked(entry.From, body, listing); err != nil {
				logger.Warn("creel machine mail: could not post the notice of a late delivery", "id", id, "sender", entry.From, "error", err)
			}
		}
		return nil
	})
}

// SettleMachineReturned returns the pending machine-outbound message id with the relay's reason code.
// A message already settled is left alone.
// An id creel holds no machine-outbound message for is refused.
// Only the relay's program may settle.
func (s *Store) SettleMachineReturned(sender Sender, id int64, reason string) error {
	if err := s.requireBridgeProgram(sender, "settle"); err != nil {
		return err
	}
	listing := s.listStrands()
	return s.do(func() error {
		entry, err := s.machineEntryLocked(id)
		if err != nil {
			return err
		}
		if entry.State != outboundPending {
			return nil
		}
		return s.returnMachineLocked(entry, reason, listing)
	})
}

// returnMachineLocked journals that the pending machine-outbound message entry came back for reason.
// It answers a wait on its reply as returned and tells its sender in a notice.
// A notice that fails to store is logged and undoes nothing.
func (s *Store) returnMachineLocked(entry *machineOutboundEntry, reason string, listing strandListing) error {
	if err := s.writer.append(record{Event: eventMachineReturned, ID: entry.ID, Reason: reason}); err != nil {
		return err
	}
	s.answerLocked(waitKey{kind: waitReply, id: entry.ID}, Outcome{Kind: OutcomeReturned, Message: entry.message()})
	if err := s.noticeReturnedLocked(entry.ID, entry.From, entry.To, entry.ReceivingMachine, reason, listing); err != nil {
		logger.Warn("creel machine mail: could not post the notice of a returned message", "id", entry.ID, "sender", entry.From, "error", err)
	}
	return nil
}

// machineHoldWayForward words what clears the reason a message to orchs of machine came back.
// The reason is a hold reason the relay reported, or MachineUnsettled when it reported none.
// Posting again only holds the message again until it is cleared.
func machineHoldWayForward(reason, machine string) string {
	const holdsAgain = ", since posting again only holds the message again until then"
	switch reason {
	case MachineUnsettled:
		return "ask the operator to check that this hub's slack relay runs and that a slack button of this hub is on" + holdsAgain
	case holdPeerUnapproved:
		return fmt.Sprintf("ask the operator to type `lym approve %s` in an orch channel of this machine%s", machine, holdsAgain)
	case holdBridgeNameTaken, holdBridgeArchived:
		return "act on the relay notice in your orch's Slack channel" + holdsAgain
	case holdButtonOff:
		return "ask the operator to turn your slack button on" + holdsAgain
	}
	return fmt.Sprintf("act on the relay notice for %s in your orch's Slack channel%s", reason, holdsAgain)
}

// SetMachineHook sets the function called when a message to another machine was journaled.
// The hook runs after the verb has released the store's mutex, so it may call back into the store.
func (s *Store) SetMachineHook(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machineHook = hook
}

// returnDueMachine returns every pending machine-outbound message whose hour has passed.
// It answers the earliest hour still pending, and false when none is.
func (s *Store) returnDueMachine() (time.Time, bool, error) {
	listing := s.listStrands()
	var (
		next    time.Time
		pending bool
	)
	err := s.do(func() error {
		remaining, err := s.returnDueMachineLocked(listing)
		if err != nil {
			return err
		}
		for _, entry := range remaining {
			if due := entry.Posted.Add(outboundDeadline); !pending || due.Before(next) {
				next, pending = due, true
			}
		}
		return nil
	})
	return next, pending, err
}

// wakeMachineHour tells the machine hour loop that a message to another machine was journaled.
func (s *Scheduler) wakeMachineHour() {
	select {
	case s.machineWake <- struct{}{}:
	default:
	}
}

// machineHourLoop returns each machine-outbound message still unsettled an hour after its post.
// Each pass returns every message that is due, then waits on the scheduler's clock until the earliest hour still pending.
// The wait is never under machineHourFloor.
// With none pending it waits for a wake, which a post to another machine sends.
// A failed pass is retried after a wait that starts at machineHourFloor and doubles to outboundRetryCeiling.
// A stop ends the loop.
func (s *Scheduler) machineHourLoop() {
	defer s.wg.Done()
	retry := machineHourFloor
	for {
		var timer <-chan time.Time
		next, pending, err := s.store.returnDueMachine()
		switch {
		case err != nil:
			logger.Warn("creel machine mail: could not return the messages past their hour, so it is tried again", "error", err)
			timer = s.clock.After(retry)
			retry = min(retry*2, outboundRetryCeiling)
		case pending:
			retry = machineHourFloor
			timer = s.clock.After(max(next.Sub(s.clock.Now()), machineHourFloor))
		default:
			retry = machineHourFloor
		}
		select {
		case <-timer:
		case <-s.machineWake:
		case <-s.stop:
			return
		}
	}
}
