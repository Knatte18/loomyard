// hold.go holds callers while a pause covers them: the shared wait at entry, and the process gate that holds network calls and marks the ones in flight.

package pauserecord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// holdPollFloor is the first wake delay of a hold, and the delay it returns to when the covering entry changes.
	holdPollFloor = time.Second
	// holdPollCeiling is the longest wake delay of a hold while the covering entry is unchanged.
	holdPollCeiling = 5 * time.Second
	// netDirName is the directory beside the record that holds the in-flight network operation markers.
	netDirName = "net"
	// waitingDirName is the directory beside the record that holds the markers of calls waiting at the gate.
	waitingDirName = "waiting"
)

// Waiter carries the seams of WaitUncovered; its zero value waits on real timers and writes to stderr.
type Waiter struct {
	// Notify receives the one line a wait writes when it starts; nil writes the line to stderr.
	Notify func(line string)
	// Sleep waits d or until ctx ends; nil waits on a timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// SlugEntryExempt, while it reports true, lets a slug entry pass so that only a hub entry holds the wait.
	// It serves a caller whose run holds a parent lock.
	SlugEntryExempt func() bool
	// ExemptEntry, when it accepts a live entry, lets that entry pass whatever it covers; any entry it rejects holds the wait as usual.
	// It serves a caller that owns one pause entry and must still reach origin while it stands.
	ExemptEntry func(Entry) bool
}

// covering returns the oldest live entry of record that holds slug once the waiter's exemptions are applied.
func (w Waiter) covering(record Record, slug string) (Entry, bool) {
	coveredSlug := slug
	if w.SlugEntryExempt != nil && w.SlugEntryExempt() {
		coveredSlug = ""
	}
	if w.ExemptEntry != nil {
		var holding []Entry
		for _, live := range record.Live {
			if !w.ExemptEntry(live) {
				holding = append(holding, live)
			}
		}
		record.Live = holding
	}
	return record.Covering(coveredSlug)
}

// WaitUncovered returns once no live entry in the record at path covers slug, and at once when none does.
// It writes one line through the waiter when it starts waiting, polls with a delay doubling from one second to five while the covering entry is unchanged, and has no deadline of its own: it returns the context's error when ctx ends.
// An unreadable record holds nothing: the wait writes one line naming the path and the error, then returns nil.
func WaitUncovered(ctx context.Context, path, slug string, w Waiter) error {
	notify := w.Notify
	if notify == nil {
		notify = func(line string) { fmt.Fprintln(os.Stderr, line) }
	}
	sleep := w.Sleep
	if sleep == nil {
		sleep = sleepOnTimer
	}

	delay := holdPollFloor
	var held Entry
	waiting := false
	for {
		record, err := Read(path)
		if err != nil {
			notify(fmt.Sprintf("lyx: pause record unreadable, not holding: %v", err))
			return nil
		}
		entry, covered := w.covering(record, slug)
		if !covered {
			return nil
		}
		switch {
		case !waiting:
			notify(heldLine(entry))
			waiting = true
		case entry.Scope != held.Scope || !entry.PausedAt.Equal(held.PausedAt):
			delay = holdPollFloor
		default:
			delay = min(delay*2, holdPollCeiling)
		}
		held = entry
		if err := sleep(ctx, delay); err != nil {
			return err
		}
	}
}

// heldLine renders the line a wait writes when it starts, naming the pause and the resume that lifts it.
func heldLine(entry Entry) string {
	pause, resume := "a hub-wide pause", `"lyx tender resume"`
	if entry.Scope != ScopeHub {
		pause = fmt.Sprintf("the pause of %s", entry.Scope)
		resume += fmt.Sprintf(` or "lyx tender resume %s"`, entry.Scope)
	}
	return fmt.Sprintf("lyx: held by %s since %s; it continues after %s", pause, entry.PausedAt.Format(time.RFC3339), resume)
}

// sleepOnTimer waits d on a timer, or until ctx ends.
func sleepOnTimer(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NetOp is the marker of one network operation in flight.
type NetOp struct {
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	Slug      string `json:"slug"`
	Strand    string `json:"strand"`
	Op        string `json:"op"`
	// UnderParentLock reports whether the process held its slug's parent lock when the operation began.
	UnderParentLock bool      `json:"under_parent_lock"`
	StartedAt       time.Time `json:"started_at"`
}

// NetDir returns the directory of network operation markers beside the record.
func NetDir(recordPath string) string {
	return filepath.Join(filepath.Dir(recordPath), netDirName)
}

// ListNetOps lists the operation markers beside the record.
// A marker whose process alive reports dead is removed and skipped, so the result holds only operations in flight.
func ListNetOps(recordPath string, alive func(pid int, startTime string) bool) ([]NetOp, error) {
	dir := NetDir(recordPath)
	ops, err := listLiveMarkers(dir, alive, func(op NetOp) (int, string) { return op.PID, op.StartTime })
	if err != nil {
		return nil, fmt.Errorf("list network operations in %s: %w", dir, err)
	}
	return ops, nil
}

// WaitingMarker is the marker of one network call that is waiting at the gate for a pause to lift.
// It records only what the gate waits on: the process and the slug the wait is judged by.
type WaitingMarker struct {
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	Slug      string `json:"slug"`
}

// WaitingDir returns the directory of waiting markers beside the record, a sibling of NetDir and never inside it, so the in-flight reading never lists a waiting marker.
func WaitingDir(recordPath string) string {
	return filepath.Join(filepath.Dir(recordPath), waitingDirName)
}

// ListWaiting lists the waiting markers beside the record.
// A marker whose process alive reports dead is removed and skipped, so the result holds only calls waiting now.
func ListWaiting(recordPath string, alive func(pid int, startTime string) bool) ([]WaitingMarker, error) {
	dir := WaitingDir(recordPath)
	markers, err := listLiveMarkers(dir, alive, func(marker WaitingMarker) (int, string) { return marker.PID, marker.StartTime })
	if err != nil {
		return nil, fmt.Errorf("list waiting network calls in %s: %w", dir, err)
	}
	return markers, nil
}

// listLiveMarkers decodes the marker files in dir into T, removing and skipping those whose process identity reports dead and skipping files that do not decode.
// A missing directory lists nothing.
func listLiveMarkers[T any](dir string, alive func(pid int, startTime string) bool, identity func(T) (int, string)) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var markers []T
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		markerPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(markerPath)
		if err != nil {
			continue
		}
		var marker T
		if err := json.Unmarshal(data, &marker); err != nil {
			continue
		}
		if pid, startTime := identity(marker); !alive(pid, startTime) {
			os.Remove(markerPath)
			continue
		}
		markers = append(markers, marker)
	}
	return markers, nil
}

// Gate is the process-wide hold that network calls go through.
type Gate struct {
	// Geometry names the record and the process's pair slug.
	Geometry Geometry
	// Strand is the process's strand name, empty outside a strand.
	Strand string
	// PID and StartTime identify the process, so a marker of a dead process is told from a live one.
	PID       int
	StartTime string
	// HoldsParentLock reports whether the process holds slug's parent lock; nil means it never does.
	HoldsParentLock func(slug string) bool
	// Notify and Sleep are the Waiter seams of the waits the gate makes.
	Notify func(line string)
	Sleep  func(ctx context.Context, d time.Duration) error
	// Now is the clock Network measures the time it held a call on; nil means time.Now.
	Now func() time.Time
	// ExemptScope and ExemptPausedAt name the one live entry the gate lets pass; both zero means no entry passes.
	ExemptScope    string
	ExemptPausedAt time.Time
	// ExemptAll lets every live entry pass, for the one process an operator's explicit verb runs in.
	ExemptAll bool
	// Defer makes a call that finds a live entry covering the process hand off to the deferral SetGateDeferral stored, before any wait.
	Defer bool
	// OwnerEntryExempt lets a live entry of the gate's own slug whose Caller is OwnerCaller pass.
	OwnerEntryExempt bool
}

var (
	installedGate  atomic.Pointer[Gate]
	gateDeferral   atomic.Pointer[func()]
	netOpCounter   atomic.Int64
	waitingCounter atomic.Int64
)

// SetGateDeferral stores the function a deferring gate calls when a live entry covers its process, and returns the function that restores the previous one.
// The function is a process's exit and does not return; when none is stored, or it returns, the gate waits as it does without Defer.
func SetGateDeferral(onCovered func()) (restore func()) {
	var stored *func()
	if onCovered != nil {
		stored = &onCovered
	}
	previous := gateDeferral.Swap(stored)
	return func() { gateDeferral.Store(previous) }
}

// GateCovered reports whether a live entry covers the installed gate's process right now, under the exemptions the gate's waits apply.
// It reports false with no gate installed or an unreadable record.
func GateCovered() bool {
	gate := installedGate.Load()
	if gate == nil {
		return false
	}
	record, err := Read(gate.Geometry.RecordPath)
	if err != nil {
		return false
	}
	_, covered := gate.waiter(nil).covering(record, gate.Geometry.Slug)
	return covered
}

// waiter returns the Waiter that applies this gate's exemptions; onSleep, when non-nil, runs before every sleep of a wait.
func (g *Gate) waiter(onSleep func()) Waiter {
	slug := g.Geometry.Slug
	sleep := g.Sleep
	if sleep == nil {
		sleep = sleepOnTimer
	}
	waiter := Waiter{
		Notify: g.Notify,
		Sleep: func(ctx context.Context, d time.Duration) error {
			if onSleep != nil {
				onSleep()
			}
			return sleep(ctx, d)
		},
		SlugEntryExempt: func() bool { return g.HoldsParentLock != nil && g.HoldsParentLock(slug) },
	}
	switch {
	case g.ExemptAll:
		waiter.ExemptEntry = func(Entry) bool { return true }
	case g.ExemptScope != "" || g.OwnerEntryExempt:
		exemptScope, exemptPausedAt := g.ExemptScope, g.ExemptPausedAt
		ownerEntryExempt := g.OwnerEntryExempt
		waiter.ExemptEntry = func(entry Entry) bool {
			if ownerEntryExempt && entry.Scope == slug && entry.Caller == OwnerCaller {
				return true
			}
			return exemptScope != "" && entry.Scope == exemptScope && entry.PausedAt.Equal(exemptPausedAt)
		}
	}
	return waiter
}

// InstallGate stores g as the process's network gate and returns a function that reinstates the previous one.
func InstallGate(g Gate) (restore func()) {
	previous := installedGate.Swap(&g)
	return func() { installedGate.Store(previous) }
}

// SetGateSlug replaces the installed gate's slug, for a process that runs in the prime on behalf of one pair.
// It does nothing when no gate is installed.
func SetGateSlug(slug string) {
	for {
		current := installedGate.Load()
		if current == nil {
			return
		}
		updated := *current
		updated.Geometry.Slug = slug
		if installedGate.CompareAndSwap(current, &updated) {
			return
		}
	}
}

// SetGateExempt replaces the installed gate's exempt entry with the one of scope paused at pausedAt, for a process that owns that entry and must still reach origin while it stands.
// It does nothing when no gate is installed, and returns the function that restores the previous exempt entry.
func SetGateExempt(scope string, pausedAt time.Time) (restore func()) {
	previous := replaceGateExempt(scope, pausedAt)
	return func() { replaceGateExempt(previous.scope, previous.pausedAt) }
}

// SetGateExemptAll makes the installed gate let every live entry pass, for the one process an explicit operator verb runs in, and returns the function that restores the previous setting.
// It does nothing when no gate is installed.
// The exemption stays in this process: a child installs its own gate.
func SetGateExemptAll() (restore func()) {
	previous := replaceGateExemptAll(true)
	return func() { replaceGateExemptAll(previous) }
}

// replaceGateExemptAll sets ExemptAll on the current gate and returns the value it replaced; it replaces nothing when no gate is installed.
func replaceGateExemptAll(exemptAll bool) bool {
	for {
		current := installedGate.Load()
		if current == nil {
			return false
		}
		updated := *current
		updated.ExemptAll = exemptAll
		if installedGate.CompareAndSwap(current, &updated) {
			return current.ExemptAll
		}
	}
}

// gateExempt is the exempt entry a gate carries.
type gateExempt struct {
	scope    string
	pausedAt time.Time
}

// replaceGateExempt installs the exempt entry on the current gate and returns the one it replaced; it replaces nothing when no gate is installed.
func replaceGateExempt(scope string, pausedAt time.Time) gateExempt {
	for {
		current := installedGate.Load()
		if current == nil {
			return gateExempt{}
		}
		updated := *current
		updated.ExemptScope = scope
		updated.ExemptPausedAt = pausedAt
		if installedGate.CompareAndSwap(current, &updated) {
			return gateExempt{scope: current.ExemptScope, pausedAt: current.ExemptPausedAt}
		}
	}
}

// Network waits while a pause covers the process, then marks an operation op in flight and returns the function that clears the mark.
// While it waits it keeps a waiting marker under WaitingDir, removed as soon as the wait returns and before the in-flight marker is written.
// It returns held, the time from entry until the wait returned, which is zero unless the wait slept at least once.
// With no gate installed it returns zero and a no-op at once.
// It only waits at entry: it fails no call, and a marker it cannot write is skipped.
func Network(op string) (held time.Duration, done func()) {
	gate := installedGate.Load()
	if gate == nil {
		return 0, func() {}
	}
	now := gate.Now
	if now == nil {
		now = time.Now
	}
	slug := gate.Geometry.Slug
	holdsParentLock := func() bool { return gate.HoldsParentLock != nil && gate.HoldsParentLock(slug) }
	slept := false
	waiter := gate.waiter(func() { slept = true })
	if gate.Defer && GateCovered() {
		if deferral := gateDeferral.Load(); deferral != nil {
			(*deferral)()
		}
	}
	entered := now()
	waiting := WaitingMarker{PID: gate.PID, StartTime: gate.StartTime, Slug: slug}
	waitingPath := filepath.Join(WaitingDir(gate.Geometry.RecordPath), fmt.Sprintf("%d-%d.json", gate.PID, waitingCounter.Add(1)))
	if err := writeMarker(waitingPath, waiting); err != nil {
		waitingPath = ""
	}
	// The background context never ends, so the wait returns nil.
	_ = WaitUncovered(context.Background(), gate.Geometry.RecordPath, slug, waiter)
	if waitingPath != "" {
		os.Remove(waitingPath)
	}
	if slept {
		held = now().Sub(entered)
	}

	marker := NetOp{
		PID:             gate.PID,
		StartTime:       gate.StartTime,
		Slug:            slug,
		Strand:          gate.Strand,
		Op:              op,
		UnderParentLock: holdsParentLock(),
		StartedAt:       time.Now(),
	}
	markerPath := filepath.Join(NetDir(gate.Geometry.RecordPath), fmt.Sprintf("%d-%d.json", gate.PID, netOpCounter.Add(1)))
	if err := writeMarker(markerPath, marker); err != nil {
		return held, func() {}
	}
	return held, func() { os.Remove(markerPath) }
}

// writeMarker writes the marker to path, creating its directory.
func writeMarker(path string, marker any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return writeAtomically(path, data)
}

// writeAtomically writes data to a temp file beside path and renames it over path.
func writeAtomically(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
