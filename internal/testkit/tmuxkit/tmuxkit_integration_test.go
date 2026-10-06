//go:build tmux

package tmuxkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
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
