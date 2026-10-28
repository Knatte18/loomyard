//go:build tier0 && !windows

// socketfile_unix_test.go holds the socket file tests; psmux on Windows keeps no socket file.

package reedengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

//testtiming:keep pins the post-kill socket removal waiting out the grace period: a stale socket file is removed and one a live listener still answers on stays with no error; its covering tests run this code without asserting it
func TestRemoveSocketFileOnceGone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir, key string)
		wantExists bool
	}{
		{
			name: "stale socket is removed",
			setup: func(t *testing.T, dir, key string) {
				tmuxkit.BindStaleSocket(t, filepath.Join(dir, key))
			},
			wantExists: false,
		},
		{
			name: "socket still answering after the wait stays with no error",
			setup: func(t *testing.T, dir, key string) {
				l := listenUnixSocket(t, dir, key)
				t.Cleanup(func() { _ = l.Close() })
			},
			wantExists: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, err := lymtemp.MkdirTemp("rg")
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
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir, key string)
		wantExists bool
	}{
		{
			name: "stale socket is removed",
			setup: func(t *testing.T, dir, key string) {
				tmuxkit.BindStaleSocket(t, filepath.Join(dir, key))
			},
			wantExists: false,
		},
		{
			name: "socket with a live listener is kept",
			setup: func(t *testing.T, dir, key string) {
				l := listenUnixSocket(t, dir, key)
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
			dir, err := lymtemp.MkdirTemp("rs")
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
