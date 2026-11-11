// pool.go implements the hub-wide pool of gate slots.
// It covers acquiring a slot, the holder records beside the slot locks, and the environment a slot's child inherits.

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
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// InheritEnv names the environment variable that carries the lock path of the slot a gate run holds,
// so a nested gate run inside it does not acquire a second slot.
const InheritEnv = "LYM_GATE_SLOT"

// PrebuiltLymEnv names the environment variable that carries the path of the `lym` binary.
// A tagged gate run builds it once for all packages, and each test binary runs that build instead.
const PrebuiltLymEnv = "LYM_PREBUILT_LYM"

// StripPrebuilt returns env without PrebuiltLymEnv.
// A command that is not given a fresh build from the same tree must not inherit a stale one.
func StripPrebuilt(env []string) []string {
	stripped := make([]string, 0, len(env))
	for _, entry := range env {
		if key, _, _ := strings.Cut(entry, "="); key != PrebuiltLymEnv {
			stripped = append(stripped, entry)
		}
	}
	return stripped
}

// defaultPoll is the pause between acquire attempts while every slot is held.
const defaultPoll = 2 * time.Second

const (
	slotPrefix        = "slot-"
	lockSuffix        = ".lock"
	holderSuffix      = ".yaml"
	goFlagsEnvName    = "GOFLAGS"
	gateDirName       = "gate"
	drainLockName     = "drain.lock"
	prebuiltDirName   = "prebuilt"
	slotLockPattern   = slotPrefix + "*" + lockSuffix
	holderGlobPattern = slotPrefix + "*" + holderSuffix
)

// Limits are the pool bounds read at every acquire.
type Limits struct {
	// Slots is the number of gate runs that may hold a slot at once.
	Slots int
	// Tier0Parallel is the effective parallel of `tier0`, this machine's override included.
	// Go commands of a non-tier slot holder run under it; it is 0 when the hub declares no `tier0`.
	Tier0Parallel int
	// BusyQuit is the continuous CPU-busy window after which `lym gate test` quits a Go test binary.
	// Zero means the watchdog is off.
	BusyQuit time.Duration
}

// Holder is the record written beside a held slot's lock.
type Holder struct {
	// Worktree is the directory the holding run works in: its worktree root, or the anchored directory inside it.
	Worktree string `yaml:"worktree"`
	// Site is the command or Go-side site label that holds the slot.
	Site string `yaml:"site"`
	// PID is the holding process; Acquire sets it.
	PID int `yaml:"pid"`
	// Started is when the slot was acquired; Acquire sets it.
	Started time.Time `yaml:"started"`
	// Tier is the tier key, such as tier0, of the tier run holding the slot; empty when the holder is not a tier run.
	Tier string `yaml:"tier,omitempty"`
	// Slot is the number of the held slot, read from the record's file name; the record never carries it.
	Slot int `yaml:"-"`
}

// Pool is the hub-wide pool of gate slots, one OS file lock per slot in Dir.
type Pool struct {
	// Dir is the directory that holds the slot locks and holder records.
	Dir string
	// Limits is read at every acquire attempt.
	// A lowered count takes effect at the next acquire, and a running holder of a higher slot finishes.
	Limits func() (Limits, error)
	// Poll is the pause between attempts while every slot is held; zero means two seconds.
	Poll time.Duration
}

// Lease is the slots one holder holds, in ascending slot order.
type Lease struct {
	slots []heldSlot
}

// heldSlot is one slot a Lease holds: its lock and its holder record.
type heldSlot struct {
	lock       *lock.FileLock
	lockPath   string
	recordPath string
}

// Dir returns the hub-wide slot directory under boardDir.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, gateDirName)
}

// PrebuiltDir returns the hub's store of prebuilt `lym` binaries, a directory inside the pool's slot directory.
// A tagged gate run keeps its build there under a name derived from the binary's content, so an unchanged tree hands every run the same path.
func (p *Pool) PrebuiltDir() string {
	return filepath.Join(p.Dir, prebuiltDirName)
}

// Acquire holds one slot for h until the lease is released.
// It tries every slot from 1 to the current Limits().Slots.
// While all are held, it logs the wait start, polls, and logs the wait end on acquiring.
// While a multi-slot acquire holds the drain lock, it waits the same way.
// The drain lock keeps a multi-slot waiter from being starved by new single-slot holders.
// It imposes no deadline of its own; a caller wanting a bound passes a deadline context.
// A cancelled context returns its error with no slot held.
// Polling gives no FIFO order among waiters.
func (p *Pool) Acquire(ctx context.Context, h Holder) (*Lease, error) {
	poll, err := p.prepare(&h)
	if err != nil {
		return nil, err
	}

	var waitStart time.Time
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("wait for gate slot: %w", err)
		}
		limits, err := p.readLimits()
		if err != nil {
			return nil, err
		}
		draining, err := lockIsHeld(p.drainLockPath())
		if err != nil {
			return nil, err
		}
		var lease *Lease
		if !draining {
			if lease, err = p.tryAcquireAny(limits, h); err != nil {
				return nil, err
			}
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
		sleepPoll(ctx, poll)
	}
}

// TryAcquire holds one slot for h until the lease is released, when one is free at once.
// It returns a nil lease otherwise.
// It takes none while a multi-slot acquire holds the drain lock, the state in which Acquire waits.
// It never waits.
func (p *Pool) TryAcquire(h Holder) (*Lease, error) {
	if _, err := p.prepare(&h); err != nil {
		return nil, err
	}
	limits, err := p.readLimits()
	if err != nil {
		return nil, err
	}
	draining, err := lockIsHeld(p.drainLockPath())
	if err != nil || draining {
		return nil, err
	}
	return p.tryAcquireAny(limits, h)
}

// TryAcquireSpare takes one slot for h, held until the lease is released.
// It takes it only when at least two slots are free and no multi-slot acquire holds the drain lock.
// One slot thus stays free after the take, for the next gate run that arrives.
// It returns a nil lease otherwise, and always on a pool of fewer than two slots.
// It never waits.
func (p *Pool) TryAcquireSpare(h Holder) (*Lease, error) {
	if _, err := p.prepare(&h); err != nil {
		return nil, err
	}
	limits, err := p.readLimits()
	if err != nil || limits.Slots < 2 {
		return nil, err
	}
	draining, err := lockIsHeld(p.drainLockPath())
	if err != nil || draining {
		return nil, err
	}
	free := 0
	for number := 1; number <= limits.Slots && free < 2; number++ {
		held, err := lockIsHeld(p.slotLockPath(number))
		if err != nil {
			return nil, err
		}
		if !held {
			free++
		}
	}
	if free < 2 {
		return nil, nil
	}
	return p.tryAcquireAny(limits, h)
}

// Contended reports whether every slot but those lease holds is held, or a multi-slot acquire holds the drain lock.
// A holder that took its slot through TryAcquireSpare gives it back in that state,
// so an arriving gate run waits no longer.
func (p *Pool) Contended(lease *Lease) (bool, error) {
	draining, err := p.Draining()
	if err != nil || draining {
		return draining, err
	}
	limits, err := p.readLimits()
	if err != nil {
		return false, err
	}
	own := map[string]bool{}
	for _, slot := range lease.slots {
		own[slot.lockPath] = true
	}
	for number := 1; number <= limits.Slots; number++ {
		lockPath := p.slotLockPath(number)
		if own[lockPath] {
			continue
		}
		held, err := lockIsHeld(lockPath)
		if err != nil || !held {
			return false, err
		}
	}
	return true, nil
}

// AcquireSlots holds count slots for h until the lease is released.
// A count of 1 takes one slot and does not take the drain lock.
// A count of 0, or above the current Limits().Slots, is clamped to every slot.
// A multi-slot acquire first takes the pool's drain lock at `<Dir>/drain.lock`, an OS file lock.
// Process death releases it, and new single-slot acquires queue behind it.
// It then takes slots one at a time in ascending slot order, holding each while it waits for the next.
// It releases the drain lock once it holds them all.
// The drain admits one multi-slot waiter at a time.
// Every single-slot holder finishes, so two multi-slot waiters never deadlock.
// A cancelled context releases every slot and the drain lock already taken and returns its error with nothing held.
func (p *Pool) AcquireSlots(ctx context.Context, h Holder, count int) (*Lease, error) {
	if count == 1 {
		return p.Acquire(ctx, h)
	}
	poll, err := p.prepare(&h)
	if err != nil {
		return nil, err
	}

	var waitStart time.Time
	noteWait := func() {
		if waitStart.IsZero() {
			waitStart = time.Now()
			logger.Info("gate slot wait start", "site", h.Site, "worktree", h.Worktree, "slots", count)
		}
	}
	drain, err := p.acquireDrain(ctx, poll, noteWait)
	if err != nil {
		return nil, err
	}
	lease, err := p.takeSlotsInOrder(ctx, h, count, poll, noteWait)
	if err != nil {
		return nil, errors.Join(err, drain.Release())
	}
	if err := drain.Release(); err != nil {
		return nil, errors.Join(fmt.Errorf("release gate drain lock: %w", err), lease.Release())
	}
	if !waitStart.IsZero() {
		logger.Info("gate slot wait end", "site", h.Site, "worktree", h.Worktree, "waited", time.Since(waitStart).String())
	}
	return lease, nil
}

// prepare creates the slot directory, stamps h with this process and returns the poll interval.
func (p *Pool) prepare(h *Holder) (time.Duration, error) {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return 0, fmt.Errorf("create gate slot directory: %w", err)
	}
	h.PID = os.Getpid()
	if p.Poll == 0 {
		return defaultPoll, nil
	}
	return p.Poll, nil
}

// readLimits returns the current limits, refusing a count of slots below 1.
func (p *Pool) readLimits() (Limits, error) {
	limits, err := p.Limits()
	if err != nil {
		return Limits{}, fmt.Errorf("read gate limits: %w", err)
	}
	if limits.Slots < 1 {
		return Limits{}, fmt.Errorf("gate limits name %d slots; want at least 1", limits.Slots)
	}
	return limits, nil
}

// sleepPoll waits one poll interval, or until ctx is done.
func sleepPoll(ctx context.Context, poll time.Duration) {
	timer := time.NewTimer(poll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// acquireDrain takes the drain lock, calling noteWait before each wait for another holder to release it.
func (p *Pool) acquireDrain(ctx context.Context, poll time.Duration, noteWait func()) (*lock.FileLock, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("wait for gate drain lock: %w", err)
		}
		drain, acquired, err := lock.TryAcquireWriteLock(p.drainLockPath())
		if err != nil {
			return nil, err
		}
		if acquired {
			return drain, nil
		}
		noteWait()
		sleepPoll(ctx, poll)
	}
}

// takeSlotsInOrder takes slots 1, 2, ... one at a time, holding each while it waits for the next.
// It returns once it holds count slots.
// A count of 0, or above the current Limits().Slots, means every slot.
// Limits are re-read at each attempt, so a lowered count takes effect.
// It does not check the drain lock, which the caller holds.
// On an error or a cancelled context it releases every slot it holds.
func (p *Pool) takeSlotsInOrder(ctx context.Context, h Holder, count int, poll time.Duration, noteWait func()) (*Lease, error) {
	lease := &Lease{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(fmt.Errorf("wait for gate slots: %w", err), lease.Release())
		}
		limits, err := p.readLimits()
		if err != nil {
			return nil, errors.Join(err, lease.Release())
		}
		target := count
		if target < 1 || target > limits.Slots {
			target = limits.Slots
		}
		if len(lease.slots) >= target {
			return lease, nil
		}
		slot, acquired, err := p.tryAcquireSlot(len(lease.slots)+1, h)
		if err != nil {
			return nil, errors.Join(err, lease.Release())
		}
		if acquired {
			lease.slots = append(lease.slots, slot)
			continue
		}
		noteWait()
		sleepPoll(ctx, poll)
	}
}

// tryAcquireAny takes the first free slot, or reports (nil, nil) when every slot is held.
func (p *Pool) tryAcquireAny(limits Limits, h Holder) (*Lease, error) {
	for number := 1; number <= limits.Slots; number++ {
		slot, acquired, err := p.tryAcquireSlot(number, h)
		if err != nil {
			return nil, err
		}
		if acquired {
			return &Lease{slots: []heldSlot{slot}}, nil
		}
	}
	return nil, nil
}

// tryAcquireSlot takes slot number and writes its holder record, or reports false when the slot is held.
// It does not check the drain lock, so the drain holder's own slot takes go through it.
func (p *Pool) tryAcquireSlot(number int, h Holder) (heldSlot, bool, error) {
	lockPath := p.slotLockPath(number)
	fileLock, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil || !acquired {
		return heldSlot{}, false, err
	}
	h.Started = time.Now()
	recordPath := p.holderRecordPath(number)
	data, err := yaml.Marshal(h)
	if err == nil {
		err = fsx.AtomicWriteBytes(recordPath, data)
	}
	if err != nil {
		_ = fileLock.Release()
		return heldSlot{}, false, fmt.Errorf("write gate holder record: %w", err)
	}
	return heldSlot{lock: fileLock, lockPath: lockPath, recordPath: recordPath}, true, nil
}

// Holders returns the holder records of the slots whose lock is held right now, oldest first, each with its slot number.
// A record is trusted only while its lock is held, so a record a crashed holder left behind is dropped.
// It reads every slot record in the directory, so a holder of a slot above a lowered count still shows.
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
		h.Slot, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(recordPath), slotPrefix), holderSuffix))
		holders = append(holders, h)
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].Started.Before(holders[j].Started) })
	return holders, nil
}

// Draining reports whether a multi-slot acquire holds the drain lock.
// While it does, every new single-slot acquire waits, even when a slot is free.
// A pool whose drain lock file was never created is not draining, and the check creates no file there.
func (p *Pool) Draining() (bool, error) {
	if _, err := os.Stat(p.drainLockPath()); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return lockIsHeld(p.drainLockPath())
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

func (p *Pool) drainLockPath() string {
	return filepath.Join(p.Dir, drainLockName)
}

// Release removes the holder record and releases the lock of every slot the lease holds.
// An OS lock is released by process death too, so a crashed holder never leaks a slot.
func (l *Lease) Release() error {
	var errs []error
	for _, slot := range l.slots {
		removeErr := os.Remove(slot.recordPath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		errs = append(errs, removeErr, slot.lock.Release())
	}
	return errors.Join(errs...)
}

// TestCacheOffFlag is the go flag that makes `go test` neither read nor write its test result cache.
// The gate puts it first in the GOFLAGS of every tier command it runs and every non-tier slot holder's child.
// A cacheable test run logs every file and environment variable its test touches into the build cache.
// A gate run's tests touch fresh temp directories, so that log grows large and its result is never reused;
// the gate's own green record already carries a pass over an unchanged tree.
// The build cache is unaffected, and a later `-count` in GOFLAGS or on the command line wins.
const TestCacheOffFlag = "-count=1"

// WithTestCacheOff returns a copy of env whose GOFLAGS opens with TestCacheOffFlag.
// The rest of the value is that of env's last GOFLAGS entry, the one a child sees; env with none gets the flag alone.
// The copy holds one GOFLAGS entry, last.
func WithTestCacheOff(env []string) []string {
	flags := []string{TestCacheOffFlag}
	kept := make([]string, 0, len(env)+1)
	existingFlags := ""
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, goFlagsEnvName) {
			kept = append(kept, entry)
			continue
		}
		existingFlags = value
	}
	if existingFlags != "" {
		flags = append(flags, existingFlags)
	}
	return append(kept, goFlagsEnvName+"="+strings.Join(flags, " "))
}

// HolderEnv returns a copy of base for a non-tier slot holder's child.
// GOFLAGS is TestCacheOffFlag, then the base's value when it has one, then `-p=<tier0Parallel>` and `-trimpath`.
// The `-p` flag is added only when tier0Parallel is above 0.
// A non-nil lease also sets InheritEnv to its first slot's lock path,
// so a gate run inside the child runs without acquiring.
func HolderEnv(base []string, lease *Lease, tier0Parallel int) []string {
	existingFlags := ""
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		switch {
		case strings.EqualFold(key, goFlagsEnvName):
			existingFlags = value
		case key == InheritEnv && lease != nil:
		default:
			env = append(env, entry)
		}
	}
	flags := []string{TestCacheOffFlag}
	if existingFlags != "" {
		flags = append(flags, existingFlags)
	}
	if tier0Parallel > 0 {
		flags = append(flags, "-p="+strconv.Itoa(tier0Parallel))
	}
	flags = append(flags, "-trimpath")
	env = append(env, goFlagsEnvName+"="+strings.Join(flags, " "))
	if lease != nil {
		env = append(env, InheritEnv+"="+lease.slots[0].lockPath)
	}
	return env
}

// InheritEnv returns a copy of base with InheritEnv replaced by the first held slot's lock path,
// so a nested gate run inside the lease runs without acquiring.
// It appends no GOFLAGS: a tier's command and env set its own parallelism.
func (l *Lease) InheritEnv(base []string) []string {
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if key, _, _ := strings.Cut(entry, "="); key != InheritEnv {
			env = append(env, entry)
		}
	}
	return append(env, InheritEnv+"="+l.slots[0].lockPath)
}
