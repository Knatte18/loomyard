// ops.go — the muxpoc subcommand operations.
//
// One in-place column (no worktrees). `up` creates or cold-recovers it; `review` stacks a
// reviewer pane below; `daemon` is the watchdog that re-creates + `--resume`s everything
// after the psmux server dies — the hard part this PoC exists to prove.
package muxpoc

import (
	"fmt"
	"io"
	"time"
)

const sessionName = "muxpoc"

// opUp ensures the column is running. Fresh start when there is no state; cold recovery
// (rebuild + resume) when state exists but the server is down; no-op when already up.
func opUp(cfg Config, cwd string, w, h int) (map[string]any, error) {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	st, have, err := loadState(cwd)
	if err != nil {
		return nil, err
	}

	if have && r.hasSession(st.Session) {
		return map[string]any{
			"action": "noop", "session": st.Session, "socket": r.Socket,
			"panes": len(st.Panes), "stripped_env": strippedEnvKeys(),
		}, nil
	}

	if have {
		// State exists but server/session is gone → cold recovery.
		if err := rebuild(cfg, r, cwd, &st); err != nil {
			return nil, err
		}
		if err := saveState(cwd, st); err != nil {
			return nil, err
		}
		return map[string]any{
			"action": "recovered", "session": st.Session, "socket": r.Socket,
			"panes": len(st.Panes), "stripped_env": strippedEnvKeys(),
		}, nil
	}

	// Fresh start.
	sid, err := newUUID()
	if err != nil {
		return nil, err
	}
	st = State{Socket: r.Socket, Session: sessionName, Width: w, Height: h,
		Panes: []PaneState{{Role: "main", SessionID: sid, CWD: cwd}}}
	if err := r.newSession(cfg, st.Session, cwd, w, h); err != nil {
		return nil, err
	}
	if err := r.setRemainOnExit(); err != nil {
		return nil, err
	}
	panes, err := r.listPanes(st.Session)
	if err != nil || len(panes) == 0 {
		return nil, fmt.Errorf("could not read main pane: %v", err)
	}
	st.Panes[0].PaneID = panes[0].ID
	if err := r.sendLine(st.Panes[0].PaneID, cfg.launchCmd(sid)); err != nil {
		return nil, err
	}
	if err := saveState(cwd, st); err != nil {
		return nil, err
	}
	return map[string]any{
		"action": "started", "session": st.Session, "socket": r.Socket,
		"session_id": sid, "pane": st.Panes[0].PaneID, "stripped_env": strippedEnvKeys(),
	}, nil
}

// rebuild re-creates the server, the column, and resumes each pane's claude from state.
// Used by cold start (opUp) and hot recovery (daemon). Pane ids are re-derived.
func rebuild(cfg Config, r *Runner, cwd string, st *State) error {
	if len(st.Panes) == 0 {
		return fmt.Errorf("no panes in state")
	}
	if err := r.newSession(cfg, st.Session, cwd, st.Width, st.Height); err != nil {
		return err
	}
	if err := r.setRemainOnExit(); err != nil {
		return err
	}
	panes, err := r.listPanes(st.Session)
	if err != nil || len(panes) == 0 {
		return fmt.Errorf("could not read main pane after rebuild: %v", err)
	}
	// Main pane (index 0) is the new-session pane.
	st.Panes[0].PaneID = panes[0].ID
	if err := r.sendLine(st.Panes[0].PaneID, cfg.resumeCmd(st.Panes[0].SessionID)); err != nil {
		return err
	}
	// Stacked reviewers: split below the previous pane, resume each.
	last := st.Panes[0].PaneID
	for i := 1; i < len(st.Panes); i++ {
		id, err := r.splitV(cfg, last, st.Panes[i].CWD)
		if err != nil {
			return err
		}
		st.Panes[i].PaneID = id
		if err := r.sendLine(id, cfg.resumeCmd(st.Panes[i].SessionID)); err != nil {
			return err
		}
		last = id
	}
	return nil
}

// opReview stacks a reviewer pane below the column and launches a fresh claude in it.
func opReview(cfg Config, cwd string) (map[string]any, error) {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	st, have, err := loadState(cwd)
	if err != nil {
		return nil, err
	}
	if !have || !r.hasSession(st.Session) {
		return nil, fmt.Errorf("no running muxpoc session; run `mhgo muxpoc up` first")
	}
	last := st.Panes[len(st.Panes)-1].PaneID
	id, err := r.splitV(cfg, last, cwd)
	if err != nil {
		return nil, err
	}
	sid, err := newUUID()
	if err != nil {
		return nil, err
	}
	if err := r.sendLine(id, cfg.launchCmd(sid)); err != nil {
		return nil, err
	}
	st.Panes = append(st.Panes, PaneState{Role: "reviewer", SessionID: sid, CWD: cwd, PaneID: id})
	if err := saveState(cwd, st); err != nil {
		return nil, err
	}
	return map[string]any{"action": "reviewer-added", "pane": id, "session_id": sid,
		"panes": len(st.Panes)}, nil
}

// opStatus reports the live panes plus the env that would be / was stripped.
func opStatus(cfg Config, cwd string) (map[string]any, error) {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	st, have, err := loadState(cwd)
	if err != nil {
		return nil, err
	}
	res := map[string]any{"have_state": have, "socket": r.Socket,
		"server_up": r.hasServer(), "stripped_env": strippedEnvKeys()}
	if have {
		res["session"] = st.Session
		res["state_panes"] = st.Panes
		if live, err := r.listPanes(st.Session); err == nil {
			res["live_panes"] = live
		}
	}
	return res, nil
}

// opDown tears down the server and clears state.
func opDown(cfg Config, cwd string) (map[string]any, error) {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	_ = r.killServer()
	if err := clearState(cwd); err != nil {
		return nil, err
	}
	return map[string]any{"action": "down", "socket": r.Socket}, nil
}

// opDaemon is the foreground watchdog: it ensures the column is up, then polls the psmux
// server and rebuilds+resumes on death. It blocks until the process is interrupted. This
// is the PoC's reason to exist — proving crash-survival for one column. Log lines (not
// JSON) go to logw so a human can watch recoveries.
func opDaemon(cfg Config, cwd string, interval time.Duration, logw io.Writer) error {
	r := &Runner{Bin: cfg.Psmux, Socket: socketFor(cwd)}
	logf := func(f string, a ...any) { fmt.Fprintf(logw, "[muxpoc-daemon] "+f+"\n", a...) }

	if _, err := opUp(cfg, cwd, 200, 50); err != nil {
		return err
	}
	logf("watching socket=%s session=%s every %s (stripped env: %v)",
		r.Socket, sessionName, interval, strippedEnvKeys())

	for {
		time.Sleep(interval)
		st, have, err := loadState(cwd)
		if err != nil || !have {
			logf("no state; stopping")
			return err
		}
		if r.hasSession(st.Session) {
			continue
		}
		logf("psmux server/session gone — recovering (%d pane(s))", len(st.Panes))
		if err := rebuild(cfg, r, cwd, &st); err != nil {
			logf("recovery FAILED: %v", err)
			continue
		}
		if err := saveState(cwd, st); err != nil {
			logf("state save after recovery failed: %v", err)
		}
		logf("recovered: claude --resume per pane (main session %.8s…)", st.Panes[0].SessionID)
	}
}
