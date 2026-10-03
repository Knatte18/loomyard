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

func TestPlanSessionDiff(t *testing.T) {
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

func TestSessionsAreIdle(t *testing.T) {
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
			got := sessionsAreIdle(tt.names, tt.err)
			if got != tt.want {
				t.Errorf("sessionsAreIdle(%v, %v) = %v; want %v", tt.names, tt.err, got, tt.want)
			}
		})
	}
}

// reapCycle is one planReapCycle call and the state it must leave behind. A nil inFlight means no
// name is in flight, and a nil wantCounters means the counter map ends empty. wantReap and
// wantRemaining are compared after sorting, since planReapCycle returns them in live order.
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

// worktreeRootGoneFixture builds a t.TempDir()-rooted fixture with a missing path, a plain-file
// path, and a directory path, for worktreeRootGone/hubIsLiveDir tests.
func worktreeRootGoneFixture(t *testing.T) (missing, plainFile, dir string) {
	t.Helper()
	root := t.TempDir()
	missing = filepath.Join(root, "missing")
	plainFile = filepath.Join(root, "plain-file")
	if err := os.WriteFile(plainFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dir = filepath.Join(root, "a-directory")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	return missing, plainFile, dir
}

func TestWorktreeRootGone(t *testing.T) {
	missing, plainFile, dir := worktreeRootGoneFixture(t)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"missing path is gone", missing, true},
		{"plain file is gone", plainFile, true},
		{"directory is not gone", dir, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worktreeRootGone(tt.path); got != tt.want {
				t.Errorf("worktreeRootGone(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHubIsLiveDir(t *testing.T) {
	missing, plainFile, dir := worktreeRootGoneFixture(t)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"existing directory is live", dir, true},
		{"missing path is not live", missing, false},
		{"plain file is not live", plainFile, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hubIsLiveDir(tt.path); got != tt.want {
				t.Errorf("hubIsLiveDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestWorktreeRootGoneAndHubIsLiveDir_StatErrorIsConservativeForBoth pins the one case where
// treating an unreadable path as gone would destroy live work: a stat that fails with neither a
// not-exist result nor success (the EACCES shape) must answer false — conservative — from both
// predicates, and deliberately not each other's negation.
//
// Skipped on Windows, where directory mode bits do not deny traversal this way, and skipped when the
// test runs as uid 0, where mode bits are not enforced at all — in both cases the stat would succeed
// and the assertion would pass for the wrong reason.
func TestWorktreeRootGoneAndHubIsLiveDir_StatErrorIsConservativeForBoth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not deny traversal on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("mode bits are not enforced for uid 0")
	}

	root := t.TempDir()
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

	if got := worktreeRootGone(target); got {
		t.Errorf("worktreeRootGone(%q) = true, want false (conservative) for a stat error that is not not-exist", target)
	}
	if got := hubIsLiveDir(target); got {
		t.Errorf("hubIsLiveDir(%q) = true, want false (conservative) for a stat error", target)
	}
}

// validateWatchdogFlags is asserted here rather than through watchdogCmd's RunE deliberately — a
// CLI-level test of the accepting case would fall through the pre-flight into a global logger
// mutation, a lock acquisition under a scratch directory the command never creates, and then the
// discovery loop, whose first tick shells out to tmux via the os/exec package and is forbidden in
// an untagged file.
func TestValidateWatchdogFlags(t *testing.T) {
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

// watchdogDefaultTiming guards against a test-only default silently becoming production's cadence,
// mirroring the coverage internal/reedengine/watchloop_test.go already gives its own default-timing
// constructor.
func TestWatchdogDefaultTiming(t *testing.T) {
	got := watchdogDefaultTiming()
	want := watchdogTiming{
		DiscoveryCycle:   watchdogHubDiscoveryCycle,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
	}
	if got != want {
		t.Errorf("watchdogDefaultTiming() = %+v, want %+v", got, want)
	}
}
