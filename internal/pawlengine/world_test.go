//go:build tier0

// world_test.go holds the fake beam every pawlengine test drives: told seams that log their calls in order, a fixed clock and a sleep that records its waits.

package pawlengine_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	testSlug  = "my-beam"
	testRunID = "run-1"
	// maxSleeps ends a scenario that never finishes on its own.
	maxSleeps = 50
)

// world is a fake beam: its entry status, its seams' failures, and the ordered record of what the producers did to it.
type world struct {
	mu    sync.Mutex
	paths pawlengine.Paths
	now   time.Time

	status string
	// statusFail returns the error the call-th write naming producer fails with, or nil.
	statusFail  func(producer string, call int) error
	statusCalls map[string]int

	owned      bool
	dobbyLive  bool
	dobbyReads int

	blockers []string

	recheckErr, stopErr, startErr, snapshotErr, notifyErr error
	markErr, pushErr, workspaceErr                        error

	events  []string
	notices []string
	sleeps  []time.Duration

	// onSleep runs after the n-th sleep is recorded, with the world unlocked.
	onSleep func(n int)
	// blockFirstSleep makes the first sleep wait for its context to end.
	blockFirstSleep bool
	cancel          context.CancelFunc
	watch           chan struct{}
}

// newWorld returns a beam whose entry status is Beam-Up, whose dobby runs and whose files sit under a temp directory.
func newWorld(t *testing.T) *world {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "beam")
	return &world{
		paths: pawlengine.Paths{
			LandRequest:     filepath.Join(dir, pawlengine.LandRequestFileName),
			DobbyStopMarker: filepath.Join(dir, pawlengine.DobbyStopMarkerName),
			AwaitMarker:     filepath.Join(dir, pawlengine.AwaitMarkerName),
		},
		now:         time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		status:      "Beam-Up",
		statusCalls: map[string]int{},
		dobbyLive:   true,
	}
}

func (w *world) record(event string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
}

// snapshotEvents returns a copy of the events recorded so far.
func (w *world) snapshotEvents() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.events...)
}

// snapshotNotices returns a copy of the notices posted so far.
func (w *world) snapshotNotices() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.notices...)
}

// snapshotSleeps returns a copy of the waits recorded so far.
func (w *world) snapshotSleeps() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Duration(nil), w.sleeps...)
}

// request writes a land request naming runID.
func (w *world) request(t *testing.T, runID string) {
	t.Helper()
	if err := pawlengine.WriteLandRequest(w.paths, pawlengine.LandRequest{RunID: runID, RequestedAt: w.now}); err != nil {
		t.Fatal(err)
	}
}

// touch writes an empty marker file at path.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// exists reports whether the file at path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

// deps returns the told values and seams over this world.
func (w *world) deps() pawlengine.Deps {
	return pawlengine.Deps{
		Slug:  testSlug,
		Paths: w.paths,
		RunID: testRunID,
		SetStatus: func(producer string) error {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.statusCalls[producer]++
			if w.statusFail != nil {
				if err := w.statusFail(producer, w.statusCalls[producer]); err != nil {
					w.events = append(w.events, "status:"+producer+"(failed)")
					return err
				}
			}
			w.status = producer
			w.events = append(w.events, "status:"+producer)
			return nil
		},
		EntryProducer: func() (string, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.status, nil
		},
		OwnedElsewhere: func() (bool, error) { return w.owned, nil },
		MarkBeam:       func() error { w.record("mark"); return w.markErr },
		SpawnPush:      func() error { w.record("push"); return w.pushErr },
		RegenerateWorkspace: func() error {
			w.record("workspace")
			return w.workspaceErr
		},
		StopDobby: func() error {
			w.record(fmt.Sprintf("stop marker=%t", exists(w.paths.DobbyStopMarker)))
			return w.stopErr
		},
		StartDobby: func() error { w.record("start"); return w.startErr },
		DobbyLive: func() (bool, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.dobbyReads++
			return w.dobbyLive, nil
		},
		Recheck:       func() ([]string, error) { w.record("recheck"); return w.blockers, w.recheckErr },
		WriteSnapshot: func() error { w.record("snapshot"); return w.snapshotErr },
		Notify: func(line string) error {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.notices = append(w.notices, line)
			return w.notifyErr
		},
		Down:  func() error { w.record("down"); return nil },
		Watch: watchOf(w.watch),
		Now:   func() time.Time { return w.now },
		Sleep: w.sleep,
	}
}

// watchOf returns ch as a receive-only channel, and a nil channel for a nil ch.
func watchOf(ch chan struct{}) <-chan struct{} {
	if ch == nil {
		return nil
	}
	return ch
}

// sleep records the wait, runs the scenario's hook and returns at once, bar a first sleep that blocks until its context ends.
func (w *world) sleep(ctx context.Context, d time.Duration) error {
	w.mu.Lock()
	w.sleeps = append(w.sleeps, d)
	n := len(w.sleeps)
	block := w.blockFirstSleep && n == 1
	w.mu.Unlock()

	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if w.onSleep != nil {
		w.onSleep(n)
	}
	if n >= maxSleeps && w.cancel != nil {
		w.cancel()
	}
	return ctx.Err()
}

// callAwait runs a Beam-Await row over the world's deps until it ends or the scenario's hook cancels the context.
func (w *world) callAwait(t *testing.T) (shedengine.Outcome, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.cancel = cancel
	producer, err := pawlengine.NewBeamAwait(pawlengine.NameBeamAwait, w.deps())
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, err := producer.Call(ctx)
	return outcome, err
}

// seedDownNotice writes the await marker as an earlier re-entry left it after posting the dobby-down notice.
func seedDownNotice(t *testing.T, w *world) {
	t.Helper()
	touch(t, w.paths.AwaitMarker)
	if err := os.WriteFile(w.paths.AwaitMarker, []byte("2026-01-01T00:00:00Z\ndobby-down-notified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
