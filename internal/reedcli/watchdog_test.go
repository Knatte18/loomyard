// watchdog_test.go pins the watchdog daemon's pure seams — planSessionDiff, sessionsAreIdle,
// planReapCycle, worktreeRootGone, hubIsLiveDir, validateWatchdogFlags, watchdogDefaultTiming, discoveryWake and consumeDiscoverSignal —
// against no tmux server and no filesystem at all beyond t.TempDir(), table-driven.

package reedcli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

//testtiming:keep pins the appeared and departed sets for the empty, populated, partial-overlap and unchanged listings, which the live daemon scenario reaches for two at most
func TestPlanSessionDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		live         []string
		known        map[string]watchedSession
		wantAppeared []string
		wantDeparted []string
	}{
		{
			name:         "empty to populated",
			live:         []string{"alpha", "beta"},
			known:        map[string]watchedSession{},
			wantAppeared: []string{"alpha", "beta"},
			wantDeparted: nil,
		},
		{
			name: "populated to empty",
			live: nil,
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: nil,
			wantDeparted: []string{"alpha", "beta"},
		},
		{
			name: "partial overlap",
			live: []string{"alpha", "gamma"},
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: []string{"gamma"},
			wantDeparted: []string{"beta"},
		},
		{
			name: "no change",
			live: []string{"alpha", "beta"},
			known: map[string]watchedSession{
				"alpha": {},
				"beta":  {},
			},
			wantAppeared: nil,
			wantDeparted: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAppeared, gotDeparted := planSessionDiff(tt.live, tt.known)
			sort.Strings(gotAppeared)
			sort.Strings(gotDeparted)
			if !slices.Equal(gotAppeared, tt.wantAppeared) {
				t.Errorf("planSessionDiff() appeared = %v; want %v", gotAppeared, tt.wantAppeared)
			}
			if !slices.Equal(gotDeparted, tt.wantDeparted) {
				t.Errorf("planSessionDiff() departed = %v; want %v", gotDeparted, tt.wantDeparted)
			}
		})
	}
}

//testtiming:keep pins the idle rule for all four listing and error combinations, which the live watchdog test reaches for one
func TestSessionsAreIdle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		names []string
		err   error
		want  bool
	}{
		{
			name:  "non-empty listing, no error",
			names: []string{"alpha"},
			err:   nil,
			want:  false,
		},
		{
			name:  "empty listing, no error",
			names: nil,
			err:   nil,
			want:  true,
		},
		{
			name:  "non-empty listing, error",
			names: []string{"alpha"},
			err:   errors.New("stale listing"),
			want:  true,
		},
		{
			name:  "empty listing, error",
			names: nil,
			err:   errors.New("no server running"),
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sessionsAreIdle(tt.names, tt.err)
			if got != tt.want {
				t.Errorf("sessionsAreIdle(%v, %v) = %v; want %v", tt.names, tt.err, got, tt.want)
			}
		})
	}
}

// reapCycle is one planReapCycle call and the state it must leave behind.
// A nil inFlight means no name is in flight,
// and a nil wantCounters means the counter map ends empty.
// wantReap and wantRemaining are compared after sorting, since planReapCycle returns them in live order.
type reapCycle struct {
	live          []string
	hubDown       bool
	gone          map[string]bool
	inFlight      map[string]bool
	wantReap      []string
	wantRemaining []string
	wantCounters  map[string]int
}

func TestPlanReapCycle(t *testing.T) {
	t.Parallel()
	alpha := []string{"alpha"}
	alphaGone := map[string]bool{"alpha": true}
	alphaPresent := map[string]bool{"alpha": false}

	tests := []struct {
		name      string
		threshold int
		counters  map[string]int
		cycles    []reapCycle
	}{
		{
			name:      "live directory never reaps",
			threshold: 3,
			cycles: slices.Repeat([]reapCycle{{
				live:          alpha,
				gone:          map[string]bool{},
				wantRemaining: alpha,
			}}, 5),
		},
		{
			name:      "missing fewer than threshold cycles does not reap yet",
			threshold: 3,
			cycles: []reapCycle{
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 2}},
			},
		},
		{
			name:      "missing exactly threshold cycles reaps and deletes the counter",
			threshold: 3,
			cycles: []reapCycle{
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 2}},
				{live: alpha, gone: alphaGone, wantReap: alpha},
			},
		},
		{
			name:      "a present cycle mid-streak resets the counter",
			threshold: 3,
			cycles: []reapCycle{
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 2}},
				{live: alpha, gone: alphaPresent, wantRemaining: alpha},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 2}},
			},
		},
		{
			name:      "a name leaving the live list prunes its counter",
			threshold: 3,
			cycles: []reapCycle{
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 2}},
				{live: []string{}, gone: map[string]bool{}},
				{live: alpha, gone: alphaGone, wantRemaining: alpha, wantCounters: map[string]int{"alpha": 1}},
			},
		},
		{
			name:      "several sessions progress independently",
			threshold: 3,
			counters:  map[string]int{"beta": 1, "gamma": 2},
			cycles: []reapCycle{
				{
					live:          []string{"alpha", "beta", "gamma"},
					gone:          map[string]bool{"alpha": false, "beta": true, "gamma": true},
					wantReap:      []string{"gamma"},
					wantRemaining: []string{"alpha", "beta"},
					wantCounters:  map[string]int{"beta": 2},
				},
			},
		},
		{
			name:      "a churning name never accumulates counter entries",
			threshold: 3,
			cycles: slices.Repeat([]reapCycle{
				{
					live:          []string{"churner"},
					gone:          map[string]bool{"churner": true},
					wantRemaining: []string{"churner"},
					wantCounters:  map[string]int{"churner": 1},
				},
				{live: []string{}, gone: map[string]bool{}},
			}, 50),
		},
		{
			name:      "hub probe refuses to act",
			threshold: 3,
			counters:  map[string]int{"alpha": 2, "beta": 1},
			cycles: []reapCycle{
				{
					live:          []string{"alpha", "beta", "gamma"},
					hubDown:       true,
					gone:          map[string]bool{"alpha": true, "beta": true, "gamma": true},
					inFlight:      map[string]bool{"gamma": true},
					wantRemaining: []string{"alpha", "beta"},
					wantCounters:  map[string]int{"alpha": 2, "beta": 1},
				},
			},
		},
		{
			name:      "in-flight name is excluded from both returns while the hub is live",
			threshold: 3,
			cycles: []reapCycle{
				{
					live:          []string{"alpha", "beta"},
					gone:          map[string]bool{"alpha": true, "beta": false},
					inFlight:      map[string]bool{"alpha": true},
					wantRemaining: []string{"beta"},
				},
			},
		},
		{
			name:      "in-flight name is excluded from both returns while the hub is gone",
			threshold: 3,
			cycles: []reapCycle{
				{
					live:          []string{"alpha", "beta"},
					hubDown:       true,
					gone:          map[string]bool{"alpha": true, "beta": false},
					inFlight:      map[string]bool{"alpha": true},
					wantRemaining: []string{"beta"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			counters := maps.Clone(tt.counters)
			if counters == nil {
				counters = map[string]int{}
			}
			for i, c := range tt.cycles {
				reap, remaining := planReapCycle(c.live, !c.hubDown, c.gone, counters, c.inFlight, tt.threshold)
				sort.Strings(reap)
				sort.Strings(remaining)
				if !slices.Equal(reap, c.wantReap) {
					t.Fatalf("cycle %d: reap = %v, want %v", i, reap, c.wantReap)
				}
				if !slices.Equal(remaining, c.wantRemaining) {
					t.Fatalf("cycle %d: remaining = %v, want %v", i, remaining, c.wantRemaining)
				}
				if !maps.Equal(counters, c.wantCounters) {
					t.Fatalf("cycle %d: counters = %v, want %v", i, counters, c.wantCounters)
				}
			}
		})
	}
}

// TestWorktreeRootGoneAndHubIsLiveDir drives both path predicates over the same path shapes.
// A missing path and a plain file are gone and not live, a directory is the reverse.
// The unreadable row pins the one case where treating an unreadable path as gone would destroy live work: a stat that fails with neither a not-exist result nor success (the EACCES shape) must answer false — conservative — from both predicates, and deliberately not each other's negation.
// That row is skipped on Windows, where directory mode bits do not deny traversal this way, and skipped when the test runs as uid 0, where mode bits are not enforced at all — in both cases the stat would succeed and the assertion would pass for the wrong reason.
func TestWorktreeRootGoneAndHubIsLiveDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// path builds the path under test inside the subtest's own temp directory.
		path     func(t *testing.T, root string) string
		wantGone bool
		wantLive bool
	}{
		{
			name:     "missing path",
			path:     func(t *testing.T, root string) string { return filepath.Join(root, "missing") },
			wantGone: true,
			wantLive: false,
		},
		{
			name: "plain file",
			path: func(t *testing.T, root string) string {
				plainFile := filepath.Join(root, "plain-file")
				if err := os.WriteFile(plainFile, []byte("x"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return plainFile
			},
			wantGone: true,
			wantLive: false,
		},
		{
			name: "directory",
			path: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "a-directory")
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				return dir
			},
			wantGone: false,
			wantLive: true,
		},
		{
			name: "stat error that is not not-exist",
			path: func(t *testing.T, root string) string {
				if runtime.GOOS == "windows" {
					t.Skip("directory mode bits do not deny traversal on Windows")
				}
				if os.Geteuid() == 0 {
					t.Skip("mode bits are not enforced for uid 0")
				}
				parent := filepath.Join(root, "denied-parent")
				if err := os.Mkdir(parent, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				target := filepath.Join(parent, "target")
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				if err := os.Chmod(parent, 0o000); err != nil {
					t.Fatalf("Chmod: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(parent, 0o755); err != nil {
						t.Errorf("Chmod restore: %v", err)
					}
				})
				if _, err := os.Stat(target); err == nil {
					t.Skip("stat of target succeeded despite the denied parent mode; cannot exercise the EACCES shape here")
				}
				return target
			},
			wantGone: false,
			wantLive: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.path(t, t.TempDir())
			if got := worktreeRootGone(path); got != tt.wantGone {
				t.Errorf("worktreeRootGone(%q) = %v, want %v", path, got, tt.wantGone)
			}
			if got := hubIsLiveDir(path); got != tt.wantLive {
				t.Errorf("hubIsLiveDir(%q) = %v, want %v", path, got, tt.wantLive)
			}
		})
	}
}

// validateWatchdogFlags is asserted here rather than through watchdogCmd's RunE deliberately — a CLI-level test of the accepting case would fall through the pre-flight into a global logger mutation, a lock acquisition under a scratch directory the command never creates, and then the discovery loop, whose first tick shells out to tmux via the os/exec package and is forbidden in an untagged file.
func TestValidateWatchdogFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hubPath  string
		tmuxPath string
		wantErr  string
	}{
		{"empty hubPath", "", "/usr/bin/tmux", "--hub-path must be an absolute, non-empty path"},
		{"relative hubPath", "relative/path", "/usr/bin/tmux", "--hub-path must be an absolute, non-empty path"},
		{"empty tmuxPath", "/abs/hub", "", "--tmux must not be empty"},
		{"accepts absolute hubPath and non-empty tmuxPath", "/abs/hub", "/usr/bin/tmux", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWatchdogFlags(tt.hubPath, tt.tmuxPath)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateWatchdogFlags(%q, %q) = %v, want nil", tt.hubPath, tt.tmuxPath, err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("validateWatchdogFlags(%q, %q) = %v, want %q", tt.hubPath, tt.tmuxPath, err, tt.wantErr)
			}
		})
	}
}

// watchdogDefaultTiming guards against a test-only default silently becoming production's cadence, mirroring the coverage internal/reedengine/watchloop_test.go already gives its own default-timing constructor.
//
//testtiming:keep a guard that fires when watchdogDefaultTiming diverges from the production cadence constants, which no covering test asserts
func TestWatchdogDefaultTiming(t *testing.T) {
	t.Parallel()
	got := watchdogDefaultTiming()
	want := watchdogTiming{
		DiscoveryCycle:   watchdogHubDiscoveryCycle,
		DiscoveryCeiling: watchdogHubDiscoveryCeiling,
		StopFloor:        watchdogStopFloor,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
		ReapTimeout:      0,
	}
	if got != want {
		t.Errorf("watchdogDefaultTiming() = %+v, want %+v", got, want)
	}
	// A periodic wake below one second is the idle wake-up cost the daemon's cadence floor rules out.
	for name, period := range map[string]time.Duration{"DiscoveryCycle": got.DiscoveryCycle, "DiscoveryCeiling": got.DiscoveryCeiling, "StopFloor": got.StopFloor} {
		if period < time.Second {
			t.Errorf("watchdogDefaultTiming().%s = %v, want at least one second", name, period)
		}
	}
}

// TestDiscoveryWake pins the wake planner over scripted cycles and signals: the wait doubles to the ceiling across unchanged listings (order aside) and returns to the base on a changed set or a signal, and a signal runs its cycle once the base has passed since the last one, at once when it already has.
// A signal carrying a stop request waits only for the stop floor, whichever signal arrives first, and the cycle that serves it restores the base.
func TestDiscoveryWake(t *testing.T) {
	t.Parallel()
	const base, ceiling, stopFloor = 5 * time.Second, 20 * time.Second, time.Second
	type step struct {
		at     time.Duration
		signal bool
		stop   bool
		live   []string
		want   time.Duration
	}
	backedOff := []step{
		{at: 5 * time.Second, live: []string{"a"}, want: base},
		{at: 10 * time.Second, live: []string{"a"}, want: 10 * time.Second},
		{at: 20 * time.Second, live: []string{"a"}, want: ceiling},
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"doubles to the ceiling and stays there", append(slices.Clone(backedOff), step{at: 40 * time.Second, live: []string{"a"}, want: ceiling})},
		{"a changed set returns to the base", append(slices.Clone(backedOff), step{at: 40 * time.Second, live: []string{"a", "b"}, want: base})},
		{"the same set in another order is unchanged", []step{
			{at: 5 * time.Second, live: []string{"a", "b"}, want: base},
			{at: 10 * time.Second, live: []string{"b", "a"}, want: 10 * time.Second},
		}},
		{"a signal within the base waits for the base to pass and resets the cadence", append(slices.Clone(backedOff),
			step{at: 21 * time.Second, signal: true, want: 4 * time.Second},
			step{at: 25 * time.Second, live: []string{"a"}, want: 10 * time.Second},
		)},
		{"a signal after the base runs at once", append(slices.Clone(backedOff), step{at: 100 * time.Second, signal: true, want: 0})},
		{"a stop shortly after a cycle waits for the stop floor", []step{
			{at: 300 * time.Millisecond, signal: true, stop: true, want: 700 * time.Millisecond},
		}},
		{"a stop after the stop floor runs at once", []step{
			{at: 2 * time.Second, signal: true, stop: true, want: 0},
		}},
		{"a stop then an ordinary signal still waits only for the stop floor", []step{
			{at: 300 * time.Millisecond, signal: true, stop: true, want: 700 * time.Millisecond},
			{at: 400 * time.Millisecond, signal: true, want: 600 * time.Millisecond},
		}},
		{"an ordinary signal then a stop waits only for the stop floor", []step{
			{at: 300 * time.Millisecond, signal: true, want: 4700 * time.Millisecond},
			{at: 400 * time.Millisecond, signal: true, stop: true, want: 600 * time.Millisecond},
		}},
		{"a cycle after a stop restores the base floor", []step{
			{at: 300 * time.Millisecond, signal: true, stop: true, want: 700 * time.Millisecond},
			{at: time.Second, live: []string{"a"}, want: base},
			{at: 1500 * time.Millisecond, signal: true, want: 4500 * time.Millisecond},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			wake := &discoveryWake{base: base, ceiling: ceiling, stopFloor: stopFloor, cadence: base, lastCycle: start}
			var previous []string
			for i, s := range tt.steps {
				now := start.Add(s.at)
				if s.signal {
					wake.onSignal(now, s.stop)
				} else {
					wake.afterCycle(now, !sameSessionSet(s.live, previous))
					previous = s.live
				}
				if got := wake.next(now); got != s.want {
					t.Errorf("step %d at %v: next = %v, want %v", i, s.at, got, s.want)
				}
			}
		})
	}
}

// TestConsumeDiscoverSignal pins that a present signal file is consumed and removed, and reports a stop only for the stop marker; an absent file answers false, false and leaves nothing; a read or removal failure is an error with both results false.
func TestConsumeDiscoverSignal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		setup        func(t *testing.T, dir string) string
		wantConsumed bool
		wantStop     bool
		wantErr      bool
		wantRemoved  bool
	}{
		{
			name: "an empty file is a discover",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "empty.signal")
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatalf("write signal: %v", err)
				}
				return path
			},
			wantConsumed: true,
			wantRemoved:  true,
		},
		{
			name: "the stop marker is a stop",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "stop.signal")
				if err := os.WriteFile(path, []byte(reedengine.DiscoverStopMarker), 0o644); err != nil {
					t.Fatalf("write signal: %v", err)
				}
				return path
			},
			wantConsumed: true,
			wantStop:     true,
			wantRemoved:  true,
		},
		{
			name: "an absent file is not consumed",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "absent.signal")
			},
			wantRemoved: true,
		},
		{
			name: "a directory with a child fails the read",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "unreadable")
				if err := os.MkdirAll(filepath.Join(path, "child"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return path
			},
			wantErr: true,
		},
		{
			name: "a readable file in a read-only directory fails the removal",
			setup: func(t *testing.T, dir string) string {
				if runtime.GOOS == "windows" || os.Geteuid() == 0 {
					t.Skip("removal from a read-only directory succeeds on Windows and as root")
				}
				locked := filepath.Join(dir, "locked")
				if err := os.Mkdir(locked, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				path := filepath.Join(locked, "stop.signal")
				if err := os.WriteFile(path, []byte(reedengine.DiscoverStopMarker), 0o644); err != nil {
					t.Fatalf("write signal: %v", err)
				}
				if err := os.Chmod(locked, 0o555); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
				return path
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setup(t, t.TempDir())
			consumed, stop, err := consumeDiscoverSignal(path)
			if (err != nil) != tt.wantErr || consumed != tt.wantConsumed || stop != tt.wantStop {
				t.Errorf("consumeDiscoverSignal = %v, %v, %v; want %v, %v, error=%v", consumed, stop, err, tt.wantConsumed, tt.wantStop, tt.wantErr)
			}
			_, statErr := os.Stat(path)
			if removed := errors.Is(statErr, fs.ErrNotExist); removed != tt.wantRemoved {
				t.Errorf("signal path removed = %v, want %v (stat error %v)", removed, tt.wantRemoved, statErr)
			}
		})
	}
}

// TestExeReplaced pins the comparison over real files: only a baseline whose file is replaced or rewritten answers true,
// and a missing baseline or a failed stat never does.
func TestExeReplaced(t *testing.T) {
	t.Parallel()
	statOf := func(t *testing.T, path string) os.FileInfo {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		return info
	}
	tests := []struct {
		name string
		// outcome returns the start, now and statErr the comparison is called with, over files it creates under dir.
		outcome func(t *testing.T, dir string) (start, now os.FileInfo, statErr error)
		want    bool
	}{
		{
			name: "the same unchanged file",
			outcome: func(t *testing.T, dir string) (os.FileInfo, os.FileInfo, error) {
				path := filepath.Join(dir, "lyx")
				if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
					t.Fatalf("write: %v", err)
				}
				return statOf(t, path), statOf(t, path), nil
			},
			want: false,
		},
		{
			name: "a stat error",
			outcome: func(t *testing.T, dir string) (os.FileInfo, os.FileInfo, error) {
				path := filepath.Join(dir, "lyx")
				if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
					t.Fatalf("write: %v", err)
				}
				return statOf(t, path), nil, errors.New("binary missing mid-build")
			},
			want: false,
		},
		{
			name: "a nil baseline",
			outcome: func(t *testing.T, dir string) (os.FileInfo, os.FileInfo, error) {
				path := filepath.Join(dir, "lyx")
				if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
					t.Fatalf("write: %v", err)
				}
				return nil, statOf(t, path), nil
			},
			want: false,
		},
		{
			name: "a different file",
			outcome: func(t *testing.T, dir string) (os.FileInfo, os.FileInfo, error) {
				first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
				for _, path := range []string{first, second} {
					if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
						t.Fatalf("write: %v", err)
					}
				}
				return statOf(t, first), statOf(t, second), nil
			},
			want: true,
		},
		{
			name: "the same file with its modification time moved",
			outcome: func(t *testing.T, dir string) (os.FileInfo, os.FileInfo, error) {
				path := filepath.Join(dir, "lyx")
				if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
					t.Fatalf("write: %v", err)
				}
				start := statOf(t, path)
				later := start.ModTime().Add(time.Hour)
				if err := os.Chtimes(path, later, later); err != nil {
					t.Fatalf("chtimes: %v", err)
				}
				return start, statOf(t, path), nil
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, now, statErr := tt.outcome(t, t.TempDir())
			if got := exeReplaced(start, now, statErr); got != tt.want {
				t.Errorf("exeReplaced = %v, want %v", got, tt.want)
			}
		})
	}
}

// listerEnv is what a scripted session lister sees on one call.
type listerEnv struct {
	// call is the 1-based number of this call.
	call int
	// goneCall is the number of the first call that found the stop marker consumed, 0 while none has.
	goneCall int
	// writeStop writes the stop marker onto the discover signal path, the way Engine.writeDiscoverStop does.
	writeStop func()
	cancel    context.CancelFunc
}

// TestWatchdogCmd_ExitsZeroBeforeOrInsteadOfLooping is the table of watchdogCmd's exit paths.
// Losing the lock race returns nil, respawns nothing and writes no trace file at the default verbosity.
// A replaced binary with a live hub releases the lock before it respawns and passes the hub, tmux and shell values and suppress true under go test.
// A replaced binary, an idle-cycle exit or a stop respawns only when the re-check after the loop still finds a session, and a cancelled context respawns nothing.
// Each row asserts its own trace lines.
// It points the process-global durable log sink at the hub's logs directory, so it is not parallel.
func TestWatchdogCmd_ExitsZeroBeforeOrInsteadOfLooping(t *testing.T) {
	const tmuxPath, shellPath = "/fixed/tmux", "/fixed/shell"
	const manyCycles = 1000000
	var idle, live = []string(nil), []string{"ghost"}
	type respawnCall struct {
		hub, tmux, shell string
		suppress         bool
		lockWasFree      bool
	}
	tests := []struct {
		name        string
		holdLock    bool
		replacedExe bool
		idleCycles  int
		// lister answers each session listing; nil answers idle.
		lister func(env listerEnv) []string
		// wantRunning has the test wait for the stop-woken cycle and two more lister calls, assert the command is still running, then cancel it.
		wantRunning  bool
		wantRespawns int
		// wantNoCallsAfterGone asserts the lister is never called after the call that found the marker consumed.
		wantNoCallsAfterGone bool
		wantTrace            []string
		wantNoTrace          []string
	}{
		{name: "lock already held", holdLock: true, idleCycles: watchdogHubIdleCycles},
		{
			name: "binary replaced, every listing affirmative", replacedExe: true, idleCycles: watchdogHubIdleCycles,
			lister:       func(listerEnv) []string { return live },
			wantRespawns: 1,
			wantTrace:    []string{"reed: watchdog retired"},
		},
		{
			name: "idle-cycle exit", idleCycles: watchdogHubIdleCycles,
			wantNoTrace: []string{"successor was spawned"},
			wantTrace:   []string{"exiting after consecutive idle discovery cycles"},
		},
		{
			name: "idle-cycle exit, re-check affirmative", idleCycles: watchdogHubIdleCycles,
			lister: func(env listerEnv) []string {
				if env.call > watchdogHubIdleCycles {
					return live
				}
				return idle
			},
			wantRespawns: 1,
			wantTrace:    []string{"exiting after consecutive idle discovery cycles", "a successor was spawned for a live hub"},
		},
		{
			name: "stop with an idle listing", idleCycles: manyCycles,
			lister: func(env listerEnv) []string {
				if env.call == 1 {
					env.writeStop()
				}
				return idle
			},
			wantTrace:   []string{"exiting on a stop request"},
			wantNoTrace: []string{"successor was spawned", "exiting after consecutive idle"},
		},
		{
			name: "stop with an affirmative listing", idleCycles: manyCycles,
			lister: func(env listerEnv) []string {
				if env.call == 1 {
					env.writeStop()
				}
				return live
			},
			wantRunning: true,
			wantNoTrace: []string{"successor was spawned", "exiting on a stop request"},
		},
		{
			name: "stop with an idle listing and a cancelled context", idleCycles: manyCycles,
			lister: func(env listerEnv) []string {
				switch {
				case env.call == 1:
					env.writeStop()
					return idle
				case env.goneCall == 0:
					return idle
				case env.call == env.goneCall:
					env.cancel()
					return idle
				default:
					return live
				}
			},
			wantNoCallsAfterGone: true,
			wantTrace:            []string{"exiting on a stop request"},
			wantNoTrace:          []string{"successor was spawned"},
		},
		{
			name: "binary replaced, idle cycle listing and idle re-check", replacedExe: true, idleCycles: manyCycles,
			wantTrace:   []string{"retires on a replaced binary with no successor"},
			wantNoTrace: []string{"watchdog retired", "successor was spawned"},
		},
		{
			name: "binary replaced, idle cycle listing and affirmative re-check", replacedExe: true, idleCycles: manyCycles,
			lister: func(env listerEnv) []string {
				if env.call > 1 {
					return live
				}
				return idle
			},
			wantRespawns: 1,
			wantTrace:    []string{"retires on a replaced binary with no successor", "a successor was spawned for a live hub"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			t.Cleanup(func() {
				logger.SetDurableSinkDir("")
				logger.SetVerbosity(0)
			})
			scratch := fabricengine.HubScratchDir(hub)
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatalf("mkdir scratch: %v", err)
			}
			lockPath := filepath.Join(scratch, watchdogLockFileName)
			signalPath := filepath.Join(scratch, reedengine.DiscoverSignalFileName)
			if tt.holdLock {
				held, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err != nil || !acquired {
					t.Fatalf("hold the watchdog lock: acquired=%v err=%v", acquired, err)
				}
				t.Cleanup(func() { held.Release() })
			}

			firstBinary, secondBinary := filepath.Join(hub, "first"), filepath.Join(hub, "second")
			for _, path := range []string{firstBinary, secondBinary} {
				if err := os.WriteFile(path, []byte("a"), 0o755); err != nil {
					t.Fatalf("write binary: %v", err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stats := 0
			var respawns []respawnCall
			var calls, goneCall atomic.Int32
			var stopWritten atomic.Bool
			env := listerEnv{
				cancel: cancel,
				writeStop: func() {
					if err := fsx.AtomicWriteBytes(signalPath, []byte(reedengine.DiscoverStopMarker)); err != nil {
						t.Errorf("write the stop marker: %v", err)
					}
					stopWritten.Store(true)
				},
			}
			timing := watchdogTiming{DiscoveryCycle: time.Millisecond, DiscoveryCeiling: time.Millisecond, StopFloor: time.Millisecond, IdleCycles: tt.idleCycles, OrphanGoneCycles: 1000}
			c := &reedCLI{
				watchdogTiming: &timing,
				watchdogExeStat: func() (os.FileInfo, error) {
					stats++
					if stats == 1 || !tt.replacedExe {
						return os.Stat(firstBinary)
					}
					return os.Stat(secondBinary)
				},
				respawnWatchdog: func(hubPath, tmux, shell string, suppress bool) {
					probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
					if err == nil && acquired {
						probe.Release()
					}
					respawns = append(respawns, respawnCall{hub: hubPath, tmux: tmux, shell: shell, suppress: suppress, lockWasFree: err == nil && acquired})
				},
				watchdogListSessions: func(string, string) ([]string, error) {
					call := env
					call.call = int(calls.Add(1))
					if _, err := os.Stat(signalPath); stopWritten.Load() && errors.Is(err, fs.ErrNotExist) {
						goneCall.CompareAndSwap(0, int32(call.call))
					}
					call.goneCall = int(goneCall.Load())
					if tt.lister == nil {
						return idle, nil
					}
					return tt.lister(call), nil
				},
			}
			cmd := c.watchdogCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs([]string{"--hub-path", hub, "--tmux", tmuxPath, "--shell", shellPath})
			done := make(chan error, 1)
			go func() { done <- cmd.ExecuteContext(ctx) }()

			if tt.wantRunning {
				deadline := time.Now().Add(10 * time.Second)
				for gone := goneCall.Load(); gone == 0 || calls.Load() < gone+2; gone = goneCall.Load() {
					if time.Now().After(deadline) {
						t.Fatalf("lister calls = %d, marker consumed at call %d; want the command to run on past a consumed stop", calls.Load(), gone)
					}
					time.Sleep(5 * time.Millisecond)
				}
				select {
				case err := <-done:
					t.Fatalf("watchdogCmd returned %v after a stop with an affirmative listing, want it still running", err)
				default:
				}
				cancel()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("watchdogCmd returned %v, want nil", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("watchdogCmd did not return within 10s")
			}

			if len(respawns) != tt.wantRespawns {
				t.Fatalf("respawn calls = %d, want %d", len(respawns), tt.wantRespawns)
			}
			if tt.wantRespawns > 0 {
				want := respawnCall{hub: hub, tmux: tmuxPath, shell: shellPath, suppress: true, lockWasFree: true}
				if respawns[0] != want {
					t.Errorf("respawn call = %+v, want %+v", respawns[0], want)
				}
			}
			if tt.wantNoCallsAfterGone && calls.Load() != goneCall.Load() {
				t.Errorf("lister calls = %d, want none after the call that found the marker consumed (%d)", calls.Load(), goneCall.Load())
			}
			var trace strings.Builder
			entries, err := os.ReadDir(fabricengine.HubLogsDir(hub))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("read the hub logs directory: %v", err)
			}
			for _, entry := range entries {
				body, err := os.ReadFile(filepath.Join(fabricengine.HubLogsDir(hub), entry.Name()))
				if err != nil {
					t.Fatalf("read trace file: %v", err)
				}
				trace.Write(body)
			}
			if tt.holdLock && len(entries) != 0 {
				t.Errorf("hub logs directory holds %d files, want none at the default verbosity", len(entries))
			}
			for _, want := range tt.wantTrace {
				if !strings.Contains(trace.String(), want) {
					t.Errorf("trace = %q, want it to hold %q", trace.String(), want)
				}
			}
			for _, unwanted := range tt.wantNoTrace {
				if strings.Contains(trace.String(), unwanted) {
					t.Errorf("trace = %q, want it to omit %q", trace.String(), unwanted)
				}
			}
			if tt.wantRunning {
				if _, err := os.Stat(signalPath); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("stop marker after the run: stat error = %v, want it consumed", err)
				}
			}
		})
	}
}
