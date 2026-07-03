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
// all stale) from a normal no-op bring-up.
func (e *Engine) ensureServerAndSessionLocked() (booted bool, err error) {
	session := e.SessionName()
	up, err := e.psmux.hasSession(session)
	if err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	if up {
		return false, nil
	}

	// Env hygiene: a spawned server must never inherit this process's own
	// Claude Code session identity (CleanClaudeEnv is the single documented
	// chokepoint for that decision).
	clean, _ := CleanClaudeEnv(os.Environ())
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
		return false, fmt.Errorf("start psmux: %w", err)
	}

	// Wait for the freshly spawned server to accept commands by polling
	// against a generous deadline, not a fixed handful of attempts: a loaded
	// machine (e.g. an orchestrator running many psmux servers and agent
	// TUIs at once) can take noticeably longer to bring a server online than
	// a quiet one, and a too-short window would fail the whole op with a
	// spurious "did not start" error. hasSession returns (false, nil) while
	// the server is still coming up (exit 1 = no server yet), so the loop
	// simply retries until it reports up or the deadline passes.
	const bootTimeout = 5 * time.Second
	const bootPoll = 100 * time.Millisecond
	deadline := time.Now().Add(bootTimeout)
	for {
		up, err := e.psmux.hasSession(session)
		if err != nil {
			return false, fmt.Errorf("check session: %w", err)
		}
		if up {
			break
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("psmux session did not start within %s", bootTimeout)
		}
		time.Sleep(bootPoll)
	}

	// remain-on-exit keeps a pane whose command exits around as
	// pane_dead=1 instead of vanishing (which would also kill the session
	// if it were the last pane) — the mechanism reconcile's dead-pane
	// detection depends on.
	if err := e.psmux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		return false, fmt.Errorf("set remain-on-exit: %w", err)
	}
	return true, nil
}

// Up ensures the named server and this worktree's session exist (booting
// them if absent, no-op if already up), then reconciles and re-applies the
// layout from the current strand table. Up never launches or relaunches a
// strand command — bringing strand content back after a server restart is
// Resume's job, not Up's.
func (e *Engine) Up() (UpResult, error) {
	var result UpResult
	err := e.withOpLock(func() error {
		booted, err := e.ensureServerAndSessionLocked()
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
		if booted {
			clearAllPaneBindings(st)
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
		booted, err := e.ensureServerAndSessionLocked()
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
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result = ResumeResult{Session: e.SessionName(), Resumed: len(toLaunch)}
		return nil
	})
	return result, err
}

// Down tears this worktree's session down: kill-session (never kill-server —
// the per-hub server is shared with sibling worktrees, and killing it would
// destroy their live sessions too) and delete mux.json (ignoring not-exist).
// Errors from kill-session are ignored so Down stays idempotent against an
// already-stopped session. When this was the last session on the server, the
// now-empty server is cleaned up so no stray process lingers. It does not
// reconcile or apply — there is nothing left to render once the state file is
// gone.
func (e *Engine) Down() (DownResult, error) {
	var result DownResult
	err := e.withOpLock(func() error {
		// Ignore the error: the session may already be gone, and Down must
		// stay idempotent either way.
		_ = e.psmux.run("kill-session", "-t", e.SessionName())

		// Tidy the server only if no sessions remain. A missing server makes
		// list-sessions error, which we treat as "already gone" and skip.
		if out, err := e.psmux.output("list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(out) == "" {
			_ = e.psmux.run("kill-server")
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
