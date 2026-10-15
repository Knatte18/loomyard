// sweep.go runs the removed-holder sweep: mail held by a strand that no reed state holds any more goes back along its path, and injects queued for it expire.

package creelengine

import (
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// sweepBackoffBase is the wait between sweeps after a change.
	sweepBackoffBase = 30 * time.Second
	// sweepBackoffCeiling is the longest the wait between sweeps grows to while the strand set holds.
	sweepBackoffCeiling = 4 * time.Minute
	// goneAfterSweeps is how many successful sweeps in a row must miss a holder before it counts as gone.
	goneAfterSweeps = 2
)

// sweeper is the sweep loop's memory between sweeps.
type sweeper struct {
	// absent counts the successful sweeps in a row that found a holder in no reed state.
	absent    map[string]int
	names     string
	haveNames bool
	lastError string
}

// sweepLoop sweeps while unread mail or a queued inject exists.
// The wait starts at 30 seconds and doubles to a 4-minute ceiling while the strand-name set holds, and it returns to the base on a change of that set, a new post or inject, or a failed sweep.
func (s *Scheduler) sweepLoop() {
	defer s.wg.Done()
	state := &sweeper{absent: map[string]int{}}
	wait := sweepBackoffBase
	var seq uint64
	// idle is set while nothing needs sweeping, so the mail that makes sweeping needed starts a period at its own sequence number.
	idle := true
	for {
		if !s.store.sweepNeeded() {
			idle = true
			select {
			case <-s.sweepWake:
				wait = sweepBackoffBase
				continue
			case <-s.stop:
				return
			}
		}
		if idle {
			seq, idle = s.store.mailSequence(), false
		}
		timer := s.clock.After(wait)
	waiting:
		for {
			select {
			case <-timer:
				break waiting
			case <-s.sweepWake:
				if now := s.store.mailSequence(); now != seq {
					seq, wait = now, sweepBackoffBase
					timer = nil
					break waiting
				}
			case <-s.stop:
				return
			}
		}
		if timer == nil {
			continue
		}
		switch ok, changed := s.sweepOnce(state); {
		case !ok, changed:
			wait = sweepBackoffBase
		default:
			wait = min(wait*2, sweepBackoffCeiling)
		}
	}
}

// sweepOnce lists the strands and moves what the gone holders hold.
// It reports whether the listing succeeded, and whether the strand-name set differs from the previous successful sweep's.
// A failed listing moves nothing and counts toward no streak.
func (s *Scheduler) sweepOnce(state *sweeper) (ok, changed bool) {
	strands, err := s.delivery.Strands()
	if err != nil {
		if state.lastError != err.Error() {
			state.lastError = err.Error()
			logger.Warn("creel sweep: could not list the hub's strands", "error", err)
		}
		return false, false
	}
	state.lastError = ""
	names := make([]string, len(strands))
	present := make(map[string]bool, len(strands))
	for i, strand := range strands {
		names[i] = strand.Name
		present[strand.Name] = true
	}
	sort.Strings(names)
	key := strings.Join(names, "\n")
	changed = state.haveNames && state.names != key
	state.names, state.haveNames = key, true

	holders := s.store.sweepSubjects()
	for _, holder := range holders {
		if present[holder] {
			state.absent[holder] = 0
			continue
		}
		state.absent[holder]++
		if state.absent[holder] < goneAfterSweeps {
			continue
		}
		delete(state.absent, holder)
		if err := s.store.releaseGoneHolder(holder); err != nil {
			logger.Warn("creel sweep: could not move what a removed strand held", "holder", holder, "error", err)
		}
	}
	for holder := range state.absent {
		if !contains(holders, holder) {
			delete(state.absent, holder)
		}
	}
	return true, changed
}

func contains(names []string, name string) bool {
	for _, other := range names {
		if other == name {
			return true
		}
	}
	return false
}

// isStrandName reports whether full is a strand's name, as opposed to a program's or EXTERNAL.
func isStrandName(full string) bool {
	_, err := agentname.Parse(full)
	return err == nil
}

// sweepNeeded reports whether a strand holds unread mail or has an inject queued.
func (s *Store) sweepNeeded() bool {
	return len(s.sweepSubjects()) > 0
}

// sweepSubjects returns the strands that hold unread mail or have an inject queued, in name order.
func (s *Store) sweepSubjects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, msg := range s.state.messages {
		if msg.Status == StatusUnread && isStrandName(msg.Holder) {
			seen[msg.Holder] = true
		}
	}
	for _, entry := range s.state.injects {
		if entry.Outcome == injectQueued {
			seen[entry.Receiver] = true
		}
	}
	holders := make([]string, 0, len(seen))
	for holder := range seen {
		holders = append(holders, holder)
	}
	sort.Strings(holders)
	return holders
}

// releaseGoneHolder moves what a removed strand held.
// Each unread message goes one step back along its path, the last forwarder or else the sender, marked with the receiver that ended unread.
// When that step is no strand, the message is journaled undelivered and a wait on it is answered that it returned undelivered.
// Each inject queued for the strand expires.
func (s *Store) releaseGoneHolder(holder string) error {
	return s.do(func() error {
		var ids []int64
		for id, msg := range s.state.messages {
			if msg.Holder == holder && msg.Status == StatusUnread {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			msg := s.state.messages[id]
			next := msg.From
			if n := len(msg.Path); n > 0 {
				next = msg.Path[n-1]
			}
			if !isStrandName(next) {
				if err := s.writer.append(record{Event: eventUndelivered, ID: id}); err != nil {
					return err
				}
				s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeReturned, Message: *s.state.messages[id]})
				continue
			}
			if err := s.writer.append(record{Event: eventReturned, ID: id, To: next, Receiver: holder}); err != nil {
				return err
			}
			if err := s.deliverMailLocked(id, next); err != nil {
				return err
			}
		}
		for _, entry := range s.queuedEntriesLocked(holder) {
			if err := s.settleInjectLocked(entry.ID, eventExpired); err != nil {
				return err
			}
		}
		return nil
	})
}
