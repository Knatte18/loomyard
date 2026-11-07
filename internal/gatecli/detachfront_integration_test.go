// detachfront_integration_test.go drives the capped front of `lym gate test` over hubforge hubs.
// Each call runs in a process of its own, against a stand-in tier command and the runners the front starts.
// It covers the cap and the phase it answers; the join of a live run; the runs a changed tree supersedes;
// a run that never started; the lock, the lease and its grace; the signals; and the registration of a pair's runner.

package gatecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// capStrandEntry is the strand-name entry of a capped call: a name that parses as an agent's.
const capStrandEntry = agentname.StrandNameEnv + "=lym:implementer"

// holdEnv makes the stand-in hold its tier until it is killed.
var holdEnv = []string{helperHoldEnv + "=1"}

// The periods a capped call's front polls at and its runner reads the lease at, in place of the production second.
const (
	testPollPeriod      = 50 * time.Millisecond
	testLeaseReadPeriod = 100 * time.Millisecond
)

// leaseSlack is the time past a lease read that a test waits before it reads a run's state.
const leaseSlack = 500 * time.Millisecond

// pollEnv gives a capped call's front and runner the test periods.
var pollEnv = []string{helperPollEnv + "=" + testPollPeriod.String(), helperLeaseReadEnv + "=" + testLeaseReadPeriod.String()}

// newRelease returns the path of a release file and the function that creates it.
// A stand-in or a card line that waits for the file at path ends once release ran.
// The test runs release when it ends too, so a failed test leaves nothing waiting.
func newRelease(t *testing.T) (path string, release func()) {
	t.Helper()

	path = filepath.Join(t.TempDir(), "release")
	var once sync.Once
	release = func() {
		once.Do(func() {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(release)
	return path, release
}

// releaseEnv is the entry that holds the stand-ins of a call until the file at path exists.
func releaseEnv(path string) string {
	return helperReleaseEnv + "=" + path
}

// releaseLine is the shell command that waits until the file at path exists.
func releaseLine(path string) string {
	return "while [ ! -e '" + path + "' ]; do sleep 0.05; done"
}

// waitPhaseOf waits until the phase file of run names state.
func waitPhaseOf(t *testing.T, run detachRun, state string) {
	t.Helper()

	eventually(t, "the run's phase "+state, func() bool {
		phase, found := readRunFile[runPhase](run.path(phaseFile))
		return found && phase.State == state
	})
}

// goingAnswer is the envelope of a capped call whose run goes on.
type goingAnswer struct {
	OK       bool                `json:"ok"`
	Done     *bool               `json:"done"`
	Phase    string              `json:"phase"`
	SlotFree *bool               `json:"slot_free"`
	Holders  []map[string]string `json:"holders"`
	Message  string              `json:"message"`
}

// startCappedGate starts a capped call from cwd, with the gate helper given a strand.
// The stand-in record is at record, the keep-warm cap is capWait, and env follows them in the environment.
func startCappedGate(t *testing.T, cwd, record, capWait string, env []string, args ...string) *runningGate {
	t.Helper()

	all := slices.Concat([]string{capStrandEntry, helperRecordEnv + "=" + record, helperCapEnv + "=" + capWait}, pollEnv, env)
	return startGate(t, gateRun{cwd: cwd, args: append([]string{"test"}, args...), env: all})
}

// requireGoing waits for g and fails unless it answered that its run goes on.
// The answer is SlotBusyExit, ok true, done false and the re-run message.
// A non-empty phase must be the phase the answer names.
func requireGoing(t *testing.T, g *runningGate, phase string) goingAnswer {
	t.Helper()

	code := g.wait(t)
	var answer goingAnswer
	if err := json.Unmarshal(g.stdout.Bytes(), &answer); err != nil {
		t.Fatalf("exit %d, stdout %q is not a JSON envelope: %v", code, g.stdout.String(), err)
	}
	if code != SlotBusyExit || !answer.OK || answer.Done == nil || *answer.Done || !strings.Contains(answer.Message, "run the same command again in a turn that does nothing else") {
		t.Fatalf("gate = exit %d, %+v; want exit %d, ok true, done false and a re-run message", code, answer, SlotBusyExit)
	}
	if phase != "" && answer.Phase != phase {
		t.Errorf("answered phase = %q; want %q", answer.Phase, phase)
	}
	return answer
}

// currentRunOf returns the run the key of a call from cwd with words names as current, once there is one.
func currentRunOf(t *testing.T, anchor, cwd string, words ...string) detachRun {
	t.Helper()

	return currentRunIn(t, detachKeyDir(anchor, verbTest, cwd, words))
}

// currentRunIn returns the run the key directory keyDir names as current, once there is one.
func currentRunIn(t *testing.T, keyDir string) detachRun {
	t.Helper()

	path := filepath.Join(keyDir, currentFile)
	var current currentRecord
	eventually(t, "the key's current run", func() bool {
		var found bool
		current, found = readRunFile[currentRecord](path)
		return found
	})
	return detachRun{dir: filepath.Join(filepath.Dir(path), current.RunID)}
}

// runnerPIDOf returns the pid in the process file of run, once run holds one.
func runnerPIDOf(t *testing.T, run detachRun) int {
	t.Helper()

	var process processRecord
	eventually(t, "the run's process file", func() bool {
		var found bool
		process, found = readRunFile[processRecord](run.path(processFile))
		return found
	})
	return process.PID
}

// waitExitOf returns the exit file of run, once it holds one.
func waitExitOf(t *testing.T, run detachRun) detachExit {
	t.Helper()

	var exit detachExit
	eventually(t, "the run's exit file", func() bool {
		var found bool
		exit, found = readRunFile[detachExit](run.path(exitFile))
		return found
	})
	return exit
}

// signalPID sends sig to pid, or kills it where the platform has no such signal.
func signalPID(t *testing.T, pid int, sig syscall.Signal) {
	t.Helper()

	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		err = process.Kill()
	} else {
		err = process.Signal(sig)
	}
	if err != nil {
		t.Fatalf("signal %d: %v", pid, err)
	}
}

// settleRunners ends every runner whose run directory lies under anchor's gate directory.
// It then settles the stand-ins that observed recorded, and waits for them.
// A runner ends at its next lease read once the lease is expired, killing its tier's tree as a passed lease does.
func settleRunners(t *testing.T, anchor string, observed *runningGate) {
	t.Helper()

	runs, _ := filepath.Glob(filepath.Join(gateslot.WaitDir(anchor), detachDirName, "*", "*"))
	var runners []int
	for _, dir := range runs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		run := detachRun{dir: dir}
		if err := run.expireLease(time.Now()); err != nil {
			t.Errorf("expire the lease of %s: %v", dir, err)
		}
		if process, found := readRunFile[processRecord](run.path(processFile)); found {
			runners = append(runners, process.PID)
		}
	}
	for _, pid := range runners {
		eventually(t, "a runner to end", func() bool { return processGone(pid) })
	}
	for _, record := range observed.tierRecords(t) {
		for _, pid := range []int{record.PID, record.Grandchild} {
			if pid != 0 {
				_ = (&os.Process{Pid: pid}).Kill()
			}
		}
	}
	for _, record := range observed.tierRecords(t) {
		eventually(t, "a stand-in to end", func() bool {
			return processGone(record.PID) && (record.Grandchild == 0 || processGone(record.Grandchild))
		})
	}
}

// detachRig is one hubforge hub holding the gate.yaml every call shares, and the stand-in record each call appends to.
type detachRig struct {
	t        *testing.T
	h        *hubforge.Hub
	prime    string
	anchor   string
	pool     *gateslot.Pool
	observed *runningGate
}

// newDetachRig builds a hub whose gate.yaml declares the scenario tiers and a `finalize` key over tier0.
// It settles every runner its calls started when the test ends.
func newDetachRig(t *testing.T) *detachRig {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	config := scenarioConfig(t, func(cfg map[string]any) {
		cfg["gates"] = map[string]any{"finalize": map[string]any{"tiers": []int{0}, "scope": "all"}}
	})
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	rig := &detachRig{
		t: t, h: h, prime: h.PrimeWorktree(), anchor: h.Location.AnchorPath(), pool: hubgeom.GateSlots(h.Location),
		observed: &runningGate{record: filepath.Join(t.TempDir(), "standin.json")},
	}
	t.Cleanup(func() { settleRunners(t, rig.anchor, rig.observed) })
	return rig
}

// call starts a capped call from the prime over the targets args name.
func (r *detachRig) call(capWait string, env []string, args ...string) *runningGate {
	r.t.Helper()

	return startCappedGate(r.t, r.prime, r.observed.record, capWait, env, args...)
}

// currentRun returns the run the prime's key for the targets names as current.
func (r *detachRig) currentRun(targets ...string) detachRun {
	r.t.Helper()

	return currentRunOf(r.t, r.anchor, r.prime, targets...)
}

// invocations returns the tier invocations the stand-in recorded over every call of the rig.
func (r *detachRig) invocations() []standInRecord {
	r.t.Helper()

	return r.observed.tierRecords(r.t)
}

// waitInvocations waits for the stand-in to have recorded count tier invocations and returns the latest.
func (r *detachRig) waitInvocations(count int) standInRecord {
	r.t.Helper()

	eventually(r.t, "the stand-in's tier invocations", func() bool { return len(r.invocations()) >= count })
	records := r.invocations()
	return records[count-1]
}

// requireFree fails unless every gate slot is free and no wait record is left.
func (r *detachRig) requireFree() {
	r.t.Helper()

	eventually(r.t, "the slots to be free", func() bool {
		holders, err := r.pool.Holders()
		return err == nil && len(holders) == 0
	})
	if waits, err := gateslot.ReadWaits(gateslot.WaitDir(r.anchor)); err != nil || len(waits) != 0 {
		r.t.Errorf("ReadWaits = (%+v, %v); want no wait record left", waits, err)
	}
}

// requireEnded fails unless the stand-in invocation record and its sleeper end.
func requireEnded(t *testing.T, record standInRecord) {
	t.Helper()

	eventually(t, "the tier's command and its sleeper to end", func() bool {
		return processGone(record.PID) && (record.Grandchild == 0 || processGone(record.Grandchild))
	})
}

// writePlan replaces the plan of the rig's prime with one of cards.
func (r *detachRig) writePlan(cards ...plankit.Card) {
	r.t.Helper()

	planDir := planparser.PlanDir(r.anchor)
	if err := os.RemoveAll(planDir); err != nil {
		r.t.Fatal(err)
	}
	plankit.Write(r.t, planDir, plankit.Plan{Approved: true, Cards: cards})
}

// cardCall starts a capped `card-verify` call for the card id from the prime.
func (r *detachRig) cardCall(capWait string, env []string, id string) *runningGate {
	r.t.Helper()

	all := slices.Concat([]string{capStrandEntry, helperRecordEnv + "=" + r.observed.record, helperCapEnv + "=" + capWait}, pollEnv, env)
	return startGate(r.t, gateRun{cwd: r.prime, args: []string{"card-verify", id}, env: all})
}

// cardKeyDir returns the key directory of a `card-verify` call from the prime that names the card id.
func (r *detachRig) cardKeyDir(id string) string {
	r.t.Helper()

	location, inHub, err := resolveHub(r.prime)
	if err != nil || !inHub {
		r.t.Fatalf("resolveHub(%s) = (%v, %v); want a hub", r.prime, inHub, err)
	}
	return detachKeyDir(r.anchor, verbCardVerify, location.WorktreePath(), []string{id})
}

// cardRun returns the current run of the key of a `card-verify` call that names the card id, once there is one.
func (r *detachRig) cardRun(id string) detachRun {
	r.t.Helper()

	return currentRunIn(r.t, r.cardKeyDir(id))
}

// readPIDFile returns the pid the file at path holds, once it holds one.
func readPIDFile(t *testing.T, path string) int {
	t.Helper()

	var pid int
	eventually(t, "the pid file", func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil
	})
	return pid
}

func TestGateTest_DetachedRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("a run that outlasts the cap answers done false in its phase, the next call after it ended prints its result, and a third call starts a fresh run", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		path, release := newRelease(t)
		env := []string{releaseEnv(path), helperDelayEnv + "=0s", helperExitEnv + "=7"}

		requireGoing(t, rig.call("3s", env, "./pkg"), "running tier0")
		run := rig.currentRun("./pkg")
		release()
		waitExitOf(t, run)

		second := rig.call("3s", env, "./pkg")
		if code := second.wait(t); code != 7 {
			t.Fatalf("second call = exit %d, stdout %q; want the stand-in's 7", code, second.stdout.String())
		}
		for _, want := range []string{"standin ran", "gate test: tier0 "} {
			if !strings.Contains(second.stdout.String(), want) {
				t.Errorf("second call stdout = %q; want the run's output holding %q", second.stdout.String(), want)
			}
		}

		requireGoing(t, rig.call("3s", slices.Concat([]string{helperExitEnv + "=7"}, holdEnv), "./pkg"), "")
		if rig.currentRun("./pkg").dir == run.dir {
			t.Errorf("the third call joined run %s, whose result the second call printed; want a fresh run", run.dir)
		}
		rig.waitInvocations(2)
	})

	t.Run("a call with every slot held names the holders, a second call joins the same run, and releasing the slots lets it finish", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		first, second := acquireAll(t, rig.pool)

		answer := requireGoing(t, rig.call("4s", nil, "./pkg"), "slots tier0")
		var sites []string
		for _, holder := range answer.Holders {
			sites = append(sites, holder["site"])
		}
		slices.Sort(sites)
		if answer.SlotFree == nil || *answer.SlotFree || !slices.Equal(sites, []string{"holder one", "holder two"}) {
			t.Errorf("answer = %+v; want slot_free false naming both holders", answer)
		}
		run := rig.currentRun("./pkg")

		requireGoing(t, rig.call("2s", nil, "./pkg"), "")
		if joined := rig.currentRun("./pkg"); joined.dir != run.dir {
			t.Errorf("second call ran %s; want it to join %s", joined.dir, run.dir)
		}

		if err := first.Release(); err != nil {
			t.Fatal(err)
		}
		if err := second.Release(); err != nil {
			t.Fatal(err)
		}
		if exit := waitExitOf(t, run); exit != (detachExit{}) {
			t.Errorf("exit file = %+v; want code 0, not abandoned", exit)
		}
		if got := len(rig.invocations()); got != 1 {
			t.Errorf("tier invocations = %d; want the joined run's one", got)
		}
	})

	t.Run("a call after a new untracked file changed the dirty paths starts a fresh run and ends the superseded runner", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		requireGoing(t, rig.call("2s", holdEnv, "./pkg"), "")
		held := rig.waitInvocations(1)
		run := rig.currentRun("./pkg")
		runner := runnerPIDOf(t, run)

		untracked := filepath.Join(rig.prime, "row-dirty.txt")
		if err := os.WriteFile(untracked, []byte("dirty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(untracked) })

		second := rig.call("4s", []string{helperExitEnv + "=5"}, "./pkg")
		if code := second.wait(t); code != 5 {
			t.Errorf("second call = exit %d, stdout %q; want the new run's 5", code, second.stdout.String())
		}
		if rig.currentRun("./pkg").dir == run.dir {
			t.Errorf("the second call joined run %s; want a fresh run over the changed dirty paths", run.dir)
		}
		eventually(t, "the superseded runner to end", func() bool { return processGone(runner) })
		requireEnded(t, held)
		rig.requireFree()
	})

	t.Run("a call after a commit changed HEAD's tree over the same dirty paths starts a fresh run and ends the superseded runner", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		requireGoing(t, rig.call("2s", holdEnv, "./pkg"), "")
		held := rig.waitInvocations(1)
		run := rig.currentRun("./pkg")
		runner := runnerPIDOf(t, run)

		gitkit.CommitFile(t, rig.prime, "row-commit.txt", "committed\n", "row commit")

		second := rig.call("4s", []string{helperExitEnv + "=6"}, "./pkg")
		if code := second.wait(t); code != 6 {
			t.Errorf("second call = exit %d, stdout %q; want the new run's 6", code, second.stdout.String())
		}
		if rig.currentRun("./pkg").dir == run.dir {
			t.Errorf("the second call joined run %s; want a fresh run over HEAD's new tree", run.dir)
		}
		eventually(t, "the superseded runner to end", func() bool { return processGone(runner) })
		requireEnded(t, held)
	})

	t.Run("a run whose stand-in exits 75 is replayed as exit 1 with the line saying so", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		g := rig.call("5s", []string{helperExitEnv + "=75"}, "./pkg")
		if code := g.wait(t); code != 1 {
			t.Errorf("call = exit %d, stdout %q; want 1", code, g.stdout.String())
		}
		if !strings.Contains(g.stdout.String(), replayedBusyLine(verbTest)) {
			t.Errorf("stdout = %q; want the line %q", g.stdout.String(), replayedBusyLine(verbTest))
		}
	})

	t.Run("a call after the runner was killed without an exit file starts a fresh run", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		requireGoing(t, rig.call("2s", holdEnv, "./pkg"), "")
		rig.waitInvocations(1)
		run := rig.currentRun("./pkg")
		runner := runnerPIDOf(t, run)
		signalPID(t, runner, syscall.SIGKILL)
		eventually(t, "the killed runner to end", func() bool { return processGone(runner) })

		second := rig.call("4s", nil, "./pkg")
		if code := second.wait(t); code != 0 {
			t.Errorf("second call = exit %d, stdout %q; want the fresh run's 0", code, second.stdout.String())
		}
		if rig.currentRun("./pkg").dir == run.dir {
			t.Errorf("the second call joined the killed run %s; want a fresh run", run.dir)
		}
		if got := len(rig.invocations()); got != 2 {
			t.Errorf("tier invocations = %d; want one per run", got)
		}
	})

	t.Run("a runner that never started exits 1 with its log, leaves the lease passed and the run refused", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		g := rig.call("5s", []string{helperLaunchEnv + "=none", helperClaimBoundEnv + "=1s"}, "./pkg")
		if code := g.wait(t); code != 1 {
			t.Fatalf("call = exit %d, stdout %q; want 1", code, g.stdout.String())
		}
		for _, want := range []string{runnerNeverStartedLine, "--wait", "run the same command again"} {
			if !strings.Contains(g.stdout.String(), want) {
				t.Errorf("stdout = %q; want it to hold %q", g.stdout.String(), want)
			}
		}
		run := rig.currentRun("./pkg")
		if run.leaseHolds(time.Now()) {
			t.Error("the lease of the run that never started still holds; want it passed")
		}
		if process, found := readRunFile[processRecord](run.path(processFile)); !found || process.PID != g.cmd.Process.Pid {
			t.Errorf("process file = %+v, %v; want it to name the front %d", process, found, g.cmd.Process.Pid)
		}

		code, output := runGate(t, gateRun{cwd: rig.prime, args: []string{"run-detached", run.dir}})
		if code != 1 || !strings.Contains(output, "no live lease") {
			t.Errorf("run-detached on the run = exit %d, stdout %q; want 1 refusing a run with no live lease", code, output)
		}
	})

	t.Run("a launch that fails exits 1 at once, leaves the lease passed and the next call starts a fresh run", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		started := time.Now()
		g := rig.call("5s", []string{helperLaunchEnv + "=error"}, "./pkg")
		if code := g.wait(t); code != 1 {
			t.Fatalf("call = exit %d, stdout %q; want 1", code, g.stdout.String())
		}
		if elapsed := time.Since(started); elapsed > 4*time.Second {
			t.Errorf("call took %s; want it to end at once, not wait out the claim bound", elapsed)
		}
		for _, want := range []string{"the launch failed", "--wait", "run the same command again"} {
			if !strings.Contains(g.stdout.String(), want) {
				t.Errorf("stdout = %q; want it to hold %q", g.stdout.String(), want)
			}
		}
		run := rig.currentRun("./pkg")
		if run.leaseHolds(time.Now()) {
			t.Error("the lease of the run that never started still holds; want it passed")
		}
		if process, found := readRunFile[processRecord](run.path(processFile)); !found || process.PID != g.cmd.Process.Pid {
			t.Errorf("process file = %+v, %v; want it to name the front %d", process, found, g.cmd.Process.Pid)
		}

		second := rig.call("5s", nil, "./pkg")
		if code := second.wait(t); code != 0 {
			t.Errorf("second call = exit %d, stdout %q; want a fresh run's 0", code, second.stdout.String())
		}
		if got := len(rig.invocations()); got != 1 {
			t.Errorf("tier invocations = %d; want the fresh run's one", got)
		}
	})

	t.Run("a runner that starts late is claimed within the claim bound and the call answers before its cap", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		started := time.Now()
		g := rig.call("12s", []string{helperLaunchEnv + "=late", helperClaimBoundEnv + "=4s"}, "./pkg")
		if code := g.wait(t); code != 0 {
			t.Errorf("call = exit %d, stdout %q; want the stand-in's 0", code, g.stdout.String())
		}
		if elapsed := time.Since(started); elapsed >= 12*time.Second {
			t.Errorf("call took %s; want it to answer before its 12s cap", elapsed)
		}
	})

	t.Run("a held key lock past the lock margin answers done false in the phase starting and creates no run", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		keyDir := detachKeyDir(rig.anchor, verbTest, rig.prime, []string{"./pkg"})
		if err := os.MkdirAll(keyDir, 0o755); err != nil {
			t.Fatal(err)
		}
		held, err := lock.AcquireWriteLock(filepath.Join(keyDir, keyLockFile))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = held.Release() })

		requireGoing(t, rig.call("2s", []string{helperLockMarginEnv + "=1s"}, "./pkg"), "starting")
		entries, err := os.ReadDir(keyDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != keyLockFile {
				t.Errorf("key directory holds %s; want only the lock, since the call created no run", entry.Name())
			}
		}
	})

	t.Run("--wait under a strand runs in this process, creates no key directory and exits with the tier's code", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		g := rig.call("2s", []string{helperExitEnv + "=3"}, "--wait", "./pkg")
		if code := g.wait(t); code != 3 {
			t.Errorf("call = exit %d, stdout %q; want the tier's 3", code, g.stdout.String())
		}
		if _, err := os.Stat(filepath.Join(gateslot.WaitDir(rig.anchor), detachDirName)); !os.IsNotExist(err) {
			t.Errorf("detach directory stat error = %v; want none created", err)
		}
	})

	t.Run("a resolver fallback still caps the call and is logged once", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		g := rig.call("fallback", []string{helperExitEnv + "=7"}, "./pkg")
		if code := g.wait(t); code != 7 {
			t.Errorf("call = exit %d, stdout %q; want the stand-in's 7 replayed", code, g.stdout.String())
		}
		rig.currentRun("./pkg")
		if got := strings.Count(g.stderr.String(), "the keep-warm resolver fell back"); got != 1 {
			t.Errorf("stderr = %q; want the fallback Warn exactly once, got %d", g.stderr.String(), got)
		}
	})

	t.Run("RunGate under a strand is no capped call and creates no key directory", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)

		g := startGate(t, gateRun{cwd: rig.prime, args: []string{"finalize"}, env: []string{
			helperEnv + "=" + helperRoleRunGate, capStrandEntry, helperCapEnv + "=1s", helperDelayEnv + "=2s", helperExitEnv + "=7",
		}})
		if code := g.wait(t); code != 7 {
			t.Errorf("RunGate = exit %d, stdout %q; want the stand-in's 7 after its 2s, not a cap's 75", code, g.stdout.String())
		}
		if _, err := os.Stat(filepath.Join(gateslot.WaitDir(rig.anchor), detachDirName)); !os.IsNotExist(err) {
			t.Errorf("detach directory stat error = %v; want none created", err)
		}
	})

	t.Run("a capped call outside every hub runs unslotted in a runner and registers nothing", func(t *testing.T) {
		t.Parallel()
		worktree := standaloneWorktree(t, true)
		gitkit.CommitFile(t, worktree, "README.md", "standalone\n", "init")
		location, err := lymcwd.ResolveWorktree(worktree)
		if err != nil {
			t.Fatal(err)
		}
		observed := &runningGate{record: filepath.Join(t.TempDir(), "standin.json")}
		t.Cleanup(func() { settleRunners(t, location.AnchorPath(), observed) })
		path, release := newRelease(t)
		env := []string{releaseEnv(path), helperDelayEnv + "=0s", helperExitEnv + "=4"}

		requireGoing(t, startCappedGate(t, worktree, observed.record, "2s", env, "./pkg"), "")
		release()
		waitExitOf(t, currentRunOf(t, location.AnchorPath(), worktree, "./pkg"))
		second := startCappedGate(t, worktree, observed.record, "2s", env, "./pkg")
		if code := second.wait(t); code != 4 {
			t.Errorf("second call = exit %d, stdout %q; want the stand-in's 4", code, second.stdout.String())
		}
		if record := observed.tierRecords(t)[0]; record.Slot != "" {
			t.Errorf("stand-in slot = %q; want an unslotted run", record.Slot)
		}
		if _, err := os.Stat(fabricengine.RunProcDir(location.HubPath)); !os.IsNotExist(err) {
			t.Errorf("run-process directory stat error = %v; want nothing registered", err)
		}
	})

	t.Run("a capped call from a directory in no git worktree is not capped and refuses as an uncapped call does", func(t *testing.T) {
		t.Parallel()
		observed := &runningGate{record: filepath.Join(t.TempDir(), "standin.json")}

		g := startCappedGate(t, t.TempDir(), observed.record, "2s", nil, "./pkg")
		if code := g.wait(t); code != 1 {
			t.Fatalf("call = exit %d, stdout %q; want 1", code, g.stdout.String())
		}
		if !strings.Contains(g.stdout.String(), "lym config gate") || !strings.Contains(g.stdout.String(), "git worktree") {
			t.Errorf("stdout = %q; want the refusal naming lym config gate run in a git worktree", g.stdout.String())
		}
	})

	t.Run("a passed lease ends the stand-in, frees its slot and abandons the run, and the next call replays nothing from it", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		env := append([]string{helperGraceEnv + "=3s"}, holdEnv...)

		requireGoing(t, rig.call("1s", env, "./pkg"), "")
		held := rig.waitInvocations(1)
		run := rig.currentRun("./pkg")
		if exit := waitExitOf(t, run); exit != (detachExit{Code: 1, Abandoned: true}) {
			t.Errorf("exit file = %+v; want code 1, abandoned", exit)
		}
		requireEnded(t, held)
		rig.requireFree()

		requireGoing(t, rig.call("1s", env, "./pkg"), "")
		if rig.currentRun("./pkg").dir == run.dir {
			t.Errorf("the next call joined the abandoned run %s; want a fresh run", run.dir)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("a SIGTERM to a waiting front exits 143 while the runner goes on, and the next call replays the run", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			path, release := newRelease(t)
			env := []string{releaseEnv(path), helperDelayEnv + "=0s", helperExitEnv + "=9"}

			g := rig.call("60s", env, "./pkg")
			run := rig.currentRun("./pkg")
			runner := runnerPIDOf(t, run)
			waitPhaseOf(t, run, phaseRunning)
			signalPID(t, g.cmd.Process.Pid, syscall.SIGTERM)
			if code := g.wait(t); code != 128+int(syscall.SIGTERM) {
				t.Errorf("front = exit %d, stdout %q; want 143", code, g.stdout.String())
			}
			if !proc.IsAlive(runner) {
				t.Error("the runner ended with its front; want it to go on")
			}
			release()
			if exit := waitExitOf(t, run); exit != (detachExit{Code: 9}) {
				t.Errorf("exit file = %+v; want code 9, not abandoned", exit)
			}

			second := rig.call("5s", env, "./pkg")
			if code := second.wait(t); code != 9 {
				t.Errorf("second call = exit %d, stdout %q; want the run's 9 replayed", code, second.stdout.String())
			}
		})

		t.Run("a SIGTERM to the runner ends a waiting call with done false and replays nothing, and the next call starts a fresh run", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)

			waiting := rig.call("60s", holdEnv, "./pkg")
			held := rig.waitInvocations(1)
			run := rig.currentRun("./pkg")
			signalPID(t, runnerPIDOf(t, run), syscall.SIGTERM)
			started := time.Now()
			requireGoing(t, waiting, "")
			if elapsed := time.Since(started); elapsed > 30*time.Second {
				t.Errorf("waiting call took %s after the runner ended; want it to answer without waiting out its cap", elapsed)
			}
			if exit := waitExitOf(t, run); exit != (detachExit{Code: 128 + int(syscall.SIGTERM), Abandoned: true}) {
				t.Errorf("exit file = %+v; want 143, abandoned", exit)
			}
			requireEnded(t, held)

			second := rig.call("5s", nil, "./pkg")
			if code := second.wait(t); code != 0 {
				t.Errorf("second call = exit %d, stdout %q; want a fresh run's 0", code, second.stdout.String())
			}
			if rig.currentRun("./pkg").dir == run.dir {
				t.Errorf("the second call joined the abandoned run %s; want a fresh run", run.dir)
			}
		})
	}

	t.Run("a short call joining a long call's run leaves the lease at the later deadline", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		const grace = time.Second
		env := append([]string{helperGraceEnv + "=" + grace.String()}, holdEnv...)

		long := rig.call("10s", env, "./pkg")
		rig.waitInvocations(1)
		run := rig.currentRun("./pkg")
		runner := runnerPIDOf(t, run)
		leased, _ := readRunFile[leaseRecord](run.path(leaseFile))

		requireGoing(t, rig.call("1s", env, "./pkg"), "")
		if joined := rig.currentRun("./pkg"); joined.dir != run.dir {
			t.Errorf("short call ran %s; want it to join %s", joined.dir, run.dir)
		}
		if after, _ := readRunFile[leaseRecord](run.path(leaseFile)); !after.Deadline.Equal(leased.Deadline) {
			t.Errorf("lease deadline = %s after the short call; want the long call's %s", after.Deadline, leased.Deadline)
		}
		// The short call's deadline plus grace has passed by the time the pause ends, and the runner has read the lease since.
		time.Sleep(grace + testLeaseReadPeriod + leaseSlack)
		if _, ended := readRunFile[detachExit](run.path(exitFile)); ended || !proc.IsAlive(runner) {
			t.Errorf("runner alive = %v, exit file present = %v after the short call's deadline plus grace; want it live", proc.IsAlive(runner), ended)
		}
		requireGoing(t, long, "")
	})

	t.Run("a joining call moves the lease later and keeps the runner past the first call's deadline plus grace", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		env := append([]string{helperGraceEnv + "=2s"}, holdEnv...)

		requireGoing(t, rig.call("3s", env, "./pkg"), "")
		run := rig.currentRun("./pkg")
		runner := runnerPIDOf(t, run)
		first, _ := readRunFile[leaseRecord](run.path(leaseFile))

		requireGoing(t, rig.call("3s", env, "./pkg"), "")
		if joined := rig.currentRun("./pkg"); joined.dir != run.dir {
			t.Fatalf("second call ran %s; want it to join %s", joined.dir, run.dir)
		}
		second, _ := readRunFile[leaseRecord](run.path(leaseFile))
		if !second.Deadline.After(first.Deadline) {
			t.Fatalf("lease deadline = %s after the joining call; want it later than the first call's %s", second.Deadline, first.Deadline)
		}

		// The runner reads its lease once per test period, so one that was not extended has ended by now.
		time.Sleep(time.Until(first.Deadline.Add(testLeaseReadPeriod + leaseSlack)))
		if _, ended := readRunFile[detachExit](run.path(exitFile)); ended || !proc.IsAlive(runner) {
			t.Errorf("runner alive = %v, exit file present = %v after the first call's deadline plus grace; want it live", proc.IsAlive(runner), ended)
		}
	})

	t.Run("a --gate key whose earlier runs outlast the break-even waits its long cap and ends with the run's result on the first call", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		for i := range 3 {
			start := time.Now().Add(-time.Duration(i+1) * time.Hour)
			record := gateslot.RunRecord{
				GateKey: "finalize", CallID: "seeded-" + strconv.Itoa(i), TierList: []int{0}, Tier: 0, Command: "run", Outcome: "passed",
				WaitStart: start, Start: start, End: start.Add(10 * time.Second),
			}
			if _, err := gateslot.WriteRunRecord(runLogDirOf(rig.h.Location, true), record, nil, start); err != nil {
				t.Fatal(err)
			}
		}

		started := time.Now()
		g := rig.call("1s", []string{helperLongCapEnv + "=8s", helperMaxRerunsEnv + "=2", helperDelayEnv + "=2s"}, "--gate", "finalize")
		if code := g.wait(t); code != 0 {
			t.Errorf("call = exit %d, stdout %q; want the run's 0 and not the short cap's %d", code, g.stdout.String(), SlotBusyExit)
		}
		if elapsed := time.Since(started); elapsed < 2*time.Second {
			t.Errorf("call took %s; want it to wait out the stand-in's 2s", elapsed)
		}
		if !strings.Contains(g.stdout.String(), "gate test: tier0 ") {
			t.Errorf("stdout = %q; want the run's summary line", g.stdout.String())
		}
	})

	t.Run("a pair's runner is registered under its slug with the gate role, ended by the sparing teardown, and the prime registers nothing", func(t *testing.T) {
		t.Parallel()
		rig := newDetachRig(t)
		runProcDir := fabricengine.RunProcDir(rig.h.Location.HubPath)

		requireGoing(t, rig.call("2s", holdEnv, "./prime"), "")
		rig.waitInvocations(1)
		if entries, err := os.ReadDir(runProcDir); err == nil && len(entries) != 0 {
			t.Errorf("run-process directory holds %d entries after a prime call; want none", len(entries))
		}

		hubforge.AddPair(t, rig.h, "gate-pair")
		pair := rig.h.PairCodeWorktree("gate-pair")
		location, err := lymcwd.ResolveWorktree(pair)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { settleRunners(t, location.AnchorPath(), rig.observed) })
		requireGoing(t, startCappedGate(t, pair, rig.observed.record, "2s", holdEnv, "./pair"), "")
		runner := runnerPIDOf(t, currentRunOf(t, location.AnchorPath(), pair, "./pair"))

		records, err := runproc.List(runProcDir, location.WorktreeName)
		if err != nil || len(records) != 1 || records[0].PID != runner || records[0].Role != runproc.RoleGate {
			t.Fatalf("registered processes of %s = (%+v, %v); want the runner %d with the gate role", location.WorktreeName, records, err, runner)
		}
		if entries, err := os.ReadDir(runProcDir); err != nil || len(entries) != 1 {
			t.Errorf("run-process directory = (%v, %v); want the pair's owner alone", entries, err)
		}

		if _, err := runproc.EndSparing(runProcDir, location.WorktreeName, os.Getpid(), runproc.RoleDobby); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the pair's runner to end", func() bool { return processGone(runner) })
	})

	// The card lines below are shell scripts, and the runner ties them to itself with the parent-death signal.
	// Windows has no counterpart to that signal.
	if runtime.GOOS != "windows" {
		t.Run("a card line that outlasts the cap answers done false in the phase running, and the next call after it ended prints its output and exits with its code", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			path, release := newRelease(t)
			rig.writePlan(verifyCard(4, "alpha", releaseLine(path)+"; echo done-line; exit 4"))

			answer := requireGoing(t, rig.cardCall("2s", nil, "4"), "")
			if !strings.HasPrefix(answer.Phase, phaseRunning) {
				t.Errorf("answered phase = %q; want one starting with %q", answer.Phase, phaseRunning)
			}
			release()
			waitExitOf(t, rig.cardRun("4-alpha"))

			second := rig.cardCall("3s", nil, "4")
			if code := second.wait(t); code != 4 || !strings.Contains(second.stdout.String(), "done-line") {
				t.Errorf("second call = exit %d, stdout %q; want the line's 4 and its output", code, second.stdout.String())
			}
		})

		t.Run("a call with a card's number and a call with its N-slug join one run, and the line runs once", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			marker := filepath.Join(t.TempDir(), "ran")
			path, release := newRelease(t)
			rig.writePlan(verifyCard(4, "alpha", "echo ran >> "+marker+"; "+releaseLine(path)))

			requireGoing(t, rig.cardCall("1s", nil, "4"), "")
			requireGoing(t, rig.cardCall("1s", nil, "4-alpha"), "")
			entries, err := os.ReadDir(rig.cardKeyDir("4-alpha"))
			if err != nil {
				t.Fatal(err)
			}
			var runs int
			for _, entry := range entries {
				if entry.IsDir() {
					runs++
				}
			}
			if runs != 1 {
				t.Errorf("key directory holds %d run directories; want the one both calls joined", runs)
			}
			release()
			waitExitOf(t, rig.cardRun("4-alpha"))
			if data, err := os.ReadFile(marker); err != nil || string(data) != "ran\n" {
				t.Errorf("marker = %q, %v; want the line to have run once", data, err)
			}
		})

		t.Run("a card line waiting for a slot answers done false in the phase slots with the line and the holders, and releasing the slots lets the next call replay exit 0", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			rig.writePlan(verifyCard(4, "alpha", "true"))
			first, second := acquireAll(t, rig.pool)

			answer := requireGoing(t, rig.cardCall("2s", nil, "4"), "slots true")
			var sites []string
			for _, holder := range answer.Holders {
				sites = append(sites, holder["site"])
			}
			slices.Sort(sites)
			if answer.SlotFree == nil || *answer.SlotFree || !slices.Equal(sites, []string{"holder one", "holder two"}) {
				t.Errorf("answer = %+v; want slot_free false naming both holders", answer)
			}

			if err := first.Release(); err != nil {
				t.Fatal(err)
			}
			if err := second.Release(); err != nil {
				t.Fatal(err)
			}
			replay := rig.cardCall("10s", nil, "4")
			if code := replay.wait(t); code != 0 {
				t.Errorf("call after the release = exit %d, stdout %q; want the line's 0", code, replay.stdout.String())
			}
		})

		t.Run("a passed lease ends a card line that holds the runner's slot with no second call, and frees the slot", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			pidFile := filepath.Join(t.TempDir(), "pid")
			rig.writePlan(verifyCard(4, "alpha", "echo $$ > "+pidFile+"; exec sleep 60"))

			requireGoing(t, rig.cardCall("1s", []string{helperGraceEnv + "=1s"}, "4"), "")
			sleeper := readPIDFile(t, pidFile)
			if exit := waitExitOf(t, rig.cardRun("4-alpha")); exit != (detachExit{Code: 1, Abandoned: true}) {
				t.Errorf("exit file = %+v; want code 1, abandoned", exit)
			}
			eventually(t, "the line's sleep to end", func() bool { return processGone(sleeper) })
			rig.requireFree()
		})

		t.Run("a passed lease ends a --wait gate line through its child, which kills its tiers and frees its slots", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			stubDir := t.TempDir()
			// The stub stands for `lym`: it drops the `gate` word and runs the test binary in the gate role.
			stub := "#!/bin/sh\nshift\nexec env " + helperEnv + "=" + helperRoleGate + " " + os.Args[0] + " \"$@\"\n"
			if err := os.WriteFile(filepath.Join(stubDir, "lym"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			rig.writePlan(verifyCard(4, "alpha", "lym gate test --wait ./pkg"))
			env := append([]string{helperGraceEnv + "=1s", "PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH")}, holdEnv...)

			requireGoing(t, rig.cardCall("1s", env, "4"), "")
			held := rig.waitInvocations(1)
			if exit := waitExitOf(t, rig.cardRun("4-alpha")); exit != (detachExit{Code: 1, Abandoned: true}) {
				t.Errorf("exit file = %+v; want code 1, abandoned", exit)
			}
			requireEnded(t, held)
			rig.requireFree()
		})

		t.Run("a plan edited between the front's read and the runner's makes the runner record the refusal, and the call replays it with exit 1", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			rig.writePlan(verifyCard(4, "alpha", "true"))

			g := rig.cardCall("6s", []string{helperLaunchEnv + "=late", helperClaimBoundEnv + "=4s"}, "4")
			run := rig.cardRun("4-alpha")
			rig.writePlan(verifyCard(9, "other", "true"))

			if code := g.wait(t); code != 1 {
				t.Fatalf("call = exit %d, stdout %q; want 1", code, g.stdout.String())
			}
			for _, want := range []string{"names no card", "9-other", "way forward"} {
				if !strings.Contains(g.stdout.String(), want) {
					t.Errorf("stdout = %q; want the replayed refusal holding %q", g.stdout.String(), want)
				}
			}
			if exit := waitExitOf(t, run); exit != (detachExit{Code: 1}) {
				t.Errorf("exit file = %+v; want code 1, not abandoned", exit)
			}
		})

		t.Run("a call after the runner was killed ends the group its line recorded, backgrounded descendants included, and starts a fresh run", func(t *testing.T) {
			t.Parallel()
			rig := newDetachRig(t)
			pidFile := filepath.Join(t.TempDir(), "pid")
			rig.writePlan(verifyCard(4, "alpha", "sleep 60 & echo $! > "+pidFile+"; wait"))

			requireGoing(t, rig.cardCall("1s", nil, "4"), "")
			sleeper := readPIDFile(t, pidFile)
			run := rig.cardRun("4-alpha")
			eventually(t, "the line's process tree record", func() bool {
				_, found := readRunFile[proc.TreeRecord](run.path(lineFile))
				return found
			})
			runner := runnerPIDOf(t, run)
			signalPID(t, runner, syscall.SIGKILL)
			eventually(t, "the killed runner to end", func() bool { return processGone(runner) })
			if !proc.IsAlive(sleeper) {
				t.Fatal("the backgrounded sleep ended with its runner; want it to outlive the parent-death signal")
			}

			requireGoing(t, rig.cardCall("1s", nil, "4"), "")
			eventually(t, "the backgrounded sleep to end", func() bool { return processGone(sleeper) })
			if rig.cardRun("4-alpha").dir == run.dir {
				t.Errorf("the second call joined the killed run %s; want a fresh run", run.dir)
			}
		})
	}
}
