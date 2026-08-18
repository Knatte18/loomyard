//go:build smoke

// smoke_staterecovery_test.go drives the R5 review's state-loss/corruption recovery findings at the
// CLI seam a real operator uses, against a real tmux server: a reed.json that outlives the session
// incarnation it was written for (R5-F2), a worktree renamed while its session is up (R5-F5), and a
// .lyx directory copied between two worktrees of one hub (R5-F4).
// All three are ordinary operator/environment events — a backup restore, a `mv`, a `cp -r` — not
// adversarial misuse, which is why they belong in the live suite rather than only in hermetic unit
// tests.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
)

// statusStrandLive returns whether guid is reported live by `status` run in cwd, and fails the test
// when the strand is missing from the report entirely.
func statusStrandLive(t *testing.T, cwd, guid string) (live bool, paneID string) {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(cwd, &out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, ok := statusStrand(t, out.Bytes(), guid)
	if !ok {
		t.Fatalf("status missing strand %s: %s", guid, out.String())
	}
	live, _ = strand["live"].(bool)
	paneID, _ = strand["paneId"].(string)
	return live, paneID
}

// sessionNamesOnSocket returns every session name tmux currently lists on socket.
// An errored or empty list-sessions means no server, which is reported as no sessions rather than
// as a failure — tmux exits 1 with "no server running" for exactly that case.
func sessionNamesOnSocket(t *testing.T, tmuxPath, socket string) []string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// TestSmokeStaleStateFileIsNotMistakenForLiveStrands is the end-to-end regression guard for the R5
// review's R5-F2, driven at the CLI seam.
//
// Reproduced live before the fix: tmux pane ids are server-global and restart at %0 on every server
// rebirth, so a reed.json restored over a LATER session incarnation — a backup tool, a resurrected
// untracked copy, or simply a file older than the session now running — names panes that exist and
// belong to something else. `lyx reed status` then reported the strand live:true against the new
// session's bare initial shell (no such process anywhere on the box), and `lyx reed resume` answered
// resumed:0, refusing to rebuild it: the recovery verb itself entrenched the false-healthy report.
//
// The load-bearing assertions are that status reports live:false AND that resume actually rebuilds
// the strand. Asserting only the first would pass for a fix that merely stopped trusting the
// binding without restoring the strand.
func TestSmokeStaleStateFileIsNotMistakenForLiveStrands(t *testing.T) {
	tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())

	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	guid := addStrand(t, smokeReapLaunchCmd(), "--name", "generation-1")

	statePath := filepath.Join(h.PrimeWorktree(), ".lyx", "reed.json")
	generationOne, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read %s: %v", statePath, err)
	}

	// down + up is the ordinary route to a second incarnation: down kills the server, so the next
	// up's pane ids restart at %0 and collide with the ids generation 1 recorded.
	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("down = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	if err := os.WriteFile(statePath, generationOne, 0o600); err != nil {
		t.Fatalf("restore %s: %v", statePath, err)
	}

	live, paneID := statusStrandLive(t, "", guid)
	if live {
		t.Errorf("status strand %s live = true on pane %q after a stale reed.json was restored; want false — that pane belongs to a different session incarnation", guid, paneID)
	}

	out.Reset()
	if code := RunCLI(&out, []string{"resume"}); code != 0 {
		t.Fatalf("resume = %d; want 0, output: %s", code, out.String())
	}
	var resumeResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &resumeResult); err != nil {
		t.Fatalf("parse resume result: %v", err)
	}
	if resumed, _ := resumeResult["resumed"].(float64); resumed < 1 {
		t.Errorf("resume resumed = %v; want at least 1 — the stale binding must not make resume skip a strand whose process is not running", resumeResult["resumed"])
	}

	if live, _ := statusStrandLive(t, "", guid); !live {
		t.Errorf("status strand %s live = false after resume rebuilt it; want true", guid)
	}
}

// TestSmokeRenamedWorktreeRefusesRatherThanDoubleLaunching is the end-to-end regression guard for
// the R5 review's R5-F5, driven at the CLI seam.
//
// Reproduced live before the fix: the tmux session name derives from the worktree basename while
// .lyx travels with the directory, so renaming a worktree whose session is up made `lyx reed resume`
// boot a SECOND session and relaunch every strand into it — the strand's process ran twice, and the
// orphaned original session survived `lyx reed down` in the renamed worktree, addressable by no reed
// verb ever again because no worktree of that name exists to derive it from.
//
// The sibling is a plain clone rather than a linked git worktree precisely so it can be renamed:
// moving a linked worktree breaks its gitdir link and lyxcwd.Resolve would fail for an unrelated
// reason, masking the behaviour under test.
func TestSmokeRenamedWorktreeRefusesRatherThanDoubleLaunching(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	original := materializeSibling(t, h, "before-rename")
	renamed := filepath.Join(h.Path, "after-rename")

	deferHubRelease(t, h.PrimeWorktree())
	// Only the renamed path is released: deferHubRelease waits (up to 100s) for a directory to
	// become renameable, and the pre-rename path no longer exists by cleanup time, so registering
	// it would burn that whole budget on a path nothing is holding.
	deferHubRelease(t, renamed)

	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(renamed, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(original, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, originalSession := socketAndSessionIn(t, original)
	addStrandIn(t, original, smokeReapLaunchCmd(), "--name", "pre-rename")

	if err := os.Rename(original, renamed); err != nil {
		t.Fatalf("rename %s -> %s: %v", original, renamed, err)
	}

	out.Reset()
	if code := RunCLIIn(renamed, &out, []string{"resume"}); code == 0 {
		t.Fatalf("resume in the renamed worktree = 0; want a refusal — continuing double-launches every strand and orphans %q, output: %s", originalSession, out.String())
	}
	refusal := out.String()
	for _, want := range []string{originalSession, "kill-session"} {
		if !strings.Contains(refusal, want) {
			t.Errorf("resume refusal = %s; want it to contain %q so the operator can act on it", refusal, want)
		}
	}

	// The refusal must leave no residue: a second session on the SHARED per-hub server is exactly
	// the stranded-substrate outcome reed refuses before creating.
	names := sessionNamesOnSocket(t, tmuxPath, socket)
	if len(names) != 1 || names[0] != originalSession {
		t.Errorf("sessions on socket %s = %v; want exactly [%s] — a refusal must not deposit a session of its own", socket, names, originalSession)
	}

	// Following the remedy the refusal names must actually clear it.
	if err := exec.Command(tmuxPath, "-L", socket, "kill-session", "-t", "="+originalSession).Run(); err != nil {
		t.Fatalf("kill-session -t =%s: %v", originalSession, err)
	}
	out.Reset()
	if code := RunCLIIn(renamed, &out, []string{"resume"}); code != 0 {
		t.Fatalf("resume after the orphan was killed = %d; want 0 — the refusal must be escapable by the remedy it names, output: %s", code, out.String())
	}
}
