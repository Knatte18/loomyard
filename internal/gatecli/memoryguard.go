// memoryguard.go samples the memory of the Go test binaries under a process and kills a runaway one whole.
// It names the killed binary's package directory and the tests it was told to run.
// It tells a memory limit other load filled apart from the run's own growth, naming the other holders.

package gatecli

import (
	"cmp"
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

// pressureOwnShareDivisor sets when a pressure kill is a slice-pressure kill:
// the processes of the guarding gate call hold less than the limit's use divided by this.
// The rest of the use then lies with other processes, which no change to the code under test frees.
const pressureOwnShareDivisor = 2

// pressureHoldersShown caps the other holders of memory a slice-pressure kill's line names.
const pressureHoldersShown = 3

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
	// own returns every process of the calling gate call, those outside the guarded tree included.
	own func() ([]proc.ProcessSample, error)
	// pressure reads the memory limit the calling process sits under; found is false where none is set.
	pressure func() (limit proc.MemoryLimit, found bool)
	// holders reads every process under a cgroup with the memory it holds resident.
	holders func(cgroup string) ([]proc.ProcessSample, error)
	// describe reads a process's working directory and command line; ok is false once it is gone.
	describe func(pid int) (process proc.Process, ok bool)
	// kill sends SIGKILL to pid.
	kill func(pid int) error
	// onKill, when set, is called with the pid of each test binary the guard is about to kill.
	// It is also called with the pid of each test binary that holds a killed one, before any SIGKILL goes out.
	onKill func(pid int)
	// foreign, when set, names the processes another guard watches; a test binary below one is left to that guard.
	foreign func(pid int) bool
	after   func(time.Duration) <-chan time.Time
	// out receives the line of each kill as it happens; nil prints none.
	out io.Writer
}

// newMemoryGuard returns the production memory guard over trees past limit bytes, printing each kill's line to out.
// It samples /proc, reads the cgroup memory limit and its processes, kills with SIGKILL and waits on the real clock.
func newMemoryGuard(limit int64, out io.Writer) memoryGuard {
	return memoryGuard{
		limit:    limit,
		floor:    pressureKillFloor,
		period:   memoryGuardPeriod,
		sample:   proc.TreeSample,
		own:      func() ([]proc.ProcessSample, error) { return proc.TreeSample(os.Getpid()) },
		pressure: proc.ReadMemoryLimit,
		holders:  proc.ResidentInCgroup,
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
	// slicePressure is set when the kill is a slice-pressure kill: other load filled the memory limit.
	slicePressure bool
	// enclosing are the pids of the Go test binaries that started the killed binary, directly or not.
	// Their own tests fail with the killed binary's death, so they count as killed too.
	enclosing []int
}

// guardKill is the line of one kill the guard made.
type guardKill struct {
	line string
	// slicePressure is set when other load filled the memory limit that drove the kill, so the kill judged no code.
	slicePressure bool
}

// run samples the tree under root once per period until ctx ends, and returns each kill.
// A test binary whose tree, itself and every process it started, holds more than limit is killed whole.
// When none does, the memory limit the calling process sits under is checked.
// With less than a tenth of its Max left, the largest tree holding at least floor is killed whole.
// That kill is a slice-pressure kill when the calling gate call's processes hold less than half the limit's use.
// Each binary is killed once, and a binary below a process foreign names is never killed.
// A platform that cannot sample ends run after one log line.
func (g memoryGuard) run(ctx context.Context, root int) []guardKill {
	logger.Info("gate: memory guard started", "limit_mb", g.limit>>20, "period", g.period)
	var kills []guardKill
	killed := map[int]bool{}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return kills
		case <-g.after(g.period):
		}
		tree, err := g.sample(root)
		if errors.Is(err, errors.ErrUnsupported) {
			logger.Info("gate: no memory guard applies on this platform")
			return kills
		}
		if err != nil {
			logger.Warn("gate: the memory guard could not sample the process tree", "cause", err)
			continue
		}
		trees := testTrees(tree)
		if g.foreign != nil {
			trees = withoutForeign(trees, tree, g.foreign)
		}
		for _, kill := range g.kills(trees, killed) {
			killed[kill.tree.binary.PID] = true
			kill.enclosing = enclosingTestBinaries(tree, kill.tree.binary)
			kills = append(kills, g.killTree(kill))
		}
	}
	return kills
}

// withoutForeign returns trees without each tree whose binary has an ancestor in sample that foreign names.
func withoutForeign(trees []testTree, sample []proc.ProcessSample, foreign func(pid int) bool) []testTree {
	parents := make(map[int]int, len(sample))
	for _, process := range sample {
		parents[process.PID] = process.Parent
	}
	return slices.DeleteFunc(trees, func(tree testTree) bool {
		seen := map[int]bool{tree.binary.PID: true}
		for pid := tree.binary.Parent; !seen[pid]; {
			if foreign(pid) {
				return true
			}
			seen[pid] = true
			parent, sampled := parents[pid]
			if !sampled {
				return false
			}
			pid = parent
		}
		return false
	})
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
// trees are the test binaries of one sample that the guard may kill.
// Every tree over limit is one, bar a tree that holds another tree over limit: the inner binary is the runaway.
// With none over limit and the memory limit short, the largest tree holding at least floor is the one.
// That kill is a slice-pressure kill when the calling gate call's processes hold less than half the limit's use.
// Its reason then names the other holders.
// A call whose processes cannot be sampled counts as holding the use itself.
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
			kills = append(kills, memoryKill{tree: tree, reason: fmt.Sprintf("it held %d MB, over %s's %d MB", tree.held>>20, keyName(memoryQuitMBKey), g.limit>>20)})
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
	headroom := max(limit.Headroom(), 0) >> 20
	ordinary := []memoryKill{{tree: *largest, reason: fmt.Sprintf("the memory limit of %s had %d MB left of %d MB, and it was the largest test binary at %d MB",
		limit.Cgroup, headroom, limit.Max>>20, largest.held>>20)}}
	call, err := g.own()
	if err != nil {
		logger.Warn("gate: the memory guard could not sample the gate call's processes, so the kill counts as the run's own", "cause", err)
		return ordinary
	}
	var own int64
	for _, process := range call {
		own += process.RSS
	}
	if own*pressureOwnShareDivisor >= limit.Used {
		return ordinary
	}
	reason := fmt.Sprintf("the memory limit of %s had %d MB left of %d MB, and this run's processes held only %d MB of the %d MB in use; %s; it was the largest test binary at %d MB",
		limit.Cgroup, headroom, limit.Max>>20, own>>20, limit.Used>>20, g.otherHolders(limit.Cgroup, call), largest.held>>20)
	return []memoryKill{{tree: *largest, reason: reason, slicePressure: true}}
}

// otherHolders returns the clause naming the processes under cgroup, outside call, that hold the most memory resident.
// It names at most pressureHoldersShown of them, largest first.
func (g memoryGuard) otherHolders(cgroup string, call []proc.ProcessSample) string {
	listed, err := g.holders(cgroup)
	if err != nil {
		logger.Warn("gate: the memory guard could not read the processes of the memory limit's cgroup", "cgroup", cgroup, "cause", err)
		return "the other holders of its memory could not be read"
	}
	own := make(map[int]bool, len(call))
	for _, process := range call {
		own[process.PID] = true
	}
	others := slices.DeleteFunc(slices.Clone(listed), func(process proc.ProcessSample) bool {
		return own[process.PID] || process.RSS == 0
	})
	if len(others) == 0 {
		return "no other process under it holds memory the guard could read"
	}
	slices.SortStableFunc(others, func(a, b proc.ProcessSample) int { return cmp.Compare(b.RSS, a.RSS) })
	named := make([]string, 0, pressureHoldersShown)
	for _, process := range others[:min(len(others), pressureHoldersShown)] {
		name := "a process whose executable could not be read"
		if process.Path != "" {
			name = filepath.Base(process.Path)
		}
		named = append(named, fmt.Sprintf("%s (pid %d) at %d MB", name, process.PID, process.RSS>>20))
	}
	return "its largest other holders are " + strings.Join(named, ", ")
}

// killTree reads the binary's package directory and test filter.
// It sends SIGKILL to every process of the tree.
// It returns the kill with the line naming it, which it also logs and prints to out.
// A process already gone is no failure.
func (g memoryGuard) killTree(kill memoryKill) guardKill {
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
	line := memoryGuardLine(filepath.Base(binary.Path), binary.PID, dir, filter, kill.reason, kill.slicePressure)
	logger.Warn("gate: the memory guard killed a test binary and every process it started", "binary", binary.Path, "pid", binary.PID, "dir", dir, "filter", filter, "held_mb", kill.tree.held>>20, "reason", kill.reason, "slice_pressure", kill.slicePressure)
	printLine(g.out, line)
	return guardKill{line: line, slicePressure: kill.slicePressure}
}

// memoryGuardLine is the line a memory guard kill prints and logs.
// dir is the binary's working directory, the package's own under go test, empty when it could not be read;
// filter is its -test.run value, empty when it was given none.
// A slice-pressure kill's line calls it an infrastructure stop and gives the way forward of waiting for the other load;
// any other kill's line says its tests fail and points at the test that holds the memory.
func memoryGuardLine(binary string, pid int, dir, filter, reason string, slicePressure bool) string {
	where := "of a package whose directory could not be read"
	if dir != "" {
		where = "of the package in " + dir
	}
	tests := ""
	if filter != "" {
		tests = ", running the tests -test.run=" + filter + " selects"
	}
	outcome := fmt.Sprintf("its tests fail; way forward: find the test that holds memory without bound, fix it and re-run the same command; "+
		"raise %s through \"lym config gate\" from the prime only for a package whose tests need that much", keyName(memoryQuitMBKey))
	if slicePressure {
		outcome = "an infrastructure stop, not a test failure: other load filled the memory limit, and no change to the code under test frees it; " +
			"way forward: wait for that load to end, then re-run the same command"
	}
	return fmt.Sprintf("lym gate: memory guard: killed the test binary %s (pid %d) %s%s, with every process it started: %s; %s",
		binary, pid, where, tests, reason, outcome)
}

// memoryGuardApplies decides whether the memory guard runs, and returns the reason it logs.
// It does not run with a limit of 0.
// It also does not run when enclosing names a live ancestor `lym gate test`.
// That ancestor's own guard already sees these test binaries.
// An empty, unparsable or non-ancestor enclosing value is ignored.
func memoryGuardApplies(limit int64, enclosing string, isAncestor func(pid int) bool) (bool, string) {
	if limit <= 0 {
		return false, keyName(memoryQuitMBKey) + " is 0"
	}
	if pid, err := strconv.Atoi(enclosing); err == nil && isAncestor(pid) {
		return false, fmt.Sprintf("an enclosing lym gate test (pid %d) already guards this run's memory", pid)
	}
	return true, keyName(memoryQuitMBKey) + " is set and no enclosing lym gate test guards this run"
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
