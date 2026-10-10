package tmuxkit

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// TestServerEnviron_MatchesReedServerSpawn pins the kit's copy of reed's environment hygiene to reedengine.CleanClaudeEnv and the three names reed's server spawn also strips.
func TestServerEnviron_MatchesReedServerSpawn(t *testing.T) {
	t.Parallel()

	environ := []string{
		"HOME=/home/x", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_CODEX=keep",
		"LYX_TRACE_ID=abc", "LYX_STRAND_NAME=ab:orch", "LYX_PARENT=ab:driver", "LYX_OTHER=keep", "PATH=/bin",
	}

	got := serverEnviron(environ)

	clean, _ := reedengine.CleanClaudeEnv(environ)
	var want []string
	for _, entry := range clean {
		switch strings.SplitN(entry, "=", 2)[0] {
		case "LYX_TRACE_ID", "LYX_STRAND_NAME", "LYX_PARENT":
		default:
			want = append(want, entry)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("serverEnviron = %q; want %q", got, want)
	}
}

func TestSetEnv(t *testing.T) {
	t.Setenv("TMUX", "/outer/tmux,1,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("TMUX_TMPDIR", "/elsewhere")
	// TMP and TEMP are the Windows temp variables; setting them first has t.Setenv restore them.
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, os.Getenv(name))
	}
	dir := filepath.Join(t.TempDir(), dirPrefix+"0")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := kitBase(); got != "" {
		t.Errorf("kitBase() before setEnv = %q, want \"\", the system temp directory", got)
	}

	setEnv(dir)

	for _, name := range []string{"TMUX_TMPDIR", "TMPDIR"} {
		if got := os.Getenv(name); got != dir {
			t.Errorf("%s = %q, want %q", name, got, dir)
		}
	}
	// A helper re-executed under this environment creates its directory beside dir, never inside it.
	if got := kitBase(); got != filepath.Dir(dir) {
		t.Errorf("kitBase() after setEnv = %q, want %q", got, filepath.Dir(dir))
	}
	// A subtest's TempDir resolves its base afresh, so it sees the TMPDIR setEnv just set.
	t.Run("temp directory lands under TMPDIR", func(t *testing.T) {
		if got := t.TempDir(); !strings.HasPrefix(got, dir+string(filepath.Separator)) {
			t.Errorf("t.TempDir() = %q, want it under %q", got, dir)
		}
	})
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

	for _, path := range []string{live, plain, filepath.Join(dir, "absent")} {
		removeDeadSocket(path, 0)
	}
	// A parallel test's fork holds a copy of the closed listener until its exec, so the dead socket gets the wait a killed server gets.
	removeDeadSocket(dead, deadSocketWait)
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

// TestCheckSockets pins which sockets under a package's directory the end-of-package check names, and that afterRun reads them before its sweep removes them.
func TestCheckSockets(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets under TMUX_TMPDIR are not used on Windows")
	}

	newDir := func(t *testing.T) string {
		t.Helper()
		dir, err := os.MkdirTemp("", dirPrefix)
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
		code := afterRun(&out, dir, 0, 100*time.Millisecond, removeAll)

		if code != 1 {
			t.Errorf("afterRun code = %d; want 1 for an unregistered socket the sweep removed", code)
		}
		if !strings.Contains(out.String(), "checkafter1") {
			t.Errorf("afterRun output = %q; want it to name the key", out.String())
		}
	})
}
