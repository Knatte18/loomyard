//go:build smoke

// smoke_debuglog_test.go exercises the composed live behavior of the
// debug_log opt-in: a real boot with LYX_MUX_DEBUG=1 must write a genuine
// tmux verbose server log into the hub's .lyx/logs/ dir, and the
// boot-time prune must have already trimmed pre-existing logs there down to
// the newest 2. This is the one live-tmux composed test for this batch;
// debugLogArgs and planLogPrune's own unit tests already cover the pure
// planning logic in isolation (see internal/muxengine/serverlog_test.go).

package muxcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeDebugLog arms debug_log via LYX_MUX_DEBUG, pre-seeds three stale
// fake server logs with staggered mtimes in the hub logs dir, boots the
// substrate, and asserts (a) a fresh tmux verbose log newer than the
// fakes appears there and (b) the oldest fake was pruned (boot keeps the
// newest 2 pre-existing logs, so with the fresh log at most 3 ever exist).
func TestSmokeDebugLog(t *testing.T) {
	tmuxBinaryPath(t)

	// The template's ${env:LYX_MUX_DEBUG:-0} resolves this in-process at
	// LoadConfig time — no rebuild or restart needed for the override to
	// take effect on the boot below.
	t.Setenv("LYX_MUX_DEBUG", "1")

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	// lyxtest.PairedFixture.Hub is the WORKTREE root, while Layout.Hub (what
	// HubLogsDir() joins on) is its parent container — compute the logs dir
	// exactly as the engine does, never fixture.Hub/.lyx/logs.
	logsDir := filepath.Join(filepath.Dir(fixture.Hub), ".lyx", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir fake logs dir: %v", err)
	}

	// Three fake pre-existing server logs with staggered old mtimes, oldest
	// first: boot-time prune must keep only the newest 2 of these before
	// writing its own fresh log, so "fake-oldest" is the one that must be
	// gone afterward.
	now := time.Now()
	fakeOldest := filepath.Join(logsDir, "tmux-server-fake-oldest.log")
	fakeMiddle := filepath.Join(logsDir, "tmux-server-fake-middle.log")
	fakeNewest := filepath.Join(logsDir, "tmux-server-fake-newest.log")
	writeFakeLog(t, fakeOldest, now.Add(-3*time.Hour))
	writeFakeLog(t, fakeMiddle, now.Add(-2*time.Hour))
	writeFakeLog(t, fakeNewest, now.Add(-1*time.Hour))

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	// The oldest fake must be pruned; the two newer fakes must survive.
	if _, err := os.Stat(fakeOldest); !os.IsNotExist(err) {
		t.Errorf("fake-oldest server log survived the boot prune (stat err = %v); want removed", err)
	}
	if _, err := os.Stat(fakeMiddle); err != nil {
		t.Errorf("fake-middle server log missing after boot prune: %v", err)
	}
	if _, err := os.Stat(fakeNewest); err != nil {
		t.Errorf("fake-newest server log missing after boot prune: %v", err)
	}

	// The fresh boot's own verbose log must appear, newer than every fake.
	// Deadline-based poll: the server writes its log asynchronously relative
	// to `up` returning, so a fixed sleep would be either flaky or slow.
	freshLog := waitForFreshServerLog(t, logsDir, now)
	if freshLog == "" {
		t.Fatalf("no tmux-server-*.log newer than the fakes appeared in %s within the deadline", logsDir)
	}

	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("down = %d; want 0, output: %s", code, out.String())
	}
}

// writeFakeLog creates an empty file at path and backdates its mtime to
// mtime via os.Chtimes, simulating a stale pre-existing server log for the
// prune-planning test above.
func writeFakeLog(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fake log %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes fake log %s: %v", path, err)
	}
}

// waitForFreshServerLog polls logsDir until a tmux-server-*.log file newer
// than after appears, returning its name, or returns "" once a
// saturation-sized deadline passes without one appearing.
func waitForFreshServerLog(t *testing.T, logsDir string, after time.Time) string {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		entries, err := os.ReadDir(logsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				info, err := entry.Info()
				if err != nil {
					continue
				}
				if info.ModTime().After(after) {
					return name
				}
			}
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// countLogsWithPrefix counts entries in logsDir whose name starts with
// prefix, failing the test on a read error.
func countLogsWithPrefix(t *testing.T, logsDir, prefix string) int {
	t.Helper()
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("read logs dir %s: %v", logsDir, err)
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			n++
		}
	}
	return n
}

// TestSmokeDebugLog_RepeatedCrashBootsBoundServerClientAndOutLogs pins a
// real defect found live-driving debug_log against native tmux (not
// reproducible against psmux, the Windows dev-box default the original
// debug-logging batch was developed/reviewed against): -v/-vv are GLOBAL
// tmux flags on the spawn invocation, and that invocation is simultaneously
// a CLIENT (the local process issuing the command) and, once forked, the
// SERVER it starts — so a debug-armed boot leaves BOTH a
// tmux-server-<pid>.log (documented, already pruned) and a
// tmux-client-<pid>.log (previously unpruned — it accumulated unbounded
// across repeated debug-armed boots since pruneServerLogsLocked only ever
// matched the server-prefixed shape). At -vv (debug_log: 2) the server
// additionally writes a tmux-out-<pid>.log protocol-output log — a THIRD
// shape that only appears at the higher verbosity, so the earlier client-log
// fix (driven at -v) never surfaced it and it too accumulated unbounded
// across repeated -vv boots. This test runs at LYX_MUX_DEBUG=2 (which emits
// all three shapes, a strict superset of -v) so five kill-server-then-up
// cycles must leave at most 3 of EACH prefix in the hub logs dir, never an
// unbounded pile of any of them.
func TestSmokeDebugLog_RepeatedCrashBootsBoundServerClientAndOutLogs(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	t.Setenv("LYX_MUX_DEBUG", "2")

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	logsDir := filepath.Join(filepath.Dir(fixture.Hub), ".lyx", "logs")

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("initial up = %d; want 0, output: %s", code, out.String())
	}
	socket, session := socketAndSession(t)

	for cycle := 0; cycle < 4; cycle++ {
		if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
			t.Fatalf("cycle %d kill-server: %v", cycle, err)
		}
		waitServerGone(t, tmuxPath, socket, session)

		out.Reset()
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
	}

	// Deadline-based poll: the fresh server's own log (and its paired client
	// log) are written asynchronously relative to the last `up` returning.
	if waitForFreshServerLog(t, logsDir, time.Time{}) == "" {
		t.Fatalf("no tmux-server-*.log ever appeared in %s", logsDir)
	}
	// Give the paired client and out logs the same asynchronous-write grace as
	// the server log above before counting any prefix.
	deadline := time.Now().Add(10 * time.Second)
	for (countLogsWithPrefix(t, logsDir, "tmux-client-") == 0 ||
		countLogsWithPrefix(t, logsDir, "tmux-out-") == 0) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}

	if got := countLogsWithPrefix(t, logsDir, "tmux-server-"); got > 3 {
		t.Errorf("tmux-server-*.log count = %d after 5 debug-armed boots; want <= 3 (pruning must keep this bounded)", got)
	}
	if got := countLogsWithPrefix(t, logsDir, "tmux-client-"); got > 3 {
		t.Errorf("tmux-client-*.log count = %d after 5 debug-armed boots; want <= 3 (the client-side log a debug-armed boot also leaves must be pruned too, not left to accumulate unbounded)", got)
	}
	if got := countLogsWithPrefix(t, logsDir, "tmux-out-"); got > 3 {
		t.Errorf("tmux-out-*.log count = %d after 5 debug-armed boots; want <= 3 (this is the defect this test pins: the -vv-only protocol-output log must be pruned too, not left to accumulate unbounded)", got)
	}
}
