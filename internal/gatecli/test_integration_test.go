//go:build integration

// test_integration_test.go drives `lyx gate test` over one hubforge hub and a stand-in go binary, each run in a process of its own so a test can signal the gate it started:
// the slot, the wait bound, inheritance, hub resolution from -C, the unslotted run outside every hub, the child's lifetime under a catchable signal and a SIGKILL, and the refusals that need a hub.
// It is Tier 2: it builds a real hub and spawns git, the stand-in and one real go test, so it needs the integration build tag.

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

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
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
)

func init() {
	helperRoles[helperRoleGate] = runGateHelper
	helperRoles[helperRoleStandIn] = runStandIn
	helperRoles[helperRoleSleeper] = func() int { select {} }
}

// standInRecord is what the stand-in go binary appends to helperRecordEnv, one line of JSON per invocation.
type standInRecord struct {
	Args    []string `json:"args"`
	GoFlags string   `json:"goflags"`
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
	// The go binary the gate spawns is this same binary, in the stand-in role.
	os.Setenv(helperEnv, helperRoleStandIn)
	goBinary := defaultGoBinary
	if standIn := os.Getenv(helperGoEnv); standIn != "" {
		goBinary = standIn
	}
	code := clihelp.ExecuteIn(newCommand(goBinary, time.Second, time.Second), os.Getenv(helperCwdEnv), os.Stdout, os.Args[1:])
	// The helper stands for `lyx`'s main, which removes the process run directory on exit.
	_ = lyxtemp.Remove()
	return code
}

func runStandIn() int {
	record := standInRecord{Args: os.Args[1:], GoFlags: os.Getenv("GOFLAGS"), Slot: os.Getenv(gateslot.InheritEnv), Strand: os.Getenv(agentname.StrandNameEnv), Prebuilt: os.Getenv(gateslot.PrebuiltLyxEnv), Enclosing: os.Getenv(enclosingGateEnv), Trace: os.Getenv(logger.TraceIDEnv), PID: os.Getpid()}
	// A build invocation only records itself and exits with its own code; the hold belongs to the test invocation.
	isBuild := len(os.Args) > 1 && os.Args[1] == "build"
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

// strandEnvEntry is the strand-name entry a gate run is started with, which the gate must not hand to its go binary.
var strandEnvEntry = agentname.StrandNameEnv + "=gate-test-strand"

// gateRun is one `lyx gate test` process to start.
type gateRun struct {
	cwd  string
	args []string
	// env holds extra KEY=VALUE entries for the gate process, and so for its go binary.
	env []string
	// realGo runs the real go command instead of the stand-in.
	realGo bool
}

// runningGate is a started gate process and the files and buffers its run leaves.
type runningGate struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	record string
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

// standIn waits for the stand-in go binary to record its test invocation and returns the record.
func (g *runningGate) standIn(t *testing.T) standInRecord {
	t.Helper()

	var record standInRecord
	eventually(t, "the stand-in's test record", func() bool {
		for _, candidate := range g.records(t) {
			if len(candidate.Args) > 0 && candidate.Args[0] == "test" {
				record = candidate
				return true
			}
		}
		return false
	})
	return record
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

func TestGateTest_Scenario(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gateConfig := "slots: 1\ngo_parallel: 3\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateConfig), 0o644); err != nil {
		t.Fatalf("seed gate.yaml: %v", err)
	}
	prime := h.PrimeWorktree()
	// The package arguments the rows name must be directories, since the verb refuses one that is not.
	for _, pkg := range []string{"pkg", "held", "nested", "enclosed"} {
		if err := os.MkdirAll(filepath.Join(prime, pkg), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cwdWithPkg := func(t *testing.T) string {
		t.Helper()
		cwd := t.TempDir()
		if err := os.Mkdir(filepath.Join(cwd, "pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		return cwd
	}
	pool := hubgeom.GateSlots(h.Location)
	waitDir := gateslot.WaitDir(h.Location.AnchorPath())

	requireFree := func(t *testing.T) {
		t.Helper()
		eventually(t, "the slot to be free", func() bool {
			holders, err := pool.Holders()
			return err == nil && len(holders) == 0
		})
		if waits, err := gateslot.ReadWaits(waitDir); err != nil || len(waits) != 0 {
			t.Errorf("ReadWaits = (%+v, %v); want no wait record left", waits, err)
		}
	}

	t.Run("a free slot runs go test under the cap and exits with its code", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tags", "integration", "./pkg", "--", "-run", "X"}, env: []string{helperExitEnv + "=7", strandEnvEntry, logger.TraceIDEnv + "=0123456789abcdef"}})
		if code := g.wait(t); code != 7 {
			t.Errorf("exit code = %d; want the stand-in's 7", code)
		}
		record := g.standIn(t)
		if record.Strand != "" {
			t.Errorf("go environment %s = %q; want it dropped", agentname.StrandNameEnv, record.Strand)
		}
		if record.Trace != "" {
			t.Errorf("go environment %s = %q; want it dropped", logger.TraceIDEnv, record.Trace)
		}
		if want := strconv.Itoa(g.cmd.Process.Pid); record.Enclosing != want {
			t.Errorf("go environment %s = %q; want the gate's pid %s", enclosingGateEnv, record.Enclosing, want)
		}
		if want := []string{"test", "-C", prime, "-p", "3", "-tags", "integration", "./pkg", "-run", "X"}; !slices.Equal(record.Args, want) {
			t.Errorf("go args = %q; want %q", record.Args, want)
		}
		if !strings.Contains(record.GoFlags, "-p=3 -trimpath") || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("go environment GOFLAGS = %q, %s = %q; want the -p cap, -trimpath and a slot of %s", record.GoFlags, gateslot.InheritEnv, record.Slot, pool.Dir)
		}
		requireFree(t)
	})

	t.Run("every slot held past the wait cap answers slot_free false naming the holders and exits SlotBusyExit", func(t *testing.T) {
		held, err := pool.Acquire(context.Background(), gateslot.Holder{Worktree: "/other/worktree", Site: "holder site"})
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}})
		code := g.wait(t)
		if err := held.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}

		var envelope struct {
			OK       bool                `json:"ok"`
			SlotFree *bool               `json:"slot_free"`
			Message  string              `json:"message"`
			Holders  []map[string]string `json:"holders"`
		}
		if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
		}
		wantHolders := []map[string]string{{"worktree": "/other/worktree", "site": "holder site"}}
		if code != SlotBusyExit || !envelope.OK || envelope.SlotFree == nil || *envelope.SlotFree || !strings.Contains(envelope.Message, "run the same command again in a turn that does nothing else") || !slices.EqualFunc(envelope.Holders, wantHolders, func(a, b map[string]string) bool { return a["worktree"] == b["worktree"] && a["site"] == b["site"] }) {
			t.Errorf("gate = exit %d, %+v; want exit %d, ok true, slot_free false, a re-run message and holders %v", code, envelope, SlotBusyExit, wantHolders)
		}
		if _, err := os.Stat(g.record); err == nil {
			t.Error("the stand-in ran although no slot was free")
		}
		requireFree(t)
	})

	t.Run("an inherited held slot runs without acquiring", func(t *testing.T) {
		holder := startGate(t, gateRun{cwd: prime, args: []string{"test", "./held"}, env: []string{helperHoldEnv + "=1"}})
		held := holder.standIn(t)
		holders, err := pool.Holders()
		if err != nil || len(holders) != 1 || holders[0].Worktree != prime || holders[0].Site != "lyx gate test ./held" {
			t.Errorf("Holders = (%+v, %v); want the holding gate's worktree and site", holders, err)
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
			t.Errorf("nested go environment %s = %q; want it dropped", agentname.StrandNameEnv, nestedRecord.Strand)
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

		_ = holder.cmd.Process.Kill()
		holder.wait(t)
		_ = (&os.Process{Pid: held.Grandchild}).Kill()
	})

	t.Run("-C names a worktree from a cwd outside the hub and the slot is taken", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", prime, "./pkg"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		record := g.standIn(t)
		if want := []string{"test", "-C", prime, "-p", "3", "./pkg"}; !slices.Equal(record.Args, want) || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("go args = %q, slot %q; want %q inside a slot of %s", record.Args, record.Slot, want, pool.Dir)
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
		if record.Args[2] != nestedModule || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("go args = %q, slot %q; want -C %s inside a slot of %s", record.Args, record.Slot, nestedModule, pool.Dir)
		}
	})

	t.Run("a real go test runs in the nested module and exits with go's code", func(t *testing.T) {
		files := map[string]string{
			"go.mod":         "module example.com/nested\n\ngo 1.21\n",
			"nested.go":      "package nested\n\nfunc Add(a, b int) int { return a + b }\n",
			"nested_test.go": "package nested\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n",
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(nestedModule, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout := runGate(t, gateRun{cwd: t.TempDir(), realGo: true, args: []string{"test", "-C", nestedModule, "./..."}})
		if code != 0 || !strings.Contains(stdout, "ok") {
			t.Errorf("real go test = exit %d, %q; want 0 and go's ok line", code, stdout)
		}
		requireFree(t)
	})

	t.Run("a test binary that keeps cores busy past busy_quit_sec is quit with its stacks printed", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("the CPU watchdog samples /proc, which only Linux has")
		}
		spinModule := filepath.Join(prime, "spin")
		if err := os.MkdirAll(spinModule, 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"go.mod":       "module example.com/spin\n\ngo 1.21\n",
			"spin_test.go": "package spin\n\nimport (\n\t\"sync/atomic\"\n\t\"testing\"\n)\n\nvar sink atomic.Uint64\n\nfunc spinForever() {\n\tfor {\n\t\tsink.Add(1)\n\t}\n}\n\nfunc TestSpin(t *testing.T) {\n\tfor i := 0; i < 4; i++ {\n\t\tgo spinForever()\n\t}\n\tspinForever()\n}\n",
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(spinModule, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		configPath := configengine.ConfigFile(h.BoardDir(), "gate")
		if err := os.WriteFile(configPath, []byte(gateConfig+"busy_quit_sec: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.WriteFile(configPath, []byte(gateConfig), 0o644); err != nil {
				t.Fatal(err)
			}
		}()

		started := time.Now()
		g := startGate(t, gateRun{cwd: t.TempDir(), realGo: true, args: []string{"test", "-C", spinModule, "./..."}})
		code := g.wait(t)
		elapsed := time.Since(started)

		stdout := g.stdout.String()
		if code == 0 || !strings.Contains(stdout, "SIGQUIT: quit") || !strings.Contains(stdout, "goroutine ") {
			t.Errorf("spin run = exit %d, stdout %q; want a non-zero exit and the Go runtime's SIGQUIT stacks", code, stdout)
		}
		// The test helper hands the verb's stdout as its error stream, so the watchdog line is in stdout.
		if window, ok := gateslot.FindWatchdogQuit(stdout); !ok || window != "1s" {
			t.Errorf("FindWatchdogQuit(stdout) = %q, %v; want the run's own 1s window, which the template's 120s cannot produce", window, ok)
		}
		if elapsed > time.Minute {
			t.Errorf("spin run took %s; want the quit to end it long before go test's own timeout", elapsed)
		}
		requireFree(t)
	})

	t.Run("a target outside every hub runs unslotted and logs it", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: cwdWithPkg(t), args: []string{"test", "./pkg/...", "example.com/x", "./absent/a_test.go"}, env: []string{strandEnvEntry}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		record := g.standIn(t)
		if record.Slot != "" || !slices.Contains(record.Args, "4") {
			t.Errorf("go args = %q, slot %q; want the template's -p 4 and no slot", record.Args, record.Slot)
		}
		// A pattern, an import path and a .go file argument reach go test unread.
		if want := []string{"./pkg/...", "example.com/x", "./absent/a_test.go"}; len(record.Args) < len(want) || !slices.Equal(record.Args[len(record.Args)-len(want):], want) {
			t.Errorf("go args = %q; want them to end with %q", record.Args, want)
		}
		if record.Strand != "" {
			t.Errorf("go environment %s = %q; want it dropped", agentname.StrandNameEnv, record.Strand)
		}
		if !strings.Contains(g.stderr.String(), "no hub bound applies") {
			t.Errorf("stderr = %q; want the log that no hub bound applies", g.stderr.String())
		}
	})

	t.Run("a catchable signal kills the child's whole group and frees the slot", func(t *testing.T) {
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
		eventually(t, "the child and its sleeper to end", func() bool { return processGone(record.PID) && processGone(record.Grandchild) })
		requireFree(t)
	})

	t.Run("a SIGKILL of the gate ends its child", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("only Linux gives the child a parent-death signal")
		}
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./held"}, env: []string{helperHoldEnv + "=1"}})
		record := g.standIn(t)
		t.Cleanup(func() { _ = (&os.Process{Pid: record.Grandchild}).Kill() })
		if err := g.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		g.wait(t)
		eventually(t, "go to end with its killed parent", func() bool { return processGone(record.PID) })
		requireFree(t)
	})

	t.Run("a tagged run builds lyx once, exports it to go test from the store under its content's name and removes the build", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(prime, "cmd", "lyx"), 0o755); err != nil {
			t.Fatal(err)
		}
		// buildAndExport runs one tagged gate whose stand-in build writes content, and returns the build path and the path go test saw.
		buildAndExport := func(t *testing.T, content string) (bin, exported string) {
			t.Helper()
			g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tags", "integration", "./pkg"}, env: []string{helperBuildContentEnv + "=" + content}})
			if code := g.wait(t); code != 0 {
				t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
			}
			records := g.records(t)
			if len(records) != 2 || len(records[0].Args) != 9 || records[0].Args[0] != "build" || records[1].Args[0] != "test" {
				t.Fatalf("records = %+v; want one build invocation, then the test invocation", records)
			}
			bin = records[0].Args[7]
			if want := []string{"build", "-C", prime, "-p", "3", "-trimpath", "-o", bin, "./cmd/lyx"}; !slices.Equal(records[0].Args, want) {
				t.Errorf("build args = %q; want %q", records[0].Args, want)
			}
			return bin, records[1].Prebuilt
		}

		bin, exported := buildAndExport(t, "lyx build one")
		if filepath.Dir(exported) != pool.PrebuiltDir() || !strings.HasPrefix(filepath.Base(exported), "lyx-") {
			t.Errorf("go test saw %s = %q; want a lyx- entry of the store %s", gateslot.PrebuiltLyxEnv, exported, pool.PrebuiltDir())
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

	t.Run("a tagged run outside every hub exports its own build path and removes it afterwards", func(t *testing.T) {
		cwd := cwdWithPkg(t)
		if err := os.MkdirAll(filepath.Join(cwd, "cmd", "lyx"), 0o755); err != nil {
			t.Fatal(err)
		}
		g := startGate(t, gateRun{cwd: cwd, args: []string{"test", "--tags", "integration", "./pkg"}, env: []string{helperBuildContentEnv + "=lyx build"}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		records := g.records(t)
		if len(records) != 2 || len(records[0].Args) != 9 || records[0].Args[0] != "build" || records[1].Args[0] != "test" {
			t.Fatalf("records = %+v; want one build invocation, then the test invocation", records)
		}
		bin := records[0].Args[7]
		if records[1].Prebuilt != bin {
			t.Errorf("go test saw %s = %q; want the build's own -o path %q", gateslot.PrebuiltLyxEnv, records[1].Prebuilt, bin)
		}
		if _, err := os.Stat(filepath.Dir(bin)); !os.IsNotExist(err) {
			t.Errorf("the build directory %s survived the gate (stat err=%v); want it removed", filepath.Dir(bin), err)
		}
	})

	stale := gateslot.PrebuiltLyxEnv + "=/stale/lyx"
	t.Run("an untagged run builds nothing and strips an inherited prebuilt variable", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}, env: []string{stale}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		records := g.records(t)
		if len(records) != 1 || records[0].Args[0] != "test" || records[0].Prebuilt != "" {
			t.Errorf("records = %+v; want the test invocation alone, with no %s", records, gateslot.PrebuiltLyxEnv)
		}
	})

	t.Run("a tagged run in a root without cmd/lyx builds nothing and strips an inherited prebuilt variable", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: cwdWithPkg(t), args: []string{"test", "--tags", "integration", "./pkg"}, env: []string{stale}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		records := g.records(t)
		if len(records) != 1 || records[0].Args[0] != "test" || records[0].Prebuilt != "" {
			t.Errorf("records = %+v; want the test invocation alone, with no %s", records, gateslot.PrebuiltLyxEnv)
		}
	})

	t.Run("a tagged run whose lyx build fails refuses before go test", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tags", "integration", "./pkg"}, env: []string{helperBuildExitEnv + "=1"}})
		code := g.wait(t)
		var envelope struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
		}
		const wantError = "cannot build lyx for the tagged run"
		if code != 1 || envelope.OK || !strings.Contains(envelope.Error, wantError) {
			t.Errorf("gate = exit %d, %+v; want exit 1, ok false and an error holding %q", code, envelope, wantError)
		}
		if records := g.records(t); len(records) != 1 || records[0].Args[0] != "build" {
			t.Errorf("records = %+v; want the build invocation alone, with no test invocation after it", records)
		}
		requireFree(t)
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
			name:      "a missing package directory refuses before the slot and the prebuild",
			args:      []string{"test", "--tags", "integration", "./nope"},
			setup:     func(t *testing.T) func() { return func() {} },
			wantError: []string{"./nope"},
		},
		{
			name: "an absent gate.yaml names lyx fabric reconcile",
			setup: func(t *testing.T) func() {
				configPath := configengine.ConfigFile(h.BoardDir(), "gate")
				if err := os.Remove(configPath); err != nil {
					t.Fatal(err)
				}
				return func() {
					if err := os.WriteFile(configPath, []byte(gateConfig), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantError: []string{`way forward: run "lyx fabric reconcile", then re-run the same command; a session lyx refuses the verb from reports status: FAILED and the orch runs it`},
		},
		{
			name: "a gate.yaml value below 1 names lyx config gate",
			setup: func(t *testing.T) func() {
				configPath := configengine.ConfigFile(h.BoardDir(), "gate")
				if err := os.WriteFile(configPath, []byte("slots: 0\ngo_parallel: 3\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return func() {
					if err := os.WriteFile(configPath, []byte(gateConfig), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantError: []string{`fix it with "lyx config gate" from the prime`},
		},
		{
			name: "a machine go_parallel below 1 names both files and both ways forward",
			setup: func(t *testing.T) func() {
				machinePath := configengine.MachineFile(h.BoardDir(), "gate")
				if err := os.MkdirAll(filepath.Dir(machinePath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(machinePath, []byte("go_parallel: 0\n"), 0o644); err != nil {
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
		{
			name:      "a go binary that cannot start names putting go on PATH",
			setup:     func(t *testing.T) func() { return func() {} },
			env:       []string{helperGoEnv + "=" + filepath.Join(t.TempDir(), "absent-go")},
			wantError: []string{"put go on PATH"},
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
	// testInvocations returns the arguments of the stand-in's test invocations, in order, leaving out a build.
	testInvocations := func(t *testing.T, g *runningGate) [][]string {
		t.Helper()
		var invocations [][]string
		for _, record := range g.records(t) {
			if len(record.Args) > 0 && record.Args[0] == "test" {
				invocations = append(invocations, record.Args)
			}
		}
		return invocations
	}

	guarded := writeModule(t, "guarded", map[string]string{
		"pkg/pkg.go":  "package pkg\n",
		"g/g.go":      "package g\n",
		"g/g_test.go": "package g\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuard(t *testing.T) {}\n",
		"h/h.go":      "package h\n",
		"h/h_test.go": "package h\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuardH(t *testing.T) {}\n",
	})
	guardedInvocation := func(pkg string, flags ...string) []string {
		return append([]string{"test", "-C", guarded, "-p", "3", "-tags", "integration", pkg}, flags...)
	}
	guardTests := []struct {
		name     string
		args     []string
		env      []string
		wantExit int
		want     [][]string
	}{
		{
			name: "guards alone run each guard package by name in directory order",
			args: []string{"--guards"},
			want: [][]string{guardedInvocation("./g", "-run", "^(TestGuard)$"), guardedInvocation("./h", "-run", "^(TestGuardH)$")},
		},
		{
			name: "a named package runs before the guards",
			args: []string{"--guards", "./pkg"},
			want: [][]string{guardedInvocation("./pkg"), guardedInvocation("./g", "-run", "^(TestGuard)$"), guardedInvocation("./h", "-run", "^(TestGuardH)$")},
		},
		{
			name: "go test flags precede each guard's own -run, which wins",
			args: []string{"--guards", "./pkg", "--", "-run", "X"},
			want: [][]string{guardedInvocation("./pkg", "-run", "X"), guardedInvocation("./g", "-run", "X", "-run", "^(TestGuard)$"), guardedInvocation("./h", "-run", "X", "-run", "^(TestGuardH)$")},
		},
		{
			name:     "a failing package run stops before the guards and returns its code",
			args:     []string{"--guards", "./pkg"},
			env:      []string{helperExitEnv + "=3"},
			wantExit: 3,
			want:     [][]string{guardedInvocation("./pkg")},
		},
	}
	for _, tc := range guardTests {
		t.Run("--guards: "+tc.name, func(t *testing.T) {
			g := startGate(t, gateRun{cwd: t.TempDir(), args: append([]string{"test", "-C", guarded, "--tags", "integration"}, tc.args...), env: tc.env})
			if code := g.wait(t); code != tc.wantExit {
				t.Errorf("exit code = %d, stdout %q; want %d", code, g.stdout.String(), tc.wantExit)
			}
			if got := testInvocations(t, g); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("test invocations = %q; want %q", got, tc.want)
			}
			requireFree(t)
		})
	}

	t.Run("--guards: a module with no marked test and no package runs nothing and exits 0", func(t *testing.T) {
		bare := writeModule(t, "bare", map[string]string{"pkg/pkg.go": "package pkg\n"})
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", bare, "--tags", "integration", "--guards"}})
		if code := g.wait(t); code != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		if _, err := os.Stat(g.record); err == nil {
			t.Error("the stand-in ran although the module holds no guard test")
		}
		requireFree(t)
	})

	t.Run("--guards: a guard in the module's root package runs as the . package", func(t *testing.T) {
		rooted := writeModule(t, "rooted", map[string]string{
			"root.go":      "package rooted\n",
			"root_test.go": "package rooted\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuardRoot(t *testing.T) {}\n",
		})
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", rooted, "--tags", "integration", "--guards"}})
		if code := g.wait(t); code != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0", code, g.stdout.String())
		}
		want := [][]string{{"test", "-C", rooted, "-p", "3", "-tags", "integration", ".", "-run", "^(TestGuardRoot)$"}}
		if got := testInvocations(t, g); !reflect.DeepEqual(got, want) {
			t.Errorf("test invocations = %q; want %q", got, want)
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
			g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "-C", module, "--tags", "integration", "--guards"}})
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
			wants := append(slices.Clone(tc.wantError), "way forward")
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
