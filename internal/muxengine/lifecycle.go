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
func (e *Engine) ensureServerAndSessionLocked() error {
	session := e.SessionName()
	up, err := e.psmux.hasSession(session)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if up {
		return nil
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
		return fmt.Errorf("start psmux: %w", err)
	}

	// Poll rather than assume a fixed delay is always enough, mirroring
	// muxpoccli's up.go boot-wait loop.
	const pollAttempts = 3
	const pollInterval = 200 * time.Millisecond
	time.Sleep(500 * time.Millisecond)
	for i := 0; i < pollAttempts; i++ {
		up, err := e.psmux.hasSession(session)
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if up {
			break
		}
		if i == pollAttempts-1 {
			return fmt.Errorf("psmux session did not start in time")
		}
		time.Sleep(pollInterval)
	}

	// remain-on-exit keeps a pane whose command exits around as
	// pane_dead=1 instead of vanishing (which would also kill the session
	// if it were the last pane) — the mechanism reconcile's dead-pane
	// detection depends on.
	if err := e.psmux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		return fmt.Errorf("set remain-on-exit: %w", err)
	}
	return nil
}

// Up ensures the named server and this worktree's session exist (booting
// them if absent, no-op if already up), then reconciles and re-applies the
// layout from the current strand table. Up never launches or relaunches a
// strand command — bringing strand content back after a server restart is
// Resume's job, not Up's.
func (e *Engine) Up() (UpResult, error) {
	var result UpResult
	err := e.withOpLock(func() error {
		if err := e.ensureServerAndSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
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
		if err := e.ensureServerAndSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
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
		toLaunch := planResumeLaunches(st.Strands, liveIDSet(live))

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

// Down tears the session down: kill-server (ignoring "already down" —
// Down is idempotent, matching muxpoccli's down.go) and delete mux.json
// (ignoring not-exist). It does not reconcile or apply — there is nothing
// left to render once the state file is gone.
func (e *Engine) Down() (DownResult, error) {
	var result DownResult
	err := e.withOpLock(func() error {
		// Ignore the error: the server may already be down, and Down must
		// stay idempotent either way.
		_ = e.psmux.run("kill-server")

		path := filepath.Join(e.layout.DotLyxDir(), muxStateFileName)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete state: %w", err)
		}

		result = DownResult{Session: e.SessionName()}
		return nil
	})
	return result, err
}

// Status reconciles against live panes and reports this session's tracked
// strands (guid, name, pane id, live/dead). Unlike the mutating ops, it
// deliberately stops after reconcile: reconcile's dead-pane-kill/binding-
// clear is a real state correction, but Status is a read verb, so it does
// NOT run applyLayoutLocked's select-layout/select-pane (which would move
// input focus and rewrite the window layout as a side effect of a query) or
// re-persist the reconciled table (the next mutating op re-derives and
// persists the same correction; nothing is lost by leaving mux.json as-is
// between queries). It returns a non-nil error when the server/session is
// absent, so a pre-flight caller (e.g. attach) can surface that on its
// envelope before attempting anything that needs a live session. Status
// only reports this session — active stray-server enumeration across the
// hub is deferred (NOTE3).
func (e *Engine) Status() (StatusResult, error) {
	var result StatusResult
	err := e.withOpLock(func() error {
		session := e.SessionName()
		up, err := e.psmux.hasSession(session)
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if !up {
			return fmt.Errorf(`no mux session; run "lyx mux up"`)
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		live, err := e.psmux.listPanes(session)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		if _, err := e.reconcileLocked(st, live); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}

		liveIDs := liveIDSet(live)
		strands := make([]StrandStatus, len(st.Strands))
		for i, s := range st.Strands {
			strands[i] = StrandStatus{GUID: s.GUID, Name: s.Name, PaneID: s.PaneID, Live: liveIDs[s.PaneID]}
		}

		result = StatusResult{Session: session, Socket: e.Socket(), Strands: strands}
		return nil
	})
	return result, err
}
