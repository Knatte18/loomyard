// outbound.go implements the sending half's delivery of mail across hubs: one worker per receiving hub that delivers the pending outbound messages with retries and returns them after an hour, and the settling of an outbound message.

package creelengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// outboundRetryBase is the wait before the second attempt to deliver a message, and outboundRetryCeiling the longest wait between two attempts.
	outboundRetryBase    = time.Second
	outboundRetryCeiling = time.Minute
	// outboundDeadline is how long an outbound message is tried, counted from its journaled post time.
	outboundDeadline = time.Hour
	// outboundCallTimeout bounds one deliver call to a receiving hub's daemon, on the scheduler's clock.
	outboundCallTimeout = 30 * time.Second
	// maxHubStarts caps how many times the sending daemon starts one hub's daemon before that daemon answers a call.
	maxHubStarts = 3
	// returnReasonMaxBytes bounds the text of another hub's answer that a return notice quotes.
	returnReasonMaxBytes = 200
)

// The reasons creel writes itself for a message that came back, as opposed to text the other hub's daemon wrote.
const (
	codeConnectFailed = "connect-failed"
	codeTimeout       = "timeout"
	codeHubUnreadable = "hub-unreadable"
	codeStartCapped   = "start-capped"
	codeBadAnswer     = "bad-answer"
)

// deliverOutcome is what one attempt to deliver a message made of the receiving hub's answer.
type deliverOutcome struct {
	// delivery is deliveryAccepted or deliveryRefused, and empty for an answer the worker retries.
	delivery string
	// remoteID is the id the receiving hub gave an accepted message.
	remoteID int64
	// reason is why a refused or retried delivery did not go through: a code creel wrote, or the other hub's text, quoted.
	reason string
}

// quoteAnswer renders text another hub's daemon wrote so it reaches a notice only as quoted data.
// Control characters, line breaks included, become spaces, the text is cut on a rune boundary to returnReasonMaxBytes, and it is wrapped in double quotes with its quote marks and backslashes escaped.
func quoteAnswer(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	if len(text) > returnReasonMaxBytes {
		cut := returnReasonMaxBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"`
}

// SetOutboundHook sets the function called with a receiving hub's path whenever an outbound message to that hub is queued.
// The hook runs after the verb has released the store's mutex, so it may call back into the store.
func (s *Store) SetOutboundHook(hook func(hubPath string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outboundHook = hook
}

// wakeOutboundLocked signals that an outbound message to the hub at hubPath is queued.
func (s *Store) wakeOutboundLocked(hubPath string) {
	s.outboundWakes = append(s.outboundWakes, hubPath)
}

// hubDirectory returns the seam to the other hubs, nil for a daemon that serves no other hub.
func (s *Store) hubDirectory() HubDirectory {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hubs
}

// pendingOutbound returns the outbound messages to the hub at hubPath that are not yet settled, oldest first.
func (s *Store) pendingOutbound(hubPath string) []outboundEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pending []outboundEntry
	for _, entry := range s.state.outbound {
		if entry.ReceivingHub == hubPath && entry.State == outboundPending {
			pending = append(pending, *entry)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return pending
}

// outboundHubs returns the paths of the hubs with an unsettled outbound message, in path order.
func (s *Store) outboundHubs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var hubs []string
	for _, entry := range s.state.outbound {
		if entry.State == outboundPending && !seen[entry.ReceivingHub] {
			seen[entry.ReceivingHub] = true
			hubs = append(hubs, entry.ReceivingHub)
		}
	}
	sort.Strings(hubs)
	return hubs
}

// settleDelivered journals that the receiving hub's daemon accepted the outbound message id and gave it remoteID.
// A message that is no longer pending is left alone.
func (s *Store) settleDelivered(id, remoteID int64) error {
	return s.do(func() error {
		if sent := s.state.outbound[id]; sent == nil || sent.State != outboundPending {
			return nil
		}
		return s.writer.append(record{Event: eventRemoteDelivered, ID: id, RemoteID: remoteID})
	})
}

// settleReturned journals that the outbound message id came back undelivered for reason, answers a wait on its reply and tells its sender in a notice.
// The strand listing is read before the lock, and a sender no longer listed gets no notice; a notice that fails to store is logged and undoes nothing.
// A message that is no longer pending is left alone.
func (s *Store) settleReturned(id int64, reason string) error {
	listing := s.listStrands()
	return s.do(func() error {
		sent := s.state.outbound[id]
		if sent == nil || sent.State != outboundPending {
			return nil
		}
		if err := s.writer.append(record{Event: eventOutboundReturned, ID: id, Reason: reason}); err != nil {
			return err
		}
		s.answerLocked(waitKey{kind: waitReply, id: id}, Outcome{Kind: OutcomeReturned, Message: sent.message()})
		if err := s.noticeReturnedLocked(sent, reason, listing); err != nil {
			logger.Warn("creel outbound: could not post the notice of a returned message", "id", id, "sender", sent.From, "error", err)
		}
		return nil
	})
}

// noticeReturnedLocked posts sent's sender a notice naming the receiver and the reason the message came back.
func (s *Store) noticeReturnedLocked(sent *outboundEntry, reason string, listing strandListing) error {
	if listing.err != nil {
		return listing.err
	}
	if _, listed := findStrand(listing.strands, sent.From); !listed {
		return nil
	}
	sender, err := s.chaserSender()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("Message #%d to %s came back undelivered, reason: %s; way forward: lym creel log --id %d shows its path, then post it again or decide without the answer", sent.ID, sent.To, reason, sent.ID)
	id, err := s.appendPostedLocked(sender, record{To: sent.From, Kind: kindMail, Body: body})
	if err != nil {
		return err
	}
	return s.deliverMailLocked(id, sent.From)
}

// wakeOutbound starts the worker of the hub at hubPath when the hub has a pending outbound message and no worker runs.
// A running worker is left alone: it reads the queue afresh before it retires, and a wake never hastens its retry.
// A daemon told no HubDirectory starts none.
func (s *Scheduler) wakeOutbound(hubPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping || s.store.hubDirectory() == nil || s.outbound[hubPath] != nil {
		return
	}
	if len(s.store.pendingOutbound(hubPath)) == 0 {
		return
	}
	started := &outboundWorker{scheduler: s, hubPath: hubPath}
	s.outbound[hubPath] = started
	s.wg.Add(1)
	go started.run()
}

// countHubStart counts one successful start of the daemon of the hub at hubPath; a start that failed to launch is not counted, since it started nothing.
func (s *Scheduler) countHubStart(hubPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hubStarts[hubPath]++
}

// hubStartsCapped reports whether the daemon of the hub at hubPath was started maxHubStarts times without answering a call since.
func (s *Scheduler) hubStartsCapped(hubPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hubStarts[hubPath] >= maxHubStarts
}

// hubAnswered resets the start count and the cap warning of the hub at hubPath, since its daemon answered a call.
func (s *Scheduler) hubAnswered(hubPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.hubStarts, hubPath)
	delete(s.hubCapWarned, hubPath)
}

// firstCapWarning reports whether this is the first time in the current episode that a connect to the hub at hubPath was refused for want of a start.
func (s *Scheduler) firstCapWarning(hubPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hubCapWarned[hubPath] {
		return false
	}
	s.hubCapWarned[hubPath] = true
	return true
}

// outboundWorker delivers the outbound messages to one receiving hub, one at a time, oldest first.
type outboundWorker struct {
	scheduler *Scheduler
	hubPath   string
	// journalWarned is true while the Warn for a failing journal write of the current episode is out.
	journalWarned bool
}

// run attempts the oldest pending message of its hub and acts on the answer, until nothing is pending.
// An accepted message is journaled delivered and a refused one returned at once.
// Any other answer is retried after a wait that starts at outboundRetryBase and doubles to outboundRetryCeiling, capped at what is left of the message's hour, and a wake during the wait changes nothing.
// A transient answer at or past the hour returns the message with that answer as its reason.
func (w *outboundWorker) run() {
	defer w.scheduler.wg.Done()
	store, clock := w.scheduler.store, w.scheduler.clock
	var (
		current int64
		wait    time.Duration
	)
	for {
		select {
		case <-w.scheduler.stop:
			return
		default:
		}
		pending := store.pendingOutbound(w.hubPath)
		if len(pending) == 0 {
			if w.retire() {
				return
			}
			continue
		}
		entry := pending[0]
		if entry.ID != current {
			current, wait = entry.ID, outboundRetryBase
		}
		if w.conclude(entry, w.attempt(entry)) {
			continue
		}
		remaining := entry.Posted.Add(outboundDeadline).Sub(clock.Now())
		next := min(wait, max(remaining, outboundRetryBase))
		wait = min(wait*2, outboundRetryCeiling)
		select {
		case <-clock.After(next):
		case <-w.scheduler.stop:
			return
		}
	}
}

// retire removes the worker once nothing is pending; it reports false when something arrived meanwhile and the worker carries on.
func (w *outboundWorker) retire() bool {
	s := w.scheduler
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.store.pendingOutbound(w.hubPath)) > 0 && !s.stopping {
		return false
	}
	delete(s.outbound, w.hubPath)
	return true
}

// conclude journals what answer decides for entry and reports whether the entry is settled.
// A journal write that fails leaves the entry pending, logs one Warn per episode and counts as not settled, so the worker waits and tries again.
func (w *outboundWorker) conclude(entry outboundEntry, answer deliverOutcome) bool {
	store := w.scheduler.store
	var err error
	switch {
	case answer.delivery == deliveryAccepted:
		err = store.settleDelivered(entry.ID, answer.remoteID)
	case answer.delivery == deliveryRefused, !w.scheduler.clock.Now().Before(entry.Posted.Add(outboundDeadline)):
		err = store.settleReturned(entry.ID, answer.reason)
	default:
		return false
	}
	if err != nil {
		if !w.journalWarned {
			w.journalWarned = true
			logger.Warn("creel outbound: could not journal the settling of a message to another hub, so it is tried again", "id", entry.ID, "hub", w.hubPath, "error", err)
		}
		return false
	}
	w.journalWarned = false
	return true
}

// attempt makes one deliver call for entry to the daemon of its receiving hub, with no store lock held, and reads the answer.
// The call starts that daemon when none answers, at most maxHubStarts times until a daemon answers a call.
func (w *outboundWorker) attempt(entry outboundEntry) (answer deliverOutcome) {
	scheduler := w.scheduler
	hubs := scheduler.store.hubDirectory()
	geometry, err := hubs.Geometry(w.hubPath)
	if err != nil {
		return deliverOutcome{reason: codeHubUnreadable}
	}
	capped := scheduler.hubStartsCapped(w.hubPath)
	spawnFailed := false
	var spawn func() (Spawn, error)
	if !capped {
		spawn = func() (Spawn, error) {
			started, err := hubs.Spawn(w.hubPath, geometry.DaemonLogPath)
			spawnFailed = err != nil
			if err == nil {
				scheduler.countHubStart(w.hubPath)
			}
			return started, err
		}
	}
	mail := HubMail{
		SendingID: entry.ID, Sender: entry.From, Receiver: entry.To, ReplyTo: entry.ReplyTo,
		ExpectReply: entry.ExpectReply, Posted: entry.Posted, Body: entry.Body,
	}
	resp, err := NewClient(geometry, Claim{}, spawn, scheduler.clock).Call(Request{Verb: VerbDeliver, Mail: &mail}, outboundCallTimeout, nil)
	var (
		refusal *daemonRefusal
		lost    *connLostError
	)
	switch {
	case errors.As(err, &refusal):
		scheduler.hubAnswered(w.hubPath)
		return deliverOutcome{reason: quoteAnswer(refusal.Error())}
	case errors.Is(err, ErrWaitTimeout):
		return deliverOutcome{reason: codeTimeout}
	case spawnFailed:
		return deliverOutcome{reason: codeConnectFailed}
	case errors.As(err, &lost):
		if capped {
			if scheduler.firstCapWarning(w.hubPath) {
				logger.Warn("creel outbound: the daemon of another hub was started too often without answering, so it is not started again until it answers; see its daemon log", "hub", w.hubPath, "daemon_log", geometry.DaemonLogPath)
			}
			return deliverOutcome{reason: codeStartCapped}
		}
		return deliverOutcome{reason: codeConnectFailed}
	case err != nil:
		return deliverOutcome{reason: codeBadAnswer}
	}
	scheduler.hubAnswered(w.hubPath)
	switch resp.Delivery {
	case deliveryAccepted:
		return deliverOutcome{delivery: deliveryAccepted, remoteID: resp.ID}
	case deliveryRefused:
		return deliverOutcome{delivery: deliveryRefused, reason: quoteAnswer(resp.Reason)}
	}
	return deliverOutcome{reason: codeBadAnswer}
}
