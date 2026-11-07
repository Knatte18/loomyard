// generation.go reconciles reed.json's pane bindings with the tmux session they were minted against.
// It holds the generation stamp, its probe, and the load-time guards that clear or refuse on a mismatch.
// Pane ids restart at %0 on every tmux server rebirth, so a recorded pane id is meaningful only within one generation.

package reedengine

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/logger"
)

// paneGenerationFormat is the tmux display-message format the generation probe reads.
// paneGenerationFieldSeparator joins its fields.
// The three fields are jointly unique across everything that can make a recorded pane id mean something different:
// session_id tells two sessions on one server apart.
// pid tells two servers on one socket apart.
// session_created tells apart a session id reused after a server rebirth.
// None of them changes over a session's life, so a healthy worktree's stamp is stable,
// and this probe never triggers a spurious clear.
const (
	paneGenerationFormat         = "#{session_id}|#{pid}|#{session_created}"
	paneGenerationFieldSeparator = "|"
	paneGenerationExpectedFields = 3
	paneGenerationSessionIDField = 0
	paneGenerationServerPIDField = 1
	paneGenerationCreatedField   = 2
)

// parsePaneGeneration turns one display-message answer into a PaneGeneration for sessionName.
// A short or empty answer is an error rather than a partially filled stamp: a stamp missing a field
// would compare unequal to every later probe and clear this worktree's bindings on every op.
//
// The empty-field rejection is also how an absent session is detected.
// display-message exits 0 for a -t target that names no session, and it expands the session-scoped fields to empty.
// So "|<serverpid>|" is tmux's answer for a session that is gone, not an error.
// Relaxing this check would admit a partial stamp, and the refusal against a still-running orphan would never fire.
// TestDisplayMessageDoesNotErrorForAnAbsentSession in contract_integration_test.go pins both halves.
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

// sessionNameListed reports whether sessionName is a whole line of a `list-sessions -F '#{session_name}'` answer.
// The match is whole-line and exact.
// A prefix or substring match would let a sibling worktree's session stand in for the recorded one.
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
// An absent session is not an error for this probe.
// On tmux 3.6, display-message exits 0 for a -t target that resolves to no session,
// and it expands the session-scoped fields to empty while the server-global #{pid} still fills.
// For example, -t '=nosuch:' answers "|2912080|" with exit 0.
// So the absent case surfaces as parsePaneGeneration's empty-field rejection, not as a tmux error.
//
// A returned error means the probe could not be run or its answer could not be parsed.
// This covers a failed exec of the tmux binary, an unreachable socket, and a tmux failure other than exit 1.
// Callers must not read it as "the session is gone".
// To decide that, ask list-sessions instead.
//
// The target stays the session's current-window form, "=<session>:", outside the strand-window seam.
// The format reads session fields only, so whichever window is current gives the same answer.
// A bare "=<session>" target expands every session field to empty on tmux 3.6.
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
// Detecting a rebirth from the boot itself covers only a server this invocation spawned.
// Stale tables arrive between boots.
// Examples are a restored backup, a resurrected untracked copy, and an operator's hand-copy.
// A reed.json older than the session now running is stale too.
// Their bindings name panes that exist but belong to something else.
// A presence check cannot tell that apart from a healthy binding.
// It can only ask whether the pane is present in list-panes.
// A stale binding can therefore report a strand live against a shell that is not running it.
// Resume then refuses to rebuild that strand.
//
// A probe failure never clears.
// Clearing on "could not tell" would discard a healthy worktree's whole binding table over a transient tmux error.
// That loss is strictly worse than the staleness this guards against.
// The failure path leaves st as loaded and logs.
// That keeps the fail-open floor: the bindings stay as the file had them.
//
// A state file with no stamp is adopted rather than cleared.
// Bindings without a stamp can only come from a binary that predates stamps.
// That is an upgrade over a session already live, so those bindings are valid.
// Clearing them would make the first op after the upgrade tear down and relaunch every running strand.
// The internal-consistency repair does not depend on the stamp, so such a file still gets it.
func (e *Engine) adoptPaneGenerationLocked(st *ReedState) error {
	live, err := e.paneGenerationLocked(e.SessionName())
	if err != nil {
		e.calleeFailures.Fail("pane generation", "reed: could not read the live pane generation, leaving persisted pane bindings as they are",
			"socket", e.Socket(), "session", e.SessionName(), "err", err)
		return nil
	}
	e.calleeFailures.Recover("pane generation", "reed: live pane generation reads again", "socket", e.Socket(), "session", e.SessionName())

	recorded := st.PaneGeneration
	if !recorded.Recorded() || recorded.SameIncarnation(live) {
		// The match ignores the session name.
		// So an in-place tmux rename-session refreshes the stamp rather than reading as a different generation.
		e.stateLoadNotices.forgetGenerationClear()
		st.PaneGeneration = live
		return nil
	}

	if err := e.refuseLiveForeignSessionLocked(recorded); err != nil {
		return err
	}

	// The clear is made in memory only, so every later load of the same file clears it again;
	// only the first load of a recorded and live pair warns.
	logClear := logger.Debug
	if e.stateLoadNotices.generationClearIsNew(recorded, live) {
		logClear = logger.Warn
	}
	logClear("reed: persisted pane bindings were minted against a different tmux session incarnation, clearing them",
		"socket", e.Socket(), "session", e.SessionName(),
		"recordedSession", recorded.SessionName, "recordedTmuxSession", recorded.TmuxSessionID, "recordedServerPID", recorded.ServerPID,
		"liveTmuxSession", live.TmuxSessionID, "liveServerPID", live.ServerPID)
	clearAllPaneBindings(st)
	clearSelvagePaneBinding(st)
	st.PaneGeneration = live
	return nil
}

// refuseRecordedForeignSessionBeforeBootLocked returns an error for a recorded-session collision.
// A booting verb calls it before spawning, so the refusal happens before any substrate is created.
// Refusing after the spawn would leave a bare session squatting on the shared per-hub server.
// It reads reed.json directly under the op lock the caller already holds, which costs one small extra read.
// The same check also runs on every state load, which covers the verbs that do not boot.
// Running it twice is harmless because it mutates nothing.
//
// A corrupt or absent state file is not this check's concern.
// A corrupt file is reported by the state load with a diagnosis an operator can act on.
// An absent file records no session to collide with.
func (e *Engine) refuseRecordedForeignSessionBeforeBootLocked() error {
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		return nil
	}
	return e.refuseLiveForeignSessionLocked(st.PaneGeneration)
}

// recordedSessionVerdict is what a state file's recorded session name resolves to on this engine's
// socket right now.
// The refusal and the abandonment report both decide from it.
type recordedSessionVerdict int

const (
	// recordedSessionAbsent means the recorded name is unset or is this worktree's own;
	// no session reachable on this socket has that name, so nothing can collide with it.
	recordedSessionAbsent recordedSessionVerdict = iota
	// recordedSessionLive means a session of that name is running;
	// it answered the identity probe as the same incarnation the state was recorded against.
	recordedSessionLive
	// recordedSessionUnidentifiable means a session of that name is listed on the socket;
	// it did not answer the identity probe, so reed cannot tell whether it is the recorded one.
	recordedSessionUnidentifiable
)

// classifyRecordedSessionLocked resolves recorded.SessionName against this socket into a verdict.
// It returns the probe error alongside the verdict when the probe fails, so the caller can quote it.
//
// Existence is answered by list-sessions, never by the generation probe's error.
// display-message exits 0 for an absent session, so a probe error means the round trip could not run.
// Reading that error as gone would fail open.
// An errored list-sessions is read as no reachable server on this socket, so nothing runs under the recorded name.
func (e *Engine) classifyRecordedSessionLocked(recorded PaneGeneration) (recordedSessionVerdict, error) {
	if recorded.SessionName == "" || recorded.SessionName == e.SessionName() {
		return recordedSessionAbsent, nil
	}

	listed, err := e.tmux.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...)
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

// refuseLiveForeignSessionLocked refuses the operation when the recorded session is still running on this socket;
// it is not this worktree's own, or reed cannot identify it.
//
// Two ordinary routes lead here, and both are silently destructive without the guard:
//
//   - The worktree was renamed: its .lym state moves with it, but SessionName follows the new basename;
//     so `resume` boots a second session and relaunches every strand into it.
//   - A .lym directory was copied from another worktree of the same hub.
//     The socket is per hub and pane ids are server-global;
//     so the copied ids address the source worktree's live panes.
//
// The identity comparison keeps this from firing on a legitimate namesake.
// A new worktree that reuses the old name answers the probe with a different incarnation,
// so the caller falls through to the ordinary clear.
//
// Refuse rather than adopt or kill: its panes may hold live agent work.
// Destroying it unasked is not reed's call.
// The message names the session, the socket, and the exact command that clears it.
//
// A session that reed cannot identify is refused too, not waved through.
// Guessing that it is gone would launch a second copy of every strand or target another worktree's panes.
//
// The pre-boot and post-boot call sites run this check on the same recorded value;
// a transient probe failure must therefore not give them different verdicts,
// or a boot can leave a bare session on the shared per-hub server.
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
			"either the worktree directory was renamed while its session was up, or a .lym directory was copied here from another worktree. "+
			"Continuing would launch a second copy of every strand and leave %q running unreachably. "+
			"Tear the old session down with \"%s -L %s kill-session -t '=%s'\" (or attach to it first to rescue its work), then re-run this command",
		recorded.SessionName, e.Socket(), e.SessionName(), recorded.SessionName, e.TmuxPath(), e.Socket(), recorded.SessionName)
}

// stateLoadNotices remembers the last pane-binding clear a state load logged at Warn.
// A state load clears in memory and never saves;
// so the next load of the same file clears again and would log the same line.
// The first clear of a pair or set warns, a repeat of it logs at Debug,
// and a load that finds nothing to clear forgets the record.
// Both the watch and name-repair goroutines load state, so a mutex guards it.
type stateLoadNotices struct {
	mu sync.Mutex

	generationLogged bool
	generationPair   [2]PaneGeneration

	conflictLogged bool
	conflictSet    string
}

// generationClearIsNew reports whether the clear of this recorded and live pair differs from the last one logged.
// It records this pair as the last one logged.
func (n *stateLoadNotices) generationClearIsNew(recorded, live PaneGeneration) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	pair := [2]PaneGeneration{recorded, live}
	isNew := !n.generationLogged || n.generationPair != pair
	n.generationLogged = true
	n.generationPair = pair
	return isNew
}

// forgetGenerationClear drops the recorded generation pair, so a later mismatch warns again.
func (n *stateLoadNotices) forgetGenerationClear() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.generationLogged = false
}

// conflictClearIsNew reports whether the cleared strand GUIDs differ from the last set logged.
// It records this set as the last one logged.
func (n *stateLoadNotices) conflictClearIsNew(cleared []string) bool {
	set := strings.Join(slices.Sorted(slices.Values(cleared)), ",")
	n.mu.Lock()
	defer n.mu.Unlock()
	isNew := !n.conflictLogged || n.conflictSet != set
	n.conflictLogged = true
	n.conflictSet = set
	return isNew
}

// forgetConflictClear drops the recorded cleared set, so a later conflict warns again.
func (n *stateLoadNotices) forgetConflictClear() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.conflictLogged = false
}
