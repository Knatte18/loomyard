// verbs.go implements the pool's verbs and the privilege rules that gate them.
// The verbs are post, reply, read, inbox, forward, log, inject, interrupt, hold, release and withdraw.

package creelengine

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const holdTimeLayout = time.RFC3339

// primeBeamSlug is the slug `lym beam start` takes for the prime, whose orch name carries no slug.
const primeBeamSlug = "root"

// validateBody refuses an empty body and one over the cap.
// The cap bounds only a reply to an EXTERNAL sender, which only a waiting post receives and no claim hands out.
func validateBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("creel: the message body is empty")
	}
	if len(body) > maxBodyBytes {
		return fmt.Errorf("creel: the message body is %d bytes, over the %d-byte limit; write it to a file and mail the file's path", len(body), maxBodyBytes)
	}
	return nil
}

// LineFits reports whether the line of a message from from with that body fits one claim's byte budget.
// It measures the widest id an int64 takes and the longer, marked form.
// The answer then never depends on the id the post gets or on whether it expects a reply.
// A line within that budget has no more UTF-16 units than a hook field keeps, so a fitting line always arrives whole.
func LineFits(from, body string) bool {
	return len(widestLine(from, body)) <= boundaryClaimBudget
}

// widestLine renders the longest line a message from from with that body can take.
func widestLine(from, body string) string {
	return MessageLine(Message{ID: math.MaxInt64, From: from, Body: body, ExpectReply: true})
}

// validateBodyPresent refuses an empty body.
func validateBodyPresent(body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("creel: the message body is empty")
	}
	return nil
}

// lineOverBudget is the refusal of a message whose widest line is over the bytes one hook delivery carries.
func lineOverBudget(widest string) error {
	return fmt.Errorf("creel: the message's line is %d bytes, over the %d-byte limit one hook delivery carries; write it to a file and mail the file's path", len(widest), boundaryClaimBudget)
}

// validateLine refuses an empty body and one whose line does not fit one hook delivery.
func validateLine(from, body string) error {
	if err := validateBodyPresent(body); err != nil {
		return err
	}
	if LineFits(from, body) {
		return nil
	}
	return lineOverBudget(widestLine(from, body))
}

// lineOverHookField reports whether msg's line is longer than hookFieldCap UTF-16 units.
// Claude Code shows such a line as a preview instead of the whole line.
func lineOverHookField(msg *Message) bool {
	return len(utf16.Encode([]rune(MessageLine(*msg)))) > hookFieldCap
}

// appendPostedLocked journals rec as a posted record from sender under a new id and returns that id.
// rec carries the receiver, kind, body and the fields a post adds; the event, id, sender and envelope are filled here.
func (s *Store) appendPostedLocked(sender Sender, rec record) (int64, error) {
	rec.Event, rec.ID = eventPosted, s.state.newID()
	rec.From, rec.Envelope = sender.Name, sender.Envelope()
	err := s.writer.append(rec)
	return rec.ID, err
}

// deliverMailLocked finishes a post, reply or forward to receiver.
// It answers the waits the receiver holds with OutcomeMail and wakes the receiver.
func (s *Store) deliverMailLocked(id int64, receiver string) error {
	s.answerLocked(waitKey{kind: waitHolder, holder: receiver}, Outcome{Kind: OutcomeMail})
	s.mailSeq++
	s.wakeLocked(receiver)
	return nil
}

// PostOptions are the choices of a post beyond its receiver and body.
type PostOptions struct {
	// Wait registers a wait for the reply to the new message, before any reply can arrive; it implies ExpectReply.
	Wait bool
	// Interrupt stops the receiver's running turn first.
	Interrupt bool
	// ExpectReply marks the message as expecting a reply from its holder.
	ExpectReply bool
}

// PostResult is what a post did and what it learned of its receiver.
type PostResult struct {
	// ID is the new message's id.
	ID int64
	// Interrupted reports whether the post stopped the receiver's running turn.
	Interrupted bool
	// Waiter is the wait for the reply the options registered, nil without Wait.
	Waiter *Waiter
	// ReceiverLive reports whether the receiver had a live pane when the post was stored.
	// It is nil when creel cannot tell: for a program and for a strand of another hub.
	ReceiverLive *bool
	// Warning names a receiver that is not live and the command that starts it, and is empty for every other post.
	Warning string
}

// PostWith stores a message for receiver as opts say and answers what the post did.
// A receiver that is not live still gets the message, which waits in its mail, and the result warns of it.
// The post is validated.
// With Interrupt, the receiver must run creel's hook delivery and be one the sender may interrupt.
// Both checks run before any key is sent.
// A post that stopped a turn and then failed to store answers Interrupted beside its error.
func (s *Store) PostWith(sender Sender, receiver, body string, opts PostOptions) (PostResult, error) {
	return s.post(sender, receiver, body, opts)
}

// Post stores a message for receiver and returns its id.
func (s *Store) Post(sender Sender, receiver, body string) (int64, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{})
	return posted.ID, err
}

// PostWait stores a message for receiver and registers a wait for its reply, before any reply can arrive.
func (s *Store) PostWait(sender Sender, receiver, body string) (int64, *Waiter, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{Wait: true})
	return posted.ID, posted.Waiter, err
}

// PostInterrupt stores a message for receiver after stopping the receiver's running turn.
// The post lands as the input of the next turn, and with wait set it also registers a wait for the reply.
// It reports whether a running turn was stopped; a receiver with no running turn still gets the post.
// The post is validated.
// The receiver must run creel's hook delivery and be one the sender may interrupt, both checked before any key is sent.
// A refusal, and a post whose store fails, say what was and was not done and the way forward.
func (s *Store) PostInterrupt(sender Sender, receiver, body string, wait bool) (int64, bool, *Waiter, error) {
	posted, err := s.post(sender, receiver, body, PostOptions{Wait: wait, Interrupt: true})
	return posted.ID, posted.Interrupted, posted.Waiter, err
}

// interruptBeforePost stops the running turn of receiver for a post from sender and reports whether one was stopped.
// Every refusal leaves nothing interrupted, and its wording tells the sender what to do next.
func (s *Store) interruptBeforePost(sender Sender, receiver string) (bool, error) {
	target, err := s.strandTarget(receiver)
	if err != nil {
		return false, err
	}
	if err := s.checkMayAct(sender, target.Name, "interrupt"); err != nil {
		return false, err
	}
	if !target.CreelHooks {
		return false, fmt.Errorf("creel: %s runs no creel hook delivery, so post --interrupt cannot hand it the post; way forward: lym creel inject --interrupt %s <text>, which stops the turn and types the text, or a post without --interrupt, which the receiver gets by its typed notice or reminder", target.Name, target.Name)
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

func (s *Store) post(sender Sender, receiver, body string, opts PostOptions) (PostResult, error) {
	if opts.Interrupt {
		if err := s.refuseOtherHub(receiver, "post --interrupt"); err != nil {
			return PostResult{}, err
		}
	}
	if shortname, other := s.otherHub(receiver); other && s.hubs != nil {
		id, waiter, err := s.postOtherHub(sender, receiver, shortname, body, opts)
		return PostResult{ID: id, Waiter: waiter}, err
	}
	if err := validateLine(sender.Name, body); err != nil {
		return PostResult{}, err
	}
	var interrupted bool
	if opts.Interrupt {
		var err error
		interrupted, err = s.interruptBeforePost(sender, receiver)
		if err != nil && !interrupted {
			return PostResult{}, err
		}
		if err != nil {
			return PostResult{Interrupted: true}, storedAfterInterrupt(receiver, err)
		}
	}
	posted, err := s.storePost(sender, receiver, body, opts.Wait, opts.ExpectReply || opts.Wait)
	if err != nil && interrupted {
		return PostResult{Interrupted: true}, storedAfterInterrupt(receiver, err)
	}
	posted.Interrupted = interrupted
	return posted, err
}

// storedAfterInterrupt words a store failure that came after a turn was stopped.
func storedAfterInterrupt(receiver string, cause error) error {
	return fmt.Errorf("creel: the turn of %s was interrupted, but the post was not stored: %w; way forward: post again without --interrupt", receiver, cause)
}

// storePost journals a validated post to receiver, marked as expecting a reply when expect is set.
// With wait, it also registers the wait for the reply.
// A stored post answers its receiver's liveness from the same strand listing the receiver check read.
func (s *Store) storePost(sender Sender, receiver, body string, wait, expect bool) (PostResult, error) {
	strands, err := s.readStrands()
	if err != nil {
		return PostResult{}, err
	}
	var posted PostResult
	err = s.do(func() error {
		if err := s.checkReceiverLocked(strands, receiver); err != nil {
			return err
		}
		var err error
		if posted.ID, err = s.appendPostedLocked(sender, record{To: receiver, Kind: kindMail, Body: body, ExpectReply: expect}); err != nil {
			return err
		}
		if wait {
			if posted.Waiter, err = s.addWaiterLocked(waitKey{kind: waitReply, id: posted.ID}); err != nil {
				return err
			}
		}
		return s.deliverMailLocked(posted.ID, receiver)
	})
	if err != nil {
		return posted, err
	}
	posted.ReceiverLive, posted.Warning = s.receiverLiveness(strands, receiver)
	return posted, nil
}

// receiverLiveness reports whether receiver has a live pane among strands.
// When it has none, it also returns the warning that says so.
// A listed strand answers its own flag, and an orch name of this hub that no strand lists has no pane.
// Any other receiver is a program, whose liveness creel cannot tell, and answers nil with no warning.
func (s *Store) receiverLiveness(strands []Strand, receiver string) (*bool, string) {
	strand, listed := findStrand(strands, receiver)
	switch {
	case listed && strand.Live:
		return new(true), ""
	case listed:
		return new(false), notLiveWarning(receiver, strand.Worktree)
	case s.isHubOrchName(receiver):
		return new(false), notLiveWarning(receiver, "")
	default:
		return nil, ""
	}
}

// notLiveWarning says that receiver has no live pane, so its mail waits, and names the command that starts it.
// An orch starts with its beam, the prime's under primeBeamSlug.
// Any other strand starts with `lym reed resume` in worktree.
func notLiveWarning(receiver, worktree string) string {
	start := "lym reed resume in " + worktree
	if name, err := agentname.Parse(receiver); err == nil && agentname.MatchesRole(name.Role, agentname.RoleOrch) {
		start = "lym beam start " + cmp.Or(name.Slug, primeBeamSlug)
	}
	return fmt.Sprintf("%s is not live, so the message waits in its mail until it runs again; way forward: start it with %s", receiver, start)
}

// Reply posts text as an answer to message id, addressed to the original sender.
// The caller must hold or have read the message.
// A reply to an EXTERNAL sender is handed to the wait registered on id, and journaled undelivered when none is.
// A reply to a chase answers the message the chase chases, and only from that message's holder.
// A reply to a message from another hub posts nothing here;
// it is journaled as an outbound message to the sending strand.
// The line is validated with the longer stamped length, and the original message is marked replied.
// A sender notice from creel takes no reply, since creel holds no mailbox.
func (s *Store) Reply(sender Sender, id int64, text string) (int64, error) {
	var replyID int64
	err := s.do(func() error {
		original := s.state.messages[id]
		if original == nil {
			return fmt.Errorf("creel: message #%d is no longer in the pool", id)
		}
		if original.Holder != sender.Name {
			return fmt.Errorf("creel: message #%d is not yours to reply to: you neither hold nor have read it", id)
		}
		if original.ChaseOf != 0 {
			chased := s.state.messages[original.ChaseOf]
			if chased == nil {
				return fmt.Errorf("creel: message #%d chases #%d, which is no longer in the pool", id, original.ChaseOf)
			}
			if chased.Holder != sender.Name {
				return fmt.Errorf("creel: message #%d chases #%d, which %s holds now; way forward: the strand that holds #%d answers it, as lym creel log --id %d shows", id, chased.ID, chased.Holder, chased.ID, chased.ID)
			}
			original, id = chased, chased.ID
		} else if isChaserName(original.From) {
			return fmt.Errorf("creel: message #%d is a creel notice and takes no reply; way forward: post to the strand it names with lym creel post <strand> \"<text>\"", id)
		}
		if original.SendingHub != "" {
			if err := validateStampedLine(sender.Name, text); err != nil {
				return err
			}
			answerID := s.state.newID()
			if err := s.writer.append(record{
				Event: eventOutbound, ID: answerID, From: sender.Name, To: original.From, ReceivingHub: original.SendingHub,
				Kind: kindReply, Body: text, ReplyTo: original.SendingID, Answers: id,
			}); err != nil {
				return err
			}
			replyID = answerID
			s.wakeOutboundLocked(original.SendingHub)
			return nil
		}
		validate := validateBody
		if !isExternalName(original.From) {
			validate = func(text string) error { return validateLine(sender.Name, text) }
		}
		if err := validate(text); err != nil {
			return err
		}
		var err error
		if replyID, err = s.appendPostedLocked(sender, record{To: original.From, Kind: kindReply, Body: text, ReplyTo: id}); err != nil {
			return err
		}
		reply := *s.state.messages[replyID]
		if !isExternalName(original.From) {
			// A strand's or program's wait ends with the reply, which still reaches it as mail.
			s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeReply, Message: reply})
			return s.deliverMailLocked(replyID, original.From)
		}
		if s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeReply, Message: reply}) > 0 {
			return s.writer.append(record{Event: eventDelivered, ID: replyID})
		}
		return s.writer.append(record{Event: eventUndelivered, ID: replyID})
	})
	return replyID, err
}

// EchoHeading is the line hook-read puts above the echoed lines.
// The echo reaches the operator alone.
// It is mail an idle ring already put in the model's context, shown at the woken turn's first claim.
const EchoHeading = "[creel] Delivered while idle:"

// echoItem is one line of ring mail the operator is still owed, with the id of the message it carries.
// reserved is true while a claim that handed the line out has not yet been acknowledged or released.
type echoItem struct {
	id       int64
	line     string
	reserved bool
}

// Reading is the set of messages a read or a claim handed out.
// They are unread messages, reserved until Ack or Release.
type Reading struct {
	// Messages are the messages handed out, oldest first.
	Messages []Message
	// Echo are the echoed lines a claim handed out ahead of its messages, oldest first.
	Echo   []string
	store  *Store
	holder string
	ids    []int64
	// echoIDs are the ids of the echo items handed out, kept after the reading settles them so its log line can name them.
	// echoSettled is true once the reading has settled them.
	echoIDs     []int64
	echoSettled bool
	// path names the claim that made the reading for its log line: tool, stop or ring.
	// A read's reading has none and logs nothing.
	path string
	// read is true for a read verb's reading, whose client already printed the mail, and false for a claim's.
	read bool
	// ring is true for the claim an idle waiter's ring carries, whose acknowledgement answers that ring.
	ring bool
}

// Read returns the caller's unread mail, oldest first, that no other read has reserved.
// An empty `from` returns every message; otherwise only that sender's mail is returned.
// They are reserved so a concurrent read by the same holder skips them.
// A program sender's first read makes its mailbox known.
func (s *Store) Read(sender Sender, from string) (*Reading, error) {
	if sender.Kind == KindExternal {
		return nil, errExternalHoldsNothing
	}
	reading := &Reading{store: s, holder: sender.Name, read: true}
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

// boundaryClaimBudget bounds the bytes one tool-boundary claim hands out as message lines, newlines included.
// A boundary therefore adds a bounded amount to the session's context.
const boundaryClaimBudget = 8 << 10

// Boundary names the hook that asks for a claim.
type Boundary struct {
	// Stop is true for the turn-end Stop hook and false for the tool-boundary hook.
	Stop bool
	// StopHookActive is the Stop hook input's stop_hook_active.
	// It is true when an earlier Stop claim that handed out mail already continued the turn.
	StopHookActive bool
}

// stopBlockCap bounds how many Stop claims in a row may hand out mail and continue one turn.
// It stays below the continuation limit Claude Code allows before it overrides a Stop hook.
const stopBlockCap = 6

// ClaimAtBoundary hands a strand's tool-boundary hook the strand's unread mail, oldest first.
// It takes only mail that no read has reserved, agent mail included.
// What it hands out stays reserved until Ack or Release, as a read's messages do.
// Under a hold it hands out only the hold owner's mail, and while a pause covers the strand it hands out nothing.
// It never claims a message whose MessageLine is longer than hookFieldCap UTF-16 units.
// Such a message stays unread for `lym creel read`.
// It stops before a message that would take the lines past boundaryClaimBudget.
// That count includes the MessageLine and the newline joining it to the line before.
// It always hands out the first message, and what it leaves stays unread for the next boundary.
// A Stop claim hands out nothing once stopBlockCap Stop claims in the strand's current turn have handed out mail.
// It also hands out nothing when no Stop or tool-boundary claim of this daemon has set its count,
// so the turn end is recorded.
// A claim that hands out at least one message also marks the strand's last ring answered.
// A strand owed an echo gets its unreserved echo lines first, oldest first.
// They are handed out even when a pause, a hold or the Stop cap keeps its mail back.
// The echo and the messages share boundaryClaimBudget, and an echo alone continues no turn.
// Only a strand sender may claim; any other sender is refused with the read verb as the way forward.
func (s *Store) ClaimAtBoundary(sender Sender, boundary Boundary) (*Reading, error) {
	if sender.Kind != KindStrand {
		return nil, fmt.Errorf("creel: only a strand's tool-boundary hook claims mail, and %s is no strand; read with `lym creel read`", sender.Name)
	}
	path := "tool"
	if boundary.Stop {
		path = "stop"
	}
	reading := &Reading{store: s, holder: sender.Name, path: path}
	strand, err := s.strandTarget(sender.Name)
	if err != nil {
		return nil, err
	}
	paused, err := s.delivery.Paused(strand)
	if err != nil {
		return nil, fmt.Errorf("creel: could not read the pause record for %s: %w", sender.Name, err)
	}
	// reason names why the mail steps handed no mail out, for the Debug line of a claim that handed out nothing at all.
	reason := ""
	err = s.do(func() error {
		s.claimEchoLocked(sender.Name, reading)
		if paused {
			reason = "paused"
			return nil
		}
		if boundary.Stop && !boundary.StopHookActive {
			s.stopBlocks[sender.Name] = 0
		}
		if boundary.Stop {
			blocks, known := s.stopBlocks[sender.Name]
			switch {
			case !known:
				reason = "stop-count-unknown"
				return nil
			case blocks >= stopBlockCap:
				reason = "stop-cap"
				return nil
			}
		}
		s.claimLocked(sender.Name, reading)
		if len(reading.ids) == 0 {
			reason = s.nothingClaimableReasonLocked(sender.Name)
			return nil
		}
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(reading.ids) == 0 && len(reading.echoIDs) == 0 {
		// A standing flag runs the claim at every tool call, so an empty claim logs at Debug.
		logger.Debug("creel claim", "strand", sender.Name, "path", path, "outcome", "nothing", "reason", reason)
	}
	return reading, nil
}

// nothingClaimableReasonLocked names why a claim found no mail for holder.
// It returns "held" when a live hold covers the holder's unread mail and none of it is from the hold owner.
// Otherwise it returns "nothing-claimable".
func (s *Store) nothingClaimableReasonLocked(holder string) string {
	if hold := s.liveHoldLocked(holder); hold != nil {
		unread := s.unreadLocked(holder, "")
		if len(unread) > 0 && !slices.ContainsFunc(unread, func(msg *Message) bool { return msg.From == hold.Owner }) {
			return "held"
		}
	}
	return "nothing-claimable"
}

// claimEchoLocked reserves to reading the holder's unreserved echo items, oldest first.
// It stops before the line that would take the echo lines past boundaryClaimBudget, but always takes the first.
func (s *Store) claimEchoLocked(holder string, reading *Reading) {
	items := s.echoes[holder]
	used := 0
	for i := range items {
		if items[i].reserved {
			continue
		}
		size := len(items[i].line)
		if len(reading.Echo) > 0 {
			size++
			if used+size > boundaryClaimBudget {
				break
			}
		}
		used += size
		items[i].reserved = true
		reading.Echo = append(reading.Echo, items[i].line)
		reading.echoIDs = append(reading.echoIDs, items[i].id)
	}
}

// echoBytes is the byte length of lines joined by newlines.
func echoBytes(lines []string) int {
	total := 0
	for i, line := range lines {
		if i > 0 {
			total++
		}
		total += len(line)
	}
	return total
}

// claimLocked reserves to reading the holder's claimable messages, oldest first.
// It stops before a message whose line would take reading's echo and message lines past boundaryClaimBudget.
// The joining newline counts toward the budget too.
// It always takes the first line, echo or message.
func (s *Store) claimLocked(holder string, reading *Reading) {
	used := echoBytes(reading.Echo)
	for _, msg := range s.claimableLocked(holder) {
		size := len(MessageLine(*msg))
		if len(reading.ids) > 0 || len(reading.Echo) > 0 {
			// The newline the hook joins this line to the one before with counts against the budget too.
			size++
			if used+size > boundaryClaimBudget {
				break
			}
		}
		used += size
		s.reserved[msg.ID] = holder
		reading.ids = append(reading.ids, msg.ID)
		reading.Messages = append(reading.Messages, *msg)
	}
}

// claimableLocked returns the unread messages holder may claim, oldest first, with no byte budget applied.
// It excludes messages a read has reserved.
// Under a live hold it keeps only the hold owner's messages.
// It also excludes any message whose line passes hookFieldCap.
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

// resetStopBlocks sets the Stop count of strand back to zero, adding its entry if none exists.
func (s *Store) resetStopBlocks(strand string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopBlocks[strand] = 0
}

// HookWait registers the idle waiter of the calling strand.
// The daemon answers the waiter with OutcomeRing when it wakes the idle strand.
// With takesMail the waiter is marked: its ring carries the strand's claimed mail, which the waiter acknowledges.
// A newer waiter of the same strand ends the older one.
// The older waiter is answered OutcomeSuperseded before the new one registers.
// Only a strand sender may wait; any other sender is refused with the way forward for its kind.
func (s *Store) HookWait(sender Sender, takesMail bool) (*Waiter, error) {
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
			waiter.takesMail = takesMail
			s.wakeLocked(sender.Name)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return waiter, nil
}

// hookWakeState reads the state the ring decision for strand depends on.
// It returns the ids of strand's claimable messages, the mark of its last ring, and whether a hook waiter is registered.
func (s *Store) hookWakeState(strand string) (ids []int64, mark wakeMark, waiting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.claimableLocked(strand) {
		ids = append(ids, msg.ID)
	}
	return ids, s.wakeMarks[strand], len(s.waiters[waitKey{kind: waitHook, holder: strand}]) > 0
}

// hookWaiterTakesMail reports whether a registered idle waiter of strand is marked as taking mail.
func (s *Store) hookWaiterTakesMail(strand string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return anyTakesMail(s.waiters[waitKey{kind: waitHook, holder: strand}])
}

// anyTakesMail reports whether one of waiters is marked as taking mail.
func anyTakesMail(waiters []*Waiter) bool {
	for _, waiter := range waiters {
		if waiter.takesMail {
			return true
		}
	}
	return false
}

// ringHook answers strand's registered waiters with OutcomeRing and records the mark of the ring at now.
// It rings only when the claimable set is still ids and a waiter is still registered.
// A marked waiter's ring carries a claim of the set through claimLocked, in a Reading the waiter acknowledges.
// That claim is made only when the pause read just before the locked step ran for a marked waiter and found no pause.
// A pause, a failed pause read, or a marked waiter that registered after the read means the ring is not made.
// The waiter then stays registered.
// A pause that keeps a marked ring from claiming logs at Debug.
// The ring's claim logs its outcome like any other claim.
// It reports whether it rang, and starts the strand's Stop count over when it did.
func (s *Store) ringHook(strand Strand, ids []int64, now time.Time) bool {
	pauseRead := s.hookWaiterTakesMail(strand.Name)
	if pauseRead {
		if paused, err := s.delivery.Paused(strand); err != nil || paused {
			logger.Debug("creel claim", "strand", strand.Name, "path", "ring", "outcome", "nothing", "reason", "paused")
			return false
		}
	}
	rang := false
	_ = s.do(func() error {
		var current []int64
		for _, msg := range s.claimableLocked(strand.Name) {
			current = append(current, msg.ID)
		}
		if !slices.Equal(current, ids) {
			return nil
		}
		key := waitKey{kind: waitHook, holder: strand.Name}
		waiters := s.waiters[key]
		if len(waiters) == 0 {
			return nil
		}
		outcome := Outcome{Kind: OutcomeRing}
		if anyTakesMail(waiters) {
			if !pauseRead {
				return nil
			}
			reading := &Reading{store: s, holder: strand.Name, ring: true, path: "ring"}
			s.claimLocked(strand.Name, reading)
			outcome.Reading = reading
		}
		rang = s.answerLocked(key, outcome) > 0
		s.wakeMarks[strand.Name] = wakeMark{ids: ids, at: now}
		return nil
	})
	if rang {
		s.resetStopBlocks(strand.Name)
	}
	return rang
}

var errExternalHoldsNothing = errors.New("creel: an EXTERNAL caller holds no mail; wait for an answer with `lym creel post --wait-reply`")

// journalReadLocked journals msg as read by holder.
// When msg expects a reply, it wakes holder, so the message's chase clock starts at the read.
// The read message then gives the removed-holder sweep something to watch.
func (s *Store) journalReadLocked(msg *Message, holder string) error {
	if err := s.writer.append(record{Event: eventRead, ID: msg.ID, By: holder}); err != nil {
		return err
	}
	if msg.ExpectReply {
		s.wakeLocked(holder)
	}
	return nil
}

// Ack marks the reserved messages read and journals each one; a message that left the holder meanwhile stays as it is.
// Once every id is journaled, the Ack of a ring's claim marks the strand's last ring answered.
// When a journal write fails, a claim's Ack releases the id it failed on and every id after it,
// so a later claim hands them out once more, and answers the holder's mail waiters;
// a read's Ack leaves them reserved, since its client already printed the mail.
// The ids before the failing one stay journaled read, and the error is returned.
// A claim's Ack drops the echo items it handed out whatever the journal writes do.
// The hook has already shown them.
// A ring's Ack appends one echo item per id it passed to the strand's echo.
// The item goes in whether the id was journaled read or skipped as no longer the holder's unread mail.
// It appends none from a failing id on.
// The Ack of a claim or a ring that handed something out logs its outcome once.
func (r *Reading) Ack() error {
	return r.store.do(func() error {
		var dropped []int64
		if len(r.echoIDs) > 0 && !r.echoSettled {
			dropped = r.echoIDs
		}
		changedEcho := r.settleEchoLocked(true)
		defer func() {
			if changedEcho {
				r.store.syncFlagsLocked()
			}
		}()
		echo := func(i int) {
			if r.ring {
				r.store.echoes[r.holder] = append(r.store.echoes[r.holder], echoItem{id: r.ids[i], line: MessageLine(r.Messages[i])})
				changedEcho = true
			}
		}
		for i, id := range r.ids {
			msg := r.store.state.messages[id]
			if msg == nil || msg.Holder != r.holder || msg.Status != StatusUnread {
				delete(r.store.reserved, id)
				echo(i)
				continue
			}
			if err := r.store.journalReadLocked(msg, r.holder); err != nil {
				var released []int64
				if !r.read {
					released = slices.Clone(r.ids[i:])
					for _, unjournaled := range r.ids[i:] {
						delete(r.store.reserved, unjournaled)
					}
					r.store.answerLocked(waitKey{kind: waitHolder, holder: r.holder}, Outcome{Kind: OutcomeMail})
					r.ids = nil
				}
				r.logOutcome(claimAckFailed, released, dropped, err)
				return err
			}
			delete(r.store.reserved, id)
			echo(i)
		}
		if r.ring {
			if mark, rung := r.store.wakeMarks[r.holder]; rung && !mark.at.IsZero() {
				mark.answered = true
				r.store.wakeMarks[r.holder] = mark
			}
		}
		if len(r.Messages) > 0 || len(r.echoIDs) > 0 {
			r.logOutcome(claimAcknowledged, nil, nil, nil)
		}
		r.ids = nil
		return nil
	})
}

// The outcomes a delivered claim's log line carries.
const (
	claimAcknowledged = "acknowledged"
	claimAckFailed    = "ack-failed"
	claimReleased     = "released"
)

// logOutcome writes the one daemon log line of this claim's delivery attempt.
// The line carries the strand, the path that claimed (tool, stop or ring), the message ids and echo ids handed out, and the outcome.
// An acknowledged or released claim logs at Info.
// A claim whose journal write failed logs at Warn, with the error, the message ids released for a later claim, and the echo ids dropped.
// A read's reading has no path and logs nothing.
func (r *Reading) logOutcome(outcome string, released, dropped []int64, err error) {
	if r.path == "" {
		return
	}
	ids := make([]int64, len(r.Messages))
	for i, msg := range r.Messages {
		ids[i] = msg.ID
	}
	attrs := []any{"strand", r.holder, "path", r.path, "ids", ids, "echo_ids", r.echoIDs, "outcome", outcome}
	if outcome == claimAckFailed {
		logger.Warn("creel claim", append(attrs, "error", err, "released_ids", released, "dropped_echo_ids", dropped)...)
		return
	}
	logger.Info("creel claim", attrs...)
}

// settleEchoLocked ends this reading's hold on the echo items it handed out, once.
// With drop set it drops them; otherwise it leaves them owed and unreserved.
// It reports whether it settled any echo items, and syncs nothing; the caller does.
func (r *Reading) settleEchoLocked(drop bool) bool {
	if len(r.echoIDs) == 0 || r.echoSettled {
		return false
	}
	r.echoSettled = true
	var kept []echoItem
	for _, item := range r.store.echoes[r.holder] {
		if item.reserved && slices.Contains(r.echoIDs, item.id) {
			if drop {
				continue
			}
			item.reserved = false
		}
		kept = append(kept, item)
	}
	if len(kept) == 0 {
		delete(r.store.echoes, r.holder)
	} else {
		r.store.echoes[r.holder] = kept
	}
	return true
}

// Release drops the reservation and leaves the messages unread.
// A wait for the holder's mail is answered, since the mail is back.
// The echo items the claim handed out are unreserved, so they stay owed.
// A release of a claim or a ring that still held ids or echo items logs its outcome once.
func (r *Reading) Release() {
	_ = r.store.do(func() error {
		held := len(r.ids) > 0 || (len(r.echoIDs) > 0 && !r.echoSettled)
		if r.settleEchoLocked(false) {
			r.store.syncFlagsLocked()
		}
		for _, id := range r.ids {
			delete(r.store.reserved, id)
		}
		if len(r.ids) > 0 {
			r.store.answerLocked(waitKey{kind: waitHolder, holder: r.holder}, Outcome{Kind: OutcomeMail})
		}
		if held {
			r.logOutcome(claimReleased, nil, nil, nil)
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
		from := ""
		if sent := s.state.outbound[id]; sent != nil {
			from = sent.From
		} else if msg := s.state.messages[id]; msg != nil {
			from = msg.From
		}
		if from != sender.Name {
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

// InboxResult holds the unread counts per sender of the caller's mail.
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

// Forward moves an unread message the caller holds to receiver.
// The holder changes, From stays, and the caller joins the message's path.
// A message from another hub is forwarded only to a strand of this hub.
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
		case msg.ChaseOf != 0:
			return fmt.Errorf("creel: message #%d is a chase and cannot be forwarded; way forward: answer it with lym creel post --reply %d \"<text>\", which answers #%d", id, id, msg.ChaseOf)
		case msg.Status != StatusUnread:
			return fmt.Errorf("creel: message #%d is already read and cannot be forwarded", id)
		case receiver == sender.Name:
			return fmt.Errorf("creel: message #%d cannot be forwarded to yourself", id)
		}
		if msg.SendingHub != "" {
			if _, other := s.otherHub(receiver); other || !isStrandName(receiver) {
				return fmt.Errorf("creel: message #%d came from another hub and is forwarded only to a strand of this hub; way forward: post it anew with lym creel post %s \"<text>\"", id, receiver)
			}
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

// Log returns the journal events matching the filter, oldest first.
// Each event carries its ids, parties, time and subject, and never a body.
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

// InjectResult reports the id of a queued inject.
// For an inject that interrupts, it reports whether a running turn was stopped.
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

// InjectWait queues text to be typed into receiver's pane at its next idle, behind earlier injects.
// It also registers a wait for the inject's outcome, before any outcome can happen.
func (s *Store) InjectWait(sender Sender, receiver, text string, ttl time.Duration, interrupt bool) (InjectResult, *Waiter, error) {
	return s.inject(sender, receiver, text, Command{}, ttl, interrupt, true)
}

// InjectCommand queues a command to run in an orch strand's session at its next idle, behind earlier injects.
// It follows every rule that applies to a text inject.
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
	if err := s.refuseOtherHub(receiver, "inject"); err != nil {
		return InjectResult{}, nil, err
	}
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

// Queued returns the injects waiting for receiver in the order they will be typed:
// the interrupt-queued ones first, newest first, then the rest oldest first.
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
// A privileged sender (EXTERNAL, a program, or an orch strand) may act on any strand, an orch strand included.
// Any other sender may act only on its own strand.
func (s *Store) checkMayAct(sender Sender, receiver, action string) error {
	if !sender.privileged() && sender.Name != receiver {
		return fmt.Errorf("creel: %s may not %s another strand's pane; send it mail with `lym creel post`", sender.Name, action)
	}
	return nil
}

// Interrupt sends the guarded interrupt to receiver at once, bypassing the queue, and reports whether a running turn was stopped.
// A receiver with no running turn gets nothing and false.
// A receiver a pause covers is claimed into that pause and then interrupted,
// so the pause's resume gives it its resume line;
// an unreadable pause record and another sender's hold refuse the interrupt.
func (s *Store) Interrupt(sender Sender, receiver string) (bool, error) {
	if err := s.refuseOtherHub(receiver, "interrupt"); err != nil {
		return false, err
	}
	target, err := s.strandTarget(receiver)
	if err != nil {
		return false, err
	}
	if err := s.checkMayAct(sender, target.Name, "interrupt"); err != nil {
		return false, err
	}
	return s.interruptTarget(sender, target)
}

// interruptTarget runs the checks that follow the privilege check, in order:
// another sender's hold, then the claim of a covering pause, then the interrupt itself.
// A strand a live pause covers is recorded in that pause before the key,
// so the pause's resume gives it its resume line;
// an unreadable pause record refuses with nothing recorded, and so does a pause lifted before the key.
// The interrupt runs under the receiver's pane lock, so no delivery reaches the receiver meanwhile,
// and the receiver's worker is woken once it ends.
// An interrupt whose key was sent and whose clear failed counts as sent.
// A hold, a lifted pause and an unreadable pause record each refuse with an *interruptRefusal of their kind;
// its message names the interrupt.
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

// interruptRefusal is a refusal of an interrupt that sent no key.
// It carries its kind, so a caller can word the refusal differently, while Error keeps the message.
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

// HoldResult is the outcome of holding a strand:
// the hold's owner and expiry, and whether it renewed the owner's earlier hold.
type HoldResult struct {
	Owner   string
	Expires time.Time
	Renewed bool
}

// Hold holds receiver's deliveries for the given duration: only the owner's injects pass.
// Only the owner's mail is delivered:
// through the receiver's hooks, or by its notice or reminder when it is a pane-mail strand.
// The same owner holding again renews the hold; another sender's hold on a held receiver is refused.
// An orch strand is held by a program only.
func (s *Store) Hold(sender Sender, receiver string, duration time.Duration) (HoldResult, error) {
	if err := s.refuseOtherHub(receiver, "hold"); err != nil {
		return HoldResult{}, err
	}
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

// WithdrawResult reports what a withdrawal removed.
// It holds the ids of the injects expired and the messages returned undelivered, both in id order.
type WithdrawResult struct {
	Injects  []int64
	Messages []WithdrawnMessage
}

// Withdraw removes the caller's own queued injects to receiver and its own unread, unreserved mail held by receiver.
// Each inject is journaled expired and each message undelivered, with reason `WithdrawnReason`.
// A wait on an inject is answered expired, and a wait on a message's reply is answered returned.
// An inject already sending and a message already read or being read are left alone.
// Only a program may withdraw; any other sender gets an error.
// It acts on receiver by name, whether or not creel still lists that strand.
// With nothing queued, the result is empty.
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

// Release ends the caller's hold on receiver and reports whether there was one, even when the strand is gone.
// Another sender's hold is refused.
// With no live hold, a name that is no existing strand is refused, and the error lists the strands that exist.
// With no live hold on an existing strand, it changes nothing.
func (s *Store) Release(sender Sender, receiver string) (bool, error) {
	if err := s.refuseOtherHub(receiver, "release"); err != nil {
		return false, err
	}
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
