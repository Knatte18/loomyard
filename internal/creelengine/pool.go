// pool.go holds the Store: the folded pool, its journal writer and clock behind one mutex, with the waiter registry and the wake hook.

package creelengine

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
)

const (
	// maxBodyBytes caps a message body; a longer message is a file the sender writes and the mail names.
	maxBodyBytes = 64 << 10
	// defaultInjectTTL is how long an inject lives when its sender names no TTL.
	defaultInjectTTL = 10 * time.Minute
	// maxInjectTTL is the longest TTL an inject may carry.
	maxInjectTTL = time.Hour
	// maxHoldDuration is the longest a hold may last.
	maxHoldDuration = time.Hour
	// noticesName is the stage that lifts the withholding of an orch strand's reminders.
	noticesName = "creel-notices"
)

// Store is the pool of one hub: the state folded from the journal, the journal writer and the clock behind one mutex.
// Every mutating verb appends and syncs its journal line before it changes the pool and before it answers.
// A Store is safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	geom     Geometry
	delivery Delivery
	clock    Clock
	state    *pool
	writer   *journalWriter
	// reserved maps a message id to the holder whose read is handing it out and has not yet been acked.
	reserved map[int64]string
	waiters  map[waitKey][]*Waiter
	wakeHook func(receiver string)
	wakes    []string
	// mailSeq counts the posts, replies, forwards, returns and injects that put something new in the pool.
	mailSeq uint64
	// agedWritten maps an inject id to the consumed seconds its last aged event recorded.
	agedWritten map[int64]int64
}

// OpenStore folds the journal under geom.JournalDir into a pool and opens the journal for appending.
func OpenStore(geom Geometry, delivery Delivery, clock Clock) (*Store, error) {
	state, err := foldJournal(geom.JournalDir)
	if err != nil {
		return nil, err
	}
	writer, err := openJournal(geom.JournalDir, clock, state)
	if err != nil {
		return nil, err
	}
	return &Store{
		geom: geom, delivery: delivery, clock: clock, state: state, writer: writer,
		reserved: map[int64]string{}, waiters: map[waitKey][]*Waiter{}, agedWritten: map[int64]int64{},
	}, nil
}

// Close closes the journal's open day file.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writer.close()
}

// RollDay opens the journal's file for the clock's UTC day when that day has changed, which also carries live state out of the files past retention.
func (s *Store) RollDay() error {
	return s.do(func() error {
		now := s.clock.Now().UTC()
		if day := journalDay(now); day != s.writer.day {
			return s.writer.rotate(now, day)
		}
		return nil
	})
}

// SetWakeHook sets the function called with a receiver name whenever mail, an inject or a release makes that receiver worth a delivery attempt.
// The hook runs after the verb has released the store's mutex, so it may call back into the store.
func (s *Store) SetWakeHook(hook func(receiver string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wakeHook = hook
}

// do runs fn under the mutex, then calls the wake hook for every receiver fn signalled once the mutex is free.
func (s *Store) do(fn func() error) error {
	s.mu.Lock()
	err := fn()
	wakes, hook := s.wakes, s.wakeHook
	s.wakes = nil
	s.mu.Unlock()
	if hook != nil {
		seen := map[string]bool{}
		for _, receiver := range wakes {
			if !seen[receiver] {
				seen[receiver] = true
				hook(receiver)
			}
		}
	}
	return err
}

// wakeLocked signals that receiver has something to deliver.
func (s *Store) wakeLocked(receiver string) {
	s.wakes = append(s.wakes, receiver)
}

// readStrands lists the hub's strands, wrapping a failure so a verb reports what could not be read instead of an unknown name.
func (s *Store) readStrands() ([]Strand, error) {
	strands, err := s.delivery.Strands()
	if err != nil {
		return nil, fmt.Errorf("creel: could not read the hub's strands: %w", err)
	}
	return strands, nil
}

// findStrand returns the strand with the given name.
func findStrand(strands []Strand, name string) (Strand, bool) {
	for _, strand := range strands {
		if strand.Name == name {
			return strand, true
		}
	}
	return Strand{}, false
}

// isExternalName reports whether full is a hub's EXTERNAL sender name.
func isExternalName(full string) bool {
	_, name, err := agentname.ParseProgram(full)
	return err == nil && name == agentname.External
}

// checkReceiverLocked refuses a name that is no receiver: EXTERNAL always, and a name that is neither a strand nor a program that has posted or read.
func (s *Store) checkReceiverLocked(strands []Strand, name string) error {
	if isExternalName(name) {
		return fmt.Errorf("creel: %s is never a receiver: it has no address; an external sender waits for its answer with `lyx creel post --wait-reply`, and a reply reaches it with `--reply`", name)
	}
	if _, ok := findStrand(strands, name); ok || s.state.programs[name] {
		return nil
	}
	known := make([]string, 0, len(strands)+len(s.state.programs))
	for _, strand := range strands {
		known = append(known, strand.Name)
	}
	for program := range s.state.programs {
		known = append(known, program)
	}
	sort.Strings(known)
	listing := "none"
	if len(known) > 0 {
		listing = strings.Join(known, ", ")
	}
	return fmt.Errorf("creel: unknown receiver %q; the names that exist: %s", name, listing)
}

// liveHoldLocked returns the hold on receiver that has not expired, or nil.
func (s *Store) liveHoldLocked(receiver string) *holdEntry {
	hold := s.state.holds[receiver]
	if hold == nil || !hold.Expires.After(s.clock.Now()) {
		return nil
	}
	return hold
}

// appendWithheldLocked journals that the reminder of message id to an orch receiver waits for creel-notices; any other receiver gets nothing.
func (s *Store) appendWithheldLocked(id int64, receiver string) error {
	if !isOrchName(receiver) {
		return nil
	}
	return s.writer.append(record{Event: eventWithheld, ID: id, To: receiver, Notice: noticesName})
}

// OutcomeKind names how a waited-on thing settled.
type OutcomeKind string

// The outcomes a Waiter receives.
const (
	// OutcomeReply is a reply handed over to an EXTERNAL sender's waiting post.
	OutcomeReply OutcomeKind = "reply"
	// OutcomeReturned is a message that returned undelivered.
	OutcomeReturned OutcomeKind = "returned"
	// OutcomeInjected, OutcomeUnconfirmed and OutcomeExpired are the three ends of an inject.
	OutcomeInjected    OutcomeKind = "injected"
	OutcomeUnconfirmed OutcomeKind = "unconfirmed"
	OutcomeExpired     OutcomeKind = "expired"
	// OutcomeMail means the waited-on holder now holds unread mail.
	OutcomeMail OutcomeKind = "mail"
)

// Outcome is the answer to a Waiter; Message is the reply or the returned message, and zero for the others.
type Outcome struct {
	Kind    OutcomeKind
	Message Message
}

type waitKind int

const (
	waitReply waitKind = iota
	waitInject
	waitHolder
)

// waitKey names what a waiter waits on: the reply to a message id, an inject id, or mail for a holder.
type waitKey struct {
	kind   waitKind
	id     int64
	holder string
}

// Waiter is one registered wait; its channel receives the outcome once, and the wait ends there.
type Waiter struct {
	// C receives the outcome.
	C     <-chan Outcome
	ch    chan Outcome
	store *Store
	key   waitKey
}

// Cancel unregisters the wait; a waiter already answered is unaffected.
func (w *Waiter) Cancel() {
	w.store.mu.Lock()
	defer w.store.mu.Unlock()
	list := w.store.waiters[w.key]
	for i, other := range list {
		if other == w {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(w.store.waiters, w.key)
	} else {
		w.store.waiters[w.key] = list
	}
}

// addWaiterLocked registers a wait on key, and answers it at once when its outcome already stands in the pool.
func (s *Store) addWaiterLocked(key waitKey) (*Waiter, error) {
	ch := make(chan Outcome, 1)
	waiter := &Waiter{C: ch, ch: ch, store: s, key: key}
	outcome, settled, err := s.settledOutcomeLocked(key)
	if err != nil {
		return nil, err
	}
	if settled {
		ch <- outcome
		return waiter, nil
	}
	s.waiters[key] = append(s.waiters[key], waiter)
	return waiter, nil
}

// answerLocked answers every waiter on key with outcome and reports how many there were.
func (s *Store) answerLocked(key waitKey, outcome Outcome) int {
	list := s.waiters[key]
	delete(s.waiters, key)
	for _, waiter := range list {
		waiter.ch <- outcome
	}
	return len(list)
}

// settledOutcomeLocked reports an outcome that already stands in the pool for key.
// A reply waiting undelivered for an EXTERNAL sender is handed over here and journaled delivered.
func (s *Store) settledOutcomeLocked(key waitKey) (Outcome, bool, error) {
	switch key.kind {
	case waitReply:
		var replies []*Message
		for _, msg := range s.state.messages {
			if msg.Kind == kindReply && msg.ReplyTo == key.id && msg.Status == StatusUndelivered && isExternalName(msg.Holder) {
				replies = append(replies, msg)
			}
		}
		if len(replies) > 0 {
			sort.Slice(replies, func(i, j int) bool { return replies[i].ID < replies[j].ID })
			reply := replies[0]
			if err := s.writer.append(record{Event: eventDelivered, ID: reply.ID}); err != nil {
				return Outcome{}, false, err
			}
			return Outcome{Kind: OutcomeReply, Message: *reply}, true, nil
		}
		if msg := s.state.messages[key.id]; msg != nil && msg.Status == StatusUndelivered {
			return Outcome{Kind: OutcomeReturned, Message: *msg}, true, nil
		}
	case waitInject:
		if entry := s.state.injects[key.id]; entry != nil {
			switch entry.Outcome {
			case injectInjected:
				return Outcome{Kind: OutcomeInjected}, true, nil
			case injectUnconfirmed:
				return Outcome{Kind: OutcomeUnconfirmed}, true, nil
			case injectExpired:
				return Outcome{Kind: OutcomeExpired}, true, nil
			}
		}
	case waitHolder:
		if len(s.unreadLocked(key.holder, "")) > 0 {
			return Outcome{Kind: OutcomeMail}, true, nil
		}
	}
	return Outcome{}, false, nil
}

// unreadLocked returns the unread messages holder holds and no read is handing out, oldest first, narrowed to one sender when from is set.
func (s *Store) unreadLocked(holder, from string) []*Message {
	var unread []*Message
	for id, msg := range s.state.messages {
		if msg.Holder == holder && msg.Status == StatusUnread && s.reserved[id] == "" && (from == "" || msg.From == from) {
			unread = append(unread, msg)
		}
	}
	sort.Slice(unread, func(i, j int) bool { return unread[i].ID < unread[j].ID })
	return unread
}
