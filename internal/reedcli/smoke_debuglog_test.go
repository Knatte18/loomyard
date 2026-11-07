// smoke_debuglog_test.go runs the debug_log opt-in against a real tmux server.
// A boot with LYM_REED_DEBUG set writes a verbose tmux server log into the hub's logs dir.
// The boot-time prune trims older logs there to the newest 2.
// Repeated crash boots keep every debug log shape bounded.
// The unit tests of the pure log-planning logic are in internal/reedengine/serverlog_test.go.

package reedcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestSmokeDebugLog arms debug_log through LYM_REED_DEBUG on one hub's prime worktree, one step per verbosity.
// The steps run serially in a fixed order.
// The second step starts from the logs the first left behind, which only tightens its bound.
// The scenario does not call t.Parallel, because each step sets LYM_REED_DEBUG, which every boot in the process reads.
func TestSmokeDebugLog(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})
	logsDir := fabricengine.HubLogsDir(h.Location.HubPath)

	// With LYM_REED_DEBUG=1 the boot prunes pre-existing logs down to the newest 2,
	// and it writes a fresh verbose log of its own.
	if !t.Run("ArmedBootPrunesOldLogsAndWritesAFreshOne", func(t *testing.T) {
		t.Setenv("LYM_REED_DEBUG", "1")

		if err := os.MkdirAll(logsDir, 0o755); err != nil {
			t.Fatalf("mkdir fake logs dir: %v", err)
		}

		now := time.Now()
		fakeOldest := filepath.Join(logsDir, "tmux-server-fake-oldest.log")
		fakeMiddle := filepath.Join(logsDir, "tmux-server-fake-middle.log")
		fakeNewest := filepath.Join(logsDir, "tmux-server-fake-newest.log")
		writeFakeLog(t, fakeOldest, now.Add(-3*time.Hour))
		writeFakeLog(t, fakeMiddle, now.Add(-2*time.Hour))
		writeFakeLog(t, fakeNewest, now.Add(-1*time.Hour))

		mustRunReed(t, prime, "up")
		// Registered after the boot on purpose, because the test asserts the verbose log reed's own server spawn writes.
		// A pre-started server never writes that log.
		tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))

		if _, err := os.Stat(fakeOldest); !os.IsNotExist(err) {
			t.Errorf("fake-oldest server log survived the boot prune (stat err = %v); want removed", err)
		}
		if _, err := os.Stat(fakeMiddle); err != nil {
			t.Errorf("fake-middle server log missing after boot prune: %v", err)
		}
		if _, err := os.Stat(fakeNewest); err != nil {
			t.Errorf("fake-newest server log missing after boot prune: %v", err)
		}

		// The fresh boot's verbose log must appear, newer than every fake.
		// The server writes that log asynchronously after `up` returns, so the test polls until a deadline;
		// a fixed sleep would be either flaky or slow.
		freshLog := waitForFreshServerLog(t, logsDir, now)
		if freshLog == "" {
			t.Fatalf("no tmux-server-*.log newer than the fakes appeared in %s within the deadline", logsDir)
		}

		mustRunReed(t, prime, "down")
	}) {
		return
	}

	// RepeatedCrashBootsBoundServerClientAndOutLogs pins that repeated debug-armed boots keep all debug log shapes bounded.
	// At -v and above, the spawn invocation is both the client and the server it starts,
	// so a boot leaves a tmux-client-<pid>.log as well as a tmux-server-<pid>.log.
	// At -vv the server also writes a tmux-out-<pid>.log protocol-output log.
	// This step runs at LYM_REED_DEBUG=2, which emits all three shapes.
	// Five debug-armed boots must leave at most 3 of each prefix in the logs dir.
	t.Run("RepeatedCrashBootsBoundServerClientAndOutLogs", func(t *testing.T) {
		t.Setenv("LYM_REED_DEBUG", "2")

		mustRunReed(t, prime, "up")
		socket, session := socketAndSessionIn(t, prime)

		for cycle := 0; cycle < 4; cycle++ {
			if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
				t.Fatalf("cycle %d kill-server: %v", cycle, err)
			}
			waitServerGone(t, tmuxPath, socket, session)

			mustRunReed(t, prime, "up")
		}

		// The fresh server's log and its paired client log are written asynchronously after the last `up` returns,
		// so the test polls for them.
		if waitForFreshServerLog(t, logsDir, time.Time{}) == "" {
			t.Fatalf("no tmux-server-*.log ever appeared in %s", logsDir)
		}
		// The client and out logs are written asynchronously too; the test polls for them before counting any prefix.
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
			t.Errorf("tmux-out-*.log count = %d after 5 debug-armed boots; want <= 3 (this is the defect this step pins: the -vv-only protocol-output log must be pruned too, not left to accumulate unbounded)", got)
		}
	})
}

// writeFakeLog writes a stand-in server log at path and sets its modification time to mtime.
func writeFakeLog(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fake log %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes fake log %s: %v", path, err)
	}
}

// waitForFreshServerLog polls logsDir until a file modified after the given time appears, and returns its name.
// It returns "" if no such file appears before the deadline.
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
