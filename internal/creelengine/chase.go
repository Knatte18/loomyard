// chase.go chases a message that expects a reply and whose holder read it without answering: the chase clock, the two chases it sends and the unanswered outcome.

package creelengine

import (
	"fmt"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// chaseInterval is the running time a holder gets after a message was read, and after each chase, before the next step.
	chaseInterval = 15 * time.Minute
	// maxChases is how many chases one message gets before it is settled unanswered.
	maxChases = 2
	// chaseProbeCeiling is the longest a chaser waits between probes while the holder's clock is stopped.
	chaseProbeCeiling = 8 * time.Minute
	// ChaserProgram is the program name creel signs its chases and sender notices with.
	ChaserProgram = "CREEL"
)

// Journal events of the chase clock.
const (
	// eventChased records chase number Chase of the message in ID.
	eventChased = "chased"
	// eventChaseAged records the chase clock of the message in ID, in ConsumedSeconds.
	eventChaseAged = "chase-aged"
	// eventUnanswered records that the message in ID was settled without a reply, for the reason in Reason.
	eventUnanswered = "unanswered"
)

// The reasons a message is settled unanswered.
const (
	// UnansweredNoReply means the holder read the message and neither of its chases drew a reply.
	UnansweredNoReply = "no-reply"
	// UnansweredHolderGone means the holder was removed after reading the message, with no reply.
	UnansweredHolderGone = "holder-gone"
)

// isChaseable reports whether the chase clock runs on msg: it expects a reply, its strand holder read it and has not answered it, and no unanswered outcome is recorded.
// A chase carries no mark, so it is never chaseable itself, and a message a program holds is never chased.
func isChaseable(msg *Message) bool {
	return msg.ExpectReply && msg.Status == StatusRead && !msg.Replied && msg.Unanswered == "" && isStrandName(msg.Holder)
}

// chaseableLocked returns the chaseable messages holder holds, oldest first.
func (s *Store) chaseableLocked(holder string) []*Message {
	var found []*Message
	for _, msg := range s.state.messages {
		if msg.Holder == holder && isChaseable(msg) {
			found = append(found, msg)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	return found
}

// chaseSubjects returns the strands that hold a chaseable message, in name order.
func (s *Store) chaseSubjects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, msg := range s.state.messages {
		if isChaseable(msg) {
			seen[msg.Holder] = true
		}
	}
	holders := make([]string, 0, len(seen))
	for holder := range seen {
		holders = append(holders, holder)
	}
	sort.Strings(holders)
	return holders
}

// chasePending reports whether holder holds a chaseable message.
func (s *Store) chasePending(holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.state.messages {
		if msg.Holder == holder && isChaseable(msg) {
			return true
		}
	}
	return false
}

// seedChaseClock records what the journal already holds of the chase clocks, as the store opens.
// A chaseable message whose next step was already due keeps the clock at this opening as the base of that step, so overdue steps go one interval of running time apart after the restart.
func (s *Store) seedChaseClock() {
	for id, msg := range s.state.messages {
		if !isChaseable(msg) {
			continue
		}
		s.chaseWritten[id] = int64(msg.ChaseConsumed / time.Second)
		if msg.ChaseConsumed >= msg.ChaseMark+chaseInterval {
			s.chaseBase[id] = msg.ChaseConsumed
		}
	}
}

// repairChases writes the chased record a crash between a chase's post and its chased record left missing.
func (s *Store) repairChases() error {
	return s.do(func() error {
		var ids []int64
		for id, msg := range s.state.messages {
			if msg.Chases > s.state.chased[id] {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			msg := s.state.messages[id]
			for number := s.state.chased[id] + 1; number <= msg.Chases; number++ {
				if err := s.writer.append(record{Event: eventChased, ID: id, To: msg.Holder, Chase: number}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ageChases adds elapsed running time to the chase clock of each chaseable message holder holds.
// A message that another sender's live hold stands on is left as it is; a hold by the message's own sender lets the clock run.
func (s *Store) ageChases(holder string, elapsed time.Duration) error {
	return s.do(func() error {
		hold := s.liveHoldLocked(holder)
		for _, msg := range s.chaseableLocked(holder) {
			if hold != nil && hold.Owner != msg.From {
				continue
			}
			msg.ChaseConsumed += elapsed
		}
		return nil
	})
}

// persistChaseAged journals the chase clock of each chaseable message holder holds whose whole seconds changed since its last chase-aged record.
func (s *Store) persistChaseAged(holder string) error {
	return s.do(func() error {
		for _, msg := range s.chaseableLocked(holder) {
			seconds := int64(msg.ChaseConsumed / time.Second)
			if seconds == s.chaseWritten[msg.ID] {
				continue
			}
			if err := s.writer.append(record{Event: eventChaseAged, ID: msg.ID, ConsumedSeconds: seconds}); err != nil {
				return err
			}
			s.chaseWritten[msg.ID] = seconds
		}
		return nil
	})
}

// stepChases takes the chase steps that are due for holder's chaseable messages: a step is due when the message's clock reaches its base plus chaseInterval.
// Steps one and two send a chase; the step after the second settles the message unanswered.
func (s *Store) stepChases(holder string) error {
	listing := s.listStrands()
	return s.do(func() error {
		for _, msg := range s.chaseableLocked(holder) {
			base := msg.ChaseMark
			if overdue, ok := s.chaseBase[msg.ID]; ok {
				base = overdue
			}
			if msg.ChaseConsumed < base+chaseInterval {
				continue
			}
			delete(s.chaseBase, msg.ID)
			var err error
			if msg.Chases >= maxChases {
				err = s.settleUnansweredLocked(msg.ID, UnansweredNoReply, listing)
			} else {
				err = s.postChaseLocked(msg)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// chaserSender returns the sender creel signs a chase or a sender notice with.
func (s *Store) chaserSender() (Sender, error) {
	name, err := agentname.FormatProgram(s.geom.Shortname, ChaserProgram)
	if err != nil {
		return Sender{}, fmt.Errorf("creel: cannot name the chaser: %w", err)
	}
	return ProgramSender(name), nil
}

// postChaseLocked sends the next chase of msg to its holder and journals it.
// The body names the message, its sender and the reply command, and carries no text the sender wrote, so a chase cannot pose as an instruction.
func (s *Store) postChaseLocked(msg *Message) error {
	sender, err := s.chaserSender()
	if err != nil {
		return err
	}
	number := msg.Chases + 1
	body := fmt.Sprintf("Chase %d of %d: message #%d from %s was read and has not been answered; answer it with: lym creel post --reply %d \"<text>\"", number, maxChases, msg.ID, msg.From, msg.ID)
	id, err := s.appendPostedLocked(sender, record{
		To: msg.Holder, Kind: kindMail, Body: body,
		Chase: number, Chased: msg.ID, ConsumedSeconds: int64(msg.ChaseConsumed / time.Second),
	})
	if err != nil {
		return err
	}
	if err := s.writer.append(record{Event: eventChased, ID: msg.ID, To: msg.Holder, Chase: number}); err != nil {
		return err
	}
	return s.deliverMailLocked(id, msg.Holder)
}

// strandListing is one reading of the hub's strands, taken before the store mutex so a slow listing never stalls a store verb.
type strandListing struct {
	strands []Strand
	err     error
}

// listStrands reads the hub's strands once and keeps a failure in the listing for the notice that needs it.
func (s *Store) listStrands() strandListing {
	strands, err := s.readStrands()
	return strandListing{strands: strands, err: err}
}

// settleUnansweredLocked journals that message id ended without a reply, tells its sender when the sender will read the notice, and answers every wait on the message's reply.
// The listing, read before the mutex was taken, decides whether a strand sender is still listed.
// A notice that fails to store is logged and never undoes the record.
func (s *Store) settleUnansweredLocked(id int64, reason string, listing strandListing) error {
	msg := s.state.messages[id]
	if err := s.writer.append(record{Event: eventUnanswered, ID: id, To: msg.Holder, Reason: reason}); err != nil {
		return err
	}
	if err := s.noticeUnansweredLocked(msg, reason, listing); err != nil {
		logger.Warn("creel chase: could not post the unanswered notice", "id", id, "sender", msg.From, "error", err)
	}
	s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeUnanswered, Message: *msg})
	return nil
}

// noticeUnansweredLocked posts msg's sender a notice that the message got no reply.
// Only a strand the delivery seam lists and a program that has read get one; EXTERNAL, a program that never read and a strand no longer listed get none.
func (s *Store) noticeUnansweredLocked(msg *Message, reason string, listing strandListing) error {
	if isStrandName(msg.From) {
		if listing.err != nil {
			return listing.err
		}
		if _, listed := findStrand(listing.strands, msg.From); !listed {
			return nil
		}
	} else if _, read := s.state.readers[msg.From]; !read {
		return nil
	}
	sender, err := s.chaserSender()
	if err != nil {
		return err
	}
	why := fmt.Sprintf("was read and chased %d times with no reply", msg.Chases)
	if reason == UnansweredHolderGone {
		why = "got no reply: its holder is gone"
	}
	body := fmt.Sprintf("Message #%d to %s %s; way forward: lym creel log --id %d shows its path, then ask again or decide without the answer", msg.ID, msg.Holder, why, msg.ID)
	id, err := s.appendPostedLocked(sender, record{To: msg.From, Kind: kindMail, Body: body})
	if err != nil {
		return err
	}
	return s.deliverMailLocked(id, msg.From)
}

// chaser ticks the chase clock of one holder's chaseable messages and takes the steps that fall due.
type chaser struct {
	scheduler *Scheduler
	holder    string
	// wake tells the chaser that mail reached the holder or that a message of the holder's was read.
	wake chan struct{}
	// lastError holds the last error logged per kind, so a repeated one logs once.
	lastError map[string]string
}

// run probes the holder once when it starts and counts that reading as the previous tick's.
// While the clock runs it ticks every agedPersistInterval, and a tick whose probe and the previous one's both read the clock running charges one whole interval and takes the steps that fall due.
// While the clock is stopped each tick doubles the next wait up to chaseProbeCeiling, and charges nothing.
// A running tick, or a wake of the holder during a backed-off wait, returns the wait to agedPersistInterval; a wake during a running wait keeps the tick's deadline.
// The chaser ends when the holder holds nothing chaseable, and journals the clock it counted when the scheduler stops.
func (c *chaser) run() {
	defer c.scheduler.wg.Done()
	store, clock := c.scheduler.store, c.scheduler.clock
	running := c.probe()
	wait := agedPersistInterval
	timer := clock.After(wait)
	for {
		select {
		case <-c.scheduler.stop:
			c.report("journal", store.persistChaseAged(c.holder))
			return
		case <-c.wake:
			if c.retire() {
				return
			}
			if !running {
				wait = agedPersistInterval
				timer = clock.After(wait)
			}
		case <-timer:
			now := c.probe()
			if now && running {
				c.report("journal", store.ageChases(c.holder, agedPersistInterval))
				c.report("journal", store.stepChases(c.holder))
			}
			c.report("journal", store.persistChaseAged(c.holder))
			running = now
			if now {
				wait = agedPersistInterval
			} else {
				wait = min(wait*2, chaseProbeCeiling)
			}
			if c.retire() {
				return
			}
			timer = clock.After(wait)
		}
	}
}

// retire removes the chaser once its holder holds nothing chaseable; it reports false when something chaseable remains and the chaser carries on.
func (c *chaser) retire() bool {
	scheduler := c.scheduler
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.store.chasePending(c.holder) && !scheduler.stopping {
		return false
	}
	delete(scheduler.chasers, c.holder)
	return true
}

// probe reports whether the holder's clock runs: the holder is listed, live and no pause covers it.
// An error reading the strands or the pause counts as stopped.
func (c *chaser) probe() bool {
	delivery := c.scheduler.delivery
	strands, err := delivery.Strands()
	c.report("strands", err)
	if err != nil {
		return false
	}
	strand, found := findStrand(strands, c.holder)
	if !found || !strand.Live {
		return false
	}
	paused, err := delivery.Paused(strand)
	c.report("pause", err)
	return err == nil && !paused
}

// report logs err once per change of error for kind, and clears the kind when err is nil.
func (c *chaser) report(kind string, err error) {
	if err == nil {
		delete(c.lastError, kind)
		return
	}
	if c.lastError[kind] == err.Error() {
		return
	}
	c.lastError[kind] = err.Error()
	logger.Warn("creel chase: "+kind+" failed", "holder", c.holder, "error", err)
}
