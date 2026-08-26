// lifecycle.go implements the four lifecycle engine ops — Up, Resume, Down, Status — plus the pure
// planning helpers that make their decisions unit-testable without a live tmux server.
// The sharp boundary the batch discussion settles on: Up ensures the substrate (server + session)
// exists and never launches a strand command;
// Resume is the only replayer,
// and it skips anchor:hidden strands (pending first surface, not dead).

package reedengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// stateDir returns the path to the worktree-level ephemeral tree holding reed.json and reed.lock.
// It is AnchorPath-anchored so it is a directory sibling of the durable, fabric-synced _lyx tree —
// distinct from Geometry.LogsDir, the hub-anchored directory this engine is TOLD (built by
// fabricengine.HubLogsDir, never derived here), which stays one deterministic place per hub rather
// than per worktree.
func (e *Engine) stateDir() string {
	return filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName)
}

// UpResult reports the outcome of Up.
type UpResult struct {
	Session string
	Socket  string
	Strands int
}

// ResumeResult reports the outcome of Resume.
type ResumeResult struct {
	Session string
	Resumed int
}

// DownResult reports the outcome of Down: the session name that was torn down, plus the name of a
// live session Down knowingly walked away from.
type DownResult struct {
	Session string
	// AbandonedSession names a tmux session this worktree's state file was recorded against, which
	// is STILL RUNNING on the shared per-hub socket under a name that is not this worktree's, and
	// which Down did not kill and could not track any further because it deleted the state file
	// naming it. Empty in every ordinary teardown. See Down for why it is reported rather than
	// killed.
	AbandonedSession string
}

// StrandStatus is one strand's status in StatusResult.
type StrandStatus struct {
	GUID   string
	Name   string
	PaneID string
	Live   bool
}

// StatusResult reports this session's tracked strands and their live/dead state.
type StatusResult struct {
	Session string
	Socket  string
	Strands []StrandStatus
}

// Boot-loop tuning for ensureServerAndSessionLocked. maxBootAttempts caps
// attempt count to guard against fast-failure spirals (observed: 30-90+ spawns
// within a single bootOverallTimeout when fork fails near-instantly under
// resource pressure). staleSocketGrace is the grace window before a
// session-less socket is treated as stale (vs. a sibling worktree's fresh boot).
const (
	bootAttemptTimeout = 20 * time.Second
	bootPoll           = 100 * time.Millisecond
	bootOverallTimeout = 90 * time.Second
	maxBootAttempts    = 8
	staleSocketGrace   = 5 * time.Second
)

// serverLogPruneKeep keeps 2 pre-existing logs; newest 3 total (2 + fresh boot).
const serverLogPruneKeep = 2

// Log filename shapes from -v/-vv: server, client, and -vv-only out logs.
// At -vv, server additionally writes tmux-out-<pid>.log; all three are pruned.
const (
	serverLogNamePrefix = "tmux-server-"
	clientLogNamePrefix = "tmux-client-"
	outLogNamePrefix    = "tmux-out-"
	serverLogNameSuffix = ".log"
)

// pruneServerLogsLocked prunes prefix+*+suffix files to keep newest by mtime.
// It ignores remove errors when files vanish between scan and deletion
// (sibling worktree may have already pruned them).
func pruneServerLogsLocked(logsDir, prefix string, keep int) error {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", logsDir, err)
	}

	var names []string
	var mtimes []time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, serverLogNameSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			// The file vanished between ReadDir and Info (e.g. a concurrent
			// prune by a sibling worktree's boot); it is already gone, which
			// is exactly what pruning wants, so skip it rather than error.
			continue
		}
		names = append(names, name)
		mtimes = append(mtimes, info.ModTime())
	}

	for _, name := range planLogPrune(names, mtimes, keep) {
		if err := os.Remove(filepath.Join(logsDir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale server log %s: %w", name, err)
		}
	}
	return nil
}

// planUpLaunches always returns nil: Up never launches strands.
func planUpLaunches(strands []Strand) []Strand {
	return nil
}

// planResumeLaunches returns non-live, non-hidden strands for Resume to relaunch.
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

// ensureServerAndSessionLocked ensures this hub's tmux server and this
// worktree's session exist. Reports booted=true on fresh spawn; validates
// capability, debug_log, mouse, and header template before any tmux round trip.
func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []string, err error) {
	// Validate debug_log before anything else touches tmux: a misconfigured
	// value is a pure config error, unrelated to server/session state, so it
	// must surface before the capability probe or any spawn attempt.
	debugArgs, err := debugLogArgs(e.cfg.DebugLog)
	if err != nil {
		return false, nil, err
	}

	// Validate mouse alongside debug_log, at the same early point: this too
	// is a pure config error that must surface before the capability probe
	// or any spawn attempt, not partway through a boot.
	mouse, err := mouseOption(e.cfg.Mouse)
	if err != nil {
		return false, nil, err
	}

	// Validate the header template in the same pre-tmux block — it reads
	// only cfg+geometry (HeaderText makes no tmux round trip), so like
	// debug_log and mouse it must fail the boot before anything is spawned.
	// An earlier version validated only AFTER the session existed, which
	// left a half-created session behind on a bad template — and, on the
	// crash-recovery path, lost the booted=true rebirth signal: the boot
	// had already replaced the session (pane ids reset) when validation
	// failed, so the NEXT resume saw the session simply "up", skipped
	// clearAllPaneBindings, and mistook stale pre-crash pane bindings for
	// live strands (observed live: resumed:0 with a bare shell reported
	// live). Validating up front removes the realistic — config-mistake —
	// path into that trap; a set-option failure between spawn and return
	// can still theoretically lose the signal, but has no config-shaped
	// trigger.
	if err := e.ValidateHeader(); err != nil {
		return false, nil, err
	}

	// Fail loud, once per ensure/boot, if the configured multiplexer binary
	// is below the pinned version floor or missing a required subcommand —
	// far better than letting an unknown surface surface later as a cryptic
	// tmux error deep inside the boot loop below.
	if err := e.probeCapabilityLocked(); err != nil {
		return false, nil, err
	}

	// Refuse a recorded-session collision here, ahead of the spawn below, rather than leaving it to
	// loadOrInitStateLocked after the caller has already booted: a refusal must not deposit a bare
	// session on the shared per-hub server as its residue. This is the first check needing a tmux
	// round trip, so it cannot join the pre-tmux block above — but it still precedes everything that
	// CREATES anything. See refuseRecordedForeignSessionBeforeBootLocked.
	if err := e.refuseRecordedForeignSessionBeforeBootLocked(); err != nil {
		return false, nil, err
	}

	session := e.SessionName()
	up, err := e.tmux.hasSession(session)
	if err != nil {
		return false, nil, fmt.Errorf("check session: %w", err)
	}
	if up {
		// A session that exists but holds ZERO panes is broken substrate: it
		// cannot host a strand (there is no pane to adopt or split, and tmux
		// offers no way to add a pane to an empty window), so add would fail
		// forever while up kept reporting success. The state is reachable
		// when an applied layout once destroyed every pane (tmux reaps any
		// pane absent from a select-layout string). Kill the husk and fall
		// through to a fresh boot — the booted=true return then makes the
		// caller clear every stale binding, exactly like a server rebirth.
		live, err := e.tmux.listPanes(session)
		if err != nil {
			return false, nil, fmt.Errorf("list panes: %w", err)
		}
		if len(live) > 0 {
			// The header template was already validated in the pre-tmux
			// block above, so this healthy already-up path returns directly.
			return false, nil, nil
		}
		_ = e.tmux.run("kill-session", "-t", exactSessionTarget(session))
	}

	// A stale socket-holder wedges a fresh boot: on Windows, psmux's internal
	// "__warm__" helper can outlive a kill-server and sit on the -L socket
	// without ever hosting a session, so a new-session spawned against it
	// never materializes. A socket whose holder reports zero sessions across
	// the grace window is such a stale helper, a dying server, or an
	// unreachable zombie — never a healthy shared server (sibling worktrees'
	// sessions would list) — so force-reaping it before spawning is safe.
	if e.sessionlessSocketHolderPersists() {
		if err := e.reapSocketProcesses(); err != nil {
			logger.Warn("reed: failed to reap stale tmux socket-holder", "socket", e.Socket(), "err", err)
			return false, nil, fmt.Errorf("stale tmux socket-holder: %w", err)
		}
	}

	// Route the server's tmux-server-<pid>.log to the hub's deterministic
	// forensic location (Shared Decision hub-logs-dir): tmux writes that log
	// to the server process's own cwd with no redirect flag, so cmd.Dir below
	// is the only lever. This happens on every boot, regardless of
	// debug_log, and runs before the boot loop so a fresh server's log always
	// lands in a directory that already exists and is already pruned.
	logsDir := e.geom.LogsDir
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		logger.Warn("reed: failed to create hub logs dir", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("create %s: %w", logsDir, err)
	}
	// Prune to the newest 2 pre-existing logs before this boot's own log is
	// written, so at most 3 (2 kept + this boot's fresh one) ever exist
	// (Shared Decision log-prune-keep-3) — forensic history without
	// unbounded growth across repeated boots. Both filename shapes a
	// debug-armed boot can leave (server, client — see clientLogNamePrefix)
	// are pruned, independently bounded, so neither accumulates unbounded.
	if err := pruneServerLogsLocked(logsDir, serverLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune server logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune server logs: %w", err)
	}
	if err := pruneServerLogsLocked(logsDir, clientLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune client logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune client logs: %w", err)
	}
	// The -vv-only tmux-out-<pid>.log protocol log is the third shape a
	// debug-armed boot can leave; prune it on the same newest-3 budget so it
	// does not accumulate unbounded across repeated debug_log: 2 boots.
	if err := pruneServerLogsLocked(logsDir, outLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune out logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune out logs: %w", err)
	}

	// Env hygiene: a spawned server must never inherit this process's own
	// Claude Code session identity (CleanClaudeEnv is the single documented
	// chokepoint for that decision).
	clean, stripped := CleanClaudeEnv(os.Environ())
	// The tmux server is a long-lived singleton later invocations reattach
	// to, unlike a one-shot child (board/fabric spawn) that belongs to a
	// single invocation and should keep inheriting the trace ID. Filtering
	// LYX_TRACE_ID is therefore kept as its own explicitly-named step, not
	// folded into CleanClaudeEnv: CleanClaudeEnv's stripped-keys return
	// value is persisted verbatim into ReedState.StrippedEnv as a
	// Claude-injected-variable diagnostic, and LYX_TRACE_ID is neither
	// Claude-injected nor meant to be recorded there.
	clean = stripTraceID(clean)
	spawnSession := func() error {
		// debugArgs are tmux GLOBAL flags (e.g. -v/-vv) and must precede
		// -L/new-session on the argv; -c pins new-session's pane default cwd
		// to Geometry.PaneCwd, the told pane spawn directory, even though
		// the server process's own cwd (cmd.Dir) has moved to logsDir.
		argv := append([]string{}, debugArgs...)
		argv = append(argv,
			"-L", e.Socket(),
			"new-session", "-d", "-s", session,
			"-c", e.geom.PaneCwd,
			"-x", strconv.Itoa(e.cfg.Width),
			"-y", strconv.Itoa(e.cfg.Height),
			e.cfg.Shell,
		)
		cmd := exec.Command(e.cfg.Tmux, argv...)
		cmd.Dir = logsDir
		cmd.Env = clean
		proc.Detach(cmd)
		if err := cmd.Start(); err != nil {
			logger.Warn("reed: failed to start tmux server", "socket", e.Socket(), "session", session, "err", err)
			return fmt.Errorf("start tmux: %w", err)
		}
		logger.Info("reed: spawned tmux server", "socket", e.Socket(), "session", session, "pid", cmd.Process.Pid)
		return nil
	}

	// Boot with a deadline-based retry. Two distinct slow paths hide behind
	// "the session is not answering yet": (a) an honestly slow boot on a
	// loaded machine — a real boot is ~1-2s quiet but has been observed to
	// exceed two full 20s attempt windows when several concurrent test
	// suites peg the CPU, so the loop retries against an overall deadline
	// rather than a fixed attempt count; and (b) a ZOMBIE boot — tmux has a
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
	bootStart := time.Now()
	bootDeadline := bootStart.Add(bootOverallTimeout)
	attempt := 0
	for {
		attempt++
		logger.Info("reed: boot attempt", "socket", e.Socket(), "session", session, "attempt", attempt)
		if err := spawnSession(); err != nil {
			return false, nil, err
		}

		attemptDeadline := time.Now().Add(bootAttemptTimeout)
		sessionUp := false
		for time.Now().Before(attemptDeadline) {
			up, err := e.tmux.hasSession(session)
			if err != nil {
				// hasSession's error path returns immediately (the poll loop
				// never retries an errored check itself — only the outer
				// attempt loop above does), so this Warn fires at most once
				// per boot attempt rather than once per poll: there is no
				// per-iteration spam to guard against here.
				logger.Warn("reed: boot poll has-session check failed", "socket", e.Socket(), "session", session, "attempt", attempt, "err", err)
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

		if out, err := e.tmux.output("list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(out) != "" {
			logger.Warn("reed: tmux server up but session did not materialize", "socket", e.Socket(), "session", session, "attempt", attempt)
			return false, nil, fmt.Errorf("tmux server is up but session %q did not materialize within %s", session, bootAttemptTimeout)
		}
		logger.Warn("reed: zombie boot, reaping socket before retry", "socket", e.Socket(), "session", session, "attempt", attempt)
		if err := e.reapSocketProcesses(); err != nil {
			return false, nil, fmt.Errorf("reap zombie tmux boot: %w", err)
		}
		if attempt >= maxBootAttempts {
			logger.Warn("reed: boot gave up after fast-failure spiral guard", "socket", e.Socket(), "session", session, "attempts", attempt, "elapsed", time.Since(bootStart))
			return false, nil, fmt.Errorf("tmux session did not start after %d attempts (fast-failure spiral guard; see maxBootAttempts)", attempt)
		}
		if time.Now().After(bootDeadline) {
			logger.Warn("reed: boot gave up after overall timeout", "socket", e.Socket(), "session", session, "attempts", attempt, "elapsed", time.Since(bootStart))
			return false, nil, fmt.Errorf("tmux session did not start within %s", bootOverallTimeout)
		}
	}

	// remain-on-exit keeps a pane whose command exits around as
	// pane_dead=1 instead of vanishing (which would also kill the session
	// if it were the last pane) — the mechanism reconcile's dead-pane
	// detection depends on.
	if err := e.tmux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		return false, nil, fmt.Errorf("set remain-on-exit: %w", err)
	}
	// Pin the mouse mode explicitly, in both directions: this call always
	// runs on this fresh-boot path, even to set "off", so the live mouse
	// state is deterministic regardless of the tmux backend's own
	// default (Shared Decision explicit-set-both-ways-at-boot). Like
	// remain-on-exit, this never re-applies on an already-up session — the
	// early return above skips this whole path in that case.
	if err := e.tmux.run("set-option", "-g", "mouse", mouse); err != nil {
		return false, nil, fmt.Errorf("set mouse: %w", err)
	}

	// Unlike the two set-option calls above, this one is non-fatal by design:
	// remain-on-exit and mouse are correctness dependencies, while status and
	// window-size are geometry-quality options whose absence degrades to
	// tmux's own proportional rescale — a working session — and psmux's
	// support for both is unverified anywhere in this repo, so a capability
	// reed cannot confirm must not be able to take the boot down (Shared
	// Decision geometry-tmux-failures-are-non-fatal-everywhere).
	// Boot options never re-apply to an already-up session (the healthy
	// already-up path returns early, above this block), which is why
	// AttachArgv re-pins them in its own pre-flight rather than relying on
	// this call.
	e.pinGeometryOptionsLocked()

	return true, stripped, nil
}

// stripTraceID removes LYX_TRACE_ID from env before the tmux server inherits it.
// The server is a long-lived singleton reattached by unrelated invocations.
func stripTraceID(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.SplitN(entry, "=", 2)[0] == "LYX_TRACE_ID" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// ensureHeaderPaneLocked ensures the header pane exists and is alive.
// (Re)creates it when missing, dead, or gone. The header is separate from
// strands and must land physically topmost so layout heights stay correct.
// The (re)creation itself is splitHeaderPaneAtTopLocked's job, including the
// even-vertical retry that keeps a stale or lost HeaderPaneID from wedging the
// worktree — see that function for why a split against the top pane can fail
// at all.
func (e *Engine) ensureHeaderPaneLocked(st *ReedState) error {
	session := e.SessionName()
	live, err := e.tmux.listPanes(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	if len(live) == 0 {
		// A zero-pane husk cannot host a split; surface it rather than
		// panicking on an empty slice below. ensureServerAndSessionLocked
		// kills husks before this runs, so reaching this means the session
		// emptied between the two probes — an error, not an invariant.
		return fmt.Errorf("session %s has no panes to split a header pane from", session)
	}

	if st.HeaderPaneID != "" && aliveIDSet(live)[st.HeaderPaneID] {
		// Present AND alive: idempotent no-op across up/resume. Aliveness,
		// not mere presence, is the check — a dead-but-present header corpse
		// (kept enumerable by reconcile's deliberate exemption) must be
		// healed here, not mistaken for a working header.
		return nil
	}

	// A dead-but-present header corpse is killed before the replacement is
	// split, so its top row is freed and the topmost target below is a real
	// (usually alive) pane — unless the corpse is the session's SOLE pane,
	// where killing first would end the session; then the corpse itself is
	// the split target and it is killed after the new pane exists.
	corpseID := ""
	if st.HeaderPaneID != "" && liveIDSet(live)[st.HeaderPaneID] {
		corpseID = st.HeaderPaneID
		if len(live) > 1 {
			if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
				return fmt.Errorf("kill dead header pane %s: %w", corpseID, err)
			}
			corpseID = ""
			live, err = e.tmux.listPanes(session)
			if err != nil {
				return fmt.Errorf("list panes after killing dead header: %w", err)
			}
			if len(live) == 0 {
				return fmt.Errorf("session %s has no panes to split a header pane from", session)
			}
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve lyx binary path: %w", err)
	}

	paneID, err := e.splitHeaderPaneAtTopLocked(session, live)
	if err != nil {
		return fmt.Errorf("split header pane: %w", err)
	}

	if corpseID != "" {
		// The sole-pane corpse the new header was split off of: now that a
		// second pane exists, killing it can no longer end the session.
		// Best-effort — a corpse that somehow vanished already is fine; the
		// discard is still worth a Debug line so the step is observable at
		// the trace level without upgrading routine cleanup to a Warn.
		if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
			logger.Debug("reed: best-effort kill of header corpse pane failed", "socket", e.Socket(), "pane", corpseID, "err", err)
		}
	}

	launchCmd := headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())
	if launchCmd == "" {
		// Under go test the header pane stays a bare blocking shell — see
		// headerLaunchLine: re-exec'ing exe here would run the test binary's
		// entire suite recursively. The pane still exists and its id is still
		// recorded below, so layout geometry and up/resume idempotence are
		// unchanged.
		logger.Info("reed: header re-exec suppressed under go test, pane left as bare shell", "socket", e.Socket(), "pane", paneID, "exe", exe)
	} else {
		// Same literal send-keys mechanics launchStrandLocked (spawn.go) uses:
		// -l so tmux never reinterprets any part of the launch line, then a
		// separate Enter to submit it.
		if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(launchCmd)); err != nil {
			logger.Warn("reed: failed to send header launch command", "socket", e.Socket(), "pane", paneID, "err", err)
			return fmt.Errorf("send header launch command: %w", err)
		}
		if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
			logger.Warn("reed: failed to submit header launch command", "socket", e.Socket(), "pane", paneID, "err", err)
			return fmt.Errorf("submit header launch command: %w", err)
		}
	}

	st.HeaderPaneID = paneID
	if err := SaveState(e.stateDir(), st); err != nil {
		return fmt.Errorf("persist header pane id: %w", err)
	}
	return nil
}

// topmostPaneID returns the id of the pane sitting physically highest in the window — the smallest
// pane_top — which is the only place a header pane may be split in.
// live must be non-empty.
func topmostPaneID(live []LivePane) string {
	topmost := live[0]
	for _, p := range live[1:] {
		if p.Top < topmost.Top {
			topmost = p
		}
	}
	return topmost.ID
}

// splitHeaderPaneAtTopLocked splits a new pane in above the physically topmost pane of session and
// returns its id, retrying once behind an even-vertical re-tile when the first attempt has no room.
//
// The retry is what keeps a lost or stale ReedState.HeaderPaneID from wedging a worktree
// permanently (R4 review finding R4-F4). The header band is one row by default
// (HeaderConfig.HeightRows), and tmux cannot split a one-row pane at all — so the moment
// HeaderPaneID stops naming the pane at the top, the topmost split target IS an untracked one-row
// band and every later up/resume fails with "no space for new pane", forever, while status keeps
// reporting the session healthy and the only escape ("lyx reed down", then up) is named nowhere.
// Two ordinary routes reach that state: scrubbing .lyx/reed.json, a never-tracked machine-local
// tree the Durable-vs-Ephemeral State Invariant makes disposable (a plain `git clean -xdf` in the
// worktree does it), and a process death in the window between the split above and the SaveState
// that records its id.
//
// select-layout even-vertical evens every pane's height using tmux's own built-in layout — no reed
// layout string is computed or applied here, so anyPlacedStrand's empty-layout hazard (apply.go) is
// not in play — after which the same split has room and STILL lands the new pane at pane_top 0
// (verified live, tmux 3.6). The op's normal reconcileApplyPersistLocked tail then restores reed's
// real geometry and reaps the untracked band; an op that fails before reaching that tail leaves the
// window evenly tiled, a cosmetic state the next successful op corrects.
// Both subcommands are already in requiredSubcommands, so the multiplexer capability contract is
// unchanged.
//
// On a failed retry the FIRST error is returned, not the retry's: it describes the state the
// operator actually has, and the re-tile is an internal repair attempt rather than something they
// asked for.
func (e *Engine) splitHeaderPaneAtTopLocked(session string, live []LivePane) (string, error) {
	paneID, firstErr := e.splitPaneAboveLocked(topmostPaneID(live), live)
	if firstErr == nil {
		return paneID, nil
	}
	logger.Warn("reed: failed to split header pane, retrying behind an even-vertical re-tile", "socket", e.Socket(), "session", session, "err", firstErr)

	if err := e.tmux.run("select-layout", "-t", exactSessionWindowTarget(session), "even-vertical"); err != nil {
		logger.Warn("reed: even-vertical re-tile failed, header split not retried", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	retiled, err := e.tmux.listPanes(session)
	if err != nil || len(retiled) == 0 {
		logger.Warn("reed: could not re-enumerate panes after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	paneID, err = e.splitPaneAboveLocked(topmostPaneID(retiled), retiled)
	if err != nil {
		logger.Warn("reed: header split still had no room after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	logger.Info("reed: header split recovered by an even-vertical re-tile", "socket", e.Socket(), "session", session, "pane", paneID)
	return paneID, nil
}

// splitPaneAboveLocked splits a new pane in directly above target and returns its id, refusing an
// id that was already present in preSplitLive.
//
// -b places the NEW pane above target rather than below it (tmux's default split direction is
// vertical, new pane below): render.Rules always emits the header cell FIRST, assuming a fixed top
// band, and psmux/tmux apply layout cells POSITIONALLY to the window's actual top-to-bottom pane
// order — so the header pane must physically stay topmost, or the very first select-layout would
// invert the header and the first strand's heights (verified live: without -b, a stacked-adds smoke
// scenario failed a later split with "no space for new pane" because the 1-row header cell landed
// on the STRAND's physically-top pane instead). Every strand split (spawn.go) always targets a
// non-header pane and inserts below it, so the header is the only split in the whole engine that
// needs -b.
//
// The genuinely-new-pane guard is the same one launchStrandLocked runs: psmux's silent
// too-small-to-split failure prints an EXISTING pane's id with exit 0, and recording that id as the
// header would bind the header to a strand's pane — the next layout string would then carry a
// duplicate pane number, destroying the session's panes wholesale (see
// validateSplitCreatedNewPane).
func (e *Engine) splitPaneAboveLocked(target string, preSplitLive []LivePane) (string, error) {
	out, err := e.tmux.output("split-window", "-b", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	paneID := strings.TrimSpace(out)
	if err := validateSplitCreatedNewPane(paneID, preSplitLive, target); err != nil {
		return "", err
	}
	return paneID, nil
}

// Up ensures the server and session exist.
// Up never launches strands;
// Resume rebuilds content after a server restart.
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
		// The stripped env keys are stamped for diagnosis — reed.json records
		// what the server spawn actually removed. HeaderPaneID is cleared
		// alongside every strand binding for the identical reason — a
		// reborn session's reused pane id would otherwise be mistaken for
		// the still-live header pane — so ensureHeaderPaneLocked below
		// rebuilds it fresh; the clear lives here, not inside
		// clearAllPaneBindings itself, since the header is not a strand
		// binding.
		if booted {
			clearAllPaneBindings(st)
			st.StrippedEnv = stripped
			st.HeaderPaneID = ""
		}

		if err := e.ensureHeaderPaneLocked(st); err != nil {
			return err
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		// len(st.Strands) deliberately excludes the header pane: the header
		// is not in st.Strands (Shared Decision header-is-not-a-strand), so
		// this count is already correct by construction. Do not "fix" a
		// future off-by-one here by adding the header — it must never be
		// counted as a strand.
		result = UpResult{Session: e.SessionName(), Socket: e.Socket(), Strands: len(st.Strands)}
		return nil
	})
	return result, err
}

// Resume boots server+session if absent, reconciles stale bindings, relaunches non-live strands,
// and re-applies the layout.
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
		// HeaderPaneID is cleared alongside them for the identical reason —
		// a reborn session's reused pane id would otherwise be mistaken for
		// the still-live header pane — so ensureHeaderPaneLocked below
		// rebuilds it fresh before any strand replay below runs; the clear
		// lives here, not inside clearAllPaneBindings itself, since the
		// header is not a strand binding.
		if booted {
			clearAllPaneBindings(st)
			st.StrippedEnv = stripped
			st.HeaderPaneID = ""
		}

		if err := e.ensureHeaderPaneLocked(st); err != nil {
			return err
		}

		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		killed, err := e.reconcileLocked(st, live)
		if err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if len(killed) > 0 {
			live, err = e.tmux.listPanes(e.SessionName())
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
			if err := SaveState(e.stateDir(), st); err != nil {
				return fmt.Errorf("persist strand: %w", err)
			}
			// Re-apply the layout after each launch, not once at the end:
			// consecutive splits without a re-apply halve the same target
			// pane until tmux silently refuses to split it, while a
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

// Down tears this worktree's session down and waits for async teardown to finish.
// Only kill-session (not kill-server, which other worktrees share).
// Waits for server and pane-child processes to actually release resources.
//
// Down is also the only lyx-only escape from the foreign-session refusal (generation.go): it loads
// no state and so never reaches that check, which matters for an operator who can run lyx but not
// raw tmux — a CI-like environment, a sandboxed agent — since the refusal's own message names only a
// `tmux kill-session` remedy. That escape used to be silent about its cost: with a worktree renamed
// while its session was up, `down` reported ok:true, deleted reed.json, and left the recorded session
// and every strand process in it running on the shared socket, addressable by no reed verb ever
// again because no worktree of that name exists to derive it from (R6 review finding R6-F3,
// reproduced live). It now names that session in the result and at Warn.
//
// It still does not KILL it, and must not: the recorded name is this worktree's own former session
// after a rename, but a SIBLING worktree's live session after a hand-copied .lyx (R5-F4), and reed
// cannot tell those apart. Killing it would re-open R5-F4's damage under a different verb.
// It still DELETES the state file, so `down` stays the idempotent escape it is — reporting the
// abandonment is what keeps that from being a silent loss.
func (e *Engine) Down() (DownResult, error) {
	var result DownResult
	err := e.withOpLock(func() error {
		start := time.Now()
		defer func() {
			logger.Info("reed: down complete", "socket", e.Socket(), "session", e.SessionName(), "duration", time.Since(start))
		}()
		// Grab the server's OS pid while our session can still be queried —
		// it is the only reliable death signal: tmux's CLI cannot report
		// "no server" at all (list-sessions exits 0 with empty output and
		// kill-server exits 0 whether or not a server holds the socket).
		serverPID := e.serverPIDLocked()

		// Capture this session's pane process subtrees BEFORE kill-session,
		// while the panes still exist to be listed — the shells tmux ran in
		// each pane plus their descendants (on Windows the process actually
		// holding the worktree directory is a deeper descendant of the pane
		// pid). Reaping them is how down keeps its "no stray state" guarantee
		// at the pane level (see reapPaneChildren).
		// Roots come from ALIVE panes only (sessionReapRootsLocked): a dead
		// pane's recorded #{pane_pid} names a process that already exited, and
		// expanding a recycled pid would make this reap wait on, and then
		// SIGKILL, an unrelated process tree — the rule RemoveStrand already
		// followed and this path did not (R2 review finding R2-F2).
		panePIDs := e.paneProcessTreePIDsLocked()

		// Ignore the error: the session may already be gone, and Down must
		// stay idempotent either way. The exact-match target is load-bearing
		// on exactly this ignored-error path: a bare -t name would PREFIX
		// match once this session is gone, so a second down would kill a
		// prefix-sharing sibling worktree's session (see exactSessionTarget).
		// Still worth a Debug line so the discard is observable at the
		// step-trace level without upgrading routine idempotent-teardown
		// noise to a Warn.
		if err := e.tmux.run("kill-session", "-t", exactSessionTarget(e.SessionName())); err != nil {
			logger.Debug("reed: best-effort kill-session failed (session may already be gone)", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}

		// Tidy the server only if no sessions remain. An EMPTY list-sessions
		// covers both "zero sessions" and "no server" (tmux does not
		// distinguish them, and kill-server is harmless in both); an ERRORED
		// list-sessions means the socket-holder is unreachable — a zombie
		// server cannot be hosting healthy sibling sessions, so it is torn
		// down too rather than left squatting on the socket (the server's
		// own cwd is the hub's _board/.lyx/logs dir, not this worktree, since the
		// debug-logging batch — the same sessionless-holder reasoning the
		// pre-boot check applies regardless).
		var serverErr error
		if out, err := e.tmux.output("list-sessions", "-F", "#{session_name}"); err != nil || strings.TrimSpace(out) == "" {
			logger.Info("reed: tearing down tmux server", "socket", e.Socket(), "serverPID", serverPID)
			// Best-effort — the more significant "server did not confirm
			// gone" outcome is already covered at Warn by ensureServerGoneLocked
			// below; this earlier, routine kill-server discard only needs a
			// Debug line, not a duplicate Warn.
			if err := e.tmux.run("kill-server"); err != nil {
				logger.Debug("reed: best-effort kill-server failed", "socket", e.Socket(), "serverPID", serverPID, "err", err)
			}
			serverErr = e.ensureServerGoneLocked(serverPID)
			if serverErr != nil {
				logger.Warn("reed: server did not confirm gone after kill-server", "socket", e.Socket(), "serverPID", serverPID, "err", serverErr)
			}
		}

		// ALWAYS reap this session's pane child subtree, even when the server
		// teardown above hit trouble: a slow or failed server death must never
		// skip the pane reap. An earlier fixed-deadline server wait returned on
		// timeout and aborted down BEFORE this reap under CPU saturation,
		// leaking pane children that kept the worktree directory busy (the
		// server itself no longer holds a worktree busy since the
		// debug-logging batch — its own cwd is now the hub's _board/.lyx/logs dir,
		// not this worktree — though a lingering server process is still its
		// own leak worth reaping). kill-session / kill-server terminate pane
		// children asynchronously, so force-kill any that outlive the
		// deadline.
		reapPaneChildren(panePIDs, reapExitTimeout)

		if serverErr != nil {
			return serverErr
		}

		// Read the state file for the abandonment report BEFORE deleting it: after the delete there
		// is nothing left that names the orphan. A corrupt or absent file simply reports nothing —
		// Down must stay idempotent and must never fail over a file it is about to remove.
		abandoned := ""
		if st, loadErr := LoadState(e.stateDir()); loadErr == nil && st != nil {
			if verdict, _ := e.classifyRecordedSessionLocked(st.PaneGeneration); verdict == recordedSessionLive {
				abandoned = st.PaneGeneration.SessionName
				logger.Warn("reed: down left a recorded session running on the shared socket and deleted the state naming it",
					"socket", e.Socket(), "session", e.SessionName(), "abandonedSession", abandoned,
					"remedy", fmt.Sprintf("%s -L %s kill-session -t '=%s'", e.TmuxPath(), e.Socket(), abandoned))
			}
		}

		path := filepath.Join(e.stateDir(), reedStateFileName)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete state: %w", err)
		}

		result = DownResult{Session: e.SessionName(), AbandonedSession: abandoned}
		return nil
	})
	return result, err
}

// sessionlessSocketHolderPersists reports whether a process holds this engine's
// socket without any session, persisting across staleSocketGrace (grace prevents
// reaping a sibling worktree's just-spawned server).
func (e *Engine) sessionlessSocketHolderPersists() bool {
	deadline := time.Now().Add(staleSocketGrace)
	for {
		// A socket that lists sessions hosts a healthy shared server — never
		// in scope for reaping, no matter what else is pending.
		if out, err := e.tmux.output("list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(out) != "" {
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

// serverPIDLocked returns the pid of the tmux server holding this engine's socket, or 0 when no
// server answers.
// Must run before kill-session when the caller intends to wait on the server.
//
// It is the SERVER's pid, not this session's, and it comes back even when this worktree's session
// does not exist: #{pid} is server-global and display-message exits 0 for a -t target naming no
// session (see generation.go's paneGenerationLocked). So on a socket a sibling worktree is using,
// this answers that shared server's pid rather than 0 — which is correct for its one consumer, since
// Down only spends the value after list-sessions came back empty, but is not what "this session's
// server, or 0 if unknown" would lead a reader to expect (R6 review finding R6-F4).
func (e *Engine) serverPIDLocked() int {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{pid}")
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return pid
}

// sessionReapRootsLocked returns this session's safe descendant-closure reap roots — the
// #{pane_pid} of every pane that is present AND still running (see safeReapRoot, strand.go).
// Returns nil on failure.
// Must run before kill-session while panes exist.
func (e *Engine) sessionReapRootsLocked() []int {
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		return nil
	}
	return sessionReapRoots(live)
}

// paneProcessTreePIDsLocked returns this session's safe reap roots and their descendants.
// Must run before kill-session while panes exist.
func (e *Engine) paneProcessTreePIDsLocked() []int {
	return e.descendantClosurePIDs(e.sessionReapRootsLocked())
}

// forceKillExitGrace bounds how long to wait for force-kill to land
// (TerminateProcess is asynchronous on Windows; generous for CPU saturation).
const forceKillExitGrace = 5 * time.Second

// reapExitTimeout bounds pane-child and server reaps before force-killing.
// Generous for CPU saturation; reaps confirm actual exit, not just timer.
const reapExitTimeout = 15 * time.Second

// processExitPoll is how often waitProcessExit re-probes a pid's liveness.
// It polls rather than blocking on the kernel because every pid reed waits on belongs to another
// process's child tree — see waitProcessExit.
const processExitPoll = 50 * time.Millisecond

// ensureServerGoneLocked guarantees no tmux process remains on this engine's
// socket after kill-server. Force-reaps if needed; waits for async teardown.
func (e *Engine) ensureServerGoneLocked(serverPID int) error {
	_ = waitProcessExit(serverPID, reapExitTimeout)
	if len(e.serverProcessesOnSocket()) == 0 {
		return nil
	}
	return e.reapSocketProcesses()
}

// reapPaneChildren waits for pane child processes to exit, force-killing
// stragglers. Pane-destroying ops must reap children to avoid worktree dir locks.
func reapPaneChildren(pids []int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if err := waitProcessExit(pid, time.Until(deadline)); err == nil {
			continue
		}
		// The graceful window elapsed with the process still up — a pane child
		// that ignores or is out of reach of tmux's own SIGHUP cascade (a
		// trapped SIGHUP, a detached session). Force-kill it and confirm.
		if err := proc.KillPID(pid); err != nil {
			logger.Warn("reed: failed to force-kill straggling pane child", "pid", pid, "err", err)
			continue
		}
		if err := waitProcessExit(pid, forceKillExitGrace); err != nil {
			logger.Warn("reed: pane child survived force-kill", "pid", pid, "err", err)
		}
	}
}

// waitServerProcessesGone polls until no tmux process names this socket.
// Catches both main server and any "__warm__" helper.
func (e *Engine) waitServerProcessesGone(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		pids := e.serverProcessesOnSocket()
		if len(pids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tmux processes %v still on socket %s after %s", pids, e.Socket(), timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// reapSocketProcesses force-terminates every tmux process on this engine's
// socket and confirms they're gone. Necessary for zombie servers and helpers
// that ignore socket-routed kill-server.
func (e *Engine) reapSocketProcesses() error {
	_ = e.tmux.run("kill-server")
	for _, pid := range e.serverProcessesOnSocket() {
		// Best-effort per pid: a socket holder that already exited between the
		// scan and this kill is exactly what the reap wanted, and
		// waitServerProcessesGone below is what actually decides the outcome.
		if err := proc.KillPID(pid); err != nil {
			logger.Debug("reed: best-effort kill of socket holder failed", "socket", e.Socket(), "pid", pid, "err", err)
		}
	}
	return e.waitServerProcessesGone(reapExitTimeout)
}

// waitProcessExit blocks until the process named by pid is no longer running, or errors after
// timeout.
// It is necessary because tmux's kill-server and kill-session both terminate their process trees
// asynchronously.
//
// Liveness is POLLED via proc.IsAlive rather than blocked on with os.Process.Wait, and that choice
// is load-bearing rather than stylistic: every pid reed waits on — the tmux server and each pane's
// process subtree — is a child of the TMUX server, never of this process. os.Process.Wait on a
// non-child returns ECHILD ("waitid: no child processes") immediately, measured at ~20µs against a
// demonstrably live process, so a Wait-based implementation reported EVERY such pid as
// already-exited and reapPaneChildren's force-kill fallback never ran once.
//
// A pid that was recycled by an unrelated process after the caller snapshotted it reads as still
// alive here, burns the full timeout, and is then force-killed along with its whole subtree by
// reapPaneChildren — so what bounds this is entirely the CALLER's snapshot discipline: both reap
// paths take their descendant-closure roots from panes that are present AND not dead at snapshot
// time (safeReapRoot, strand.go), because tmux keeps reporting a dead pane's #{pane_pid} long after
// that process exited. Down did not honour that rule until the R2 review; the two snapshots now
// share one predicate precisely so this comment cannot describe only one of them again.
//
// Honest limit: proc.IsAlive is a signal-0 probe, so a not-yet-reaped ZOMBIE reads as alive here.
// That window is short in practice — every pid reed waits on is a child of the tmux server, which
// reaps its own children, and once the server itself dies the survivors are reparented to init and
// reaped there — but a caller must not read this function as proof the process's resources are
// released, only that its pid no longer answers.
func waitProcessExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for {
		if !proc.IsAlive(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process %d still up %s after teardown", pid, timeout)
		}
		time.Sleep(processExitPoll)
	}
}

// noSessionMessage builds operator-facing text for an absent session,
// pointing at resume (if strands exist) or up (if empty).
//
// stateReadable distinguishes "reed read the state and it holds no strands" from "reed could not
// read the state at all", which strandCount alone cannot express: an unreadable file yields a count
// of zero and would otherwise be reported as an empty worktree, sending the operator to `up` — which
// then fails with the corrupt-file error (see unreadableStateError). Claiming nothing is persisted
// when reed simply cannot tell is the one wrong thing this message can say (R5 review finding
// R5-F8), so an unreadable file gets its own branch and points at the same two remedies the load
// error names, rather than inventing a third.
func noSessionMessage(strandCount int, stateReadable bool) string {
	if !stateReadable {
		return `no reed session, and reed's persisted state could not be read; run "lyx reed down" to clear it, or "lyx reed up" for the full diagnosis`
	}
	if strandCount <= 0 {
		return `no reed session; run "lyx reed up"`
	}
	return fmt.Sprintf(`no reed session (%d strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`, strandCount)
}

// requireSessionLocked returns an actionable error when this worktree's
// session does not exist, including whether resume would have content to rebuild.
//
// "Actionable" is why the foreign-session diagnosis is consulted here rather than left to the two
// booting verbs. Both routes into refuseLiveForeignSessionLocked — a worktree renamed while its
// session was up, a .lyx copied between worktrees of one hub — leave THIS worktree's session absent,
// so every non-booting verb lands in this function, and the plain no-session text it used to return
// names `lyx reed resume` and `lyx reed up` as the remedies. Both of those then refuse, for exactly
// the reason this text did not mention: the operator's whole diagnostic surface, `status` above all,
// reported a bare "no session" and routed them into a loop, while the state file naming the orphan
// was already loaded right here (R6 review finding R6-F1).
//
// It costs no tmux round trip on the healthy path: refuseLiveForeignSessionLocked returns
// immediately when the recorded session name is empty or is this worktree's own, which is every
// state file reed itself writes.
func (e *Engine) requireSessionLocked() error {
	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if up {
		return nil
	}

	// len(st.Strands) deliberately excludes the header pane (see
	// noSessionMessage's doc comment): st.HeaderPaneID is a separate field,
	// never part of Strands, so this count is already correct by
	// construction.
	strandCount := 0
	st, loadErr := LoadState(e.stateDir())
	if st != nil {
		strandCount = len(st.Strands)
		if err := e.refuseLiveForeignSessionLocked(st.PaneGeneration); err != nil {
			return err
		}
	}
	// An ABSENT file is readable-and-empty, not unreadable: a brand-new worktree has no reed.json
	// and must still get the plain "run lyx reed up" text.
	return errors.New(noSessionMessage(strandCount, loadErr == nil))
}

// Status reports this session's tracked strands and their live/dead state by cross-referencing the
// persisted table against live panes.
// Read-only.
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

		live, err := e.tmux.listPanes(session)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}

		// aliveIDSet, not liveIDSet: report a strand bound to a
		// dead-but-present pane as not live — the operator asks status whether
		// the strand's process is running, not whether tmux still lists a
		// (dead) pane for it.
		aliveIDs := aliveIDSet(live)
		// This loop iterates st.Strands only — the header pane is
		// deliberately never reported as a strand here (it is not one; see
		// ReedState.HeaderPaneID). Status still succeeds (the session is up)
		// when st.Strands is empty but the header pane is alive; a future
		// edit must not "fix" a missing header row by appending one here.
		strands := make([]StrandStatus, len(st.Strands))
		for i, s := range st.Strands {
			strands[i] = StrandStatus{GUID: s.GUID, Name: s.Name, PaneID: s.PaneID, Live: aliveIDs[s.PaneID]}
		}

		result = StatusResult{Session: session, Socket: e.Socket(), Strands: strands}
		return nil
	})
	return result, err
}
