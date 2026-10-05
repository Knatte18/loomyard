package tmuxkit

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestCheckSocketPath(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		uid     int
		wantErr bool
	}{
		{"short directory", "/tmp/lyx123", 1000, false},
		{"directory at the limit", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000, false},
		{"directory one byte over", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes+1), 1000, true},
		{"longer uid widens the path", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSocketPath(tt.dir, tt.uid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkSocketPath(%q, %d) error = %v, wantErr %v", tt.dir, tt.uid, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "TMPDIR") {
				t.Errorf("error %q does not name TMPDIR as the remedy", err)
			}
		})
	}
}

func TestSetEnv(t *testing.T) {
	t.Setenv("TMUX", "/outer/tmux,1,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("TMUX_TMPDIR", "/elsewhere")

	setEnv("/tmp/lyxabc")

	if got := os.Getenv("TMUX_TMPDIR"); got != "/tmp/lyxabc" {
		t.Errorf("TMUX_TMPDIR = %q, want /tmp/lyxabc", got)
	}
	for _, name := range []string{"TMUX", "TMUX_PANE"} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s still set to %q", name, v)
		}
	}
}

func TestListSockets_OwnDirectoryOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets under TMUX_TMPDIR are not used on Windows")
	}
	dir, err := os.MkdirTemp("", dirPrefix)
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

func TestListSockets_MissingDirectory(t *testing.T) {
	if got := listSockets(t.TempDir(), 1000); len(got) != 0 {
		t.Errorf("listSockets of a directory with no tmux subdirectory = %v, want none", got)
	}
}

func TestRemoveDeadSocket(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets are not used on Windows")
	}
	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	listen := func(name string) (string, *net.UnixListener) {
		path := filepath.Join(dir, name)
		l, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		ul := l.(*net.UnixListener)
		ul.SetUnlinkOnClose(false)
		t.Cleanup(func() { ul.Close() })
		return path, ul
	}
	live, _ := listen("live")
	dead, deadListener := listen("dead")
	// Closing the listener leaves the socket file behind, as a killed tmux server does.
	deadListener.Close()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{live, plain, dead, filepath.Join(dir, "absent")} {
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

func TestMaxKeyBytes_HoldsReedServerName(t *testing.T) {
	hub := "/" + strings.Repeat("h", 300) + "-LYXHUB"
	if got := len(reedengine.ServerName(hub)); got > maxKeyBytes {
		t.Errorf("reedengine.ServerName of an over-long hub path is %d bytes, over the kit's %d-byte key bound", got, maxKeyBytes)
	}
}
