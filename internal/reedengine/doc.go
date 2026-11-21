// doc.go carries the package-level godoc comment for reedengine.
// It holds no code, only the package's role and contract.

// Package reedengine is the domain kernel for lym's tmux window manager.
// It holds the tmux subprocess overlay, strand bookkeeping, persisted state and config.
// Its operations layer holds the lifecycle verbs that compose them.
// It is the "dumb carrier" for its caller's strand data:
// it stores every field a caller writes into a strand and reads none of them semantically.
// A strand has no domain `type` field:
// `cmd`, `resumeCmd`, `Provider` and `CreelHooks` are opaque values that reedengine never parses or branches on.
// `--role` and `--round` are formatting-only inputs, consumed at add-time to fill the strand-name template.
//
// reedengine imports internal/reedengine/render (the pure display-vocabulary leaf) and render never imports it back.
//
// # Told inputs
//
// reedengine is told its geometry as a Geometry value (geometry.go) and derives none of it.
// internal/lymcwd is absent from its direct production imports (the Told-Geometry Invariant).
// hubgeom.ReedGeometry builds a Geometry from a resolved *lymcwd.Location for every hub-mode caller.
// The one fabricengine call, fabricengine.HubScratchDir in SpawnWatchdog, takes hubPath as a parameter.
// The environment keys the tmux server spawn must not inherit are told as an EnvScrub passed to New;
// reedengine carries no provider's names itself.
// The spawn order of a hub's worktrees is told as Geometry.SpawnOrder, nil in standalone mode.
//
// # Invariants
//
// Exactly one named tmux server per hub.
// The server name is derived from the hub path (ServerName), so every worktree under one hub shares that server.
//
// Every session carries exactly one permanent pane beyond its strands: Selvage (ReedState.SelvagePaneID).
// Selvage is never a Strand (Shared Decision header-is-not-a-strand).
// It is excluded from strand accounting and is never the preferred split target.
// It is also outside both halves of reconcile's kill schedule.
// Removing a session's last strand therefore never destroys the session.
// Selvage runs the operator's configured shell, which reed never writes to, clears or sends keys to.
// It is always the bottom-most pane of the shell window.
// Every split that creates or rebuilds it targets the bottommost live pane.
// A Selvage pane whose shell died is kept by reconcile as a corpse and healed by the next Up or Resume.
// These rules live here rather than in a PATTERN entry because they describe this package's own design.
// selvagepane_enforcement_test.go is the mechanical check.
//
// Every strand pane reed creates resolves `lym` to the binary that spawned it.
// launchStrandLocked prepends that binary's directory to the pane's PATH and exports LYM_BIN.
// It then exports `proc.BuildServerOffEnv` and the variables Geometry.PaneEnv tells.
// The composition lives in panebin.go and is reached from the one pane-creation chokepoint,
// so the property holds by construction for every AddStrand caller, Resume's replay included.
// panebin_enforcement_test.go fails on a split-window or new-window call outside the chokepoint;
// its named allowlist is the only exception.
// Every key binding that runs `lym` names the pinning binary's absolute path from selfexec.ResolveLymBinary.
// So no prelude, binding, button or Slack line names a running test binary.
//
// A strand's full name is formed once, in AddStrand under the state lock, from the told prefix and the role.
// The name is a birth attribute and the only lookup key; a lookup matches the full name or its role segment.
// A role already held is numbered `-N`, and an explicit name that is held refuses.
// An empty told shortname refuses before anything boots.
// An ordinary add reclaims the name it asks for only from a holder that is dead, not hidden, childless and not paused.
// The name is mirrored outward, never read back as truth.
//
// # Strand lifecycle
//
// A strand's Retiring flag is reed's own bookkeeping: `reed remove --detach` sets it through Engine.MarkRetiring.
// A strand's DoneWhen list is the one place reed learns a strand finished.
// Resume drops a non-live strand whose list is non-empty and whose every path exists.
// It reports the drops as ResumeResult.Dropped.
// Resume relaunches nothing for a held strand or a pair under a live pause entry, and reports them as ResumeResult.Held.
// EndStrandProcess marks a strand held and keeps its record.
// RelaunchStrand, ReleaseStrandHold and RemoveStrand are the only ways to end the hold;
// reed never clears one on its own.
//
// Strand liveness (StrandStatus.Live) holds only for a pane that is listed in this session and is not dead.
// The pane must also not show only the shell it was started with.
// engineAliveIDs is the one predicate.
// Every pane on Windows reads live.
// An engine that never takes the foreground reads dead, and a shell-only pane younger than the launch grace reads live.
//
// # Revival
//
// A restarted server's sessions come back in spawn order (revive.go).
// A booter whose reed.json records a session that is not live runs Revive for each earlier Geometry.SpawnOrder entry.
// Revive runs first, outside the booter's own lock; the booter then runs its own step again.
// No two op locks are ever held at once.
// Revival is for the booter's own recorded session only, never a first boot, and it starts no strand.
// An entry whose Revive fails is logged and skipped.
//
// # Placement and windows
//
// Every placed strand gets a window of its own, a group window or the overview.
// The shell window holds Selvage alone.
// The anchors are own-window, overview and hidden (internal/reedengine/render); a hidden strand never owns a pane.
// The retired anchors `below-parent` and `top` read as own-window on decode, and a write refuses them.
// Reed computes no layout in the shell window; the only layout it issues is the even layout of a group window.
// Windows sit in this index order:
//
//   - overview windows by ascending order key
//   - agents' own windows in start order
//   - the window named shell
//   - hidden windows
//
// Reed ends only a window carrying @lym_own and reuses only one carrying @lym_hidden; a window opened by hand stays.
//
// The strands live in one window of the session.
// Every op addresses it by its id ("@<n>").
// No op uses the session's current window (windowtarget.go's TmuxCmd.strandWindowTarget).
//
// Strands declaring a window group (Display.Window, Display.WindowSize) share one window.
// That holds while the declared size is at most window_share_max.
// ReedState.WindowGroups records the choice until the group's last strand goes,
// so a later window_share_max edit never splits a group.
// A strand declaring another size than the record's is refused.
// UpdateStrand refuses a move into or out of the anchor, and a change of an overview strand's order.
// An invalid window_split or window_share_max never refuses a removal; it is reported in Removed.ConfigWarning.
//
// OpenWindow opens a detached, hidden, named window running a `lym` command in the worktree's reed session.
// It never adopts, kills or replaces an own window or a window the operator opened by hand.
//
// # Focus
//
// The follow state is the session option @lym_follow; it is off only while it reads off.
// An automatic window switch happens only at three points:
// attach, a new own-window strand's window, and a viewed window ending.
// Each is decided from reed state, window ids, strand roles, pane liveness and the follow state.
// None reads @lym_unseen, @lym_hidden or a wait mark.
// A viewer on another window is never moved.
// FollowRun is the one engine operation that turns follow on, beside the engine-less FollowSession.
// Opening a window never changes the follow state.
//
// # Engine-less functions
//
// Each of these addresses a told server by socket path and reads no reed state:
// SwitchClient, FollowSession, RunButton, EndSessionByName, ReapSession and StopServer.
// SwitchClient is the only reed code that issues switch-client.
// RunButton runs only the entry the session's @lym_button_spec holds at the clicked index,
// and its approval prompts re-call the verb with the entry's run-text hash, so no run text crosses tmux's parsing.
//
// # Display-only options
//
// The wait, check, @strand, @strand_color, @lym_unseen and @lym_orch_dir options are display only (PATTERN-reed-panes).
// A Go decision reads only these options:
//
//   - pane options: @lym_strand_name and @lym_rc
//   - session options: @lym_follow, @lym_button_spec, @lym_button_dir and @lym_button_answer
//
// The navigation bindings (bindings.go) are root-table bindings on the hub's reed server.
// They are pinned at boot and at every attach pre-flight, and every one is non-fatal.
// They take Alt+Up, Alt+Down, Alt+Left, Alt+Right and the left status click from every pane on the server.
// An older build never re-pins over a newer one's stamp in @lym_bindings_build.
//
// # Daemon
//
// The hub watchdog daemon runs a name-repair tick (namerepair.go), gated by the reed.yaml key `watchdog`.
// It types a session-name rename only into an idle pane with no active operator.
// The discover signal DiscoverSignalFileName, at Geometry.DiscoverSignalPath, is touched by every cold session boot.
// `Down` writes DiscoverStopMarker into it before kill-server to ask the daemon to stop.
//
// # Multiplexer contract surface
//
// This package assumes its configured binary honors the tmux-derived wire contract documented here.
// The binary is psmux on Windows and tmux elsewhere.
// contract_integration_test.go's TestMultiplexerContract exercises this surface against a running binary.
//
// Tmux floor: off Windows, every tmux command first checks the version against `toolfloor.TmuxFloor`.
// A below-floor tmux returns `*toolfloor.FloorError`.
//
// Operator config: a server reed starts reads no `~/.tmux.conf` (`-f /dev/null`), except on Windows.
// The reed.yaml key `tmux_conf` names a file sourced once per server start;
// the boot fails when that file ends the session or unpins `destroy-unattached` or `exit-unattached`.
//
// Session targeting: every -t argument that names a session is passed in an exact-match form.
// Session targets use "=<name>"; window and pane targets use "=<name>:".
// tmux falls back to prefix matching a bare name.
// On the shared per-hub server a bare name can therefore silently address a sibling worktree's session.
// contract_integration_test.go's TestExactSessionTargetsNeverPrefixMatchSiblings pins both grammars.
// Pane-id (-t %N) targets are already exact and stay bare.
//
// Load-bearing behavioral assumptions:
//
//   - Silent split failure: split-window against a pane too small to split exits 0 and prints an existing pane's id.
//     Every split site checks that the returned pane id was absent from the pre-split live set
//     (validateSplitCreatedNewPane).
//   - Dead panes under remain-on-exit: a pane whose command exits stays enumerable (pane_dead=1).
//     A corpse is never reused as a strand's pane, because send-keys into a dead pane exits 0 while running nothing.
//   - Untracked-pane reap gate: every pane in a reed session is either Selvage or a bound strand's pane.
//     The reap runs at one chokepoint inside launchStrandLocked, and a dead Selvage id never authorizes sparing
//     the untracked set.
//   - Last-pane fate depends on the binary: killing a session's true last pane destroys the session on tmux
//     and corpses the pane on psmux.
//     RemoveStrand's post-kill re-probe therefore uses hasSession, never list-sessions.
//   - Told-geometry lifetime: a told Geometry is pinned for the process's life, so a `mv` of the worktree makes it stale.
//     withOpLock and withTryOpLock refuse when the told worktree root is no longer live.
//   - Silent session-name rewriting: tmux rewrites '.' and ':' to '_', doubles '\' and vis-encodes control
//     and invalid-UTF-8 bytes, with exit 0.
//     The session is then unreachable by the exact-match name that created it.
//     validateToldTmuxIdentity refuses such a name at withOpLock rather than sanitizing,
//     because sanitizing would map sibling worktrees "svc.v2" and "svc_v2" onto one session.
//     contract_integration_test.go's TestSessionNameRewriteIsSilentAndExactTargetsMissIt pins the wire behavior.
//     ServerName substitutes separators out at the derivation instead of refusing.
//   - The -l leading-dash send-keys bug: send-keys -l silently drops a '-'-prefixed literal,
//     so sendKeysLiteralArg prefixes a space onto it.
//   - Pane ids are server-global and not stable across a server rebirth (generation.go):
//     the counter restarts at %0, so a persisted pane id identifies a pane only within one server generation.
//     reed.json records the generation (session_id, server pid, session_created),
//     and the engine discards every binding whose generation is not the live one.
//     A recorded session still running on this socket under another name is refused rather than cleared.
//   - display-message does not exit 1 for an absent session (generation.go).
//     has-session and list-panes exit 1 for a `-t` target that names no session.
//     display-message exits 0 and expands every session-scoped format to empty, while the server-global #{pid} still fills.
//     It does not fall back to a current session when several exist.
//     The generation probe's absent case therefore surfaces as parsePaneGeneration's empty-field rejection,
//     not as a tmux error.
//     An error from that probe means the round trip could not be run, never that the session is gone.
//   - Duplicate-pane-cell session destruction: select-layout accepts a layout that names one pane twice
//     and destroys every pane the short cell list no longer covers.
//     The engine clears such bindings from a corrupt persisted table at the single load chokepoint.
//   - Async kill-server: no probe distinguishes "no server" from "server dying asynchronously",
//     so Down waits on the OS process exiting and trusts no CLI exit code as a death signal.
//   - Mouse boot pin: "-g mouse" is pinned at boot to the configured value, default "on".
//     "off" is not neutral: the terminal's wheel translation then types arrow keys into the agent.
//   - Window size: window-size is "latest" and is not changed, made configurable or conditioned on the client count.
//     With two clients of different sizes some client is mismatched under every policy,
//     and "latest" gives the artifact to the client the operator is not using.
//     The dots are painted by tmux itself, so capture-pane never sees them.
//   - Geometry option pins are non-fatal, unlike the remain-on-exit and mouse pins,
//     which are correctness dependencies (Shared Decision geometry-tmux-failures-are-non-fatal-everywhere).
//     set-hook and list-clients are absent from requiredSubcommands for the same reason.
//   - The attach argv separates "; select-window" as a literal one-character ";" element, never "\;",
//     because exec.Command passes argv to the child with no shell to unescape it.
//
// StopServer stops a hub's whole tmux server from the hub's path alone, whether or not the hub's directory still exists.
// It reports false when no live server answered on the key.
package reedengine
