//go:build integration || tmux

// smoke_helpers_test.go holds the fixtures and subprocess helpers shared by the integration-tier and tmux-tier smoke files:
// a real wired hub with one pair, the runner of the built cmd/lyx binary, the run seeds, and the bad-reed-config fixture.
// It spawns git and runs lyx, so it carries the disjunction of its two users' tags.

package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// runLoomCLINoFatal runs exe with args in dir as a real subprocess, bounded by timeout, and returns
// its combined stdout+stderr and exit code without ever calling a *testing.T method -- the pure seam
// case (h)'s concurrent invocations need, since t.Fatalf from a non-test goroutine is unsafe. Callers
// on the test's own goroutine that want fail-fast behaviour check the returned err themselves.
func runLoomCLINoFatal(exe, dir string, timeout time.Duration, args ...string) (stdout string, exitCode int, err error) {
	return lyxbin.Run(exe, dir, timeout, args...)
}

// newWiredPairFixture builds a real fabric hub, seeds every module's config template into it, and
// adds one worktree pair -- the shape every test in this suite needs before it can bootstrap loom
// against it. AddPair already writes and commits the pair's origin record (see the plan's
// origin-record-records-both-its-write-and-its-commit decision), so a caller never needs a --parent
// flag on its own first "loom start" call.
func newWiredPairFixture(t *testing.T) (h *hubforge.Hub, loc *lyxcwd.Location, worktree, slug string) {
	t.Helper()

	h = hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})

	slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree = h.PairCodeWorktree(slug)

	var err error
	loc, err = lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	return h, loc, worktree, slug
}

// providerlessShuttleConfig returns shuttle's shipped config template with two values overridden so
// no fixture in this suite can ever start a real provider session: the provider binary is a path
// that cannot exist, and the startup window is short enough that the resulting launch failure is
// classified within a test's own patience rather than ninety seconds later.
//
// This is a correction, not a convenience. The suite's own header used to assert that every fixture
// here "bounces before a real spawn", and the campaign's cost model was written on that claim. It
// stopped being true: a crucible round measured one real `claude` subprocess alive for the full
// thirty seconds of TestSmokeRunStandalone's first step, which is why that test
// timed out rather than merely being slow. Every test in this file is about bootstrap and driver
// mechanics -- locks, handshakes, strands, the status file -- and none of them is about what a
// provider does, so removing the provider makes the suite test what it claims to test AND makes the
// zero-subprocess claim true by construction instead of by hope.
func providerlessShuttleConfig() string {
	cfg := shuttleengine.ConfigTemplate()
	cfg = strings.Replace(cfg, "claude: ${env:LYX_SHUTTLE_CLAUDE:-}", "claude: /nonexistent/lyx-smoke-has-no-provider", 1)
	cfg = strings.Replace(cfg, "startup_timeout_s: 90", "startup_timeout_s: 2", 1)
	return cfg
}

// fastDeadlineLoomConfig returns loom's shipped config template with the discussion producer's
// deadline cut from eight hours to one minute.
//
// The deadline and provider overrides work as a pair with providerlessShuttleConfig above, and
// neither is sufficient alone. Removing the provider stops a real session from starting; it does
// not stop the wait. A launch that fails still leaves reed holding the pane, so shuttle's wait
// loop polls until the SPEC's own deadline -- and Discussion-Write's deadline comes from
// loom.yaml's discussion_timeout_min, which ships at 480 so an autonomous agent exploring a
// codebase is never cut off. In a fixture with no agent at all that is an eight-hour block on a
// thirty-second test.
func fastDeadlineLoomConfig() string {
	return strings.Replace(loomengine.ConfigTemplate(), "discussion_timeout_min: 480", "discussion_timeout_min: 1", 1)
}

// smokeStatusRel is the status file's relative path for the self run.
func smokeStatusRel(loc *lyxcwd.Location) string {
	return shedrun.StatusRel(loc, shedrun.SelfRunID)
}

// recordsHeadChangedFiles returns the paths HEAD's own commit changed, relative to dir's repository
// root.
func recordsHeadChangedFiles(t *testing.T, dir string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").Output()
	if err != nil {
		t.Fatalf("git -C %s diff-tree HEAD: %v", dir, err)
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
}

// seedGoDriverRun writes loc's own self-run seed.json directly to disk for the go driver, with
// Params agreeing byte-for-byte with what seedAndCommitBootstrap itself would write on the very next
// bootstrap against it (loomSeedFor) -- the same recorded.ParentBranch-matching shape
// smoke_driverstrand_test.go's own seedLLMDriver uses for the llm driver.
//
// It exists because arm.go's resolveRunID now refuses "run"/"step"/"status"/"pause" outright when no
// seed exists at the addressed run-id, before either verb's own bootstrap or absent-status-file
// checks ever run: several of this file's fixtures drive one of those four verbs directly against a
// pair whose STATUS FILE is deliberately still absent (to exercise the status-file-absent path
// itself), which now also needs the run's own seed.json present first -- exactly the state a seed
// written via "lyx shed seed" before any bootstrap has ever run would leave.
func seedGoDriverRun(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the go driver run: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{
		Recipe: shedrun.RecipeLoom,
		Driver: shedrun.DriverGo,
		Params: map[string]string{"parent": recorded.ParentBranch},
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(go driver): %v", err)
	}
}

// seedAndCommitStatus seeds loc's status file directly through the production Seed function and
// commits it records-side -- the same seed-then-commit shape "lyx loom start" itself performs, used here
// so a standalone-driver test does not need a live tmux server just to get a valid seeded pair.
func seedAndCommitStatus(t *testing.T, loc *lyxcwd.Location, slug string) {
	t.Helper()
	if err := loomshed.Seed(shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID), slug, "main"); err != nil {
		t.Fatalf("loomshed.Seed: %v", err)
	}
	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitRecordsPaths(rec, fabricengine.RecordsWorktree(loc), loc.AnchorRel, []string{smokeStatusRel(loc)}, "smoke: seed status", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit seed: %v", err)
	}
}

// seedLLMDriver writes loc's own self-run seed directly to disk with driver=llm, uncommitted --
// mirroring seedAndCommitStatus and
// TestSmokeBootstrapLifecycle's origin-record step's shape of driving a
// production primitive directly rather than through a CLI subprocess.
//
// The written seed's Params must agree byte-for-byte with the one seedAndCommitBootstrap's own step
// 1b (loomSeedFor) writes on the first "loom start" against it, since WriteSeed refuses a disagreeing
// existing seed outright rather than silently accepting the later write: recorded.ParentBranch is
// read from the origin record newWiredPairFixture-style callers already committed via
// hubforge.AddPair, so this seed's own params.parent matches what the bootstrap itself would compute.
func seedLLMDriver(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the llm driver: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{
		Recipe: shedrun.RecipeLoom,
		Driver: shedrun.DriverLLM,
		Params: map[string]string{"parent": recorded.ParentBranch},
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(llm driver): %v", err)
	}
}

// newBadReedUpFixture builds a hub whose reed config carries an invalid mouse value, so reed Up fails.
// It seeds the pair with seed and returns the resolved location and the worktree path.
func newBadReedUpFixture(t *testing.T, seed func(*testing.T, *lyxcwd.Location)) (*lyxcwd.Location, string) {
	t.Helper()
	reedCfg := reedengine.ConfigTemplate()
	var lines []string
	replaced := false
	for _, line := range strings.Split(reedCfg, "\n") {
		if strings.HasPrefix(line, "mouse:") {
			line = "mouse: not-a-mouse-value"
			replaced = true
		}
		lines = append(lines, line)
	}
	if !replaced {
		t.Fatalf("reed config template drift: no mouse: line found")
	}

	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"loom":    fastDeadlineLoomConfig(),
		"reed":    strings.Join(lines, "\n"),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)
	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	seed(t, loc)
	return loc, worktree
}

// hubStampPath returns the hub's build stamp file.
func hubStampPath(loc *lyxcwd.Location) string {
	return hubreconcile.Geometry{BoardDir: fabricengine.BoardDir(loc.HubPath)}.StampPath()
}

// commitRetiredBatcherKey commits a batcher.yaml carrying a retired key into loc's records worktree, the state a hub reconcile after a binary change removes.
func commitRetiredBatcherKey(t *testing.T, loc *lyxcwd.Location) {
	t.Helper()
	batcher, _ := configreg.Lookup("batcher")
	retired := strings.Replace(batcher.Template(), "orientation: 31400", "master_base: 52000", 1)
	gitkit.CommitFile(t, fabricengine.RecordsWorktree(loc), configengine.ConfigFileRel("batcher"), retired, "fixture: retired key")
}

// findWatchdogPIDs returns the pids of every live process whose argv contains an adjacent "reed"
// followed by "watchdog" pair AND a "--hub-path" argument equal to hubPath -- the /proc-native way to
// find the per-hub watchdog daemon a bootstrap spawned, mirroring findDriverPIDs' own shape and the
// same find-then-proc.KillPID pattern this file's cleanup already applies to driver pids. A lone
// "watchdog" argv element is not a sufficient discriminator, for the identical reason a lone "run" is
// not for the driver: "watchdog" is this verb's own unique name today, but matching on the adjacent
// verb pair plus the --hub-path value it was told is what keeps this scan specific to THIS fixture's
// own daemon rather than a sibling fixture's, since the daemon is a detached, per-hub process this
// test's own cwd does not identify the way a driver's cwd does. Linux only, mirroring
// findDriverPIDs; returns nil on any other GOOS.
func findWatchdogPIDs(hubPath string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		var isWatchdog, matchesHub bool
		for i, arg := range argv {
			if i+1 < len(argv) && arg == "reed" && argv[i+1] == "watchdog" {
				isWatchdog = true
			}
			if arg == "--hub-path" && i+1 < len(argv) && argv[i+1] == hubPath {
				matchesHub = true
			}
		}
		if isWatchdog && matchesHub {
			pids = append(pids, pid)
		}
	}
	return pids
}

// findDriverPIDs returns the pids of every live process whose current working directory is worktree
// and whose argv contains an adjacent "loom" followed by "run" pair -- the /proc-native way to find
// the detached "lyx loom run" process a bootstrap spawned, since it inherits its parent's cwd and its
// own argv is fixed. A lone "run" argv element is not a sufficient discriminator: "run" is also a verb
// on shuttle, burler, and webster, so it would over-match any such process sharing the worktree cwd;
// the adjacent-pair requirement is what makes this specific to the loom driver. Linux only, mirroring
// internal/reedcli's own /proc-native probes; returns nil on any other GOOS.
// A process whose parent also matches is dropped: between fork and exec the driver's own child,
// such as a git call, still carries the driver's argv and cwd, and is not a second driver.
func findDriverPIDs(worktree string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	parents := map[int]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err != nil || cwd != worktree {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "loom" && argv[i+1] == "run" {
				parents[pid] = parentPID(pid)
				break
			}
		}
	}
	var pids []int
	for pid, ppid := range parents {
		if _, parentMatches := parents[ppid]; !parentMatches {
			pids = append(pids, pid)
		}
	}
	return pids
}

// parentPID returns pid's parent pid from /proc/<pid>/stat, or 0 when it cannot be read.
// The comm field may hold spaces and parentheses, so the fields are counted from the last ')'.
func parentPID(pid int) int {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	rest := string(raw)
	if i := strings.LastIndex(rest, ")"); i >= 0 {
		rest = rest[i+1:]
	}
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}
