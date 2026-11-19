// open_test.go tests the open sequence in process.
// It uses fake spawn, probe, process-liveness and opener seams.
// The lock and state files are real, in a temp state directory.

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

// fakePrimeAnchor is the directory the fake Prime resolves.
const fakePrimeAnchor = "/prime/anchored"

// openFixture is runOpen's deps over a temp state directory, with fakes that record what they were asked.
type openFixture struct {
	deps  openDeps
	paths boardserve.StatePaths
	// running and answering name the pids that run and the tokens whose server answers a probe.
	running   map[int]bool
	answering map[string]bool
	// onSpawn and onRunning let a test play the server's side; nil does nothing.
	onSpawn   func() error
	onRunning func(pid int)
	// openerResult supplies the opener's start; nil starts one that exited cleanly.
	openerResult func() (<-chan error, error)
	spawned      []spawnCall
	opened       []string
}

// spawnCall is one call of the fake Spawn.
type spawnCall struct{ dir, logPath string }

func newOpenFixture(t *testing.T) *openFixture {
	t.Helper()
	paths := boardserve.Paths(t.TempDir())
	fixture := &openFixture{paths: paths, running: map[int]bool{}, answering: map[string]bool{}}
	fixture.deps = openDeps{
		Paths: paths,
		Prime: func() (string, error) { return fakePrimeAnchor, nil },
		Spawn: func(dir, logPath string) error {
			fixture.spawned = append(fixture.spawned, spawnCall{dir, logPath})
			if fixture.onSpawn != nil {
				return fixture.onSpawn()
			}
			return nil
		},
		Probe: func(st boardserve.State, _ time.Duration) error {
			if fixture.answering[st.Token] {
				return nil
			}
			return errors.New("no answer")
		},
		Running: func(pid int) bool {
			if fixture.onRunning != nil {
				fixture.onRunning(pid)
			}
			return fixture.running[pid]
		},
		StartOpener: func(path string) (<-chan error, error) {
			fixture.opened = append(fixture.opened, path)
			if fixture.openerResult != nil {
				return fixture.openerResult()
			}
			exited := make(chan error, 1)
			exited <- nil
			return exited, nil
		},
		StartBound: 300 * time.Millisecond,
		Poll:       5 * time.Millisecond,
	}
	return fixture
}

// writeState writes the state file a server leaves.
func (f *openFixture) writeState(t *testing.T, st boardserve.State) {
	t.Helper()
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("encode the state: %v", err)
	}
	if err := os.MkdirAll(f.paths.Dir, 0o755); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	if err := os.WriteFile(f.paths.State, data, 0o600); err != nil {
		t.Fatalf("write the state: %v", err)
	}
}

// holdLock takes the server lock for the rest of the test, as a live or starting server does.
func (f *openFixture) holdLock(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(f.paths.Dir, 0o755); err != nil {
		t.Fatalf("create the state directory: %v", err)
	}
	held, err := lock.AcquireWriteLock(f.paths.Lock)
	if err != nil {
		t.Fatalf("hold the server lock: %v", err)
	}
	t.Cleanup(func() { held.Release() })
}

// TestRunOpen drives runOpen over every path a server's state and lock can leave open with.
func TestRunOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("LiveServerThatAnswersIsReusedWithoutASpawn", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.holdLock(t)
		fixture.writeState(t, boardserve.State{PID: 4242, Port: 5555, Token: "live-token"})
		fixture.running[4242] = true
		fixture.answering["live-token"] = true

		result, err := runOpen(fixture.deps)
		if err != nil {
			t.Fatalf("runOpen: %v", err)
		}
		if result.Started || result.Address != "http://127.0.0.1:5555/" || len(fixture.spawned) != 0 {
			t.Errorf("result = %+v with %d spawns; want the live server's address, not started, no spawn", result, len(fixture.spawned))
		}
		if len(fixture.opened) != 1 || fixture.opened[0] != fixture.paths.Launcher {
			t.Errorf("opener paths = %v; want the launcher %s", fixture.opened, fixture.paths.Launcher)
		}
		for _, text := range []string{result.Address, fixture.opened[0]} {
			if strings.Contains(text, "live-token") {
				t.Errorf("%q holds the token", text)
			}
		}
		if launcher, _ := os.ReadFile(fixture.paths.Launcher); !strings.Contains(string(launcher), "live-token") {
			t.Errorf("launcher = %q; want it to carry the token", launcher)
		}
	})

	t.Run("StartingServerWritesItsStateMidWait", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.holdLock(t)
		fixture.writeState(t, boardserve.State{PID: 1, Port: 1111, Token: "old-token"})
		fixture.answering["new-token"] = true
		fixture.running[2] = true
		// The new server's state lands while open is re-reading the previous one's.
		fixture.onRunning = func(int) {
			fixture.writeState(t, boardserve.State{PID: 2, Port: 2222, Token: "new-token"})
		}

		result, err := runOpen(fixture.deps)
		if err != nil || result.Address != "http://127.0.0.1:2222/" {
			t.Errorf("runOpen = %+v, %v; want the new server's address", result, err)
		}
	})

	t.Run("HeldLockWithNoAnsweringServerIsRefusedWithoutASpawn", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			state *boardserve.State
		}{
			{"NoState", nil},
			{"StateOfAProcessThatIsNotRunning", &boardserve.State{PID: 9, Port: 9999, Token: "dead-token"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fixture := newOpenFixture(t)
				fixture.holdLock(t)
				if tt.state != nil {
					fixture.writeState(t, *tt.state)
					fixture.answering[tt.state.Token] = true
				}
				_, err := runOpen(fixture.deps)
				if err == nil || !strings.Contains(err.Error(), fixture.paths.Log) || !strings.Contains(err.Error(), "way forward") {
					t.Errorf("runOpen error = %v; want a refusal naming the server log and its way forward", err)
				}
				if len(fixture.spawned) != 0 || len(fixture.opened) != 0 {
					t.Errorf("spawns, openers = %d, %d; want none", len(fixture.spawned), len(fixture.opened))
				}
			})
		}
	})

	t.Run("FreeLockSpawnsInThePrimeAndWaitsForTheNewState", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		// The previous server exited, or is draining, and its state still answers.
		fixture.writeState(t, boardserve.State{PID: 77, Port: 1111, Token: "previous-token"})
		fixture.running[77] = true
		fixture.answering["previous-token"] = true
		fixture.running[88] = true
		fixture.answering["new-token"] = true
		fixture.onSpawn = func() error {
			fixture.holdLock(t)
			fixture.writeState(t, boardserve.State{PID: 88, Port: 3333, Token: "new-token"})
			return nil
		}

		result, err := runOpen(fixture.deps)
		if err != nil {
			t.Fatalf("runOpen: %v", err)
		}
		if !result.Started || result.Address != "http://127.0.0.1:3333/" {
			t.Errorf("result = %+v; want the new server's address, started", result)
		}
		if want := []spawnCall{{fakePrimeAnchor, fixture.paths.Log}}; len(fixture.spawned) != 1 || fixture.spawned[0] != want[0] {
			t.Errorf("spawns = %v; want one in %s with the server log", fixture.spawned, fakePrimeAnchor)
		}
		if len(fixture.opened) != 1 || fixture.opened[0] != fixture.paths.Launcher {
			t.Errorf("opener paths = %v; want the launcher", fixture.opened)
		}
	})

	t.Run("FreeLockWhoseNewServerNeverAnswersIsRefusedNamingTheLog", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.writeState(t, boardserve.State{PID: 77, Port: 1111, Token: "previous-token"})
		fixture.running[77] = true
		fixture.answering["previous-token"] = true

		_, err := runOpen(fixture.deps)
		if err == nil || !strings.Contains(err.Error(), fixture.paths.Log) || len(fixture.spawned) != 1 || len(fixture.opened) != 0 {
			t.Errorf("runOpen = %v with %d spawns and %d openers; want a refusal naming the log after one spawn", err, len(fixture.spawned), len(fixture.opened))
		}
	})

	t.Run("FreeLockWithAFailingPrimeSpawnsNothing", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		primeErr := errors.New("the prime cannot be resolved")
		fixture.deps.Prime = func() (string, error) { return "", primeErr }

		if _, err := runOpen(fixture.deps); !errors.Is(err, primeErr) || len(fixture.spawned) != 0 {
			t.Errorf("runOpen = %v with %d spawns; want the prime's error and no spawn", err, len(fixture.spawned))
		}
	})

	t.Run("FreeLockWithAFailingSpawnNamesTheLogAndStartsNoOpener", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.onSpawn = func() error { return errors.New("exec format error") }

		_, err := runOpen(fixture.deps)
		for _, fragment := range []string{"start the board server", "exec format error", fixture.paths.Log, "way forward"} {
			if err == nil || !strings.Contains(err.Error(), fragment) {
				t.Errorf("runOpen error = %v; want it to hold %q", err, fragment)
			}
		}
		if len(fixture.opened) != 0 {
			t.Errorf("an opener started after a failed spawn")
		}
	})

	t.Run("FailingOpenerNamesItselfAndTheLauncherToOpenByHand", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			opener func() (<-chan error, error)
		}{
			{"StartFails", func() (<-chan error, error) { return nil, errors.New("not found") }},
			{"ExitsWithAnError", func() (<-chan error, error) {
				exited := make(chan error, 1)
				exited <- errors.New("exit status 3")
				return exited, nil
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fixture := newOpenFixture(t)
				fixture.holdLock(t)
				fixture.writeState(t, boardserve.State{PID: 4242, Port: 5555, Token: "live-token"})
				fixture.running[4242] = true
				fixture.answering["live-token"] = true
				fixture.openerResult = tt.opener

				_, err := runOpen(fixture.deps)
				for _, fragment := range []string{openerName, fixture.paths.Launcher, "by hand"} {
					if err == nil || !strings.Contains(err.Error(), fragment) {
						t.Errorf("runOpen error = %v; want it to hold %q", err, fragment)
					}
				}
			})
		}
	})

	t.Run("OpenerCapEndsTheWaitOnAnOpenerThatNeverExits", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.holdLock(t)
		fixture.writeState(t, boardserve.State{PID: 4242, Port: 5555, Token: "live-token"})
		fixture.running[4242] = true
		fixture.answering["live-token"] = true
		fixture.openerResult = func() (<-chan error, error) { return make(chan error), nil }
		fixture.deps.OpenerCap = 20 * time.Millisecond

		if _, err := runOpen(fixture.deps); err != nil {
			t.Errorf("runOpen = %v; want success at the cap", err)
		}
	})

	t.Run("NoOpenerCapWaitsUntilTheOpenerExits", func(t *testing.T) {
		t.Parallel()
		fixture := newOpenFixture(t)
		fixture.holdLock(t)
		fixture.writeState(t, boardserve.State{PID: 4242, Port: 5555, Token: "live-token"})
		fixture.running[4242] = true
		fixture.answering["live-token"] = true
		exit := make(chan error, 1)
		started := make(chan struct{})
		fixture.openerResult = func() (<-chan error, error) {
			close(started)
			return exit, nil
		}

		returned := make(chan error, 1)
		go func() {
			_, err := runOpen(fixture.deps)
			returned <- err
		}()
		<-started
		select {
		case err := <-returned:
			t.Fatalf("runOpen returned %v before the opener exited", err)
		default:
		}
		exit <- nil
		select {
		case err := <-returned:
			if err != nil {
				t.Errorf("runOpen = %v; want success once the opener exited", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("runOpen did not return after the opener exited")
		}
	})
}
