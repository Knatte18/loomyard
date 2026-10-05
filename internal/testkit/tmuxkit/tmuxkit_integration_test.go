//go:build tmux

package tmuxkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	var key string
	t.Run("owner", func(t *testing.T) {
		key = Socket(t, tmux)
		if out, err := exec.Command(tmux, "-L", key, "new-session", "-d", "-s", "kit").CombinedOutput(); err != nil {
			t.Fatalf("start server: %v\n%s", err, out)
		}
		if !hasServer(tmux, key) {
			t.Fatal("server not running after new-session")
		}
	})

	if hasServer(tmux, key) {
		t.Errorf("server on key %q survived the owning test's cleanup", key)
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
}
