// open_test.go drives the open sequence over real lock and state files, with every outside step faked.

package boardcli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	oldPID = 41
	newPID = 42
)

// openWorld is one fake world around runOpen, and what was asked of it.
type openWorld struct {
	deps    openDeps
	spawned []string
	opened  []string
}

// newOpenWorld has no server, a prime that resolves and an opener that exits well.
// The processes oldPID and newPID run, and only the token "live" answers.
func newOpenWorld(t *testing.T) *openWorld {
	t.Helper()
	w := &openWorld{}
	paths := boardserve.Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	w.deps = openDeps{
		Paths: paths,
		Prime: func() (string, error) { return "/prime", nil },
		Spawn: func(dir, logPath string) error {
			w.spawned = append(w.spawned, dir+" "+logPath)
			return nil
		},
		Probe: func(st boardserve.State) error {
			if st.Token != "live" {
				return errors.New("no answer")
			}
			return nil
		},
		Running: func(pid int) bool { return pid == oldPID || pid == newPID },
		StartOpener: func(path string) (<-chan error, error) {
			w.opened = append(w.opened, path)
			exited := make(chan error, 1)
			exited <- nil
			return exited, nil
		},
		StartBound: time.Minute,
		Poll:       time.Millisecond,
	}
	return w
}

// hold takes the server's lock, as a running server does.
func (w *openWorld) hold(t *testing.T) {
	t.Helper()
	held, err := lock.AcquireWriteLock(w.deps.Paths.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

func (w *openWorld) record(t *testing.T, st boardserve.State) {
	t.Helper()
	if err := state.WriteJSON(w.deps.Paths.State, w.deps.Paths.State+".lock", st); err != nil {
		t.Error(err)
	}
}

// wantOpened asserts a success that opened the launcher for the server on port.
func (w *openWorld) wantOpened(t *testing.T, port int, started bool) {
	t.Helper()
	got, err := runOpen(w.deps)
	want := openResult{Address: fmt.Sprintf("http://127.0.0.1:%d/", port), Launcher: w.deps.Paths.Launcher, Started: started}
	if err != nil || got != want {
		t.Errorf("runOpen() = %+v, %v; want %+v", got, err, want)
	}
	if !slices.Equal(w.opened, []string{w.deps.Paths.Launcher}) {
		t.Errorf("opened = %q; want the launcher", w.opened)
	}
}

// wantRefused asserts the refusal message and that nothing was opened.
func (w *openWorld) wantRefused(t *testing.T, message string) {
	t.Helper()
	if _, err := runOpen(w.deps); err == nil || err.Error() != message {
		t.Errorf("runOpen() error = %v; want %q", err, message)
	}
	if len(w.opened) != 0 {
		t.Errorf("opened = %q; want nothing", w.opened)
	}
}

// unanswered is the refusal of a server that never answered within the world's bound.
func (w *openWorld) unanswered() string {
	return fmt.Sprintf("the board server did not answer within %s; its log %s opens with its pid: way forward: end that process if it hangs, then run lym board open again",
		w.deps.StartBound, w.deps.Paths.Log)
}

func TestRunOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("LiveServer", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.hold(t)
		w.record(t, boardserve.State{PID: oldPID, Port: 5000, Token: "live"})
		w.wantOpened(t, 5000, false)
		if len(w.spawned) != 0 {
			t.Errorf("spawned = %q; want no spawn", w.spawned)
		}
	})

	t.Run("TheAddressHoldsNoToken", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.hold(t)
		w.record(t, boardserve.State{PID: oldPID, Port: 5000, Token: "live"})
		got, err := runOpen(w.deps)
		if err != nil || strings.Contains(got.Address, "live") || strings.Contains(strings.Join(w.opened, " "), "#") {
			t.Errorf("runOpen() = %+v, %v, opened %q; want no token", got, err, w.opened)
		}
	})

	t.Run("StateWrittenMidWait", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.hold(t)
		const gonePID = 7
		w.record(t, boardserve.State{PID: gonePID, Port: 4000, Token: "live"})
		w.deps.Running = func(pid int) bool {
			if pid == gonePID {
				w.record(t, boardserve.State{PID: newPID, Port: 5001, Token: "live"})
				return false
			}
			return true
		}
		w.wantOpened(t, 5001, false)
	})

	t.Run("NoServerAnswers", func(t *testing.T) {
		t.Parallel()
		for name, recorded := range map[string]*boardserve.State{
			"NoState":           nil,
			"ProcessNotRunning": {PID: 7, Port: 4000, Token: "live"},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				w := newOpenWorld(t)
				w.deps.StartBound = 10 * time.Millisecond
				w.hold(t)
				if recorded != nil {
					w.record(t, *recorded)
				}
				w.wantRefused(t, w.unanswered())
				if len(w.spawned) != 0 {
					t.Errorf("spawned = %q; want no spawn", w.spawned)
				}
			})
		}
	})

	t.Run("FreeLockSpawns", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.record(t, boardserve.State{PID: oldPID, Port: 4000, Token: "old"})
		spawn := w.deps.Spawn
		w.deps.Spawn = func(dir, logPath string) error {
			w.hold(t)
			w.record(t, boardserve.State{PID: newPID, Port: 5001, Token: "live"})
			return spawn(dir, logPath)
		}
		w.wantOpened(t, 5001, true)
		if want := []string{"/prime " + w.deps.Paths.Log}; !slices.Equal(w.spawned, want) {
			t.Errorf("spawned = %q; want %q", w.spawned, want)
		}
	})

	t.Run("SpawnedServerNeverAnswers", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.deps.StartBound = 10 * time.Millisecond
		// The leftover state would answer, but no process holds the lock.
		w.record(t, boardserve.State{PID: oldPID, Port: 4000, Token: "live"})
		w.wantRefused(t, w.unanswered())
		if len(w.spawned) != 1 {
			t.Errorf("spawned = %q; want one spawn", w.spawned)
		}
	})

	t.Run("PrimeFails", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		failure := errors.New("no prime")
		w.deps.Prime = func() (string, error) { return "", failure }
		if _, err := runOpen(w.deps); err != failure || len(w.spawned) != 0 {
			t.Errorf("runOpen() error = %v, spawned %q; want the prime's error and no spawn", err, w.spawned)
		}
	})

	t.Run("SpawnFails", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.deps.Spawn = func(string, string) error { return errors.New("boom") }
		w.wantRefused(t, "start the board server: boom: way forward: check that lym runs from this shell and read the server log "+
			w.deps.Paths.Log+", then run lym board open again")
	})

	t.Run("OpenerFails", func(t *testing.T) {
		t.Parallel()
		for name, opener := range map[string]func(string) (<-chan error, error){
			"AtStart": func(string) (<-chan error, error) { return nil, errors.New("boom") },
			"AtExit": func(string) (<-chan error, error) {
				exited := make(chan error, 1)
				exited <- errors.New("boom")
				return exited, nil
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				w := newOpenWorld(t)
				w.hold(t)
				w.record(t, boardserve.State{PID: oldPID, Port: 5000, Token: "live"})
				w.deps.StartOpener = opener
				w.wantRefused(t, "the opener "+openerName+" failed: boom: way forward: open "+w.deps.Paths.Launcher+" in a browser by hand")
			})
		}
	})

	t.Run("CappedOpener", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.hold(t)
		w.record(t, boardserve.State{PID: oldPID, Port: 5000, Token: "live"})
		w.deps.OpenerCap = time.Millisecond
		w.deps.StartOpener = func(string) (<-chan error, error) { return make(chan error), nil }
		if _, err := runOpen(w.deps); err != nil {
			t.Errorf("runOpen() error = %v; want success at the cap", err)
		}
	})

	t.Run("UncappedOpener", func(t *testing.T) {
		t.Parallel()
		w := newOpenWorld(t)
		w.hold(t)
		w.record(t, boardserve.State{PID: oldPID, Port: 5000, Token: "live"})
		exit, started := make(chan error), make(chan struct{})
		w.deps.StartOpener = func(string) (<-chan error, error) {
			close(started)
			return exit, nil
		}
		returned := make(chan error, 1)
		go func() {
			_, err := runOpen(w.deps)
			returned <- err
		}()
		<-started
		// The send is taken only by a runOpen still waiting on the opener.
		select {
		case exit <- nil:
		case err := <-returned:
			t.Fatalf("runOpen() returned before the opener exited: %v", err)
		}
		if err := <-returned; err != nil {
			t.Errorf("runOpen() error = %v", err)
		}
	})
}
