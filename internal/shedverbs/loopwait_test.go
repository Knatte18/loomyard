// loopwait_test.go drives step's --until-stop waiter through stepCmd.
// The detached loop is the same stepCmd run in a goroutine with the fake runner of loop_test.go, started by a fake Spawn;
// a fake Watch emits nothing and a fake Sleep records the waiter's waits.
// No test in this file calls t.Parallel: the waiters and loops log through the process-global logger.

package shedverbs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// waitWorld is a loop fixture armed for the waiter: spawning starts the detached half in a goroutine, and waiting records its sleeps.
type waitWorld struct {
	*loopFixture
	mu     sync.Mutex
	sleeps []time.Duration
	// onSleep, when set, runs in the waiter at each wait with the 1-based number of the wait.
	onSleep func(call int)
	// spawned lists the loop ids the fake spawn started.
	spawned []string
	exits   sync.WaitGroup
}

func newWaitWorld(t *testing.T, scripts ...childScript) *waitWorld {
	t.Helper()
	w := &waitWorld{loopFixture: newLoopFixture(t, scripts...)}
	if err := os.MkdirAll(w.stepsDir, 0o755); err != nil {
		t.Fatalf("create the steps directory: %v", err)
	}
	w.spec.Loop.LockGrace = 20 * time.Millisecond
	w.spec.Loop.Spawn = w.spawnDetached
	w.spec.Loop.Sleep = w.sleep
	w.spec.Loop.Watch = func(string, ...string) (<-chan struct{}, func(), error) {
		return make(chan struct{}), func() {}, nil
	}
	t.Cleanup(w.exits.Wait)
	return w
}

// loopIDOf is the loop id a spawned loop's command line carries.
func loopIDOf(argv []string) string {
	for _, arg := range argv {
		if id, found := strings.CutPrefix(arg, "--"+LoopDetachedFlag+"="); found {
			return id
		}
	}
	return ""
}

// spawnDetached is the fake Spawn: it runs the detached half in a goroutine and closes the channel when that returns.
func (w *waitWorld) spawnDetached(argv []string) (<-chan struct{}, error) {
	w.mu.Lock()
	w.spawned = append(w.spawned, loopIDOf(argv))
	w.mu.Unlock()
	done := make(chan struct{})
	w.exits.Add(1)
	go func() {
		defer w.exits.Done()
		defer close(done)
		w.runDetached(argv)
	}()
	return done, nil
}

// runDetached runs the detached half of the loop for argv to its end.
func (w *waitWorld) runDetached(argv []string) {
	clihelp.Execute(stepCmd(stepTexts(), w.spec), io.Discard, argv)
}

// sleep is the fake Sleep: it records the wait and yields for a millisecond so the loops beside the waiter progress.
func (w *waitWorld) sleep(ctx context.Context, delay time.Duration) error {
	w.mu.Lock()
	w.sleeps = append(w.sleeps, delay)
	call := len(w.sleeps)
	w.mu.Unlock()
	if w.onSleep != nil {
		w.onSleep(call)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Millisecond):
		return nil
	}
}

func (w *waitWorld) recordedSleeps() []time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Duration(nil), w.sleeps...)
}

func (w *waitWorld) spawnedIDs() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.spawned...)
}

// waitFor runs `step --until-stop` over spec and decodes the envelope it printed; it is safe to call from a goroutine.
func waitFor(spec *Spec) (map[string]any, int, error) {
	var buf bytes.Buffer
	code := clihelp.Execute(stepCmd(stepTexts(), spec), &buf, []string{"--" + UntilStopFlag})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	var env map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &env); err != nil {
		return nil, code, errors.New("decode envelope from " + buf.String() + ": " + err.Error())
	}
	return env, code, nil
}

func (w *waitWorld) wait() (map[string]any, int) {
	w.t.Helper()
	env, code, err := waitFor(w.spec)
	if err != nil {
		w.t.Fatal(err)
	}
	return env, code
}

// envelopeAt is a halted loop's envelope describing st.
func envelopeAt(st shedengine.Status) []byte {
	var buf bytes.Buffer
	obj := loopObject{Stop: LoopStopHalted, CurrentProducer: st.CurrentProducer, HistoryLength: len(st.History), State: string(st.State)}
	output.Ok(&buf, loopEnvelope(map[string]any{"run_id": "run-1"}, obj))
	return buf.Bytes()
}

// liveLoop is a loop the test plays by hand: it holds the loop lock and has recorded itself in the pid file.
type liveLoop struct {
	w    *waitWorld
	id   string
	held *lock.FileLock
}

func (w *waitWorld) startLiveLoop(id string) *liveLoop {
	w.t.Helper()
	held, locked, err := lock.TryAcquireWriteLock(w.spec.Loop.LockPath)
	if err != nil || !locked {
		w.t.Fatalf("hold the loop lock = %v, locked %v; want held", err, locked)
	}
	l := &liveLoop{w: w, id: id, held: held}
	w.writePID(LoopPIDRecord{LoopID: id, Loop: proc.TreeRecord{PID: 1}})
	w.t.Cleanup(func() { _ = l.held.Release() })
	return l
}

func (w *waitWorld) writePID(rec LoopPIDRecord) {
	w.t.Helper()
	if err := WriteLoopPIDRecord(w.spec.Loop.PIDPath, rec); err != nil {
		w.t.Fatalf("write the pid file: %v", err)
	}
}

// finish ends the live loop the way a real one ends: envelope first, then the pid file, then the lock.
func (l *liveLoop) finish(envelope []byte) {
	l.w.t.Helper()
	if err := writeLoopEnvelopeFile(l.w.spec.Loop.EnvelopePath, l.id, envelope); err != nil {
		l.w.t.Fatalf("write the loop envelope: %v", err)
	}
	_ = os.Remove(l.w.spec.Loop.PIDPath)
	_ = l.held.Release()
}

// die ends the live loop without an envelope, leaving its pid file.
func (l *liveLoop) die() {
	_ = l.held.Release()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// assertNoFailedWrite checks the status file was not written failed.
func (w *waitWorld) assertNotFailed() {
	w.t.Helper()
	if st := w.readStatus(); st.State == shedengine.StateFailed {
		w.t.Errorf("status state = failed (%q); want it left alone", st.Error)
	}
}

func TestUntilStop_SecondWaiterAdoptsTheLiveLoop(t *testing.T) {
	t.Run("a second waiter spawns nothing and prints the loop's envelope", func(t *testing.T) {
		tests := []struct {
			name       string
			onSleep    func(w *waitWorld, live *liveLoop, call int)
			wantSleeps []time.Duration
		}{
			{
				name:       "waits double while nothing changes",
				onSleep:    func(w *waitWorld, live *liveLoop, call int) { finishAt(w, live, call, 3) },
				wantSleeps: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second},
			},
			{
				name: "wait returns to the floor on a change",
				onSleep: func(w *waitWorld, live *liveLoop, call int) {
					if call == 2 {
						w.writePID(LoopPIDRecord{LoopID: live.id, Loop: proc.TreeRecord{PID: 1}, Child: proc.TreeRecord{PID: 2}})
					}
					finishAt(w, live, call, 3)
				},
				wantSleeps: []time.Duration{time.Second, 2 * time.Second, time.Second},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := newWaitWorld(t)
				w.spec.Loop.Spawn = func([]string) (<-chan struct{}, error) {
					t.Error("the waiter spawned a loop beside a live one")
					return nil, errors.New("unexpected spawn")
				}
				live := w.startLiveLoop("live-1")
				w.onSleep = func(call int) { tt.onSleep(w, live, call) }

				env, code := w.wait()

				if code != 0 || loopOf(t, env)["state"] != "blocked" {
					t.Errorf("exit/loop = %d/%v; want the live loop's blocked envelope", code, env["loop"])
				}
				if got := w.recordedSleeps(); !reflect.DeepEqual(got, tt.wantSleeps) {
					t.Errorf("waits = %v; want %v", got, tt.wantSleeps)
				}
				if !fileExists(w.spec.Loop.Delivered("live-1")) || fileExists(w.spec.Loop.EnvelopePath) || fileExists(w.spec.Loop.PIDPath) {
					t.Errorf("after delivery the envelope should sit in the delivered record with no pid file left")
				}
				w.assertNotFailed()
			})
		}
	})

	t.Run("two waiters alive when the envelope lands both print it", func(t *testing.T) {
		w := newWaitWorld(t)
		live := w.startLiveLoop("live-1")
		w.setStatus("B", shedengine.StateBlocked, 1)
		var ready sync.WaitGroup
		ready.Add(2)
		var finishOnce sync.Once
		sleeper := func() func(context.Context, time.Duration) error {
			first := true
			return func(ctx context.Context, _ time.Duration) error {
				if first {
					first = false
					ready.Done()
					ready.Wait()
					finishOnce.Do(func() { live.finish(envelopeAt(w.readStatus())) })
				}
				time.Sleep(time.Millisecond)
				return ctx.Err()
			}
		}
		type outcome struct {
			env  map[string]any
			code int
			err  error
		}
		results := make(chan outcome, 2)
		for range 2 {
			spec := *w.spec
			spec.Loop.Sleep = sleeper()
			go func() {
				env, code, err := waitFor(&spec)
				results <- outcome{env, code, err}
			}()
		}

		first, second := <-results, <-results

		for _, got := range []outcome{first, second} {
			if got.err != nil || got.code != 0 || loopOf(t, got.env)["state"] != "blocked" {
				t.Errorf("waiter = %v, exit %d, %v; want the live loop's envelope", got.env, got.code, got.err)
			}
		}
		if !reflect.DeepEqual(first.env, second.env) {
			t.Errorf("waiters printed %v and %v; want the same envelope", first.env, second.env)
		}
		if spawned := w.spawnedIDs(); len(spawned) != 0 {
			t.Errorf("spawned loops = %v; want none beside a live loop", spawned)
		}
		w.assertNotFailed()
	})

	t.Run("a waiter that sleeps through its loop's delivery prints that loop's envelope", func(t *testing.T) {
		tests := []struct {
			name string
			// scripts are the children the loops run: the later loop's alone where the first loop is played by hand, else the first loop's then the later one's.
			scripts []childScript
			// arrange makes the world in which the first waiter's loop L1 has finished, its envelope has gone stale and a later waiter has run a second loop to its end, before the first waiter looks again.
			arrange func(w *waitWorld, secondWaiter *Spec)
		}{
			{
				name:    "it had read the pid file",
				scripts: []childScript{halting(shedengine.StateDone, "", 1)},
				arrange: func(w *waitWorld, secondWaiter *Spec) {
					live := w.startLiveLoop("live-1")
					w.onSleep = func(call int) {
						if call != 1 {
							return
						}
						w.setStatus("B", shedengine.StateBlocked, 1)
						live.finish(envelopeAt(w.readStatus()))
						w.setStatus("A", shedengine.StateRunning, 0)
						runSecondWaiter(t, secondWaiter)
					}
				},
			},
			{
				name:    "it never had",
				scripts: []childScript{halting(shedengine.StateBlocked, "", 1), halting(shedengine.StateDone, "", 1)},
				arrange: func(w *waitWorld, secondWaiter *Spec) {
					var once sync.Once
					w.spec.Loop.Spawn = func(argv []string) (<-chan struct{}, error) {
						done := make(chan struct{})
						once.Do(func() {
							w.mu.Lock()
							w.spawned = append(w.spawned, loopIDOf(argv))
							w.mu.Unlock()
							w.runDetached(argv)
							w.setStatus("A", shedengine.StateRunning, 0)
							runSecondWaiter(t, secondWaiter)
						})
						close(done)
						return done, nil
					}
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := newWaitWorld(t, tt.scripts...)
				second := *w.spec
				second.Loop.Sleep = func(ctx context.Context, _ time.Duration) error {
					time.Sleep(time.Millisecond)
					return ctx.Err()
				}
				tt.arrange(w, &second)

				env, code := w.wait()

				if code != 0 || loopOf(t, env)["state"] != "blocked" {
					t.Errorf("exit/loop = %d/%v; want the first loop's blocked envelope from its delivered record, not the later loop's", code, env["loop"])
				}
				if st := w.readStatus(); st.State != shedengine.StateDone {
					t.Errorf("status state = %s (%q); want the later loop's done, with no loop-exited write from the first waiter", st.State, st.Error)
				}
			})
		}
	})
}

// finishAt ends the live loop on the call-th wait with a halted envelope describing a blocked run.
func finishAt(w *waitWorld, live *liveLoop, call, want int) {
	if call != want {
		return
	}
	w.setStatus("B", shedengine.StateBlocked, 1)
	live.finish(envelopeAt(w.readStatus()))
}

// runSecondWaiter runs a later invocation to its end.
func runSecondWaiter(t *testing.T, spec *Spec) {
	t.Helper()
	if _, _, err := waitFor(spec); err != nil {
		t.Fatal(err)
	}
}

func TestUntilStop_EnvelopeCurrency(t *testing.T) {
	tests := []struct {
		name string
		// status is the run's status file when the invocation starts; the envelope describes a blocked run at B after one step.
		status       shedengine.State
		producer     string
		history      int
		wantDeliver  bool
		wantRetireTo string
	}{
		{name: "a current envelope is delivered, then retired with its leftover pid file", status: shedengine.StateBlocked, producer: "B", history: 1, wantDeliver: true},
		{name: "an envelope the run has moved past is retired and a fresh loop spawned", status: shedengine.StateBlocked, producer: "B", history: 2},
		{name: "a halt envelope is stale once the file reads running again", status: shedengine.StateRunning, producer: "B", history: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWaitWorld(t, halting(shedengine.StateDone, "", 3))
			stale := envelopeAt(shedengine.Status{CurrentProducer: "B", State: shedengine.StateBlocked, History: historyOf(1)})
			if err := writeLoopEnvelopeFile(w.spec.Loop.EnvelopePath, "old-1", stale); err != nil {
				t.Fatalf("write the envelope: %v", err)
			}
			w.writePID(LoopPIDRecord{LoopID: "old-1"})
			w.setStatus(tt.producer, tt.status, tt.history)

			env, code := w.wait()

			if tt.wantDeliver {
				if code != 0 || loopOf(t, env)["state"] != "blocked" || len(w.spawnedIDs()) != 0 {
					t.Errorf("exit/loop/spawned = %d/%v/%v; want the stored envelope delivered with no spawn", code, env["loop"], w.spawnedIDs())
				}
			} else if code != 0 || loopOf(t, env)["state"] != "done" || len(w.spawnedIDs()) != 1 {
				t.Errorf("exit/loop/spawned = %d/%v/%v; want a fresh loop's done envelope", code, env["loop"], w.spawnedIDs())
			}
			if retired, found := readLoopEnvelopeFile(w.spec.Loop.Delivered("old-1")); !found || !bytes.Equal(retired.Envelope, stale[:len(stale)-1]) {
				t.Errorf("delivered record of old-1 = %q, found %v; want the stored envelope", retired.Envelope, found)
			}
			if fileExists(w.spec.Loop.PIDPath) {
				t.Errorf("the leftover pid file of the finished loop was not retired")
			}
		})
	}
}

// deadLoopWorld is a world whose last loop died: the lock is free and its pid file stays.
// The recorded producer and history differ from the status file's, as after a step moved the run.
func deadLoopWorld(t *testing.T) *waitWorld {
	t.Helper()
	w := newWaitWorld(t)
	w.setStatus("B", shedengine.StateRunning, 1)
	w.writePID(LoopPIDRecord{LoopID: "dead-1", Child: proc.TreeRecord{PID: 4242}, CurrentProducer: "A", HistoryLength: 0})
	return w
}

// assertLoopExited checks env is the interrupted loop-exited stop and the status file records it.
func assertLoopExited(t *testing.T, w *waitWorld, env map[string]any, code int) {
	t.Helper()
	wantError := "shedverbs: step " + w.readStatus().CurrentProducer + " interrupted (loop-exited); way forward: read loop.trace_copy and loop.stderr_path, apply the interrupted rule under loop.interrupt_policy, then re-run lyx shed step run-1 --until-stop"
	loop := loopOf(t, env)
	if code != 1 || env["kind"] != KindInterrupted || env["error"] != wantError || loop["stop"] != string(LoopStopInterrupted) {
		t.Errorf("exit/kind/error/stop = %d/%v/%v/%v; want the interrupted loop-exited stop", code, env["kind"], env["error"], loop["stop"])
	}
	if st := w.readStatus(); st.State != shedengine.StateFailed || st.Error != wantError {
		t.Errorf("status state/error = %s/%q; want failed with the stop's text", st.State, st.Error)
	}
}

func TestUntilStop_DeadLoopStopsLoopExited(t *testing.T) {
	t.Run("a dead loop found at start is stopped, delivered and retired", func(t *testing.T) {
		w := deadLoopWorld(t)

		env, code := w.wait()

		assertLoopExited(t, w, env, code)
		loop := loopOf(t, env)
		if loop["status_moved"] != true || !strings.Contains(loop["detail"].(string), "dead-1 ended without an envelope; its output is in "+w.spec.Loop.LogPath) {
			t.Errorf("loop status_moved/detail = %v/%v; want moved, naming the loop log", loop["status_moved"], loop["detail"])
		}
		if fileExists(w.spec.Loop.PIDPath) || fileExists(w.spec.Loop.EnvelopePath) || !fileExists(w.spec.Loop.Delivered("dead-1")) {
			t.Errorf("want the pid file retired and the stop delivered under the dead loop's id")
		}
		if len(w.spawnedIDs()) != 0 {
			t.Errorf("spawned loops = %v; want none over a dead loop's pid file", w.spawnedIDs())
		}
	})

	t.Run("a loop that dies while the watch emits nothing is seen at the next tick", func(t *testing.T) {
		w := newWaitWorld(t)
		live := w.startLiveLoop("live-1")
		w.onSleep = func(call int) {
			if call == 2 {
				live.die()
			}
		}

		env, code := w.wait()

		assertLoopExited(t, w, env, code)
		if len(w.spawnedIDs()) != 0 {
			t.Errorf("spawned loops = %v; want none", w.spawnedIDs())
		}
	})

	t.Run("a held run lock makes the stop busy, keeps the pid file, and the next invocation retries", func(t *testing.T) {
		w := deadLoopWorld(t)
		held, locked, err := lock.TryAcquireWriteLock(w.paths.LockPath)
		if err != nil || !locked {
			t.Fatalf("hold the run lock = %v, locked %v; want held", err, locked)
		}

		env, code := w.wait()

		wantError := "shedverbs: the run lock is held, so the dead loop of run run-1 is not yet stopped; way forward: re-run lyx shed step run-1 --until-stop once the holder ends"
		if code != 1 || env["error"] != wantError || loopOf(t, env)["stop"] != string(LoopStopBusy) {
			t.Errorf("exit/error/stop = %d/%v/%v; want the busy stop", code, env["error"], loopOf(t, env)["stop"])
		}
		if !fileExists(w.spec.Loop.PIDPath) || fileExists(w.spec.Loop.EnvelopePath) {
			t.Errorf("want the pid file kept and no loop envelope written")
		}
		if st := w.readStatus(); st.State != shedengine.StateRunning {
			t.Errorf("status state = %s; want running, untouched", st.State)
		}

		if err := held.Release(); err != nil {
			t.Fatalf("release the run lock: %v", err)
		}
		env, code = w.wait()
		assertLoopExited(t, w, env, code)
	})

	t.Run("a detached run over a dead loop's pid file steps nothing and leaves the record", func(t *testing.T) {
		w := deadLoopWorld(t)

		var out bytes.Buffer
		code := clihelp.Execute(stepCmd(stepTexts(), w.spec), &out, detachedArgs("late-1"))

		if code == 0 || len(w.requests) != 0 || out.Len() != 0 {
			t.Errorf("exit/children/output = %d/%d/%q; want a refusal that steps nothing and prints no envelope", code, len(w.requests), out.String())
		}
		if !fileExists(w.spec.Loop.PIDPath) || fileExists(w.spec.Loop.EnvelopePath) {
			t.Errorf("want the dead loop's pid file left and no envelope written")
		}
		env, code := w.wait()
		assertLoopExited(t, w, env, code)
	})

	t.Run("a record with no child reports the status unmoved", func(t *testing.T) {
		w := newWaitWorld(t)
		w.setStatus("B", shedengine.StateRunning, 1)
		w.writePID(LoopPIDRecord{LoopID: "dead-1", CurrentProducer: "A", HistoryLength: 0})

		env, code := w.wait()

		assertLoopExited(t, w, env, code)
		if loopOf(t, env)["status_moved"] != false {
			t.Errorf("status_moved = %v; want false for a record naming no child", loopOf(t, env)["status_moved"])
		}
	})

	t.Run("the teardown's mark stops every invocation the same way and stays", func(t *testing.T) {
		for _, withEnvelope := range []bool{false, true} {
			w := newWaitWorld(t)
			w.writePID(LoopPIDRecord{LoopID: TeardownLoopID})
			if withEnvelope {
				if err := writeLoopEnvelopeFile(w.spec.Loop.EnvelopePath, "old-1", envelopeAt(w.readStatus())); err != nil {
					t.Fatalf("write the envelope: %v", err)
				}
			}

			var statusError string
			for invocation := 1; invocation <= 3; invocation++ {
				env, code := w.wait()

				assertLoopExited(t, w, env, code)
				if !strings.Contains(loopOf(t, env)["detail"].(string), "session end") {
					t.Errorf("invocation %d detail = %v; want the teardown named", invocation, loopOf(t, env)["detail"])
				}
				if invocation == 1 {
					statusError = w.readStatus().Error
				}
				if record, found, err := ReadLoopPIDRecord(w.spec.Loop.PIDPath); err != nil || !found || record.LoopID != TeardownLoopID {
					t.Errorf("invocation %d pid file = %+v, %v, %v; want the mark kept", invocation, record, found, err)
				}
				if got := w.readStatus().Error; got != statusError {
					t.Errorf("invocation %d status error = %q; want it written once", invocation, got)
				}
			}
			if len(w.spawnedIDs()) != 0 || fileExists(w.spec.Loop.Delivered(TeardownLoopID)) {
				t.Errorf("spawned %v loops, delivered record present %v; want none and no teardown envelope", w.spawnedIDs(), fileExists(w.spec.Loop.Delivered(TeardownLoopID)))
			}
			if _, found := readLoopEnvelopeFile(w.spec.Loop.EnvelopePath); found != withEnvelope {
				t.Errorf("envelope file present = %v; want %v, untouched", found, withEnvelope)
			}
		}
	})

	t.Run("a waiter that read a held lock with no pid file reports the mark once the lock is released", func(t *testing.T) {
		w := newWaitWorld(t)
		held, locked, err := lock.TryAcquireWriteLock(w.spec.Loop.LockPath)
		if err != nil || !locked {
			t.Fatalf("hold the loop lock = %v, locked %v; want held", err, locked)
		}
		t.Cleanup(func() { _ = held.Release() })
		w.onSleep = func(call int) {
			if call == 2 {
				w.writePID(LoopPIDRecord{LoopID: TeardownLoopID})
				_ = held.Release()
			}
		}

		env, code := w.wait()

		assertLoopExited(t, w, env, code)
		if len(w.spawnedIDs()) != 0 {
			t.Errorf("spawned loops = %v; want none beside the mark", w.spawnedIDs())
		}
	})

	t.Run("an arming refusal over a dead loop's pid file writes failed once and keeps the record", func(t *testing.T) {
		w := deadLoopWorld(t)
		stop := ArmStop{
			RunID:          "run-1",
			StatusPath:     w.paths.StatusPath,
			RunLockPath:    w.paths.LockPath,
			StatusLockPath: w.paths.StatusLockPath,
			LoopLockPath:   w.spec.Loop.LockPath,
			PIDPath:        w.spec.Loop.PIDPath,
			EnvelopePath:   w.spec.Loop.EnvelopePath,
		}

		ReportLoopArmError(io.Discard, stop, errors.New("config broken"))
		armed := w.readStatus()
		pidKept := fileExists(w.spec.Loop.PIDPath)
		env, code := w.wait()

		if armed.State != shedengine.StateFailed || !strings.Contains(armed.Error, "config broken") || !pidKept {
			t.Fatalf("after the refusal status = %s/%q, pid file kept %v; want failed naming the cause with the pid file kept", armed.State, armed.Error, pidKept)
		}
		loop := loopOf(t, env)
		if code != 1 || env["kind"] != KindInterrupted || loop["stop"] != string(LoopStopInterrupted) {
			t.Errorf("next invocation exit/kind/stop = %d/%v/%v; want the loop-exited stop", code, env["kind"], loop["stop"])
		}
		if got := w.readStatus(); got.Error != armed.Error {
			t.Errorf("status error = %q; want the refusal's text, with no second write", got.Error)
		}
	})

	t.Run("a spawned loop whose own pre-run refuses has its bootstrap envelope printed by its waiter", func(t *testing.T) {
		w := newWaitWorld(t)
		w.spec.Loop.Spawn = func(argv []string) (<-chan struct{}, error) {
			stop := ArmStop{
				RunID:          "run-1",
				StatusPath:     w.paths.StatusPath,
				RunLockPath:    w.paths.LockPath,
				StatusLockPath: w.paths.StatusLockPath,
				LoopLockPath:   w.spec.Loop.LockPath,
				PIDPath:        w.spec.Loop.PIDPath,
				EnvelopePath:   w.spec.Loop.EnvelopePath,
				LoopID:         loopIDOf(argv),
			}
			var printed bytes.Buffer
			ReportLoopArmError(&printed, stop, errors.New("config broken"))
			if printed.Len() != 0 {
				t.Errorf("the loop's pre-run printed %q; want its envelope in the loop envelope file instead", printed.String())
			}
			done := make(chan struct{})
			close(done)
			return done, nil
		}

		env, code := w.wait()

		if code != 1 || env["kind"] != KindBootstrap || !strings.Contains(env["error"].(string), "config broken") || loopOf(t, env)["stop"] != string(LoopStopError) {
			t.Errorf("exit/kind/error/stop = %d/%v/%v/%v; want the bootstrap error stop", code, env["kind"], env["error"], loopOf(t, env)["stop"])
		}
		if st := w.readStatus(); st.State != shedengine.StateFailed {
			t.Errorf("status state = %s; want failed", st.State)
		}
	})

	t.Run("a spawned loop that exits before any pid file names it is reported with its log", func(t *testing.T) {
		w := newWaitWorld(t)
		w.spec.Loop.Spawn = func([]string) (<-chan struct{}, error) {
			done := make(chan struct{})
			close(done)
			return done, nil
		}
		w.setStatus("B", shedengine.StateRunning, 1)

		env, code := w.wait()

		assertLoopExited(t, w, env, code)
		if detail := loopOf(t, env)["detail"].(string); !strings.Contains(detail, w.spec.Loop.LogPath) {
			t.Errorf("detail = %q; want it naming the loop log %s", detail, w.spec.Loop.LogPath)
		}
		if fileExists(w.spec.Loop.EnvelopePath) || fileExists(w.spec.Loop.PIDPath) {
			t.Errorf("want no loop envelope and no pid file written for a loop that never recorded itself")
		}
	})
}

func TestUntilStop_LoserWaitsOnWinner(t *testing.T) {
	t.Run("two loops spawned at once step once and the loser's waiter writes no failed", func(t *testing.T) {
		release := make(chan struct{})
		blocked := func(fx *loopFixture, traceID string, req ChildRequest) error {
			<-release
			return halting(shedengine.StateDone, "", 1)(fx, traceID, req)
		}
		w := newWaitWorld(t, blocked)
		w.spec.Loop.Spawn = func(argv []string) (<-chan struct{}, error) {
			rival := make(chan struct{})
			w.exits.Add(1)
			go func() {
				defer w.exits.Done()
				defer close(rival)
				w.runDetached(detachedArgs("rival-1"))
			}()
			for !fileExists(w.spec.Loop.PIDPath) {
				time.Sleep(time.Millisecond)
			}
			w.mu.Lock()
			w.spawned = append(w.spawned, loopIDOf(argv))
			w.mu.Unlock()
			loser := make(chan struct{})
			w.exits.Add(1)
			go func() {
				defer w.exits.Done()
				defer close(loser)
				w.runDetached(argv)
			}()
			return loser, nil
		}
		w.onSleep = func(call int) {
			if call == 2 {
				close(release)
			}
		}

		env, code := w.wait()

		if code != 0 || loopOf(t, env)["state"] != "done" {
			t.Errorf("exit/loop = %d/%v; want the lock holder's done envelope", code, env["loop"])
		}
		if len(w.requests) != 1 {
			t.Errorf("children started = %d; want only the lock holder to step", len(w.requests))
		}
		w.assertNotFailed()
	})

	t.Run("a waiter that polls before its loop takes the lock keeps waiting", func(t *testing.T) {
		w := newWaitWorld(t, halting(shedengine.StateDone, "", 1))
		started := make(chan struct{})
		w.spec.Loop.Spawn = func(argv []string) (<-chan struct{}, error) {
			w.mu.Lock()
			w.spawned = append(w.spawned, loopIDOf(argv))
			w.mu.Unlock()
			return started, nil
		}
		w.onSleep = func(call int) {
			if call == 2 {
				w.exits.Add(1)
				go func() {
					defer w.exits.Done()
					defer close(started)
					w.runDetached(detachedArgs(w.spawnedIDs()[0]))
				}()
			}
		}

		env, code := w.wait()

		if code != 0 || loopOf(t, env)["state"] != "done" || len(w.recordedSleeps()) < 2 {
			t.Errorf("exit/loop/waits = %d/%v/%d; want the loop's envelope after waiting through its start", code, env["loop"], len(w.recordedSleeps()))
		}
		w.assertNotFailed()
	})

	t.Run("an instant liveness probe with no loop behind it is waited out and a loop spawned", func(t *testing.T) {
		w := newWaitWorld(t, halting(shedengine.StateDone, "", 1))
		probe, locked, err := lock.TryAcquireWriteLock(w.spec.Loop.LockPath)
		if err != nil || !locked {
			t.Fatalf("hold the loop lock = %v, locked %v; want held", err, locked)
		}
		t.Cleanup(func() { _ = probe.Release() })
		w.onSleep = func(call int) {
			if call == 1 {
				_ = probe.Release()
			}
		}

		env, code := w.wait()

		if code != 0 || loopOf(t, env)["state"] != "done" || len(w.spawnedIDs()) != 1 {
			t.Errorf("exit/loop/spawned = %d/%v/%v; want one loop spawned once the lock read free", code, env["loop"], w.spawnedIDs())
		}
		w.assertNotFailed()
	})

	t.Run("a loop that cannot be started is refused with its way forward", func(t *testing.T) {
		w := newWaitWorld(t)
		w.spec.Loop.Spawn = func([]string) (<-chan struct{}, error) { return nil, errors.New("shedverbs: start the loop: no such file") }

		env, code := w.wait()

		if code != 1 || !strings.Contains(env["error"].(string), "no such file; way forward: re-run lyx shed step run-1 --until-stop") {
			t.Errorf("exit/error = %d/%v; want the start error with its way forward", code, env["error"])
		}
		w.assertNotFailed()
	})
}
