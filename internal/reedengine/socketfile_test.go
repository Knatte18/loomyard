package reedengine

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// listenUnixSocket listens on the socket path dir/key.
// A unix socket path is limited to about 104 bytes, so callers keep dir short.
func listenUnixSocket(t *testing.T, dir, key string, unlinkOnClose bool) *net.UnixListener {
	t.Helper()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, key), Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l.SetUnlinkOnClose(unlinkOnClose)
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

//testtiming:keep pins the post-kill socket removal waiting out the grace period: a stale socket file is removed and one a live listener still answers on stays with no error; its covering tests run this code without asserting it
func TestRemoveSocketFileOnceGone(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("psmux keeps no socket file")
	}

	tests := []struct {
		name       string
		setup      func(t *testing.T, dir, key string)
		wantExists bool
	}{
		{
			name: "stale socket is removed",
			setup: func(t *testing.T, dir, key string) {
				if err := listenUnixSocket(t, dir, key, false).Close(); err != nil {
					t.Fatalf("close: %v", err)
				}
			},
			wantExists: false,
		},
		{
			name: "socket still answering after the wait stays with no error",
			setup: func(t *testing.T, dir, key string) {
				l := listenUnixSocket(t, dir, key, true)
				t.Cleanup(func() { _ = l.Close() })
			},
			wantExists: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, err := os.MkdirTemp("", "rg")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			tt.setup(t, dir, "k")

			if err := removeSocketFileOnceGone(dir, "k", 2*processExitPoll); err != nil {
				t.Fatalf("removeSocketFileOnceGone = %v, want nil", err)
			}
			_, err = os.Lstat(filepath.Join(dir, "k"))
			if got := err == nil; got != tt.wantExists {
				t.Errorf("path exists = %v, want %v", got, tt.wantExists)
			}
		})
	}
}

func TestRemoveStaleSocket(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("psmux keeps no socket file")
	}

	listen := listenUnixSocket

	tests := []struct {
		name       string
		setup      func(t *testing.T, dir, key string)
		wantExists bool
	}{
		{
			name: "stale socket is removed",
			setup: func(t *testing.T, dir, key string) {
				if err := listen(t, dir, key, false).Close(); err != nil {
					t.Fatalf("close: %v", err)
				}
			},
			wantExists: false,
		},
		{
			name: "socket with a live listener is kept",
			setup: func(t *testing.T, dir, key string) {
				l := listen(t, dir, key, true)
				t.Cleanup(func() { _ = l.Close() })
			},
			wantExists: true,
		},
		{
			name: "regular file is kept",
			setup: func(t *testing.T, dir, key string) {
				if err := os.WriteFile(filepath.Join(dir, key), nil, 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
			},
			wantExists: true,
		},
		{
			name:       "missing path is no error",
			setup:      func(t *testing.T, dir, key string) {},
			wantExists: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, err := os.MkdirTemp("", "rs")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			tt.setup(t, dir, "k")

			if err := removeStaleSocket(dir, "k"); err != nil {
				t.Fatalf("removeStaleSocket = %v, want nil", err)
			}
			_, err = os.Lstat(filepath.Join(dir, "k"))
			if got := err == nil; got != tt.wantExists {
				t.Errorf("path exists = %v, want %v", got, tt.wantExists)
			}
		})
	}
}
