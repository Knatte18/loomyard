// Package creelfake fakes the two seams creelengine is told: Delivery, the hub's strands and their panes, and Clock.
//
// Delivery answers from scripted strands, idle and pause flags and errors, and records every send, interrupt and idle probe.
// Clock advances only when a test says so and records every duration passed to After, in order.
// Neither fake asserts anything or takes a testing.TB; a test reads the recorded values and asserts itself.
//
// creelengine's tests that use the kit are external (package creelengine_test), since an in-package test importing the kit is an import cycle.
package creelfake

import (
	"sort"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

// DefaultPanePID is the pane pid a scripted strand carries when its pid is left zero.
// No real process has it, so a live strand left at its default is neither an ancestor of the test process nor refused as unreadable.
const DefaultPanePID = 999_999_999

// LiveStrand returns a live strand with the given full name and the default pane pid.
func LiveStrand(name string) creelengine.Strand {
	return creelengine.Strand{Name: name, GUID: "guid-" + name, Worktree: "/hub/worktree", Live: true, PanePID: DefaultPanePID}
}

// DormantStrand returns a strand with the given full name that has no running pane.
func DormantStrand(name string) creelengine.Strand {
	strand := LiveStrand(name)
	strand.Live = false
	return strand
}

// Send is one call to Delivery.Send.
type Send struct {
	Strand string
	Text   string
}

// Delivery is a creelengine.Delivery whose answers are scripted.
// A strand is idle unless set otherwise and no pause covers it.
type Delivery struct {
	mu      sync.Mutex
	strands []creelengine.Strand
	idle    map[string]bool
	paused  map[string]bool

	strandsErr, idleErr, pausedErr, sendErr, interruptErr error
	sendFn                                                func(strand, text string) error

	sends      []Send
	interrupts []string
	idleCalls  map[string]int
}

// NewDelivery returns a Delivery with no strands.
func NewDelivery() *Delivery {
	return &Delivery{idle: map[string]bool{}, paused: map[string]bool{}, idleCalls: map[string]int{}}
}

// SetStrands replaces the strand list; a strand with pane pid zero gets DefaultPanePID.
func (d *Delivery) SetStrands(strands ...creelengine.Strand) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.strands = nil
	for _, strand := range strands {
		if strand.PanePID == 0 {
			strand.PanePID = DefaultPanePID
		}
		d.strands = append(d.strands, strand)
	}
}

// SetIdle scripts whether the named strand reads idle.
func (d *Delivery) SetIdle(name string, idle bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idle[name] = idle
}

// SetPaused scripts whether a pause covers the named strand.
func (d *Delivery) SetPaused(name string, paused bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.paused[name] = paused
}

// SetStrandsError makes Strands fail with err until it is set to nil.
func (d *Delivery) SetStrandsError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.strandsErr = err
}

// SetIdleError makes Idle fail with err until it is set to nil.
func (d *Delivery) SetIdleError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleErr = err
}

// SetPausedError makes Paused fail with err until it is set to nil.
func (d *Delivery) SetPausedError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pausedErr = err
}

// SetSendError makes Send fail with err until it is set to nil.
func (d *Delivery) SetSendError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sendErr = err
}

// SetSendFunc makes Send answer with fn's result, called without the fake's lock held; nil restores the scripted error.
func (d *Delivery) SetSendFunc(fn func(strand, text string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sendFn = fn
}

// SetInterruptError makes Interrupt fail with err until it is set to nil.
func (d *Delivery) SetInterruptError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interruptErr = err
}

// Sends returns every call to Send, in order.
func (d *Delivery) Sends() []Send {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Send(nil), d.sends...)
}

// Interrupts returns the name of the strand of every call to Interrupt, in order.
func (d *Delivery) Interrupts() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.interrupts...)
}

// IdleCalls returns how many times Idle was asked about the named strand.
func (d *Delivery) IdleCalls(name string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.idleCalls[name]
}

// Strands implements creelengine.Delivery.
func (d *Delivery) Strands() ([]creelengine.Strand, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.strandsErr != nil {
		return nil, d.strandsErr
	}
	return append([]creelengine.Strand(nil), d.strands...), nil
}

// Idle implements creelengine.Delivery.
func (d *Delivery) Idle(strand creelengine.Strand) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleCalls[strand.Name]++
	if d.idleErr != nil {
		return false, d.idleErr
	}
	idle, scripted := d.idle[strand.Name]
	return idle || !scripted, nil
}

// Paused implements creelengine.Delivery.
func (d *Delivery) Paused(strand creelengine.Strand) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pausedErr != nil {
		return false, d.pausedErr
	}
	return d.paused[strand.Name], nil
}

// Send implements creelengine.Delivery.
func (d *Delivery) Send(strand creelengine.Strand, text string) error {
	d.mu.Lock()
	d.sends = append(d.sends, Send{Strand: strand.Name, Text: text})
	fn, err := d.sendFn, d.sendErr
	d.mu.Unlock()
	if fn != nil {
		return fn(strand.Name, text)
	}
	return err
}

// Interrupt implements creelengine.Delivery.
func (d *Delivery) Interrupt(strand creelengine.Strand) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interrupts = append(d.interrupts, strand.Name)
	return d.interruptErr
}

type timer struct {
	due time.Time
	ch  chan time.Time
}

// Clock is a creelengine.Clock that stands still until Advance moves it.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []timer
	waits   []time.Duration
	changed chan struct{}
}

// NewClock returns a Clock standing at start.
func NewClock(start time.Time) *Clock {
	return &Clock{now: start, changed: make(chan struct{})}
}

// Now implements creelengine.Clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After implements creelengine.Clock: it records d and returns a channel that receives once Advance has moved the clock d past now.
// A duration of zero or less receives at once.
func (c *Clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waits = append(c.waits, d)
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
	} else {
		c.timers = append(c.timers, timer{due: c.now.Add(d), ch: ch})
	}
	close(c.changed)
	c.changed = make(chan struct{})
	return ch
}

// Advance moves the clock forward by d and fires every timer that is now due, oldest due time first.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	sort.SliceStable(c.timers, func(i, j int) bool { return c.timers[i].due.Before(c.timers[j].due) })
	pending := c.timers[:0]
	for _, t := range c.timers {
		if t.due.After(c.now) {
			pending = append(pending, t)
			continue
		}
		t.ch <- c.now
	}
	c.timers = pending
}

// Waits returns every duration passed to After, in order.
func (c *Clock) Waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

// AwaitWaits blocks until at least n durations have been passed to After and returns all of them.
// It gives up after a real ten seconds and returns what it has, so a test that never reaches n fails on its own assertion instead of hanging.
func (c *Clock) AwaitWaits(n int) []time.Duration {
	return c.AwaitWaitsWhere(n, nil)
}

// AwaitWaitsWhere is AwaitWaits counting and returning only the durations keep accepts, in order; a nil keep accepts all.
// It tells apart the waits of loops that share the clock.
func (c *Clock) AwaitWaitsWhere(n int, keep func(time.Duration) bool) []time.Duration {
	deadline := time.After(10 * time.Second)
	for {
		c.mu.Lock()
		var kept []time.Duration
		for _, wait := range c.waits {
			if keep == nil || keep(wait) {
				kept = append(kept, wait)
			}
		}
		changed := c.changed
		c.mu.Unlock()
		if len(kept) >= n {
			return kept
		}
		select {
		case <-changed:
		case <-deadline:
			return kept
		}
	}
}
