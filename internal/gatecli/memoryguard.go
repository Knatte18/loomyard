// memoryguard.go samples the memory of the Go test binaries under a process and kills a runaway one whole.
// It names the killed binary's package directory and the tests it was told to run.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// memoryGuardPeriod is how often the memory guard samples its tree.
// A runaway test binary grows by a gigabyte or more a second, so the guard samples at the one-second floor.
const memoryGuardPeriod = time.Second

// pressureHeadroomDivisor sets the pressure trigger:
// the memory limit the guarding process sits under has less than its Max divided by this left.
const pressureHeadroomDivisor = 10

// pressureKillFloor is the least memory a test binary's tree holds for the pressure trigger to kill it.
// A smaller tree is not what drives the limit, so killing it would fail a healthy test and free little.
const pressureKillFloor = 1 << 30

// testRunFlagPrefix opens the argument that tells a test binary which tests to run.
const testRunFlagPrefix = "-test.run="

// memoryGuard kills the tree of a Go test binary that holds more than limit,
// or the largest one when the memory limit the calling process sits under runs short.
type memoryGuard struct {
	// limit is the bytes one test binary's tree may hold.
	limit int64
	// floor is the least bytes a tree holds for the pressure trigger to kill it.
	floor  int64
	period time.Duration
	// sample returns the process tree under root.
	sample func(root int) ([]proc.ProcessSample, error)
	// pressure reads the memory limit the calling process sits under; found is false where none is set.
	pressure func() (limit proc.MemoryLimit, found bool)
	// describe reads a process's working directory and command line; ok is false once it is gone.
	describe func(pid int) (process proc.Process, ok bool)
	// kill sends SIGKILL to pid.
	kill func(pid int) error
	// onKill, when set, is called with the pid of each test binary the guard is about to kill.
	// It is also called with the pid of each test binary that holds a killed one, before any SIGKILL goes out.
	onKill func(pid int)
	after  func(time.Duration) <-chan time.Time
	// out receives the line of each kill as it happens; nil prints none.
	out io.Writer
}

// newMemoryGuard returns the production memory guard over trees past limit bytes, printing each kill's line to out.
// It samples /proc, reads the cgroup memory limit, kills with SIGKILL and waits on the real clock.
func newMemoryGuard(limit int64, out io.Writer) memoryGuard {
	return memoryGuard{
		limit:    limit,
		floor:    pressureKillFloor,
		period:   memoryGuardPeriod,
		sample:   proc.TreeSample,
		pressure: proc.ReadMemoryLimit,
		describe: proc.ReadProcess,
		kill:     func(pid int) error { return proc.SignalProcess(pid, true) },
		after:    time.After,
		out:      out,
	}
}

// testTree is one Go test binary of a sample with every process it started.
type testTree struct {
	binary proc.ProcessSample
	// pids are the binary's pid followed by its descendants'.
	pids []int
	// held is the resident memory of the binary and its descendants together.
	held int64
}

// memoryKill is one tree the guard kills, with the reason its line gives.
type memoryKill struct {
	tree   testTree
	reason string
	// enclosing are the pids of the Go test binaries that started the killed binary, directly or not.
	// Their own tests fail with the killed binary's death, so they count as killed too.
	enclosing []int
}

// run samples the tree under root once per period until ctx ends, and returns the line of each kill.
// A test binary whose tree, itself and every process it started, holds more than limit is killed whole.
// When none does, the memory limit the calling process sits under is checked.
// With less than a tenth of its Max left, the largest tree holding at least floor is killed whole.
// Each binary is killed once.
// A platform that cannot sample ends run after one log line.
func (g memoryGuard) run(ctx context.Context, root int) []string {
	logger.Info("gate: memory guard started", "limit_mb", g.limit>>20, "period", g.period)
	var lines []string
	killed := map[int]bool{}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return lines
		case <-g.after(g.period):
		}
		tree, err := g.sample(root)
		if errors.Is(err, errors.ErrUnsupported) {
			logger.Info("gate: no memory guard applies on this platform")
			return lines
		}
		if err != nil {
			logger.Warn("gate: the memory guard could not sample the process tree", "cause", err)
			continue
		}
		for _, kill := range g.kills(testTrees(tree), killed) {
			killed[kill.tree.binary.PID] = true
			kill.enclosing = enclosingTestBinaries(tree, kill.tree.binary)
			lines = append(lines, g.killTree(kill))
		}
	}
	return lines
}

// enclosingTestBinaries returns the pids of the Go test binaries of tree that are ancestors of binary.
func enclosingTestBinaries(tree []proc.ProcessSample, binary proc.ProcessSample) []int {
	byPID := make(map[int]proc.ProcessSample, len(tree))
	for _, process := range tree {
		byPID[process.PID] = process
	}
	var enclosing []int
	visited := map[int]bool{binary.PID: true}
	for parent, ok := byPID[binary.Parent]; ok && !visited[parent.PID]; parent, ok = byPID[parent.Parent] {
		visited[parent.PID] = true
		if strings.HasSuffix(parent.Path, testBinarySuffix) {
			enclosing = append(enclosing, parent.PID)
		}
	}
	return enclosing
}

// testTrees returns each Go test binary of tree with its descendants and the memory they hold together.
func testTrees(tree []proc.ProcessSample) []testTree {
	children := map[int][]proc.ProcessSample{}
	for _, process := range tree {
		children[process.Parent] = append(children[process.Parent], process)
	}
	var trees []testTree
	for _, process := range tree {
		if !strings.HasSuffix(process.Path, testBinarySuffix) {
			continue
		}
		found := testTree{binary: process}
		pending := []proc.ProcessSample{process}
		for len(pending) > 0 {
			next := pending[0]
			pending = pending[1:]
			found.pids = append(found.pids, next.PID)
			found.held += next.RSS
			pending = append(pending, children[next.PID]...)
		}
		trees = append(trees, found)
	}
	return trees
}

// kills returns the trees of one sample to kill, leaving out a binary in killed.
// Every tree over limit is one, bar a tree that holds another tree over limit: the inner binary is the runaway.
// With none over limit and the memory limit short, the largest tree holding at least floor is the one.
func (g memoryGuard) kills(trees []testTree, killed map[int]bool) []memoryKill {
	var over []testTree
	for _, tree := range trees {
		if !killed[tree.binary.PID] && tree.held > g.limit {
			over = append(over, tree)
		}
	}
	var kills []memoryKill
	for _, tree := range over {
		holdsInner := false
		for _, other := range over {
			if other.binary.PID != tree.binary.PID && slices.Contains(tree.pids, other.binary.PID) {
				holdsInner = true
			}
		}
		if !holdsInner {
			kills = append(kills, memoryKill{tree: tree, reason: fmt.Sprintf("it held %d MB, over memory_quit_mb's %d MB", tree.held>>20, g.limit>>20)})
		}
	}
	if len(kills) > 0 {
		return kills
	}
	limit, found := g.pressure()
	if !found || limit.Headroom() >= limit.Max/pressureHeadroomDivisor {
		return nil
	}
	var largest *testTree
	for i, tree := range trees {
		if !killed[tree.binary.PID] && tree.held >= g.floor && (largest == nil || tree.held > largest.held) {
			largest = &trees[i]
		}
	}
	if largest == nil {
		return nil
	}
	reason := fmt.Sprintf("the memory limit of %s had %d MB left of %d MB, and it was the largest test binary at %d MB", limit.Cgroup, max(limit.Headroom(), 0)>>20, limit.Max>>20, largest.held>>20)
	return []memoryKill{{tree: *largest, reason: reason}}
}

// killTree reads the binary's package directory and test filter.
// It sends SIGKILL to every process of the tree.
// It returns the line naming the kill, which it also logs and prints to out.
// A process already gone is no failure.
func (g memoryGuard) killTree(kill memoryKill) string {
	binary := kill.tree.binary
	dir, filter := "", ""
	if process, ok := g.describe(binary.PID); ok {
		dir = process.Cwd
		for _, arg := range process.Argv {
			if value, found := strings.CutPrefix(arg, testRunFlagPrefix); found {
				filter = value
			}
		}
	}
	if g.onKill != nil {
		for _, pid := range append([]int{binary.PID}, kill.enclosing...) {
			g.onKill(pid)
		}
	}
	for _, pid := range kill.tree.pids {
		if err := g.kill(pid); err != nil {
			logger.Debug("gate: the memory guard could not kill a process", "pid", pid, "cause", err)
		}
	}
	line := memoryGuardLine(filepath.Base(binary.Path), binary.PID, dir, filter, kill.reason)
	logger.Warn("gate: the memory guard killed a test binary and every process it started", "binary", binary.Path, "pid", binary.PID, "dir", dir, "filter", filter, "held_mb", kill.tree.held>>20, "reason", kill.reason)
	printLine(g.out, line)
	return line
}

// memoryGuardLine is the line a memory guard kill prints and logs.
// dir is the binary's working directory, the package's own under go test, empty when it could not be read;
// filter is its -test.run value, empty when it was given none.
func memoryGuardLine(binary string, pid int, dir, filter, reason string) string {
	where := "of a package whose directory could not be read"
	if dir != "" {
		where = "of the package in " + dir
	}
	tests := ""
	if filter != "" {
		tests = ", running the tests -test.run=" + filter + " selects"
	}
	return fmt.Sprintf("lym gate: memory guard: killed the test binary %s (pid %d) %s%s, with every process it started: %s; "+
		"its tests fail; way forward: find the test that holds memory without bound, fix it and re-run the same command; "+
		"raise memory_quit_mb in the hub's gate.yaml through \"lym config gate\" from the prime only for a package whose tests need that much",
		binary, pid, where, tests, reason)
}

// memoryGuardApplies decides whether the memory guard runs, and returns the reason it logs.
// It does not run with a limit of 0.
// It also does not run when enclosing names a live ancestor `lym gate test`.
// That ancestor's own guard already sees these test binaries.
// An empty, unparsable or non-ancestor enclosing value is ignored.
func memoryGuardApplies(limit int64, enclosing string, isAncestor func(pid int) bool) (bool, string) {
	if limit <= 0 {
		return false, "memory_quit_mb is 0"
	}
	if pid, err := strconv.Atoi(enclosing); err == nil && isAncestor(pid) {
		return false, fmt.Sprintf("an enclosing lym gate test (pid %d) already guards this run's memory", pid)
	}
	return true, "memory_quit_mb is set and no enclosing lym gate test guards this run"
}

// guardOwnTests starts the memory guard over the test binaries the calling process starts, when the guard applies.
// limit is `memory_quit_mb` in bytes, and out receives the line of each kill.
// It returns the function that ends the guard and waits for its end,
// and the function that reports whether the guard killed the process with a given pid.
func guardOwnTests(ctx context.Context, limit int64, out io.Writer) (stop func(), guardKilled func(pid int) bool) {
	applies, reason := memoryGuardApplies(limit, os.Getenv(enclosingGateEnv), proc.IsAncestor)
	logger.Info("gate: memory guard decision", "applies", applies, "reason", reason)
	return startMemoryGuard(ctx, newMemoryGuard(limit, out), os.Getpid(), applies)
}

// startMemoryGuard runs guard over the process tree under root in the background when applies is set.
// It runs until stop is called or ctx ends.
// stop ends the guard and returns once its run has ended, and returns at once when no guard runs.
// guardKilled reports whether the guard killed the test binary with pid, or a test binary that pid's binary started.
// It is false for every pid when no guard runs.
// It reads each pid once and then forgets it, so a later process that reuses the pid reads as not killed.
// The guard notes a binary before it sends SIGKILL, so a caller that sees the binary end finds it noted.
func startMemoryGuard(ctx context.Context, guard memoryGuard, root int, applies bool) (stop func(), guardKilled func(pid int) bool) {
	if !applies {
		return func() {}, func(int) bool { return false }
	}
	var mu sync.Mutex
	killed := map[int]bool{}
	guard.onKill = func(pid int) {
		mu.Lock()
		killed[pid] = true
		mu.Unlock()
	}
	guardCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		guard.run(guardCtx, root)
	}()
	return func() {
			cancel()
			<-done
		}, func(pid int) bool {
			mu.Lock()
			defer mu.Unlock()
			was := killed[pid]
			delete(killed, pid)
			return was
		}
}
