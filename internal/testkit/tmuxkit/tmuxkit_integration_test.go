//go:build tmux

package tmuxkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
)

func requireTmux(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("socket isolation is POSIX-only")
	}
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not on PATH")
	}
	return tmux
}

func hasServer(tmux, key string) bool {
	return exec.Command(tmux, "-L", key, "has-session").Run() == nil
}

func TestSocket_KillsServerAtCleanup(t *testing.T) {
	tmux := requireTmux(t)

	tests := []struct {
		name string
		key  func(t *testing.T) string
	}{
		{"minted by Socket", func(t *testing.T) string { return Socket(t, tmux) }},
		{"chosen by the test", func(t *testing.T) string {
			key := "lyxchosen-" + strconv.Itoa(os.Getpid())
			KillOnCleanup(t, tmux, key)
			return key
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var key string
			t.Run("owner", func(t *testing.T) {
				key = tt.key(t)
				if out, err := exec.Command(tmux, "-L", key, "new-session", "-d", "-s", "kit").CombinedOutput(); err != nil {
					t.Fatalf("start server: %v\n%s", err, out)
				}
				if !hasServer(tmux, key) {
					t.Fatal("server not running after new-session")
				}
				if _, err := os.Stat(socketPath(key)); err != nil {
					t.Fatalf("socket file missing while the server runs: %v", err)
				}
			})

			if hasServer(tmux, key) {
				t.Errorf("server on key %q survived the owning test's cleanup", key)
			}
			if _, err := os.Lstat(socketPath(key)); !os.IsNotExist(err) {
				t.Errorf("socket file of key %q survived the owning test's cleanup: %v", key, err)
			}
		})
	}
}

func TestSocket_LandsUnderMainDirectory(t *testing.T) {
	tmux := requireTmux(t)
	tmpdir := os.Getenv("TMUX_TMPDIR")
	if tmpdir == "" {
		t.Fatal("TMUX_TMPDIR unset: the package's TestMain does not run through Main")
	}

	key := Socket(t, tmux)
	if out, err := exec.Command(tmux, "-L", key, "new-session", "-d", "-s", "kit").CombinedOutput(); err != nil {
		t.Fatalf("start server: %v\n%s", err, out)
	}

	own := filepath.Join(socketDir(tmpdir, os.Getuid()), key)
	if _, err := os.Stat(own); err != nil {
		t.Errorf("socket not under Main's directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(socketDir(os.TempDir(), os.Getuid()), key)); err == nil {
		t.Errorf("socket %q also landed in the default directory", key)
	}
}

func TestSweep_KillsServersUnderItsDirectoryOnly(t *testing.T) {
	tmux := requireTmux(t)

	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	start := exec.Command(tmux, "-L", "swept", "new-session", "-d", "-s", "kit")
	start.Env = append(os.Environ(), "TMUX_TMPDIR="+dir)
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start server: %v\n%s", err, out)
	}
	has := exec.Command(tmux, "-L", "swept", "has-session")
	has.Env = start.Env
	if err := has.Run(); err != nil {
		t.Fatalf("server not running before the sweep: %v", err)
	}

	other := Socket(t, tmux)
	if out, err := exec.Command(tmux, "-L", other, "new-session", "-d", "-s", "kit").CombinedOutput(); err != nil {
		t.Fatalf("start second server: %v\n%s", err, out)
	}

	sweep(dir, os.Getuid())

	after := exec.Command(tmux, "-L", "swept", "has-session")
	after.Env = start.Env
	if after.Run() == nil {
		t.Error("server under the swept directory survived the sweep")
	}
	if !hasServer(tmux, other) {
		t.Error("the sweep killed a server outside its own directory")
	}
	if _, err := os.Lstat(filepath.Join(socketDir(dir, os.Getuid()), "swept")); !os.IsNotExist(err) {
		t.Errorf("socket file under the swept directory survived the sweep: %v", err)
	}
	if _, err := os.Lstat(socketPath(other)); err != nil {
		t.Errorf("the sweep removed a socket file outside its own directory: %v", err)
	}
}

// TestSocket_StartsHermeticNonLoginServer pins the pre-started server: it ignores the operator's ~/.tmux.conf, starts non-login panes, outlives its last session, carries the kit's marker and inherits none of the variables a pane may not.
// It sets HOME and the variables through t.Setenv, which are process-global state, so it does not call t.Parallel.
func TestSocket_StartsHermeticNonLoginServer(t *testing.T) {
	tmux := requireTmux(t)

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte("set -g @operator_marker leaked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	leaked := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "LYX_TRACE_ID", agentname.StrandNameEnv, agentname.ParentEnv}
	for _, name := range leaked {
		t.Setenv(name, "leaked")
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	key := Socket(t, tmux)
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(tmux, append([]string{"-L", key}, args...)...).Output()
		if err != nil {
			t.Fatalf("tmux %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}

	for option, want := range map[string]string{
		"@operator_marker": "",
		"default-command":  shell,
		"default-shell":    shell,
		"exit-empty":       "off",
		testServerOption:   "on",
	} {
		if got := run("show-options", "-gqv", option); got != want {
			t.Errorf("option %s = %q; want %q", option, got, want)
		}
	}

	run("new-session", "-d", "-s", "kit")
	if got := run("split-window", "-d", "-P", "-F", "#{pane_start_command}", "-t", "kit"); got != shell {
		t.Errorf("split pane start command = %q; want %q, which a login-shell pane leaves empty", got, shell)
	}

	for _, line := range strings.Split(run("show-environment", "-g"), "\n") {
		name, _, _ := strings.Cut(line, "=")
		if slices.Contains(leaked, name) {
			t.Errorf("server environment carries %q", line)
		}
	}
}

func TestPackageServer_OneKeyPerBinary(t *testing.T) {
	tmux := requireTmux(t)

	first := PackageServer(t, tmux)
	if second := PackageServer(t, tmux); second != first {
		t.Errorf("second call returned key %q; want the first call's %q", second, first)
	}
	if out, err := exec.Command(tmux, "-L", first, "list-sessions").CombinedOutput(); err != nil && !strings.Contains(string(out), "no sessions") {
		t.Errorf("package server does not answer list-sessions: %v\n%s", err, out)
	}
	registryMu.Lock()
	registered := registeredKeys[first]
	registryMu.Unlock()
	if !registered {
		t.Errorf("package server key %q is not registered", first)
	}
}
