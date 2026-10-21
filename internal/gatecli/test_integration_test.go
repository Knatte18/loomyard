//go:build integration

// test_integration_test.go drives `lyx gate test` over one hubforge hub and a stand-in tier command, each run in a process of its own so a test can signal the gate it started:
// the tier run, its slots, the wait bound, inheritance, hub resolution from -C, the unslotted run outside every hub, the child's lifetime under a catchable signal, a SIGKILL and a timeout, the prebuild, the run record, and the refusals that need a hub.
// It is Tier 2: it builds a real hub and spawns git, the stand-in and a few real go test runs, so it needs the integration build tag.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

const (
	// helperRoleGate runs `lyx gate test` over helperCwdEnv and the process arguments, with the stand-in as its go binary unless helperGoEnv is empty.
	helperRoleGate = "gate"
	// helperRoleStandIn records how it was invoked, then exits with helperExitEnv or, under helperHoldEnv, sleeps until killed with a sleeper child.
	// A build invocation instead writes helperBuildContentEnv to its `-o` file when it succeeds and exits with helperBuildExitEnv.
	helperRoleStandIn = "standin"
	// helperRoleSleeper sleeps until killed.
	helperRoleSleeper = "sleeper"

	helperCwdEnv          = "GATECLI_TEST_CWD"
	helperGoEnv           = "GATECLI_TEST_GO"
	helperRecordEnv       = "GATECLI_TEST_RECORD"
	helperExitEnv         = "GATECLI_TEST_EXIT"
	helperBuildExitEnv    = "GATECLI_TEST_BUILD_EXIT"
	helperBuildContentEnv = "GATECLI_TEST_BUILD_CONTENT"
	helperHoldEnv         = "GATECLI_TEST_HOLD"
	// helperSlotWaitEnv, helperCeilingEnv and helperUnitEnv are durations the gate helper passes to newCommand in place of its defaults.
	helperSlotWaitEnv = "GATECLI_TEST_SLOT_WAIT"
	helperCeilingEnv  = "GATECLI_TEST_CEILING"
	helperUnitEnv     = "GATECLI_TEST_UNIT"
)

func init() {
	helperRoles[helperRoleGate] = runGateHelper
	helperRoles[helperRoleStandIn] = runStandIn
	helperRoles[helperRoleSleeper] = func() int { select {} }
}

// standInRecord is what the stand-in appends to helperRecordEnv, one line of JSON per invocation.
type standInRecord struct {
	// Args are the stand-in's arguments as the tier command or the prebuild passed them.
	Args []string `json:"args"`
	// Build is set for the `go build` of the prebuild, which the stand-in also stands for.
	Build   bool   `json:"build"`
	Cwd     string `json:"cwd"`
	GoFlags string `json:"goflags"`
	// Slot is the inherited-slot variable the stand-in saw.
	Slot string `json:"slot"`
	// Strand is the strand-name variable the stand-in saw; empty when the gate dropped it.
	Strand string `json:"strand"`
	// Prebuilt is the prebuilt-lyx variable the stand-in saw.
	Prebuilt string `json:"prebuilt"`
	// Enclosing is the enclosing-gate pid variable the stand-in saw.
	Enclosing string `json:"enclosing"`
	// Trace is the trace-id variable the stand-in saw; empty when the gate dropped it.
	Trace      string `json:"trace"`
	PID        int    `json:"pid"`
	Grandchild int    `json:"grandchild"`
}

func runGateHelper() int {
	// The helper stands for `lyx`'s main, whose logs reach the real stderr; under go test the logger defaults to discarding them.
	logger.SetOutput(os.Stderr)
	logger.SetVerbosity(1)
	// The tier commands and the prebuild run this same binary, in the stand-in role.
	os.Setenv(helperEnv, helperRoleStandIn)
	goBinary := defaultGoBinary
	if standIn := os.Getenv(helperGoEnv); standIn != "" {
		goBinary = standIn
	}
	durations := map[string]time.Duration{helperSlotWaitEnv: time.Second, helperCeilingEnv: 2 * time.Minute, helperUnitEnv: time.Minute}
	for name := range durations {
		if value := os.Getenv(name); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return 1
			}
			durations[name] = parsed
		}
	}
	code := clihelp.ExecuteIn(newCommand(goBinary, time.Second, durations[helperSlotWaitEnv], durations[helperCeilingEnv], durations[helperUnitEnv]), os.Getenv(helperCwdEnv), os.Stdout, os.Args[1:])
	// The helper stands for `lyx`'s main, which removes the process run directory on exit.
	_ = lyxtemp.Remove()
	return code
}

func runStandIn() int {
	cwd, _ := os.Getwd()
	isBuild := len(os.Args) > 1 && os.Args[1] == "build"
	record := standInRecord{Args: os.Args[1:], Build: isBuild, Cwd: cwd, GoFlags: os.Getenv("GOFLAGS"), Slot: os.Getenv(gateslot.InheritEnv), Strand: os.Getenv(agentname.StrandNameEnv), Prebuilt: os.Getenv(gateslot.PrebuiltLyxEnv), Enclosing: os.Getenv(enclosingGateEnv), Trace: os.Getenv(logger.TraceIDEnv), PID: os.Getpid()}
	// A build invocation only records itself and exits with its own code; the hold belongs to the tier's invocation.
	hold := !isBuild && os.Getenv(helperHoldEnv) != ""
	if hold {
		sleeper := exec.Command(os.Args[0])
		sleeper.Env = append(os.Environ(), helperEnv+"="+helperRoleSleeper)
		if err := sleeper.Start(); err != nil {
			return 1
		}
		record.Grandchild = sleeper.Process.Pid
	}
	data, err := json.Marshal(record)
	if err != nil {
		return 1
	}
	// One write of one whole line to an append-mode file, so a poller never reads half a record.
	file, err := os.OpenFile(os.Getenv(helperRecordEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 1
	}
	_, writeErr := file.Write(append(data, '\n'))
	if closeErr := file.Close(); writeErr != nil || closeErr != nil {
		return 1
	}
	if isBuild {
		code, _ := strconv.Atoi(os.Getenv(helperBuildExitEnv))
		if output := slices.Index(os.Args, "-o"); code == 0 && output >= 0 && output+1 < len(os.Args) {
			if err := os.WriteFile(os.Args[output+1], []byte(os.Getenv(helperBuildContentEnv)), 0o755); err != nil {
				return 1
			}
		}
		return code
	}
	if hold {
		select {}
	}
	code, _ := strconv.Atoi(os.Getenv(helperExitEnv))
	return code
}

// strandEnvEntry is the strand-name entry a gate run is started with, which the gate must not hand to its tier commands.
var strandEnvEntry = agentname.StrandNameEnv + "=gate-test-strand"

// gateRun is one `lyx gate test` process to start.
type gateRun struct {
	cwd  string
	args []string
	// env holds extra KEY=VALUE entries for the gate process, and so for its tier commands.
	env []string
	// realGo makes the prebuild run the real go command instead of the stand-in.
	realGo bool
}

// runningGate is a started gate process and the files and buffers its run leaves.
type runningGate struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	record string
	// exit is the exit code of a gate a helper has waited for.
	exit int
}

// startGate starts the test binary in the gate role and returns it running.
func startGate(t *testing.T, run gateRun) *runningGate {
	t.Helper()

	g := &runningGate{record: filepath.Join(t.TempDir(), "standin.json")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	g.cmd = exec.CommandContext(ctx, os.Args[0], run.args...)
	// The outer environment's slot or enclosing gate, if any, must not make the gate think it is nested.
	g.cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, gateslot.InheritEnv+"=") || strings.HasPrefix(entry, enclosingGateEnv+"=")
	})
	g.cmd.Env = append(g.cmd.Env, helperEnv+"="+helperRoleGate, helperCwdEnv+"="+run.cwd, helperRecordEnv+"="+g.record)
	if !run.realGo {
		g.cmd.Env = append(g.cmd.Env, helperGoEnv+"="+os.Args[0])
	}
	g.cmd.Env = append(g.cmd.Env, run.env...)
	// A child that outlives its gate keeps the output pipe open, and the delay lets Wait return so the test reports that instead of hanging.
	g.cmd.WaitDelay = 5 * time.Second
	g.cmd.Stdout = &g.stdout
	g.cmd.Stderr = &g.stderr
	if err := g.cmd.Start(); err != nil {
		t.Fatalf("start gate: %v", err)
	}
	t.Cleanup(func() { _ = g.cmd.Process.Kill() })
	return g
}

// wait waits for the gate to end and returns its exit code.
func (g *runningGate) wait(t *testing.T) int {
	t.Helper()

	err := g.cmd.Wait()
	if err == nil {
		return 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("wait gate: %v", err)
	}
	return exitErr.ExitCode()
}

// stop ends a gate that holds its tier, with a catchable signal where the platform has one so the gate kills its tier's whole tree, and waits for it.
func (g *runningGate) stop(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		_ = g.cmd.Process.Kill()
	} else if err := g.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	g.wait(t)
}

// records returns every complete record the stand-in has appended so far, in invocation order.
func (g *runningGate) records(t *testing.T) []standInRecord {
	t.Helper()

	data, err := os.ReadFile(g.record)
	if err != nil {
		return nil
	}
	var records []standInRecord
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		var record standInRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("stand-in record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

// tierRecords returns the stand-in's tier invocations so far, in order, leaving out a build.
func (g *runningGate) tierRecords(t *testing.T) []standInRecord {
	t.Helper()

	return slices.DeleteFunc(g.records(t), func(record standInRecord) bool { return record.Build })
}

// standIn waits for the stand-in to record its first tier invocation and returns the record.
func (g *runningGate) standIn(t *testing.T) standInRecord {
	t.Helper()

	var record standInRecord
	eventually(t, "the stand-in's tier record", func() bool {
		if records := g.tierRecords(t); len(records) > 0 {
			record = records[0]
			return true
		}
		return false
	})
	return record
}

// tierArgs returns the arguments of each tier invocation the stand-in recorded, in order.
func (g *runningGate) tierArgs(t *testing.T) [][]string {
	t.Helper()

	var invocations [][]string
	for _, record := range g.tierRecords(t) {
		invocations = append(invocations, record.Args)
	}
	return invocations
}

// eventually polls ok until it holds, failing the test after a generous bound.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()

	for deadline := time.Now().Add(30 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// runGate runs the gate to its end and returns its exit code and stdout.
func runGate(t *testing.T, run gateRun) (int, string) {
	t.Helper()

	g := startGate(t, run)
	code := g.wait(t)
	return code, g.stdout.String()
}

// processGone reports whether pid has ended; a zombie nobody reaped counts as ended.
func processGone(pid int) bool {
	state, ok := tmuxkit.ProcState(pid)
	if !ok {
		return !proc.IsAlive(pid)
	}
	return state == "Z"
}

// summaryLogPath returns the log path the last summary line of tier in output names.
// The gate helper hands the verb's stdout as its error stream, so the summary lines are in stdout.
func summaryLogPath(t *testing.T, output string, tier int) string {
	t.Helper()

	prefix := "gate test: tier" + strconv.Itoa(tier) + " "
	var summary string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			summary = line
		}
	}
	_, logPath, found := strings.Cut(summary, " log ")
	if !found {
		t.Fatalf("output %q holds no summary line of tier%d", output, tier)
	}
	return logPath
}

// runRecordOf reads the run record and log that the last summary line of tier in output names.
func runRecordOf(t *testing.T, output string, tier int) (record gateslot.RunRecord, log string) {
	t.Helper()

	logPath := summaryLogPath(t, output, tier)
	data, err := os.ReadFile(strings.TrimSuffix(logPath, ".log") + ".json")
	if err != nil {
		t.Fatalf("read the run record of tier%d: %v", tier, err)
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("run record %s: %v", data, err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read the run log of tier%d: %v", tier, err)
	}
	return record, string(logData)
}

// scenarioConfig returns the gate.yaml the scenario's hub holds, after mutate has changed it.
// tier0 runs the stand-in over targets and args with a GOFLAGS of its own, tier1 holds every slot, tier2 holds no placeholder, tier3 prebuilds lyx, tier5 runs a real go test, tier6 is the guard tier and tier7 replaces its shell with the stand-in.
func scenarioConfig(t *testing.T, mutate func(cfg map[string]any)) []byte {
	t.Helper()

	standIn := "'" + os.Args[0] + "'"
	tier := func(command string, extra map[string]any) map[string]any {
		tier := map[string]any{"command": command, "all_targets": "./pkg", "timeout_min": 1, "counter": "go-json"}
		for key, value := range extra {
			tier[key] = value
		}
		return tier
	}
	cfg := map[string]any{
		"slots":      2,
		"scoper":     "go",
		"guard_tier": 6,
		"tiers": map[string]any{
			"tier0": tier(standIn+" {targets} {args}", map[string]any{"parallel": 3, "env": map[string]string{"GOFLAGS": "-trimpath -p={parallel}"}}),
			"tier1": tier(standIn+" {targets} {args}", map[string]any{"slots": "all"}),
			"tier2": tier(standIn+" whole", nil),
			"tier3": tier(standIn+" {targets} {args}", map[string]any{"parallel": 3, "prebuilt_lyx": true}),
			"tier5": tier("go test -json {targets} {args}", map[string]any{"all_targets": "./..."}),
			"tier6": tier(standIn+" {targets} {args}", nil),
			"tier7": tier("exec "+standIn+" {targets} {args}", nil),
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGateTest_Scenario(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	configPath := configengine.ConfigFile(h.BoardDir(), "gate")
	gateConfig := scenarioConfig(t, nil)
	writeConfig := func(t *testing.T, data []byte) {
		t.Helper()
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(t, gateConfig)
	prime := h.PrimeWorktree()
	pool := hubgeom.GateSlots(h.Location)
	waitDir := gateslot.WaitDir(h.Location.AnchorPath())

	requireFree := func(t *testing.T) {
		t.Helper()
		eventually(t, "the slots to be free", func() bool {
			holders, err := pool.Holders()
			return err == nil && len(holders) == 0
		})
		if waits, err := gateslot.ReadWaits(waitDir); err != nil || len(waits) != 0 {
			t.Errorf("ReadWaits = (%+v, %v); want no wait record left", waits, err)
		}
	}
	acquire := func(t *testing.T, site string) *gateslot.Lease {
		t.Helper()
		lease, err := pool.Acquire(context.Background(), gateslot.Holder{Worktree: "/other/worktree", Site: site})
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		t.Cleanup(func() { _ = lease.Release() })
		return lease
	}
	// writeModule writes a nested module named name under the prime holding files, and returns its directory.
	writeModule := func(t *testing.T, name string, files map[string]string) string {
		t.Helper()
		root := filepath.Join(prime, name)
		files["go.mod"] = "module example.com/" + name + "\n\ngo 1.21\n"
		for rel, content := range files {
			full := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	t.Run("a free slot runs the tier under its env, exits with its code and records the run", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg", "--", "-run", "X"}, env: []string{helperExitEnv + "=7", strandEnvEntry, logger.TraceIDEnv + "=0123456789abcdef"}})
		if code := g.wait(t); code != 7 {
			t.Errorf("exit code = %d; want the stand-in's 7", code)
		}
		record := g.standIn(t)
		if record.Strand != "" {
			t.Errorf("tier environment %s = %q; want it dropped", agentname.StrandNameEnv, record.Strand)
		}
		if record.Trace != "" {
			t.Errorf("tier environment %s = %q; want it dropped", logger.TraceIDEnv, record.Trace)
		}
		if want := strconv.Itoa(g.cmd.Process.Pid); record.Enclosing != want {
			t.Errorf("tier environment %s = %q; want the gate's pid %s", enclosingGateEnv, record.Enclosing, want)
		}
		if want := []string{"./pkg", "-run", "X"}; !slices.Equal(record.Args, want) {
			t.Errorf("tier args = %q; want %q", record.Args, want)
		}
		if record.GoFlags != "-trimpath -p=3" || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("tier environment GOFLAGS = %q, %s = %q; want the tier's own GOFLAGS and a slot of %s", record.GoFlags, gateslot.InheritEnv, record.Slot, pool.Dir)
		}
		if record.Cwd != prime {
			t.Errorf("tier cwd = %q; want the tier's dir under the worktree root %q", record.Cwd, prime)
		}

		run, log := runRecordOf(t, g.stdout.String(), 0)
		if run.Worktree != prime || run.Slug != "" || run.Tier != 0 || !slices.Equal(run.TierList, []int{0}) || run.ScopeForm != "targets" || run.ExitCode != 7 || run.Outcome != "failed" || run.TimedOut || run.Discovered != 0 {
			t.Errorf("run record = %+v; want the prime's failed tier0 run over targets, exit 7, nothing discovered", run)
		}
		if got, want := filepath.Dir(summaryLogPath(t, g.stdout.String(), 0)), gateslot.RunLogDir(gateslot.Dir(h.BoardDir())); got != want {
			t.Errorf("run record directory = %s; want the board's %s", got, want)
		}
		if log != "" {
			t.Errorf("run log = %q; want the stand-in's empty output", log)
		}
		requireFree(t)
	})

	t.Run("every slot held past the wait cap answers slot_free false naming the holders and exits SlotBusyExit", func(t *testing.T) {
		acquire(t, "holder one")
		acquire(t, "holder two")
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}, env: []string{helperSlotWaitEnv + "=50ms"}})
		code := g.wait(t)

		var envelope struct {
			OK       bool                `json:"ok"`
			SlotFree *bool               `json:"slot_free"`
			Message  string              `json:"message"`
			Holders  []map[string]string `json:"holders"`
		}
		if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
		}
		var sites []string
		for _, holder := range envelope.Holders {
			sites = append(sites, holder["site"])
		}
		slices.Sort(sites)
		if code != SlotBusyExit || !envelope.OK || envelope.SlotFree == nil || *envelope.SlotFree || !strings.Contains(envelope.Message, "run the same command again in a turn that does nothing else") || !slices.Equal(sites, []string{"holder one", "holder two"}) {
			t.Errorf("gate = exit %d, %+v; want exit %d, ok true, slot_free false, a re-run message and both holders", code, envelope, SlotBusyExit)
		}
		if _, err := os.Stat(g.record); err == nil {
			t.Error("the stand-in ran although no slot was free")
		}
	})

	t.Run("--wait blocks past the cap until a slot frees", func(t *testing.T) {
		first, second := acquire(t, "holder one"), acquire(t, "holder two")
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--wait", "./pkg"}, env: []string{helperSlotWaitEnv + "=50ms"}})
		eventually(t, "the gate's wait record", func() bool {
			waits, err := gateslot.ReadWaits(waitDir)
			return err == nil && len(waits) == 1
		})
		ended := make(chan error, 1)
		go func() { ended <- g.cmd.Wait() }()
		// The cap is 50ms, so a gate that still waits several caps later has lifted it.
		select {
		case err := <-ended:
			t.Fatalf("gate ended with %v while every slot was held; want it to keep waiting", err)
		case <-time.After(500 * time.Millisecond):
		}
		if err := first.Release(); err != nil {
			t.Fatal(err)
		}
		if err := <-ended; err != nil {
			t.Fatalf("gate after a slot freed: %v, stdout %q; want exit 0", err, g.stdout.String())
		}
		if err := second.Release(); err != nil {
			t.Fatal(err)
		}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, [][]string{{"./pkg"}}) {
			t.Errorf("tier invocations = %q; want the one run once the slot freed", got)
		}
		requireFree(t)
	})

	t.Run("a tier with slots all needs every slot and holds them", func(t *testing.T) {
		held := acquire(t, "holder")
		busy := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "1", "./pkg"}, env: []string{helperSlotWaitEnv + "=50ms"}})
		if code := busy.wait(t); code != SlotBusyExit {
			t.Errorf("exit code with one of two slots held = %d, stdout %q; want %d", code, busy.stdout.String(), SlotBusyExit)
		}
		if err := held.Release(); err != nil {
			t.Fatal(err)
		}

		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "1", "./pkg"}, env: []string{helperHoldEnv + "=1"}})
		record := g.standIn(t)
		t.Cleanup(func() { _ = (&os.Process{Pid: record.Grandchild}).Kill() })
		holders, err := pool.Holders()
		if err != nil || len(holders) != 2 || holders[0].Tier != "tier1" || holders[1].Tier != "tier1" {
			t.Errorf("Holders = (%+v, %v); want both slots held by tier1", holders, err)
		}
		g.stop(t)
		requireFree(t)
	})

	t.Run("an inherited held slot runs without acquiring", func(t *testing.T) {
		holder := startGate(t, gateRun{cwd: prime, args: []string{"test", "./held"}, env: []string{helperHoldEnv + "=1"}})
		held := holder.standIn(t)
		holders, err := pool.Holders()
		if err != nil || len(holders) != 1 || holders[0].Worktree != prime || holders[0].Tier != "tier0" || holders[0].Site != "lyx gate test tier0 ./held" {
			t.Errorf("Holders = (%+v, %v); want the holding gate's worktree, tier and site", holders, err)
		}

		nested := startGate(t, gateRun{cwd: prime, args: []string{"test", "./nested"}, env: []string{gateslot.InheritEnv + "=" + held.Slot, strandEnvEntry}})
		if code := nested.wait(t); code != 0 {
			t.Errorf("nested exit code = %d, stdout %q; want 0 inside the held slot", code, nested.stdout.String())
		}
		nestedRecord := nested.standIn(t)
		if nestedRecord.Slot != held.Slot {
			t.Errorf("nested %s = %q; want the held slot %q", gateslot.InheritEnv, nestedRecord.Slot, held.Slot)
		}
		if nestedRecord.Strand != "" {
			t.Errorf("nested tier environment %s = %q; want it dropped", agentname.StrandNameEnv, nestedRecord.Strand)
		}
		if !strings.Contains(nested.stderr.String(), "CPU watchdog started") {
			t.Errorf("nested stderr = %q; want the watchdog's start log, an inherited slot being no enclosing run", nested.stderr.String())
		}

		// The gate is a child of this process, so a value naming this process names a live ancestor of the gate.
		enclosed := startGate(t, gateRun{cwd: prime, args: []string{"test", "./enclosed"}, env: []string{gateslot.InheritEnv + "=" + held.Slot, enclosingGateEnv + "=" + strconv.Itoa(os.Getpid())}})
		if code := enclosed.wait(t); code != 0 {
			t.Errorf("enclosed exit code = %d, stdout %q; want 0 inside the held slot", code, enclosed.stdout.String())
		}
		if stderr := enclosed.stderr.String(); !strings.Contains(stderr, "enclosing lyx gate test") || strings.Contains(stderr, "CPU watchdog started") {
			t.Errorf("enclosed stderr = %q; want the enclosing-run reason and no watchdog start", stderr)
		}

		holder.stop(t)
		_ = (&os.Process{Pid: held.Grandchild}).Kill()
	})

	t.Run("-C names a worktree from a cwd outside the hub and the slot is taken", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", prime, "./pkg"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		record := g.standIn(t)
		if record.Cwd != prime || !slices.Equal(record.Args, []string{"./pkg"}) || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("tier cwd %q, args %q, slot %q; want %q running ./pkg inside a slot of %s", record.Cwd, record.Args, record.Slot, prime, pool.Dir)
		}
	})

	nestedModule := filepath.Join(prime, "nested")
	t.Run("-C names a nested module directory inside a hub worktree and the slot is taken", func(t *testing.T) {
		if err := os.MkdirAll(nestedModule, 0o755); err != nil {
			t.Fatal(err)
		}
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", nestedModule, "./..."}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		record := g.standIn(t)
		if record.Cwd != nestedModule || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("tier cwd %q, slot %q; want %s inside a slot of %s", record.Cwd, record.Slot, nestedModule, pool.Dir)
		}
	})

	t.Run("a real go test tier in the nested module passes with nothing discovered", func(t *testing.T) {
		files := map[string]string{
			"go.mod":         "module example.com/nested\n\ngo 1.21\n",
			"nested.go":      "package nested\n\nfunc Add(a, b int) int { return a + b }\n",
			"nested_test.go": "package nested\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n",
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(nestedModule, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout := runGate(t, gateRun{cwd: t.TempDir(), realGo: true, args: []string{"test", "--tier", "5", "-C", nestedModule, "./..."}})
		if code != 0 || !strings.Contains(stdout, "ok") {
			t.Errorf("real go test tier = exit %d, %q; want 0 and go's ok line", code, stdout)
		}
		if run, _ := runRecordOf(t, stdout, 5); run.Discovered != 0 || run.Outcome != "passed" {
			t.Errorf("run record = %+v; want a pass with nothing discovered", run)
		}
		requireFree(t)
	})

	t.Run("a test binary that keeps cores busy past busy_quit_sec is quit with its stacks printed", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("the CPU watchdog samples /proc, which only Linux has")
		}
		spinModule := writeModule(t, "spin", map[string]string{
			"spin_test.go": "package spin\n\nimport (\n\t\"sync/atomic\"\n\t\"testing\"\n)\n\nvar sink atomic.Uint64\n\nfunc spinForever() {\n\tfor {\n\t\tsink.Add(1)\n\t}\n}\n\nfunc TestSpin(t *testing.T) {\n\tfor i := 0; i < 4; i++ {\n\t\tgo spinForever()\n\t}\n\tspinForever()\n}\n",
		})
		writeConfig(t, scenarioConfig(t, func(cfg map[string]any) { cfg["busy_quit_sec"] = 1 }))
		defer writeConfig(t, gateConfig)

		started := time.Now()
		g := startGate(t, gateRun{cwd: t.TempDir(), realGo: true, args: []string{"test", "--tier", "5", "-C", spinModule, "./..."}})
		code := g.wait(t)
		elapsed := time.Since(started)

		stdout := g.stdout.String()
		if code == 0 || !strings.Contains(stdout, "SIGQUIT: quit") || !strings.Contains(stdout, "goroutine ") {
			t.Errorf("spin run = exit %d, stdout %q; want a non-zero exit and the Go runtime's SIGQUIT stacks", code, stdout)
		}
		if window, ok := gateslot.FindWatchdogQuit(stdout); !ok || window != "1s" {
			t.Errorf("FindWatchdogQuit(stdout) = %q, %v; want the run's own 1s window, which the template's 120s cannot produce", window, ok)
		}
		if elapsed > time.Minute {
			t.Errorf("spin run took %s; want the quit to end it long before the tier's timeout", elapsed)
		}
		requireFree(t)
	})

	t.Run("a two-tier call runs both tiers in turn", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "6", "--tier", "0", "./pkg"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, [][]string{{"./pkg"}, {"./pkg"}}) {
			t.Errorf("tier invocations = %q; want one per tier", got)
		}
		for _, tier := range []int{0, 6} {
			if run, _ := runRecordOf(t, g.stdout.String(), tier); !slices.Equal(run.TierList, []int{0, 6}) {
				t.Errorf("tier%d run record tier list = %v; want [0 6]", tier, run.TierList)
			}
		}
	})

	t.Run("a two-tier call stops at the first failing tier", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "0", "--tier", "6", "./pkg"}, env: []string{helperExitEnv + "=7"}})
		if code := g.wait(t); code != 7 {
			t.Errorf("exit code = %d; want the failing tier's 7", code)
		}
		if got := g.tierArgs(t); len(got) != 1 {
			t.Errorf("tier invocations = %q; want tier6 not run after tier0 failed", got)
		}
		requireFree(t)
	})

	t.Run("a command without {targets} runs whole under explicit targets", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "2", "./pkg"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, [][]string{{"whole"}}) {
			t.Errorf("tier invocations = %q; want the command as written", got)
		}
		if !strings.Contains(g.stdout.String(), "the command of tier2 holds no {targets}") {
			t.Errorf("stdout = %q; want the notice that the command runs whole", g.stdout.String())
		}
	})

	t.Run("a tier that outlives timeout_min is killed with the timeout line", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}, env: []string{helperHoldEnv + "=1", helperUnitEnv + "=1s"}})
		code := g.wait(t)
		record := g.standIn(t)
		t.Cleanup(func() { _ = (&os.Process{Pid: record.Grandchild}).Kill() })
		if tier, ok := gateslot.FindTierTimeout(g.stdout.String()); code != 1 || !ok || tier != "tier0" {
			t.Errorf("gate = exit %d, FindTierTimeout = (%q, %v), stdout %q; want exit 1 and the tier0 timeout line", code, tier, ok, g.stdout.String())
		}
		eventually(t, "the tier's command and its sleeper to end", func() bool { return processGone(record.PID) && processGone(record.Grandchild) })
		run, log := runRecordOf(t, g.stdout.String(), 0)
		if !run.TimedOut || run.ExitCode != 1 || !strings.Contains(log, "tier timeout") {
			t.Errorf("run record = %+v, log %q; want a timed-out run with the timeout line in its log", run, log)
		}
		requireFree(t)
	})

	t.Run("a catchable signal kills the tier's whole group and frees the slot", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("a catchable signal cannot be sent to a Windows process")
		}
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./held"}, env: []string{helperHoldEnv + "=1"}})
		record := g.standIn(t)
		if err := g.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code := g.wait(t); code != 128+int(syscall.SIGTERM) {
			t.Errorf("exit code = %d; want 128 plus SIGTERM", code)
		}
		eventually(t, "the tier's command and its sleeper to end", func() bool { return processGone(record.PID) && processGone(record.Grandchild) })
		requireFree(t)
	})

	t.Run("a SIGKILL of the gate ends the tier's command", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("only Linux gives the child a parent-death signal")
		}
		// Only the shell the gate started carries the parent-death signal, so the tier's command is the one `exec` replaces it with.
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "7", "./held"}, env: []string{helperHoldEnv + "=1"}})
		record := g.standIn(t)
		t.Cleanup(func() { _ = (&os.Process{Pid: record.Grandchild}).Kill() })
		if err := g.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		g.wait(t)
		eventually(t, "the tier's command to end with its killed parent", func() bool { return processGone(record.PID) })
		requireFree(t)
	})

	stale := gateslot.PrebuiltLyxEnv + "=/stale/lyx"
	t.Run("an untagged tier builds nothing and strips an inherited prebuilt variable", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}, env: []string{stale}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		records := g.records(t)
		if len(records) != 1 || records[0].Build || records[0].Prebuilt != "" {
			t.Errorf("records = %+v; want the tier invocation alone, with no %s", records, gateslot.PrebuiltLyxEnv)
		}
	})

	t.Run("a prebuilding tier in a root without cmd/lyx builds nothing and strips an inherited prebuilt variable", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "3", "./pkg"}, env: []string{stale}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		records := g.records(t)
		if len(records) != 1 || records[0].Build || records[0].Prebuilt != "" {
			t.Errorf("records = %+v; want the tier invocation alone, with no %s", records, gateslot.PrebuiltLyxEnv)
		}
	})

	t.Run("a prebuilding tier builds lyx once, exports it from the store under its content's name and removes the build", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(prime, "cmd", "lyx"), 0o755); err != nil {
			t.Fatal(err)
		}
		// buildAndExport runs one prebuilding gate whose stand-in build writes content, and returns the build path and the path the tier saw.
		buildAndExport := func(t *testing.T, content string) (bin, exported string) {
			t.Helper()
			g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tier", "3", "./pkg"}, env: []string{helperBuildContentEnv + "=" + content}})
			if code := g.wait(t); code != 0 {
				t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
			}
			records := g.records(t)
			if len(records) != 2 || !records[0].Build || len(records[0].Args) != 9 || records[1].Build {
				t.Fatalf("records = %+v; want one build invocation, then the tier invocation", records)
			}
			bin = records[0].Args[7]
			if want := []string{"build", "-C", prime, "-p", "3", "-trimpath", "-o", bin, "./cmd/lyx"}; !slices.Equal(records[0].Args, want) {
				t.Errorf("build args = %q; want %q", records[0].Args, want)
			}
			return bin, records[1].Prebuilt
		}

		bin, exported := buildAndExport(t, "lyx build one")
		if filepath.Dir(exported) != pool.PrebuiltDir() || !strings.HasPrefix(filepath.Base(exported), "lyx-") {
			t.Errorf("the tier saw %s = %q; want a lyx- entry of the store %s", gateslot.PrebuiltLyxEnv, exported, pool.PrebuiltDir())
		}
		if got, err := os.ReadFile(exported); err != nil || string(got) != "lyx build one" {
			t.Errorf("the stored binary %s = (%q, %v); want the build's content", exported, got, err)
		}
		if _, again := buildAndExport(t, "lyx build one"); again != exported {
			t.Errorf("a second run of the same build exported %q; want the same %q", again, exported)
		}
		if _, changed := buildAndExport(t, "lyx build two"); changed == exported || filepath.Dir(changed) != pool.PrebuiltDir() {
			t.Errorf("a changed build exported %q; want a store entry other than %q", changed, exported)
		}
		if _, err := os.Stat(filepath.Dir(bin)); !os.IsNotExist(err) {
			t.Errorf("the build directory %s survived the gate (stat err=%v); want it removed", filepath.Dir(bin), err)
		}
		// The gate runs in a helper process of its own, so its run directory is a sibling of this process's, made by the temp package's beside-the-parent rule.
		parentRun, err := lyxtemp.Dir()
		if err != nil {
			t.Fatal(err)
		}
		gateRunDir := filepath.Dir(filepath.Dir(bin))
		if filepath.Dir(gateRunDir) != filepath.Dir(parentRun) || gateRunDir == parentRun {
			t.Errorf("the prebuilt binary %s is not in a run directory beside %s", bin, parentRun)
		}
		if _, err := os.Stat(gateRunDir); !os.IsNotExist(err) {
			t.Errorf("the gate's run directory %s survived its exit (stat err=%v); want it removed", gateRunDir, err)
		}
	})

	// standalone returns a git worktree outside every hub, holding gate.yaml when withConfig is set.
	standalone := func(t *testing.T, withConfig bool) string {
		t.Helper()
		dir := t.TempDir()
		gitkit.Git(t, dir, "init", "-q")
		if withConfig {
			file := configengine.ConfigFile(dir, "gate")
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, scenarioConfig(t, func(cfg map[string]any) { cfg["scoper"] = "none" }), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	t.Run("a worktree outside every hub runs unslotted under its own gate.yaml and records under its own .lyx", func(t *testing.T) {
		worktree := standalone(t, true)
		g := startGate(t, gateRun{cwd: worktree, args: []string{"test", "./pkg"}, env: []string{strandEnvEntry}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		record := g.standIn(t)
		if record.Slot != "" || record.GoFlags != "-trimpath -p=3" || record.Cwd != worktree || record.Strand != "" {
			t.Errorf("tier slot %q, GOFLAGS %q, cwd %q, strand %q; want no slot, the tier's GOFLAGS, %q and no strand", record.Slot, record.GoFlags, record.Cwd, record.Strand, worktree)
		}
		if !strings.Contains(g.stderr.String(), "no hub bound applies") {
			t.Errorf("stderr = %q; want the log that no hub bound applies", g.stderr.String())
		}
		run, _ := runRecordOf(t, g.stdout.String(), 0)
		if run.Worktree != worktree || run.Slug != "" {
			t.Errorf("run record = %+v; want the standalone worktree and no slug", run)
		}
		if _, err := os.Stat(gateslot.RunLogDir(gateslot.WaitDir(worktree))); err != nil {
			t.Errorf("the worktree's own run log directory: %v", err)
		}
	})

	t.Run("a prebuilding tier outside every hub exports its own build path and removes it afterwards", func(t *testing.T) {
		worktree := standalone(t, true)
		if err := os.MkdirAll(filepath.Join(worktree, "cmd", "lyx"), 0o755); err != nil {
			t.Fatal(err)
		}
		g := startGate(t, gateRun{cwd: worktree, args: []string{"test", "--tier", "3", "./pkg"}, env: []string{helperBuildContentEnv + "=lyx build"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		records := g.records(t)
		if len(records) != 2 || !records[0].Build || len(records[0].Args) != 9 || records[1].Build {
			t.Fatalf("records = %+v; want one build invocation, then the tier invocation", records)
		}
		bin := records[0].Args[7]
		if records[1].Prebuilt != bin {
			t.Errorf("the tier saw %s = %q; want the build's own -o path %q", gateslot.PrebuiltLyxEnv, records[1].Prebuilt, bin)
		}
		if _, err := os.Stat(filepath.Dir(bin)); !os.IsNotExist(err) {
			t.Errorf("the build directory %s survived the gate (stat err=%v); want it removed", filepath.Dir(bin), err)
		}
	})

	// Each row breaks the shared hub in its setup and repairs it in the returned restore, so the rows run in order.
	refusals := []struct {
		name string
		// args defaults to `test ./pkg`.
		args  []string
		setup func(t *testing.T) (restore func())
		env   []string
		// wantError lists text the refusal must hold.
		wantError []string
	}{
		{
			name: "an absent gate.yaml names lyx fabric reconcile",
			setup: func(t *testing.T) func() {
				if err := os.Remove(configPath); err != nil {
					t.Fatal(err)
				}
				return func() { writeConfig(t, gateConfig) }
			},
			wantError: []string{`way forward: run "lyx fabric reconcile", then re-run the same command; a session lyx refuses the verb from reports status: FAILED and the orch runs it`},
		},
		{
			name: "a gate.yaml value below 1 names lyx config gate",
			setup: func(t *testing.T) func() {
				writeConfig(t, []byte("slots: 0\n"))
				return func() { writeConfig(t, gateConfig) }
			},
			wantError: []string{`fix it with "lyx config gate" from the prime`},
		},
		{
			name: "a machine tier_parallel below 1 names both files and both ways forward",
			setup: func(t *testing.T) func() {
				machinePath := configengine.MachineFile(h.BoardDir(), "gate")
				if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(machinePath, []byte("tier_parallel:\n  tier0: 0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return func() {
					if err := os.Remove(machinePath); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantError: []string{
				configengine.ConfigFile(h.BoardDir(), "gate"),
				configengine.MachineFile(h.BoardDir(), "gate"),
				"lyx config gate --layer machine",
				"by removing",
				"take the step it names, then re-run the same command",
				"a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them",
			},
		},
		{
			name: "a hub with no tier names lyx config gate",
			setup: func(t *testing.T) func() {
				writeConfig(t, []byte("slots: 2\n"))
				return func() { writeConfig(t, gateConfig) }
			},
			wantError: []string{"declares no tier0", `"lyx config gate"`, "reports status: FAILED"},
		},
		{
			name:      "an undeclared tier names lyx config gate",
			args:      []string{"test", "--tier", "9", "./pkg"},
			setup:     func(t *testing.T) func() { return func() {} },
			wantError: []string{"declares no tier9", `"lyx config gate"`},
		},
		{
			name:      "words after -- for a tier without {args} name lyx config gate",
			args:      []string{"test", "--tier", "2", "./pkg", "--", "-v"},
			setup:     func(t *testing.T) func() { return func() {} },
			wantError: []string{"command of tier2 holds no {args}", `"lyx config gate"`, "reports status: FAILED"},
		},
		{
			name: "a tier whose dir does not exist is a refusal naming the dir",
			setup: func(t *testing.T) func() {
				writeConfig(t, scenarioConfig(t, func(cfg map[string]any) {
					cfg["tiers"].(map[string]any)["tier0"].(map[string]any)["dir"] = "absent"
				}))
				return func() { writeConfig(t, gateConfig) }
			},
			wantError: []string{"could not start the shell for tier0", filepath.Join(prime, "absent")},
		},
		{
			name:      "work before the first tier outliving the ceiling names the re-run",
			setup:     func(t *testing.T) func() { return func() {} },
			env:       []string{helperCeilingEnv + "=1ns"},
			wantError: []string{"outlived its 1ns ceiling", "run the same command again"},
		},
		{
			name: "a slot directory that cannot be created is a re-run refusal",
			setup: func(t *testing.T) func() {
				aside := pool.Dir + ".aside"
				if err := os.MkdirAll(pool.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(pool.Dir, aside); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(pool.Dir, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				return func() {
					if err := os.Remove(pool.Dir); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(aside, pool.Dir); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantError: []string{"cannot acquire a gate slot"},
		},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			restore := tc.setup(t)
			args := tc.args
			if args == nil {
				args = []string{"test", "./pkg"}
			}
			g := startGate(t, gateRun{cwd: prime, args: args, env: tc.env})
			code := g.wait(t)
			restore()

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
			}
			if code != 1 || envelope.OK {
				t.Errorf("gate = exit %d, %+v; want exit 1 and ok false", code, envelope)
			}
			for _, want := range tc.wantError {
				if !strings.Contains(envelope.Error, want) {
					t.Errorf("gate error = %q; want it to hold %q", envelope.Error, want)
				}
			}
			if _, err := os.Stat(g.record); err == nil {
				t.Error("the stand-in ran although the gate refused")
			}
			requireFree(t)
		})
	}

	t.Run("a worktree outside every hub with no gate.yaml names lyx config gate", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: standalone(t, false), args: []string{"test", "./pkg"}})
		code := g.wait(t)
		if code != 1 || !strings.Contains(g.stdout.String(), `lyx config gate`) {
			t.Errorf("gate = exit %d, stdout %q; want exit 1 and a refusal naming lyx config gate", code, g.stdout.String())
		}
	})

	t.Run("a directory in no git worktree names lyx config gate run in one", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "./pkg"}})
		code := g.wait(t)
		if code != 1 || !strings.Contains(g.stdout.String(), "is in no git worktree") || !strings.Contains(g.stdout.String(), `lyx config gate`) {
			t.Errorf("gate = exit %d, stdout %q; want exit 1 and a refusal naming lyx config gate in a git worktree", code, g.stdout.String())
		}
	})

	guarded := writeModule(t, "guarded", map[string]string{
		"pkg/pkg.go":  "package pkg\n",
		"g/g.go":      "package g\n",
		"g/g_test.go": "package g\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuard(t *testing.T) {}\n",
		"h/h.go":      "package h\n",
		"h/h_test.go": "package h\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuardH(t *testing.T) {}\n",
	})
	guardRun := []string{"./g", "./h", "-run", "^(TestGuard|TestGuardH)$"}
	guardTests := []struct {
		name     string
		args     []string
		env      []string
		wantExit int
		want     [][]string
	}{
		{
			name: "named targets run before the guard tier runs over the guard packages by name",
			args: []string{"--guards", "./pkg"},
			want: [][]string{{"./pkg"}, guardRun},
		},
		{
			name: "words after -- precede the guard's own -run, which wins",
			args: []string{"--guards", "./pkg", "--", "-run", "X"},
			want: [][]string{{"./pkg", "-run", "X"}, {"./g", "./h", "-run", "X", "-run", "^(TestGuard|TestGuardH)$"}},
		},
		{
			name:     "a failing tier stops before the guards and returns its code",
			args:     []string{"--guards", "./pkg"},
			env:      []string{helperExitEnv + "=3"},
			wantExit: 3,
			want:     [][]string{{"./pkg"}},
		},
	}
	for _, tc := range guardTests {
		t.Run("--guards: "+tc.name, func(t *testing.T) {
			g := startGate(t, gateRun{cwd: t.TempDir(), args: append([]string{"test", "-C", guarded}, tc.args...), env: tc.env})
			if code := g.wait(t); code != tc.wantExit {
				t.Errorf("exit code = %d, stdout %q; want %d", code, g.stdout.String(), tc.wantExit)
			}
			if got := g.tierArgs(t); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tier invocations = %q; want %q", got, tc.want)
			}
			requireFree(t)
		})
	}

	t.Run("--guards: a module with no marked test runs the listed tier and no guard step", func(t *testing.T) {
		bare := writeModule(t, "bare", map[string]string{"pkg/pkg.go": "package pkg\n"})
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", bare, "--guards", "./pkg"}})
		if code := g.wait(t); code != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, [][]string{{"./pkg"}}) {
			t.Errorf("tier invocations = %q; want the listed tier alone", got)
		}
		requireFree(t)
	})

	t.Run("--guards: a guard in the module's root package runs as the . package", func(t *testing.T) {
		rooted := writeModule(t, "rooted", map[string]string{
			"root.go":      "package rooted\n",
			"root_test.go": "package rooted\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuardRoot(t *testing.T) {}\n",
		})
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", rooted, "--guards", "."}})
		if code := g.wait(t); code != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		want := [][]string{{"."}, {".", "-run", "^(TestGuardRoot)$"}}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, want) {
			t.Errorf("tier invocations = %q; want %q", got, want)
		}
		requireFree(t)
	})

	// Each row writes its own module, so a broken one never disturbs another; makeUnreadable removes a path's permissions until the row ends.
	makeUnreadable := func(t *testing.T, path string) {
		t.Helper()
		if os.Geteuid() == 0 || runtime.GOOS == "windows" {
			t.Skip("the platform or user reads any file")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
	}
	scanRefusals := []struct {
		name  string
		files map[string]string
		// unreadable is a module-relative path made unreadable before the run.
		unreadable string
		// wantError lists text the refusal must hold.
		wantError []string
		// wantModule is whether the refusal must also name the module directory.
		wantModule bool
	}{
		{
			name:      "a misplaced marker names its file and line and says to move it",
			files:     map[string]string{"p/p.go": "package p\n", "p/p_test.go": "package p\n\n//lyx:guard\nfunc helper() {}\n"},
			wantError: []string{"p/p_test.go:3", "move the marker directly above its func Test line"},
		},
		{
			name:      "a marker in a tmux file names its file and line and says to compile it under integration",
			files:     map[string]string{"p/p.go": "package p\n", "p/p_test.go": "//go:build tmux\n\npackage p\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestA(t *testing.T) {}\n"},
			wantError: []string{"p/p_test.go:7", "compile the marked test's file under the integration tag"},
		},
		{
			name:       "an unreadable test file names it and says to make it readable or fix it",
			files:      map[string]string{"p/p.go": "package p\n", "p/p_test.go": "package p\n"},
			unreadable: "p/p_test.go",
			wantError:  []string{"p/p_test.go", "make the file readable or fix it until it parses"},
		},
		{
			name:       "an unreadable package directory names the module and says to make go list succeed",
			files:      map[string]string{"p/p.go": "package p\n", "c/c.go": "package c\n"},
			unreadable: "c",
			wantError:  []string{`"go list ./..." succeeds there`},
			wantModule: true,
		},
	}
	for i, tc := range scanRefusals {
		t.Run("--guards: "+tc.name, func(t *testing.T) {
			module := writeModule(t, "scan"+strconv.Itoa(i), tc.files)
			if tc.unreadable != "" {
				makeUnreadable(t, filepath.Join(module, filepath.FromSlash(tc.unreadable)))
			}
			g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", module, "--guards", "./p"}})
			code := g.wait(t)

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
			}
			if code != 1 || envelope.OK {
				t.Errorf("gate = exit %d, %+v; want exit 1 and ok false", code, envelope)
			}
			wants := append(slices.Clone(tc.wantError), "way forward", "the guard step")
			if tc.wantModule {
				wants = append(wants, module)
			}
			for _, want := range wants {
				if !strings.Contains(envelope.Error, want) {
					t.Errorf("error %q lacks %q", envelope.Error, want)
				}
			}
			if _, err := os.Stat(g.record); err == nil {
				t.Error("the stand-in ran although the scan refused")
			}
			requireFree(t)
		})
	}
}
