// pool.go — the hub-wide pool of gate slots: acquiring a slot, the holder records beside the slot locks, and the environment a slot's child inherits.

package gateslot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// InheritEnv names the environment variable that carries the lock path of the slot a gate run holds, so a nested gate run inside it does not acquire a second slot.
const InheritEnv = "LYX_GATE_SLOT"

// defaultPoll is the pause between acquire attempts while every slot is held.
const defaultPoll = 2 * time.Second

const (
	slotPrefix        = "slot-"
	lockSuffix        = ".lock"
	holderSuffix      = ".yaml"
	goFlagsEnvName    = "GOFLAGS"
	gateDirName       = "gate"
	slotLockPattern   = slotPrefix + "*" + lockSuffix
	holderGlobPattern = slotPrefix + "*" + holderSuffix
)

// Limits are the pool bounds read at every acquire.
type Limits struct {
	// Slots is the number of gate runs that may hold a slot at once.
	Slots int
	// GoParallel is the `-p` cap each slot holder's go commands run under.
	GoParallel int
	// CLIWait is how long `lyx gate test` waits for a slot before refusing.
	CLIWait time.Duration
}

// Holder is the record written beside a held slot's lock.
type Holder struct {
	// Worktree is the worktree the holding run works in.
	Worktree string `yaml:"worktree"`
	// Site is the command or Go-side site label that holds the slot.
	Site string `yaml:"site"`
	// PID is the holding process; Acquire sets it.
	PID int `yaml:"pid"`
	// Started is when the slot was acquired; Acquire sets it.
	Started time.Time `yaml:"started"`
}

// Pool is the hub-wide pool of gate slots, one OS file lock per slot in Dir.
type Pool struct {
	// Dir is the told slot directory.
	Dir string
	// Limits is read at every acquire attempt, so a lowered count takes effect on the next acquire while a running holder of a higher slot finishes.
	Limits func() (Limits, error)
	// Poll is the pause between attempts while every slot is held; zero means two seconds.
	Poll time.Duration
}

// Lease is one held slot.
type Lease struct {
	lock       *lock.FileLock
	lockPath   string
	recordPath string
	goParallel int
}

// Dir returns the hub-wide slot directory under boardDir.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lyxdirs.DotLyxDirName, gateDirName)
}

// Acquire holds one slot for h until the lease is released.
// It tries every slot from 1 to the current Limits().Slots; while all are held it logs the wait start, polls, and logs the wait end on acquiring.
// It imposes no deadline of its own: a caller wanting a bound passes a deadline context, and a cancelled context returns its error with no slot held.
// Polling gives no FIFO order among waiters.
func (p *Pool) Acquire(ctx context.Context, h Holder) (*Lease, error) {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create gate slot directory: %w", err)
	}
	poll := p.Poll
	if poll == 0 {
		poll = defaultPoll
	}
	h.PID = os.Getpid()

	var waitStart time.Time
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("wait for gate slot: %w", err)
		}
		limits, err := p.Limits()
		if err != nil {
			return nil, fmt.Errorf("read gate limits: %w", err)
		}
		if limits.Slots < 1 {
			return nil, fmt.Errorf("gate limits name %d slots; want at least 1", limits.Slots)
		}
		lease, err := p.tryAcquireAny(limits, h)
		if err != nil {
			return nil, err
		}
		if lease != nil {
			if !waitStart.IsZero() {
				logger.Info("gate slot wait end", "site", h.Site, "worktree", h.Worktree, "waited", time.Since(waitStart).String())
			}
			return lease, nil
		}
		if waitStart.IsZero() {
			waitStart = time.Now()
			logger.Info("gate slot wait start", "site", h.Site, "worktree", h.Worktree)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// tryAcquireAny takes the first free slot, or reports (nil, nil) when every slot is held.
func (p *Pool) tryAcquireAny(limits Limits, h Holder) (*Lease, error) {
	for slot := 1; slot <= limits.Slots; slot++ {
		lockPath := p.slotLockPath(slot)
		fileLock, held, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, err
		}
		if !held {
			continue
		}
		h.Started = time.Now()
		recordPath := p.holderRecordPath(slot)
		data, err := yaml.Marshal(h)
		if err == nil {
			err = fsx.AtomicWriteBytes(recordPath, data)
		}
		if err != nil {
			_ = fileLock.Release()
			return nil, fmt.Errorf("write gate holder record: %w", err)
		}
		return &Lease{lock: fileLock, lockPath: lockPath, recordPath: recordPath, goParallel: limits.GoParallel}, nil
	}
	return nil, nil
}

// Holders returns the holder records of the slots whose lock is held right now, oldest first.
// A record is trusted only while its lock is held, so a record a crashed holder left behind is dropped; it reads every slot record in the directory, so a holder of a slot above a lowered count still shows.
func (p *Pool) Holders() ([]Holder, error) {
	records, err := filepath.Glob(filepath.Join(p.Dir, holderGlobPattern))
	if err != nil {
		return nil, fmt.Errorf("list gate holder records: %w", err)
	}
	var holders []Holder
	for _, recordPath := range records {
		lockPath := strings.TrimSuffix(recordPath, holderSuffix) + lockSuffix
		held, err := lockIsHeld(lockPath)
		if err != nil {
			return nil, err
		}
		if !held {
			continue
		}
		data, err := os.ReadFile(recordPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read gate holder record: %w", err)
		}
		var h Holder
		if err := yaml.Unmarshal(data, &h); err != nil {
			return nil, fmt.Errorf("parse gate holder record %s: %w", recordPath, err)
		}
		holders = append(holders, h)
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].Started.Before(holders[j].Started) })
	return holders, nil
}

// Inherited reports whether slot names a lock file inside Dir whose lock is held.
func (p *Pool) Inherited(slot string) bool {
	slot = filepath.Clean(slot)
	if filepath.Dir(slot) != filepath.Clean(p.Dir) {
		return false
	}
	if matched, _ := filepath.Match(slotLockPattern, filepath.Base(slot)); !matched {
		return false
	}
	held, err := lockIsHeld(slot)
	return err == nil && held
}

// lockIsHeld reports whether another holder has lockPath locked; a lock it can take is released at once.
func lockIsHeld(lockPath string) (bool, error) {
	fileLock, free, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, err
	}
	if !free {
		return true, nil
	}
	return false, fileLock.Release()
}

func (p *Pool) slotLockPath(slot int) string {
	return filepath.Join(p.Dir, slotPrefix+strconv.Itoa(slot)+lockSuffix)
}

func (p *Pool) holderRecordPath(slot int) string {
	return filepath.Join(p.Dir, slotPrefix+strconv.Itoa(slot)+holderSuffix)
}

// Release removes the holder record and releases the slot lock.
// An OS lock is released by process death too, so a crashed holder never leaks a slot.
func (l *Lease) Release() error {
	removeErr := os.Remove(l.recordPath)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	releaseErr := l.lock.Release()
	return errors.Join(removeErr, releaseErr)
}

// Env returns a copy of base for a child that runs inside this slot.
// GOFLAGS carries the base's value with the slot's `-p` cap appended, and InheritEnv names the held slot's lock path.
func (l *Lease) Env(base []string) []string {
	existingFlags := ""
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		switch {
		case strings.EqualFold(key, goFlagsEnvName):
			existingFlags = value
		case key == InheritEnv:
		default:
			env = append(env, entry)
		}
	}
	return append(env, goFlagsEnvName+"="+GoFlags(existingFlags, l.goParallel), InheritEnv+"="+l.lockPath)
}

// GoFlags appends `-p=<goParallel>` to an existing GOFLAGS value, space-separated, or returns it alone when existing is empty.
func GoFlags(existing string, goParallel int) string {
	parallelFlag := "-p=" + strconv.Itoa(goParallel)
	if existing == "" {
		return parallelFlag
	}
	return existing + " " + parallelFlag
}
