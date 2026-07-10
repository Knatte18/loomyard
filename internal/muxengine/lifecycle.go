// lifecycle.go implements the four lifecycle engine ops — Up, Resume, Down,
// Status — plus the pure planning helpers that make their decisions
// unit-testable without a live psmux server. The sharp boundary the batch
// discussion settles on: Up ensures the substrate (server + session) exists
// and never launches a strand command; Resume is the only replayer, and it
// skips anchor:hidden strands (pending first surface, not dead).

package muxengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
	"github.com/Knatte18/loomyard/internal/proc"
)

// UpResult reports the outcome of Up: the resolved session/socket identity
// and how many strands are currently tracked in the persisted table.
type UpResult struct {
	Session string
	Socket  string
	Strands int
}

// ResumeResult reports the outcome of Resume: the session name and how many
// not-live, non-hidden strands were relaunched.
type ResumeResult struct {
	Session string
	Resumed int
}

// DownResult reports the outcome of Down: the session name that was torn
// down.
type DownResult struct {
	Session string
}

// StrandStatus is one strand's reporting projection for StatusResult: its
// identity plus whatever mux can currently observe about it.
type StrandStatus struct {
	GUID   string
	Name   string
	PaneID string
	Live   bool
}

// StatusResult reports this session's tracked strands and their live/dead
// state. Status only reports this session — active stray-server
// enumeration across the hub is deferred (NOTE3).
type StatusResult struct {
	Session string
	Socket  string
	Strands []StrandStatus
}

// Boot-loop tuning for ensureServerAndSessionLocked. bootAttemptTimeout is
// the per-spawn window before a still-session-less socket is treated as a
// zombie boot and reaped; bootOverallTimeout is the total budget across
// spawn/reap/respawn cycles, sized for a CPU-saturated machine (a quiet boot
// is ~1-2s; three concurrent smoke suites have been observed to starve a
// boot past two full 20s windows). staleSocketGrace is how long a
// session-less socket-holder must persist before the pre-boot check treats
// it as stale rather than a sibling worktree's still-registering fresh boot.
const (
	bootAttemptTimeout = 20 * time.Second
	bootPoll           = 100 * time.Millisecond
	bootOverallTimeout = 90 * time.Second
	staleSocketGrace   = 5 * time.Second
)

// planUpLaunches always returns nil: Up never launches or relaunches a
// strand command — only Resume replays. This trivial-but-explicit function
// exists so Up's "never launches" contract has a concrete, unit-testable
// seam symmetric with planResumeLaunches, rather than being an implicit
// absence of behavior a reader has to infer from Up's body never calling
// launchStrandLocked.
func planUpLaunches(strands []Strand) []Strand {
	return nil
}

// planResumeLaunches returns the strands Resume must (re)launch: every
// strand that is not live (no pane, or its pane is absent from liveIDs) and
// not anchor:hidden. A hidden strand is "pending first surface", not dead,
// so Resume must not surface it (GAP1) — that is UpdateStrand's job.
// liveIDs is the set of pane ids currently present in the psmux window per
// list-panes, matching toRenderStrands' Live derivation.
func planResumeLaunches(strands []Strand, liveIDs map[string]bool) []Strand {
	var out []Strand
	for _, s := range strands {
		live := s.PaneID != "" && liveIDs[s.PaneID]
		if live {
			continue
		}
		if s.Display.Anchor == render.AnchorHidden {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ensureServerAndSessionLocked ensures this hub's named psmux server and
// this worktree's session exist, spawning a fresh server via a raw
// new-session when has-session reports absent. It never runs a strand
// command: Up composes only this plus reconcile/apply, and Resume replays
// separately via launchStrandLocked after this returns — matching the
// sharp up=substrate / resume=replay boundary. It assumes the op lock is
// already held and always makes a real psmux round trip.
// It reports booted=true when it spawned a fresh session and false when the
// session already existed, so callers can tell a server rebirth (bindings are
// all stale) from a normal no-op bring-up; strippedKeys names the env keys
// CleanClaudeEnv removed from the spawn env (nil when nothing was booted),
// so the caller can stamp them into MuxState.StrippedEnv for diagnosis.
// Before any of that, it runs the capability probe (probe.go) once and
// returns a *CapabilityError immediately if the configured multiplexer
// binary is below the pinned version floor or missing a required
// subcommand.
func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []string, err error) {
	// Fail loud, once per ensure/boot, if the configured multiplexer binary
	// is below the pinned version floor or missing a required subcommand —
	// far better than letting an unknown surface surface later as a cryptic
	// psmux/tmux error deep inside the boot loop below.
	if err := e.probeCapabilityLocked(); err != nil {
		return false, nil, err
	}

	session := e.SessionName()
	up, err := e.psmux.hasSession(session)
	if err != nil {
		return false, nil, fmt.Errorf("check session: %w", err)
	}
	if up {
		// A session that exists but holds ZERO panes is broken substrate: it
		// cannot host a strand (there is no pane to adopt or split, and psmux
		// offers no way to add a pane to an empty window), so add would fail
		// forever while up kept reporting success. The state is reachable
		// when an applied layout once destroyed every pane (psmux reaps any
		// pane absent from a select-layout string). Kill the husk and fall
		// through to a fresh boot — the booted=true return then makes the
		// caller clear every stale binding, exactly like a server rebirth.
		live, err := e.psmux.listPanes(session)
		if err != nil {
			return false, nil, fmt.Errorf("list panes: %w", err)
		}
		if len(live) > 0 {
			return false, nil, nil
		}
		_ = e.psmux.run("kill-session", "-t", session)
	}

	// A stale socket-holder wedges a fresh boot: psmux's internal "__warm__"
	// helper can outlive a kill-server and sit on the -L socket without ever
	// hosting a session, so a new-session spawned against it never
	// materializes. A socket whose holder reports zero sessions across the
	// grace window is such a stale helper, a dying server, or an unreachable
	// zombie — never a healthy shared server (sibling worktrees' sessions
	// would list) — so force-reaping it before spawning is safe.
	if e.sessionlessSocketHolderPersists() {
		if err := e.reapSocketProcesses(); err != nil {
			return false, nil, fmt.Errorf("stale psmux socket-holder: %w", err)
		}
	}

	// Env hygiene: a spawned server must never inherit this process's own
	// Claude Code session identity (CleanClaudeEnv is the single documented
	// chokepoint for that decision).
	clean, stripped := CleanClaudeEnv(os.Environ())
	spawnSession := func() error {
		cmd := exec.Command(e.cfg.Psmux,
			"-L", e.Socket(),
			"new-session", "-d", "-s", session,
			"-x", strconv.Itoa(e.cfg.Width),
			"-y", strconv.Itoa(e.cfg.Height),
			e.cfg.Pwsh,
		)
		cmd.Env = clean
		proc.Detach(cmd)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start psmux: %w", err)
		}
		return nil
	}

	// Boot with a deadline-based retry. Two distinct slow paths hide behind
	// "the session is not answering yet": (a) an honestly slow boot on a
	// loaded machine — a real boot is ~1-2s quiet but has been observed to
	// exceed two full 20s attempt windows when several concurrent test
	// suites peg the CPU, so the loop retries against an overall deadline
	// rather than a fixed attempt count; and (b) a ZOMBIE boot — psmux has a
	// race under concurrent server startups where the spawned server process
	// runs but never becomes reachable on its socket (list-sessions empty,
	// has-session exit 1, forever), which no amount of waiting fixes. After
	// a full attempt window with the socket still session-less, everything
	// on the socket is force-reaped by pid (kill-server cannot reach an
	// unreachable zombie) and the spawn retried. If the socket DOES list
	// sessions, the server is healthy/shared and is never reaped — the error
	// then reports the truly unexpected state instead. A genuine
	// never-boots regression still fails, at bootOverallTimeout instead of
	// after two attempts.
	bootDeadline := time.Now().Add(bootOverallTimeout)
	for {
		if err := spawnSession(); err != nil {
			return false, nil, err
		}

		attemptDeadline := time.Now().Add(bootAttemptTimeout)
		sessionUp := false
		for time.Now().Before(attemptDeadline) {
			up, err := e.psmux.hasSession(session)
			if err != nil {
				return false, nil, fmt.Errorf("check session: %w", err)
			}
			if up {
				sessionUp = true
				break
			}
			time.Sleep(bootPoll)
		}
		if sessionUp {
			break
		}

		if out, err := e.psmux.output("list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(out) != "" {
			return false, nil, fmt.Errorf("psmux server is up but session %q did not materialize within %s", session, bootAttemptTimeout)
		}
		if err := e.reapSocketProcesses(); err != nil {
			return false, nil, fmt.Errorf("reap zombie psmux boot: %w", err)
		}
		if time.Now().After(bootDeadline) {
			return false, nil, fmt.Errorf("psmux session did not start within %s", bootOverallTimeout)
		}
	}

	// remain-on-exit keeps a pane whose command exits around as
	// pane_dead=1 instead of vanishing (which would also kill the session
	// if it were the last pane) — the mechanism reconcile's dead-pane
	// detection depends on.
	if err := e.psmux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		return false, nil, fmt.Errorf("set remain-on-exit: %w", err)
	}
	return true, stripped, nil
}

// Up ensures the named server and this worktree's session exist (booting
// them if absent, no-op if already up), then reconciles and re-applies the
// layout from the current strand table. Up never launches or relaunches a
// strand command — bringing strand content back after a server restart is
// Resume's job, not Up's.
func (e *Engine) Up() (UpResult, error) {
	var result UpResult
	err := e.withOpLock(func() error {
		booted, stripped, err := e.ensureServerAndSessionLocked()
		if err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		// On a server rebirth the reborn session reuses pane ids (the initial
		// pane is %1 again), so a persisted binding would be mistaken for a
		// live strand. Clear every binding: a just-booted session hosts none
		// of the prior strands. Up leaves them not-live (Resume rebuilds them).
		// The stripped env keys are stamped for diagnosis — mux.json records
		// what the server spawn actually removed.
		if booted {
			clearAllPaneBindings(st)
			st.StrippedEnv = stripped
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result = UpResult{Session: e.SessionName(), Socket: e.Socket(), Strands: len(st.Strands)}
		return nil
	})
	return result, err
}

// Resume boots the server+session if absent, then reconciles first (clearing
// any stale pane bindings a crashed-and-reborn server left behind — a
// standalone resume-after-crash has no earlier op in this process to have
// run reconcile already, unlike add/surface's up-then-mutate sequencing).
// Without this, a strand's stale non-empty PaneID from before the crash
// would make planLaunch see a "pane already held" table and split instead of
// adopting the new session's sole initial pane, leaving it orphaned and
// causing the final layout apply to enumerate one pane fewer than psmux
// actually holds (GAP2). Then — for every persisted strand that is not live
// and not anchor:hidden — it realizes the strand into a live pane via the
// shared launchStrandLocked (GAP A), replaying its ResumeCmd (or Cmd, when
// ResumeCmd is empty; every strand has at least a Cmd, so every such strand
// is rebuildable). Already-live strands are left untouched (no double
// send-keys); hidden strands are skipped (pending first surface, not dead —
// GAP1). Finishes by reconciling again, re-applying the layout, and
// re-persisting pane ids (the reconcileApplyPersistLocked tail is a cheap
// no-op re-check when the pre-launch reconcile already left the table
// clean).
func (e *Engine) Resume() (ResumeResult, error) {
	var result ResumeResult
	err := e.withOpLock(func() error {
		booted, stripped, err := e.ensureServerAndSessionLocked()
		if err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		// On a server rebirth the reborn session reuses pane ids, so a stale
		// binding would look live to reconcile below and wrongly skip relaunch.
		// Clear every binding first so all non-hidden strands are rebuilt.
		if booted {
			clearAllPaneBindings(st)
			st.StrippedEnv = stripped
		}

		live, err := e.psmux.listPanes(e.SessionName())
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		killed, err := e.reconcileLocked(st, live)
		if err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if len(killed) > 0 {
			live, err = e.psmux.listPanes(e.SessionName())
			if err != nil {
				return fmt.Errorf("list panes after reconcile: %w", err)
			}
		}
		// aliveIDSet, not liveIDSet: a strand bound to a dead-but-present pane
		// (e.g. the kept sole dead pane) is not live and must be relaunched.
		toLaunch := planResumeLaunches(st.Strands, aliveIDSet(live))

		launch := make(map[string]bool, len(toLaunch))
		for _, s := range toLaunch {
			launch[s.GUID] = true
		}
		launched := 0
		for i := range st.Strands {
			if !launch[st.Strands[i].GUID] {
				continue
			}
			resumeCmd := st.Strands[i].ResumeCmd
			if resumeCmd == "" {
				resumeCmd = st.Strands[i].Cmd
			}
			if err := e.launchStrandLocked(st, &st.Strands[i], resumeCmd); err != nil {
				return fmt.Errorf("resume strand %s: %w", st.Strands[i].GUID, err)
			}
			// Persist immediately after each launch, before the apply below —
			// same orphan-avoidance as AddStrand: if a later launch or apply
			// fails, this pane is already tracked, so it is never reaped as
			// untracked or double-launched by the next resume.
			if err := SaveState(e.layout.DotLyxDir(), st); err != nil {
				return fmt.Errorf("persist strand: %w", err)
			}
			// Re-apply the layout after each launch, not once at the end:
			// consecutive splits without a re-apply halve the same target
			// pane until psmux silently refuses to split it, while a
			// re-apply re-stretches the bottom/active pane so the next
			// launch always splits the tallest pane the policy just sized.
			if _, err := e.reconcileApplyPersistLocked(st); err != nil {
				return err
			}
			launched++
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result = ResumeResult{Session: e.SessionName(), Resumed: launched}
		return nil
	})
	return result, err
}

// Down tears this worktree's session down: kill-session (never kill-server
// while sibling worktrees still hold sessions — the per-hub server is shared,
// and killing it would destroy theirs too) and delete mux.json (ignoring
// not-exist). Errors from kill-session are ignored so Down stays idempotent
// against an already-stopped session. When this was the last session on the
// server, the now-empty server is cleaned up — and Down then WAITS until the
// server process has actually released the socket: psmux's kill-server is
// asynchronous, and returning while the old server still holds the socket
// lets an immediately following up spawn a second server process on the same
// -L name, whose loser lingers forever as an unreachable stray (observed
// live under down->up churn). Down also waits for this session's pane CHILD
// processes (the shell psmux ran in each pane, and whatever it launched) to
// exit before returning: psmux terminates pane children asynchronously, so a
// Down that waited only on the server process could return while a pane's
// shell — whose cwd is the worktree — was still alive, a "no stray state"
// violation (observed as a worktree-dir-in-use failure under load). It does
// not reconcile or apply — there is nothing left to render once the state
// file is gone.
func (e *Engine) Down() (DownResult, error) {
	var result DownResult
	err := e.withOpLock(func() error {
		// Grab the server's OS pid while our session can still be queried —
		// it is the only reliable death signal: psmux's CLI cannot report
		// "no server" at all (list-sessions exits 0 with empty output and
		// kill-server exits 0 whether or not a server holds the socket).
		serverPID := e.serverPIDLocked()

		// Capture this session's pane process subtrees BEFORE kill-session,
		// while the panes still exist to be listed — the shells psmux ran in
		// each pane plus their descendants (on Windows the process actually
		// holding the worktree directory is a deeper descendant of the pane
		// pid). Reaping them is how down keeps its "no stray state" guarantee
		// at the pane level (see reapPaneChildren).
		panePIDs := e.paneProcessTreePIDsLocked()

		// Ignore the error: the session may already be gone, and Down must
		// stay idempotent either way.
		_ = e.psmux.run("kill-session", "-t", e.SessionName())

		// Tidy the server only if no sessions remain. An EMPTY list-sessions
		// covers both "zero sessions" and "no server" (psmux does not
		// distinguish them, and kill-server is harmless in both); an ERRORED
		// list-sessions means the socket-holder is unreachable — a zombie
		// server cannot be hosting healthy sibling sessions, so it is torn
		// down too rather than left squatting on the socket with the
		// worktree as its cwd (the same sessionless-holder reasoning the
		// pre-boot check applies).
		var serverErr error
		if out, err := e.psmux.output("list-sessions", "-F", "#{session_name}"); err != nil || strings.TrimSpace(out) == "" {
			_ = e.psmux.run("kill-server")
			serverErr = e.ensureServerGoneLocked(serverPID)
		}

		// ALWAYS reap this session's pane child subtree, even when the server
		// teardown above hit trouble: a slow or failed server death must never
		// skip the pane reap. An earlier fixed-deadline server wait returned on
		// timeout and aborted down BEFORE this reap under CPU saturation,
		// leaking both the server (whose cwd is this worktree) and pane children
		// that kept the worktree directory busy. kill-session / kill-server
		// terminate pane children asynchronously, so force-kill any that outlive
		// the deadline.
		reapPaneChildren(panePIDs, reapExitTimeout)

		if serverErr != nil {
			return serverErr
		}

		path := filepath.Join(e.layout.DotLyxDir(), muxStateFileName)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete state: %w", err)
		}

		result = DownResult{Session: e.SessionName()}
		return nil
	})
	return result, err
}

// sessionlessSocketHolderPersists reports whether a psmux process is
// squatting on this engine's socket without hosting any session, and keeps
// doing so across staleSocketGrace. The grace window exists because the mux
// op lock is per-worktree: a SIBLING worktree's up may have just spawned
// this shared server, and between its process appearing and its session
// registering the socket looks exactly like a stale holder — reaping it then
// would kill the sibling's healthy boot out from under it. A genuinely stale
// holder (a "__warm__" helper that outlived kill-server, a dying server, an
// unreachable zombie) stays session-less forever, so waiting the grace out
// never misses it. The common fresh-boot path (nothing on the socket)
// returns false on the first probe.
func (e *Engine) sessionlessSocketHolderPersists() bool {
	deadline := time.Now().Add(staleSocketGrace)
	for {
		// A socket that lists sessions hosts a healthy shared server — never
		// in scope for reaping, no matter what else is pending.
		if out, err := e.psmux.output("list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(out) != "" {
			return false
		}
		if len(e.serverProcessesOnSocket()) == 0 {
			return false
		}
		if time.Now().After(deadline) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// serverPIDLocked returns the psmux server's OS pid as psmux reports it via
// the #{pid} format variable, or 0 when it cannot be determined (server or
// session absent, unparseable output) — callers treat 0 as "nothing to wait
// on". It targets this worktree's session, so it must run BEFORE
// kill-session when the caller intends to wait on the server afterwards.
// It assumes the op lock is already held.
func (e *Engine) serverPIDLocked() int {
	out, err := e.psmux.output("display-message", "-p", "-t", e.SessionName(), "#{pid}")
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return pid
}

// panePIDsLocked returns the OS pids of this worktree's session's pane child
// processes — the immediate shell psmux launched in each pane, as psmux
// reports them via the #{pane_pid} format variable (carried on LivePane by
// listPanes). It returns nil when the session is absent or the query fails
// (callers treat that as "no children to reap"). It must run BEFORE
// kill-session, while the panes still exist to be listed, and assumes the op
// lock is already held. Callers that need the process actually holding the
// worktree directory want the whole subtree (paneProcessTreePIDsLocked), not
// just these launcher pids.
func (e *Engine) panePIDsLocked() []int {
	live, err := e.psmux.listPanes(e.SessionName())
	if err != nil {
		return nil
	}
	var pids []int
	for _, p := range live {
		if p.PID > 0 {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// paneProcessTreePIDsLocked returns this session's pane child pids AND their
// full descendant subtrees — the snapshot Down reaps after kill-session. It
// returns nil when there is no session or pane. Must run BEFORE
// kill-session, while the panes still exist, and assumes the op lock is
// held.
func (e *Engine) paneProcessTreePIDsLocked() []int {
	return e.descendantClosurePIDs(e.panePIDsLocked())
}

// forceKillExitGrace bounds how long reapPaneChildren waits, per pid, for a
// force-killed process to actually exit. TerminateProcess is asynchronous on
// Windows, so a kill with no follow-up wait closes nothing — the killed
// process can still hold the worktree directory when the caller returns. It is
// generous because on a CPU-saturated machine even a TerminateProcess'd process
// can take seconds to be reaped by the OS and release its handles.
const forceKillExitGrace = 5 * time.Second

// reapExitTimeout bounds how long the pane-child and server reaps wait for a
// graceful async teardown before force-killing stragglers. It is deliberately
// generous rather than the old fixed 5s: on a CPU-saturated machine psmux's
// async pane/server teardown — and the Win32_Process probe the reap relies on —
// both slow down, so a tight deadline risks force-killing prematurely (harmless)
// or, in the pre-fix code, a fixed wait that ERRORED and aborted the teardown.
// The reaps confirm each process is actually gone rather than trusting the
// timer, so this value only bounds a pathological hang; the common quiet-machine
// path returns as soon as the processes exit.
const reapExitTimeout = 15 * time.Second

// ensureServerGoneLocked guarantees no psmux process remains on this engine's
// socket after a kill-server, so down provably leaves zero psmux for its socket
// the moment it returns. kill-server is asynchronous and, under CPU saturation,
// the main server AND its "__warm__" helper — both spawned with the worktree as
// their cwd, so both keep the worktree directory busy — can outlive any fixed
// wait. It first gives the graceful teardown a bounded window to finish on its
// own (the common, quiet-machine path), then, if any psmux still names the
// socket, force-reaps them and confirms the socket is clear. Force-reap-and-
// confirm rather than a fixed wait that aborts down: a stray server holding the
// worktree directory is a real "no stray state" leak, so down must actively
// clear it, never merely hope it dies in time. Returns an error only if the
// socket is still not clear after the force-reap — a genuine, reportable
// failure. It assumes the op lock is already held.
func (e *Engine) ensureServerGoneLocked(serverPID int) error {
	_ = waitProcessExit(serverPID, reapExitTimeout)
	if len(e.serverProcessesOnSocket()) == 0 {
		return nil
	}
	return e.reapSocketProcesses()
}

// reapPaneChildren waits for every pane child process to exit, force-killing
// any that outlive the deadline, so a pane-destroying op leaves no pane
// grandchild holding worktree resources. psmux terminates pane children
// asynchronously when a pane/session/server is killed, so an op that
// returned without this wait could do so while a pane's shell (whose cwd is
// the worktree) was still alive — the "no stray state" gap that surfaces as
// a worktree-dir-in-use failure under load. The wait is the normal path (the
// graceful psmux teardown reaps each child moments later); the force-kill
// fires only for a pid that failed to exit within the deadline, so it can
// never target a reused pid (a pid that never exited cannot have been
// reused) — and it then waits again, briefly, for the kill to land, because
// a fire-and-forget TerminateProcess leaves the same window it was supposed
// to close.
func reapPaneChildren(pids []int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if err := waitProcessExit(pid, time.Until(deadline)); err == nil {
			continue
		}
		p, findErr := os.FindProcess(pid)
		if findErr != nil {
			continue
		}
		_ = p.Kill()
		_ = waitProcessExit(pid, forceKillExitGrace)
	}
}

// waitServerProcessesGone polls serverProcessesOnSocket until no psmux
// process names this socket, erroring after timeout. It is the belt to
// waitProcessExit's suspenders: the pid wait covers the main server exactly
// and instantly, while this drain also catches the "__warm__" helper, which
// has no queryable pid of its own and has been observed to outlive the main
// server and wedge the next boot by squatting on the socket.
func (e *Engine) waitServerProcessesGone(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		pids := e.serverProcessesOnSocket()
		if len(pids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("psmux processes %v still on socket %s after %s", pids, e.Socket(), timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// reapSocketProcesses force-terminates every psmux process on this engine's
// socket and drains until the process table confirms they are gone: a
// graceful kill-server first, then TerminateProcess by pid — necessary
// because a zombie-booted server (running but never reachable on its
// socket) and a lagging "__warm__" helper both ignore the socket-routed
// kill-server entirely. Callers must first establish the socket hosts no
// live sessions (list-sessions empty/unreachable), so a healthy shared
// server is never in scope here.
func (e *Engine) reapSocketProcesses() error {
	_ = e.psmux.run("kill-server")
	for _, pid := range e.serverProcessesOnSocket() {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
		}
	}
	return e.waitServerProcessesGone(reapExitTimeout)
}

// waitProcessExit blocks until the process with pid has exited, or errors
// after timeout. A pid of 0 (unknown) and an already-gone process both
// return nil immediately. This exists because psmux's kill-server is
// asynchronous AND its CLI cannot report server absence (every probe exits
// 0), so the only trustworthy "the socket is free" signal is the server
// process itself disappearing — without this wait, a down immediately
// followed by up spawns a duplicate server process on the same -L name,
// whose loser lingers forever as an unreachable stray. On Windows (psmux's
// only platform in practice) os.Process.Wait works for non-child processes;
// on other platforms Wait errors immediately for a non-child, which the
// select treats as "done" — a benign no-wait rather than a failure.
func waitProcessExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		_, _ = proc.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("psmux server (pid %d) still up %s after kill-server", pid, timeout)
	}
}

// requireSessionLocked returns a friendly, actionable error when this
// worktree's psmux session does not exist, instead of letting a caller fall
// through to a raw psmux error surfacing later from deep inside
// launchStrandLocked or listPanes. Status has always pre-flighted this way;
// AddStrand and RemoveStrand share the identical check (same error string)
// because both hit psmux directly with no earlier session check of their
// own — AddStrand via launchStrandLocked, RemoveStrand via
// reconcileApplyPersistLocked's listPanes — which otherwise surfaces a
// cryptic psmux error when a caller runs add/remove before up. It assumes
// the op lock is already held and always makes a real psmux round trip.
func (e *Engine) requireSessionLocked() error {
	up, err := e.psmux.hasSession(e.SessionName())
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !up {
		return fmt.Errorf(`no mux session; run "lyx mux up"`)
	}
	return nil
}

// Status reports this session's tracked strands (guid, name, pane id,
// live/dead) purely by cross-referencing the persisted table against the
// live pane set list-panes just reported. Status is a read verb, so unlike
// the mutating ops it must not touch psmux beyond the read-only
// has-session/list-panes calls: it does NOT run reconcileLocked (which kills
// dead-but-not-sole panes and clears their strands' bindings — a real state
// correction that belongs to a mutating op, not a query) and it does NOT run
// applyLayoutLocked's select-layout/select-pane (which would move input
// focus and rewrite the window layout as a side effect of a query). The
// persisted PaneID is reported unchanged; Live is derived by checking it
// against the live set. Nothing is lost by leaving mux.json as-is between
// queries — the next mutating op reconciles and persists the correction
// itself. It returns a non-nil error when the server/session is absent, so a
// pre-flight caller (e.g. attach) can surface that on its envelope before
// attempting anything that needs a live session. Status only reports this
// session — active stray-server enumeration across the hub is deferred
// (NOTE3).
func (e *Engine) Status() (StatusResult, error) {
	var result StatusResult
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		session := e.SessionName()

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		live, err := e.psmux.listPanes(session)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}

		// aliveIDSet, not liveIDSet: report a strand bound to a
		// dead-but-present pane as not live — the operator asks status whether
		// the strand's process is running, not whether psmux still lists a
		// (dead) pane for it.
		aliveIDs := aliveIDSet(live)
		strands := make([]StrandStatus, len(st.Strands))
		for i, s := range st.Strands {
			strands[i] = StrandStatus{GUID: s.GUID, Name: s.Name, PaneID: s.PaneID, Live: aliveIDs[s.PaneID]}
		}

		result = StatusResult{Session: session, Socket: e.Socket(), Strands: strands}
		return nil
	})
	return result, err
}
