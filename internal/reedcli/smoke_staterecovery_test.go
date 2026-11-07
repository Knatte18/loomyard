// smoke_staterecovery_test.go drives state-loss and corruption recovery through the CLI, against a real tmux server.
// They all run in the one TestSmokeStateRecovery scenario and cover three cases.
// A reed.json outlives the session it was written for.
// A worktree is renamed while its session is up.
// A .lym directory is copied between two worktrees of one hub.
// All three are ordinary operator and environment events: a backup restore, a `mv`, a `cp -r`.
// They are not adversarial misuse, so they belong in the live suite rather than only in hermetic unit tests.
// The hermetic counterparts in reedengine (generation_test.go, strand_test.go) pin the decision each one rests on.
// This file pins the outcome an operator sees.

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
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// statusStrandLive reports whether guid is live in the output of `status` run in cwd.
// It fails the test when the strand is missing from that report entirely.
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

// sessionNamesOnSocket returns the names of every session tmux lists on socket.
// An errored or empty list-sessions means no server: tmux exits 1 with "no server running".
// That yields no sessions, not a failure.
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

// envelopeError returns the "error" field of a one-line JSON output envelope.
// It returns the message reed produced, not its JSON-escaped rendering, so an assertion compares the real text.
func envelopeError(t *testing.T, envelope []byte) string {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(envelope, &result); err != nil {
		t.Fatalf("parse output envelope %s: %v", envelope, err)
	}
	message, _ := result["error"].(string)
	if message == "" {
		t.Fatalf("output envelope %s carries no error message", envelope)
	}
	return message
}

// assertDiagnosticVerbsRefuseAndDepositNothing runs status, attach and add against cwd.
// cwd is a worktree whose reed.json names originalSession; a session still runs on socket under a different name.
// Each verb must refuse, name originalSession and the kill-session remedy, and never point at resume.
// It must also deposit no session under cwd's own session name, which is derivable but not yet created.
// status reaches the refusal directly through requireSessionLocked;
// attach and add reach it through their own self-healing boot path.
// Either way the refusal must fire before anything creates substrate.
func assertDiagnosticVerbsRefuseAndDepositNothing(t *testing.T, tmuxPath, cwd, socket, originalSession string) {
	t.Helper()
	ownSession := reedengine.SessionName(cwd)

	var out bytes.Buffer
	for _, verb := range [][]string{{"status"}, {"attach"}, {"add", "--cmd", smokeReapLaunchCmd(), "--name", "post-rename"}} {
		out.Reset()
		if code := RunCLIIn(cwd, &out, verb); code == 0 {
			t.Fatalf("%v in %s = 0; want a failure naming the orphan session, output: %s", verb, cwd, out.String())
		}
		reported := out.String()
		for _, want := range []string{originalSession, "kill-session"} {
			if !strings.Contains(reported, want) {
				t.Errorf("%v error = %s; want it to name %q so the operator can act on it", verb, reported, want)
			}
		}
		// Resume refuses for the reason the message leaves out;
		// naming it as the remedy would send the operator back into that loop.
		if strings.Contains(reported, `run "lym reed resume"`) {
			t.Errorf("%v error = %s; want it NOT to point at resume, which refuses while %q is still running", verb, reported, originalSession)
		}

		// Both self-healing verbs, add and attach, reach a boot path before this refusal,
		// so each must still refuse before depositing any substrate under cwd's own session name.
		if sessionAlive(tmuxPath, socket, ownSession) {
			t.Errorf("%v in %s deposited a session %q on socket %s despite refusing; want the refusal to leave no residue", verb, cwd, ownSession, socket)
		}
	}
}

// TestSmokeStateRecovery runs the state-loss and corruption recovery claims against one hub.
// It uses the prime worktree and one fresh sibling clone per step that needs a worktree to rename or copy from.
// The steps run serially in a fixed order, and each tears down every session it brought up,
// so each starts with no session on the hub's shared tmux server and no state in the prime worktree.
// The test calls t.Parallel, but no step does, because every step shares the one hub and server.
func TestSmokeStateRecovery(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	primeStatePath := filepath.Join(prime, ".lym", "reed.json")

	// downPrime registers a best-effort teardown of the prime worktree's session for the calling step.
	downPrime := func(t *testing.T) {
		t.Helper()
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(prime, &buf, []string{"down"})
		})
	}

	// NoSessionMessageDistinguishesAnUnreadableStateFromAnEmptyOne pins the readable/unreadable split at the call site.
	// It checks that requireSessionLocked passes the real load outcome to noSessionMessage, not a constant.
	//
	// The table test of noSessionMessage in reedengine's lifecycle_test.go covers both branches of the pure function.
	// Nothing else checks the wiring, so a hard-coded outcome would leave the hermetic and smoke suites green.
	//
	// The branch is unreachable through the hermetic execHook seam, which is why this subtest lives here:
	// it needs TmuxCmd.hasSession to answer (false, nil), which takes a real *exec.ExitError with code 1;
	// os.ProcessState offers no way to construct one.
	// A real tmux gives that answer for free: has-session against a socket with no server exits 1.
	// This subtest therefore boots no session and costs one tmux invocation per assertion.
	//
	// Both branches are asserted, so pinning either constant fails.
	// An unreadable file must not be reported as an empty worktree:
	// that sends the operator to `up`, which fails again with the corrupt-file error.
	// An absent file must still get the plain "run lym reed up" text;
	// a brand-new worktree is readable and empty, not unreadable.
	// It leaves the prime worktree without a state file, as it found it.
	if !t.Run("NoSessionMessageDistinguishesAnUnreadableStateFromAnEmptyOne", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(primeStatePath), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(primeStatePath), err)
		}
		// A truncated write is the ordinary way this file becomes unreadable:
		// a crash, a kill -9, a full disk, or a power loss partway through a save.
		if err := os.WriteFile(primeStatePath, []byte(`{"socket":"lym-x","strands":[{"gu`), 0o600); err != nil {
			t.Fatalf("write %s: %v", primeStatePath, err)
		}

		var out bytes.Buffer
		if code := RunCLIIn(prime, &out, []string{"status"}); code == 0 {
			t.Fatalf("status with an unreadable reed.json = 0; want a failure, output: %s", out.String())
		}
		unreadable := envelopeError(t, out.Bytes())
		if !strings.Contains(unreadable, "could not be read") {
			t.Errorf("status error with an unreadable reed.json = %s; want it to say reed could not read the state, not that the worktree is empty", unreadable)
		}
		if strings.Contains(unreadable, `no reed session; run "lym reed up"`) {
			t.Errorf("status error with an unreadable reed.json = %s; want it NOT to claim nothing is persisted — reed cannot tell, and `up` fails again with the corrupt-file error", unreadable)
		}

		if err := os.Remove(primeStatePath); err != nil {
			t.Fatalf("remove %s: %v", primeStatePath, err)
		}

		out.Reset()
		if code := RunCLIIn(prime, &out, []string{"status"}); code == 0 {
			t.Fatalf("status with no reed.json = 0; want a failure, output: %s", out.String())
		}
		absent := envelopeError(t, out.Bytes())
		if !strings.Contains(absent, `no reed session; run "lym reed up"`) {
			t.Errorf("status error with no reed.json at all = %s; want the plain up text — an ABSENT file is readable-and-empty, not unreadable", absent)
		}
		if strings.Contains(absent, "could not be read") {
			t.Errorf("status error with no reed.json at all = %s; want it NOT to report an unreadable state for a brand-new worktree", absent)
		}
	}) {
		return
	}

	// UpgradeFromAPreRenameStateFileHealsInOneOp pins one claim about upgrades:
	// a reed.json carrying the old "headerPaneId" key leaves the operator one stale pane for less than one op.
	//
	// The key is not read: there is no compatibility shim, no dual read, and no migration step in loadOrInitStateLocked.
	// On the first `up`, SelvagePaneID reads empty, and ensureSelvagePaneLocked splits a fresh Selvage at the bottom;
	// the session's own reconcile reaps the old header-shaped pane as untracked in the same op;
	// that reconcile is authorized by the freshly alive Selvage, so no follow-up verb is needed.
	if !t.Run("UpgradeFromAPreRenameStateFileHealsInOneOp", func(t *testing.T) {
		downPrime(t)
		mustRunReed(t, prime, "up")
		socket, session := socketAndSessionIn(t, prime)

		raw, err := os.ReadFile(primeStatePath)
		if err != nil {
			t.Fatalf("read %s: %v", primeStatePath, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("parse %s: %v", primeStatePath, err)
		}
		oldSelvagePaneID, _ := fields["selvagePaneId"].(string)
		if oldSelvagePaneID == "" {
			t.Fatalf("freshly booted state at %s carries no selvagePaneId: %s", primeStatePath, raw)
		}
		// Rewrite the state under the pre-rename key.
		// The pane it names is alive: it is Selvage's own pane, freshly split by the up above.
		// The current code does not recognize the key, not the pane's liveness.
		delete(fields, "selvagePaneId")
		fields["headerPaneId"] = oldSelvagePaneID
		rewritten, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("marshal rewritten state: %v", err)
		}
		if err := os.WriteFile(primeStatePath, rewritten, 0o600); err != nil {
			t.Fatalf("write rewritten state to %s: %v", primeStatePath, err)
		}

		mustRunReed(t, prime, "up")

		st, err := reedengine.LoadState(filepath.Join(prime, ".lym"))
		if err != nil || st == nil || st.SelvagePaneID == "" {
			t.Fatalf("LoadState after the healing up = (%+v, %v); want a freshly persisted SelvagePaneID", st, err)
		}
		if st.SelvagePaneID == oldSelvagePaneID {
			t.Fatalf("SelvagePaneID after the healing up = %s; want a NEW id distinct from the pre-rename pane %s", st.SelvagePaneID, oldSelvagePaneID)
		}

		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 1 || !paneLiveOnSession(panes, st.SelvagePaneID) {
			t.Fatalf("panes after the healing up = %v; want exactly the freshly rebuilt Selvage pane %s", panes, st.SelvagePaneID)
		}
		for _, line := range panes {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == oldSelvagePaneID {
				t.Errorf("old pre-rename pane %s still present after the healing up; want it reaped in this same op", oldSelvagePaneID)
			}
		}
	}) {
		return
	}

	// StaleStateFileIsNotMistakenForLiveStrands drives a stale reed.json through the CLI seam.
	//
	// Tmux pane ids are server-global and restart at %0 on every server rebirth.
	// A reed.json restored over a later session incarnation names panes that exist and belong to something else.
	// The file may be a backup, a resurrected untracked copy, or simply older than the running session.
	// `lym reed status` must report the strand live:false, not live:true against the new session's bare initial shell.
	// `lym reed resume` must rebuild the strand rather than answer resumed:0.
	//
	// The load-bearing assertions are that status reports live:false and that resume rebuilds the strand.
	// Asserting only the first would pass for a fix that merely stops trusting the binding;
	// such a fix leaves the strand unrestored.
	if !t.Run("StaleStateFileIsNotMistakenForLiveStrands", func(t *testing.T) {
		downPrime(t)
		mustRunReed(t, prime, "up")
		guid := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "generation-1")

		generationOne, err := os.ReadFile(primeStatePath)
		if err != nil {
			t.Fatalf("read %s: %v", primeStatePath, err)
		}

		// down + up is the ordinary route to a second incarnation: down kills the server, so the next
		// up's pane ids restart at %0 and collide with the ids generation 1 recorded.
		mustRunReed(t, prime, "down")
		mustRunReed(t, prime, "up")

		if err := os.WriteFile(primeStatePath, generationOne, 0o600); err != nil {
			t.Fatalf("restore %s: %v", primeStatePath, err)
		}

		live, paneID := statusStrandLive(t, prime, guid)
		if live {
			t.Errorf("status strand %s live = true on pane %q after a stale reed.json was restored; want false — that pane belongs to a different session incarnation", guid, paneID)
		}

		var resumeResult map[string]any
		if err := json.Unmarshal(mustRunReed(t, prime, "resume"), &resumeResult); err != nil {
			t.Fatalf("parse resume result: %v", err)
		}
		if dropped, ok := resumeResult["dropped"].(float64); !ok || dropped != 0 {
			t.Errorf("resume dropped = %v; want 0 present for a state with no done-when lists", resumeResult["dropped"])
		}
		if resumed, _ := resumeResult["resumed"].(float64); resumed < 1 {
			t.Errorf("resume resumed = %v; want at least 1 — the stale binding must not make resume skip a strand whose process is not running", resumeResult["resumed"])
		}

		if live, _ := statusStrandLive(t, prime, guid); !live {
			t.Errorf("status strand %s live = false after resume rebuilt it; want true", guid)
		}
	}) {
		return
	}

	// RenamedWorktreeRefusesRatherThanDoubleLaunching drives a renamed worktree through the CLI seam.
	//
	// The tmux session name derives from the worktree basename, while .lym travels with the directory.
	// Renaming a worktree whose session is up makes `lym reed resume` boot a second session.
	// Resume then relaunches every strand into that session, so each strand's process runs twice.
	// The orphaned original session then survives `lym reed down` in the renamed worktree.
	// No reed verb can address it, since no worktree of that name exists to derive the name from.
	//
	// The sibling is a plain clone, not a linked git worktree, so it can be renamed:
	// Moving a linked worktree breaks its gitdir link.
	// lymcwd.Resolve would then fail for an unrelated reason, masking the behaviour under test.
	if !t.Run("RenamedWorktreeRefusesRatherThanDoubleLaunching", func(t *testing.T) {
		original := materializeSibling(t, h, "before-rename")
		renamed := filepath.Join(h.Path, "after-rename")

		// Only the renamed path is released: deferHubRelease waits up to 100s for a directory to become renameable.
		// The pre-rename path is gone by cleanup time.
		// Registering it would burn that whole budget on a path nothing is holding.
		deferHubRelease(t, renamed)
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(renamed, &buf, []string{"down"})
		})

		mustRunReed(t, original, "up")
		socket, originalSession := socketAndSessionIn(t, original)
		addStrandIn(t, original, smokeReapLaunchCmd(), "--name", "pre-rename")

		if err := os.Rename(original, renamed); err != nil {
			t.Fatalf("rename %s -> %s: %v", original, renamed, err)
		}

		var out bytes.Buffer
		if code := RunCLIIn(renamed, &out, []string{"resume"}); code == 0 {
			t.Fatalf("resume in the renamed worktree = 0; want a refusal — continuing double-launches every strand and orphans %q, output: %s", originalSession, out.String())
		}
		refusal := out.String()
		for _, want := range []string{originalSession, "kill-session"} {
			if !strings.Contains(refusal, want) {
				t.Errorf("resume refusal = %s; want it to contain %q so the operator can act on it", refusal, want)
			}
		}

		// The refusal must leave no residue:
		// a second session on the shared per-hub server is the stranded-substrate outcome reed refuses before creating.
		names := sessionNamesOnSocket(t, tmuxPath, socket)
		if len(names) != 1 || names[0] != originalSession {
			t.Errorf("sessions on socket %s = %v; want exactly [%s] — a refusal must not deposit a session of its own", socket, names, originalSession)
		}

		// The remedy the refusal names must clear it.
		if err := exec.Command(tmuxPath, "-L", socket, "kill-session", "-t", "="+originalSession).Run(); err != nil {
			t.Fatalf("kill-session -t =%s: %v", originalSession, err)
		}
		out.Reset()
		if code := RunCLIIn(renamed, &out, []string{"resume"}); code != 0 {
			t.Fatalf("resume after the orphan was killed = %d; want 0 — the refusal must be escapable by the remedy it names, output: %s", code, out.String())
		}
	}) {
		return
	}

	// DiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume checks the foreign-session refusal through the CLI seam.
	//
	// Both ordinary routes into the refusal leave this worktree's session absent:
	// a worktree renamed while its session was up, and a .lym copied between worktrees of one hub.
	// Every non-booting verb then reaches requireSessionLocked, which must name the still-running session.
	// It must not point at `lym reed resume` or `lym reed up`, since both refuse with the orphan-session error.
	//
	// attach and add reach the same diagnosis through their own self-healing boot path.
	// refuseRecordedForeignSessionBeforeBootLocked consults the same diagnosis before anything creates substrate.
	// Status, attach and add all refuse with the same message.
	// The refusal deposits nothing on the hub socket under this worktree's own session name.
	//
	// The rename and copied-state cases share their helpers and assertion body.
	//
	// The diagnosis is asserted at the call site, not in the helper.
	// refuseLiveForeignSessionLocked has its own hermetic coverage.
	// This test pins that requireSessionLocked and both self-healing boot paths consult the diagnosis.
	// Removing any of those calls would reintroduce the misleading pointer or deposit a stray session;
	// the assertions below catch both.
	if !t.Run("DiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume", func(t *testing.T) {
		t.Run("worktree renamed while its session was up", func(t *testing.T) {
			original := materializeSibling(t, h, "diag-before-rename")
			renamed := filepath.Join(h.Path, "diag-after-rename")

			deferHubRelease(t, renamed)

			mustRunReed(t, original, "up")
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

			assertDiagnosticVerbsRefuseAndDepositNothing(t, tmuxPath, renamed, socket, originalSession)
		})

		t.Run(".lym copied between worktrees of one hub while the original session was up", func(t *testing.T) {
			original := materializeSibling(t, h, "diag-copy-source")
			copied := materializeSibling(t, h, "diag-copy-target")

			deferHubRelease(t, original)
			deferHubRelease(t, copied)

			mustRunReed(t, original, "up")
			socket, originalSession := socketAndSessionIn(t, original)
			t.Cleanup(func() {
				var buf bytes.Buffer
				RunCLIIn(copied, &buf, []string{"down"})
				buf.Reset()
				RunCLIIn(original, &buf, []string{"down"})
			})
			addStrandIn(t, original, smokeReapLaunchCmd(), "--name", "copy-source-strand")

			// A hand-copied .lym directory: an operator moves it between worktrees of one hub.
			// The original session is still live.
			originalState, err := os.ReadFile(filepath.Join(original, ".lym", "reed.json"))
			if err != nil {
				t.Fatalf("read original state: %v", err)
			}
			copiedStatePath := filepath.Join(copied, ".lym", "reed.json")
			if err := os.MkdirAll(filepath.Dir(copiedStatePath), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", filepath.Dir(copiedStatePath), err)
			}
			if err := os.WriteFile(copiedStatePath, originalState, 0o600); err != nil {
				t.Fatalf("write copied state to %s: %v", copiedStatePath, err)
			}

			assertDiagnosticVerbsRefuseAndDepositNothing(t, tmuxPath, copied, socket, originalSession)
		})
	}) {
		return
	}

	// DownReportsTheSessionItAbandons is an end-to-end regression guard, driven at the CLI seam.
	//
	// down loads no state, so it never reaches the foreign-session refusal.
	// That makes it the only lym-only escape from the refusal, and the one a tmux-less operator is steered toward.
	// The bug it guards against: in a renamed worktree, down reports {"ok":true,...} and deletes reed.json.
	// It leaves the recorded session and its strand process running on the shared per-hub socket.
	// No reed verb can address them afterwards, and nothing left on disk names them.
	//
	// The test asserts that down still succeeds, since it is the escape and it is idempotent;
	// that the envelope names the abandoned session; and that down does not kill it.
	if !t.Run("DownReportsTheSessionItAbandons", func(t *testing.T) {
		original := materializeSibling(t, h, "abandon-before-rename")
		renamed := filepath.Join(h.Path, "abandon-after-rename")

		deferHubRelease(t, renamed)

		mustRunReed(t, original, "up")
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

		var out bytes.Buffer
		if code := RunCLIIn(renamed, &out, []string{"down"}); code != 0 {
			t.Fatalf("down in the renamed worktree = %d; want 0 — down is the only lym-only escape from the refusal, output: %s", code, out.String())
		}
		var downResult map[string]any
		if err := json.Unmarshal(out.Bytes(), &downResult); err != nil {
			t.Fatalf("parse down result: %v", err)
		}
		if got, _ := downResult["abandonedSession"].(string); got != originalSession {
			t.Errorf("down abandonedSession = %q; want %q — deleting reed.json removes the only record naming that still-running session", got, originalSession)
		}

		// Reporting the session must not become killing it.
		// After a hand-copied .lym, the recorded name belongs to a sibling worktree's live session.
		// Reed cannot tell that case from this one.
		if names := sessionNamesOnSocket(t, tmuxPath, socket); !slices.Contains(names, originalSession) {
			t.Errorf("sessions on socket %s = %v; want %s still among them — down must report the abandoned session, never kill it", socket, names, originalSession)
		}
	}) {
		return
	}

	// RemoveNeverKillsASiblingWorktreesPane is an end-to-end regression guard, driven at the CLI seam.
	//
	// The tmux socket is per hub and tmux pane ids are server-global.
	// So a reed.json carrying another worktree's pane ids addresses that worktree's live panes.
	// Removing a strand must never kill a pane it does not own: the sibling's process would die with it.
	t.Run("RemoveNeverKillsASiblingWorktreesPane", func(t *testing.T) {
		victim := materializeSibling(t, h, "victim")

		deferHubRelease(t, victim)
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(victim, &buf, []string{"down"})
			buf.Reset()
			RunCLIIn(prime, &buf, []string{"down"})
		})

		mustRunReed(t, victim, "up")
		socket, victimSession := socketAndSessionIn(t, victim)
		victimGUID := addStrandIn(t, victim, smokeReapLaunchCmd(), "--name", "victim-strand")
		victimPane := paneIDForStrandIn(t, victim, victimGUID)

		mustRunReed(t, prime, "up")

		// The pane ids in a copied reed.json are the victim's live panes.
		victimState, err := os.ReadFile(filepath.Join(victim, ".lym", "reed.json"))
		if err != nil {
			t.Fatalf("read victim state: %v", err)
		}
		if err := os.WriteFile(primeStatePath, victimState, 0o600); err != nil {
			t.Fatalf("write copied state to %s: %v", primeStatePath, err)
		}

		// Remove may succeed or be refused; both outcomes are acceptable for a copied state file.
		// Destroying the victim's pane is the one unacceptable outcome.
		var out bytes.Buffer
		RunCLIIn(prime, &out, []string{"remove", victimGUID})

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
	})
}
