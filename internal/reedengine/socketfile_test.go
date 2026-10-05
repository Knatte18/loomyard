package reedengine

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRemoveStaleSocket(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("psmux keeps no socket file")
	}

	// A unix socket path is limited to about 104 bytes, so the directory is short and the test spawns nothing.
	listen := func(t *testing.T, dir, key string, unlinkOnClose bool) *net.UnixListener {
		t.Helper()
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, key), Net: "unix"})
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		l.SetUnlinkOnClose(unlinkOnClose)
		return l
	}

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
