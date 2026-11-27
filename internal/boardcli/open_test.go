// open_test.go drives what `lym board open` does over real lock and state files, with a fake server and opener.

package boardcli

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The states of a former server and of the one that answers.
var (
	formerServer = boardserve.State{PID: 11, Port: 1111, Token: "former-token"}
	liveServer   = boardserve.State{PID: 22, Port: 2222, Token: "live-token"}
)

// openFixture is the dependencies of one open over a temp state directory, and what the open did with them.
// liveServer alone answers a probe, and both servers' pids run.
type openFixture struct {
	t      *testing.T
	deps   openDeps
	spawns []string
	opened []string
}

func newOpenFixture(t *testing.T) *openFixture {
	t.Helper()
	f := &openFixture{t: t}
	paths := boardserve.Paths(t.TempDir())
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f.deps = openDeps{
		Paths: paths,
		Prime: func() (string, error) { return "/hub/prime", nil },
		Spawn: func(anchor, logPath string) error {
			f.spawns = append(f.spawns, anchor+" "+logPath)
			return nil
		},
		Probe: func(server boardserve.State) error {
			if server != liveServer {
				return errors.New("connection refused")
			}
			return nil
		},
		Running: func(pid int) bool { return pid == formerServer.PID || pid == liveServer.PID },
		StartOpener: func(path string) (<-chan error, error) {
			f.opened = append(f.opened, path)
			exited := make(chan error, 1)
			exited <- nil
			return exited, nil
		},
		StartBound: time.Minute,
		Poll:       time.Millisecond,
	}
	return f
}

// holdLock takes the server's lock for the rest of the test, as a starting or serving server does.
func (f *openFixture) holdLock() {
	f.t.Helper()
	held, err := lock.AcquireWriteLock(f.deps.Paths.Lock)
	if err != nil {
		f.t.Fatalf("lock: %v", err)
	}
	f.t.Cleanup(func() { _ = held.Release() })
}

func (f *openFixture) writeState(server boardserve.State) {
	f.t.Helper()
	if err := state.WriteJSON(f.deps.Paths.State, f.deps.Paths.State+".lock", server); err != nil {
		f.t.Fatalf("write state: %v", err)
	}
}

// replaceStateOnProbe has the new server's state take the former one's place once the former one has been probed.
func (f *openFixture) replaceStateOnProbe() {
	probe := f.deps.Probe
	f.deps.Probe = func(server boardserve.State) error {
		if server == formerServer {
			f.writeState(liveServer)
		}
		return probe(server)
	}
}

// wantOpened asserts a successful open of liveServer.
func (f *openFixture) wantOpened(result openResult, err error, started bool) {
	f.t.Helper()
	want := openResult{Address: "http://127.0.0.1:2222/", Launcher: f.deps.Paths.Launcher, Started: started}
	if err != nil || result != want {
		f.t.Fatalf("runOpen = %+v, %v; want %+v", result, err, want)
	}
	if !slices.Equal(f.opened, []string{f.deps.Paths.Launcher}) || strings.Contains(f.deps.Paths.Launcher, liveServer.Token) {
		f.t.Errorf("opened %q; want the launcher alone", f.opened)
	}
}

// wantRefused asserts an open that failed naming every one of parts, with no opener started.
func (f *openFixture) wantRefused(err error, parts ...string) {
	f.t.Helper()
	if err == nil {
		f.t.Fatalf("runOpen = nil error; want a refusal")
	}
	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			f.t.Errorf("runOpen error = %q; want it to hold %q", err, part)
		}
	}
	if len(f.opened) != 0 {
		f.t.Errorf("opened %q; want nothing", f.opened)
	}
}

func TestOpenerCapFor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	dirs := configengine.Dirs{Anchor: dir, Board: dir, Records: dir}
	if got := openerCapFor(dirs, ""); got != 0 {
		t.Errorf("openerCapFor with no strand = %s; want no cap", got)
	}
	if got := openerCapFor(dirs, "ly:some-task:orch"); got <= 0 {
		t.Errorf("openerCapFor of an orch strand = %s; want a cap", got)
	}
}

func TestRunOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ALiveServerIsOpened", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock()
		f.writeState(liveServer)
		result, err := runOpen(f.deps)
		f.wantOpened(result, err, false)
		if len(f.spawns) != 0 {
			t.Errorf("spawns = %q; want none", f.spawns)
		}
	})

	t.Run("AStartingServersStateIsWaitedFor", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock()
		f.writeState(formerServer)
		f.replaceStateOnProbe()
		result, err := runOpen(f.deps)
		f.wantOpened(result, err, false)
	})

	t.Run("AHeldLockWithNoAnswerIsRefused", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			server boardserve.State
		}{
			{"NoState", boardserve.State{}},
			{"NoAnswer", formerServer},
			{"PIDNotRunning", boardserve.State{PID: 33, Port: liveServer.Port, Token: liveServer.Token}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newOpenFixture(t)
				f.holdLock()
				if tt.server != (boardserve.State{}) {
					f.writeState(tt.server)
				}
				f.deps.Probe = func(server boardserve.State) error {
					if server.Token != liveServer.Token {
						return errors.New("connection refused")
					}
					return nil
				}
				f.deps.StartBound = 20 * time.Millisecond
				_, err := runOpen(f.deps)
				f.wantRefused(err, "did not answer", f.deps.Paths.Log, "way forward")
				if len(f.spawns) != 0 {
					t.Errorf("spawns = %q; want none", f.spawns)
				}
			})
		}
	})

	t.Run("AFreeLockStartsAServer", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.writeState(formerServer)
		spawn := f.deps.Spawn
		f.deps.Spawn = func(anchor, logPath string) error {
			f.holdLock()
			return spawn(anchor, logPath)
		}
		f.replaceStateOnProbe()
		result, err := runOpen(f.deps)
		f.wantOpened(result, err, true)
		if want := []string{"/hub/prime " + f.deps.Paths.Log}; !slices.Equal(f.spawns, want) {
			t.Errorf("spawns = %q; want %q", f.spawns, want)
		}

		silent := newOpenFixture(t)
		silent.writeState(liveServer)
		silent.deps.StartBound = 20 * time.Millisecond
		_, err = runOpen(silent.deps)
		silent.wantRefused(err, "did not answer", silent.deps.Paths.Log)
		if len(silent.spawns) != 1 {
			t.Errorf("spawns = %q; want one", silent.spawns)
		}
	})

	t.Run("AFailedStartIsRefused", func(t *testing.T) {
		t.Parallel()
		unresolved := newOpenFixture(t)
		primeErr := errors.New("no prime")
		unresolved.deps.Prime = func() (string, error) { return "", primeErr }
		if _, err := runOpen(unresolved.deps); !errors.Is(err, primeErr) || len(unresolved.spawns) != 0 {
			t.Errorf("runOpen = %v with spawns %q; want the prime's error and none", err, unresolved.spawns)
		}

		unspawned := newOpenFixture(t)
		unspawned.deps.Spawn = func(string, string) error { return errors.New("no binary") }
		_, err := runOpen(unspawned.deps)
		unspawned.wantRefused(err, "start the board server: no binary", "way forward", unspawned.deps.Paths.Log)
	})

	t.Run("AFailedOpenerNamesTheLauncher", func(t *testing.T) {
		t.Parallel()
		exitedBadly := make(chan error, 1)
		exitedBadly <- errors.New("exit status 3")
		tests := []struct {
			name   string
			opener func(string) (<-chan error, error)
		}{
			{"Start", func(string) (<-chan error, error) { return nil, errors.New("not found") }},
			{"Exit", func(string) (<-chan error, error) { return exitedBadly, nil }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newOpenFixture(t)
				f.holdLock()
				f.writeState(liveServer)
				f.deps.StartOpener = tt.opener
				_, err := runOpen(f.deps)
				f.wantRefused(err, "the opener "+openerName+" failed", "open "+f.deps.Paths.Launcher+" in a browser by hand")
			})
		}
	})

	t.Run("TheCapEndsTheWaitOnTheOpener", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock()
		f.writeState(liveServer)
		f.deps.StartOpener = func(string) (<-chan error, error) { return make(chan error), nil }
		f.deps.OpenerCap = 10 * time.Millisecond
		if _, err := runOpen(f.deps); err != nil {
			t.Errorf("runOpen = %v; want success at the cap", err)
		}
	})

	t.Run("NoCapWaitsForTheOpenerToExit", func(t *testing.T) {
		t.Parallel()
		f := newOpenFixture(t)
		f.holdLock()
		f.writeState(liveServer)
		exited := make(chan error)
		f.deps.StartOpener = func(string) (<-chan error, error) { return exited, nil }
		returned := make(chan error, 1)
		go func() {
			_, err := runOpen(f.deps)
			returned <- err
		}()
		// The send is received only by an open still waiting on its opener.
		select {
		case exited <- nil:
		case err := <-returned:
			t.Fatalf("runOpen returned %v before the opener exited", err)
		}
		if err := <-returned; err != nil {
			t.Errorf("runOpen = %v; want success", err)
		}
	})
}
