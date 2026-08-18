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

// sessionNameListed reports whether sessionName appears as a whole line in a
// `list-sessions -F '#{session_name}'` answer.
// The comparison is whole-line and exact, matching every other session target this package builds
// (see exactSessionTarget): a prefix or substring match would let a sibling worktree's session stand
// in for the recorded one.
func sessionNameListed(listSessionsOutput, sessionName string) bool {
	for _, line := range strings.Split(listSessionsOutput, "\n") {
		if strings.TrimSpace(line) == sessionName {
			return true
		}
	}
	return false
}

// paneGenerationLocked reads sessionName's current generation off this engine's socket.
//
// It does NOT error merely because the session is absent, contrary to what a reader might expect
// from every other tmux subcommand this package spends: measured on tmux 3.6, display-message exits
// 0 for a `-t` target that resolves to no session and expands the session-scoped fields to empty,
// while the server-global #{pid} still fills — `-t '=nosuch:'` answers "|2912080|" with exit 0.
// (has-session and list-panes DO exit 1 for the same target; display-message is the odd one out.)
// The absent case therefore surfaces as parsePaneGeneration's empty-field rejection rather than as a
// tmux error, which is why that rejection is load-bearing rather than cosmetic.
//
// A returned error consequently means the probe could not be RUN or could not be understood — a
// failed exec of the tmux binary, an unreachable socket, a non-exit-1 tmux failure, or an answer
// that did not parse. Callers must not read it as "the session is gone";
// refuseLiveForeignSessionLocked below answers that question with list-sessions instead.
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

// refuseRecordedForeignSessionBeforeBootLocked runs refuseLiveForeignSessionLocked against the
// state file directly, so the two booting verbs (Up, Resume) can refuse a recorded-session collision
// BEFORE spawning rather than after.
//
// It costs one extra small read of reed.json under the already-held op lock, and buys the posture
// every other told-identity refusal in this package already has: refuse before creating substrate,
// never leave a bare session squatting on the SHARED per-hub server as the residue of a refusal
// (the property validateToldTmuxIdentity exists for). loadOrInitStateLocked runs the same check
// again on the state it loads, which is what covers the non-booting verbs; running it twice is
// harmless because it mutates nothing.
//
// A corrupt or absent state file is not this check's business: loadOrInitStateLocked reports the
// corruption with the diagnosis an operator can act on, and an absent file records no session to
// collide with.
func (e *Engine) refuseRecordedForeignSessionBeforeBootLocked() error {
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		return nil
	}
	return e.refuseLiveForeignSessionLocked(st.PaneGeneration)
}

// recordedSessionVerdict is what a state file's recorded session name resolves to on this engine's
// socket right now: the answer both the refusal and Down's abandonment report are built on.
type recordedSessionVerdict int

const (
	// recordedSessionAbsent means the recorded name is this worktree's own, is unset, or names no
	// session reachable on this socket — nothing to collide with, nothing to report.
	recordedSessionAbsent recordedSessionVerdict = iota
	// recordedSessionLive means a session of that name is running AND answered the identity probe
	// as the same incarnation the state was recorded against.
	recordedSessionLive
	// recordedSessionUnidentifiable means a session of that name IS listed on the socket but did
	// not answer the identity probe, so reed cannot tell whether it is the recorded one.
	recordedSessionUnidentifiable
)

// classifyRecordedSessionLocked resolves recorded.SessionName against this socket, returning the
// probe error alongside a recordedSessionUnidentifiable verdict so the caller can quote it.
//
// Existence is answered by list-sessions, never by the generation probe's error: display-message
// exits 0 for an absent session (see paneGenerationLocked), so a probe error means the round trip
// could not be run, and reading that as "gone" is the fail-open R6-F2 closed.
// An errored list-sessions is read as "no reachable server on this socket, so nothing can be running
// under the recorded name" — the same reading Down and sessionlessSocketHolderPersists give it.
func (e *Engine) classifyRecordedSessionLocked(recorded PaneGeneration) (recordedSessionVerdict, error) {
	if recorded.SessionName == "" || recorded.SessionName == e.SessionName() {
		return recordedSessionAbsent, nil
	}

	listed, err := e.tmux.output("list-sessions", "-F", "#{session_name}")
	if err != nil || !sessionNameListed(listed, recorded.SessionName) {
		return recordedSessionAbsent, nil
	}

	orphan, probeErr := e.paneGenerationLocked(recorded.SessionName)
	if probeErr != nil {
		return recordedSessionUnidentifiable, probeErr
	}
	if !orphan.SameIncarnation(recorded) {
		// A legitimate namesake: a new worktree reusing the old name answers with a different
		// incarnation, so it is not this state's orphan.
		return recordedSessionAbsent, nil
	}
	return recordedSessionLive, nil
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
//
// A session reed cannot IDENTIFY is refused too, not waved through — this is the one check in the
// package whose whole purpose is to refuse, so it is also the one place where failing open costs the
// most: guessing "gone" wrong launches a second copy of every strand (R5-F5) or spends another
// worktree's pane ids as targets (R5-F4). adoptPaneGenerationLocked's own fail-open is a different
// and genuinely two-sided trade — clearing a healthy table over a hiccup is worse than the staleness
// it guards — and must not be read as licence for this one.
//
// Failing open here also broke a guarantee stated elsewhere: because the pre-boot and post-boot call
// sites run this same check against the same recorded value, ONE transient probe failure at the
// pre-boot call was enough to split their verdicts, so Up booted and then refused — depositing the
// bare session on the shared per-hub server that refuseRecordedForeignSessionBeforeBootLocked exists
// to prevent (R6 review finding R6-F2, reproduced live on tmux 3.6). See
// classifyRecordedSessionLocked for the authorities the three verdicts rest on.
func (e *Engine) refuseLiveForeignSessionLocked(recorded PaneGeneration) error {
	verdict, probeErr := e.classifyRecordedSessionLocked(recorded)
	switch verdict {
	case recordedSessionAbsent:
		return nil
	case recordedSessionUnidentifiable:
		return fmt.Errorf(
			"this worktree's reed state was recorded against tmux session %q, which IS listed on socket %q but did not answer reed's identity probe (%w) — "+
				"reed will not guess whether it is the session this state was recorded against, because continuing on a wrong guess would launch a second copy of every strand. "+
				"Re-run this command, or tear that session down with \"%s -L %s kill-session -t '=%s'\" if it is not one you need",
			recorded.SessionName, e.Socket(), probeErr, e.TmuxPath(), e.Socket(), recorded.SessionName)
	}

	return fmt.Errorf(
		"this worktree's reed state was recorded against tmux session %q, which is STILL RUNNING on socket %q and is not this worktree's session (%q) — "+
			"either the worktree directory was renamed while its session was up, or a .lyx directory was copied here from another worktree. "+
			"Continuing would launch a second copy of every strand and leave %q running unreachably. "+
			"Tear the old session down with \"%s -L %s kill-session -t '=%s'\" (or attach to it first to rescue its work), then re-run this command",
		recorded.SessionName, e.Socket(), e.SessionName(), recorded.SessionName, e.TmuxPath(), e.Socket(), recorded.SessionName)
}
