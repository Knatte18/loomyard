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
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// helperRoleGate runs `lyx gate test` over helperCwdEnv and the process arguments, with the stand-in as its go binary unless helperGoEnv is empty.
	helperRoleGate = "gate"
	// helperRoleStandIn records how it was invoked, then exits with helperExitEnv or, under helperHoldEnv, sleeps until killed with a sleeper child.
	helperRoleStandIn = "standin"
	// helperRoleSleeper sleeps until killed.
	helperRoleSleeper = "sleeper"

	helperCwdEnv    = "GATECLI_TEST_CWD"
	helperGoEnv     = "GATECLI_TEST_GO"
	helperRecordEnv = "GATECLI_TEST_RECORD"
	helperExitEnv   = "GATECLI_TEST_EXIT"
	helperHoldEnv   = "GATECLI_TEST_HOLD"
)

func init() {
	helperRoles[helperRoleGate] = runGateHelper
	helperRoles[helperRoleStandIn] = runStandIn
	helperRoles[helperRoleSleeper] = func() int { select {} }
}

// standInRecord is what the stand-in go binary writes to helperRecordEnv once it runs.
type standInRecord struct {
	Args    []string `json:"args"`
	GoFlags string   `json:"goflags"`
	// Slot is the inherited-slot variable the stand-in saw.
	Slot string `json:"slot"`
	// Strand is the strand-name variable the stand-in saw; empty when the gate dropped it.
	Strand     string `json:"strand"`
	PID        int    `json:"pid"`
	Grandchild int    `json:"grandchild"`
}

func runGateHelper() int {
	logger.SetVerbosity(1)
	// The go binary the gate spawns is this same binary, in the stand-in role.
	os.Setenv(helperEnv, helperRoleStandIn)
	goBinary := defaultGoBinary
	if standIn := os.Getenv(helperGoEnv); standIn != "" {
		goBinary = standIn
	}
	return clihelp.ExecuteIn(newCommand(goBinary), os.Getenv(helperCwdEnv), os.Stdout, os.Args[1:])
}

func runStandIn() int {
	record := standInRecord{Args: os.Args[1:], GoFlags: os.Getenv("GOFLAGS"), Slot: os.Getenv(gateslot.InheritEnv), Strand: os.Getenv(agentname.StrandNameEnv), PID: os.Getpid()}
	hold := os.Getenv(helperHoldEnv) != ""
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
	// A rename makes the record appear whole, so a poller never reads half of it.
	path := os.Getenv(helperRecordEnv)
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return 1
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return 1
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
	// The outer environment's slot, if any, must not make the gate think it is nested.
	g.cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, gateslot.InheritEnv+"=") })
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

// standIn waits for the stand-in go binary to record its invocation and returns the record.
func (g *runningGate) standIn(t *testing.T) standInRecord {
	t.Helper()

	var data []byte
	eventually(t, "the stand-in's record", func() bool {
		var err error
		data, err = os.ReadFile(g.record)
		return err == nil
	})
	var record standInRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("stand-in record %q: %v", data, err)
	}
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
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return !proc.IsAlive(pid)
	}
	stat := string(data)
	return stat[strings.LastIndexByte(stat, ')')+2] == 'Z'
}

func TestGateTest_Scenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	gateConfig := "slots: 1\ngo_parallel: 3\ncli_wait_sec: 1\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateConfig), 0o644); err != nil {
		t.Fatalf("seed gate.yaml: %v", err)
	}
	prime := h.PrimeWorktree()
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
		g := startGate(t, gateRun{cwd: prime, args: []string{"test", "--tags", "integration", "./pkg", "--", "-run", "X"}, env: []string{helperExitEnv + "=7", strandEnvEntry}})
		if code := g.wait(t); code != 7 {
			t.Errorf("exit code = %d; want the stand-in's 7", code)
		}
		record := g.standIn(t)
		if record.Strand != "" {
			t.Errorf("go environment %s = %q; want it dropped", agentname.StrandNameEnv, record.Strand)
		}
		if want := []string{"test", "-C", prime, "-p", "3", "-tags", "integration", "./pkg", "-run", "X"}; !slices.Equal(record.Args, want) {
			t.Errorf("go args = %q; want %q", record.Args, want)
		}
		if !strings.Contains(record.GoFlags, "-p=3") || filepath.Dir(record.Slot) != pool.Dir {
			t.Errorf("go environment GOFLAGS = %q, %s = %q; want the -p cap and a slot of %s", record.GoFlags, gateslot.InheritEnv, record.Slot, pool.Dir)
		}
		requireFree(t)
	})

	t.Run("every slot held past the wait bound exits SlotBusyExit naming the holders", func(t *testing.T) {
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
			OK      bool                `json:"ok"`
			Error   string              `json:"error"`
			Holders []map[string]string `json:"holders"`
		}
		if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
		}
		wantHolders := []map[string]string{{"worktree": "/other/worktree", "site": "holder site"}}
		if code != SlotBusyExit || envelope.OK || !strings.Contains(envelope.Error, "re-run the same command") || !slices.EqualFunc(envelope.Holders, wantHolders, func(a, b map[string]string) bool { return a["worktree"] == b["worktree"] && a["site"] == b["site"] }) {
			t.Errorf("gate = exit %d, %+v; want exit %d, ok false, a re-run way forward and holders %v", code, envelope, SlotBusyExit, wantHolders)
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

	t.Run("a target outside every hub runs unslotted and logs it", func(t *testing.T) {
		g := startGate(t, gateRun{cwd: t.TempDir(), args: []string{"test", "./pkg"}, env: []string{strandEnvEntry}})
		if code := g.wait(t); code != 0 {
			t.Fatalf("exit code = %d; want 0", code)
		}
		record := g.standIn(t)
		if record.Slot != "" || !slices.Contains(record.Args, "4") {
			t.Errorf("go args = %q, slot %q; want the template's -p 4 and no slot", record.Args, record.Slot)
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

	// Each row breaks the shared hub in its setup and repairs it in the returned restore, so the rows run in order.
	refusals := []struct {
		name      string
		setup     func(t *testing.T) (restore func())
		env       []string
		wantError string
	}{
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
			wantError: `way forward: run "lyx fabric reconcile", then re-run the same command; a session lyx refuses the verb from reports status: FAILED and the orch runs it`,
		},
		{
			name: "a gate.yaml value below 1 names lyx config gate",
			setup: func(t *testing.T) func() {
				configPath := configengine.ConfigFile(h.BoardDir(), "gate")
				if err := os.WriteFile(configPath, []byte("slots: 0\ngo_parallel: 3\ncli_wait_sec: 1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return func() {
					if err := os.WriteFile(configPath, []byte(gateConfig), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantError: `fix the file with "lyx config gate" from the prime`,
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
			wantError: "cannot acquire a gate slot",
		},
		{
			name:      "a go binary that cannot start names putting go on PATH",
			setup:     func(t *testing.T) func() { return func() {} },
			env:       []string{helperGoEnv + "=" + filepath.Join(t.TempDir(), "absent-go")},
			wantError: "put go on PATH",
		},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			restore := tc.setup(t)
			g := startGate(t, gateRun{cwd: prime, args: []string{"test", "./pkg"}, env: tc.env})
			code := g.wait(t)
			restore()

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
			}
			if code != 1 || envelope.OK || !strings.Contains(envelope.Error, tc.wantError) {
				t.Errorf("gate = exit %d, %+v; want exit 1, ok false and an error holding %q", code, envelope, tc.wantError)
			}
			if _, err := os.Stat(g.record); err == nil {
				t.Error("the stand-in ran although the gate refused")
			}
			requireFree(t)
		})
	}
}
