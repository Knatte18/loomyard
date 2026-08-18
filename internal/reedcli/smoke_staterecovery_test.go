//go:build smoke

// smoke_staterecovery_test.go drives the R5 review's state-loss/corruption recovery findings at the
// CLI seam a real operator uses, against a real tmux server: a reed.json that outlives the session
// incarnation it was written for (R5-F2), a worktree renamed while its session is up (R5-F5), and a
// .lyx directory copied between two worktrees of one hub (R5-F4).
// All three are ordinary operator/environment events — a backup restore, a `mv`, a `cp -r` — not
// adversarial misuse, which is why they belong in the live suite rather than only in hermetic unit
// tests. Each also has a hermetic counterpart pinning the specific decision it rests on
// (generation_test.go, strand_test.go); these pin the OUTCOME an operator would actually see.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume is the end-to-end
// regression guard for the R6 review's R6-F1, driven at the CLI seam.
//
// Reproduced live before the fix: both ordinary routes into the foreign-session refusal — a worktree
// renamed while its session was up, a .lyx copied between worktrees of one hub — leave THIS
// worktree's session absent, so every non-booting verb lands in requireSessionLocked. That returned
// `no reed session (1 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a
// bare substrate`, and both commands it named then refused with the orphan-session error. The
// operator's whole diagnostic surface reported a bare "no session", never named the still-running
// session, and routed them into a loop.
//
// The test asserts the diagnosis at the CALL SITE rather than the helper: refuseLiveForeignSessionLocked
// has its own hermetic coverage (generation_test.go), and what is not otherwise pinned is that
// requireSessionLocked consults it at all. Removing that call restores the misleading text verbatim,
// which the negative assertion below catches.
func TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	original := materializeSibling(t, h, "diag-before-rename")
	renamed := filepath.Join(h.Path, "diag-after-rename")

	deferHubRelease(t, h.PrimeWorktree())
	deferHubRelease(t, renamed)

	var out bytes.Buffer
	if code := RunCLIIn(original, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, originalSession := socketAndSessionIn(t, original)
	t.Cleanup(func() {
		exec.Command(tmuxPath, "-L", socket, "kill-session", "-t", "="+originalSession).Run()
		var buf bytes.Buffer
		RunCLIIn(renamed, &buf, []string{"down"})
	})
	addStrandIn(t, original, smokeReapLaunchCmd(), "--name", "pre-rename")

	if err := os.Rename(original, renamed); err != nil {
		t.Fatalf("rename %s -> %s: %v", original, renamed, err)
	}

	// Every verb that goes through requireSessionLocked rather than through a boot must report the
	// same diagnosis: status is the one an operator reaches for first, attach the one they reach for
	// next, and add the one an agent driving reed hits.
	for _, verb := range [][]string{{"status"}, {"attach"}, {"add", "--cmd", smokeReapLaunchCmd(), "--name", "post-rename"}} {
		out.Reset()
		if code := RunCLIIn(renamed, &out, verb); code == 0 {
			t.Fatalf("%v in the renamed worktree = 0; want a failure naming the orphan session, output: %s", verb, out.String())
		}
		reported := out.String()
		for _, want := range []string{originalSession, "kill-session"} {
			if !strings.Contains(reported, want) {
				t.Errorf("%v error = %s; want it to name %q so the operator can act on it", verb, reported, want)
			}
		}
		// The pre-fix text named resume as the remedy, and resume refuses for exactly the reason
		// the message omitted. Naming it here would send the operator back into that loop.
		if strings.Contains(reported, `run "lyx reed resume"`) {
			t.Errorf("%v error = %s; want it NOT to point at resume, which refuses while %q is still running", verb, reported, originalSession)
		}
	}
}

// TestSmokeDownReportsTheSessionItAbandons is the end-to-end regression guard for the R6 review's
// R6-F3, driven at the CLI seam.
//
// Reproduced live before the fix: down loads no state and so never reaches the foreign-session
// refusal, which makes it the only lyx-only escape from that refusal — and the one a tmux-less
// operator is therefore steered toward. In a renamed worktree it reported {"ok":true,...}, deleted
// reed.json, and left the recorded session and its strand process running on the shared per-hub
// socket, addressable by no reed verb ever again and named by nothing left on disk.
//
// The assertions are that down still SUCCEEDS (it is the escape, and it is idempotent), that the
// abandoned session is named in the envelope, and that down did not kill it — the recorded name is a
// sibling worktree's live session in the hand-copied-.lyx case, so killing it would re-open R5-F4.
func TestSmokeDownReportsTheSessionItAbandons(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	original := materializeSibling(t, h, "abandon-before-rename")
	renamed := filepath.Join(h.Path, "abandon-after-rename")

	deferHubRelease(t, h.PrimeWorktree())
	deferHubRelease(t, renamed)

	var out bytes.Buffer
	if code := RunCLIIn(original, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, originalSession := socketAndSessionIn(t, original)
	t.Cleanup(func() {
		exec.Command(tmuxPath, "-L", socket, "kill-session", "-t", "="+originalSession).Run()
		var buf bytes.Buffer
		RunCLIIn(renamed, &buf, []string{"down"})
	})
	addStrandIn(t, original, smokeReapLaunchCmd(), "--name", "pre-rename")

	if err := os.Rename(original, renamed); err != nil {
		t.Fatalf("rename %s -> %s: %v", original, renamed, err)
	}

	out.Reset()
	if code := RunCLIIn(renamed, &out, []string{"down"}); code != 0 {
		t.Fatalf("down in the renamed worktree = %d; want 0 — down is the only lyx-only escape from the refusal, output: %s", code, out.String())
	}
	var downResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &downResult); err != nil {
		t.Fatalf("parse down result: %v", err)
	}
	if got, _ := downResult["abandonedSession"].(string); got != originalSession {
		t.Errorf("down abandonedSession = %q; want %q — deleting reed.json removes the only record naming that still-running session", got, originalSession)
	}

	// Reporting it must not become killing it: after a hand-copied .lyx the recorded name is a
	// SIBLING worktree's live session, and reed cannot tell that case from this one.
	if names := sessionNamesOnSocket(t, tmuxPath, socket); !slices.Contains(names, originalSession) {
		t.Errorf("sessions on socket %s = %v; want %s still among them — down must report the abandoned session, never kill it", socket, names, originalSession)
	}
}

// TestSmokeRemoveNeverKillsASiblingWorktreesPane is the end-to-end regression guard for the R5
// review's R5-F4, driven at the CLI seam.
//
// Reproduced live before the fix: the tmux socket is per HUB and tmux pane ids are server-global, so
// a reed.json carrying another worktree's pane ids addresses that worktree's LIVE panes.
// RemoveStrand spent its recorded pane ids as kill-pane targets with no membership check, so
// `lyx reed remove` in one worktree destroyed a sibling worktree's strand pane and its running
// process — reporting ok:true, with the sibling left showing only that its strand had died.
func TestSmokeRemoveNeverKillsASiblingWorktreesPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	victim := materializeSibling(t, h, "victim")

	deferHubRelease(t, h.PrimeWorktree())
	deferHubRelease(t, victim)

	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(victim, &buf, []string{"down"})
		buf.Reset()
		RunCLIIn(h.PrimeWorktree(), &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(victim, &out, []string{"up"}); code != 0 {
		t.Fatalf("victim up = %d; want 0, output: %s", code, out.String())
	}
	socket, victimSession := socketAndSessionIn(t, victim)
	victimGUID := addStrandIn(t, victim, smokeReapLaunchCmd(), "--name", "victim-strand")
	victimPane := paneIDForStrandIn(t, victim, victimGUID)

	out.Reset()
	if code := RunCLIIn(h.PrimeWorktree(), &out, []string{"up"}); code != 0 {
		t.Fatalf("attacker up = %d; want 0, output: %s", code, out.String())
	}

	// The hand-copy the R5 review drove: an operator moving a .lyx directory between worktrees of
	// one hub. The copied table's pane ids are the VICTIM's live panes.
	victimState, err := os.ReadFile(filepath.Join(victim, ".lyx", "reed.json"))
	if err != nil {
		t.Fatalf("read victim state: %v", err)
	}
	attackerStatePath := filepath.Join(h.PrimeWorktree(), ".lyx", "reed.json")
	if err := os.WriteFile(attackerStatePath, victimState, 0o600); err != nil {
		t.Fatalf("write copied state to %s: %v", attackerStatePath, err)
	}

	// remove may succeed or be refused; either is an acceptable outcome for a copied state file.
	// What is NOT acceptable is the victim's pane being destroyed by it.
	out.Reset()
	RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", victimGUID})

	if !sessionAlive(tmuxPath, socket, victimSession) {
		t.Fatalf("victim session %s died after a remove in another worktree", victimSession)
	}
	found := false
	for _, line := range listPaneLines(t, tmuxPath, socket, victimSession) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == victimPane && fields[1] == "0" {
			found = true
		}
	}
	if !found {
		t.Errorf("victim pane %s is gone or dead after a remove in another worktree; panes=%v — a persisted pane id must never be spent as a tmux target outside its own session",
			victimPane, listPaneLines(t, tmuxPath, socket, victimSession))
	}
}
