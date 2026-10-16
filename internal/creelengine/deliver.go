// deliver.go runs the Scheduler, one worker per receiver that types queued injects and reminders into its pane through the delivery seam, and the Store side those workers act on.

package creelengine

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// probeBackoffBase is the wait between a worker's probes at first and after a probe that delivered.
	probeBackoffBase = time.Second
	// probeBackoffCeiling is the longest a worker waits between probes while its probes deliver nothing.
	probeBackoffCeiling = 8 * time.Second
	// agedPersistInterval is how often the TTL an inject has consumed is journaled while it runs.
	agedPersistInterval = time.Minute
)

// Scheduler delivers what the pool holds into panes: one worker per receiver with pending deliveries, at most one delivery in flight per receiver.
// Nothing is ever typed into an orch strand, and a program's mailbox is never reminded.
type Scheduler struct {
	store    *Store
	delivery Delivery
	clock    Clock

	mu       sync.Mutex
	workers  map[string]*worker
	stopping bool
	stop     chan struct{}
	wg       sync.WaitGroup
	// sweepWake tells the removed-holder sweep that the pool changed.
	sweepWake chan struct{}
}

// NewScheduler returns a Scheduler delivering the store's pool through delivery.
func NewScheduler(store *Store, delivery Delivery, clock Clock) *Scheduler {
	return &Scheduler{
		store: store, delivery: delivery, clock: clock,
		workers: map[string]*worker{}, stop: make(chan struct{}), sweepWake: make(chan struct{}, 1),
	}
}

// Start subscribes to the store's wake notifications, starts the removed-holder sweep and launches a worker for every receiver the pool already shows with a queued inject or unread mail.
func (s *Scheduler) Start() {
	s.store.SetWakeHook(s.wake)
	s.wg.Add(1)
	go s.sweepLoop()
	for _, receiver := range s.store.pendingReceivers() {
		s.wake(receiver)
	}
}

// Stop starts no new send, waits for the sends in flight to return and their outcomes to be journaled, and persists the TTL each queued inject has consumed.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	close(s.stop)
	s.mu.Unlock()
	s.wg.Wait()
}

// wake signals that receiver has something to deliver: it nudges its worker, or starts one when the receiver has pending deliveries.
func (s *Scheduler) wake(receiver string) {
	select {
	case s.sweepWake <- struct{}{}:
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return
	}
	if running := s.workers[receiver]; running != nil {
		select {
		case running.wake <- struct{}{}:
		default:
		}
		return
	}
	if !s.store.hasPending(receiver) {
		return
	}
	started := &worker{scheduler: s, receiver: receiver, wake: make(chan struct{}, 1), lastError: map[string]string{}}
	s.workers[receiver] = started
	s.wg.Add(1)
	go started.run()
}

// worker delivers to one receiver.
type worker struct {
	scheduler *Scheduler
	receiver  string
	wake      chan struct{}

	// known says whether a probe has seen the receiver yet; running is whether the receiver was live and unpaused at the last probe.
	known     bool
	running   bool
	lastProbe time.Time
	lastAged  time.Time
	// lastError holds the last error reported per kind, so a repeated error logs once.
	lastError map[string]string
}

// run probes the receiver from a 1-second base and doubles the wait to 8 seconds after every probe that delivers nothing.
// A wake notification or a probe that delivers returns the wait to the base, and the worker ends when nothing is pending.
func (w *worker) run() {
	defer w.scheduler.wg.Done()
	var lastWait time.Duration
	for {
		select {
		case <-w.scheduler.stop:
			w.finish()
			return
		default:
		}
		delivered, pending := w.probe()
		if !pending {
			if w.retire() {
				return
			}
			continue
		}
		next := probeBackoffBase
		if !delivered && lastWait > 0 {
			next = min(lastWait*2, probeBackoffCeiling)
		}
		lastWait = next
		select {
		case <-w.wake:
			lastWait = 0
		case <-w.scheduler.clock.After(next):
		case <-w.scheduler.stop:
			w.finish()
			return
		}
	}
}

// retire removes the worker once nothing is pending; it reports false when something arrived meanwhile and the worker carries on.
func (w *worker) retire() bool {
	w.scheduler.mu.Lock()
	defer w.scheduler.mu.Unlock()
	if w.scheduler.store.hasPending(w.receiver) && !w.scheduler.stopping {
		return false
	}
	delete(w.scheduler.workers, w.receiver)
	return true
}

// finish counts the TTL consumed since the last probe and persists it.
func (w *worker) finish() {
	store := w.scheduler.store
	if w.known && w.running {
		w.report("journal", store.ageInjects(w.receiver, w.scheduler.clock.Now().Sub(w.lastProbe)))
	}
	w.report("journal", store.persistAged(w.receiver))
}

// report logs err once per change of error for kind, and clears the kind when err is nil.
func (w *worker) report(kind string, err error) {
	if err == nil {
		delete(w.lastError, kind)
		return
	}
	if w.lastError[kind] == err.Error() {
		return
	}
	w.lastError[kind] = err.Error()
	logger.Warn("creel delivery: "+kind+" failed", "receiver", w.receiver, "error", err)
}

// probe makes one delivery attempt for the receiver.
// It reports whether it typed something, and whether the receiver still has deliveries pending.
func (w *worker) probe() (delivered, pending bool) {
	store, clock := w.scheduler.store, w.scheduler.clock
	if !store.hasPending(w.receiver) {
		return false, false
	}
	now := clock.Now()
	if w.known && w.running {
		w.report("journal", store.ageInjects(w.receiver, now.Sub(w.lastProbe)))
	}
	w.lastProbe = now
	strand, running := w.locate()
	switch {
	case !w.known:
		w.lastAged = now
	case running != w.running, running && now.Sub(w.lastAged) >= agedPersistInterval:
		w.report("journal", store.persistAged(w.receiver))
		w.lastAged = now
	}
	w.known, w.running = true, running
	if !store.hasPending(w.receiver) {
		return false, false
	}
	if !running {
		return false, true
	}
	snap := store.snapshot(w.receiver)
	for _, inject := range snap.injects {
		if snap.holdOwner == "" || inject.From == snap.holdOwner {
			return w.typeInject(strand, inject), true
		}
	}
	return w.remind(strand, snap, now), true
}

// locate finds the receiver's strand and reports whether its TTL runs: it is live and no pause covers it.
// An error reading the strands or the pause types nothing for this probe and freezes the TTL.
func (w *worker) locate() (Strand, bool) {
	strands, err := w.scheduler.delivery.Strands()
	w.report("strands", err)
	if err != nil {
		return Strand{}, false
	}
	strand, found := findStrand(strands, w.receiver)
	if !found || !strand.Live {
		return strand, false
	}
	paused, err := w.scheduler.delivery.Paused(strand)
	w.report("pause", err)
	return strand, err == nil && !paused
}

// idle asks whether the strand may be typed into; an error counts as not idle.
// Every reading goes to the stall journal, an error as a reading with no reason.
func (w *worker) idle(strand Strand) bool {
	answer, err := w.scheduler.delivery.Idle(strand)
	w.report("idle", err)
	stall := answer.Stall
	if err != nil {
		stall = ""
	}
	w.report("journal", w.scheduler.store.noteStall(w.receiver, stall))
	return err == nil && answer.Idle
}

// typeInject types the inject once the receiver is idle, journaling a sending event first and the outcome after.
func (w *worker) typeInject(strand Strand, inject QueuedInject) bool {
	store := w.scheduler.store
	if !w.idle(strand) {
		return false
	}
	if err := store.markSending(inject.ID); err != nil {
		w.report("journal", err)
		return false
	}
	err := w.scheduler.delivery.Send(strand, inject.Text)
	switch {
	case err == nil:
		w.report("send", nil)
		w.report("journal", store.settleInject(inject.ID, eventInjected))
		w.report("journal", store.noteStall(w.receiver, ""))
		return true
	case errors.Is(err, ErrNotLanded):
		w.report("send", nil)
		w.report("journal", store.settleInject(inject.ID, eventUnconfirmed))
		w.report("journal", store.noteStall(w.receiver, ""))
		return true
	default:
		if !errors.Is(err, ErrBusy) {
			w.report("send", err)
		}
		store.requeue(inject.ID)
		return false
	}
}

// remind types the reminder line when the receiver has unread mail, the backoff allows it and the receiver is idle.
func (w *worker) remind(strand Strand, snap deliverySnapshot, now time.Time) bool {
	if len(snap.unread) == 0 {
		return false
	}
	ids := make([]int64, len(snap.unread))
	for i, msg := range snap.unread {
		ids[i] = msg.ID
	}
	due, step := reminderDue(snap.reminder, snap.hasReminder, ids, now)
	if !due || !w.idle(strand) {
		return false
	}
	err := w.scheduler.delivery.Send(strand, composeReminder(snap.unread))
	switch {
	case err == nil, errors.Is(err, ErrNotLanded):
		w.report("send", nil)
		w.report("journal", w.scheduler.store.journalReminded(w.receiver, ids, step))
		w.report("journal", w.scheduler.store.noteStall(w.receiver, ""))
		return true
	default:
		if !errors.Is(err, ErrBusy) {
			w.report("send", err)
		}
		return false
	}
}

// deliverableReceiver reports whether creel may type into the named receiver: a strand that is not an orch.
func deliverableReceiver(name string) bool {
	parsed, err := agentname.Parse(name)
	return err == nil && !agentname.MatchesRole(parsed.Role, agentname.RoleOrch)
}

// hasPending reports whether receiver has a queued inject or unread mail to deliver.
// A hold's muting is not considered, so a worker outlives a hold and delivers when it ends.
func (s *Store) hasPending(receiver string) bool {
	if !deliverableReceiver(receiver) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.state.injects {
		if entry.Receiver == receiver && entry.Outcome == injectQueued {
			return true
		}
	}
	for _, msg := range s.state.messages {
		if msg.Holder == receiver && msg.Status == StatusUnread {
			return true
		}
	}
	return false
}

// pendingReceivers returns the receivers the pool shows with a queued inject or unread mail, in name order.
func (s *Store) pendingReceivers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, entry := range s.state.injects {
		if entry.Outcome == injectQueued {
			seen[entry.Receiver] = true
		}
	}
	for _, msg := range s.state.messages {
		if msg.Status == StatusUnread {
			seen[msg.Holder] = true
		}
	}
	var receivers []string
	for receiver := range seen {
		if deliverableReceiver(receiver) {
			receivers = append(receivers, receiver)
		}
	}
	sort.Strings(receivers)
	return receivers
}

// mailSequence returns a counter that grows with every post, reply, forward, return and inject.
func (s *Store) mailSequence() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mailSeq
}

// deliverySnapshot is what a worker needs to decide one probe.
type deliverySnapshot struct {
	// injects are the queued injects in the order they are typed.
	injects []QueuedInject
	// holdOwner is the owner of a live hold on the receiver, empty when there is none.
	holdOwner string
	// unread are the unread messages the receiver may be reminded about: all of them, or under a hold only the hold owner's.
	unread      []Message
	reminder    reminderEntry
	hasReminder bool
}

func (s *Store) snapshot(receiver string) deliverySnapshot {
	snap := deliverySnapshot{injects: s.Queued(receiver)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if hold := s.liveHoldLocked(receiver); hold != nil {
		snap.holdOwner = hold.Owner
	}
	for _, msg := range s.state.messages {
		if msg.Holder == receiver && msg.Status == StatusUnread && (snap.holdOwner == "" || msg.From == snap.holdOwner) {
			snap.unread = append(snap.unread, *msg)
		}
	}
	sort.Slice(snap.unread, func(i, j int) bool { return snap.unread[i].ID < snap.unread[j].ID })
	snap.reminder, snap.hasReminder = s.state.reminders[receiver]
	return snap
}

// ageInjects counts elapsed against the TTL of every inject queued for receiver, and expires those that used it up.
func (s *Store) ageInjects(receiver string, elapsed time.Duration) error {
	if elapsed <= 0 {
		return nil
	}
	return s.do(func() error {
		for _, entry := range s.queuedEntriesLocked(receiver) {
			entry.Consumed += elapsed
			if entry.Consumed >= entry.TTL {
				if err := s.settleInjectLocked(entry.ID, eventExpired); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// queuedEntriesLocked returns the injects still queued for receiver, oldest first.
func (s *Store) queuedEntriesLocked(receiver string) []*injectEntry {
	var entries []*injectEntry
	for _, entry := range s.state.injects {
		if entry.Receiver == receiver && entry.Outcome == injectQueued {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

// persistAged journals the TTL each inject queued for receiver has consumed since its last aged event.
func (s *Store) persistAged(receiver string) error {
	return s.do(func() error {
		for _, entry := range s.queuedEntriesLocked(receiver) {
			seconds := int64(entry.Consumed / time.Second)
			if seconds == s.agedWritten[entry.ID] {
				continue
			}
			exact := entry.Consumed
			if err := s.writer.append(record{Event: eventAged, ID: entry.ID, ConsumedSeconds: seconds}); err != nil {
				return err
			}
			entry.Consumed = exact
			s.agedWritten[entry.ID] = seconds
		}
		return nil
	})
}

// markSending journals that the inject is about to be typed, before the send is attempted.
func (s *Store) markSending(id int64) error {
	return s.do(func() error {
		return s.writer.append(record{Event: eventSending, ID: id})
	})
}

// requeue puts an inject whose send did not happen back in its receiver's queue.
// Only the in-memory state returns: a crash before the retry folds it as unconfirmed, so it is never typed twice.
func (s *Store) requeue(id int64) {
	_ = s.do(func() error {
		if entry := s.state.injects[id]; entry != nil && entry.Outcome == injectSending {
			entry.Outcome = injectQueued
		}
		return nil
	})
}

// settleInject journals the end of an inject (injected, unconfirmed or expired) and answers a wait on it.
func (s *Store) settleInject(id int64, event string) error {
	return s.do(func() error { return s.settleInjectLocked(id, event) })
}

func (s *Store) settleInjectLocked(id int64, event string) error {
	if err := s.writer.append(record{Event: event, ID: id}); err != nil {
		return err
	}
	kinds := map[string]OutcomeKind{eventInjected: OutcomeInjected, eventUnconfirmed: OutcomeUnconfirmed, eventExpired: OutcomeExpired}
	s.answerLocked(waitKey{kind: waitInject, id: id}, Outcome{Kind: kinds[event]})
	return nil
}

// noteStall records the stall a probe read for receiver and journals a stalled record when it is non-empty and differs from the last one.
// An empty stall clears the receiver's entry with no record.
func (s *Store) noteStall(receiver string, stall Stall) error {
	return s.do(func() error {
		if stall == "" {
			delete(s.stalls, receiver)
			return nil
		}
		if s.stalls[receiver].stall == stall {
			return nil
		}
		s.stalls[receiver] = stallNote{stall: stall, at: s.clock.Now()}
		return s.writer.append(record{Event: eventStalled, To: receiver, Reason: string(stall)})
	})
}

// journalReminded records a reminder typed to receiver about the unread ids, at the given backoff step.
func (s *Store) journalReminded(receiver string, ids []int64, step int) error {
	return s.do(func() error {
		return s.writer.append(record{Event: eventReminded, To: receiver, Step: step, Unread: ids})
	})
}
