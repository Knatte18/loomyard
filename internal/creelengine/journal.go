// journal.go holds the append-only journal: the record shape, the day files, the writer with its fsync and the retention that carries live state forward.

package creelengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Journal event names.
const (
	eventPosted      = "posted"
	eventReminded    = "reminded"
	eventRead        = "read"
	eventForwarded   = "forwarded"
	eventReturned    = "returned"
	eventUndelivered = "undelivered"
	eventDelivered   = "delivered"
	eventSending     = "sending"
	eventInjected    = "injected"
	eventUnconfirmed = "unconfirmed"
	eventExpired     = "expired"
	eventAged        = "aged"
	eventInterrupted = "interrupted"
	// eventStalled marks a receiver whose delivery waits on a stall reason that changed.
	eventStalled  = "stalled"
	eventHeld     = "held"
	eventReleased = "released"
	// eventWithheld marks a stage 1 journal's held-back orch reminder; the fold reads it from old journals only and nothing writes it.
	eventWithheld = "withheld"
	// eventCarried is written by retention: a snapshot of an entry whose original lines are about to be deleted.
	eventCarried = "carried"
	// eventHighWater opens every day file with the largest id issued so far.
	eventHighWater = "high-water"
)

// Kinds of a posted record.
const (
	kindMail   = "mail"
	kindReply  = "reply"
	kindInject = "inject"
)

const (
	// journalRetention is how old a day file grows before retention carries its live state forward and deletes it.
	journalRetention = 7 * 24 * time.Hour
	dayLayout        = "2006-01-02"
	journalSuffix    = ".jsonl"
)

var journalFileName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.jsonl$`)

// record is one journal line.
// A reader ignores keys it does not know, so a newer binary's fields never refuse a load.
type record struct {
	Event string    `json:"event"`
	At    time.Time `json:"at"`
	// ID is a message or inject id; a high-water record carries the largest id issued.
	ID int64 `json:"id,omitempty"`
	// From is the sender of a posted record.
	From string `json:"from,omitempty"`
	// Envelope is what creel saw of an EXTERNAL sender: parent process, terminal and working directory.
	Envelope string `json:"envelope,omitempty"`
	// To is the receiver or holder the record concerns.
	To string `json:"to,omitempty"`
	// By is the party that acted: the reader, forwarder or hold owner.
	By   string `json:"by,omitempty"`
	Kind string `json:"kind,omitempty"`
	// Body is a message body or an inject's text.
	Body    string   `json:"body,omitempty"`
	ReplyTo int64    `json:"reply_to,omitempty"`
	Path    []string `json:"path,omitempty"`
	// Receiver is the receiver that ended a returned message unread.
	Receiver string `json:"receiver,omitempty"`
	// TTLSeconds is an inject's time to live, and ConsumedSeconds the part of it already used.
	TTLSeconds      int64 `json:"ttl_s,omitempty"`
	ConsumedSeconds int64 `json:"consumed_s,omitempty"`
	// Head marks an inject queued at the head of its receiver's queue.
	Head bool `json:"head,omitempty"`
	// ExpiresAt is a hold's expiry.
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	// Notice names what a stage 1 withheld record waited for; it is read from old journals only.
	Notice string `json:"notice,omitempty"`
	// Reason is the stall a stalled record names.
	Reason string `json:"reason,omitempty"`
	// Step is the reminder backoff step of a reminded record.
	Step int `json:"step,omitempty"`
	// Unread is the set of message ids a reminded record reminded about.
	Unread []int64 `json:"unread,omitempty"`
	// Of names the entry a carried record snapshots: posted for a message or inject, held for a hold.
	Of string `json:"of,omitempty"`
	// OrigAt is the time a carried entry was first recorded, and ReceiverAt the time its returned message ended unread.
	OrigAt     time.Time `json:"orig_at,omitzero"`
	ReceiverAt time.Time `json:"receiver_at,omitzero"`
}

// journalDay returns the UTC day a journal file name or time falls on, as YYYY-MM-DD.
func journalDay(t time.Time) string { return t.UTC().Format(dayLayout) }

// listJournalFiles returns the day-file names in dir in ascending day order; a missing directory has none.
func listJournalFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("creel journal: read %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && journalFileName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// dayOf returns the day a journal file name stands for.
func dayOf(fileName string) string { return strings.TrimSuffix(fileName, journalSuffix) }

// journalWriter appends records to the current day's file and folds each into the pool it was opened over.
// It is not safe for concurrent use: the store serializes every call under its mutex.
type journalWriter struct {
	dir   string
	clock Clock
	pool  *pool
	day   string
	file  *os.File
}

// openJournal prepares the writer over the pool folded from dir.
// A torn last line left by a crash in the newest file is cut off, so the next append starts on a clean line.
// The day file itself is opened by the first append.
func openJournal(dir string, clock Clock, state *pool) (*journalWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creel journal: create %s: %w", dir, err)
	}
	names, err := listJournalFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		if err := repairTail(filepath.Join(dir, names[len(names)-1])); err != nil {
			return nil, err
		}
	}
	return &journalWriter{dir: dir, clock: clock, pool: state}, nil
}

// repairTail leaves the file ending on a clean line.
// A complete last line that lacks its newline gets one.
// A torn last non-blank line, with or without its newline, is cut off together with any blank lines after it.
// It is the same line the fold skips as torn, so no record a fold applies is removed.
func repairTail(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("creel journal: read %s: %w", path, err)
	}
	lines := bytes.Split(data, []byte("\n"))
	tail := lastNonBlankLine(lines)
	if tail < 0 {
		return nil
	}
	if _, err := parseRecord(lines[tail]); err == nil {
		if tail == len(lines)-1 {
			return appendBytes(path, []byte("\n"))
		}
		return nil
	}
	cut := 0
	for _, line := range lines[:tail] {
		cut += len(line) + 1
	}
	logger.Warn("creel journal: cutting a torn last line left by a crash", "file", path, "bytes", len(data)-cut)
	if err := os.Truncate(path, int64(cut)); err != nil {
		return fmt.Errorf("creel journal: cut the torn last line of %s: %w", path, err)
	}
	return nil
}

func appendBytes(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creel journal: open %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("creel journal: write %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("creel journal: sync %s: %w", path, err)
	}
	return file.Close()
}

// append writes rec as one line, syncs it to disk and then folds it into the pool.
// A record with no time gets the clock's now.
// At a UTC day change it first opens the new day file and runs retention.
func (w *journalWriter) append(rec record) error {
	now := w.clock.Now().UTC()
	if rec.At.IsZero() {
		rec.At = now
	}
	if day := journalDay(now); day != w.day {
		if err := w.rotate(now, day); err != nil {
			return err
		}
	}
	if err := w.writeLine(rec); err != nil {
		return err
	}
	w.pool.apply(rec, w.day)
	return nil
}

// close closes the open day file.
func (w *journalWriter) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// writeLine writes rec as one line to the open day file and syncs it.
func (w *journalWriter) writeLine(rec record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("creel journal: encode a %s record: %w", rec.Event, err)
	}
	if _, err := w.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("creel journal: write %s: %w", w.file.Name(), err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("creel journal: sync %s: %w", w.file.Name(), err)
	}
	return nil
}

// rotate opens day's file, starts a new one with the high-water record, and carries live state out of files past retention.
func (w *journalWriter) rotate(now time.Time, day string) error {
	if err := w.close(); err != nil {
		return fmt.Errorf("creel journal: close the previous day file: %w", err)
	}
	path := filepath.Join(w.dir, day+journalSuffix)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creel journal: open %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("creel journal: stat %s: %w", path, err)
	}
	w.file, w.day = file, day
	if info.Size() == 0 {
		if err := w.writeLine(record{Event: eventHighWater, At: now, ID: w.pool.maxID}); err != nil {
			return err
		}
	}
	return w.retain(now)
}

// retain appends a carried record for every unread message, queued inject and live hold recorded in a file older than the retention, then deletes those files.
// The carried records are synced before any deletion, so a crash between the two leaves both and the fold collapses them by id.
// Each carried record is also folded into the pool, so the log lists the carry at once.
func (w *journalWriter) retain(now time.Time) error {
	names, err := listJournalFiles(w.dir)
	if err != nil {
		return err
	}
	cutoff := journalDay(now.Add(-journalRetention))
	var old []string
	for _, name := range names {
		if dayOf(name) < cutoff {
			old = append(old, name)
		}
	}
	if len(old) == 0 {
		return nil
	}
	for _, rec := range w.pool.carryRecords(cutoff, now) {
		if err := w.writeLine(rec); err != nil {
			return err
		}
		w.pool.apply(rec, w.day)
	}
	w.pool.moveOrigins(cutoff, w.day, now)
	for _, name := range old {
		if err := os.Remove(filepath.Join(w.dir, name)); err != nil {
			logger.Warn("creel journal: could not delete a day file past retention", "file", name, "error", err)
		}
	}
	w.pool.prune(cutoff)
	return nil
}
