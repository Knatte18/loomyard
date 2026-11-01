package shuttleengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// awaitClock is a fake clock whose Sleep advances the time, records each length and runs onSleep with the 1-based call number.
type awaitClock struct {
	*fakeClock
	sleeps  []time.Duration
	onSleep func(call int)
}

func (c *awaitClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	c.sleeps = append(c.sleeps, d)
	if c.onSleep != nil {
		c.onSleep(len(c.sleeps))
	}
}

// TestAwaitForks pins the capped poll over a run's outstanding forks on a fake clock: when it answers ready, what it answers at the cap, what it returns on a read error or a cancelled context, and that no sleep passes one second or the remaining cap.
func TestAwaitForks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fork := BackgroundTask{Kind: BackgroundFork, ID: "a1"}

	tests := []struct {
		name string
		ctx  context.Context
		cap  time.Duration
		// runningReads is how many reads report the fork still running before one reports none.
		runningReads int
		readErr      error
		wantRunning  []BackgroundTask
		wantReady    bool
		wantErr      bool
		wantSleeps   []time.Duration
	}{
		{name: "no fork running answers at once", cap: 10 * time.Second, wantReady: true},
		{
			name:         "the read after the last fork finishes answers ready",
			cap:          10 * time.Second,
			runningReads: 2,
			wantReady:    true,
			wantSleeps:   []time.Duration{time.Second, time.Second},
		},
		{
			name:         "cap elapsing answers the forks still running and the last sleep is the remaining cap",
			cap:          2500 * time.Millisecond,
			runningReads: 10,
			wantRunning:  []BackgroundTask{fork},
			wantSleeps:   []time.Duration{time.Second, time.Second, 500 * time.Millisecond},
		},
		{name: "a read error is returned", cap: 10 * time.Second, readErr: errors.New("transcript unreadable"), wantErr: true},
		{name: "a cancelled context is returned", ctx: cancelled, cap: 10 * time.Second, runningReads: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := &awaitClock{fakeClock: newFakeClock(time.Unix(0, 0))}
			reads := 0
			readOutstanding := func() ([]BackgroundTask, error) {
				reads++
				if tt.readErr != nil {
					return nil, tt.readErr
				}
				if reads <= tt.runningReads {
					return []BackgroundTask{fork}, nil
				}
				return nil, nil
			}
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			running, ready, err := awaitForks(ctx, clock, readOutstanding, tt.cap)

			if (err != nil) != tt.wantErr {
				t.Fatalf("awaitForks() error = %v; want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(running, tt.wantRunning) || ready != tt.wantReady {
				t.Errorf("awaitForks() = (%+v, %v); want (%+v, %v)", running, ready, tt.wantRunning, tt.wantReady)
			}
			if !reflect.DeepEqual(clock.sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", clock.sleeps, tt.wantSleeps)
			}
		})
	}
}

// forkReaderEngine is a fakeEngine with the OutstandingForkReader capability: it records the arguments of each read and answers a fixed fork list.
type forkReaderEngine struct {
	fakeEngine

	forks    []BackgroundTask
	sessions []string
	workdirs []string
}

func (e *forkReaderEngine) OutstandingForks(sessionID, workdir string) ([]BackgroundTask, error) {
	e.sessions = append(e.sessions, sessionID)
	e.workdirs = append(e.workdirs, workdir)
	return e.forks, nil
}

// TestRunnerAwaitForks pins the wrapper around the poll: its refusals for an unknown guid and an engine without the OutstandingForkReader capability,
// and that the reader is handed the run's session id and the pane's cwd.
func TestRunnerAwaitForks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const guid = "strand-1"
	fork := BackgroundTask{Kind: BackgroundFork, ID: "a1"}

	tests := []struct {
		name     string
		capable  bool
		askGUID  string
		wantErr  string
		wantFork []BackgroundTask
	}{
		{name: "engine without the capability is refused", askGUID: guid, wantErr: "OutstandingForkReader"},
		{name: "unknown guid is refused", capable: true, askGUID: "no-such-strand", wantErr: "no-such-strand"},
		{name: "capable engine is read with the session and pane cwd", capable: true, askGUID: guid, wantFork: []BackgroundTask{fork}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			anchor := filepath.Join(root, "worktree")
			paneCwd := filepath.Join(root, "pane")
			runDir := filepath.Join(root, "runs", "run-1")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := saveRunState(runDir, RunState{RunID: "run-1", StrandGUID: guid, SessionID: "session-1", Outcome: runOutcomeRunning}); err != nil {
				t.Fatal(err)
			}

			var engine Engine = &fakeEngine{}
			reader := &forkReaderEngine{forks: []BackgroundTask{fork}}
			if tt.capable {
				engine = reader
			}
			runner := NewDetachedRunner(&fakeReed{}, singleEngineSet(engine), anchor, filepath.Join(root, "standalone-worktree"), paneCwd, Config{RunDir: filepath.Join(root, "runs")})
			runner.SetClock(&awaitClock{fakeClock: newFakeClock(time.Unix(0, 0))})

			got, ready, err := runner.AwaitForks(context.Background(), tt.askGUID, 0)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("AwaitForks() error = %v; want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || ready || !reflect.DeepEqual(got, tt.wantFork) {
				t.Errorf("AwaitForks() = (%+v, %v, %v); want (%+v, false, nil)", got, ready, err, tt.wantFork)
			}
			if !reflect.DeepEqual(reader.sessions, []string{"session-1"}) || !reflect.DeepEqual(reader.workdirs, []string{paneCwd}) {
				t.Errorf("reader got sessions %v and workdirs %v; want [session-1] and [%s]", reader.sessions, reader.workdirs, paneCwd)
			}
		})
	}
}

// TestAwaitAny pins the capped poll over a fake clock: which path it names, when it answers not ready, what it returns on a stat error or a cancelled context, and that no sleep passes one second or the remaining cap.
func TestAwaitAny(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		// paths are names under the temp directory, except a "blocker/" prefix, which sits under a regular file.
		paths []string
		cap   time.Duration
		// seed names the files that exist before the poll.
		seed []string
		// appear maps a Sleep call number to the file created by it.
		appear map[int]string
		// wantPath is the name the poll answers, empty for not ready or an error.
		wantPath  string
		wantReady bool
		wantErr   bool
		// wantSleeps are the lengths Sleep is called with.
		wantSleeps []time.Duration
	}{
		{
			name:      "existing path answers at once",
			paths:     []string{"a"},
			cap:       10 * time.Second,
			seed:      []string{"a"},
			wantPath:  "a",
			wantReady: true,
		},
		{
			name:       "the path that appears first is named",
			paths:      []string{"a", "b"},
			cap:        10 * time.Second,
			appear:     map[int]string{2: "b"},
			wantPath:   "b",
			wantReady:  true,
			wantSleeps: []time.Duration{time.Second, time.Second},
		},
		{
			name:       "cap elapsing answers not ready and the last sleep is the remaining cap",
			paths:      []string{"a"},
			cap:        2500 * time.Millisecond,
			wantSleeps: []time.Duration{time.Second, time.Second, 500 * time.Millisecond},
		},
		{
			name:    "a stat error other than not-exist is returned",
			paths:   []string{"blocker/a"},
			cap:     10 * time.Second,
			seed:    []string{"blocker"},
			wantErr: true,
		},
		{
			name:    "a cancelled context is returned",
			ctx:     cancelled,
			paths:   []string{"a"},
			cap:     10 * time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for _, name := range tt.seed {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					t.Fatalf("seed %q: %v", name, err)
				}
			}
			paths := make([]string, len(tt.paths))
			for i, name := range tt.paths {
				paths[i] = filepath.Join(dir, name)
			}
			clock := &awaitClock{fakeClock: newFakeClock(time.Unix(0, 0))}
			clock.onSleep = func(call int) {
				if name, ok := tt.appear[call]; ok {
					if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
						t.Errorf("create %q: %v", name, err)
					}
				}
			}
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			got, ready, err := awaitAny(ctx, clock, paths, tt.cap)

			if (err != nil) != tt.wantErr {
				t.Fatalf("awaitAny() error = %v; want error %v", err, tt.wantErr)
			}
			wantPath := ""
			if tt.wantPath != "" {
				wantPath = filepath.Join(dir, tt.wantPath)
			}
			if got != wantPath || ready != tt.wantReady {
				t.Errorf("awaitAny() = (%q, %v); want (%q, %v)", got, ready, wantPath, tt.wantReady)
			}
			if len(clock.sleeps) != len(tt.wantSleeps) {
				t.Fatalf("sleeps = %v; want %v", clock.sleeps, tt.wantSleeps)
			}
			for i, want := range tt.wantSleeps {
				if clock.sleeps[i] != want {
					t.Errorf("sleeps = %v; want %v", clock.sleeps, tt.wantSleeps)
					break
				}
			}
		})
	}
}
