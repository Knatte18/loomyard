//go:build tier0

package reedengine

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// listenUnixSocket listens on the socket path dir/key and removes the file on close.
// A unix socket path is limited to about 104 bytes, so callers keep dir short.
func listenUnixSocket(t *testing.T, dir, key string) *net.UnixListener {
	t.Helper()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, key), Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return l
}

// TestBootSocketPath pins the socket path rules the boot refusal rests on: the limit per GOOS, the pure length check, and the base's symlinks being resolved before measuring.
// It sets TMUX_TMPDIR, which is process-global, so it does not run in parallel.
func TestBootSocketPath(t *testing.T) {
	t.Run("LimitPerGOOS", func(t *testing.T) {
		for goos, want := range map[string]int{"linux": 107, "darwin": 103, "freebsd": 103} {
			if got := unixSocketPathLimit(goos); got != want {
				t.Errorf("unixSocketPathLimit(%q) = %d, want %d", goos, got, want)
			}
		}
	})

	t.Run("CheckLength", func(t *testing.T) {
		dir := "/d"
		key := "k"
		atLimit := len(filepath.Join(dir, key))
		if err := checkSocketPathLength(dir, key, atLimit); err != nil {
			t.Errorf("checkSocketPathLength at the limit = %v, want nil", err)
		}
		err := checkSocketPathLength(dir, key, atLimit-1)
		if err == nil {
			t.Fatal("checkSocketPathLength one byte over = nil, want a refusal")
		}
		for _, want := range []string{"TMUX_TMPDIR", filepath.Join(dir, key), strconv.Itoa(atLimit), strconv.Itoa(atLimit - 1)} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not name %q", err, want)
			}
		}
	})

	t.Run("ResolvesSymlinkedBase", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "a-much-longer-directory-name")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		link := filepath.Join(root, "s")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		resolvedTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		t.Setenv("TMUX_TMPDIR", link)

		if got, want := resolvedSocketDir(), socketDirUnder(resolvedTarget); got != want {
			t.Errorf("resolvedSocketDir() = %q, want %q", got, want)
		}
	})
}
