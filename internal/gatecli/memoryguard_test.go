// memoryguard_test.go covers the memory guard's kill decisions over a scripted sampler and memory limit.
// It also covers the conditions under which the guard runs at all.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	killed  []int
	lines   []string
	out     string
	samples int
}

// runGuardScripted runs a memory guard over script and returns once the script's last sample has been handled.
// limit is the memory limit the guard reads, and limited whether it finds one.
// foreign names the processes another guard watches.
// Only the test binary at testBinaryPID can be described.
func runGuardScripted(t *testing.T, script []scriptedSample, limit proc.MemoryLimit, limited bool, foreign func(pid int) bool) guardRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var result guardRun
	var out bytes.Buffer
	g := memoryGuard{
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
		describe: func(pid int) (proc.Process, bool) {
			process := proc.Process{PID: pid, Cwd: guardTestDir, Argv: []string{testBinaryPath, testRunFlagPrefix + guardFilter, "-test.v=true"}}
			return process, pid == testBinaryPID
		},
		kill: func(pid int) error {
			result.killed = append(result.killed, pid)
			return nil
		},
		foreign: foreign,
		after: func(time.Duration) <-chan time.Time {
			elapsed := make(chan time.Time, 1)
			elapsed <- time.Time{}
			return elapsed
		},
		out: &out,
	}
	result.lines = g.run(ctx, 1)
	result.out = out.String()
	return result
}

func TestMemoryGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	overCap := guardTree(3000<<20, 2000<<20)
	overLine := memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "it held 5000 MB, over gate.memory_quit_mb's 4096 MB")
	short := proc.MemoryLimit{Used: 16384<<20 - 512<<20, Max: 16384 << 20, Cgroup: "/user.slice"}
	ample := proc.MemoryLimit{Used: 8192 << 20, Max: 16384 << 20, Cgroup: "/user.slice"}
	tests := []struct {
		name    string
		script  []scriptedSample
		limit   proc.MemoryLimit
		limited bool
		// foreign are the running tier commands, whose test binaries the tier's own guard watches.
		foreign     []int
		wantKilled  []int
		wantLines   []string
		wantSamples int
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
			wantLines:  []string{memoryGuardLine("averylongpackagenamethatpassesfifteen.test", innerPID, "", "", "it held 5000 MB, over gate.memory_quit_mb's 4096 MB")},
		},
		{
			name: "a test binary below a running tier command is left to the tier's own guard",
			script: []scriptedSample{{tree: []proc.ProcessSample{
				{PID: 2, Parent: 1, Path: "/bin/sh", RSS: 1 << 20},
				{PID: otherPID, Parent: 2, Path: longBinaryPath, RSS: 5000 << 20},
				{PID: testBinaryPID, Parent: 1, Path: testBinaryPath, RSS: 5000 << 20},
			}}},
			foreign:    []int{2},
			wantKilled: []int{testBinaryPID},
			wantLines:  []string{overLine},
		},
		{
			name: "a short memory limit kills the largest tree at or above the floor",
			script: []scriptedSample{{tree: []proc.ProcessSample{
				{PID: testBinaryPID, Parent: 1, Path: testBinaryPath, RSS: 1500 << 20},
				{PID: otherPID, Parent: 1, Path: longBinaryPath, RSS: 1200 << 20},
			}}},
			limit:      short,
			limited:    true,
			wantKilled: []int{testBinaryPID},
			wantLines:  []string{memoryGuardLine("pkg.test", testBinaryPID, guardTestDir, guardFilter, "the memory limit of /user.slice had 512 MB left of 16384 MB, and it was the largest test binary at 1500 MB")},
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
			commands := &runningCommands{}
			for _, pid := range tt.foreign {
				commands.add(pid)
			}
			got := runGuardScripted(t, tt.script, tt.limit, tt.limited, commands.holds)
			if !slices.Equal(got.killed, tt.wantKilled) {
				t.Errorf("killed pids = %v; want %v", got.killed, tt.wantKilled)
			}
			if !slices.Equal(got.lines, tt.wantLines) {
				t.Errorf("lines = %q; want %q", got.lines, tt.wantLines)
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

// TestStartMemoryGuard pins what the guard's start hands its caller.
// A guard that does not apply samples nothing, and its stop returns at once.
// One that applies kills the runaway whole, and its stop ends the guard.
// Afterwards the killed binary's pid reads as killed once, and no other pid ever does.
func TestStartMemoryGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	nested := []proc.ProcessSample{
		{PID: testBinaryPID, Parent: 1, Path: testBinaryPath, RSS: 100 << 20},
		{PID: innerPID, Parent: testBinaryPID, Path: longBinaryPath, RSS: 5000 << 20},
	}
	tests := []struct {
		name    string
		applies bool
		tree    []proc.ProcessSample
		// wantKilled are the pids guardKilled reads as killed.
		wantKilled []int
	}{
		{name: "a guard that does not apply samples nothing and stops at once", applies: false, tree: guardTree(3000<<20, 2000<<20)},
		{name: "a guard that applies kills the runaway whole before its stop returns", applies: true, tree: guardTree(3000<<20, 2000<<20), wantKilled: []int{testBinaryPID}},
		{name: "an inner binary's kill notes the outer binary that started it too", applies: true, tree: nested, wantKilled: []int{innerPID, testBinaryPID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var samples atomic.Int32
			firstKill := make(chan struct{})
			var announce sync.Once
			guard := memoryGuard{
				limit:  guardLimit,
				floor:  guardFloor,
				period: time.Millisecond,
				sample: func(int) ([]proc.ProcessSample, error) {
					samples.Add(1)
					return tt.tree, nil
				},
				pressure: func() (proc.MemoryLimit, bool) { return proc.MemoryLimit{}, false },
				describe: func(int) (proc.Process, bool) { return proc.Process{}, false },
				kill: func(int) error {
					announce.Do(func() { close(firstKill) })
					return nil
				},
				after: time.After,
			}

			stop, guardKilled := startMemoryGuard(context.Background(), guard, 1, tt.applies)
			if tt.applies {
				<-firstKill
			}
			stop()

			if !tt.applies && samples.Load() != 0 {
				t.Errorf("a guard that does not apply sampled %d times; want 0", samples.Load())
			}
			for _, pid := range []int{testBinaryPID, innerPID, guardChildPID, otherPID} {
				want := slices.Contains(tt.wantKilled, pid)
				if got := guardKilled(pid); got != want {
					t.Errorf("guardKilled(%d) = %v after stop; want %v", pid, got, want)
				}
				if guardKilled(pid) {
					t.Errorf("guardKilled(%d) = true on the second read; want false, the pid forgotten once read", pid)
				}
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
