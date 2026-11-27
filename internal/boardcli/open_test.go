// open_test.go drives runOpen over real lock and state files, with the server, the opener and the process table faked.

package boardcli

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testPrimeAnchor = "/hub/prime"
	answeringToken  = "answering-token"
	deadPID         = 99
)

// openHarness is a hub's server files with a faked world around them.
// Only a server whose state carries answeringToken answers a probe.
type openHarness struct {
	t       *testing.T
	deps    openDeps
	spawned []string
	opened  []string
}

func newOpenHarness(t *testing.T) *openHarness {
	t.Helper()
	h := &openHarness{t: t}
	h.deps = openDeps{
		Paths: boardserve.Paths(t.TempDir()),
		Prime: func() (string, error) { return testPrimeAnchor, nil },
		Spawn: func(primeAnchor, _ string) error {
			h.spawned = append(h.spawned, primeAnchor)
			h.holdLock()
			return nil
		},
		Probe: func(server boardserve.State) error {
			if server.Token != answeringToken {
				return errors.New("no answer")
			}
			return nil
		},
		Running: func(pid int) bool { return pid != deadPID },
		StartOpener: func(path string) (<-chan error, error) {
			h.opened = append(h.opened, path)
			exited := make(chan error, 1)
			exited <- nil
			return exited, nil
		},
		StartBound: time.Minute,
		Poll:       time.Millisecond,
	}
	if err := os.MkdirAll(h.deps.Paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return h
}

// holdLock takes the server's lock for the rest of the test, as a serving server does.
func (h *openHarness) holdLock() {
	h.t.Helper()
	held, err := lock.AcquireWriteLock(h.deps.Paths.Lock)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = held.Release() })
}

func (h *openHarness) writeState(pid, port int, token string) {
	h.t.Helper()
	paths := h.deps.Paths
	if err := state.WriteJSON(paths.State, paths.State+".test-lock", boardserve.State{PID: pid, Port: port, Token: token}); err != nil {
		h.t.Fatal(err)
	}
}

// replaceStateOnProbe writes the next server's state once the previous one's was asked and did not answer.
func (h *openHarness) replaceStateOnProbe() {
	probe := h.deps.Probe
	h.deps.Probe = func(server boardserve.State) error {
		err := probe(server)
		if err != nil {
			h.writeState(11, 2222, answeringToken)
		}
		return err
	}
}

// wantOpened checks a successful open of the server on port, which started a server or not.
func (h *openHarness) wantOpened(port string, started bool) {
	h.t.Helper()
	got, err := runOpen(h.deps)
	want := openResult{Address: "http://127.0.0.1:" + port + "/", Launcher: h.deps.Paths.Launcher, Started: started}
	if err != nil || got != want {
		h.t.Fatalf("runOpen = %+v, %v; want %+v", got, err, want)
	}
	if len(h.opened) != 1 || h.opened[0] != want.Launcher || strings.Contains(h.opened[0], answeringToken) {
		h.t.Errorf("opened %v; want the launcher alone, with no token in its argument", h.opened)
	}
}

// wantRefused checks an open that fails naming every fragment.
func (h *openHarness) wantRefused(fragments ...string) {
	h.t.Helper()
	_, err := runOpen(h.deps)
	if err == nil {
		h.t.Fatal("runOpen succeeded; want a refusal")
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			h.t.Errorf("error %q lacks %q", err, fragment)
		}
	}
}

// TestRunOpen pins when an open starts a server, which server it takes up and how it waits on the opener.
func TestRunOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RunningServer", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.holdLock()
		h.writeState(10, 1111, answeringToken)
		h.wantOpened("1111", false)
		if len(h.spawned) != 0 {
			t.Errorf("spawned %v; want none", h.spawned)
		}
	})

	t.Run("NewStateArrivesDuringTheWait", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.holdLock()
		h.writeState(10, 1111, "previous")
		h.replaceStateOnProbe()
		h.wantOpened("2222", false)
	})

	t.Run("HeldLockWithNoAnsweringServer", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			setup func(h *openHarness)
		}{
			{"NoState", func(*openHarness) {}},
			{"ProcessNotRunning", func(h *openHarness) { h.writeState(deadPID, 1111, answeringToken) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOpenHarness(t)
				h.holdLock()
				tt.setup(h)
				h.deps.StartBound = 20 * time.Millisecond
				h.wantRefused("did not answer", h.deps.Paths.Log)
				if len(h.spawned) != 0 || len(h.opened) != 0 {
					t.Errorf("spawned %v, opened %v; want neither", h.spawned, h.opened)
				}
			})
		}
	})

	t.Run("StartsAServerOverALeftoverState", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.writeState(10, 1111, "previous")
		h.replaceStateOnProbe()
		h.wantOpened("2222", true)
		if len(h.spawned) != 1 || h.spawned[0] != testPrimeAnchor {
			t.Errorf("spawned %v; want one in %s", h.spawned, testPrimeAnchor)
		}
	})

	t.Run("StartedServerNeverAnswers", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.writeState(10, 1111, "previous")
		h.deps.StartBound = 20 * time.Millisecond
		h.wantRefused("did not answer", h.deps.Paths.Log)
	})

	t.Run("PrimeFails", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.deps.Prime = func() (string, error) { return "", errors.New("no prime worktree") }
		h.wantRefused("no prime worktree")
		if len(h.spawned) != 0 {
			t.Errorf("spawned %v; want none", h.spawned)
		}
	})

	t.Run("SpawnFails", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.deps.Spawn = func(string, string) error { return errors.New("no such binary") }
		h.wantRefused("no such binary", "way forward", h.deps.Paths.Log)
		if len(h.opened) != 0 {
			t.Errorf("opened %v; want nothing", h.opened)
		}
	})

	t.Run("OpenerFails", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			start func(string) (<-chan error, error)
		}{
			{"AtStart", func(string) (<-chan error, error) { return nil, errors.New("opener broke") }},
			{"AtExit", func(string) (<-chan error, error) {
				exited := make(chan error, 1)
				exited <- errors.New("opener broke")
				return exited, nil
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOpenHarness(t)
				h.holdLock()
				h.writeState(10, 1111, answeringToken)
				h.deps.StartOpener = tt.start
				h.wantRefused(openerName, "opener broke", "open "+h.deps.Paths.Launcher+" in a browser by hand")
			})
		}
	})

	t.Run("CappedWaitLeavesTheOpenerRunning", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.holdLock()
		h.writeState(10, 1111, answeringToken)
		h.deps.OpenerCap = 10 * time.Millisecond
		h.deps.StartOpener = func(path string) (<-chan error, error) {
			h.opened = append(h.opened, path)
			return make(chan error), nil
		}
		h.wantOpened("1111", false)
	})

	t.Run("UncappedWaitLastsUntilTheOpenerExits", func(t *testing.T) {
		t.Parallel()
		h := newOpenHarness(t)
		h.holdLock()
		h.writeState(10, 1111, answeringToken)
		exited := make(chan error)
		h.deps.StartOpener = func(string) (<-chan error, error) { return exited, nil }
		done := make(chan error, 1)
		go func() {
			_, err := runOpen(h.deps)
			done <- err
		}()

		// The unbuffered send is received only by an open still waiting on the opener.
		select {
		case exited <- nil:
		case err := <-done:
			t.Fatalf("runOpen returned %v before the opener exited", err)
		}
		if err := <-done; err != nil {
			t.Errorf("runOpen = %v; want success", err)
		}
	})
}
