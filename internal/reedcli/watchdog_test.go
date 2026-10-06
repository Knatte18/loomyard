// watchdog_test.go pins the watchdog daemon's pure seams — planSessionDiff, sessionsAreIdle,
// planReapCycle, worktreeRootGone, hubIsLiveDir, validateWatchdogFlags and watchdogDefaultTiming —
// against no tmux server and no filesystem at all beyond t.TempDir(), table-driven.

package reedcli

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"testing"
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
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
		ReapTimeout:      0,
	}
	if got != want {
		t.Errorf("watchdogDefaultTiming() = %+v, want %+v", got, want)
	}
}
