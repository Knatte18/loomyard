// priority.go orders the waiters for gate slots by priority.
// It covers the queue ticket a waiter holds while it waits and the check that makes a lower-priority acquirer stand back.

package gateslot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Priority orders the acquirers of gate slots.
// While a waiter of a higher priority waits, no acquirer of a lower one takes a slot or the drain lock,
// so a slot that frees goes to a waiter of the highest priority waiting.
// A slot already held is never taken back.
type Priority int

const (
	// PriorityOther is the zero value: every acquirer that is neither a lock holder's gate nor an implementer's.
	PriorityOther Priority = iota
	// PriorityImplementer is a gate call that checks an implementer's card or batch.
	PriorityImplementer
	// PriorityLockHolder is a gate call whose run holds a parent lock.
	// Its landing moves the parent, which makes the gates of the runs queued behind that lock stale.
	PriorityLockHolder
)

// SlotPriority returns the priority a call of the gate key key takes its slots with.
// A call whose run holds a parent lock comes first, whatever its key.
// A call of the card or batch key comes next, and every other call, one with no key included, comes last.
func SlotPriority(key string, holdsParentLock bool) Priority {
	switch {
	case holdsParentLock:
		return PriorityLockHolder
	case key == CardGateKey || key == BatchGateKey:
		return PriorityImplementer
	default:
		return PriorityOther
	}
}

const (
	queueDirName = "queue"
	ticketSuffix = ".lock"
	// staleTicketAge is the age past which a ticket file whose lock no process holds is removed.
	// A younger one may belong to a waiter that has created the file and not yet locked it.
	staleTicketAge = time.Minute
)

// ticketSequence makes the ticket names of one process unique.
var ticketSequence atomic.Uint64

// ticket is the queue file a waiter holds an OS lock on while it waits.
// Process death releases the lock, so a crashed waiter outranks nobody.
type ticket struct {
	lock *lock.FileLock
	path string
}

func (p *Pool) queueDir() string {
	return filepath.Join(p.Dir, queueDirName)
}

// queueTicket holds a ticket naming priority at `<Dir>/queue/<priority>-<pid>-<n>.lock` until it is dropped.
// PriorityOther outranks nobody, so it gets no ticket and a nil return.
func (p *Pool) queueTicket(priority Priority) (*ticket, error) {
	if priority == PriorityOther {
		return nil, nil
	}
	if err := os.MkdirAll(p.queueDir(), 0o755); err != nil {
		return nil, fmt.Errorf("create gate queue directory: %w", err)
	}
	name := fmt.Sprintf("%d-%d-%d%s", priority, os.Getpid(), ticketSequence.Add(1), ticketSuffix)
	path := filepath.Join(p.queueDir(), name)
	fileLock, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return nil, fmt.Errorf("take gate queue ticket: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("take gate queue ticket: %s is locked by another holder", path)
	}
	return &ticket{lock: fileLock, path: path}, nil
}

// dropTicket removes the ticket's file and then releases its lock; a nil ticket holds nothing.
// A failure is logged, since the wait it served has already ended.
func dropTicket(t *ticket) {
	if t == nil {
		return
	}
	removeErr := os.Remove(t.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	if err := errors.Join(removeErr, t.lock.Release()); err != nil {
		logger.Warn("gate slot: could not drop the queue ticket", "ticket", t.path, "cause", err)
	}
}

// outranked reports whether a waiter of a priority above priority holds its ticket.
// A ticket file of a higher priority that no process holds is removed once it is older than staleTicketAge.
func (p *Pool) outranked(priority Priority) (bool, error) {
	entries, err := os.ReadDir(p.queueDir())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("list gate queue tickets: %w", err)
	}
	for _, entry := range entries {
		rank, _, found := strings.Cut(entry.Name(), "-")
		if !found || !strings.HasSuffix(entry.Name(), ticketSuffix) {
			continue
		}
		if ranked, err := strconv.Atoi(rank); err != nil || Priority(ranked) <= priority {
			continue
		}
		path := filepath.Join(p.queueDir(), entry.Name())
		fileLock, free, err := lock.TryAcquireWriteLock(path)
		if err != nil {
			return false, err
		}
		if !free {
			return true, nil
		}
		// The file is removed while the probe holds its lock, so no waiter can be holding it meanwhile.
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > staleTicketAge {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				logger.Warn("gate slot: could not remove a stale queue ticket", "ticket", path, "cause", err)
			}
		}
		if err := fileLock.Release(); err != nil {
			return false, err
		}
	}
	return false, nil
}
