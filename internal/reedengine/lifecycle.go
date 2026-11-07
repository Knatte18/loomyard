// lifecycle.go implements the lifecycle engine ops Up, Resume, Down and Status.
// It also holds the pure planning helpers that make their decisions unit-testable without a live tmux server.
// Up ensures the substrate (server and session) exists and never launches a strand command.
// Resume is the only replayer; it skips anchor:hidden strands, which are pending their first surface rather than dead.

package reedengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// stateDir returns the path to the worktree-level ephemeral tree holding reed.json and reed.lock.
// The tree is anchored at AnchorPath, so it is a directory sibling of the durable, fabric-synced _lym tree.
// It is per worktree, whereas the hub logs directory is given to the engine and never derived here.
func (e *Engine) stateDir() string {
	return filepath.Join(e.geom.AnchorPath, lymdirs.DotLymDirName)
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
	// Dropped counts the finished strands Resume removed from state instead of relaunching.
	Dropped int
	// Held names the strands a pause covering this pair left down instead of relaunching.
	Held []string
}

// DownResult reports the outcome of Down: the session name that was torn down, plus the name of a
// live session Down knowingly walked away from.
type DownResult struct {
	Session string
	// AbandonedSession names a tmux session this worktree's state file was recorded against.
	// That session is still running on the shared per-hub socket under a different name.
	// Down did not kill it, and it deleted the state file that named it, so it can no longer track it.
	// Empty in an ordinary teardown.
	AbandonedSession string
	// EndedServer is true when Down took its server-teardown branch:
	// no session was left, or the session listing errored.
	EndedServer bool
}

// StrandStatus is one strand's status in StatusResult.
type StrandStatus struct {
	GUID   string
	Name   string
	PaneID string
	// PanePID is the pane's process id from the session listing.
	// It is 0 when the strand's pane is in no listing or is dead.
	PanePID int
	Live    bool
	// Present reports that the strand's binding names a present pane, alive or dead.
	// A caller that stops a strand reads it to decide whether a dead strand still has a pane to remove.
	Present bool
	// OwnWindow reports that the strand lives in a window of its own, own-window or overview.
	// A caller that stops a strand reads it to decide whether the strand's window goes with it.
	OwnWindow bool
	// Elsewhere is the session:window placement of the strand's bound pane.
	// It is set only when that pane sits in another session of the reed server, and empty otherwise.
	Elsewhere string
	// Retiring reports that a caller asked for this strand's removal.
	// It grants nothing by itself: a caller that finds it set only completes the request.
	Retiring bool
	// Color is the palette color the strand's segment resolves to, empty when it has none.
	Color segmentcolor.Color
	// LymBin is the absolute lym path the strand's pane prelude exported as LYM_BIN at its latest launch.
	// It is empty when no prelude was composed.
	LymBin string
	// Provider is the opaque tag its spawner set.
	// reed keys the session namers by it and never reads what it means.
	Provider string
}

// StatusResult reports this session's tracked strands and their live/dead state.
type StatusResult struct {
	Session string
	Socket  string
	Strands []StrandStatus
}

// Boot-loop tuning for bringing up the tmux server and session.
// maxBootAttempts caps the attempt count against fast-failure spirals.
// A fork can fail near-instantly under resource pressure, which otherwise spawns boots in a tight loop.
// staleSocketGrace is the grace window before a socket with no session counts as stale.
// A sibling worktree may be booting a fresh session in that window.
const (
	bootAttemptTimeout = 20 * time.Second
	bootPoll           = 100 * time.Millisecond
	bootOverallTimeout = 90 * time.Second
	maxBootAttempts    = 8
	staleSocketGrace   = 5 * time.Second
)

// ErrNoSession is the sentinel a no-session refusal wraps.
// It is exported so that a caller outside reed can read Status as a liveness probe.
// Such a caller maps an absent session to not live, not to a read failure.
var ErrNoSession = errors.New("reed: no session")

// noSessionError carries its message as its visible text and unwraps to ErrNoSession.
type noSessionError struct{ msg string }

func (e noSessionError) Error() string { return e.msg }

func (e noSessionError) Unwrap() error { return ErrNoSession }

// serverLogPruneKeep is how many pre-existing server logs a prune keeps.
// The fresh boot's log is added on top of them.
const serverLogPruneKeep = 2

// The tmux log files written at -v and -vv share these name prefixes and suffix.
// At -vv the server also writes tmux-out-<pid>.log, and all three kinds are pruned.
const (
	serverLogNamePrefix = "tmux-server-"
	clientLogNamePrefix = "tmux-client-"
	outLogNamePrefix    = "tmux-out-"
	serverLogNameSuffix = ".log"
)

// pruneServerLogsLocked removes the files in logsDir named prefix+*+suffix, keeping the newest keep by mtime.
// A file already gone when it is removed is not an error, since another worktree may have pruned it first.
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
			// The file vanished between ReadDir and Info, for example to a concurrent prune by another worktree's boot.
			// It is already gone, which is what pruning wants, so it is skipped rather than reported as an error.
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

// planResumeLaunches splits the non-live strands into those to relaunch and those to drop.
// A non-live strand is dropped, hidden or not, when its DoneWhen list is non-empty and every path in it exists:
// its work finished, and a relaunch would redo it.
// Every other non-live, non-hidden strand is relaunched;
// a hidden strand that is not dropped is neither.
// A live strand is in neither list.
// When paused, every strand that would be relaunched is returned in held instead, so a pause leaves it down.
func planResumeLaunches(strands []Strand, liveIDs map[string]bool, exists func(path string) bool, paused bool) (launch, drop, held []Strand) {
	for _, s := range strands {
		live := s.PaneID != "" && liveIDs[s.PaneID]
		if live {
			continue
		}
		if len(s.DoneWhen) > 0 && allPathsExist(s.DoneWhen, exists) {
			drop = append(drop, s)
			continue
		}
		if s.Display.Anchor == render.AnchorHidden {
			continue
		}
		if paused {
			held = append(held, s)
			continue
		}
		launch = append(launch, s)
	}
	return launch, drop, held
}

// pairPaused reports whether a live pause entry covers this engine's pair.
// The prime (empty NameSlug) and a standalone geometry (empty PauseRecordPath) are never covered.
// An unreadable pause record holds nothing.
func (e *Engine) pairPaused() bool {
	if e.geom.PauseRecordPath == "" || e.geom.NameSlug == "" {
		return false
	}
	record, err := pauserecord.Read(e.geom.PauseRecordPath)
	if err != nil {
		logger.Warn("reed: resuming despite an unreadable pause record", "path", e.geom.PauseRecordPath, "cause", err)
		return false
	}
	_, covered := record.Covering(e.geom.NameSlug)
	return covered
}

// dropStrands removes drop's strands from st, logs one Info line per strand, and returns how many it removed.
func dropStrands(st *ReedState, drop []Strand) int {
	dropping := make(map[string]bool, len(drop))
	for _, s := range drop {
		dropping[s.GUID] = true
		logger.Info("reed: resume dropped a finished strand", "name", s.Name, "guid", s.GUID)
	}
	kept := st.Strands[:0]
	for _, s := range st.Strands {
		if !dropping[s.GUID] {
			kept = append(kept, s)
		}
	}
	st.Strands = kept
	pruneWindowGroups(st)
	return len(drop)
}

// allPathsExist reports whether exists holds for every path.
func allPathsExist(paths []string, exists func(path string) bool) bool {
	for _, path := range paths {
		if !exists(path) {
			return false
		}
	}
	return true
}

// pathExists reports whether a file or directory is at path.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sessionSubstrateLocked reports whether the session is up and usable.
// up reports that the session exists; usable also requires at least one pane.
// A session with zero panes is broken substrate that a strand can never be split into.
// Up is true while usable is false, which tells it apart from a session that never came up.
// Callers read this predicate rather than repeating the pane-list check.
func (e *Engine) sessionSubstrateLocked() (up bool, usable bool, err error) {
	session := e.SessionName()
	up, err = e.tmux.hasSession(session)
	if err != nil {
		return false, false, fmt.Errorf("check session: %w", err)
	}
	if !up {
		return false, false, nil
	}
	live, err := e.listStoredStrandPanes()
	if err != nil {
		return true, false, fmt.Errorf("list panes: %w", err)
	}
	return true, len(live) > 0, nil
}

// validateBootConfig refuses a boot-time config value that is wrong on its own, before any tmux round trip.
// It validates debug_log, mouse, watchdog, the segment colors, window_share_max and window_split.
// It returns the tmux global flags debug_log selects and the mouse option value.
// It runs before the boot's first tmux read, so a pure config error surfaces ahead of it.
func (e *Engine) validateBootConfig() (debugArgs []string, mouse string, err error) {
	// Validate debug_log before anything else touches tmux:
	// a misconfigured value is a pure config error, unrelated to server/session state,
	// so it must surface before the capability probe or any spawn attempt.
	debugArgs, err = debugLogArgs(e.cfg.DebugLog)
	if err != nil {
		return nil, "", err
	}

	// Validate mouse alongside debug_log, at the same early point:
	// this too is a pure config error.
	// It must surface before the capability probe or any spawn attempt, not partway through a boot.
	mouse, err = mouseOption(e.cfg.Mouse)
	if err != nil {
		return nil, "", err
	}

	// The boolean is discarded here: this is the one consumer of watchdogOption with an error channel.
	// Its only job is to make a typo fail `lym reed up` loudly and by name.
	// Every other reader of the key fails safe toward "no watchdog" instead.
	if _, err = watchdogOption(e.cfg.Watchdog); err != nil {
		return nil, "", err
	}

	// A segment color outside the palette is a pure config error too.
	// This is the only refusal;
	// every other reader goes through segmentColor and degrades to no color.
	if err = validateSegmentColors(e.cfg.SegmentColors); err != nil {
		return nil, "", err
	}
	if err = validateWindowConfig(e.cfg); err != nil {
		return nil, "", err
	}
	return debugArgs, mouse, nil
}

// preflightBootHost refuses, before any tmux round trip, a socket path over the OS limit.
// Its error names `TMUX_TMPDIR`.
// It also refuses a configured shell that does not resolve; its error names the `shell` key.
// It refuses a non-empty `tmux_conf` that does not name a usable file; its error names the `tmux_conf` key.
// It returns the resolved shell path the boot pins on the session;
// the validated tmux config path is empty when none is configured.
// On Windows it checks nothing and returns empty strings: psmux keeps no socket file;
// the boot keeps its current shell handling, and `tmux_conf` is parsed but never validated or sourced.
func (e *Engine) preflightBootHost() (shellPath, tmuxConf string, err error) {
	if runtime.GOOS == "windows" {
		return "", "", nil
	}
	if err := checkSocketPathLength(resolvedSocketDir(), e.Socket(), unixSocketPathLimit(runtime.GOOS)); err != nil {
		return "", "", err
	}
	shellPath, err = resolveShellPath(e.cfg.Shell)
	if err != nil {
		return "", "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	tmuxConf, err = tmuxConfPath(e.cfg.TmuxConf, home)
	if err != nil {
		return "", "", err
	}
	return shellPath, tmuxConf, nil
}

// ensureServerAndSessionLocked ensures this hub's tmux server and this worktree's session exist.
// It reports booted=true on a fresh spawn.
// It validates capability, debug_log, mouse, watchdog, segment colors and tmux_conf before any tmux round trip.
func (e *Engine) ensureServerAndSessionLocked() (booted bool, strippedKeys []string, err error) {
	debugArgs, mouse, err := e.validateBootConfig()
	if err != nil {
		return false, nil, err
	}

	shellPath, tmuxConf, err := e.preflightBootHost()
	if err != nil {
		return false, nil, err
	}

	// Fail loud, once per ensure/boot, if the configured multiplexer binary
	// is below the pinned version floor or missing a required subcommand.
	// Otherwise the gap surfaces later as a cryptic tmux error deep inside the boot loop.
	if err := e.probeCapabilityLocked(); err != nil {
		return false, nil, err
	}

	// Refuse a recorded-session collision here, ahead of the spawn below.
	// Doing it in loadOrInitStateLocked would come after the caller has already booted.
	// A refusal must not leave a bare session on the shared per-hub server as its residue.
	// This is the first check that needs a tmux round trip, so it cannot join the pre-tmux block above,
	// but it still precedes everything that creates anything.
	// See refuseRecordedForeignSessionBeforeBootLocked.
	if err := e.refuseRecordedForeignSessionBeforeBootLocked(); err != nil {
		return false, nil, err
	}

	session := e.SessionName()
	up, usable, err := e.sessionSubstrateLocked()
	if err != nil {
		return false, nil, err
	}
	if up {
		if usable {
			return false, nil, nil
		}
		// A session that exists but holds zero panes is broken substrate.
		// It cannot host a strand: there is no pane to split, and tmux offers no way to add a pane to an empty window.
		// Add would therefore fail forever while up kept reporting success.
		// The state is reachable when an applied layout destroyed every pane.
		// Tmux reaps any pane absent from a select-layout string.
		// Kill the husk and fall through to a fresh boot.
		// The booted=true return makes the caller clear every stale binding, as a server rebirth does.
		_ = e.tmux.run("kill-session", "-t", exactSessionTarget(session))
	}

	// This worktree's recorded session is gone with the server:
	// the earlier worktrees revive theirs first, so the session ids follow spawn order again.
	// Nothing is created yet.
	if !up && e.reviveDueLocked() {
		return false, nil, errReviveFirst
	}

	// A stale socket-holder wedges a fresh boot.
	// On Windows, psmux's internal "__warm__" helper can outlive a kill-server and sit on the -L socket
	// without ever hosting a session, so a new-session spawned against it never materializes.
	// A socket whose holder reports zero sessions across the grace window is stale.
	// Such a holder is a stale helper, a dying server, or an unreachable zombie.
	// It is never a healthy shared server: sibling worktrees' sessions would list.
	// Force-reaping it before spawning is therefore safe.
	if e.sessionlessSocketHolderPersists() {
		if err := e.reapSocketProcesses(); err != nil {
			logger.Warn("reed: failed to reap stale tmux socket-holder", "socket", e.Socket(), "err", err)
			return false, nil, fmt.Errorf("stale tmux socket-holder: %w", err)
		}
	}

	// Route the server's tmux-server-<pid>.log to the hub's deterministic forensic location (Shared Decision hub-logs-dir).
	// tmux writes that log to the server process's own cwd with no redirect flag, so cmd.Dir below is the only lever.
	// This happens on every boot, regardless of debug_log.
	// It runs before the boot loop, so a fresh server's log always lands in an existing, already pruned directory.
	logsDir := e.geom.LogsDir
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		logger.Warn("reed: failed to create hub logs dir", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("create %s: %w", logsDir, err)
	}
	// Prune to the newest 2 pre-existing logs before this boot's own log is written.
	// At most 3 logs ever exist: the 2 kept and this boot's fresh one (Shared Decision log-prune-keep-3).
	// This keeps forensic history without unbounded growth across repeated boots.
	// Both filename shapes a debug-armed boot can leave (server and client, see clientLogNamePrefix)
	// are pruned, each with its own bound, so neither accumulates without limit.
	if err := pruneServerLogsLocked(logsDir, serverLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune server logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune server logs: %w", err)
	}
	if err := pruneServerLogsLocked(logsDir, clientLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune client logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune client logs: %w", err)
	}
	// The -vv-only tmux-out-<pid>.log protocol log is the third shape a debug-armed boot can leave.
	// It is pruned on the same budget, so it does not accumulate without limit across repeated boots with debug_log: 2.
	if err := pruneServerLogsLocked(logsDir, outLogNamePrefix, serverLogPruneKeep); err != nil {
		logger.Warn("reed: failed to prune out logs", "logsDir", logsDir, "err", err)
		return false, nil, fmt.Errorf("prune out logs: %w", err)
	}

	// Env hygiene: a spawned server must never inherit this process's own agent session identity.
	// EnvScrub is the single chokepoint for that decision.
	clean, stripped := e.scrub.Apply(os.Environ())
	// The tmux server is a long-lived singleton that later invocations reattach to.
	// A one-shot child (board/fabric spawn) belongs to a single invocation and keeps inheriting the trace ID.
	// Filtering LYM_TRACE_ID is therefore its own named step, not folded into the scrub.
	// The scrub's stripped-keys return value is persisted verbatim into ReedState.StrippedEnv;
	// it is an engine-injected-variable diagnostic.
	// LYM_TRACE_ID is neither engine-injected nor meant to be recorded there.
	clean = stripTraceID(clean)
	clean = stripAgentNameEnv(clean)
	spawnSession := func() error {
		if err := CheckTmuxFloor(e.cfg.Tmux); err != nil {
			return fmt.Errorf("start tmux: %w", err)
		}
		cmd := exec.Command(e.cfg.Tmux, e.serverSpawnArgv(runtime.GOOS, debugArgs)...)
		cmd.Dir = logsDir
		cmd.Env = clean
		proc.Detach(cmd)
		if err := cmd.Start(); err != nil {
			logger.Warn("reed: failed to start tmux server", "socket", e.Socket(), "session", session, "err", err)
			return fmt.Errorf("start tmux: %w", err)
		}
		proc.Reap(cmd)
		logger.Info("reed: spawned tmux server", "socket", e.Socket(), "session", session, "pid", cmd.Process.Pid)
		return nil
	}

	// Boot retries against an overall deadline, because "the session is not answering yet" hides two slow paths.
	// (a) A slow boot on a loaded machine can outlast a whole attempt window,
	// so a small fixed attempt count would give up on a boot that would have succeeded.
	// (b) A zombie boot: tmux has a race under concurrent server startups.
	// The spawned server process runs but never becomes reachable on its socket;
	// list-sessions stays empty and has-session exits 1, forever, so no amount of waiting fixes it.
	// A socket still session-less after a full bootAttemptTimeout window has everything on it force-reaped by pid.
	// Because kill-server cannot reach an unreachable zombie, the spawn is then retried.
	// If the socket does list sessions, the server is healthy and shared, so it is never reaped;
	// the error reports the unexpected state instead.
	// A server that never boots fails at bootOverallTimeout, or at maxBootAttempts when the attempts fail fast.
	identityBefore := ""
	if runtime.GOOS != "windows" {
		identityBefore = e.serverIdentityLocked()
	}
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
				// The check returns its error at once, so this Warn fires at most once per boot.
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

		if out, err := e.tmux.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...); err == nil && strings.TrimSpace(out) != "" {
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

	e.touchDiscoverSignal()

	if runtime.GOOS != "windows" {
		if err := e.guardBootSessionLocked(identityBefore, tmuxConf); err != nil {
			return false, nil, err
		}
	}

	if err := e.pinBootOptionsLocked(mouse, shellPath); err != nil {
		return false, nil, err
	}

	return true, stripped, nil
}

// pinBootOptionsLocked pins the options a fresh boot sets on the server and session it just spawned.
// The `remain-on-exit` and `mouse` pins are correctness dependencies and fail the boot;
// the geometry pins, which include the `window-resized` unpin, are non-fatal.
// Boot options never re-apply to an already-up session.
// That is why the attach pre-flight re-pins the geometry options itself.
// Assumes the op lock is already held.
func (e *Engine) pinBootOptionsLocked(mouse, shellPath string) error {
	// remain-on-exit keeps a pane whose command exits around as pane_dead=1 instead of vanishing.
	// A vanished pane would also kill the session if it were the last pane.
	// Dead-pane detection depends on this option.
	if err := e.tmux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		return fmt.Errorf("set remain-on-exit: %w", err)
	}
	// Pin the mouse mode in both directions.
	// This call runs on every fresh boot, even to set "off",
	// so the live mouse state does not depend on the tmux backend's default.
	if err := e.tmux.run("set-option", "-g", "mouse", mouse); err != nil {
		return fmt.Errorf("set mouse: %w", err)
	}

	// The geometry pins are non-fatal.
	// The status and window-size options are geometry-quality settings.
	// When they are absent, tmux falls back to its own proportional rescale, and the session still works.
	// psmux's support for both is unverified in this repo,
	// so a capability reed cannot confirm must not take the boot down.
	e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()), shellPath)
	return nil
}

// serverSpawnArgv is the argv of the invocation that starts this hub's tmux server with its first session.
// debugArgs are tmux GLOBAL flags (e.g. -v/-vv) and lead the argv.
// Everywhere but Windows, `-f /dev/null` follows them, so the server reads none of the operator's `~/.tmux.conf`;
// a running server ignores the flag.
// `-c` pins new-session's pane default cwd to the told pane spawn directory;
// the server process's own cwd has moved to the logs directory.
// The configured shell is the first pane's trailing command.
// Off Windows, three `set-option` commands chain after it in the same invocation.
// They pin the new session `destroy-unattached off`, tag it `@lym_reed_session`, and set `@lym_follow on`,
// so all of this happens before any operator config could act.
// On Windows the follow option stays unset, which reads on.
func (e *Engine) serverSpawnArgv(goos string, debugArgs []string) []string {
	argv := append([]string{}, debugArgs...)
	if goos != "windows" {
		argv = append(argv, "-f", os.DevNull)
	}
	argv = append(argv,
		"-L", e.Socket(),
		"new-session", "-d", "-s", e.SessionName(),
		"-c", e.geom.PaneCwd,
		"-x", strconv.Itoa(e.cfg.Width),
		"-y", strconv.Itoa(e.cfg.Height),
		e.cfg.Shell,
	)
	if goos == "windows" {
		return argv
	}
	target := exactSessionWindowTarget(e.SessionName())
	return append(argv,
		";", "set-option", "-t", target, "destroy-unattached", "off",
		";", "set-option", "-t", target, reedSessionOption, "on",
		";", "set-option", "-t", target, followSessionOption, "on",
	)
}

// touchDiscoverSignal creates or truncates the told discover signal file, waking the watchdog daemon's discovery loop.
// It does nothing when no path is told, and a failure is logged at Warn, never failing the boot.
func (e *Engine) touchDiscoverSignal() {
	path := e.geom.DiscoverSignalPath
	if path == "" {
		return
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		logger.Warn("reed: could not touch the discover signal", "socket", e.Socket(), "path", path, "err", err)
	}
}

// writeDiscoverStop writes DiscoverStopMarker as the whole content of the told discover signal file.
// That asks the hub's watchdog daemon to stop.
// The write is atomic: a temp file is renamed onto the path,
// so the daemon's watch never sees the file created empty before the marker lands.
// It does nothing when no path is told, and a failure is logged at Warn, never failing Down.
func (e *Engine) writeDiscoverStop() {
	path := e.geom.DiscoverSignalPath
	if path == "" {
		return
	}
	if err := fsx.AtomicWriteBytes(path, []byte(DiscoverStopMarker)); err != nil {
		logger.Warn("reed: could not write the discover stop", "socket", e.Socket(), "path", path, "err", err)
	}
}

// stripTraceID removes LYM_TRACE_ID from env before the tmux server inherits it.
// The server is a long-lived singleton reattached by unrelated invocations.
func stripTraceID(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.SplitN(entry, "=", 2)[0] == logger.TraceIDEnv {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// stripAgentNameEnv removes LYM_STRAND_NAME and LYM_PARENT from env before the tmux server inherits them.
// A server booted from inside a strand would otherwise hand that strand's name and parent to every pane it later creates,
// and a pane told no parent exports none of its own, so it would escalate to the booting strand's parent.
func stripAgentNameEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key := strings.SplitN(entry, "=", 2)[0]
		if key == agentname.StrandNameEnv || key == agentname.ParentEnv {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// upLocked ensures the server and session exist and reports whether this call booted them.
// It must run with the op lock held.
func (e *Engine) upLocked() (UpResult, bool, error) {
	var result UpResult
	booted, stripped, err := e.ensureServerAndSessionLocked()
	if err != nil {
		return result, booted, err
	}

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		return result, booted, err
	}

	// On a server rebirth the reborn session reuses pane ids, since the initial pane is %1 again.
	// A persisted binding would then be mistaken for a live strand, so every binding is cleared:
	// a just-booted session hosts none of the prior strands.
	// Up leaves them not-live, and Resume relaunches them.
	// The stripped env keys are stamped for diagnosis: reed.json records what the server spawn removed.
	// The Selvage pane id is cleared for the same reason, since a reused id would look like the live Selvage pane;
	// ensureSelvagePaneLocked then rebuilds it fresh.
	if booted {
		clearAllPaneBindings(st)
		st.StrippedEnv = stripped
		clearSelvagePaneBinding(st)
	}

	if err := e.ensureSelvagePaneLocked(st); err != nil {
		return result, booted, err
	}
	e.markResolvedStrandWindowLocked(st)

	if _, err := e.reconcileApplyPersistLocked(st); err != nil {
		return result, booted, err
	}

	// Selvage is not a strand and never appears in st.Strands, so len(st.Strands) excludes it.
	// Do not add Selvage to st.Strands: it must never be counted as a strand.
	result = UpResult{Session: e.SessionName(), Socket: e.Socket(), Strands: len(st.Strands)}
	return result, booted, nil
}

// withBootOpLock runs fn under the operation lock, for an op that always boots.
// Before fn's first tmux round trip, it refuses these inputs:
// a boot-time config error, an over-long socket path, an unresolvable shell, or an unusable `tmux_conf`.
// The boot refuses the same inputs on the paths that reach it without this wrapper.
func (e *Engine) withBootOpLock(fn func() error) error {
	return e.withOpLock(func() error {
		if _, _, err := e.validateBootConfig(); err != nil {
			return err
		}
		if _, _, err := e.preflightBootHost(); err != nil {
			return err
		}
		return fn()
	})
}

// Up ensures the server and session exist.
// It never launches strands, and it does not rebuild content after a server restart.
func (e *Engine) Up() (UpResult, error) {
	var result UpResult
	err := e.withRevivalFirst(e.withBootOpLock, func() error {
		var err error
		result, _, err = e.upLocked()
		return err
	})
	return result, err
}

// ensureSessionLocked boots this worktree's session only when there is nothing usable to attach to.
// It reports whether it created a session.
// When the session is already usable, it returns (false, nil) at once, having done nothing else:
// no config validation, no reconcile, no state read or write.
// The warm path must not reach the boot path, for two reasons.
// The boot path's reconcile puts every live non-exempt pane on the kill list while the Selvage pane is alive.
// The boot path also runs its config validation ahead of the already-up check.
// Otherwise it returns the booted flag of its boot call unchanged, never a constant true:
// the session can come up between the two probes, as when a sibling `lym reed up` runs in another terminal;
// a constant true would make the caller's attribution log claim a spawn that never happened.
func (e *Engine) ensureSessionLocked() (bool, error) {
	_, usable, err := e.sessionSubstrateLocked()
	if err != nil {
		return false, err
	}
	if usable {
		return false, nil
	}
	_, booted, err := e.upLocked()
	return booted, err
}

// EnsureSession boots this worktree's session only when there is nothing usable to attach to.
// It reports whether it created a session.
// On the warm path it reads no state, so it raises none of reed's state-level refusals.
// A caller that needs those refusals must make its own Status call.
func (e *Engine) EnsureSession() (booted bool, err error) {
	err = e.withRevivalFirst(e.withOpLock, func() error {
		var innerErr error
		booted, innerErr = e.ensureSessionLocked()
		return innerErr
	})
	return booted, err
}

// Resume boots the server and session if they are absent, then reconciles stale bindings.
// It drops the non-live strands whose done-when paths all exist and relaunches the other non-live strands.
// It re-applies the layout.
func (e *Engine) Resume() (ResumeResult, error) {
	var result ResumeResult
	err := e.withRevivalFirst(e.withBootOpLock, func() error {
		dropped := 0
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
		// Clear the Selvage binding too, for the same reason.
		// A reused pane id would otherwise pass for the still-live Selvage pane.
		// Selvage is not a strand binding, so the strand clear does not reach it.
		// ensureSelvagePaneLocked then rebuilds it before any strand replay runs.
		if booted {
			clearAllPaneBindings(st)
			st.StrippedEnv = stripped
			clearSelvagePaneBinding(st)
		}

		if err := e.ensureSelvagePaneLocked(st); err != nil {
			return err
		}
		e.markResolvedStrandWindowLocked(st)

		live, windows, err := e.listReconcilePanesLocked(st)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		killed, err := e.reconcileLocked(st, live, windows)
		if err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if len(killed) > 0 {
			live, windows, err = e.listStrandAndWindowPanes(st)
			if err != nil {
				return fmt.Errorf("list panes after reconcile: %w", err)
			}
		}
		// Liveness is read from engineAliveIDSet.
		// A strand bound to a dead-but-present pane, such as the kept sole dead pane, is not live.
		// So is one bound to a shell-only pane past the launch grace; both must be relaunched.
		// The present set includes the own-window panes, so a live one is never relaunched beside itself.
		toLaunch, toDrop, toHold := planResumeLaunches(st.Strands, e.engineAliveIDSet(presentPanes(st.Strands, live, windows)), pathExists, e.pairPaused())

		// A finished strand has no live pane,
		// so dropping it kills nothing.
		if len(toDrop) > 0 {
			dropped = dropStrands(st, toDrop)
			if err := SaveState(e.stateDir(), st); err != nil {
				return fmt.Errorf("persist dropped strands: %w", err)
			}
		}

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
			// Persist after each launch, before the apply below, so the pane is tracked at once.
			// If a later launch or apply fails, the pane is already recorded:
			// the next resume then neither reaps it as untracked nor launches it twice.
			if err := SaveState(e.stateDir(), st); err != nil {
				return fmt.Errorf("persist strand: %w", err)
			}
			// Re-apply the layout after each launch.
			// Consecutive splits without a re-apply halve the same target pane until tmux silently refuses to split it;
			// a re-apply re-stretches the bottom or active pane,
			// so the next launch splits the tallest pane the policy just sized.
			if _, err := e.reconcileApplyPersistLocked(st); err != nil {
				return err
			}
			launched++
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		var held []string
		for _, s := range toHold {
			held = append(held, s.Name)
		}
		result = ResumeResult{Session: e.SessionName(), Resumed: launched, Dropped: dropped, Held: held}
		return nil
	})
	return result, err
}

// Down tears this worktree's session down and waits for async teardown to finish.
// It kills only the session, never the server, because other worktrees share the server.
// It waits for the server and pane-child processes to release their resources.
//
// Down is also the only lym-only escape from the foreign-session refusal in `generation.go`.
// It loads no state and so never reaches that check.
// This matters for an operator who can run lym but not raw tmux.
// Such an operator may be in a CI-like environment or a sandboxed agent.
// The refusal's own message names only a `tmux kill-session` remedy.
//
// The recorded session may be this worktree's former session after a rename.
// It may also be a sibling worktree's live session after a hand-copied .lym.
// Reed cannot tell those apart, so Down never kills it.
// Down still deletes the state file, which keeps down the idempotent escape it is.
// Naming the abandoned session in the result and at Warn keeps that deletion from being a silent loss.
func (e *Engine) Down() (DownResult, error) {
	var result DownResult
	err := e.withOpLock(func() error {
		start := time.Now()
		defer func() {
			logger.Info("reed: down complete", "socket", e.Socket(), "session", e.SessionName(), "duration", time.Since(start))
		}()
		// Grab the server's OS pid while the session can still be queried.
		// It is the only reliable death signal, because tmux's CLI cannot report "no server":
		// list-sessions exits 0 with empty output, and kill-server exits 0 whether or not a server holds the socket.
		serverPID := e.serverPIDLocked()

		// Capture this session's pane process subtrees before kill-session, while the panes still exist to be listed.
		// Each subtree is the shell tmux ran in the pane plus its descendants;
		// on Windows the process holding the worktree directory is a deeper descendant of the pane pid.
		// Reaping them keeps Down's "no stray state" guarantee at the pane level.
		// Roots come from alive panes only: a dead pane's recorded #{pane_pid} names a process that already exited.
		// Expanding a recycled pid would make this reap wait on, and then SIGKILL, an unrelated process tree.
		panePIDs := e.paneProcessTreePIDsLocked()

		// Ignore the error: the session may already be gone, and Down must stay idempotent either way.
		// The exact-match target is load-bearing on this ignored-error path:
		// a bare -t name prefix-matches once this session is gone,
		// so a second down would kill a sibling worktree's session whose name shares its prefix.
		// The discard is logged at Debug, so it is observable at the step-trace level without raising teardown to a Warn.
		if err := e.tmux.run("kill-session", "-t", exactSessionTarget(e.SessionName())); err != nil {
			logger.Debug("reed: best-effort kill-session failed (session may already be gone)", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}

		// Tidy the server only if no sessions remain.
		// An empty list-sessions covers both zero sessions and no server, because tmux does not distinguish them.
		// kill-server is harmless in both cases.
		// An errored list-sessions means the socket holder is unreachable;
		// a zombie server cannot host healthy sibling sessions, so it is torn down too rather than left holding the socket.
		var serverErr error
		if out, err := e.tmux.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...); err != nil || strings.TrimSpace(out) == "" {
			result.EndedServer = true
			logger.Info("reed: tearing down tmux server", "socket", e.Socket(), "serverPID", serverPID)
			// Written before kill-server so the stop is asked for whether or not the server then confirms gone.
			e.writeDiscoverStop()
			// kill-server is best-effort: a server that does not confirm gone is reported at Warn by the check below,
			// so a failed kill-server needs only a Debug line.
			if err := e.tmux.run("kill-server"); err != nil {
				logger.Debug("reed: best-effort kill-server failed", "socket", e.Socket(), "serverPID", serverPID, "err", err)
			}
			ensureGone := e.ensureServerGone
			if ensureGone == nil {
				ensureGone = e.ensureServerGoneLocked
			}
			serverErr = ensureGone(serverPID)
			if serverErr != nil {
				logger.Warn("reed: server did not confirm gone after kill-server", "socket", e.Socket(), "serverPID", serverPID, "err", serverErr)
			}
		}

		// ALWAYS reap this session's pane child subtree, even when the server teardown above hit trouble.
		// A slow or failed server death must never skip the pane reap:
		// a skipped reap leaks pane children that keep the worktree directory busy.
		// The server's own cwd is not this worktree, so it does not hold the directory busy,
		// but a lingering server process is still a leak worth reaping.
		// kill-session and kill-server terminate pane children asynchronously, so force-kill any that outlive the deadline.
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

		// With reed.json gone nothing can relaunch a strand, so no launch script has a strand left to belong to.
		// Best-effort: Down stays idempotent.
		if err := os.RemoveAll(launchScriptDir(e.stateDir())); err != nil {
			logger.Warn("reed: could not delete launch script directory", "path", launchScriptDir(e.stateDir()), "err", err)
		}

		result.Session = e.SessionName()
		result.AbandonedSession = abandoned
		return nil
	})
	return result, err
}

// sessionlessSocketHolderPersists reports whether a socket holder with no session outlives staleSocketGrace.
// The grace keeps it from reaping a sibling worktree's just-spawned server.
// A holder whose global user option @lym_test_server answers "on" is never stale.
// Only the test kit's tmux config sets that option, on a server it pre-starts and keeps alive with no session;
// reed's own boots never set it.
// Bound: an operator config that sets the option disables the reap for its own servers.
func (e *Engine) sessionlessSocketHolderPersists() bool {
	if out, err := e.tmux.output("show-options", "-gqv", "@lym_test_server"); err == nil && strings.TrimSpace(out) == "on" {
		return false
	}
	deadline := time.Now().Add(staleSocketGrace)
	for {
		// A socket that lists sessions hosts a healthy shared server — never
		// in scope for reaping, no matter what else is pending.
		if out, err := e.tmux.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...); err == nil && strings.TrimSpace(out) != "" {
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

// serverPIDLocked returns the pid of the tmux server holding this engine's socket, or 0 when no server answers.
// It must run before kill-session when the caller intends to wait on the server.
//
// The result is the server's pid, not this session's.
// #{pid} is server-global, and display-message exits 0 for a -t target naming no session.
// So on a socket shared with a sibling worktree, the pid is that server's even when this session is absent.
// Callers spend the value only once the session list comes back empty.
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

// sessionReapRootsLocked returns the reap roots of this session:
// the #{pane_pid} of every pane, in any window, that is present and still running.
// A dead pane's recorded pid may name a recycled process, so it is left out.
// It reads no reed state, so Down reaps over a corrupt state file too.
// It returns nil on failure and must run before kill-session while panes exist.
func (e *Engine) sessionReapRootsLocked() []int {
	live, err := reapSessionPanes(e.tmux, e.SessionName())
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

// processExitPoll is how often a pid's liveness is re-probed while waiting for it to exit.
// It polls rather than blocking on the kernel, because every pid reed waits on belongs to another process's child tree.
const processExitPoll = 50 * time.Millisecond

// ensureServerGoneLocked guarantees no tmux process remains on this engine's socket after kill-server.
// It waits for asynchronous teardown, then force-reaps any process still on the socket.
// Once no process remains, it removes that server's socket file, on both the graceful and the force-reap path.
func (e *Engine) ensureServerGoneLocked(serverPID int) error {
	_ = waitProcessExit(serverPID, reapExitTimeout)
	if len(e.serverProcessesOnSocket()) == 0 {
		e.removeSocketFile()
		return nil
	}
	if err := e.reapSocketProcesses(); err != nil {
		return err
	}
	e.removeSocketFile()
	return nil
}

// removeSocketFile removes the socket file the just-killed server left behind.
// A failure is logged and never fails the teardown.
func (e *Engine) removeSocketFile() {
	if err := removeStaleSocket(socketDirFromEnv(), e.Socket()); err != nil {
		logger.Debug("reed: could not remove the torn-down server's socket file", "socket", e.Socket(), "err", err)
	}
}

// reapPaneChildren waits for pane child processes to exit, force-killing
// stragglers. Pane-destroying ops must reap children to avoid worktree dir locks.
// pids is the descendant closure snapshotted before the kill.
// Once those are reaped, every other process still in a session one of them led is reaped too:
// a pane shell can fork a command after the snapshot,
// and a command it has not yet moved into the terminal's foreground can miss the hangup.
func reapPaneChildren(pids []int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	reapPIDsBy(pids, deadline)

	snapshotted := make(map[int]bool, len(pids))
	for _, pid := range pids {
		snapshotted[pid] = true
	}
	var late []int
	for _, pid := range sessionMemberPIDs(pids) {
		if !snapshotted[pid] {
			late = append(late, pid)
		}
	}
	reapPIDsBy(late, deadline)
}

// reapPIDsBy waits until deadline for each of pids to exit, force-killing and confirming each one still up when it passes.
func reapPIDsBy(pids []int, deadline time.Time) {
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if err := waitProcessExit(pid, time.Until(deadline)); err == nil {
			continue
		}
		// The graceful window elapsed with the process still up:
		// a pane child that ignores or is out of reach of tmux's own SIGHUP cascade (a trapped SIGHUP, a detached session).
		// Force-kill it and confirm.
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

// reapSocketProcesses force-terminates every tmux process on this engine's socket and confirms they're gone.
// Zombie servers and helpers that ignore socket-routed kill-server need this.
func (e *Engine) reapSocketProcesses() error {
	_ = e.tmux.run("kill-server")
	for _, pid := range e.serverProcessesOnSocket() {
		// Best-effort per pid: a socket holder that exited between the scan and this kill is what the reap wants.
		// waitServerProcessesGone decides the outcome.
		if err := proc.KillPID(pid); err != nil {
			logger.Debug("reed: best-effort kill of socket holder failed", "socket", e.Socket(), "pid", pid, "err", err)
		}
	}
	return e.waitServerProcessesGone(reapExitTimeout)
}

// waitProcessExit blocks until the process named by pid is no longer running, or errors after timeout.
// It is needed because tmux's kill-server and kill-session both terminate their process trees asynchronously.
//
// Liveness is polled via proc.IsAlive rather than waited on with os.Process.Wait.
// Every pid reed waits on (the tmux server and each pane's process subtree) is a child of tmux, not of this process.
// os.Process.Wait on a non-child returns ECHILD at once.
// A Wait-based check would therefore report every such pid as already exited.
//
// A pid recycled after the caller's snapshot reads as alive here and burns the full timeout.
// reapPaneChildren then force-kills it along with its whole subtree.
// The caller's snapshot discipline therefore bounds this.
// Both reap paths take their descendant-closure roots from panes present and not dead at snapshot time.
// safeReapRoot in strand.go enforces that rule: tmux keeps reporting a dead pane's #{pane_pid} after the process exits.
//
// Honest limit: proc.IsAlive is a signal-0 probe, so a not-yet-reaped zombie reads as alive here.
// That window is short: every pid reed waits on is a child of the tmux server, which reaps its own children.
// Once the server dies, the survivors are reparented to init and reaped there.
// A caller must not read this function as proof that the process's resources are released.
// It shows only that the pid no longer answers.
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
// stateReadable tells apart a state that reed read and found empty from one that reed could not read at all.
// strandCount alone cannot express that.
// An unreadable file yields a count of zero, which would read as an empty worktree and send the operator to `up`.
// `up` then fails with the corrupt-file error (see unreadableStateError).
// Claiming that nothing is persisted when reed cannot tell is wrong.
// An unreadable file therefore gets its own branch.
// It points at the same two remedies the load error names, rather than inventing a third.
func noSessionMessage(strandCount int, stateReadable bool) string {
	if !stateReadable {
		return `no reed session, and reed's persisted state could not be read; run "lym reed down" to clear it, or "lym reed up" for the full diagnosis`
	}
	if strandCount <= 0 {
		return `no reed session; run "lym reed up"`
	}
	return fmt.Sprintf(`no reed session (%d strands persisted); run "lym reed resume" to rebuild, or "lym reed up" for a bare substrate`, strandCount)
}

// requireSessionLocked returns an actionable error when this worktree's session does not exist.
// The error also says whether resume would have content to rebuild.
//
// The foreign-session diagnosis is consulted here, not left to the two booting verbs.
// It covers a worktree renamed while its session was up, and a .lym copied between worktrees of one hub.
// Both leave this worktree's session absent, so every non-booting verb lands in this function.
// The plain no-session text would then name `lym reed resume` and `lym reed up` as remedies.
// Both refuse for a reason that text does not mention.
//
// It costs no tmux round trip on the healthy path.
// refuseLiveForeignSessionLocked returns at once when the recorded session name is empty or this worktree's own.
// Every state file reed writes meets that condition.
func (e *Engine) requireSessionLocked() error {
	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if up {
		return nil
	}

	// st.SelvagePaneID is a separate field and never part of Strands, so len(st.Strands) excludes Selvage.
	strandCount := 0
	st, loadErr := LoadState(e.stateDir())
	if st != nil {
		strandCount = len(st.Strands)
		if err := e.refuseLiveForeignSessionLocked(st.PaneGeneration); err != nil {
			return err
		}
	}
	// A missing file is readable and empty, not unreadable: a new worktree has no reed.json
	// and still gets the plain "run lym reed up" text.
	return noSessionError{msg: noSessionMessage(strandCount, loadErr == nil)}
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

		live, windows, err := e.listStrandAndWindowPanes(st)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		present := presentPanes(st.Strands, live, windows)

		// Strand liveness comes from engineAliveIDSet, not liveIDSet:
		// a strand whose pane is dead, or shows only its start shell past the launch grace, is not live.
		// The operator asks status whether the strand's engine is running, not whether tmux still lists a pane for it.
		// Present, OwnWindow and PanePID stay pane-level.
		aliveIDs := e.engineAliveIDSet(present)
		presentIDs := liveIDSet(present)
		alivePIDs := make(map[string]int, len(present))
		for _, p := range present {
			if !p.Dead {
				alivePIDs[p.ID] = p.PID
			}
		}
		// This loop iterates st.Strands only;
		// Selvage is not a strand and is never reported here (see ReedState.SelvagePaneID).
		// Status still succeeds when st.Strands is empty but Selvage is alive, because the session is up.
		// Appending a Selvage row here would be wrong.
		elsewhere := e.strandsElsewhereLocked(st.Strands, windows)
		strands := make([]StrandStatus, len(st.Strands))
		for i, s := range st.Strands {
			strands[i] = StrandStatus{GUID: s.GUID, Name: s.Name, PaneID: s.PaneID, PanePID: alivePIDs[s.PaneID], Live: aliveIDs[s.PaneID], Present: presentIDs[s.PaneID], OwnWindow: isOwnWindow(s.Display), Elsewhere: elsewhere[s.PaneID], Retiring: s.Retiring, Color: e.withColor(s).Color, LymBin: s.LymBin, Provider: s.Provider}
		}

		result = StatusResult{Session: session, Socket: e.Socket(), Strands: strands}
		return nil
	})
	return result, err
}
