// Package tmuxkit gives each test package one isolated tmux socket directory.
//
// A package's TestMain calls Main, which points `TMUX_TMPDIR` at a private directory, so no test reaches the caller's own tmux server or leaves a socket in the default directory.
// Socket hands a single test a unique `-L` key and kills its server when the test ends.
//
// It is the second kit exempt from the Testkit Invariant's `os/exec` ban, after lyxbin.
// The exemption is bounded to running the `tmux` binary against sockets under the kit's own directory or its own fixture keys.
// A test binary killed by a panic or timeout skips Main's sweep and leaves one `lyx`-prefixed temp directory with its servers;
// Socket's cleanup still covers ordinary test failures.
package tmuxkit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

const (
	// maxSocketPath is the longest usable socket path: sockaddr_un's 108-byte sun_path less its terminating NUL.
	maxSocketPath = 107

	// maxKeyBytes is the longest `-L` key reed forms (`reedengine.ServerName`).
	maxKeyBytes = 61

	dirPrefix = "lyx"
)

// Main runs m inside an isolated tmux socket directory and returns the run's exit code.
// A package calls it as `os.Exit(tmuxkit.Main(m))` after its own setup.
// On Windows (psmux) the tests run unchanged.
func Main(m *testing.M) int {
	if runtime.GOOS == "windows" {
		return m.Run()
	}

	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmuxkit: create socket directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	if err := checkSocketPath(dir, os.Getuid()); err != nil {
		fmt.Fprintf(os.Stderr, "tmuxkit: %v\n", err)
		return 1
	}
	setEnv(dir)

	code := m.Run()

	sweep(dir, os.Getuid())
	return code
}

// sweep kills the tmux server behind every socket under dir's per-user socket directory.
// It does nothing when tmux is not on PATH.
func sweep(dir string, uid int) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return
	}
	for _, sock := range listSockets(dir, uid) {
		_ = exec.Command(tmux, "-S", sock, "kill-server").Run()
	}
}

// Socket returns a unique `-L` key for one test and registers a `kill-server` on it in t.Cleanup.
// tmux is the binary to run.
func Socket(t *testing.T, tmux string) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("tmuxkit: random socket key: %v", err)
	}
	key := "lyxtest-" + hex.EncodeToString(b[:])
	t.Cleanup(func() {
		_ = exec.Command(tmux, "-L", key, "kill-server").Run()
	})
	return key
}

// socketDir is the directory tmux puts its sockets in under a `TMUX_TMPDIR` of dir.
func socketDir(dir string, uid int) string {
	return filepath.Join(dir, "tmux-"+strconv.Itoa(uid))
}

// checkSocketPath reports an error naming `TMPDIR` as the remedy when dir plus the per-user directory plus the longest key would exceed the socket path limit.
func checkSocketPath(dir string, uid int) error {
	longest := filepath.Join(socketDir(dir, uid), string(make([]byte, maxKeyBytes)))
	if len(longest) > maxSocketPath {
		return fmt.Errorf("socket directory %q is too long: its longest socket path would be %d bytes, over the %d-byte limit; set TMPDIR to a shorter path", dir, len(longest), maxSocketPath)
	}
	return nil
}

// setEnv points tmux at dir and detaches the process from any enclosing tmux session.
func setEnv(dir string) {
	os.Setenv("TMUX_TMPDIR", dir)
	os.Unsetenv("TMUX")
	os.Unsetenv("TMUX_PANE")
}

// listSockets returns the socket files under dir's per-user socket directory, and nothing outside it.
func listSockets(dir string, uid int) []string {
	entries, err := os.ReadDir(socketDir(dir, uid))
	if err != nil {
		return nil
	}
	var socks []string
	for _, e := range entries {
		if e.Type()&os.ModeSocket != 0 {
			socks = append(socks, filepath.Join(socketDir(dir, uid), e.Name()))
		}
	}
	return socks
}
