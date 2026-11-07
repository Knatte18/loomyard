// smoke_helpers_test.go holds the fixtures and subprocess helpers shared by the integration-tier and tmux-tier smoke files:
// a real wired hub with one pair, the runner of the built cmd/lym binary, the run seeds, and the bad-reed-config fixture.
// It spawns git and runs lym, so it carries the disjunction of its two users' tags.

package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// runLoomCLINoFatal runs exe with args in dir as a real subprocess, bounded by timeout.
// It returns the combined stdout+stderr and the exit code, and it never calls a *testing.T method,
// so it is safe on a non-test goroutine, where t.Fatalf is unsafe.
// A caller on the test's own goroutine that wants fail-fast behaviour checks the returned err itself.
func runLoomCLINoFatal(exe, dir string, timeout time.Duration, args ...string) (stdout string, exitCode int, err error) {
	return lymbin.Run(exe, dir, timeout, args...)
}

// skipBoardPush sets the environment that keeps a verb's detached board sync from pushing.
// Without it the push would outlive the test and race TempDir's removal of the bare remote.
var skipBoardPush = []string{"BOARD_SKIP_PUSH=1"}

// skipWalkPush sets the environment for a verb that reconciles an unstamped hub.
// It skips the fabric push as well as the board sync's push.
// The reconcile commit fires a detached push that outlives the test.
// That push would race TempDir's removal of the bare remote, so a test asserts the commit, never the push.
var skipWalkPush = append([]string{"FABRIC_SKIP_PUSH=1"}, skipBoardPush...)

// runLoomCLIWithEnvNoFatal runs exe with args in dir as a real subprocess, bounded by timeout.
// It never calls a *testing.T method.
// It appends extraEnv to the subprocess's inherited environment alone.
// A parallel test can thus set a variable such as FABRIC_SKIP_PUSH for one invocation without t.Setenv.
func runLoomCLIWithEnvNoFatal(exe, dir string, extraEnv []string, timeout time.Duration, args ...string) (stdout string, exitCode int, err error) {
	return lymbin.RunWithEnv(exe, dir, extraEnv, timeout, args...)
}

// newWiredPairFixture builds a real fabric hub and seeds every module's config template into it.
// It then adds one worktree pair, the shape every test in this suite needs before it can bootstrap loom.
// AddPair writes and commits the pair's origin record, so a caller's first "loom start" call needs no --parent flag.
func newWiredPairFixture(t *testing.T) (h *hubforge.Hub, loc *lymcwd.Location, worktree, slug string) {
	t.Helper()

	h = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedConfig(t, h, map[string]string{
		"claude":  claudeConfigWithBinary(providerlessBinary),
		"loom":    fastDeadlineLoomConfig(),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})

	slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree = h.PairCodeWorktree(slug)

	var err error
	loc, err = lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}
	return h, loc, worktree, slug
}

// providerlessShuttleConfig returns shuttle's shipped config template, with a shorter startup window and no start retry.
// The shortened window lets a launch failure from providerlessBinary surface within a test's own patience.
// The fixtures launch once: a died start would otherwise be launched again after a wait.
//
// No test here is about what a provider does.
// They cover bootstrap and driver mechanics: locks, handshakes, strands and the status file.
// Without a provider to launch, no test spawns a real provider subprocess.
func providerlessShuttleConfig() string {
	cfg := shuttleengine.ConfigTemplate()
	const retriesValue = "start_retries: 1"
	if !strings.Contains(cfg, retriesValue) {
		panic("shuttle config template drift: " + retriesValue + " not found")
	}
	cfg = strings.Replace(cfg, "startup_timeout_s: 90", "startup_timeout_s: 2", 1)
	return strings.Replace(cfg, "start_retries: 1", "start_retries: 0", 1)
}

// providerlessBinary is a provider path that cannot exist, which keeps a fixture from starting a real session.
const providerlessBinary = "/nonexistent/lym-smoke-has-no-provider"

// claudeConfigWithBinary returns claude.yaml's shipped template with its binary value set to binary.
// A fixture seeding this config then launches that executable rather than the real claude on PATH.
// It panics when the template no longer holds the line it rewrites, so drift fails loudly.
func claudeConfigWithBinary(binary string) string {
	const binaryValue = "binary: ${env:LYM_SHUTTLE_CLAUDE:-}"
	template := claudeengine.ConfigTemplate()
	if !strings.Contains(template, binaryValue) {
		panic("claude config template drift: " + binaryValue + " not found")
	}
	return strings.Replace(template, binaryValue, "binary: "+binary, 1)
}

// fastDeadlineLoomConfig returns loom's shipped config template with the discussion deadline cut to one minute.
// The deadline override must be applied together with the provider override, and neither is sufficient alone.
// Removing the provider stops a real session from starting but not the wait:
// a launch that fails still leaves reed holding the pane, so shuttle's wait loop polls until the SPEC's own deadline.
// The discussion deadline comes from loom.yaml's discussion_timeout_min, which ships at 480 minutes.
// That default is set high so an autonomous agent exploring a codebase is never cut off.
// In a fixture with no agent at all, the wait would be an eight-hour block on a thirty-second test.
func fastDeadlineLoomConfig() string {
	return strings.Replace(loomengine.ConfigTemplate(), "discussion_timeout_min: 480", "discussion_timeout_min: 1", 1)
}

// smokeStatusRel is the status file's relative path for the self run.
func smokeStatusRel(loc *lymcwd.Location) string {
	return shedrun.StatusRel(loc, shedrun.SelfRunID)
}

// recordsHeadChangedFiles returns the paths HEAD's own commit changed, relative to dir's repository root.
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

// seedGoDriverRun writes loc's self-run seed.json for the go driver.
// Its Params agree byte-for-byte with what the bootstrap writes on its next run against loc.
//
// The run verbs refuse a run-id that has no seed, before they run their own bootstrap or status-file checks.
// Fixtures that drive one of them against a deliberately absent status file therefore need this seed first.
// It is the state a seed written by "lym shed seed" before any bootstrap leaves.
func seedGoDriverRun(t *testing.T, loc *lymcwd.Location) {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the go driver run: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{
		Recipe: shedrun.RecipeTapestry,
		Driver: shedrun.DriverGo,
		Params: map[string]string{"parent": recorded.ParentBranch},
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(go driver): %v", err)
	}
	// The tackler and the arbiter are turned off.
	// A go-driven halt, such as a failing Preflight, hands back with no agent spawned,
	// and a tmux-tier smoke run reaches no LLM.
	localPath := configengine.LocalFile(loc.AnchorPath(), "shed")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("create the shed local layer directory: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("tackler_max_spawns: 0\narbiter_max_spawns: 0\n"), 0o644); err != nil {
		t.Fatalf("write the shed local layer: %v", err)
	}
}

// seedAndCommitStatus seeds loc's status file through the production Seed function and commits it on the records side.
// It is the seed-then-commit sequence that "lym loom start" runs,
// so a standalone-driver test gets a valid seeded pair without a live tmux server.
func seedAndCommitStatus(t *testing.T, loc *lymcwd.Location, slug string) {
	t.Helper()
	if err := loomshed.Seed(shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID), slug, "main"); err != nil {
		t.Fatalf("loomshed.Seed: %v", err)
	}
	rec := fabricengine.NewMutations("")
	if _, _, err := fabricengine.CommitRecordsPaths(rec, fabricengine.RecordsWorktree(loc), loc.AnchorRel, []string{smokeStatusRel(loc)}, "smoke: seed status", fabricengine.EnvSyncOptions()); err != nil {
		t.Fatalf("commit seed: %v", err)
	}
}

// seedLLMDriver writes loc's self-run seed.json with driver=llm directly to disk, uncommitted.
// It drives the production primitive directly rather than through a CLI subprocess.
//
// The written Params must agree byte-for-byte with what the first "lym loom start" against loc writes.
// WriteSeed refuses a disagreeing existing seed rather than accepting the later write.
// params.parent is the origin record's parent branch, so the origin record must already be committed.
func seedLLMDriver(t *testing.T, loc *lymcwd.Location) {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the llm driver: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{
		Recipe: shedrun.RecipeTapestry,
		Driver: shedrun.DriverLLM,
		Params: map[string]string{"parent": recorded.ParentBranch},
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(llm driver): %v", err)
	}
}

// newBadReedUpFixture builds a hub whose reed config carries an invalid mouse value, so reed Up fails.
// It seeds the pair with seed and returns the resolved location and the worktree path.
func newBadReedUpFixture(t *testing.T, seed func(*testing.T, *lymcwd.Location)) (*lymcwd.Location, string) {
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

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedConfig(t, h, map[string]string{
		"claude":  claudeConfigWithBinary(providerlessBinary),
		"loom":    fastDeadlineLoomConfig(),
		"reed":    strings.Join(lines, "\n"),
		"shuttle": providerlessShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-smoke-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)
	loc, err := lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}
	seed(t, loc)
	return loc, worktree
}

// waitForFixtureProcessesToExit registers a cleanup that waits for processes whose cwd is under the test's temp dirs.
// A push into a bare repository leaves a git process there for a moment;
// that process would otherwise write into a directory the temp-dir removal is deleting.
// It is registered after the fixture, so it runs before the fixture's removal;
// the wait is best effort, and the removal reports what is still left.
func waitForFixtureProcessesToExit(t *testing.T) {
	t.Helper()
	root := filepath.Dir(t.TempDir())
	t.Cleanup(func() {
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			running := false
			for _, pid := range tmuxkit.Pids() {
				if cwd, ok := tmuxkit.ProcCwd(pid); ok && pid != os.Getpid() && strings.HasPrefix(cwd, root+string(filepath.Separator)) {
					running = true
					break
				}
			}
			if !running {
				return
			}
		}
	})
}

// hubStampPath returns the hub's build stamp file.
func hubStampPath(loc *lymcwd.Location) string {
	return hubreconcile.Geometry{BoardDir: fabricengine.BoardDir(loc.HubPath)}.StampPath()
}

// commitRetiredBatcherKey commits a batcher.yaml that carries a retired key into loc's records worktree.
// This is the state that a hub reconcile after a binary change removes.
func commitRetiredBatcherKey(t *testing.T, loc *lymcwd.Location) {
	t.Helper()
	batcher, _ := configreg.Lookup("batcher")
	retired := strings.Replace(batcher.Template(), "orientation: 31400", "master_base: 52000", 1)
	gitkit.CommitFile(t, fabricengine.RecordsWorktree(loc), configengine.ConfigFileRel("batcher"), retired, "fixture: retired key")
}

// findWatchdogPIDs returns the pids of the live reed watchdog daemons.
// It matches a daemon whose argv carries a "--hub-path" argument equal to hubPath.
// A bootstrap spawns one such daemon per hub,
// and the cleanup finds it with the find-then-proc.KillPID pattern used for driver pids.
// Unlike a driver, the daemon's working directory does not identify it,
// so its --hub-path value keeps the scan to this fixture's own daemon rather than a sibling's.
// Linux only; returns nil on any other GOOS.
func findWatchdogPIDs(hubPath string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	var pids []int
	for _, pid := range tmuxkit.Pids() {
		argv, ok := tmuxkit.ProcArgv(pid)
		if !ok || !tmuxkit.IsWatchdog(argv) {
			continue
		}
		for i, arg := range argv {
			if arg == "--hub-path" && i+1 < len(argv) && argv[i+1] == hubPath {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// findDriverPIDs returns the pids of the live processes whose working directory is worktree.
// It finds the detached "lym loom drive" process a bootstrap spawns; it runs in the worktree and its argv is fixed.
// A lone "drive" argv element is not a sufficient discriminator, so the adjacent "loom" is required.
// Linux only; returns nil on any other GOOS.
// A process whose parent also matches is dropped.
// Between fork and exec, the driver's own child, such as a git call, still carries the driver's argv and cwd,
// so it is not a second driver.
func findDriverPIDs(worktree string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	parents := map[int]int{}
	for _, pid := range tmuxkit.Pids() {
		cwd, ok := tmuxkit.ProcCwd(pid)
		if !ok || cwd != worktree {
			continue
		}
		argv, ok := tmuxkit.ProcArgv(pid)
		if !ok {
			continue
		}
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "loom" && argv[i+1] == "drive" {
				// A parent that cannot be read answers 0, which no matching process has as its pid.
				parents[pid], _ = tmuxkit.ProcPPID(pid)
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
