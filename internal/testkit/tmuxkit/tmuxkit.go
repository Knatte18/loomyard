// Package tmuxkit gives each test package one isolated tmux socket directory.
//
// A package's TestMain calls Main, which points `TMUX_TMPDIR` at a private directory, so no test reaches the caller's own tmux server or leaves a socket in the default directory.
// Socket hands a single test a unique `-L` key and, when the test ends, kills its server and removes its socket file.
// KillOnCleanup does the same for a key the test did not mint.
// PackageServer hands one shared server, which Main's sweep kills, to every test of the package that names its own sessions, kills no server and asserts nothing about the whole session set.
// Socket, KillOnCleanup and PackageServer each first start a hermetic server on the key from the kit's own tmux config, so no server a test uses reads `~/.tmux.conf`, starts login-shell panes or exits when it has no session.
// The config marks its servers with the user option `@lyx_test_server`, which reed's stale-holder probe reads to leave such a server alone.
// Reed starts a server itself, carrying no such config, only after a test's own `down` or `kill-server` on its key, or when a test registers its key after reed's boot.
//
// Main also points `TMPDIR` at that directory, so every temp file a test creates lands there, and after the run it sweeps the servers, scans `/proc` on Linux for any process whose cwd, executable or argv references the directory, kills it and fails the package, then removes the directory.
// A reed watchdog daemon is killed without failing the package, because it idles out on its own schedule after the test that spawned it.
// Before the sweep it also fails the package for a socket it finds: any socket in a test binary built without the `tmux` and `llm` tags, and in one built with either only a socket whose key no test registered.
// Pids, ProcArgv, ProcCwd, ProcExe and IsWatchdog are the read-only `/proc` probes behind that scan, exported for tests that look for processes themselves.
//
// It is the second kit exempt from the Testkit Invariant's `os/exec` ban, after lyxbin.
// The exemption is bounded to running the `tmux` binary against sockets under the kit's own directory or its own fixture keys, to starting servers under the kit's own config, and to the `/proc` scan and the kill of a leftover it finds.
// A test binary killed by a panic or timeout skips Main's sweep and leaves one `lyx`-prefixed temp directory with its servers;
// Socket's cleanup still covers ordinary test failures.
package tmuxkit

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/proc"
)

const (
	// maxSocketPath is the longest usable socket path: sockaddr_un's 108-byte sun_path less its terminating NUL.
	maxSocketPath = 107

	// maxKeyBytes is the longest `-L` key reed forms (`reedengine.ServerName`).
	maxKeyBytes = 61

	dirPrefix = "lyx"

	// deadSocketWait bounds how long removeDeadSocket waits for a killed server to stop accepting connections.
	deadSocketWait = 2 * time.Second

	deadSocketPoll = 10 * time.Millisecond

	// leftoverGrace bounds how long afterRun waits for a process referencing the package's directory to exit after the sweep.
	leftoverGrace = 2 * time.Second

	leftoverPoll = 50 * time.Millisecond
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

	return afterRun(os.Stderr, dir, code, leftoverGrace, func() { sweep(dir, os.Getuid()) })
}

// afterRun finishes a package's run: it checks the sockets under dir, sweeps the servers, then, on Linux, kills every process still referencing dir and fails the run for each that is not a reed watchdog daemon, and finally removes dir.
// The socket check runs before the sweep, because the sweep removes the socket files the check reads.
// A process that outlives its sweep gets up to grace to exit on its own first.
// It returns code unless a socket finding or a leftover forces 1, and names each on w.
func afterRun(w io.Writer, dir string, code int, grace time.Duration, sweep func()) int {
	if findings := checkSockets(dir, tmuxAllowed); len(findings) > 0 {
		for _, finding := range findings {
			fmt.Fprintln(w, finding)
		}
		code = 1
	}
	sweep()
	if runtime.GOOS == "linux" {
		var left []leftover
		for deadline := time.Now().Add(grace); ; time.Sleep(leftoverPoll) {
			left = leftovers(dir)
			if len(left) == 0 || time.Now().After(deadline) {
				break
			}
		}
		for _, l := range left {
			_ = proc.KillPID(l.pid)
			if IsWatchdog(l.argv) {
				continue
			}
			fmt.Fprintf(w, "tmuxkit: process %d left running by the package: %q\n", l.pid, l.argv)
			code = 1
		}
	}
	_ = os.RemoveAll(dir)
	return code
}

// checkSockets returns one finding per tmux server socket under dir's per-user socket directory that its package's tests may not leave.
// With allowed false every socket is a finding; with it true only a socket whose key no test registered is.
// A finding names the key and the way forward: tag the test file `tmux`, or register the key through Socket, KillOnCleanup or PackageServer.
func checkSockets(dir string, allowed bool) []string {
	registryMu.Lock()
	defer registryMu.Unlock()
	var findings []string
	for _, sock := range listSockets(dir, os.Getuid()) {
		key := filepath.Base(sock)
		switch {
		case !allowed:
			findings = append(findings, fmt.Sprintf("tmuxkit: a tmux server on key %q outlived the run in a package without a tmux or llm test tag; tag the test file that starts it `tmux`", key))
		case !registeredKeys[key]:
			findings = append(findings, fmt.Sprintf("tmuxkit: a tmux server on unregistered key %q outlived the run; register the key through Socket, KillOnCleanup or PackageServer, or tag the file `tmux`", key))
		}
	}
	return findings
}

// leftover is a process that references a package's private directory.
type leftover struct {
	pid  int
	argv []string
}

// leftovers returns every process other than the caller whose cwd or executable lies under dir or whose argv carries dir.
func leftovers(dir string) []leftover {
	var found []leftover
	for _, pid := range Pids() {
		if pid == os.Getpid() {
			continue
		}
		argv, argvOK := ProcArgv(pid)
		cwd, _ := ProcCwd(pid)
		exe, _ := ProcExe(pid)
		if !argvOK || !referencesDir(dir, cwd, exe, argv) {
			continue
		}
		found = append(found, leftover{pid: pid, argv: argv})
	}
	return found
}

// referencesDir reports whether cwd or exe lies under dir or any argv element contains dir.
func referencesDir(dir, cwd, exe string, argv []string) bool {
	if underDir(dir, cwd) || underDir(dir, exe) {
		return true
	}
	for _, arg := range argv {
		if strings.Contains(arg, dir) {
			return true
		}
	}
	return false
}

// underDir reports whether path is dir or inside it.
func underDir(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
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
		removeDeadSocket(sock, deadSocketWait)
	}
}

// Socket returns a unique `-L` key for one test, with a hermetic server already running on it, and registers its teardown in t.Cleanup through KillOnCleanup.
// tmux is the binary to run.
func Socket(t *testing.T, tmux string) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("tmuxkit: random socket key: %v", err)
	}
	key := "lyxtest-" + hex.EncodeToString(b[:])
	KillOnCleanup(t, tmux, key)
	return key
}

// PackageServer returns the `-L` key of one hermetic server shared by every test of the package that calls it, started on first use.
// It is for a test that names its own sessions, kills no server and asserts nothing about the server's whole session set.
// It registers no cleanup: Main's sweep kills every server under the kit's directory when the package ends.
// tmux is the binary to run.
func PackageServer(t *testing.T, tmux string) string {
	t.Helper()
	packageServerOnce.Do(func() {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatalf("tmuxkit: random socket key: %v", err)
		}
		key := "lyxpkg-" + hex.EncodeToString(b[:])
		registerKey(key)
		if err := startServer(tmux, key); err != nil {
			t.Fatalf("tmuxkit: %v", err)
		}
		packageServerKey = key
	})
	if packageServerKey == "" {
		t.Fatal("tmuxkit: the package server failed to start in an earlier test")
	}
	return packageServerKey
}

// KillOnCleanup pre-starts a hermetic server on the `-L` key key, registers the key, and registers in t.Cleanup a `kill-server` on it, then the removal of that key's socket file.
// It is the helper for a key a test did not mint itself, such as a `reedengine.ServerName` key of a fixture hub.
// The server reads the kit's own config instead of `~/.tmux.conf`, starts non-login panes, and survives having no session; a start failure fails the test.
// The cleanup removes the one path for key under the current `TMUX_TMPDIR`'s per-user directory, never a glob, and only a socket that no longer accepts connections.
// tmux is the binary to run.
func KillOnCleanup(t *testing.T, tmux, key string) {
	t.Helper()
	registerKey(key)
	t.Cleanup(func() {
		_ = exec.Command(tmux, "-L", key, "kill-server").Run()
		removeDeadSocket(socketPath(key), deadSocketWait)
	})
	if err := startServer(tmux, key); err != nil {
		t.Fatalf("tmuxkit: %v", err)
	}
}

const (
	// configFileName is the kit's tmux config, written once per test binary into the socket directory Main owns.
	configFileName = "lyx-test-tmux.conf"

	// testServerOption is the tmux user option the kit's config sets to "on", which reed's stale-holder probe reads to tell a pre-started test server from a stale one.
	testServerOption = "@lyx_test_server"
)

var (
	// registryMu guards registeredKeys.
	registryMu sync.Mutex
	// registeredKeys holds every `-L` key a test registered with the kit, for the end-of-package check.
	registeredKeys = map[string]bool{}

	// packageServerOnce guards the start of the key PackageServer hands out.
	packageServerOnce sync.Once
	// packageServerKey is the `-L` key of the package's shared server, empty until PackageServer starts it.
	packageServerKey string

	configOnce sync.Once
	configPath string
	configErr  error
)

// registerKey records key as one a test of this package may start a server on.
func registerKey(key string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registeredKeys[key] = true
}

// serverConfig writes the kit's tmux config once per test binary, under the directory Main set `TMUX_TMPDIR` to, and returns its path.
// The config sets the default shell and an equal default command, so panes start the shell without the login flag; it keeps an empty server alive; and it marks the server as the kit's.
func serverConfig() (string, error) {
	configOnce.Do(func() {
		dir := os.Getenv("TMUX_TMPDIR")
		if dir == "" {
			configErr = errors.New("TMUX_TMPDIR unset: the package's TestMain does not run through Main")
			return
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}
		config := fmt.Sprintf("set -g default-shell %q\nset -g default-command %q\nset -g exit-empty off\nset -g %s on\n", shell, shell, testServerOption)
		configPath = filepath.Join(dir, configFileName)
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			configErr = fmt.Errorf("write tmux config: %w", err)
		}
	})
	return configPath, configErr
}

// startServer starts a tmux server on the `-L` key key from the kit's config, never reading `~/.tmux.conf`, with no session.
// On a key whose server already runs, the start is a client command that changes nothing.
// The server is the process every session and pane on the key inherits its environment from, so its environment is the test process's less what reed's own server spawn strips.
// It does nothing on Windows.
func startServer(tmux, key string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	config, err := serverConfig()
	if err != nil {
		return err
	}
	cmd := exec.Command(tmux, "-f", config, "-L", key, "start-server")
	cmd.Dir = filepath.Dir(config)
	cmd.Env = serverEnviron(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start tmux server on %q: %w\n%s", key, err, out)
	}
	return nil
}

// serverEnviron returns environ without the variables no pane may inherit from the process that starts the server: Claude's own, and the trace id and agent names of the enclosing strand.
// The Claude filter is a copy of reedengine.CleanClaudeEnv, which this package cannot import because reedengine's own tests import the kit; a test pins the two together.
func serverEnviron(environ []string) []string {
	clean := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		switch {
		case key == "CLAUDECODE", strings.HasPrefix(key, "CLAUDE_CODE_"):
		case key == "LYX_TRACE_ID", key == agentname.StrandNameEnv, key == agentname.ParentEnv:
		default:
			clean = append(clean, entry)
		}
	}
	return clean
}

// socketPath is the socket file tmux creates for the `-L` key key, resolving the directory from `TMUX_TMPDIR` as tmux does.
func socketPath(key string) string {
	base := os.Getenv("TMUX_TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(socketDir(base, os.Getuid()), key)
}

// removeDeadSocket removes path when it is a socket that refuses connections, and leaves anything else alone.
// A live server's socket accepts the probe connection and stays.
// A server that `kill-server` was just sent to keeps listening for a moment, so a refusal is awaited for up to wait.
func removeDeadSocket(path string, wait time.Duration) {
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return
	}
	for deadline := time.Now().Add(wait); ; time.Sleep(deadSocketPoll) {
		conn, err := net.Dial("unix", path)
		if err != nil {
			_ = os.Remove(path)
			return
		}
		conn.Close()
		if time.Now().After(deadline) {
			return
		}
	}
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

// setEnv points tmux and every temporary file at dir and detaches the process from any enclosing tmux session.
func setEnv(dir string) {
	os.Setenv("TMUX_TMPDIR", dir)
	os.Setenv("TMPDIR", dir)
	if runtime.GOOS == "windows" {
		os.Setenv("TMP", dir)
		os.Setenv("TEMP", dir)
	}
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
