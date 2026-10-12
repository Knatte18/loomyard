// store.go — the parent lock record store: one record per parent branch, a ticket queue and compare-and-release.

package parentlock

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	dirName          = "parent-lock"
	recordSuffix     = ".json"
	lockSuffix       = ".lock"
	ticketsDirSuffix = ".tickets"
)

// ErrUnreadable marks a lock record that exists but cannot be read or decoded.
var ErrUnreadable = errors.New("parent lock record unreadable")

// Dir returns the hub-wide parent lock directory under boardDir.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lyxdirs.DotLyxDirName, dirName)
}

// Owner names the run and the process that own a lock or hold a ticket.
type Owner struct {
	Slug      string `json:"slug"`
	Pair      string `json:"pair"`
	RunID     string `json:"run_id"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
}

// SameRun reports whether other names the same run, whatever process it runs in.
func (o Owner) SameRun(other Owner) bool {
	return o.Slug == other.Slug && o.Pair == other.Pair && o.RunID == other.RunID
}

// SameProcess reports whether other names the same run and the same process.
func (o Owner) SameProcess(other Owner) bool {
	return o.SameRun(other) && o.PID == other.PID && o.StartTime == other.StartTime
}

// Record is the lock on one parent branch.
type Record struct {
	Branch     string    `json:"branch"`
	Owner      Owner     `json:"owner"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// Ticket is one waiter's place in a branch's queue.
type Ticket struct {
	Owner    Owner     `json:"owner"`
	QueuedAt time.Time `json:"queued_at"`
}

// Entry is one lock record as List reports it.
// Err holds the failure text of a record that could not be read, in which case only Record.Branch is set.
type Entry struct {
	Record  Record   `json:"record"`
	Alive   bool     `json:"alive"`
	Waiters []Ticket `json:"waiters"`
	Err     string   `json:"err,omitempty"`
}

// Attempt is the result of one TryAcquire.
// Holder is the record the attempt found when it was not granted, and the zero Record when the lock was free but the queue was not the acquirer's turn.
type Attempt struct {
	Granted bool
	Holder  Record
}

// ProcessAlive reports whether pid is alive and carries startTime, so a recycled pid is not taken for its dead predecessor.
func ProcessAlive(pid int, startTime string) bool {
	if !proc.IsAlive(pid) {
		return false
	}
	current, _ := proc.StartTime(pid)
	return current == startTime
}

// Store reads and writes the lock records under Dir.
type Store struct {
	// Dir is the directory holding every record, as Dir returns it.
	Dir string
	// Alive reports whether an owner process lives; nil selects ProcessAlive.
	Alive func(pid int, startTime string) bool
	// Now stamps AcquiredAt and ticket times; nil selects time.Now.
	Now func() time.Time
	// Covered reports whether a pause covers the run of slug; nil covers nothing.
	// A covered run keeps its ticket and place but is never granted, and the queue head is the oldest live ticket of an uncovered run.
	Covered func(slug string) bool
}

func (s *Store) covered(slug string) bool {
	return s.Covered != nil && s.Covered(slug)
}

func (s *Store) alive(owner Owner) bool {
	if s.Alive != nil {
		return s.Alive(owner.PID, owner.StartTime)
	}
	return ProcessAlive(owner.PID, owner.StartTime)
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) recordPath(branch string) string {
	return filepath.Join(s.Dir, hex.EncodeToString([]byte(branch))+recordSuffix)
}

func (s *Store) lockPath(branch string) string {
	return filepath.Join(s.Dir, hex.EncodeToString([]byte(branch))+lockSuffix)
}

func (s *Store) ticketsDir(branch string) string {
	return filepath.Join(s.Dir, hex.EncodeToString([]byte(branch))+ticketsDirSuffix)
}

func (s *Store) ticketPath(branch string, owner Owner) string {
	return filepath.Join(s.ticketsDir(branch), strconv.Itoa(owner.PID)+"-"+owner.StartTime+recordSuffix)
}

// withLock runs fn holding branch's exclusive lock, creating Dir on first use.
func (s *Store) withLock(branch string, fn func() error) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create parent lock directory: %w", err)
	}
	held, err := lock.AcquireWriteLock(s.lockPath(branch))
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := held.Release(); releaseErr != nil {
			logger.Warn("parentlock: release branch lock", "branch", branch, "error", releaseErr)
		}
	}()
	return fn()
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return nil
}

// readRecordFile returns the record at path, found false when none exists.
func readRecordFile(path string) (Record, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, false, fmt.Errorf("%w: %s: %v", ErrUnreadable, path, err)
	}
	return record, true, nil
}

// Read returns branch's record, found false when none exists.
func (s *Store) Read(branch string) (Record, bool, error) {
	return readRecordFile(s.recordPath(branch))
}

// liveTickets returns branch's tickets oldest first.
// A ticket whose owner is not alive, or that cannot be decoded, is dropped from the result and removed from disk when prune is set.
func (s *Store) liveTickets(branch string, prune bool) ([]Ticket, error) {
	dir := s.ticketsDir(branch)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ticket directory: %w", err)
	}
	var tickets []Ticket
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), recordSuffix) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var ticket Ticket
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			readErr = json.Unmarshal(data, &ticket)
		}
		if readErr != nil || !s.alive(ticket.Owner) {
			if prune {
				_ = os.Remove(path)
			}
			continue
		}
		tickets = append(tickets, ticket)
	}
	sort.SliceStable(tickets, func(i, j int) bool { return tickets[i].QueuedAt.Before(tickets[j].QueuedAt) })
	return tickets, nil
}

// TryAcquire makes one attempt to take branch's lock for owner.
// A free record, a record whose owner process is not alive and a record naming owner's run are each taken; a free or stale record goes to the oldest live ticket, so an acquirer that is not first in the queue keeps its ticket and is not granted.
// An acquirer whose run is covered keeps its ticket and is not granted, unless the record already names its own run; the queue head is then the oldest live ticket of an uncovered run, so a covered waiter is passed over and keeps its place.
// A record that cannot be read returns an error wrapping ErrUnreadable and changes nothing.
func (s *Store) TryAcquire(branch string, owner Owner) (Attempt, error) {
	var attempt Attempt
	err := s.withLock(branch, func() error {
		record, found, err := s.Read(branch)
		if err != nil {
			return err
		}
		ownRecord := found && record.Owner.SameRun(owner)
		if found && !ownRecord && s.alive(record.Owner) {
			attempt.Holder = record
			return s.keepTicket(branch, owner)
		}
		if !ownRecord {
			if s.covered(owner.Slug) {
				return s.keepTicket(branch, owner)
			}
			tickets, err := s.liveTickets(branch, true)
			if err != nil {
				return err
			}
			for _, ticket := range tickets {
				if s.covered(ticket.Owner.Slug) {
					continue
				}
				if !ticket.Owner.SameProcess(owner) {
					return s.keepTicket(branch, owner)
				}
				break
			}
		}
		if err := writeJSONAtomic(s.recordPath(branch), Record{Branch: branch, Owner: owner, AcquiredAt: s.now()}); err != nil {
			return fmt.Errorf("write parent lock record: %w", err)
		}
		if err := os.Remove(s.ticketPath(branch, owner)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove parent lock ticket: %w", err)
		}
		attempt.Granted = true
		return nil
	})
	return attempt, err
}

// keepTicket writes owner's ticket unless it already holds one, so a waiter keeps its place in the queue.
func (s *Store) keepTicket(branch string, owner Owner) error {
	path := s.ticketPath(branch, owner)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(s.ticketsDir(branch), 0o755); err != nil {
		return fmt.Errorf("create parent lock ticket directory: %w", err)
	}
	if err := writeJSONAtomic(path, Ticket{Owner: owner, QueuedAt: s.now()}); err != nil {
		return fmt.Errorf("write parent lock ticket: %w", err)
	}
	return nil
}

// Release removes branch's record when its owner is the same process as owner, and reports whether it did.
// A record owned by another process, of the same run or not, is left alone.
func (s *Store) Release(branch string, owner Owner) (bool, error) {
	released := false
	err := s.withLock(branch, func() error {
		record, found, err := s.Read(branch)
		if err != nil || !found || !record.Owner.SameProcess(owner) {
			return err
		}
		if err := os.Remove(s.recordPath(branch)); err != nil {
			return fmt.Errorf("remove parent lock record: %w", err)
		}
		released = true
		return nil
	})
	return released, err
}

// HandOff rewrites branch's record to name to when its owner is the same process as from, and reports whether it did.
func (s *Store) HandOff(branch string, from, to Owner) (bool, error) {
	handed := false
	err := s.withLock(branch, func() error {
		record, found, err := s.Read(branch)
		if err != nil || !found || !record.Owner.SameProcess(from) {
			return err
		}
		record.Owner = to
		if err := writeJSONAtomic(s.recordPath(branch), record); err != nil {
			return fmt.Errorf("write parent lock record: %w", err)
		}
		handed = true
		return nil
	})
	return handed, err
}

// List returns one Entry per record in Dir, in branch-name order of the files.
// A record that cannot be read lists with its error text rather than failing the list.
func (s *Store) List() ([]Entry, error) {
	files, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read parent lock directory: %w", err)
	}
	var entries []Entry
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), recordSuffix) {
			continue
		}
		branch, err := branchFromFileName(file.Name())
		if err != nil {
			entries = append(entries, Entry{Err: err.Error()})
			continue
		}
		record, _, readErr := readRecordFile(filepath.Join(s.Dir, file.Name()))
		if readErr != nil {
			entries = append(entries, Entry{Record: Record{Branch: branch}, Err: readErr.Error()})
			continue
		}
		waiters, err := s.liveTickets(branch, false)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Record: record, Alive: s.alive(record.Owner), Waiters: waiters})
	}
	return entries, nil
}

func branchFromFileName(name string) (string, error) {
	decoded, err := hex.DecodeString(strings.TrimSuffix(name, recordSuffix))
	if err != nil {
		return "", fmt.Errorf("parent lock record name %q is not a hex branch: %w", name, err)
	}
	return string(decoded), nil
}

// ForceRelease removes branch's record whatever its owner and reports whether there was one.
// It never stops the owning run or touches tickets: the released run reacquires at its next lock-taking row.
func (s *Store) ForceRelease(branch string) (bool, error) {
	released := false
	err := s.withLock(branch, func() error {
		err := os.Remove(s.recordPath(branch))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove parent lock record: %w", err)
		}
		released = true
		return nil
	})
	return released, err
}

// ReleaseAll force-releases every record and returns the branch names it released.
func (s *Store) ReleaseAll() ([]string, error) {
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	var released []string
	for _, entry := range entries {
		if entry.Record.Branch == "" {
			continue
		}
		ok, err := s.ForceRelease(entry.Record.Branch)
		if err != nil {
			return released, err
		}
		if ok {
			released = append(released, entry.Record.Branch)
		}
	}
	return released, nil
}
