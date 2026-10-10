// resources.go implements the -resources mode: it runs one package at a time through `go test` inside a gate slot and measures what each costs in wall time, CPU, peak memory and processes, and which processes it leaves behind.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// resourceRow is the measured cost of one package.
type resourceRow struct {
	pkg string
	// wall is the time from the child's start to its exit.
	wall time.Duration
	// cpu is the CPU time the child and its descendants used.
	cpu time.Duration
	// peakBytes is the cgroup's peak memory, or under rusage the largest single process's max RSS.
	peakBytes int64
	// rusage marks a row whose CPU and memory come from the child's rusage rather than a cgroup.
	rusage bool
	// procs is the system-wide process count started across the child; procsKnown is false when /proc/stat was unreadable.
	procs      int64
	procsKnown bool
	// leftovers lists every process still referencing the package's temp directory after the child exited, as "pid argv".
	leftovers []string
	// census counts the git processes the package ran, split on the fixture marker.
	census  gitCensus
	failed  bool
	noTests bool
}

// cgroupPollInterval is how often a running scope's cgroup counters are sampled; the scope vanishes with its last process, so the last sample is the reading.
const cgroupPollInterval = 50 * time.Millisecond

// cgroupDiscoveryWait bounds how long a started child may take to move into its scope.
const cgroupDiscoveryWait = 2 * time.Second

// cgroupRoot is where the unified cgroup hierarchy is mounted.
const cgroupRoot = "/sys/fs/cgroup"

// runResources measures every package matching pkgFlag, one at a time, inside one gate slot, and prints the resource table.
// It returns an error when a package failed.
func runResources(tags, pkgFlag string) error {
	patterns := strings.Split(pkgFlag, ",")
	packages, err := listPackages(tags, patterns)
	if err != nil {
		return err
	}

	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return fmt.Errorf("read the working directory: %w", err)
	}
	baseEnv, cleanupLyx, err := prebuildLyx(tags)
	if err != nil {
		return err
	}
	defer cleanupLyx()
	env, parallel, release, err := takeGateSlot(context.Background(), cwd, baseEnv)
	if err != nil {
		return err
	}
	defer release()

	tier := "Tier 1 (offline)"
	if tags != "" {
		tier = "Tags: " + tags
	}

	loadBefore := readLoadAverage()
	rows := make([]resourceRow, 0, len(packages))
	for _, importPath := range packages {
		// The prefix stays short because a tmux socket path under the child's TMPDIR must fit sun_path.
		tmpDir, err := os.MkdirTemp("", "ttr-*")
		if err != nil {
			return fmt.Errorf("create a temp directory for %s: %w", importPath, err)
		}
		row, err := measurePackageResources(tags, importPath, tmpDir, env, parallel)
		os.RemoveAll(tmpDir)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	loadAfter := readLoadAverage()

	fmt.Print(renderResources(tier, rows, loadBefore, loadAfter, systemdScopeUsable()))
	fmt.Print("\n", renderCensus(rows))

	var failed []string
	for _, row := range rows {
		if row.failed {
			failed = append(failed, shortPkg(row.pkg))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("go test reported failures in %d package(s): %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// takeGateSlot returns the environment and -p cap one measured run uses: baseEnv, plus the slot's variables inside a hub.
// Inside a hub it holds one slot of the hub's pool until release and writes no wait record; outside a hub it returns the template's cap with no slot.
func takeGateSlot(ctx context.Context, cwd string, baseEnv []string) (env []string, parallel int, release func(), err error) {
	location, err := lyxcwd.ResolveWorktree(cwd)
	if errors.Is(err, lyxcwd.ErrNotAGitRepo) || (err == nil && !preflight.BoardLyxPresent(location)) {
		template, err := gateslot.TemplateConfig()
		if err != nil {
			return nil, 0, nil, fmt.Errorf("read the gate template: %w", err)
		}
		return baseEnv, template.GoParallel, func() {}, nil
	}
	if err != nil {
		return nil, 0, nil, fmt.Errorf("resolve the worktree of %s: %w", cwd, err)
	}

	pool := hubgeom.GateSlots(location)
	limits, err := pool.Limits()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("read the hub's gate limits: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, limits.CLIWait)
	defer cancel()
	lease, err := pool.Acquire(waitCtx, gateslot.Holder{Worktree: location.WorktreePath(), Site: "testtiming -resources"})
	if err != nil {
		return nil, 0, nil, fmt.Errorf("acquire a gate slot: %w", err)
	}
	release = func() {
		if err := lease.Release(); err != nil {
			fmt.Fprintln(os.Stderr, "testtiming: release the gate slot:", err)
		}
	}
	return lease.Env(baseEnv), limits.GoParallel, release, nil
}

// measurePackageResources runs `go test` over importPath with its temp directory pointed at tmpDir and returns what the run cost.
// CPU and peak memory come from the child's systemd scope cgroup when systemd-run is on PATH and the scope is readable, and from the child's rusage otherwise.
func measurePackageResources(tags, importPath, tmpDir string, env []string, parallel int) (resourceRow, error) {
	goArgs := []string{"test", "-json", "-count=1", "-p", strconv.Itoa(parallel)}
	if tags != "" {
		goArgs = append(goArgs, "-tags", tags)
	}
	goArgs = append(goArgs, importPath)

	name, args := "go", goArgs
	scoped := systemdScopeUsable()
	if scoped {
		name, args = "systemd-run", append([]string{"--user", "--scope", "--quiet", "--", "go"}, goArgs...)
	}
	cmd := exec.Command(name, args...)
	traceDir, err := os.MkdirTemp("", "ttt-*")
	if err != nil {
		return resourceRow{}, fmt.Errorf("create a trace directory for %s: %w", importPath, err)
	}
	defer os.RemoveAll(traceDir)

	cmd.Env = append(append([]string(nil), env...), "TMPDIR="+tmpDir, "TMP="+tmpDir, "TEMP="+tmpDir)
	cmd.Env = append(cmd.Env, traceEnv(traceDir)...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return resourceRow{}, fmt.Errorf("pipe stdout of %s: %w", importPath, err)
	}

	forksBefore, forksKnownBefore := readForkCount()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return resourceRow{}, fmt.Errorf("start %s for %s: %w", name, importPath, err)
	}

	sampler := newScopeSampler(cmd.Process.Pid, scoped)
	pkgs := map[string]*pkgResult{}
	var tests []testResult
	var transcript strings.Builder
	reader := bufio.NewReader(stdout)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			parseLine(line, pkgs, &tests)
			var event testEvent
			if json.Unmarshal(line, &event) == nil && event.Action == "output" {
				transcript.WriteString(event.Output)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				sampler.stop()
				return resourceRow{}, fmt.Errorf("read go test output of %s: %w", importPath, readErr)
			}
			break
		}
	}
	waitErr := cmd.Wait()
	wall := time.Since(start)
	cpu, peak, sampled := sampler.stop()

	row := resourceRow{pkg: importPath, wall: wall, failed: waitErr != nil}
	if result := pkgs[importPath]; result != nil {
		row.failed = row.failed || result.action == "fail"
		row.noTests = result.noTests
	}
	if row.failed {
		fmt.Fprintf(os.Stderr, "testtiming: %s failed; go test output:\n%s", importPath, transcript.String())
	}
	if sampled {
		row.cpu, row.peakBytes = cpu, peak
	} else {
		row.rusage = true
		row.cpu, row.peakBytes = rusageStats(cmd.ProcessState)
	}
	if forksAfter, known := readForkCount(); known && forksKnownBefore {
		row.procs, row.procsKnown = forksAfter-forksBefore, true
	}
	row.leftovers = leftoverProcesses(tmpDir)
	if row.census, err = readCensus(traceDir); err != nil {
		return resourceRow{}, fmt.Errorf("census of %s: %w", importPath, err)
	}
	return row, nil
}

// rusageStats returns the CPU time of a finished process and its waited descendants, and the largest max RSS among them in bytes, zero where the platform does not report it.
func rusageStats(state *os.ProcessState) (cpu time.Duration, maxRSS int64) {
	return state.UserTime() + state.SystemTime(), maxRSSBytes(state)
}

var (
	systemdScopeOnce   sync.Once
	systemdScopeResult bool
)

// systemdScopeUsable reports whether `systemd-run --user --scope` is on PATH and can start a command here, probed once per process.
func systemdScopeUsable() bool {
	systemdScopeOnce.Do(func() {
		if _, err := exec.LookPath("systemd-run"); err != nil {
			return
		}
		systemdScopeResult = exec.Command("systemd-run", "--user", "--scope", "--quiet", "--", "true").Run() == nil
	})
	return systemdScopeResult
}

// scopeSampler polls a started child's cgroup scope until stopped, keeping the last reading.
type scopeSampler struct {
	done chan struct{}
	wg   sync.WaitGroup

	cpu     time.Duration
	peak    int64
	sampled bool
}

// newScopeSampler starts sampling pid's scope when scoped is true; an unscoped sampler reports nothing sampled.
func newScopeSampler(pid int, scoped bool) *scopeSampler {
	s := &scopeSampler{done: make(chan struct{})}
	if !scoped {
		return s
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		dir := discoverScopeDir(pid, s.done)
		if dir == "" {
			return
		}
		for {
			if cpu, peak, err := scopeStats(dir); err == nil {
				s.cpu, s.peak, s.sampled = cpu, peak, true
			}
			select {
			case <-s.done:
				return
			case <-time.After(cgroupPollInterval):
			}
		}
	}()
	return s
}

// stop ends the sampling and returns the last reading and whether any was taken.
func (s *scopeSampler) stop() (cpu time.Duration, peakBytes int64, sampled bool) {
	close(s.done)
	s.wg.Wait()
	return s.cpu, s.peak, s.sampled
}

// discoverScopeDir waits for pid to move into its systemd scope and returns the scope's cgroup directory, or "" when it never does or done closes first.
func discoverScopeDir(pid int, done <-chan struct{}) string {
	deadline := time.Now().Add(cgroupDiscoveryWait)
	for time.Now().Before(deadline) {
		if content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup")); err == nil {
			if path := scopeCgroupPath(string(content)); path != "" {
				return filepath.Join(cgroupRoot, path)
			}
		}
		select {
		case <-done:
			return ""
		case <-time.After(10 * time.Millisecond):
		}
	}
	return ""
}

// scopeCgroupPath returns the cgroup path of a /proc/<pid>/cgroup listing that names a transient `.scope` unit, or "" when none does.
func scopeCgroupPath(listing string) string {
	for _, line := range strings.Split(listing, "\n") {
		_, path, found := strings.Cut(line, "::")
		if found && strings.HasSuffix(path, ".scope") && strings.Contains(path, "/run-") {
			return path
		}
	}
	return ""
}

// scopeStats reads a scope cgroup's CPU time and peak memory.
func scopeStats(cgroupDir string) (cpu time.Duration, peakBytes int64, err error) {
	stat, err := os.ReadFile(filepath.Join(cgroupDir, "cpu.stat"))
	if err != nil {
		return 0, 0, err
	}
	if cpu, err = parseCPUStat(string(stat)); err != nil {
		return 0, 0, err
	}
	peak, err := os.ReadFile(filepath.Join(cgroupDir, "memory.peak"))
	if err != nil {
		return 0, 0, err
	}
	peakBytes, err = strconv.ParseInt(strings.TrimSpace(string(peak)), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse memory.peak: %w", err)
	}
	return cpu, peakBytes, nil
}

// parseCPUStat returns the CPU time of a cgroup's cpu.stat text, its `usage_usec` line.
func parseCPUStat(stat string) (time.Duration, error) {
	for _, line := range strings.Split(stat, "\n") {
		key, value, found := strings.Cut(line, " ")
		if !found || key != "usage_usec" {
			continue
		}
		micros, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse usage_usec: %w", err)
		}
		return time.Duration(micros) * time.Microsecond, nil
	}
	return 0, errors.New("cpu.stat has no usage_usec line")
}

// readForkCount returns the system-wide count of processes created since boot, and whether /proc/stat was readable.
func readForkCount() (int64, bool) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	return parseForkCount(string(stat))
}

// parseForkCount returns the value of the `processes` line of /proc/stat text.
func parseForkCount(stat string) (int64, bool) {
	for _, line := range strings.Split(stat, "\n") {
		key, value, found := strings.Cut(line, " ")
		if !found || key != "processes" {
			continue
		}
		count, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return count, err == nil
	}
	return 0, false
}

// readLoadAverage returns the one-minute load average from /proc/loadavg, or "n/a" when it is unreadable.
func readLoadAverage() string {
	content, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "n/a"
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return "n/a"
	}
	return fields[0]
}

// leftoverProcesses lists every process whose cwd, executable or an argv element references dir, as "pid argv".
// The current process is never listed.
func leftoverProcesses(dir string) []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var leftovers []string
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		base := filepath.Join("/proc", entry.Name())
		cwd, _ := os.Readlink(filepath.Join(base, "cwd"))
		exe, _ := os.Readlink(filepath.Join(base, "exe"))
		var argv []string
		if cmdline, err := os.ReadFile(filepath.Join(base, "cmdline")); err == nil {
			argv = strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		}
		if referencesDir(dir, cwd, exe, argv) {
			leftovers = append(leftovers, fmt.Sprintf("%d %s", pid, strings.Join(argv, " ")))
		}
	}
	return leftovers
}

// referencesDir reports whether a process with this cwd, executable and argv references dir.
func referencesDir(dir, cwd, exe string, argv []string) bool {
	if dir == "" {
		return false
	}
	if strings.Contains(cwd, dir) || strings.Contains(exe, dir) {
		return true
	}
	for _, arg := range argv {
		if strings.Contains(arg, dir) {
			return true
		}
	}
	return false
}

// renderResources formats the per-package resource table with its trailer.
func renderResources(tier string, rows []resourceRow, loadBefore, loadAfter string, cgroups bool) string {
	ordered := append([]resourceRow(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].wall > ordered[j].wall })

	var out strings.Builder
	fmt.Fprintf(&out, "Resources  —  %s\n\n", tier)
	fmt.Fprintf(&out, "%-40s  %8s  %8s  %9s  %6s  %8s  %11s  %8s\n", "PACKAGE", "WALL", "CPU", "PEAK_MEM", "PROCS", "LEFTOVER", "GIT_FIXTURE", "GIT_CODE")
	fmt.Fprintf(&out, "%-40s  %8s  %8s  %9s  %6s  %8s  %11s  %8s\n", strings.Repeat("-", 40), "--------", "--------", "---------", "------", "--------", "-----------", "--------")

	var wall, cpu time.Duration
	var procs int64
	var gitFixture, gitCode int
	var leftoverLines []string
	for _, row := range ordered {
		wall += row.wall
		cpu += row.cpu
		procs += row.procs
		gitFixture += row.census.fixture
		gitCode += row.census.code
		marks := ""
		if row.rusage {
			marks += "  rusage"
		}
		if row.failed {
			marks += "  FAIL"
		}
		fmt.Fprintf(&out, "%-40s  %8s  %8s  %9s  %6s  %8d  %11d  %8d%s\n",
			shortPkg(row.pkg), formatSeconds(row.wall), formatSeconds(row.cpu), formatMegabytes(row.peakBytes), formatProcs(row), len(row.leftovers), row.census.fixture, row.census.code, marks)
		for _, leftover := range row.leftovers {
			leftoverLines = append(leftoverLines, fmt.Sprintf("  %s: %s", shortPkg(row.pkg), leftover))
		}
	}
	fmt.Fprintf(&out, "%-40s  %8s  %8s  %9s  %6d  %8s  %11d  %8d\n", "TOTAL", formatSeconds(wall), formatSeconds(cpu), "", procs, "", gitFixture, gitCode)

	if len(leftoverLines) > 0 {
		out.WriteString("\nLeftover processes (pid argv):\n")
		for _, line := range leftoverLines {
			out.WriteString(line + "\n")
		}
	}

	out.WriteString("\n")
	if cgroups {
		out.WriteString("CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.\n")
	} else {
		out.WriteString("CPU and PEAK_MEM: the go test child's rusage; PEAK_MEM is the largest single process's max RSS, not the sum.\n")
	}
	out.WriteString("A row marked rusage fell back to rusage because its scope could not be read.\n")
	out.WriteString("PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.\n")
	fmt.Fprintf(&out, "Load average (1 min): %s at start, %s at end. A report reads \"quiet machine\" as a precondition.\n", loadBefore, loadAfter)
	return out.String()
}

// formatSeconds formats d as seconds with two decimals.
func formatSeconds(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// formatMegabytes formats a byte count in mebibytes, blank for zero.
func formatMegabytes(bytes int64) string {
	if bytes == 0 {
		return ""
	}
	return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
}

// formatProcs formats a row's process count, blank when it was unknown.
func formatProcs(row resourceRow) string {
	if !row.procsKnown {
		return ""
	}
	return strconv.FormatInt(row.procs, 10)
}
