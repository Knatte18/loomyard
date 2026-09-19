// watchdog_test.go pins the watchdog daemon's pure seams — planSessionDiff, sessionsAreIdle,
// planReapCycle, worktreeRootGone, hubIsLiveDir, validateWatchdogFlags and watchdogDefaultTiming —
// against no tmux server and no filesystem at all beyond t.TempDir(), table-driven.

package reedcli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
			if !equalStringSlices(gotAppeared, tt.wantAppeared) {
				t.Errorf("planSessionDiff() appeared = %v; want %v", gotAppeared, tt.wantAppeared)
			}
			if !equalStringSlices(gotDeparted, tt.wantDeparted) {
				t.Errorf("planSessionDiff() departed = %v; want %v", gotDeparted, tt.wantDeparted)
			}
		})
	}
}

// equalStringSlices treats a nil slice and an empty slice as equal, since planSessionDiff never
// distinguishes "no names" from "an empty allocated slice of names".
func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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

// TestPlanReapCycle_LiveDirectoryNeverReaps pins that a name never marked gone never reaps, and its
// counter entry stays at zero (i.e. absent from counters) across repeated cycles.
func TestPlanReapCycle_LiveDirectoryNeverReaps(t *testing.T) {
	counters := map[string]int{}
	for i := 0; i < 5; i++ {
		reap, remaining := planReapCycle([]string{"alpha"}, true, map[string]bool{}, counters, map[string]bool{}, 3)
		if len(reap) != 0 {
			t.Fatalf("cycle %d: reap = %v, want none", i, reap)
		}
		if !equalStringSlices(remaining, []string{"alpha"}) {
			t.Fatalf("cycle %d: remaining = %v, want [alpha]", i, remaining)
		}
		if got, ok := counters["alpha"]; ok && got != 0 {
			t.Fatalf("cycle %d: counters[alpha] = %d, want 0 or absent", i, got)
		}
	}
}

// TestPlanReapCycle_MissingFewerThanThresholdCyclesDoesNotReapYet pins that a name gone for fewer
// than threshold consecutive cycles is not reaped, with its counter advancing each cycle.
func TestPlanReapCycle_MissingFewerThanThresholdCyclesDoesNotReapYet(t *testing.T) {
	counters := map[string]int{}
	threshold := 3
	for i := 1; i < threshold; i++ {
		reap, remaining := planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
		if len(reap) != 0 {
			t.Fatalf("cycle %d: reap = %v, want none before threshold", i, reap)
		}
		if !equalStringSlices(remaining, []string{"alpha"}) {
			t.Fatalf("cycle %d: remaining = %v, want [alpha]", i, remaining)
		}
		if counters["alpha"] != i {
			t.Fatalf("cycle %d: counters[alpha] = %d, want %d", i, counters["alpha"], i)
		}
	}
}

// TestPlanReapCycle_MissingExactlyThresholdCyclesReaps pins that a name gone for exactly threshold
// consecutive cycles reaps, and its counter entry is deleted rather than left at threshold.
func TestPlanReapCycle_MissingExactlyThresholdCyclesReaps(t *testing.T) {
	counters := map[string]int{}
	threshold := 3
	var reap, remaining []string
	for i := 1; i <= threshold; i++ {
		reap, remaining = planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	}
	if !equalStringSlices(reap, []string{"alpha"}) {
		t.Errorf("reap = %v, want [alpha] at the threshold cycle", reap)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %v, want none at the threshold cycle", remaining)
	}
	if _, ok := counters["alpha"]; ok {
		t.Errorf("counters[alpha] still present = %d, want deleted after the reap", counters["alpha"])
	}
}

// TestPlanReapCycle_MissingThenPresentThenMissingResetsCounter pins that a present cycle in the
// middle of a gone streak resets the counter, so a subsequent missing streak shorter than threshold
// does not reap.
func TestPlanReapCycle_MissingThenPresentThenMissingResetsCounter(t *testing.T) {
	counters := map[string]int{}
	threshold := 3

	// Two missing cycles, short of threshold.
	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	if counters["alpha"] != 2 {
		t.Fatalf("counters[alpha] after two missing cycles = %d, want 2", counters["alpha"])
	}

	// One present cycle resets it.
	_, remaining := planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": false}, counters, map[string]bool{}, threshold)
	if !equalStringSlices(remaining, []string{"alpha"}) {
		t.Fatalf("remaining after the present cycle = %v, want [alpha]", remaining)
	}
	if _, ok := counters["alpha"]; ok {
		t.Fatalf("counters[alpha] present after reset = %d, want deleted", counters["alpha"])
	}

	// Two more missing cycles: still short of a fresh threshold, so no reap fires.
	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	reap, _ := planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	if len(reap) != 0 {
		t.Errorf("reap = %v, want none: the reset means only 2 consecutive gone cycles have accrued", reap)
	}
}

// TestPlanReapCycle_NameLeavingLiveListPrunesItsCounter pins that a name absent from live has its
// counter entry pruned, so its return starts from zero.
func TestPlanReapCycle_NameLeavingLiveListPrunesItsCounter(t *testing.T) {
	counters := map[string]int{}
	threshold := 3

	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	if counters["alpha"] != 2 {
		t.Fatalf("counters[alpha] = %d, want 2 before it leaves the live list", counters["alpha"])
	}

	// alpha leaves the live list entirely.
	planReapCycle([]string{}, true, map[string]bool{}, counters, map[string]bool{}, threshold)
	if _, ok := counters["alpha"]; ok {
		t.Fatalf("counters[alpha] = %d, want pruned once it left the live list", counters["alpha"])
	}

	// alpha returns: it starts from zero, not from where it left off.
	planReapCycle([]string{"alpha"}, true, map[string]bool{"alpha": true}, counters, map[string]bool{}, threshold)
	if counters["alpha"] != 1 {
		t.Errorf("counters[alpha] on return = %d, want 1 (starting from zero)", counters["alpha"])
	}
}

// TestPlanReapCycle_SeveralSessionsProgressIndependently pins that several sessions on one hub
// progress independently in the same cycle: one live, one gone-but-short-of-threshold, one at
// threshold.
func TestPlanReapCycle_SeveralSessionsProgressIndependently(t *testing.T) {
	counters := map[string]int{"beta": 1, "gamma": 2}
	threshold := 3
	live := []string{"alpha", "beta", "gamma"}
	gone := map[string]bool{"alpha": false, "beta": true, "gamma": true}

	reap, remaining := planReapCycle(live, true, gone, counters, map[string]bool{}, threshold)
	sort.Strings(reap)
	sort.Strings(remaining)

	if !equalStringSlices(reap, []string{"gamma"}) {
		t.Errorf("reap = %v, want [gamma]", reap)
	}
	if !equalStringSlices(remaining, []string{"alpha", "beta"}) {
		t.Errorf("remaining = %v, want [alpha beta]", remaining)
	}
	if counters["beta"] != 2 {
		t.Errorf("counters[beta] = %d, want 2", counters["beta"])
	}
	if _, ok := counters["gamma"]; ok {
		t.Errorf("counters[gamma] still present, want deleted after its reap")
	}
	if _, ok := counters["alpha"]; ok {
		t.Errorf("counters[alpha] present, want absent since it is live")
	}
}

// TestPlanReapCycle_CounterMapHygiene pins that counters does not grow unboundedly across a sequence
// of cycles whose live set churns: only names currently live (and previously observed gone at least
// once) ever occupy a slot.
func TestPlanReapCycle_CounterMapHygiene(t *testing.T) {
	counters := map[string]int{}
	threshold := 3

	for i := 0; i < 50; i++ {
		name := "churner"
		planReapCycle([]string{name}, true, map[string]bool{name: true}, counters, map[string]bool{}, threshold)
		planReapCycle([]string{}, true, map[string]bool{}, counters, map[string]bool{}, threshold)
	}
	if len(counters) != 0 {
		t.Errorf("counters = %v, want empty: a churning single name must never accumulate stale entries", counters)
	}
}

// TestPlanReapCycle_HubProbeRefusesToAct pins the-hub-itself-is-probed-before-the-reap-pass: with
// hubLive false and a listing naming several names all marked gone, planReapCycle reaps none of
// them, leaves every counter untouched, and returns every name as remaining except those in
// inFlight.
func TestPlanReapCycle_HubProbeRefusesToAct(t *testing.T) {
	counters := map[string]int{"alpha": 2, "beta": 1}
	before := map[string]int{"alpha": 2, "beta": 1}
	live := []string{"alpha", "beta", "gamma"}
	gone := map[string]bool{"alpha": true, "beta": true, "gamma": true}
	inFlight := map[string]bool{"gamma": true}

	reap, remaining := planReapCycle(live, false, gone, counters, inFlight, 3)
	sort.Strings(remaining)

	if len(reap) != 0 {
		t.Errorf("reap = %v, want none while the hub is not proven live", reap)
	}
	if !equalStringSlices(remaining, []string{"alpha", "beta"}) {
		t.Errorf("remaining = %v, want [alpha beta] (gamma excluded via inFlight)", remaining)
	}
	for name, want := range before {
		if counters[name] != want {
			t.Errorf("counters[%s] = %d, want unchanged at %d", name, counters[name], want)
		}
	}
}

// TestPlanReapCycle_InFlightExcludedFromBothReturns pins that a name in inFlight appears in neither
// return value, on both the hubLive true and false branches — not reaped again, and not passed to
// planSessionDiff, so it can never read as appeared while its reap is still running.
func TestPlanReapCycle_InFlightExcludedFromBothReturns(t *testing.T) {
	for _, hubLive := range []bool{true, false} {
		t.Run(map[bool]string{true: "hubLive", false: "hubGone"}[hubLive], func(t *testing.T) {
			counters := map[string]int{}
			live := []string{"alpha", "beta"}
			gone := map[string]bool{"alpha": true, "beta": false}
			inFlight := map[string]bool{"alpha": true}

			reap, remaining := planReapCycle(live, hubLive, gone, counters, inFlight, 3)
			for _, n := range reap {
				if n == "alpha" {
					t.Errorf("reap = %v, want alpha excluded (in-flight)", reap)
				}
			}
			for _, n := range remaining {
				if n == "alpha" {
					t.Errorf("remaining = %v, want alpha excluded (in-flight)", remaining)
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

// TestWorktreeRootGone pins the per-name proven-gone predicate: missing -> gone; a plain file ->
// gone; a directory -> not gone.
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

// TestHubIsLiveDir pins the hub-probe proven-live predicate: a directory -> live; a missing path ->
// not live; a plain file -> not live.
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

// TestValidateWatchdogFlags pins validateWatchdogFlags as a pure function: an empty or relative
// hubPath is rejected, an empty tmuxPath is rejected, and an absolute hubPath with a non-empty
// tmuxPath is accepted.
//
// This is asserted here rather than through watchdogCmd's RunE deliberately — a CLI-level test of
// the accepting case would fall through the pre-flight into a global logger mutation, a lock
// acquisition under a scratch directory the command never creates, and then the discovery loop,
// whose first tick shells out to tmux via the os/exec package and is forbidden in an untagged file.
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

// TestWatchdogDefaultTiming pins that watchdogDefaultTiming returns exactly the three package
// constants — the guard against a test-only default silently becoming production's cadence,
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
