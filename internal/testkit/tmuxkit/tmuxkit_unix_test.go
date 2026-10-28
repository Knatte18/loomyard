//go:build tier0 && !windows

// tmuxkit_unix_test.go holds the socket tests; tmux keeps its unix sockets under TMUX_TMPDIR, which Windows does not use.

package tmuxkit

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
)

func TestListSockets_OwnDirectoryOnly(t *testing.T) {
	dir, err := lymtemp.MkdirTemp(dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	own := socketDir(dir, 1000)
	if err := os.MkdirAll(own, 0o700); err != nil {
		t.Fatal(err)
	}
	listen := func(path string) {
		l, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
	}
	listen(filepath.Join(own, "default"))
	listen(filepath.Join(own, "other"))
	listen(filepath.Join(dir, "stray"))
	if err := os.WriteFile(filepath.Join(own, "plain"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := listSockets(dir, 1000)
	want := map[string]bool{
		filepath.Join(own, "default"): true,
		filepath.Join(own, "other"):   true,
	}
	if len(got) != len(want) {
		t.Fatalf("listSockets = %v, want exactly %v", got, want)
	}
	for _, s := range got {
		if !want[s] {
			t.Errorf("listSockets returned %q, not a socket in the per-user directory", s)
		}
	}
}

func TestRemoveDeadSocket(t *testing.T) {
	t.Parallel()
	dir, err := lymtemp.MkdirTemp(dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	live := filepath.Join(dir, "live")
	l, err := net.Listen("unix", live)
	if err != nil {
		t.Fatal(err)
	}
	liveListener := l.(*net.UnixListener)
	liveListener.SetUnlinkOnClose(false)
	t.Cleanup(func() { liveListener.Close() })
	// A stale socket file is what a killed tmux server leaves behind.
	dead := filepath.Join(dir, "dead")
	BindStaleSocket(t, dead)
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{live, dead, plain, filepath.Join(dir, "absent")} {
		removeDeadSocket(path, 0)
	}
	if _, err := os.Lstat(live); err != nil {
		t.Errorf("a socket that accepts connections was removed: %v", err)
	}
	if _, err := os.Lstat(plain); err != nil {
		t.Errorf("a regular file was removed: %v", err)
	}
	if _, err := os.Lstat(dead); !os.IsNotExist(err) {
		t.Errorf("a socket that refuses connections survived: %v", err)
	}
}

// TestCheckSockets pins which sockets under a package's directory the end-of-package check names, and that afterRun reads them before its sweep removes them.
func TestCheckSockets(t *testing.T) {
	t.Parallel()
	newDir := func(t *testing.T) string {
		t.Helper()
		dir, err := lymtemp.MkdirTemp(dirPrefix)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		return dir
	}
	listen := func(t *testing.T, dir, key string) {
		t.Helper()
		perUser := socketDir(dir, os.Getuid())
		if err := os.MkdirAll(perUser, 0o700); err != nil {
			t.Fatal(err)
		}
		l, err := net.Listen("unix", filepath.Join(perUser, key))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
	}

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		allowed   bool
		wantNamed string
		wantQuiet bool
	}{
		{name: "unregistered socket with tmux disallowed", setup: func(t *testing.T, dir string) { listen(t, dir, "checkunreg1") }, allowed: false, wantNamed: "checkunreg1"},
		{name: "unregistered socket with tmux allowed", setup: func(t *testing.T, dir string) { listen(t, dir, "checkunreg2") }, allowed: true, wantNamed: "checkunreg2"},
		{name: "registered socket with tmux allowed", setup: func(t *testing.T, dir string) {
			registerKey("checkreg1")
			listen(t, dir, "checkreg1")
		}, allowed: true, wantQuiet: true},
		{name: "registered socket with tmux disallowed", setup: func(t *testing.T, dir string) {
			registerKey("checkreg2")
			listen(t, dir, "checkreg2")
		}, allowed: false, wantNamed: "checkreg2"},
		{name: "reed probe socket with tmux allowed", setup: func(t *testing.T, dir string) { listen(t, dir, "lym-p1a2b3c4") }, allowed: true, wantQuiet: true},
		{name: "reed probe socket with tmux disallowed", setup: func(t *testing.T, dir string) { listen(t, dir, "lym-p1a2b3c5") }, allowed: false, wantNamed: "lym-p1a2b3c5"},
		{name: "lym-p key that is no probe key with tmux allowed", setup: func(t *testing.T, dir string) { listen(t, dir, "lym-pnothex") }, allowed: true, wantNamed: "lym-pnothex"},
		{name: "no per-user directory", setup: func(t *testing.T, dir string) {}, allowed: false, wantQuiet: true},
		{name: "per-user directory without a socket, as a client probe leaves it", setup: func(t *testing.T, dir string) {
			if err := os.MkdirAll(socketDir(dir, os.Getuid()), 0o700); err != nil {
				t.Fatal(err)
			}
		}, allowed: false, wantQuiet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := newDir(t)
			tt.setup(t, dir)

			got := checkSockets(dir, tt.allowed)
			if tt.wantQuiet {
				if len(got) != 0 {
					t.Fatalf("checkSockets = %q; want no finding", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tt.wantNamed) {
				t.Fatalf("checkSockets = %q; want one finding naming %q", got, tt.wantNamed)
			}
		})
	}

	t.Run("afterRun reads the sockets before the sweep removes them", func(t *testing.T) {
		t.Parallel()
		dir := newDir(t)
		listen(t, dir, "checkafter1")
		removeAll := func() {
			sockets, _ := filepath.Glob(filepath.Join(socketDir(dir, os.Getuid()), "*"))
			for _, sock := range sockets {
				os.Remove(sock)
			}
		}

		var out bytes.Buffer
		code := afterRun(&out, dir, 0, 100*time.Millisecond, removeAll, func() error { return os.RemoveAll(dir) })

		if code != 1 {
			t.Errorf("afterRun code = %d; want 1 for an unregistered socket the sweep removed", code)
		}
		if !strings.Contains(out.String(), "checkafter1") {
			t.Errorf("afterRun output = %q; want it to name the key", out.String())
		}
	})
}
