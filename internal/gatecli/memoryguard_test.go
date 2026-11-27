// memoryguard_test.go covers the memory guard's kill decisions over a scripted sampler and memory limit.
// It also covers the conditions under which the guard runs at all.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	guardLimit    = 4096 << 20
	guardFloor    = 1024 << 20
	guardTestDir  = "/work/internal/foo"
	guardFilter   = "^TestHog$"
	guardChildPID = 43
	innerPID      = 50
	otherPID      = 60
)

// guardTree is a tier's tree: a shell, the go command, the test binary and one child the binary started.
func guardTree(binaryRSS, childRSS int64) []proc.ProcessSample {
	return []proc.ProcessSample{
		{PID: 1, Path: "/bin/sh", RSS: 1 << 20},
		{PID: 2, Parent: 1, Path: nonTestBinary, RSS: 9000 << 20},
		{PID: testBinaryPID, Parent: 2, Path: testBinaryPath, RSS: binaryRSS},
		{PID: guardChildPID, Parent: testBinaryPID, Path: "/bin/sleep", RSS: childRSS},
	}
}

// guardRun is what one scripted run of the memory guard left behind.
type guardRun struct {
	killed []int
	lines  []string
	// pressureKills counts the kills the guard made as slice-pressure kills.
	pressureKills int
	out           string
	samples       int
}

// runGuardScripted runs a memory guard over script and returns once the script's last sample has been handled.
// limit is the memory limit the guard reads, and limited whether it finds one.
// holders answers the guard's read of the limit's cgroup.
// own answers the guard's read of the gate call's processes; nil answers with the tree last sampled.
// Only the test binary at testBinaryPID can be described.
func runGuardScripted(t *testing.T, script []scriptedSample, limit proc.MemoryLimit, limited bool, holders func(cgroup string) ([]proc.ProcessSample, error), own func() ([]proc.ProcessSample, error)) guardRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var result guardRun
	var out bytes.Buffer
	if own == nil {
		own = func() ([]proc.ProcessSample, error) { return script[result.samples-1].tree, nil }
	}
	g := memoryGuard{
		own:    own,
		limit:  guardLimit,
		floor:  guardFloor,
		period: time.Second,
		sample: func(int) ([]proc.ProcessSample, error) {
			next := script[result.samples]
			result.samples++
			if result.samples == len(script) {
				cancel()
			}
			return next.tree, next.err
		},
		pressure: func() (proc.MemoryLimit, bool) { return limit, limited },
		holders:  holders,
		describe: func(pid int) (proc.Process, bool) {
			process := proc.Process{PID: pid, Cwd: guardTestDir, Argv: []string{testBinaryPath, testRunFlagPrefix + guardFilter, "-test.v=true"}}
			return process, pid == testBinaryPID
		},
		kill: func(pid int) error {
			result.killed = append(result.killed, pid)
			return nil
		},
		after: func(time.Duration) <-chan time.Time {
			elapsed := make(chan time.Time, 1)
			elapsed <- time.Time{}
			return elapsed
		},
		out: &out,
	}

	for _, kill := range g.run(ctx, 1) {
		result.lines = append(result.lines, kill.line)
		if kill.slicePressure {
			result.pressureKills++
		}
	}
	result.out = out.String()
	return result
}

func TestMemoryGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	overCap := guardTree(3000<<20, 2000<<20)
	overLine := memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "it held 5000 MB, over gate.memory_quit_mb's 4096 MB", false)
	short := proc.MemoryLimit{Used: 16384<<20 - 512<<20, Max: 16384 << 20, Cgroup: "/user.slice"}
	ample := proc.MemoryLimit{Used: 8192 << 20, Max: 16384 << 20, Cgroup: "/user.slice"}
	// smallRun is a run of two test binaries holding 2700 MB together, under half of short's use.
	smallRun := []proc.ProcessSample{
		{PID: testBinaryPID, Parent: 1, Path: testBinaryPath, RSS: 1500 << 20},
		{PID: otherPID, Parent: 1, Path: longBinaryPath, RSS: 1200 << 20},
	}
	// siblingRun is smallRun with a binary the same gate call runs outside the guarded tree.
	siblingRun := func(rss int64) func() ([]proc.ProcessSample, error) {
		return func() ([]proc.ProcessSample, error) {
			return append(slices.Clone(smallRun), proc.ProcessSample{PID: innerPID, Parent: 1, Path: testBinaryPath, RSS: rss}), nil
		}
	}
	// otherLoad is the user slice's processes.
	// It holds two of the gate call's own binaries and processes that do not count.
	// It also holds large holders, one of them with an unreadable executable.
	otherLoad := func(string) ([]proc.ProcessSample, error) {
		return []proc.ProcessSample{
			{PID: 903, Path: "/usr/bin/bash", RSS: 4 << 20},
			{PID: innerPID, Path: testBinaryPath, RSS: 3500 << 20},
			{PID: 905, RSS: 2500 << 20},
			{PID: 901, Path: "/usr/bin/node", RSS: 3000 << 20},
			{PID: testBinaryPID, Path: testBinaryPath, RSS: 1500 << 20},
			{PID: 900, Path: "/usr/share/code/code", RSS: 6000 << 20},
			{PID: 904, Path: "/usr/bin/kworker", RSS: 0},
			{PID: 902, Path: "/usr/lib/firefox/firefox", RSS: 2000 << 20},
		}, nil
	}
	unreadable := func(string) ([]proc.ProcessSample, error) { return nil, errors.New("proc listing failed") }
	tests := []struct {
		name    string
		script  []scriptedSample
		limit   proc.MemoryLimit
		limited bool
		holders func(cgroup string) ([]proc.ProcessSample, error)
		// own answers the read of the gate call's processes; nil answers with the sampled tree.
		own               func() ([]proc.ProcessSample, error)
		wantKilled        []int
		wantLines         []string
		wantPressureKills int
		wantSamples       int
	}{
		{
			name:       "a tree over the cap is killed whole, once",
			script:     []scriptedSample{{tree: overCap}, {tree: overCap}},
			wantKilled: []int{testBinaryPID, guardChildPID},
			wantLines:  []string{overLine},
		},
		{
			name:   "a tree at the cap is left alone",
			script: []scriptedSample{{tree: guardTree(3000<<20, 1096<<20)}},
		},
		{
			name:   "a process that is not a test binary is never killed",
			script: []scriptedSample{{tree: guardTree(1<<20, 1<<20)[:2]}},
		},
		{
			name: "an inner test binary over the cap is killed, not the outer one",
			script: []scriptedSample{{tree: []proc.ProcessSample{
				{PID: testBinaryPID, Parent: 1, Path: testBinaryPath, RSS: 100 << 20},
				{PID: innerPID, Parent: testBinaryPID, Path: longBinaryPath, RSS: 5000 << 20},
			}}},
			wantKilled: []int{innerPID},
			wantLines:  []string{memoryGuardLine("averylongpackagenamethatpassesfifteen.test", innerPID, "", "", "it held 5000 MB, over gate.memory_quit_mb's 4096 MB", false)},
		},
		{
			name: "a short memory limit the run fills kills the largest tree at or above the floor",
			script: []scriptedSample{{tree: []proc.ProcessSample{
				{PID: 2, Parent: 1, Path: nonTestBinary, RSS: 6000 << 20},
				{PID: testBinaryPID, Parent: 2, Path: testBinaryPath, RSS: 1500 << 20},
				{PID: otherPID, Parent: 2, Path: longBinaryPath, RSS: 1200 << 20},
			}}},
			limit:      short,
			limited:    true,
			holders:    otherLoad,
			wantKilled: []int{testBinaryPID},
			wantLines:  []string{memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "the memory limit of /user.slice had 512 MB left of 16384 MB, and it was the largest test binary at 1500 MB", false)},
		},
		{
			name:       "a short memory limit other load fills kills the largest tree as an infrastructure stop naming the other holders",
			script:     []scriptedSample{{tree: smallRun}},
			limit:      short,
			limited:    true,
			holders:    otherLoad,
			own:        siblingRun(3500 << 20),
			wantKilled: []int{testBinaryPID},
			wantLines: []string{"lym gate: memory guard: killed the test binary pkg.test (pid 42) of the package in /work/internal/foo, running the tests -test.run=^TestHog$ selects, with every process it started: " +
				"the memory limit of /user.slice had 512 MB left of 16384 MB, and this run's processes held only 6200 MB of the 15872 MB in use; " +
				"its largest other holders are code (pid 900) at 6000 MB, node (pid 901) at 3000 MB, a process whose executable could not be read (pid 905) at 2500 MB; " +
				"it was the largest test binary at 1500 MB; " +
				"an infrastructure stop, not a test failure: other load filled the memory limit, and no change to the code under test frees it; " +
				"way forward: wait for that load to end, then re-run the same command"},
			wantPressureKills: 1,
		},
		{
			name:       "a gate call holding most of the use beside a small guarded tree is an ordinary pressure kill",
			script:     []scriptedSample{{tree: smallRun}},
			limit:      short,
			limited:    true,
			holders:    otherLoad,
			own:        siblingRun(6000 << 20),
			wantKilled: []int{testBinaryPID},
			wantLines:  []string{memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "the memory limit of /user.slice had 512 MB left of 16384 MB, and it was the largest test binary at 1500 MB", false)},
		},
		{
			name:       "a gate call whose processes cannot be sampled makes an ordinary pressure kill",
			script:     []scriptedSample{{tree: smallRun}},
			limit:      short,
			limited:    true,
			holders:    otherLoad,
			own:        func() ([]proc.ProcessSample, error) { return nil, errors.New("proc listing failed") },
			wantKilled: []int{testBinaryPID},
			wantLines:  []string{memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "the memory limit of /user.slice had 512 MB left of 16384 MB, and it was the largest test binary at 1500 MB", false)},
		},
		{
			name:              "a slice-pressure kill whose cgroup cannot be read says its other holders could not be read",
			script:            []scriptedSample{{tree: smallRun}},
			limit:             short,
			limited:           true,
			holders:           unreadable,
			wantKilled:        []int{testBinaryPID},
			wantLines:         []string{memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "the memory limit of /user.slice had 512 MB left of 16384 MB, and this run's processes held only 2700 MB of the 15872 MB in use; the other holders of its memory could not be read; it was the largest test binary at 1500 MB", true)},
			wantPressureKills: 1,
		},
		{
			name:    "a short memory limit spares a tree under the floor",
			script:  []scriptedSample{{tree: guardTree(900<<20, 0)}},
			limit:   short,
			limited: true,
		},
		{
			name:    "an ample memory limit kills nothing under the cap",
			script:  []scriptedSample{{tree: guardTree(3000<<20, 0)}},
			limit:   ample,
			limited: true,
		},
		{
			name:       "a sampler failure is logged and sampling goes on",
			script:     []scriptedSample{{err: errors.New("proc listing failed")}, {tree: overCap}},
			wantKilled: []int{testBinaryPID, guardChildPID},
			wantLines:  []string{overLine},
		},
		{
			name:        "an unsupported platform ends the guard after its first sample",
			script:      []scriptedSample{{err: errors.ErrUnsupported}, {tree: overCap}},
			wantSamples: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := runGuardScripted(t, tt.script, tt.limit, tt.limited, tt.holders, tt.own)
			if !slices.Equal(got.killed, tt.wantKilled) {
				t.Errorf("killed pids = %v; want %v", got.killed, tt.wantKilled)
			}
			if !slices.Equal(got.lines, tt.wantLines) {
				t.Errorf("lines = %q; want %q", got.lines, tt.wantLines)
			}
			if got.pressureKills != tt.wantPressureKills {
				t.Errorf("slice-pressure kills = %d; want %d", got.pressureKills, tt.wantPressureKills)
			}
			wantOut := ""
			if len(tt.wantLines) > 0 {
				wantOut = strings.Join(tt.wantLines, "\n") + "\n"
			}
			if got.out != wantOut {
				t.Errorf("output = %q; want %q", got.out, wantOut)
			}
			if tt.wantSamples != 0 && got.samples != tt.wantSamples {
				t.Errorf("sampler calls = %d; want %d", got.samples, tt.wantSamples)
			}
		})
	}
}

func TestMemoryGuardApplies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const ancestorPID = 77
	isAncestor := func(pid int) bool { return pid == ancestorPID }
	tests := []struct {
		name      string
		limit     int64
		enclosing string
		want      bool
	}{
		{name: "a limit of 0 turns it off", limit: 0, want: false},
		{name: "an enclosing run that is an ancestor turns it off", limit: guardLimit, enclosing: "77", want: false},
		{name: "an enclosing pid that is no ancestor leaves it on", limit: guardLimit, enclosing: "78", want: true},
		{name: "an empty enclosing value leaves it on", limit: guardLimit, want: true},
		{name: "an unparsable enclosing value leaves it on", limit: guardLimit, enclosing: "not-a-pid", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, reason := memoryGuardApplies(tt.limit, tt.enclosing, isAncestor)
			if got != tt.want || reason == "" {
				t.Errorf("memoryGuardApplies(%d, %q) = %v, %q; want %v and a reason", tt.limit, tt.enclosing, got, reason, tt.want)
			}
		})
	}
}
