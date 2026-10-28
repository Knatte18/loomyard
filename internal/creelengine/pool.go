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
	// maxBodyBytes caps the body of a reply to EXTERNAL, which only a waiting post --wait-reply receives and no claim hands out; every other message is capped by the line it renders (LineFits).
	maxBodyBytes = 64 << 10
	// defaultInjectTTL is how long an inject lives when its sender names no TTL.
	defaultInjectTTL = 10 * time.Minute
	// maxInjectTTL is the longest TTL an inject may carry.
	maxInjectTTL = time.Hour
	// maxHoldDuration is the longest a hold may last.
	maxHoldDuration = time.Hour
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
	// stalls maps a receiver to its standing stall, the clock time the stall began and the clock time a probe last read it.
	stalls map[string]stallNote
	// paneLocks maps a receiver to the mutex that serialises an interrupt of its pane with its deliveries.
	paneLocks map[string]*sync.Mutex
	// flags holds the file names of the flags the flag directory holds, as this store last set or found them.
	flags map[string]bool
	// stopBlocks maps a strand to the Stop blocks of its current turn; a strand with no entry counts as at the cap, and the count is never journaled.
	stopBlocks map[string]int
	// wakeMarks maps a hook strand to the claimable set its last ring was for, the clock time of that ring, whether it was answered and the set its bell was typed for; it is kept in memory alone.
	wakeMarks map[string]wakeMark
	// opened is the clock time the store opened, from which a strand whose hook waiter never registered counts its bell grace.
	opened time.Time
	// waiterEnds maps a hook strand to the clock time its last hook waiter ended.
	waiterEnds map[string]time.Time
	// chaseWritten maps a message id to the whole seconds of its chase clock its last chase-aged record recorded.
	chaseWritten map[int64]int64
	// chaseBase maps a message id whose chase step was already due when the store opened to the clock at that opening, the base its next step counts from.
	chaseBase map[int64]time.Duration
}

// paneLock returns the one mutex of receiver's pane.
// An interrupt holds it while the interrupt runs, and a worker holds it from before it asks whether the pane is idle until its send or reminder ends.
func (s *Store) paneLock(receiver string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.paneLocks[receiver]
	if lock == nil {
		lock = &sync.Mutex{}
		s.paneLocks[receiver] = lock
	}
	return lock
}

// stallNote is the stall standing for a receiver.
// at is when the stall began, the time every note and answer prints, and readAt is when a probe last read it.
type stallNote struct {
	stall  Stall
	at     time.Time
	readAt time.Time
}

// stallFreshness is how old a stall's last reading may be and still reach a waiter or an expired answer.
// A probe that reads runs at least every probeBackoffCeiling, so only a stall no probe has read for twice that is withheld.
const stallFreshness = 2 * probeBackoffCeiling

// freshStallLocked returns receiver's standing stall when a probe read it no longer ago than stallFreshness.
// A stale stall is dropped from the standing map, so its next reading journals a new stalled record, and nothing is returned.
func (s *Store) freshStallLocked(receiver string) (stallNote, bool) {
	note, ok := s.stalls[receiver]
	if !ok {
		return stallNote{}, false
	}
	if s.clock.Now().Sub(note.readAt) > stallFreshness {
		delete(s.stalls, receiver)
		return stallNote{}, false
	}
	return note, true
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
	store := &Store{
		geom: geom, delivery: delivery, clock: clock, state: state, writer: writer,
		reserved: map[int64]string{}, waiters: map[waitKey][]*Waiter{}, agedWritten: map[int64]int64{},
		stalls: map[string]stallNote{}, paneLocks: map[string]*sync.Mutex{}, flags: map[string]bool{}, stopBlocks: map[string]int{}, wakeMarks: map[string]wakeMark{},
		opened: clock.Now(), waiterEnds: map[string]time.Time{},
		chaseWritten: map[int64]int64{}, chaseBase: map[int64]time.Duration{},
	}
	store.seedChaseClock()
	if err := store.repairChases(); err != nil {
		_ = writer.close()
		return nil, err
	}
	return store, nil
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
// When fn journaled a record, the strands' mail flags follow the pool before the mutex is freed, whether or not fn failed after it.
func (s *Store) do(fn func() error) error {
	s.mu.Lock()
	appended := s.writer.appended
	err := fn()
	if s.writer.appended != appended {
		s.syncFlagsLocked()
	}
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
		return fmt.Errorf("creel: %s is never a receiver: it has no address; an external sender waits for its answer with `lym creel post --wait-reply`, and a reply reaches it with `--reply`", name)
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
	// OutcomeUnanswered is a marked message its holder read and never answered, after its chases.
	OutcomeUnanswered OutcomeKind = "unanswered"
	// OutcomeMail means the waited-on holder now holds unread mail.
	OutcomeMail OutcomeKind = "mail"
	// OutcomeRing means the daemon woke the idle strand whose waiter this was; it carries no mail and claims nothing.
	OutcomeRing OutcomeKind = "ring"
	// OutcomeSuperseded means a newer waiter of the same strand replaced this one.
	OutcomeSuperseded OutcomeKind = "superseded"
)

// Outcome is the answer to a Waiter; Message is the reply or the returned message, and zero for the others.
// Stall and StallAt are the receiver's fresh stall when an inject expired and the time it began, empty and zero when none stood.
type Outcome struct {
	Kind    OutcomeKind
	Message Message
	Stall   Stall
	StallAt time.Time
}

// StallNote is a stall a probe read for a receiver and the time the stall began.
type StallNote struct {
	Stall Stall
	At    time.Time
}

type waitKind int

const (
	waitReply waitKind = iota
	waitInject
	waitHolder
	waitHook waitKind = 3
)

// waitKey names what a waiter waits on: the reply to a message id, an inject id, mail for a holder, or the ring of a strand's idle waiter.
type waitKey struct {
	kind   waitKind
	id     int64
	holder string
}

// Waiter is one registered wait; its channel receives the outcome once, and the wait ends there.
type Waiter struct {
	// C receives the outcome.
	C <-chan Outcome
	// Progress receives the stall of the inject's receiver each time it changes; only a wait on an inject has one, and a newer note replaces an unread older one.
	Progress <-chan StallNote
	ch       chan Outcome
	progress chan StallNote
	store    *Store
	key      waitKey
	// told is the stall the waiter was last told, empty when none; a reading that differs from it is notified.
	told Stall
}

// notify hands note to the waiter's progress channel, replacing a note nobody has read.
// It runs under the store's mutex, the only place that sends, so the slot is free after a drain.
func (w *Waiter) notify(note StallNote) {
	for {
		select {
		case w.progress <- note:
			return
		default:
		}
		select {
		case <-w.progress:
		default:
		}
	}
}

// Cancel unregisters the wait; a waiter already answered is unaffected.
// A hook waiter's end, answered or not, is recorded at the clock time, from which its strand's bell grace runs.
func (w *Waiter) Cancel() {
	w.store.mu.Lock()
	defer w.store.mu.Unlock()
	if w.key.kind == waitHook {
		w.store.waiterEnds[w.key.holder] = w.store.clock.Now()
	}
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
// A wait on an inject also gets the receiver's fresh stall as its first progress note.
func (s *Store) addWaiterLocked(key waitKey) (*Waiter, error) {
	ch := make(chan Outcome, 1)
	waiter := &Waiter{C: ch, ch: ch, store: s, key: key}
	if key.kind == waitInject {
		waiter.progress = make(chan StallNote, 1)
		waiter.Progress = waiter.progress
	}
	outcome, settled, err := s.settledOutcomeLocked(key)
	if err != nil {
		return nil, err
	}
	if settled {
		ch <- outcome
		return waiter, nil
	}
	if key.kind == waitInject {
		if entry := s.state.injects[key.id]; entry != nil {
			if note, ok := s.freshStallLocked(entry.Receiver); ok {
				waiter.notify(StallNote{Stall: note.stall, At: note.at})
				waiter.told = note.stall
			}
		}
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
// A reply waiting undelivered for an EXTERNAL sender is handed over here and journaled delivered; a reply to any other sender is handed over without a record.
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
		if original := s.state.messages[key.id]; original != nil && !isExternalName(original.From) {
			// A reply to a strand's or program's message stays mail to it, so no delivered record is journaled.
			var oldest *Message
			for _, msg := range s.state.messages {
				if msg.Kind == kindReply && msg.ReplyTo == key.id && (oldest == nil || msg.ID < oldest.ID) {
					oldest = msg
				}
			}
			if oldest != nil {
				return Outcome{Kind: OutcomeReply, Message: *oldest}, true, nil
			}
		}
		if msg := s.state.messages[key.id]; msg != nil && msg.Unanswered != "" {
			return Outcome{Kind: OutcomeUnanswered, Message: *msg}, true, nil
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

// wakeMark is the record of a hook strand's last ring and bell: the claimable message ids the ring was for, the clock time of the ring and whether the ring was answered.
// A ring is answered by a claim that handed out mail, or by a probe reading a turn in progress after it.
// The time and the answer stay until the next ring replaces them, whatever the set does meanwhile.
// bellIDs are the claimable ids the typed bell was for, empty when none was typed; a different set is a set no bell was typed for.
type wakeMark struct {
	ids      []int64
	at       time.Time
	answered bool
	bellIDs  []int64
}
