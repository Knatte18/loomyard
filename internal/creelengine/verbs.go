// verbs.go implements the pool's verbs: post, reply, read, inbox, forward, log, inject, interrupt, hold, release and withdraw, with the privilege rules.

package creelengine

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/Knatte18/loomyard/internal/logger"
)

const holdTimeLayout = time.RFC3339

// validateBody refuses an empty body and one over the cap.
// The cap is maxBodyBytes, the bound of a reply to EXTERNAL alone, which no claim hands out.
func validateBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("creel: the message body is empty")
	}
	if len(body) > maxBodyBytes {
		return fmt.Errorf("creel: the message body is %d bytes, over the %d-byte limit; write it to a file and mail the file's path", len(body), maxBodyBytes)
	}
	return nil
}

// LineFits reports whether the line carrying a message from from with that body is at most boundaryClaimBudget bytes, measured with the widest id an int64 takes so the answer never depends on the id the post gets.
// A line of that size is at most 8,192 UTF-16 units, the measure a hook field is capped by, so a fitting line always arrives whole.
func LineFits(from, body string) bool {
	return len(MessageLine(Message{ID: math.MaxInt64, From: from, Body: body})) <= boundaryClaimBudget
}

// validateLine refuses an empty body and one whose line does not fit one hook delivery.
func validateLine(from, body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("creel: the message body is empty")
	}
	if LineFits(from, body) {
		return nil
	}
	size := len(MessageLine(Message{ID: math.MaxInt64, From: from, Body: body}))
	return fmt.Errorf("creel: the message's line is %d bytes, over the %d-byte limit one hook delivery carries; write it to a file and mail the file's path", size, boundaryClaimBudget)
}

// lineOverHookField reports whether msg's line is longer than hookFieldCap UTF-16 units, so Claude Code would show a preview of it instead of the whole line.
func lineOverHookField(msg *Message) bool {
	return len(utf16.Encode([]rune(MessageLine(*msg)))) > hookFieldCap
}

// appendPostedLocked journals a posted record from sender to receiver and returns its new id.
func (s *Store) appendPostedLocked(sender Sender, receiver, kind, body string, replyTo int64) (int64, error) {
	id := s.state.newID()
	err := s.writer.append(record{
		Event: eventPosted, ID: id, From: sender.Name, Envelope: sender.Envelope(),
		To: receiver, Kind: kind, Body: body, ReplyTo: replyTo,
	})
	return id, err
}

// deliverMailLocked finishes a post, reply or forward to receiver: the holder's waiters and the wake.
func (s *Store) deliverMailLocked(id int64, receiver string) error {
	s.answerLocked(waitKey{kind: waitHolder, holder: receiver}, Outcome{Kind: OutcomeMail})
	s.mailSeq++
	s.wakeLocked(receiver)
	return nil
}

// Post stores a message for receiver and returns its id.
func (s *Store) Post(sender Sender, receiver, body string) (int64, error) {
	id, _, _, err := s.post(sender, receiver, body, false, false)
	return id, err
}

// PostWait is Post that also registers a wait for the reply to the new message, before any reply can arrive.
func (s *Store) PostWait(sender Sender, receiver, body string) (int64, *Waiter, error) {
	id, _, waiter, err := s.post(sender, receiver, body, true, false)
	return id, waiter, err
}

// PostInterrupt is Post, or PostWait when wait is set, that first stops the receiver's running turn, so the post lands as the input of the turn after it.
// It reports whether a running turn was stopped; a receiver with no running turn still gets the post.
// The post is validated, and the receiver must run creel's hook delivery and be one the sender may interrupt, before any key is sent.
// A refusal, and a post whose store fails, say what was and was not done and the way forward.
func (s *Store) PostInterrupt(sender Sender, receiver, body string, wait bool) (int64, bool, *Waiter, error) {
	return s.post(sender, receiver, body, wait, true)
}

// interruptBeforePost stops the running turn of receiver for a post from sender and reports whether one was stopped.
// Every refusal leaves nothing interrupted; the refusals of the interrupt itself are reworded for a post.
func (s *Store) interruptBeforePost(sender Sender, receiver string) (bool, error) {
	target, err := s.strandTarget(receiver)
	if err != nil {
		return false, err
	}
	if err := s.checkMayAct(sender, target.Name, "interrupt"); err != nil {
		return false, err
	}
	if !target.CreelHooks {
		return false, fmt.Errorf("creel: %s runs no creel hook delivery, so post --interrupt cannot hand it the post; way forward: lym creel inject --interrupt %s <text>, which stops the turn and types the text, or a post without --interrupt, which the receiver gets by its typed notice or reminder when its engine is unknown or has no hooks and by lym creel read otherwise", target.Name, target.Name)
	}
	interrupted, err := s.interruptTarget(sender, target)
	var refusal *interruptRefusal
	if !errors.As(err, &refusal) {
		return interrupted, err
	}
	switch refusal.kind {
	case refusalHold:
		return false, fmt.Errorf("%w; nothing was interrupted or posted; way forward: post without --interrupt, which stores the mail for the receiver now, or post --interrupt again after the hold ends", err)
	case refusalPauseLifted:
		return false, fmt.Errorf("creel: the pause covering %s was lifted before the interrupt, so nothing was interrupted or posted; way forward: run lym creel post --interrupt %s <text> again", target.Name, target.Name)
	default:
		return false, fmt.Errorf("%w; nothing was interrupted or posted; way forward: post without --interrupt, which stores the mail for the receiver now, or post --interrupt again once the pause record reads", err)
	}
}

func (s *Store) post(sender Sender, receiver, body string, wait, interrupt bool) (int64, bool, *Waiter, error) {
	if err := validateLine(sender.Name, body); err != nil {
		return 0, false, nil, err
	}
	var interrupted bool
	if interrupt {
		var err error
		interrupted, err = s.interruptBeforePost(sender, receiver)
		if err != nil && !interrupted {
			return 0, false, nil, err
		}
		if err != nil {
			return 0, true, nil, storedAfterInterrupt(receiver, err)
		}
	}
	id, waiter, err := s.storePost(sender, receiver, body, wait)
	if err != nil && interrupted {
		return 0, true, nil, storedAfterInterrupt(receiver, err)
	}
	return id, interrupted, waiter, err
}

// storedAfterInterrupt words a store failure that came after a turn was stopped.
func storedAfterInterrupt(receiver string, cause error) error {
	return fmt.Errorf("creel: the turn of %s was interrupted, but the post was not stored: %w; way forward: post again without --interrupt", receiver, cause)
}

// storePost journals a validated post to receiver and, with wait, registers the wait for its reply.
func (s *Store) storePost(sender Sender, receiver, body string, wait bool) (int64, *Waiter, error) {
	strands, err := s.readStrands()
	if err != nil {
		return 0, nil, err
	}
	var (
		id     int64
		waiter *Waiter
	)
	err = s.do(func() error {
		if err := s.checkReceiverLocked(strands, receiver); err != nil {
			return err
		}
		var err error
		if id, err = s.appendPostedLocked(sender, receiver, kindMail, body, 0); err != nil {
			return err
		}
		if wait {
			if waiter, err = s.addWaiterLocked(waitKey{kind: waitReply, id: id}); err != nil {
				return err
			}
		}
		return s.deliverMailLocked(id, receiver)
	})
	return id, waiter, err
}

// Reply posts text as an answer to message id, addressed to the original sender.
// The caller must hold or have read the message.
// A reply to an EXTERNAL sender is handed to the wait registered on id, and journaled undelivered when none is.
func (s *Store) Reply(sender Sender, id int64, text string) (int64, error) {
	var replyID int64
	err := s.do(func() error {
		original := s.state.messages[id]
		if original == nil {
			return fmt.Errorf("creel: message #%d is no longer in the pool", id)
		}
		validate := validateBody
		if !isExternalName(original.From) {
			validate = func(text string) error { return validateLine(sender.Name, text) }
		}
		if err := validate(text); err != nil {
			return err
		}
		if original.Holder != sender.Name {
			return fmt.Errorf("creel: message #%d is not yours to reply to: you neither hold nor have read it", id)
		}
		var err error
		if replyID, err = s.appendPostedLocked(sender, original.From, kindReply, text, id); err != nil {
			return err
		}
		reply := *s.state.messages[replyID]
		if !isExternalName(original.From) {
			return s.deliverMailLocked(replyID, original.From)
		}
		if s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeReply, Message: reply}) > 0 {
			return s.writer.append(record{Event: eventDelivered, ID: replyID})
		}
		return s.writer.append(record{Event: eventUndelivered, ID: replyID})
	})
	return replyID, err
}

// Reading is the set of messages a read handed out; they are unread messages, reserved until Ack or Release.
type Reading struct {
	// Messages are the messages handed out, oldest first.
	Messages []Message
	store    *Store
	holder   string
	ids      []int64
}

// Read returns the caller's unread mail, oldest first, that no other read has reserved: every message, or with from only that sender's.
// They are reserved so a concurrent read by the same holder skips them.
// A program sender's first read makes its mailbox known.
func (s *Store) Read(sender Sender, from string) (*Reading, error) {
	if sender.Kind == KindExternal {
		return nil, errExternalHoldsNothing
	}
	reading := &Reading{store: s, holder: sender.Name}
	err := s.do(func() error {
		if sender.Kind == KindProgram && !s.state.programs[sender.Name] {
			if err := s.writer.append(record{Event: eventRead, By: sender.Name}); err != nil {
				return err
			}
		}
		for _, msg := range s.unreadLocked(sender.Name, from) {
			s.reserved[msg.ID] = sender.Name
			reading.ids = append(reading.ids, msg.ID)
			reading.Messages = append(reading.Messages, *msg)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reading, nil
}

// hookFieldCap is the number of UTF-16 units Claude Code keeps of one hook field; a longer field is shown as a preview.
const hookFieldCap = 10000

// boundaryClaimBudget bounds the bytes of message lines, with the newlines joining them, one tool-boundary claim hands out, so one boundary adds a bounded amount to the session's context.
const boundaryClaimBudget = 8 << 10

// Boundary names the hook that asks for a claim.
type Boundary struct {
	// Stop is true for the turn-end Stop hook and false for the tool-boundary hook.
	Stop bool
	// StopHookActive is the Stop hook input's stop_hook_active: true when an earlier Stop block of the same turn already continued it.
	StopHookActive bool
}

// stopBlockCap is the number of consecutive Stop blocks creel allows one turn, below the 8 Claude Code allows before it overrides a block.
const stopBlockCap = 6

// ClaimAtBoundary hands a strand's tool-boundary hook the strand's unread mail no read has reserved, oldest first and agent mail included, reserved until Ack or Release like a read's.
// Under a hold it hands out only the hold owner's mail, and while a pause covers the strand it hands out nothing.
// It never claims a message whose MessageLine is longer than hookFieldCap UTF-16 units, which stays unread for `lym creel read`.
// It stops before the message whose MessageLine, and the newline joining it to the line before, would take the lines past boundaryClaimBudget, but always hands out the first, and what it leaves stays unread for the next boundary.
// A Stop claim hands out nothing once the strand's current turn has had stopBlockCap Stop blocks, or when no Stop or tool-boundary claim of this daemon has set its count, so the turn end is recorded.
// A claim that hands out at least one message also marks the strand's last ring answered.
// Only a strand sender may claim; any other sender is refused with the read verb as the way forward.
func (s *Store) ClaimAtBoundary(sender Sender, boundary Boundary) (*Reading, error) {
	if sender.Kind != KindStrand {
		return nil, fmt.Errorf("creel: only a strand's tool-boundary hook claims mail, and %s is no strand; read with `lym creel read`", sender.Name)
	}
	reading := &Reading{store: s, holder: sender.Name}
	strand, err := s.strandTarget(sender.Name)
	if err != nil {
		return nil, err
	}
	paused, err := s.delivery.Paused(strand)
	if err != nil {
		return nil, fmt.Errorf("creel: could not read the pause record for %s: %w", sender.Name, err)
	}
	if paused {
		return reading, nil
	}
	err = s.do(func() error {
		if boundary.Stop && !boundary.StopHookActive {
			s.stopBlocks[sender.Name] = 0
		}
		if boundary.Stop {
			if blocks, known := s.stopBlocks[sender.Name]; !known || blocks >= stopBlockCap {
				return nil
			}
		}
		used := 0
		for _, msg := range s.claimableLocked(sender.Name) {
			size := len(MessageLine(*msg))
			if len(reading.ids) > 0 {
				// The newline the hook joins this line to the one before with counts against the budget too.
				size++
				if used+size > boundaryClaimBudget {
					break
				}
			}
			used += size
			s.reserved[msg.ID] = sender.Name
			reading.ids = append(reading.ids, msg.ID)
			reading.Messages = append(reading.Messages, *msg)
		}
		if len(reading.ids) > 0 {
			if mark, rung := s.wakeMarks[sender.Name]; rung && !mark.at.IsZero() {
				mark.answered = true
				s.wakeMarks[sender.Name] = mark
			}
			// Counted before the client writes the lines, so the count errs early and never late.
			if boundary.Stop {
				s.stopBlocks[sender.Name]++
			} else {
				s.stopBlocks[sender.Name] = 0
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reading, nil
}

// claimableLocked returns the unread messages ClaimAtBoundary would hand holder if the byte budget were unlimited, oldest first.
// They are the messages no read has reserved, the hold owner's alone under a live hold, and none whose line passes hookFieldCap.
func (s *Store) claimableLocked(holder string) []*Message {
	hold := s.liveHoldLocked(holder)
	var claimable []*Message
	for _, msg := range s.unreadLocked(holder, "") {
		if hold != nil && msg.From != hold.Owner {
			continue
		}
		if lineOverHookField(msg) {
			continue
		}
		claimable = append(claimable, msg)
	}
	return claimable
}

// resetStopBlocks starts strand's Stop count over at zero, which also makes the count known to this daemon.
func (s *Store) resetStopBlocks(strand string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopBlocks[strand] = 0
}

// HookWait registers the idle waiter of the calling strand: a wait the daemon answers with OutcomeRing when it wakes the idle strand.
// A newer waiter of the same strand ends the older, which is answered OutcomeSuperseded before the new one registers.
// Only a strand sender may wait; any other sender is refused with the way forward for its kind.
func (s *Store) HookWait(sender Sender) (*Waiter, error) {
	if sender.Kind != KindStrand {
		message := fmt.Sprintf("creel: only a strand's idle waiter hook waits for a wake, and %s is no strand", sender.Name)
		switch sender.Kind {
		case KindProgram:
			message += "; read with lym creel read"
		case KindExternal:
			message += "; wait for an answer with lym creel post --wait-reply"
		}
		return nil, errors.New(message)
	}
	var waiter *Waiter
	err := s.do(func() error {
		key := waitKey{kind: waitHook, holder: sender.Name}
		s.answerLocked(key, Outcome{Kind: OutcomeSuperseded})
		var err error
		waiter, err = s.addWaiterLocked(key)
		if err == nil {
			s.wakeLocked(sender.Name)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return waiter, nil
}

// hookWakeState reads what the ring decision for strand rests on: its claimable message ids, the mark of its last ring and whether a waiter is registered.
func (s *Store) hookWakeState(strand string) (ids []int64, mark wakeMark, waiting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.claimableLocked(strand) {
		ids = append(ids, msg.ID)
	}
	return ids, s.wakeMarks[strand], len(s.waiters[waitKey{kind: waitHook, holder: strand}]) > 0
}

// ringHook answers strand's registered waiters with OutcomeRing and records the mark of the ring at now, provided the claimable set is still ids and a waiter is still registered.
// It reports whether it rang, and starts the strand's Stop count over when it did.
func (s *Store) ringHook(strand string, ids []int64, now time.Time) bool {
	rang := false
	_ = s.do(func() error {
		var current []int64
		for _, msg := range s.claimableLocked(strand) {
			current = append(current, msg.ID)
		}
		if !slices.Equal(current, ids) {
			return nil
		}
		rang = s.answerLocked(waitKey{kind: waitHook, holder: strand}, Outcome{Kind: OutcomeRing}) > 0
		if rang {
			s.wakeMarks[strand] = wakeMark{ids: ids, at: now}
		}
		return nil
	})
	if rang {
		s.resetStopBlocks(strand)
	}
	return rang
}

var errExternalHoldsNothing = errors.New("creel: an EXTERNAL caller holds no mail; wait for an answer with `lym creel post --wait-reply`")

// Ack marks the reserved messages read and journals each; a message that left the holder meanwhile stays as it is.
func (r *Reading) Ack() error {
	return r.store.do(func() error {
		for _, id := range r.ids {
			delete(r.store.reserved, id)
			msg := r.store.state.messages[id]
			if msg == nil || msg.Holder != r.holder || msg.Status != StatusUnread {
				continue
			}
			if err := r.store.writer.append(record{Event: eventRead, ID: id, By: r.holder}); err != nil {
				return err
			}
		}
		r.ids = nil
		return nil
	})
}

// Release drops the reservation and leaves the messages unread; a wait for the holder's mail is answered, since the mail is back.
func (r *Reading) Release() {
	_ = r.store.do(func() error {
		for _, id := range r.ids {
			delete(r.store.reserved, id)
		}
		if len(r.ids) > 0 {
			r.store.answerLocked(waitKey{kind: waitHolder, holder: r.holder}, Outcome{Kind: OutcomeMail})
		}
		r.ids = nil
		return nil
	})
}

// WaitReply registers a wait for the reply to message id, which the caller posted.
// A reply that already came while no wait was registered is handed over at once and journaled delivered.
func (s *Store) WaitReply(sender Sender, id int64) (*Waiter, error) {
	var waiter *Waiter
	err := s.do(func() error {
		msg := s.state.messages[id]
		if msg == nil || msg.From != sender.Name {
			return fmt.Errorf("creel: message #%d is not one you posted", id)
		}
		var err error
		waiter, err = s.addWaiterLocked(waitKey{kind: waitReply, id: id})
		return err
	})
	return waiter, err
}

// WaitInject registers a wait for the outcome of inject id, which the caller queued.
func (s *Store) WaitInject(sender Sender, id int64) (*Waiter, error) {
	var waiter *Waiter
	err := s.do(func() error {
		entry := s.state.injects[id]
		if entry == nil || entry.From != sender.Name {
			return fmt.Errorf("creel: inject #%d is not one you queued", id)
		}
		var err error
		waiter, err = s.addWaiterLocked(waitKey{kind: waitInject, id: id})
		return err
	})
	return waiter, err
}

// WaitMail registers a wait for unread mail for the caller, answered at once when the caller already holds some.
func (s *Store) WaitMail(sender Sender) (*Waiter, error) {
	if sender.Kind == KindExternal {
		return nil, errExternalHoldsNothing
	}
	var waiter *Waiter
	err := s.do(func() error {
		var err error
		waiter, err = s.addWaiterLocked(waitKey{kind: waitHolder, holder: sender.Name})
		return err
	})
	return waiter, err
}

// InboxEntry is one sender with the number of its messages the caller holds unread.
type InboxEntry struct {
	From   string
	Unread int
}

// InboxResult is what Inbox answers: the unread counts per sender.
type InboxResult struct {
	Senders []InboxEntry
}

// Inbox returns the senders of the caller's unread mail with their counts, in name order.
func (s *Store) Inbox(sender Sender) (InboxResult, error) {
	if sender.Kind == KindExternal {
		return InboxResult{}, errExternalHoldsNothing
	}
	var result InboxResult
	err := s.do(func() error {
		counts := map[string]int{}
		for _, msg := range s.state.messages {
			if msg.Holder == sender.Name && msg.Status == StatusUnread {
				counts[msg.From]++
			}
		}
		for from, unread := range counts {
			result.Senders = append(result.Senders, InboxEntry{From: from, Unread: unread})
		}
		sort.Slice(result.Senders, func(i, j int) bool { return result.Senders[i].From < result.Senders[j].From })
		return nil
	})
	return result, err
}

// Forward moves an unread message the caller holds to receiver: the holder changes, From stays, and the caller joins the message's path.
func (s *Store) Forward(sender Sender, id int64, receiver string) error {
	strands, err := s.readStrands()
	if err != nil {
		return err
	}
	return s.do(func() error {
		msg := s.state.messages[id]
		switch {
		case msg == nil:
			return fmt.Errorf("creel: message #%d is no longer in the pool", id)
		case msg.Holder != sender.Name:
			return fmt.Errorf("creel: message #%d is not yours to forward: you do not hold it", id)
		case msg.Status != StatusUnread:
			return fmt.Errorf("creel: message #%d is already read and cannot be forwarded", id)
		case receiver == sender.Name:
			return fmt.Errorf("creel: message #%d cannot be forwarded to yourself", id)
		}
		if err := s.checkReceiverLocked(strands, receiver); err != nil {
			return err
		}
		if err := s.writer.append(record{Event: eventForwarded, ID: id, By: sender.Name, To: receiver}); err != nil {
			return err
		}
		return s.deliverMailLocked(id, receiver)
	})
}

// LogFilter narrows the log; a zero field filters nothing.
type LogFilter struct {
	ID    int64
	From  string
	To    string
	Since time.Time
}

// Log returns the journal events matching the filter, oldest first, with ids, parties, times and subjects and never a body.
// The id filter also keeps an event whose unread ids hold the id, such as a reminder.
func (s *Store) Log(filter LogFilter) []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []LogEntry
	for _, entry := range s.state.events {
		if (filter.ID == 0 || entry.ID == filter.ID || slices.Contains(entry.Unread, filter.ID)) &&
			(filter.From == "" || entry.From == filter.From) &&
			(filter.To == "" || entry.To == filter.To) &&
			(filter.Since.IsZero() || !entry.At.Before(filter.Since)) {
			entries = append(entries, entry)
		}
	}
	return entries
}

// InjectResult is what Inject answers: the inject's id, and for an inject that interrupts whether a running turn was stopped.
type InjectResult struct {
	ID          int64
	Interrupted bool
}

// Inject queues text to be typed into receiver's pane at its next idle, behind earlier injects.
// A zero ttl takes the default; interrupt first sends the guarded interrupt and queues the text at the head.
func (s *Store) Inject(sender Sender, receiver, text string, ttl time.Duration, interrupt bool) (InjectResult, error) {
	result, _, err := s.inject(sender, receiver, text, Command{}, ttl, interrupt, false)
	return result, err
}

// InjectWait is Inject that also registers a wait for the inject's outcome, before any outcome can happen.
func (s *Store) InjectWait(sender Sender, receiver, text string, ttl time.Duration, interrupt bool) (InjectResult, *Waiter, error) {
	return s.inject(sender, receiver, text, Command{}, ttl, interrupt, true)
}

// InjectCommand queues a command to be run in an orch strand's session at its next idle, behind earlier injects, under every rule of a text inject.
// Only a program sender may queue one, and only for an orch strand; an invalid command is refused.
func (s *Store) InjectCommand(sender Sender, receiver string, command Command, ttl time.Duration) (InjectResult, error) {
	if sender.Kind != KindProgram {
		return InjectResult{}, fmt.Errorf("creel: %s may not queue a command inject; only a program may, through the Go client", sender.Name)
	}
	if !isOrchName(receiver) {
		return InjectResult{}, fmt.Errorf("creel: a command inject goes to an orch strand only, not to %s", receiver)
	}
	if err := command.validate(); err != nil {
		return InjectResult{}, err
	}
	result, _, err := s.inject(sender, receiver, "", command, ttl, false, false)
	return result, err
}

func (s *Store) inject(sender Sender, receiver, text string, command Command, ttl time.Duration, interrupt, wait bool) (InjectResult, *Waiter, error) {
	if strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return InjectResult{}, nil, errors.New("creel: an inject is a single line without control characters")
	}
	if ttl == 0 {
		ttl = defaultInjectTTL
	}
	if ttl < 0 || ttl > maxInjectTTL {
		return InjectResult{}, nil, fmt.Errorf("creel: an inject lives at most %s", maxInjectTTL)
	}
	target, err := s.strandTarget(receiver)
	if err != nil {
		return InjectResult{}, nil, err
	}
	if err := s.checkMayAct(sender, target.Name, "inject into"); err != nil {
		return InjectResult{}, nil, err
	}
	var result InjectResult
	if interrupt {
		if result.Interrupted, err = s.interruptTarget(sender, target); err != nil {
			return InjectResult{}, nil, err
		}
	}
	var waiter *Waiter
	err = s.do(func() error {
		id := s.state.newID()
		if err := s.writer.append(record{
			Event: eventPosted, Kind: kindInject, ID: id, From: sender.Name, To: target.Name,
			Body: text, Command: command.Name, Argument: command.Argument,
			TTLSeconds: int64(ttl / time.Second), Head: interrupt,
		}); err != nil {
			return err
		}
		result.ID = id
		if wait {
			var err error
			if waiter, err = s.addWaiterLocked(waitKey{kind: waitInject, id: id}); err != nil {
				return err
			}
		}
		s.mailSeq++
		s.wakeLocked(target.Name)
		return nil
	})
	return result, waiter, err
}

// QueuedInject is one inject waiting to be typed.
type QueuedInject struct {
	ID   int64
	From string
	Text string
	// Command is the command a command inject carries, zero for a text inject.
	Command Command
	TTL     time.Duration
	// Head marks an inject queued ahead of the others by an interrupt.
	Head bool
}

// Queued returns the injects waiting for receiver in the order they will be typed: the interrupt-queued ones first, newest first, then the rest oldest first.
func (s *Store) Queued(receiver string) []QueuedInject {
	s.mu.Lock()
	defer s.mu.Unlock()
	var heads, rest []QueuedInject
	for _, entry := range s.state.injects {
		if entry.Receiver != receiver || entry.Outcome != injectQueued {
			continue
		}
		queued := QueuedInject{ID: entry.ID, From: entry.From, Text: entry.Text, Command: entry.Command, TTL: entry.TTL, Head: entry.Head}
		if entry.Head {
			heads = append(heads, queued)
		} else {
			rest = append(rest, queued)
		}
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].ID > heads[j].ID })
	sort.Slice(rest, func(i, j int) bool { return rest[i].ID < rest[j].ID })
	return append(heads, rest...)
}

// strandTarget returns the hub's strand named receiver, or an error listing the strand names that exist.
func (s *Store) strandTarget(receiver string) (Strand, error) {
	strands, err := s.readStrands()
	if err != nil {
		return Strand{}, err
	}
	if strand, ok := findStrand(strands, receiver); ok {
		return strand, nil
	}
	names := make([]string, 0, len(strands))
	for _, strand := range strands {
		names = append(names, strand.Name)
	}
	sort.Strings(names)
	listing := "none"
	if len(names) > 0 {
		listing = strings.Join(names, ", ")
	}
	return Strand{}, fmt.Errorf("creel: unknown strand %q; the strands that exist: %s", receiver, listing)
}

// checkMayAct applies the privilege rule to a sender acting on a strand's pane.
// A privileged sender (EXTERNAL, a program, an orch strand) may act on any strand, an orch strand included, and any other sender on its own.
func (s *Store) checkMayAct(sender Sender, receiver, action string) error {
	if !sender.privileged() && sender.Name != receiver {
		return fmt.Errorf("creel: %s may not %s another strand's pane; send it mail with `lym creel post`", sender.Name, action)
	}
	return nil
}

// Interrupt sends the guarded interrupt to receiver at once, bypassing the queue, and reports whether a running turn was stopped.
// A receiver with no running turn gets nothing and false.
// A receiver a pause covers is claimed into that pause and then interrupted, so the pause's resume gives it its resume line;
// an unreadable pause record and another sender's hold refuse the interrupt.
func (s *Store) Interrupt(sender Sender, receiver string) (bool, error) {
	target, err := s.strandTarget(receiver)
	if err != nil {
		return false, err
	}
	if err := s.checkMayAct(sender, target.Name, "interrupt"); err != nil {
		return false, err
	}
	return s.interruptTarget(sender, target)
}

// interruptTarget runs the checks that follow privilege and orch, in order, then the interrupt: another sender's hold, then the claim of a covering pause.
// A strand a live pause covers is recorded in that pause before the key, so the pause's resume gives it its resume line;
// an unreadable pause record refuses with nothing recorded, and so does a pause lifted before the key.
// The interrupt runs under the receiver's pane lock, so no delivery reaches the receiver meanwhile, and the receiver's worker is woken once it ends.
// An interrupt whose key was sent and whose clear failed counts as sent.
// A hold, a lifted pause and an unreadable pause record each refuse with an *interruptRefusal of their kind, whose message names the interrupt.
func (s *Store) interruptTarget(sender Sender, target Strand) (bool, error) {
	var holdErr error
	_ = s.do(func() error {
		if hold := s.liveHoldLocked(target.Name); hold != nil && hold.Owner != sender.Name {
			holdErr = holdRefusal(target.Name, hold)
		}
		return nil
	})
	if holdErr != nil {
		return false, holdErr
	}
	claim, err := s.delivery.ClaimPause(target)
	if err != nil {
		return false, unreadablePauseRefusal(target.Name, err)
	}
	lock := s.paneLock(target.Name)
	lock.Lock()
	if claim.Covered {
		live, liveErr := claim.StillLive()
		if liveErr != nil || !live {
			lock.Unlock()
			if liveErr != nil {
				return false, unreadablePauseRefusal(target.Name, liveErr)
			}
			return false, pauseLiftedRefusal(target.Name)
		}
	}
	err = s.delivery.Interrupt(target)
	lock.Unlock()
	_ = s.do(func() error {
		s.wakeLocked(target.Name)
		return nil
	})
	switch {
	case err == nil, errors.Is(err, ErrClearFailed):
		if err != nil {
			logger.Warn("creel interrupt: the clear after the key failed", "receiver", target.Name, "error", err)
		}
		return true, s.do(func() error {
			return s.writer.append(record{Event: eventInterrupted, By: sender.Name, To: target.Name})
		})
	case errors.Is(err, ErrNoRunningTurn):
		return false, nil
	default:
		return false, err
	}
}

// refusalKind tells apart the refusals of an interrupt that a post to the same receiver words for itself.
type refusalKind int

const (
	refusalHold refusalKind = iota
	refusalPauseLifted
	refusalPauseUnreadable
)

// interruptRefusal is a refusal of an interrupt that sent no key, carrying its kind so a caller can word it differently while Error keeps the message.
type interruptRefusal struct {
	kind refusalKind
	err  error
}

func (r *interruptRefusal) Error() string { return r.err.Error() }

func (r *interruptRefusal) Unwrap() error { return r.err }

func pauseLiftedRefusal(receiver string) error {
	return &interruptRefusal{kind: refusalPauseLifted, err: fmt.Errorf("creel: the pause covering %s was lifted before the interrupt, so nothing was interrupted; way forward: run \"lym creel interrupt %s\" again", receiver, receiver)}
}

func unreadablePauseRefusal(receiver string, err error) error {
	return &interruptRefusal{kind: refusalPauseUnreadable, err: fmt.Errorf("creel: could not read the pause record for %s, so nothing was interrupted: %w", receiver, err)}
}

func holdRefusal(receiver string, hold *holdEntry) error {
	return &interruptRefusal{kind: refusalHold, err: fmt.Errorf("creel: %s is held by %s until %s", receiver, hold.Owner, hold.Expires.UTC().Format(holdTimeLayout))}
}

// HoldResult is what Hold answers: the hold's owner and expiry, and whether it renewed the owner's earlier hold.
type HoldResult struct {
	Owner   string
	Expires time.Time
	Renewed bool
}

// Hold holds receiver's deliveries for the given duration: only the owner's injects pass and only its mail is delivered, through the receiver's hooks or by its notice or reminder when it is a pane-mail strand.
// The same owner holding again renews the hold; another sender's hold on a held receiver is refused.
// An orch strand is held by a program only.
func (s *Store) Hold(sender Sender, receiver string, duration time.Duration) (HoldResult, error) {
	if duration <= 0 || duration > maxHoldDuration {
		return HoldResult{}, fmt.Errorf("creel: a hold lasts at most %s", maxHoldDuration)
	}
	target, err := s.strandTarget(receiver)
	if err != nil {
		return HoldResult{}, err
	}
	if !sender.privileged() && sender.Name != target.Name {
		return HoldResult{}, fmt.Errorf("creel: %s may not hold another strand; send it mail with `lym creel post`", sender.Name)
	}
	if isOrchName(target.Name) && sender.Kind != KindProgram {
		return HoldResult{}, fmt.Errorf("creel: only a program may hold an orch strand, and %s is not one; send %s mail with `lym creel post`, or cycle the orch with `lym orch distill` or `lym orch refresh`", sender.Name, target.Name)
	}
	var result HoldResult
	err = s.do(func() error {
		current := s.liveHoldLocked(target.Name)
		if current != nil && current.Owner != sender.Name {
			return holdRefusal(target.Name, current)
		}
		result = HoldResult{Owner: sender.Name, Expires: s.clock.Now().UTC().Add(duration), Renewed: current != nil}
		event := eventHeld
		if result.Renewed {
			event = eventRenewed
		}
		return s.writer.append(record{Event: event, By: sender.Name, To: target.Name, ExpiresAt: result.Expires})
	})
	return result, err
}

// WithdrawnReason is the journal reason of an inject expired, and of a message returned undelivered, by a withdraw.
const WithdrawnReason = "withdrawn"

// WithdrawnMessage is one unread message a withdraw removed.
type WithdrawnMessage struct {
	ID      int64
	Subject string
}

// WithdrawResult is what Withdraw answers: the ids of the injects it expired and the messages it returned undelivered, both in id order.
type WithdrawResult struct {
	Injects  []int64
	Messages []WithdrawnMessage
}

// Withdraw removes the caller's own queued injects to receiver and its own unread, unreserved mail held by receiver.
// Each inject is journaled expired and each message undelivered, with reason WithdrawnReason; a wait on an inject is answered expired and a wait on a message's reply is answered returned.
// An inject already sending and a message already read or being read are left alone.
// Only a program may withdraw, and it acts on receiver by name whether or not creel still lists that strand; with nothing queued the result is empty.
func (s *Store) Withdraw(sender Sender, receiver string) (WithdrawResult, error) {
	if sender.Kind != KindProgram {
		return WithdrawResult{}, fmt.Errorf("creel: %s may not withdraw; only a program may, through the Go client", sender.Name)
	}
	var result WithdrawResult
	err := s.do(func() error {
		for _, entry := range s.queuedEntriesLocked(receiver) {
			if entry.From != sender.Name {
				continue
			}
			if err := s.writer.append(record{Event: eventExpired, ID: entry.ID, Reason: WithdrawnReason}); err != nil {
				return err
			}
			s.answerLocked(waitKey{kind: waitInject, id: entry.ID}, Outcome{Kind: OutcomeExpired})
			result.Injects = append(result.Injects, entry.ID)
		}
		unread := s.unreadLocked(receiver, sender.Name)
		for _, msg := range unread {
			if err := s.writer.append(record{Event: eventUndelivered, ID: msg.ID, Reason: WithdrawnReason}); err != nil {
				return err
			}
			s.answerLocked(waitKey{kind: waitReply, id: msg.ID}, Outcome{Kind: OutcomeReturned, Message: *msg})
			result.Messages = append(result.Messages, WithdrawnMessage{ID: msg.ID, Subject: msg.Subject})
		}
		return nil
	})
	return result, err
}

// Release ends the caller's hold on receiver and reports whether there was one, even when the strand has since been removed.
// Another sender's hold is refused; with no live hold, a name that is no existing strand is refused listing the strands that exist, and an existing strand changes nothing.
func (s *Store) Release(sender Sender, receiver string) (bool, error) {
	var released, held bool
	err := s.do(func() error {
		hold := s.liveHoldLocked(receiver)
		if hold == nil {
			return nil
		}
		held = true
		if hold.Owner != sender.Name {
			return holdRefusal(receiver, hold)
		}
		if err := s.writer.append(record{Event: eventReleased, By: sender.Name, To: receiver}); err != nil {
			return err
		}
		released = true
		s.wakeLocked(receiver)
		return nil
	})
	if err != nil || held {
		return released, err
	}
	_, err = s.strandTarget(receiver)
	return false, err
}
