//go:build smoke

// smoke_debuglog_test.go exercises the composed live behavior of the
// debug_log opt-in: a real boot with LYX_MUX_DEBUG=1 must write a genuine
// tmux/psmux verbose server log into the hub's .lyx/logs/ dir, and the
// boot-time prune must have already trimmed pre-existing logs there down to
// the newest 2. This is the one live-psmux composed test for this batch;
// debugLogArgs and planLogPrune's own unit tests already cover the pure
// planning logic in isolation (see internal/muxengine/serverlog_test.go).

package muxcli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeDebugLog arms debug_log via LYX_MUX_DEBUG, pre-seeds three stale
// fake server logs with staggered mtimes in the hub logs dir, boots the
// substrate, and asserts (a) a fresh tmux/psmux verbose log newer than the
// fakes appears there and (b) the oldest fake was pruned (boot keeps the
// newest 2 pre-existing logs, so with the fresh log at most 3 ever exist).
func TestSmokeDebugLog(t *testing.T) {
	psmuxBinaryPath(t)

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
