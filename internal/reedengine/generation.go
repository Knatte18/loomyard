// generation.go implements the pane-generation stamp: the identity of the tmux session incarnation
// that a persisted reed.json's PaneIDs and HeaderPaneID were bound against, the probe that reads
// that identity off a live session, and the two load-time guards built on it — clearing bindings
// minted against a session that is no longer the one running, and refusing to operate when this
// worktree's recorded session is still alive on the shared socket under a different name.
//
// It exists because a tmux pane id is meaningless on its own. Pane ids are server-global and restart
// at %0 on every server rebirth, so a persisted %0 does not identify a pane — it identifies a pane
// WITHIN one server generation, and reed had no record of which generation that was.

package reedengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// paneGenerationFormat is the tmux display-message format the generation probe spends, and
// paneGenerationFieldSeparator joins its fields.
// The three fields are chosen to be jointly unique across everything that can make a recorded pane
// id mean something different: session_id distinguishes two sessions on one server, pid
// distinguishes two servers on one socket, and session_created distinguishes a session id reused
// after a server rebirth. None of them changes over a session's life, so a healthy worktree's stamp
// is stable and this probe never triggers a spurious clear.
const (
	paneGenerationFormat         = "#{session_id}|#{pid}|#{session_created}"
	paneGenerationFieldSeparator = "|"
	paneGenerationExpectedFields = 3
	paneGenerationSessionIDField = 0
	paneGenerationServerPIDField = 1
	paneGenerationCreatedField   = 2
)

// parsePaneGeneration turns one display-message answer into a PaneGeneration for sessionName.
// A short or empty answer is an error rather than a partially-filled stamp: a stamp missing a field
// would compare unequal to every future probe and clear this worktree's bindings on every op.
func parsePaneGeneration(sessionName, out string) (PaneGeneration, error) {
	fields := strings.Split(strings.TrimSpace(out), paneGenerationFieldSeparator)
	if len(fields) != paneGenerationExpectedFields {
		return PaneGeneration{}, fmt.Errorf("tmux answered %q for session %q; want %d %q-separated fields", out, sessionName, paneGenerationExpectedFields, paneGenerationFieldSeparator)
	}
	for _, field := range fields {
		if strings.TrimSpace(field) == "" {
			return PaneGeneration{}, fmt.Errorf("tmux answered %q for session %q; one of the %d fields is empty", out, sessionName, paneGenerationExpectedFields)
		}
	}
	return PaneGeneration{
		SessionName:   sessionName,
		TmuxSessionID: fields[paneGenerationSessionIDField],
		ServerPID:     fields[paneGenerationServerPIDField],
		Created:       fields[paneGenerationCreatedField],
	}, nil
}

// paneGenerationLocked reads sessionName's current generation off this engine's socket.
// It errors when the session does not exist, which is how the orphan check below distinguishes
// "the session this state was recorded against is gone" from "it is still running".
func (e *Engine) paneGenerationLocked(sessionName string) (PaneGeneration, error) {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(sessionName), paneGenerationFormat)
	if err != nil {
		return PaneGeneration{}, fmt.Errorf("read pane generation for session %q: %w", sessionName, err)
	}
	return parsePaneGeneration(sessionName, out)
}

// adoptPaneGenerationLocked reconciles st's recorded pane generation against the session that is
// actually live, clearing every pane binding when they disagree and re-stamping st either way.
//
// This is the general form of the rebirth clear Up/Resume already perform under `booted`. That clear
// fires only when THIS invocation spawned the server, which covers a rebirth and nothing else — and
// the tables that need clearing mostly arrive BETWEEN boots: a restored backup, a resurrected
// untracked copy, an operator hand-copy, or simply a reed.json older than the session now running.
// Their bindings then name panes that exist but belong to something else, and reconcileLocked cannot
// tell that apart from a healthy binding, because "present in list-panes" is the only question it
// can ask. Reproduced live (R5 review finding R5-F2): after a down/up cycle reset pane ids and a
// gen-1 reed.json was restored, `status` reported the strand live:true against the new session's
// bare initial shell and `resume` answered resumed:0, refusing to rebuild a strand whose process was
// not running anywhere.
//
// A probe failure never clears. Clearing on "I could not tell" would discard a healthy worktree's
// whole binding table over a transient tmux hiccup, which is strictly worse than the staleness this
// guards against — so the failure path leaves st exactly as loaded and logs, keeping the pre-fix
// behaviour as the fail-open floor.
//
// A state file carrying NO stamp is adopted rather than cleared, for the same reason: the only way
// to hold bindings without a stamp is to have been written by a binary predating this code, i.e. an
// upgrade with a session already live, where the bindings are in fact valid. Clearing them would
// make the first post-upgrade op tear down and relaunch every running strand. The
// internal-consistency repair (clearConflictingPaneBindings) is deliberately NOT conditioned on the
// stamp and so still covers such a file.
func (e *Engine) adoptPaneGenerationLocked(st *ReedState) error {
	live, err := e.paneGenerationLocked(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not read the live pane generation, leaving persisted pane bindings as they are",
			"socket", e.Socket(), "session", e.SessionName(), "err", err)
		return nil
	}

	recorded := st.PaneGeneration
	if !recorded.Recorded() || recorded.SameIncarnation(live) {
		// SameIncarnation deliberately ignores SessionName, so an in-place tmux rename-session
		// refreshes the stamp here instead of being mistaken for a different generation.
		st.PaneGeneration = live
		return nil
	}

	if err := e.refuseLiveForeignSessionLocked(recorded); err != nil {
		return err
	}

	logger.Warn("reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them",
		"socket", e.Socket(), "session", e.SessionName(),
		"recordedSession", recorded.SessionName, "recordedTmuxSession", recorded.TmuxSessionID, "recordedServerPID", recorded.ServerPID,
		"liveTmuxSession", live.TmuxSessionID, "liveServerPID", live.ServerPID)
	clearAllPaneBindings(st)
	st.HeaderPaneID = ""
	st.PaneGeneration = live
	return nil
}

// refuseLiveForeignSessionLocked refuses the operation when recorded names a session that is STILL
// RUNNING on this socket and is not this worktree's own — the one disagreement that must not be
// resolved by silently carrying on.
//
// Two ordinary routes reach it, and both are silently destructive without this guard:
//
//   - The worktree directory was renamed (R5 review finding R5-F5). SessionName derives from the
//     worktree basename while .lyx travels with the directory, so `resume` boots a SECOND session
//     and relaunches every strand into it — reproduced live: after `mv svc-orig svc-moved`,
//     `lyx reed resume` reported ok:true/resumed:1 and the strand's process was running twice, and
//     the orphaned svc-orig session survived a `lyx reed down` because no worktree of that name
//     exists for any engine to derive it from again.
//   - A .lyx directory was hand-copied between worktrees of one hub (R5 review finding R5-F4).
//     The socket is per hub and pane ids are server-global, so the copied ids address the SOURCE
//     worktree's live panes — reproduced live: a `lyx reed remove` in svc-alpha killed svc-beta's
//     strand pane and its process, reporting ok:true.
//
// The identity comparison is what keeps this from firing on a legitimate namesake: a new worktree
// that merely reuses the old name answers the probe with a different incarnation, so its session is
// not mistaken for this state's orphan and the caller falls through to the ordinary clear.
//
// Refuse rather than adopt or kill: the recorded session's panes may hold live agent work, and
// destroying it unasked is not reed's call. The message therefore names the session, the socket, and
// the exact command that clears it.
func (e *Engine) refuseLiveForeignSessionLocked(recorded PaneGeneration) error {
	if recorded.SessionName == "" || recorded.SessionName == e.SessionName() {
		return nil
	}

	orphan, err := e.paneGenerationLocked(recorded.SessionName)
	if err != nil {
		// The recorded session is gone, so there is nothing to collide with; the caller clears the
		// stale bindings and carries on.
		return nil
	}
	if !orphan.SameIncarnation(recorded) {
		return nil
	}

	return fmt.Errorf(
		"this worktree's reed state was recorded against tmux session %q, which is STILL RUNNING on socket %q and is not this worktree's session (%q) — "+
			"either the worktree directory was renamed while its session was up, or a .lyx directory was copied here from another worktree. "+
			"Continuing would launch a second copy of every strand and leave %q running unreachably. "+
			"Tear the old session down with \"%s -L %s kill-session -t '=%s'\" (or attach to it first to rescue its work), then re-run this command",
		recorded.SessionName, e.Socket(), e.SessionName(), recorded.SessionName, e.TmuxPath(), e.Socket(), recorded.SessionName)
}
