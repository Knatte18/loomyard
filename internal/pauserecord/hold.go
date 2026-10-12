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
		coveredSlug := slug
		if w.SlugEntryExempt != nil && w.SlugEntryExempt() {
			coveredSlug = ""
		}
		entry, covered := record.Covering(coveredSlug)
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
	pause, resume := "a hub-wide pause", `"lyx resume"`
	if entry.Scope != ScopeHub {
		pause = fmt.Sprintf("the pause of %s", entry.Scope)
		resume += fmt.Sprintf(` or "lyx resume %s"`, entry.Scope)
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
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list network operations in %s: %w", dir, err)
	}
	var ops []NetOp
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		markerPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(markerPath)
		if err != nil {
			continue
		}
		var op NetOp
		if err := json.Unmarshal(data, &op); err != nil {
			continue
		}
		if !alive(op.PID, op.StartTime) {
			os.Remove(markerPath)
			continue
		}
		ops = append(ops, op)
	}
	return ops, nil
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
}

var (
	installedGate atomic.Pointer[Gate]
	netOpCounter  atomic.Int64
)

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

// Network waits while a pause covers the process, then marks an operation op in flight and returns the function that clears the mark.
// With no gate installed it returns a no-op at once.
// It only waits at entry: it fails no call, and a marker it cannot write is skipped.
func Network(op string) (done func()) {
	gate := installedGate.Load()
	if gate == nil {
		return func() {}
	}
	slug := gate.Geometry.Slug
	holdsParentLock := func() bool { return gate.HoldsParentLock != nil && gate.HoldsParentLock(slug) }
	waiter := Waiter{Notify: gate.Notify, Sleep: gate.Sleep, SlugEntryExempt: holdsParentLock}
	// The background context never ends, so the wait returns nil.
	_ = WaitUncovered(context.Background(), gate.Geometry.RecordPath, slug, waiter)

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
		return func() {}
	}
	return func() { os.Remove(markerPath) }
}

// writeMarker writes the marker to path, creating its directory.
func writeMarker(path string, marker NetOp) error {
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
