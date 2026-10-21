//go:build integration

// These tests set the prebuilt-binary variable through t.Setenv, so none of them calls t.Parallel: the process environment is the global state they share.

package lyxbin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	helperRoleSleeper = "sleeper"
	helperRoleEcho    = "echo"

	// helperPidFileEnv names the file the sleeper role writes its own pid and its child's to.
	helperPidFileEnv = "LYXBIN_TEST_PIDFILE"
	// helperEchoEnv names the variable the echo role prints.
	helperEchoEnv = "LYXBIN_TEST_ECHO"
)

func init() {
	helperRoles[helperRoleSleeper] = runSleeper
	helperRoles[helperRoleEcho] = func() int {
		fmt.Print(os.Getenv(helperEchoEnv))
		return 0
	}
}

// runSleeper starts a `sleep` child, records its own pid and the child's in the pid file, and sleeps until killed.
// The pid file appears by rename, so a reader that sees it sees both pids.
func runSleeper() int {
	child := exec.Command("sleep", "600")
	if err := child.Start(); err != nil {
		return 2
	}
	pidFile := os.Getenv(helperPidFileEnv)
	pids := fmt.Sprintf("%d\n%d\n", os.Getpid(), child.Process.Pid)
	if err := os.WriteFile(pidFile+".tmp", []byte(pids), 0o600); err != nil {
		return 2
	}
	if err := os.Rename(pidFile+".tmp", pidFile); err != nil {
		return 2
	}
	select {}
}

// cancelOnceFileExists cancels ctx with ErrTimeout once path exists, or once limit passes, polling on a short tick.
// It returns early when ctx ends first.
func cancelOnceFileExists(ctx context.Context, cancel context.CancelCauseFunc, path string, limit time.Duration) {
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
	cancel(ErrTimeout)
}

func requireExecutable(t *testing.T, bin string) {
	t.Helper()
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("built binary %s is not executable: mode %v", bin, info.Mode())
	}
}

func TestBuild_ProducesExecutableBinary(t *testing.T) {
	t.Setenv(gateslot.PrebuiltLyxEnv, "")
	built := Build(t)

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"unset: two calls share one executable build", func(t *testing.T) {
			t.Setenv(gateslot.PrebuiltLyxEnv, "")
			first, second := Build(t), Build(t)
			if first != second {
				t.Errorf("second Build = %q; want the first call's %q", second, first)
			}
			requireExecutable(t, first)
		}},
		{"set: Build returns the named binary", func(t *testing.T) {
			data, err := os.ReadFile(built)
			if err != nil {
				t.Fatal(err)
			}
			copyPath := filepath.Join(t.TempDir(), "prebuilt-lyx")
			if err := os.WriteFile(copyPath, data, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv(gateslot.PrebuiltLyxEnv, copyPath)
			if got := Build(t); got != copyPath {
				t.Errorf("Build = %q; want the prebuilt %q", got, copyPath)
			}
		}},
		{"set to a missing path: BuildInto is stale", func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "gone")
			t.Setenv(gateslot.PrebuiltLyxEnv, missing)
			_, err := BuildInto(t.TempDir(), "")
			if !errors.Is(err, ErrStalePrebuilt) || !strings.Contains(err.Error(), missing) {
				t.Errorf("BuildInto error = %v; want ErrStalePrebuilt naming %q", err, missing)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

// TestBuildWithLDFlags_ReachTheBuild checks the ldflags through their effect: `-s -w` strips the symbol table and DWARF, so the binary is smaller than the unstripped Build one.
// The build info cannot show them, since a `-trimpath` build, which a gate run's GOFLAGS makes, omits its `-ldflags` setting.
func TestBuildWithLDFlags_ReachTheBuild(t *testing.T) {
	prebuiltPath := Build(t)
	unstripped, err := os.Stat(prebuiltPath)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		prebuilt string
	}{
		{"variable unset", ""},
		{"variable set", prebuiltPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(gateslot.PrebuiltLyxEnv, tt.prebuilt)
			bin := BuildWithLDFlags(t, "-s -w")

			stripped, err := os.Stat(bin)
			if err != nil {
				t.Fatal(err)
			}
			if stripped.Size() >= unstripped.Size() {
				t.Errorf("the -s -w build is %d bytes, the unstripped build %d; want the -s -w build smaller", stripped.Size(), unstripped.Size())
			}
		})
	}
}

func TestBuildInto_UnwritableDirReturnsError(t *testing.T) {
	t.Setenv(gateslot.PrebuiltLyxEnv, "")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	bin, err := BuildInto(filepath.Join(blocker, "out"), "")
	if err == nil {
		t.Fatalf("BuildInto into a path under a regular file succeeded: %s", bin)
	}
}

// TestRun_TimesOutNamingTheTimeout passes whether or not the sleeper starts within the timeout, since either way it outlives it.
func TestRun_TimesOutNamingTheTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the sleeper's child is the Unix `sleep` command")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperEnv, helperRoleSleeper)
	t.Setenv(helperPidFileEnv, filepath.Join(t.TempDir(), "pids"))

	out, code, err := Run(self, t.TempDir(), 200*time.Millisecond)

	if !errors.Is(err, ErrTimeout) || code != -1 || !strings.Contains(err.Error(), "after 200ms") {
		t.Errorf("Run = (%q, %d, %v); want ErrTimeout naming 200ms and exit code -1", out, code, err)
	}
}

// TestRun_KillsTheGroupOnTimeout ends the run only once the sleeper has recorded its pids,
// so the time a scanner holds the freshly built helper before it runs never counts against the timeout.
func TestRun_KillsTheGroupOnTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the group kill is asserted through /proc-backed liveness")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pids")
	t.Setenv(helperEnv, helperRoleSleeper)
	t.Setenv(helperPidFileEnv, pidFile)
	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(nil) })
	go cancelOnceFileExists(ctx, cancel, pidFile, 2*time.Minute)

	out, code, err := runUntilDone(ctx, self, t.TempDir(), nil)

	if !errors.Is(err, ErrTimeout) || code != -1 {
		t.Fatalf("runUntilDone = (%q, %d, %v); want ErrTimeout and exit code -1", out, code, err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the sleeper never recorded its pids: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Fatalf("pid file = %q; want the sleeper's pid and its child's", raw)
	}
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for proc.IsAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if proc.IsAlive(pid) {
			t.Errorf("pid %d survived the timeout", pid)
		}
	}
}

func TestRun_PassesTheEnvironment(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperEnv, helperRoleEcho)
	t.Setenv(helperEchoEnv, "from-the-test")

	out, code, err := Run(self, t.TempDir(), 30*time.Second)

	if err != nil || code != 0 || out != "from-the-test" {
		t.Errorf("Run = (%q, %d, %v); want the variable's value, exit code 0 and no error", out, code, err)
	}
}
