// fold.go folds the journal's records into the pool state: messages, queued injects, holds and known program mailboxes.

package creelengine

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Status is where a message stands for its holder.
type Status string

// Message statuses.
const (
	StatusUnread      Status = "unread"
	StatusRead        Status = "read"
	StatusUndelivered Status = "undelivered"
	StatusDelivered   Status = "delivered"
)

// Message is one piece of mail in the pool.
type Message struct {
	ID int64
	// From is the original sender and never changes.
	From string
	// Envelope is what creel saw of an EXTERNAL sender; empty for any other.
	Envelope string
	// Holder is who the message is with now.
	Holder string
	// Path lists the forwarders the message passed through, oldest first.
	Path []string
	// Kind is mail or reply.
	Kind string
	// Subject is the first line of Body.
	Subject string
	Body    string
	// ReplyTo is the id of the message a reply answers, 0 for mail.
	ReplyTo int64
	Posted  time.Time
	Status  Status
	// EndedUnreadBy is the receiver the message was returned from because it never read it, with the time of the return; empty otherwise.
	EndedUnreadBy string
	EndedUnreadAt time.Time

	// origin is the journal day of the file holding the record that created the entry.
	origin string
}

// Inject outcomes; an inject still queued has none.
const (
	injectQueued      = ""
	injectSending     = "sending"
	injectInjected    = "injected"
	injectUnconfirmed = "unconfirmed"
	injectExpired     = "expired"
)

// injectEntry is one text queued to be typed into a receiver's pane.
type injectEntry struct {
	ID       int64
	From     string
	Receiver string
	Text     string
	TTL      time.Duration
	// Consumed is the part of TTL already used, from the last aged event.
	Consumed time.Duration
	// Head marks an inject queued at the head of its receiver's queue.
	Head    bool
	Outcome string
	Posted  time.Time

	origin string
}

// holdEntry is a hold on a receiver: only its owner's injects pass until it expires.
type holdEntry struct {
	Owner   string
	Expires time.Time

	origin string
}

// reminderEntry is the last reminder sent to a receiver.
type reminderEntry struct {
	At   time.Time
	Step int
	// Unread is the ids the reminder listed, oldest first.
	Unread []int64
}

// LogEntry is one event as the log verb shows it: ids, parties, time and subject, never a body.
type LogEntry struct {
	Day      string
	Event    string
	At       time.Time
	ID       int64
	From     string
	To       string
	By       string
	Receiver string
	Subject  string
	// Kind is the kind of the message or inject a carried entry snapshots, empty on every other entry.
	Kind   string
	Notice string
	// Reason is the stall a stalled event names.
	Reason string
	// Unread is the message ids a reminded event listed, oldest first.
	Unread []int64
	// Step is the backoff step a reminded event was sent at.
	Step int
}

// pool is the state folded from the journal.
// It is not safe for concurrent use: the store holds its mutex around every call.
type pool struct {
	messages  map[int64]*Message
	injects   map[int64]*injectEntry
	holds     map[string]*holdEntry
	reminders map[string]reminderEntry
	// programs holds the program senders whose mailbox is known, from their first post or read.
	programs map[string]bool
	events   []LogEntry
	// maxID is the largest id any record or high-water record carried.
	maxID int64
}

func newPool() *pool {
	return &pool{
		messages:  map[int64]*Message{},
		injects:   map[int64]*injectEntry{},
		holds:     map[string]*holdEntry{},
		reminders: map[string]reminderEntry{},
		programs:  map[string]bool{},
	}
}

// newID returns an id above every id issued so far, and counts it as issued.
func (p *pool) newID() int64 {
	p.maxID++
	return p.maxID
}

// foldJournal folds the day files in dir, oldest first, into a pool; a missing or empty directory folds to an empty pool.
// A torn last line of the newest file, left by a crash, is skipped with one Warn.
// A malformed line anywhere else fails the load, since skipping it could drop a message silently.
func foldJournal(dir string) (*pool, error) {
	p := newPool()
	names, err := listJournalFiles(dir)
	if err != nil {
		return nil, err
	}
	for fileIndex, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("creel journal: read %s: %w", path, err)
		}
		lines := bytes.Split(data, []byte("\n"))
		last := lastNonBlankLine(lines)
		for index, line := range lines {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			rec, err := parseRecord(line)
			if err != nil {
				if fileIndex == len(names)-1 && index == last {
					logger.Warn("creel journal: skipping a torn last line left by a crash", "file", path, "line", index+1)
					continue
				}
				return nil, fmt.Errorf("creel journal: %s line %d is not a valid record (%v); way forward: repair or remove that line by hand, then start the daemon again", path, index+1, err)
			}
			p.apply(rec, dayOf(name))
		}
	}
	p.finishFold()
	return p, nil
}

// lastNonBlankLine returns the index of the last line whose trimmed text is not empty, or -1 when there is none.
// The fold and the journal's tail repair share it, so the line the fold skips as torn is the line the repair cuts.
func lastNonBlankLine(lines [][]byte) int {
	last := len(lines) - 1
	for last >= 0 && len(bytes.TrimSpace(lines[last])) == 0 {
		last--
	}
	return last
}

// parseRecord decodes one journal line as a record.
// A line that is valid JSON but no record, such as an array, fails.
func parseRecord(line []byte) (record, error) {
	var rec record
	err := json.Unmarshal(line, &rec)
	return rec, err
}

// finishFold settles what a crash leaves open: an inject with a sending event and no outcome folds as unconfirmed, so one in flight is never retyped.
func (p *pool) finishFold() {
	for _, entry := range p.injects {
		if entry.Outcome == injectSending {
			entry.Outcome = injectUnconfirmed
		}
	}
}

// apply folds one record from the file of the given day.
// It is idempotent by id for the records that create an entry, so a carried record and the original line fold to one entry.
func (p *pool) apply(rec record, day string) {
	p.maxID = max(p.maxID, rec.ID)
	switch rec.Event {
	case eventHighWater:
		return
	case eventPosted:
		p.applyPosted(rec, day)
	case eventReminded:
		p.reminders[rec.To] = reminderEntry{At: rec.At, Step: rec.Step, Unread: rec.Unread}
	case eventRead:
		p.applyRead(rec)
		// A read with no id only makes a program's mailbox known and is no event for the log.
		if rec.ID == 0 {
			return
		}
	case eventForwarded:
		if msg := p.messages[rec.ID]; msg != nil {
			msg.Path = append(msg.Path, rec.By)
			msg.Holder = rec.To
		}
	case eventReturned:
		if msg := p.messages[rec.ID]; msg != nil {
			if n := len(msg.Path); n > 0 && msg.Path[n-1] == rec.To {
				msg.Path = msg.Path[:n-1]
				if len(msg.Path) == 0 {
					msg.Path = nil
				}
			}
			msg.Holder = rec.To
			msg.EndedUnreadBy, msg.EndedUnreadAt = rec.Receiver, rec.At
		}
	case eventUndelivered:
		p.setStatus(rec.ID, StatusUndelivered)
	case eventDelivered:
		p.setStatus(rec.ID, StatusDelivered)
	case eventSending:
		p.setOutcome(rec.ID, injectSending)
	case eventInjected:
		p.setOutcome(rec.ID, injectInjected)
	case eventUnconfirmed:
		p.setOutcome(rec.ID, injectUnconfirmed)
	case eventExpired:
		p.setOutcome(rec.ID, injectExpired)
	case eventAged:
		if entry := p.injects[rec.ID]; entry != nil {
			entry.Consumed = time.Duration(rec.ConsumedSeconds) * time.Second
		}
	case eventInterrupted, eventStalled, eventWithheld:
	case eventHeld:
		p.holds[rec.To] = &holdEntry{Owner: rec.By, Expires: rec.ExpiresAt, origin: day}
	case eventReleased:
		delete(p.holds, rec.To)
	case eventCarried:
		if !p.applyCarried(rec, day) {
			return
		}
	default:
		logger.Debug("creel journal: ignoring a record of an event this binary does not know", "event", rec.Event, "id", rec.ID)
		return
	}
	p.events = append(p.events, p.logEntryFor(rec, day))
}

func (p *pool) applyPosted(rec record, day string) {
	if rec.Kind == kindInject {
		if p.injects[rec.ID] == nil {
			p.injects[rec.ID] = &injectEntry{
				ID: rec.ID, From: rec.From, Receiver: rec.To, Text: rec.Body,
				TTL: time.Duration(rec.TTLSeconds) * time.Second, Head: rec.Head,
				Outcome: injectQueued, Posted: rec.At, origin: day,
			}
		}
		return
	}
	if isProgramMailbox(rec.From) {
		p.programs[rec.From] = true
	}
	if p.messages[rec.ID] != nil {
		return
	}
	kind := rec.Kind
	if kind == "" {
		kind = kindMail
	}
	p.messages[rec.ID] = &Message{
		ID: rec.ID, From: rec.From, Envelope: rec.Envelope, Holder: rec.To, Kind: kind,
		Subject: firstLine(rec.Body), Body: rec.Body, ReplyTo: rec.ReplyTo,
		Posted: rec.At, Status: StatusUnread, origin: day,
	}
}

func (p *pool) applyRead(rec record) {
	if isProgramMailbox(rec.By) {
		p.programs[rec.By] = true
	}
	if msg := p.messages[rec.ID]; msg != nil {
		msg.Status = StatusRead
		msg.EndedUnreadBy, msg.EndedUnreadAt = "", time.Time{}
	}
}

// applyCarried folds a retention snapshot; an entry the pool already holds is left as the original lines made it.
// It reports whether the record snapshots a hold, message or inject this binary knows.
func (p *pool) applyCarried(rec record, day string) bool {
	switch {
	case rec.Of == eventHeld:
		p.holds[rec.To] = &holdEntry{Owner: rec.By, Expires: rec.ExpiresAt, origin: day}
	case rec.Of == eventPosted && rec.Kind == kindInject:
		if p.injects[rec.ID] == nil {
			p.injects[rec.ID] = &injectEntry{
				ID: rec.ID, From: rec.From, Receiver: rec.To, Text: rec.Body,
				TTL:      time.Duration(rec.TTLSeconds) * time.Second,
				Consumed: time.Duration(rec.ConsumedSeconds) * time.Second,
				Head:     rec.Head, Outcome: injectQueued, Posted: rec.OrigAt, origin: day,
			}
		}
	case rec.Of == eventPosted:
		if isProgramMailbox(rec.From) {
			p.programs[rec.From] = true
		}
		if p.messages[rec.ID] == nil {
			kind := rec.Kind
			if kind == "" {
				kind = kindMail
			}
			p.messages[rec.ID] = &Message{
				ID: rec.ID, From: rec.From, Envelope: rec.Envelope, Holder: rec.To, Path: rec.Path, Kind: kind,
				Subject: firstLine(rec.Body), Body: rec.Body, ReplyTo: rec.ReplyTo,
				Posted: rec.OrigAt, Status: StatusUnread,
				EndedUnreadBy: rec.Receiver, EndedUnreadAt: rec.ReceiverAt, origin: day,
			}
		}
	default:
		logger.Debug("creel journal: ignoring a carried record of an entry this binary does not know", "of", rec.Of, "id", rec.ID)
		return false
	}
	return true
}

func (p *pool) setStatus(id int64, status Status) {
	if msg := p.messages[id]; msg != nil {
		msg.Status = status
	}
}

func (p *pool) setOutcome(id int64, outcome string) {
	if entry := p.injects[id]; entry != nil {
		entry.Outcome = outcome
	}
}

// logEntryFor builds the log line of rec: parties and subject from the message or inject it concerns, never a body.
// A carried record's line carries the snapshot's kind, mail when the record names none.
func (p *pool) logEntryFor(rec record, day string) LogEntry {
	entry := LogEntry{
		Day: day, Event: rec.Event, At: rec.At, ID: rec.ID,
		From: rec.From, To: rec.To, By: rec.By, Receiver: rec.Receiver, Notice: rec.Notice, Reason: rec.Reason,
		Unread: rec.Unread, Step: rec.Step,
	}
	if msg := p.messages[rec.ID]; msg != nil {
		entry.From, entry.Subject = msg.From, msg.Subject
		if entry.To == "" {
			entry.To = msg.Holder
		}
	} else if inject := p.injects[rec.ID]; inject != nil {
		entry.From = inject.From
		if entry.To == "" {
			entry.To = inject.Receiver
		}
	}
	if rec.Event == eventCarried {
		// A carried record's receiver field holds who a returned message ended unread for, which the entry does not show.
		entry.Receiver = ""
		if rec.Of == eventPosted {
			entry.Kind = cmp.Or(rec.Kind, kindMail)
		}
	}
	return entry
}

// carryRecords returns a carried record for every unread message, queued inject and live hold recorded in a file older than cutoffDay.
func (p *pool) carryRecords(cutoffDay string, now time.Time) []record {
	var carried []record
	for _, msg := range p.messages {
		if msg.origin < cutoffDay && msg.Status == StatusUnread {
			carried = append(carried, record{
				Event: eventCarried, At: now, Of: eventPosted, ID: msg.ID, From: msg.From, Envelope: msg.Envelope, To: msg.Holder,
				Kind: msg.Kind, Body: msg.Body, ReplyTo: msg.ReplyTo, Path: msg.Path,
				Receiver: msg.EndedUnreadBy, ReceiverAt: msg.EndedUnreadAt, OrigAt: msg.Posted,
			})
		}
	}
	for _, entry := range p.injects {
		if entry.origin < cutoffDay && entry.Outcome == injectQueued {
			carried = append(carried, record{
				Event: eventCarried, At: now, Of: eventPosted, Kind: kindInject, ID: entry.ID, From: entry.From, To: entry.Receiver,
				Body: entry.Text, TTLSeconds: int64(entry.TTL / time.Second), ConsumedSeconds: int64(entry.Consumed / time.Second),
				Head: entry.Head, OrigAt: entry.Posted,
			})
		}
	}
	sort.Slice(carried, func(i, j int) bool { return carried[i].ID < carried[j].ID })
	var receivers []string
	for receiver, hold := range p.holds {
		if hold.origin < cutoffDay && hold.Expires.After(now) {
			receivers = append(receivers, receiver)
		}
	}
	sort.Strings(receivers)
	for _, receiver := range receivers {
		hold := p.holds[receiver]
		carried = append(carried, record{Event: eventCarried, At: now, Of: eventHeld, To: receiver, By: hold.Owner, ExpiresAt: hold.Expires})
	}
	return carried
}

// moveOrigins marks the entries carryRecords returned as recorded in day's file, so the next retention does not carry them again.
func (p *pool) moveOrigins(cutoffDay, day string, now time.Time) {
	for _, msg := range p.messages {
		if msg.origin < cutoffDay && msg.Status == StatusUnread {
			msg.origin = day
		}
	}
	for _, entry := range p.injects {
		if entry.origin < cutoffDay && entry.Outcome == injectQueued {
			entry.origin = day
		}
	}
	for _, hold := range p.holds {
		if hold.origin < cutoffDay && hold.Expires.After(now) {
			hold.origin = day
		}
	}
}

// prune drops what only deleted files held: entries recorded before cutoffDay that were not carried, and their log lines.
func (p *pool) prune(cutoffDay string) {
	for id, msg := range p.messages {
		if msg.origin < cutoffDay {
			delete(p.messages, id)
		}
	}
	for id, entry := range p.injects {
		if entry.origin < cutoffDay {
			delete(p.injects, id)
		}
	}
	for receiver, hold := range p.holds {
		if hold.origin < cutoffDay {
			delete(p.holds, receiver)
		}
	}
	kept := p.events[:0]
	for _, entry := range p.events {
		if entry.Day >= cutoffDay {
			kept = append(kept, entry)
		}
	}
	p.events = kept
}

// isProgramMailbox reports whether full names a program sender that has a mailbox: EXTERNAL has none.
func isProgramMailbox(full string) bool {
	_, name, err := agentname.ParseProgram(full)
	return err == nil && name != agentname.External
}

// firstLine returns text up to its first newline.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
