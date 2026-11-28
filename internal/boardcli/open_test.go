// open_test.go drives the opening of the board page over real lock files, with its processes and probe faked.

package boardcli

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// openTestDeadline only ends a wait a passing test never reaches.
	openTestDeadline = time.Minute
	primeAnchor      = "/hub/prime"
)

var (
	oldServer = boardserve.State{PID: 10, Port: 1000, Token: "old-token"}
	newServer = boardserve.State{PID: 20, Port: 2000, Token: "new-token"}
)

// openFixture is a temp state directory with the fakes of one open.
type openFixture struct {
	deps openDeps
	// answering is the token the fake probe accepts.
	answering string
	// spawned and opened record the directories a server was started in and the paths the opener was given.
	spawned, opened []string
}

func newOpenFixture(t *testing.T) *openFixture {
	t.Helper()
	paths := boardserve.Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f := &openFixture{answering: newServer.Token}
	f.deps = openDeps{
		Paths: paths,
		Prime: func() (string, error) { return primeAnchor, nil },
		Spawn: func(dir, _ string) error {
			f.spawned = append(f.spawned, dir)
			f.holdLock(t)
			f.writeState(t, newServer)
			return nil
		},
		Probe: func(current boardserve.State) error {
			if current.Token != f.answering {
				return errors.New("no answer")
			}
			return nil
		},
		Running: func(int) bool { return true },
		StartOpener: func(path string) (<-chan error, error) {
			f.opened = append(f.opened, path)
			exited := make(chan error, 1)
			exited <- nil
			return exited, nil
		},
		StartBound: openTestDeadline,
		Poll:       time.Millisecond,
	}
	return f
}

// holdLock holds the server's lock until the test ends, as a live server does.
func (f *openFixture) holdLock(t *testing.T) {
	t.Helper()
	held, err := lock.AcquireWriteLock(f.deps.Paths.Lock)
	if err != nil {
		t.Fatalf("hold the server's lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

func (f *openFixture) writeState(t *testing.T, current boardserve.State) {
	t.Helper()
	data, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	if err := os.WriteFile(f.deps.Paths.State, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

// wantOpened checks that the page of server was opened through the launcher, with no token on the way.
func (f *openFixture) wantOpened(t *testing.T, result openResult, err error, server boardserve.State, started bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("runOpen() = %v; want success", err)
	}
	want := openResult{Address: boardserve.PageAddress(server.Port), Launcher: f.deps.Paths.Launcher, Started: started}
	if result != want || strings.Contains(result.Address, server.Token) {
		t.Errorf("runOpen() = %+v; want %+v", result, want)
	}
	if len(f.opened) != 1 || f.opened[0] != f.deps.Paths.Launcher || strings.Contains(f.opened[0], server.Token) {
		t.Errorf("opener got %q; want the launcher path alone", f.opened)
	}
	page, err := os.ReadFile(f.deps.Paths.Launcher)
	if err != nil || !strings.Contains(string(page), "#"+server.Token) {
		t.Errorf("launcher = %q, %v; want it to carry the answering server's token", page, err)
	}
}

// wantRefused checks that runOpen refused with every part and opened nothing.
func (f *openFixture) wantRefused(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("runOpen() = nil error; want a refusal")
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("runOpen() error = %q; want it to hold %q", err, part)
		}
	}
	if len(f.opened) != 0 {
		t.Errorf("opener got %q; want it not started", f.opened)
	}
}

func TestRunOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ALiveServerIsReused", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock(t)
		f.writeState(t, newServer)
		result, err := runOpen(f.deps)
		f.wantOpened(t, result, err, newServer, false)
		if len(f.spawned) != 0 {
			t.Errorf("spawned in %q; want no spawn", f.spawned)
		}
	})

	t.Run("AStartingServerIsWaitedFor", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock(t)
		f.writeState(t, oldServer)
		probe := f.deps.Probe
		f.deps.Probe = func(current boardserve.State) error {
			if current == oldServer {
				f.writeState(t, newServer)
			}
			return probe(current)
		}
		result, err := runOpen(f.deps)
		f.wantOpened(t, result, err, newServer, false)
		if len(f.spawned) != 0 {
			t.Errorf("spawned in %q; want no spawn", f.spawned)
		}
	})

	t.Run("AHeldLockWithNoAnswerRefuses", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			state   *boardserve.State
			running bool
		}{
			{"NoState", nil, true},
			{"AStateThatDoesNotAnswer", &oldServer, true},
			{"AStateWhosePidIsNotRunning", &newServer, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newOpenFixture(t)
				f.holdLock(t)
				if tt.state != nil {
					f.writeState(t, *tt.state)
				}
				f.deps.Running = func(int) bool { return tt.running }
				f.deps.StartBound = 20 * time.Millisecond
				_, err := runOpen(f.deps)
				f.wantRefused(t, err, f.deps.Paths.Log, "way forward: end that process")
				if len(f.spawned) != 0 {
					t.Errorf("spawned in %q; want no spawn", f.spawned)
				}
			})
		}
	})

	t.Run("AFreeLockStartsAServerInThePrime", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.writeState(t, oldServer)
		result, err := runOpen(f.deps)
		f.wantOpened(t, result, err, newServer, true)
		if len(f.spawned) != 1 || f.spawned[0] != primeAnchor {
			t.Errorf("spawned in %q; want one spawn in %q", f.spawned, primeAnchor)
		}
	})

	t.Run("AStartedServerThatNeverAnswersRefuses", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.writeState(t, oldServer)
		f.deps.Spawn = func(string, string) error { return nil }
		f.deps.StartBound = 20 * time.Millisecond
		_, err := runOpen(f.deps)
		f.wantRefused(t, err, f.deps.Paths.Log, "way forward")
	})

	t.Run("AnUnresolvedPrimeStartsNothing", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		unresolved := errors.New("no prime")
		f.deps.Prime = func() (string, error) { return "", unresolved }
		_, err := runOpen(f.deps)
		if !errors.Is(err, unresolved) || len(f.spawned) != 0 || len(f.opened) != 0 {
			t.Errorf("runOpen() = %v, spawned %q, opened %q; want the prime's error and nothing started", err, f.spawned, f.opened)
		}
	})

	t.Run("AFailedSpawnNamesTheLog", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.deps.Spawn = func(string, string) error { return errors.New("no binary") }
		_, err := runOpen(f.deps)
		f.wantRefused(t, err, "no binary", f.deps.Paths.Log, "way forward")
	})

	t.Run("AFailedOpenerNamesTheLauncher", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name              string
			startErr, exitErr error
		}{
			{"DoesNotStart", errors.New("not found"), nil},
			{"ExitsWithAnError", nil, errors.New("exit status 3")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newOpenFixture(t)
				f.deps.StartOpener = func(string) (<-chan error, error) {
					exited := make(chan error, 1)
					exited <- tt.exitErr
					return exited, tt.startErr
				}
				_, err := runOpen(f.deps)
				f.wantRefused(t, err, "the opener "+openerName+" failed", "open "+f.deps.Paths.Launcher+" in a browser by hand")
			})
		}
	})

	t.Run("ACappedWaitLeavesTheOpenerRunning", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.deps.StartOpener = func(path string) (<-chan error, error) {
			f.opened = append(f.opened, path)
			return make(chan error), nil
		}
		f.deps.OpenerCap = time.Millisecond
		result, err := runOpen(f.deps)
		f.wantOpened(t, result, err, newServer, true)
	})

	t.Run("AnUncappedWaitEndsWithTheOpener", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		exited := make(chan error)
		f.deps.StartOpener = func(string) (<-chan error, error) { return exited, nil }
		returned := make(chan error, 1)
		go func() {
			_, err := runOpen(f.deps)
			returned <- err
		}()
		// The send is unbuffered, so it completes only while runOpen still waits on the opener.
		select {
		case exited <- errors.New("exit status 1"):
		case err := <-returned:
			t.Fatalf("runOpen() = %v before the opener exited", err)
		case <-time.After(openTestDeadline):
			t.Fatalf("runOpen() never waited on the opener")
		}
		if err := <-returned; err == nil || !strings.Contains(err.Error(), "exit status 1") {
			t.Errorf("runOpen() = %v; want the opener's exit error", err)
		}
	})
}
