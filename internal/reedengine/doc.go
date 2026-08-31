// doc.go carries the package-level godoc comment for reedengine.
// It holds no code — its only job is documenting the package's role and contract in one place a
// reader finds first.

// Package reedengine is the domain kernel for lyx's tmux window manager: the
// tmux subprocess overlay, strand bookkeeping, persisted state, config, and
// (in the operations layer) the lifecycle verbs that compose them. It is the
// "dumb carrier" for its caller's strand data — reedengine stores every field
// a caller writes into a strand and reads none of them semantically. There is
// deliberately no domain `type` field on a strand: `cmd`/`resumeCmd` are
// opaque strings reedengine never parses or branches on, and `--role`/`--round`
// are formatting-only inputs consumed once, at add-time, to fill the
// strand-name template — they are never persisted or read back.
//
// reedengine imports internal/reedengine/render (the pure display-vocabulary
// leaf) and maps its own persisted records down to render.Strand when
// computing a layout; render never imports reedengine, so the import graph
// stays acyclic (reedcli -> reedengine -> render).
//
// reedengine is told its geometry as a Geometry value (geometry.go) and
// derives none of it. internal/lyxcwd and internal/fabricengine are
// consequently absent from this package's DIRECT production imports;
// hubgeom.ReedGeometry is the hub-mode teller that builds a Geometry from a
// resolved *lyxcwd.Location for every hub-mode caller. Stated honestly, that
// is a direct-import fact and not a transitive one: internal/lyxcwd is still
// in `go list -deps ./internal/reedengine`, reached through internal/logger,
// so what the absence buys is that reed never RESOLVES its own coordinates —
// not isolation from the package that does.
//
// One additional invariant this package enforces: exactly one named tmux
// server per hub. The server name is derived deterministically from the hub
// path (ServerName), so every worktree under the same hub locates and shares
// the same tmux server rather than each spawning its own.
//
// A second package-level invariant: every session also carries exactly one
// additional, permanent pane beyond its strands — the header
// (ReedState.HeaderPaneID). It is a first-class construct, deliberately never
// a Strand (Shared Decision header-is-not-a-strand): it is excluded from
// strand accounting, from being the preferred split target, and from both
// halves of reconcile's kill schedule (see ensureHeaderPaneLocked in
// lifecycle.go, planPaneTarget in spawn.go, and planReconcile's
// exemptPaneIDs in reconcile.go for the three exclusion seams), so that
// removing a session's last strand can never destroy the
// session or corpse its sole pane — the header keeps the session (and the
// substrate the next add needs) alive no matter how many strands come and
// go. It boots alongside the session/initial pane on both Up and Resume, and
// Engine.ValidateHeader runs eagerly on every boot path so a bad header
// template surfaces loud before the pane is ever created, never silently.
// The header pane is created by a split-window call that carries the
// keepalive command (`lyx reed header --blocking`) as its own trailing
// shell-command argument, rather than by splitting a bare shell and typing
// the command into it afterwards with send-keys: the pane runs that command
// directly from birth, so it hosts no interactive shell for anything to echo
// the launch line into or read ~/.bashrc from. This makes the corpse-and-heal
// contract below actually work as documented: with the keepalive as the
// pane's own process, "set-option -g remain-on-exit on" corpses the pane the
// moment that process dies, where a surviving bash previously kept the pane
// alive and a dead header was silently mistaken for a working one. Under
// go test the pane still boots commandless — a bare shell, no split-window
// trailing argument and no send-keys — because headerLaunchLine
// (headerpane.go) returns "" whenever the boot decides to suppress the
// launch, which prevents os.Executable() from re-exec'ing the test binary
// and running its whole suite recursively; that decision now rides on
// Engine.suppressHeaderLaunch, an unexported field New initialises from
// testing.Testing(), rather than a testing.Testing() call hard-wired at the
// boot site. A header whose keepalive process dies (pane_dead=1) is
// deliberately kept as an enumerable corpse by reconcile — never killed
// there — and healed (corpse killed, a fresh header split back in at the
// physical top, carrying the same launch command on both the first attempt
// and any even-vertical-retile retry) by ensureHeaderPaneLocked on the next
// Up/Resume; planLayout only ever emits a header cell for a pane actually
// present in the window, so a stale HeaderPaneID can never put an absent
// pane's cell into select-layout's string (which a real tmux accepts and
// misassigns positionally rather than rejecting).
//
// The live-geometry rule: the render box a layout is computed against is no
// longer the config-pinned Width/Height. planLayout (apply.go) is always
// TOLD its box as an explicit render.Box parameter and queries nothing of
// its own — that separation is what lets its two callers disagree about
// where the box comes from without planLayout itself needing to know.
// applyLayoutLocked (apply.go) resolves the live box with
// liveBoxLocked (windowsize.go) — `display-message -p -t '=<session>:'
// '#{window_width} #{window_height}'` — and falls back to the configured
// cfg.Width/cfg.Height pair on any round-trip error or malformed answer.
// AttachArgv (attach.go) passes the attaching client's own told cols/rows
// and never calls liveBoxLocked, because at argv-build time the live window
// is still the PRE-attach size — querying it there would reintroduce the
// exact rescale this task removes (Shared Decision
// told-box-wins-live-query-is-the-fallback).
//
// # Multiplexer contract surface
//
// This package assumes its configured binary (psmux on Windows today, tmux
// on Linux in the deferred follow-up) honors the tmux-derived wire contract
// documented here. contract_integration_test.go's TestMultiplexerContract
// exercises this surface against a real, running instance of that binary —
// the canary for both version drift in the on-box binary and the eventual
// tmux swap, since the same test runs unmodified against whichever binary
// LoadConfig resolves.
//
// Pane enumeration: listPanes (overlay.go) always runs
//
//	list-panes -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"
//
// and parsePaneList (parse.go) parses each output line's six
// whitespace-separated fields positionally, in that exact order, into a
// LivePane. #{pane_dead} is reported as the string "1" or "0";
// parsePaneList keys a dead pane on the literal value "1", never a numeric
// or boolean comparison.
//
// Session targeting: every -t argument that names a SESSION is passed in
// an exact-match form — "=<name>" for session targets (has-session,
// kill-session, attach-session) and "=<name>:" for window/pane targets
// (list-panes, select-layout, display-message, and the chained
// select-layout's own -t in the attach argv, plus the set-option pins'
// window-targeted -t and the two display-message readbacks that confirm
// them), since the window/pane target parser rejects the bare "=<name>"
// form. This is load-bearing:
// tmux falls back to PREFIX matching a bare -t name when no exact match
// exists, so on the shared per-hub server a bare name issued from one
// worktree can silently address a prefix-sharing sibling worktree's
// session — verified live (tmux 3.6): with only "repo2" present,
// `has-session -t repo` exits 0 and `kill-session -t repo` kills repo2.
// contract_integration_test.go's
// TestExactSessionTargetsNeverPrefixMatchSiblings pins both grammars.
// Pane-id (-t %N) targets are already exact and stay bare.
//
// Subcommand set: the engine's correctness depends on new-session,
// has-session, split-window, select-layout, select-pane, send-keys,
// capture-pane, list-panes, list-sessions, display-message,
// set-option -g remain-on-exit, set-option -g mouse, kill-pane,
// kill-session, and kill-server all behaving per tmux's own documented
// semantics for each. The engine may also pass the standard tmux -v/-vv
// verbose-logging global flags on the server-spawning invocation, opt-in
// via the debug_log config key; the configured binary must accept them.
// The capability probe (probe.go) additionally issues -V and
// list-commands, and those two differ in a way that matters: -V is
// answered CLIENT-SIDE and contacts no server, while list-commands is
// answered BY a server and starts one if none is running. Both are
// therefore issued through TmuxCmd, carrying reed's own -L socket, so the
// probe can never start a server on the operator's GLOBAL DEFAULT socket
// — see probeCapabilityLocked for what that cost while it did.
// Every verb named above is REQUIRED: a binary missing any of them is
// unusable, and requiredSubcommands (probe.go) fails the capability probe
// on it.
// set-hook and resize-pane are different: they are reed's first
// deliberately OPTIONAL verbs, absent from requiredSubcommands on purpose,
// because gating the capability probe on them would take every reed verb
// down on a psmux lacking set-hook, over a quality-only option
// (the resize-pin hook documented below, under "The resize round-robin
// and the resize-pin hook") that is already designed to degrade silently
// — their absence costs only the resize pin, never a working session.
//
// Load-bearing behavioral assumptions, each with the rationale that makes it
// load-bearing:
//
//   - Silent split failure (spawn.go): split-window against a pane too
//     small to split exits 0, creates no new pane, and prints an EXISTING
//     pane's id on stdout rather than erroring (psmux's shape; native tmux
//     errors loud with "no space for new pane") — so EVERY split site must
//     verify a split's returned pane id was absent from the pre-split live
//     set before trusting it as genuinely new: launchStrandLocked's strand
//     splits and ensureHeaderPaneLocked's header rebuild both run the shared
//     validateSplitCreatedNewPane guard.
//   - Dead panes under remain-on-exit (spawn.go): with
//     "set-option -g remain-on-exit on" set at boot, a pane whose command
//     exits stays enumerable (pane_dead=1) instead of vanishing WHILE THE
//     SESSION ITSELF SURVIVES — the only signal that lets reconcile
//     distinguish "the strand's process died" from "the pane is simply
//     gone" — and such a corpse is never reused as a strand's pane, since a
//     strand's pane is always a fresh split: send-keys into a dead pane
//     exits 0 while running nothing, silently swallowing the strand's
//     command, which is exactly the failure a fresh split can never
//     reproduce. This corpse-and-session-survives behavior is scoped to a
//     non-last pane (any backend) and to psmux even for the true last pane;
//     it does NOT hold for tmux's true last pane — see the next bullet.
//   - The untracked-pane reap gate (spawn.go, reconcile.go): every pane in
//     a reed session is either the header or a bound strand's pane, and the
//     untracked reap enforces that rule as `anyBoundPresent || headerAlive`,
//     where the header anchor requires ALIVENESS rather than mere
//     presence — launchStrandLocked makes the gate fire from AddStrand and
//     UpdateStrand, neither of which calls ensureHeaderPaneLocked to heal a
//     header corpse first, so a dead header id must never itself authorize
//     sparing the untracked set. The reap runs before pane allocation at
//     one chokepoint inside launchStrandLocked, so the property holds by
//     construction on every realization path rather than requiring two
//     call sites to stay in sync. Two consequences follow: an `up` against
//     a session with zero tracked strands ends up header-only and
//     full-height, because applyLayoutLockedOpts deliberately skips
//     select-layout when no strand owns a present pane, and the header
//     snaps back to its configured height the moment a strand pane exists;
//     and RemoveStrand's own code is unchanged, but its
//     reconcileApplyPersistLocked tail inherits the new gate, so removing
//     the last strand now reaps any untracked alive pane in the same verb.
//   - Last-pane fate is BINARY-DEPENDENT, not universally the corpse
//     behavior above (strand.go's kill-pane loop, RemoveStrand): killing a
//     session's TRUE LAST pane behaves oppositely depending on the
//     configured multiplexer. On tmux (the PATH-resolved POSIX default per
//     template_posix.go) it DESTROYS the session outright (and, if it was
//     the server's only session, the server exits) — this is what the
//     original bug's "exit status 1: no server running" reproduction
//     observed. On psmux (the Windows default) it corpses the pane
//     (pane_dead=1, exit 0) and the session survives — verified, not
//     unverified, by internal/reedcli/smoke_lifecycle_test.go's
//     TestSmokeRemoveLastStrandThenAddRunsTheNewCommand (remove of the sole
//     strand returns 0, then a subsequent add — which calls
//     requireSessionLocked and never re-boots — yields a live second
//     strand, which can only hold if the session survived). has-session and
//     list-panes exit 1 for "no server running" (the same exit-1 the
//     reproduction showed from listPanes), which hasSession (overlay.go)
//     maps to (false, nil) — in CONTRAST to the next bullet's list-sessions
//     and kill-server, which exit 0 regardless of server state and so
//     cannot distinguish "no server" from "server dying asynchronously".
//     That reliable exit-1 is what lets RemoveStrand's post-kill re-probe
//     (hasSession, never list-sessions) classify the emptied session on
//     tmux and swallow the resulting applyErr as an expected success
//     (removalEmptiedSession, strand.go) only when the session is
//     confirmed gone, rather than the fix mispredicting a corpse
//     universally, as an earlier version of this assumption did.
//   - Told-geometry lifetime and the vanished worktree root (server.go's
//     validateToldWorktreeRootLive, lock.go's withOpLock/withTryOpLock): a
//     told Geometry is resolved once per process and pinned for that
//     process's whole life, so a long-lived process such as the header
//     pane's keepalive holds a frozen WorktreeRoot that a `mv` of the
//     worktree makes stale. Every operation therefore re-checks that told
//     worktree root's liveness at the op-lock chokepoint, and refuses
//     rather than creating substrate under a path that is no longer a
//     worktree. One user-visible consequence is a deliberate behaviour
//     change in standalone mode: a `--target-dir` naming a directory that
//     does not exist is now refused at the first engine op, instead of
//     proceeding and deriving a state directory for it. Once the resize
//     watch loop (watchloop.go) itself learns its told worktree root is
//     gone — via errWorktreeRootGone surfacing from a re-apply attempt — it
//     logs exactly one warning and drops to a sixty-second dormant cadence
//     rather than the ordinary two-second poll, so a session abandoned by
//     `down` costs one log line instead of a warning every two seconds for
//     the rest of its life. It automatically returns to whichever mode
//     (poll or signal) it was in before dormancy, logging exactly one more
//     line, once the worktree root exists again. Dormancy never tears down
//     the header pane and never stops the watch loop itself: the session
//     reed walked away from may still be hosting the operator's live
//     strands.
//   - Silent session-name rewriting (server.go's validateToldTmuxIdentity):
//     tmux does not REJECT a session name containing '.' or ':' — it
//     rewrites each to '_', creates the session under the rewritten name,
//     and exits 0 (verified live, tmux 3.6). The same silence covers two
//     further classes: a backslash is DOUBLED (vis(3) doubles '\' unless
//     VIS_NOSLASH is passed, which tmux does not — "bs\slash" becomes
//     "bs\\slash"), and every ASCII control character, DEL, and every
//     invalid-UTF-8 byte is vis-encoded into a multi-character escape (TAB
//     becomes the two literal characters `\t`, 0xFF becomes `\377` —
//     all verified live, tmux 3.6), also with exit 0; valid multi-byte
//     UTF-8 passes through verbatim. Those three classes are the whole
//     rewrite surface: an exhaustive round-trip sweep of every printable
//     ASCII byte through new-session and an exact-match has-session on
//     tmux 3.6 finds exactly '.', ':' and '\'. Because every session
//     target this package issues is the
//     exact-match "=<name>" form above, the created session is then
//     unreachable by the very name that created it: the boot loop polls a
//     target that can never match, and the rewritten session is left
//     squatting on the SHARED per-hub server where no reed verb can
//     address or tear it down. The told identity is therefore validated at
//     withOpLock — the one chokepoint every public op passes — and a name
//     tmux would rewrite is REFUSED rather than sanitized: substituting
//     here would map sibling worktrees "svc.v2" and "svc_v2" onto one
//     session and have each adopt the other's panes.
//     contract_integration_test.go's
//     TestSessionNameRewriteIsSilentAndExactTargetsMissIt pins the wire
//     behavior; the socket key is the mirror-image case (its readable half
//     carries no identity, so ServerName substitutes separators out at the
//     derivation instead of refusing).
//   - The -l leading-dash send-keys bug (spawn.go): send-keys -l parses a
//     '-'-prefixed literal argument as flags and silently drops it (a "--"
//     separator does not stop this parsing), so sendKeysLiteralArg prefixes
//     a single space onto any opaque cmd/resumeCmd beginning with '-' before
//     it is ever handed to send-keys.
//   - Empty-layout session destruction (apply.go): select-layout accepts a
//     layout string that enumerates zero panes (exit 0) and answers it by
//     destroying every pane in the session, wedging it into a zero-pane
//     zombie that no later add can host a strand in — anyPlacedStrand
//     refuses to call select-layout at all when no strand would place a
//     present pane.
//   - Pane ids are server-global and NOT stable across a server rebirth
//     (generation.go): tmux hands out %0, %1, … per SERVER, not per session,
//     and the counter restarts at %0 when the server dies. A persisted pane
//     id therefore does not identify a pane — it identifies a pane within one
//     server generation — and a reed.json that outlives its generation names
//     panes that exist and belong to something else. list-panes cannot tell
//     the two apart, so reconcile cannot either: "present" is the only
//     question it can ask. reed.json therefore records the generation its
//     bindings were minted in (session_id + server pid + session_created,
//     jointly unique and individually immutable over a session's life) and
//     discards every binding whose generation is not the live one, which
//     generalizes Up/Resume's `booted` clear to the tables that arrive
//     BETWEEN boots (R5 review findings R5-F2/R5-F5). The one disagreement
//     that is refused rather than cleared is a recorded session still RUNNING
//     on this socket under a name that is not this worktree's — a renamed
//     worktree or a copied .lyx — because carrying on there launches a second
//     copy of every strand and leaves the first unreachable.
//   - display-message does NOT exit 1 for an absent session (generation.go):
//     unlike has-session and list-panes, which exit 1 for a `-t` target that
//     names no session, display-message exits 0 and expands every
//     session-scoped format to empty while the server-global #{pid} still
//     fills — `-t '=nosuch:' '#{session_id}|#{pid}|#{session_created}'`
//     answers "|2912080|" with exit 0 (verified live, tmux 3.6), and it does
//     not fall back to a current session when several exist. The generation
//     probe's absent case therefore surfaces as parsePaneGeneration's
//     empty-field rejection rather than as a tmux error, and an error from
//     that probe means the round trip could not be RUN, never that the
//     session is gone. The still-running-orphan refusal answers existence
//     with list-sessions for exactly this reason and refuses a listed session
//     it cannot identify, rather than failing open on it (R6 review finding
//     R6-F2).
//   - Duplicate-pane-cell session destruction (render/policy.go's
//     removeDuplicatePaneCells, reconcile.go's clearConflictingPaneBindings):
//     the twin of the empty-layout hazard above, reached from the opposite
//     direction. select-layout likewise does not REJECT a layout string that
//     names one pane TWICE — it accepts it (exit 0), assigns cells
//     positionally, and destroys every pane the resulting short cell list no
//     longer covers (verified live, tmux 3.6: one `up` reduced a two-pane
//     session to one and reported ok:true). Nothing reed constructs can
//     produce such a table — planPaneTarget always yields a split whose
//     result validateSplitCreatedNewPane proves genuinely new — so the
//     source is always a CORRUPT persisted table: a restored backup, a hand-edited
//     or partially-restored reed.json (R5 review finding R5-F3). It is
//     guarded twice on purpose: the engine clears such bindings at the single
//     load chokepoint, and render.Rules independently drops a duplicated pane
//     cell so the destructive string is unreachable from inside that pure,
//     total function's own contract rather than only by a caller remembering
//     to sanitize first.
//   - Async kill-server / probe-always-exits-0 (lifecycle.go): kill-server
//     returns before the server process (and its "__warm__" helper) have
//     actually released the -L socket, and no probe — list-sessions,
//     kill-server itself — can distinguish "no server" from "server dying
//     asynchronously", since both exit 0 either way — so Down/reap logic
//     waits on the underlying OS process actually exiting rather than
//     trusting any CLI exit code as a death signal.
//   - Mouse boot pin (lifecycle.go): the engine pins "-g mouse" to the
//     configured mouse value (default "on") on a fresh boot, right
//     alongside remain-on-exit. Like remain-on-exit and debug_log, this is
//     applied only on the boot that spawns the session, so toggling mouse in
//     config or LYX_REED_MOUSE on an already-running hub has no effect until
//     the reed server restarts -- and an already-materialized reed.yaml keeps
//     whatever value it holds, since reconcile is key-based and never rewrites
//     a value.
//     "on" is the default because "off" is not neutral: with mouse off tmux
//     never claims the wheel, so the terminal's own alternate-screen wheel
//     translation delivers arrow keys straight to the pane's foreground
//     process. In a reed session that process is a live agent, so scrolling
//     scrolls nothing and types into the agent instead -- and under an
//     interactive producer, where an operator is at the pane by design, a
//     scroll gesture edits the answer they were trying to re-read.
//     The cost of "on" is that native terminal text selection needs the
//     terminal's shift-bypass (hold Shift while dragging); tmux copy-mode is
//     the in-band alternative. "on" also enables click-to-switch-pane.
//   - Header band divider row (render/rules.go, height.go): the header pane
//     and the strand stack below it are physically adjacent, so tmux/psmux
//     always renders the same one-row border between them that
//     buildStackBody already budgets for between individual strands —
//     omitting that budget still lets select-layout return success, but
//     tmux inserts the border row anyway, silently overflowing the window
//     by one row. clampHeaderHeight (height.go) also never clamps the
//     header below 1 row for the same reason: a real tmux/psmux
//     select-layout does not cleanly support a genuinely zero-height cell
//     for an always-on pane either. Verified against a real tmux instance;
//     contract_integration_test.go's TestHeaderNeverGetsZeroHeightLayoutCell
//     pins it.
//   - Silent layout rescale (apply.go, windowsize.go): select-layout accepts
//     a layout string whose dimensions disagree with the live window (exit
//     0) and silently rescales it proportionally — measured live on tmux
//     3.6, a "220x50" string applied to a "100x30" window turned a 3-row
//     collapsed strip into 1 row — so every absolute row budget reed
//     computes (Header.HeightRows, CollapsedStripRows, MinFullRows) is
//     scaled by live_height/string_height unless the string is sized to the
//     live window. This is why applyLayoutLocked always plans against
//     liveBoxLocked's live box rather than the configured one. The detached
//     counterpart is the opposite failure: an OVER-budget string is not
//     refused either — with no client attached, tmux GROWS the window to fit
//     the cells, so a client-less session can end up taller than its
//     configured boot height until the next attach snaps it back; with a
//     client attached, the "window-size latest" pin holds the window at the
//     client's size and tmux rescales the cells into it instead.
//   - The resize round-robin and the resize-pin hook (windowsize.go,
//     apply.go, attach.go): tmux has no fixed-height pane concept, and a
//     window-size delta arriving after attach time is handed out one row at
//     a time, round-robin across every vertical cell in the stack — so no
//     absolute row budget reed computes survives a resize on its own.
//     Measured live on tmux 3.6: a healthy attached session's header went
//     from 1 row to 6 across a 76-to-90-row client resize, and to 16 across
//     a further 90-to-120 one.
//     The answer is a window-resized window hook holding one
//     "resize-pane -y" array entry per fixed-height pane — the header band
//     and every collapsed strip — installed by reed and executed by the
//     tmux server itself, refreshed on every successful apply
//     (applyLayoutLocked) and again in AttachArgv's pre-flight, with the
//     pinned heights coming from render.FixedHeightPins: the heights render
//     actually placed the cells at, after clampHeaderHeight and
//     clampToFit, never the raw configured budgets.
//     The watchdog's own signal entry rides the SAME array, always as its
//     last entry, and installResizePinsLocked is its only install site —
//     the array is a whole-snapshot rebuild, so a second writer could only
//     clear the pins this one just installed or accumulate a duplicate
//     touch per attach. Ordering it last means a resize fires the pin
//     fixups before the watcher is told about it, and that is safe because
//     array entries fire independently (see the dead-strip-pin case
//     below), so a pin naming a destroyed pane cannot swallow the touch
//     behind it. It is installed even when the pin set is empty:
//     "nothing is pinned" and "nobody wants to hear about a resize" are
//     different opinions, and gating the touch on a non-empty pin set would
//     pin such a session's watcher into poll mode forever. The one gate it
//     does carry is watchdog: off (and Windows, where the hook is never
//     installed at all), since a touch entry nobody reads is a run-shell
//     spawn per resize for nothing.
//     Two candidate hooks were measured and rejected: client-resized fires
//     BEFORE the layout is resized, so a resize-pane inside it cannot work,
//     and window-layout-changed also fires on reed's own select-layout,
//     inviting re-entrancy for no benefit.
//     The paths that install nothing are deliberate: an apply returning at
//     either of applyLayoutLocked's guards, and every AttachArgv degrade
//     return, issue no set-hook at all — not even the clear — so a
//     previously installed array survives them on purpose, since a clear
//     with no rebuild behind it would drift on the very next resize.
//     That is safe in both guard cases. resize-pane -y against a window's
//     sole pane is a verified silent no-op (exit 0, height unchanged), so
//     the len(live) < 2 case's surviving header pin cannot contradict
//     render.Rules' sole-cell branch.
//     And in the !anyPlacedStrand case — reachable via the operator remedy
//     state.go documents, which deletes reed.json while the session keeps
//     running untracked only until the next mutating verb reaps it — the
//     surviving array is a benefit, still holding the live header and
//     strips at the budgets reed last
//     computed for them.
//     Since the signal entry rides the same array, the same rule decides it:
//     a session that has never reached an install keeps no touch entry and
//     so keeps its watcher in poll mode until the first real apply, and a
//     session that has reached one keeps it across every later guard-skip
//     and degrade.
//     The ~50-row threshold in the original bug report is
//     template_posix.yaml's "height: 50" boot box showing through the BARE
//     (unchained) attach path, not evidence of a miscomputed layout — a
//     synthetic bare attach reproduces the reported table exactly, with 40
//     and 50 rows leaving the header at 1 row and 76 rows taking it to 10.
//     The watchdog's own run-shell signal command (watchdog.go's
//     resizeHookCommand) lives in THIS SAME array, appended as one further
//     entry by installResizePinsLocked whenever the watchdog is enabled —
//     never installed independently. window-resized has no session scope to
//     fall back on: verified live, tmux 3.6, `set-hook` against any target
//     that does not resolve to a specific window errors "no such window",
//     and a hook installed against a window target is invisible from any
//     sibling window of the same session, -w or no -w. Since the array's
//     plain (non-"-a") form replaces the whole thing, a signal hook installed
//     by a separate call would be silently clobbered the next time this
//     rebuild ran (or vice versa) — folding it into the one rebuild is what
//     keeps both mechanisms alive on the same event. hookInstalledLocked
//     (reapply.go) reads the array back and matches the signal command
//     against each line of the multi-entry answer, never the answer as a
//     whole, so its position among the resize-pane pins does not matter.
//   - Measurement record (repaint candidates) (internal/reedcli's
//     smoke_dotfill_measure_test.go, TestSmokeRepaintCandidateMeasurement):
//     measured live on tmux 3.6, across two independent runs of the gate.
//     Candidates were tried in order: candidate 1 (a run-shell -b fragment
//     enumerating the session's attached clients via
//     "list-clients -F '##{client_name}'" and refreshing each with
//     refresh-client) and candidate 2 (the bare refresh-client tmux
//     command, no target). Both candidates cleared the dot-fill artifact
//     (paneStaysCleanOfDotRun held over a fixed 3 s window after the
//     shrink-then-grow trigger) and both left the window size settled
//     (sampleWindowSize's final third held at a single repeated value,
//     both runs), but both were rejected on the same gate: no repeated
//     hook fire. The scenario's shrink-then-grow trigger fires
//     window-resized exactly twice — once per real resize-window call —
//     and fireCount read back 2 for both candidates, identically, which is
//     symmetric evidence that the count comes from the harness's own
//     two-action trigger rather than either candidate's own feedback; the
//     repaint-must-not-self-retrigger decision's exactly-one-fire
//     criterion is nonetheless tripped by that count as literally stated.
//     No candidate was accepted. No repaint entry ships from this task:
//     the resize control and cross-client scenarios stand as the durable
//     reproduction record, and the resize treatment scenario is inverted
//     to assert the artifact's continued appearance rather than its
//     absence, per the no-candidate-accepted disposition.
//   - The dot-fill artifact's mechanism (windowsize.go, attach.go): the dots are painted by tmux
//     itself, in the region of an attached client's terminal that the current window geometry does
//     not cover or whose paint has gone stale relative to a just-changed window size. They are
//     never content reed writes and are in no pane's grid — which is also why capture-pane against
//     the reed session is structurally blind to them, and why the regression test
//     (internal/reedcli's smoke_dotfill_test.go) captures the harness pane hosting the attach client
//     instead. Both reported triggers are one mechanism, a transient or standing mismatch between a
//     client's terminal size and the window's size: a resize changes the client's size and
//     window-size latest moves the window to follow it, and delivering any input to a second client
//     — a keystroke, a mouse report, a focus report — makes that client most-recently-used, which is
//     exactly what window-size latest keys on, so the window resizes to it and every other client is
//     left mismatched with no resize from the operator's point of view. The reporter's
//     "mouse-tracking escape sequences leak through the shared session" hypothesis is rejected:
//     mouse bytes are consumed by tmux as client input and never reach another client's screen; the
//     mouse is only how the second client announced itself as most-recently-used.
//   - The stale-paint / uncovered split, and which trigger is which (windowsize.go): the stale-paint
//     subset is a client fully covered by the new window geometry showing leftover paint, and a
//     forced redraw removes it — this is the resize trigger. The uncovered subset is a client whose
//     terminal is genuinely larger than the window, so tmux has real estate with nothing behind it
//     and legitimately pads it — this is the cross-client trigger as reported, because a VS Code
//     integrated terminal is smaller than a standalone Konsole window. No repaint mechanism can
//     remove the uncovered subset, and reed does not attempt to: removing it would require changing
//     window-size, which is refused for stronger reasons (see the next bullet).
//   - Why window-size latest is not changed, made configurable, or conditioned on the client count
//     (windowsize.go): with two clients of different sizes attached, tmux must pick one window size,
//     so some client is always mismatched under every available policy; latest gives the artifact to
//     the client the operator is not currently using, which is the best of the three. It is also
//     structurally load-bearing: AttachArgv reads #{window-size} back and suppresses the chained
//     select-layout on anything other than latest. Rejected: largest breaks attach-time layout for
//     every operator with a second client open; smallest turns an intermittent artifact into a
//     standing one; manual abandons client-following and is already a chain-suppressing value.
//   - The repaint entry (windowsize.go's installResizePinsLocked): none shipped. The measurement
//     gate (the Measurement record bullet above) rejected both candidates on the same criterion — no
//     repeated hook fire — so installResizePinsLocked installs no forced-repaint entry into the
//     window-resized array; its own doc comment records the disposition and points back at that
//     Measurement record block. With no repaint entry, a resize's dot-fill artifact is cleared by
//     the watchdog's own round trip when the watchdog is on (the hook's run-shell touch,
//     watchdogSignalTick, watchdogDebounceQuiet, and the re-apply round trip — roughly a second) or
//     stands until the next reed operation when the watchdog is off; the cross-client trigger is
//     never cleared by anything, per the split above.
//   - The two acceptance criteria and why "did it clear the artifact" was not sufficient:
//     window-resized fires on a settled size change rather than on a paint, and a server-issued
//     refresh-client repaints a client at its existing size without moving the most-recently-used
//     pointer — but that reasoning was measured rather than assumed, because refreshing every client
//     would otherwise hand most-recently-used to whichever client was refreshed last, resize the
//     window to it, fire the hook again, and loop. Both criteria — no repeated hook fire, no resize
//     storm — were measured live on tmux 3.6 (Measurement record above); both candidates cleared the
//     artifact and held the window settled, and both failed the same first criterion.
//   - The (would-be) repaint entry's independence from watchdog, with the full call-site map stated
//     plainly (windowsize.go, attach.go, apply.go, lifecycle.go), rather than left for a reader to
//     re-derive across four files: watchdog gates the watch loop and its signal entry only; a forced
//     redraw would mutate nothing, so an operator who turns off self-healing would still keep a
//     repaint entry if one existed. The two unset sites and the two install sites are different
//     sites: pinGeometryOptionsLocked is called from lifecycle.go's boot path and from AttachArgv's
//     pre-flight, while installResizePinsLocked is called from that same pre-flight and from
//     applyLayoutLockedOpts in apply.go, and nowhere else. Only the attach path holds both, in that
//     order, in one locked closure. The boot path clears and returns without rebuilding. AttachArgv
//     reaches the install only when its chain succeeds: it returns bare before taking the lock on
//     non-positive cols/rows, and every in-closure degrade returns before the install — while
//     pinGeometryOptionsLocked has already run, so under watchdog: off a degrading attach issues the
//     unconditional clear and leaves without rebuilding. And applyLayoutLockedOpts returns
//     immediately after select-layout when opts.SkipFocus is set, which is exactly the mode the
//     watchdog's own re-apply (reapplyLayout) uses — so the watchdog re-apply never installs the
//     array. So the array is (re)established only by an attach whose chain succeeds or by a focusing
//     apply (up, add, remove, resume); under watchdog: off it is empty from boot and any degrading
//     attach returns it to empty until one of those runs. This is accepted as-is and unchanged by
//     this task — it is already today's behaviour for the resize-pane pins — and, had a repaint
//     entry shipped, it would have shared their lifecycle rather than inventing one; widening the
//     install to the SkipFocus path remains out of scope because it would put a hook-array rebuild
//     inside the watchdog's own re-apply loop.
//   - Windows (windowsize.go): installResizePinsLocked carries no runtime.GOOS gate: on Windows it
//     issues the clear and every resize-pane pin argv exactly as elsewhere, and
//     pinGeometryOptionsLocked's early Windows return covers only the unset half of the lifecycle.
//     The single mechanism keeping the signal entry off Windows is resizeSignalHookCommand returning
//     "" combined with resizePinHookArgvs emitting no entry for an empty body. Since no repaint
//     entry shipped, this bullet records what its wiring would have been rather than what runs: it
//     would have carried its own ""-returning check for the same unchanged underlying reason —
//     set-hook and run-shell are outside requiredSubcommands and psmux's support for them is
//     unverified.
//   - The attach-time multi-client warning (attach.go's warnMismatchedClientsLocked): AttachArgv's
//     pre-flight lists the session's attached clients and emits one logger.Warn per client whose
//     size differs from the size this attach was told, naming that client and both sizes; zero
//     differing clients produce zero lines. It never blocks, never changes the argv, and never
//     reaches the JSON envelope. This is the primary deliverable for the cross-client trigger rather
//     than a consolation prize, because that trigger's dots are the uncovered subset and cannot be
//     repainted away — the residual's real operator cost is bewilderment, and the warning turns a
//     mysterious artifact into a logged, searchable fact at the moment the operator creates the
//     condition. The target-form rule binding both new call sites: list-clients -t takes a SESSION
//     target, so warnMismatchedClientsLocked uses the bare "=<name>" form, while set-hook keeps the
//     "=<name>:" window form because the array is window-scoped.
//   - list-clients and refresh-client stay out of requiredSubcommands (probe.go): both are
//     geometry-quality only, exactly like the status/window-size pins the Shared Decision
//     geometry-tmux-failures-are-non-fatal-everywhere already exempts from fatality. A failing or
//     unsupported list-clients logs one warning and emits no multi-client warning
//     (warnMismatchedClientsLocked); refresh-client was never wired in, since no candidate shipped,
//     but the same exemption would have applied to it — a refresh-client the multiplexer does not
//     implement would have made the hook entry a server-fired no-op, the same outcome as the
//     mitigation not helping. Adding either would make a multiplexer that runs reed perfectly well
//     today fail at boot over a cosmetic feature.
//   - The chained attach (attach.go): AttachArgv's argv is
//     "attach-session … ; select-layout -t '=<session>:' <layout>", with the
//     separator a literal one-character ";" argv element — never "\;",
//     since exec.Command passes argv directly to the child and no shell ever
//     sees it to unescape. The chained select-layout runs only after the
//     client has attached and tmux has already resized the window to it, so
//     the layout string lands verbatim with no rescale — but only until the
//     next window resize; see the resize round-robin bullet above for what
//     holds the fixed-height budgets afterward. attach-session is
//     first in the chain, so a failing or unsupported select-layout still
//     leaves the operator attached — strictly no worse than before this
//     task. The window between building the string and applying it is not
//     closed: it is planned under the op lock and executed by the shell
//     seconds later, outside it, so the live pane set can have moved on by
//     the time it runs. When the pane count no longer matches, tmux REFUSES
//     the layout (exit 1, "have 3 panes but need 2") and destroys nothing;
//     when the count still matches but membership shifted, cells apply
//     positionally, so a strand ends up mis-sized rather than lost.
//   - The two geometry option pins (windowsize.go): "status off" and
//     "window-size latest" are pinned session/window-targeted
//     (-t '=<session>:', and -w for window-size, per the Session targeting
//     grammar above) both at boot and again in AttachArgv's pre-flight, and
//     their EFFECTIVE values are read back with display-message rather than
//     trusted from set-option's exit status, because a -g pin plus exit 0 is
//     not proof the option took — verified live, tmux 3.6: a session-scoped
//     "status on" survives a global "set-option -g status off" with exit 0,
//     and a window-scoped "window-size manual" survives the global "latest"
//     pin the same way. "#{status}" feeds the reserved-row count reserved
//     for the status line ("off" -> 0, "on" -> 1, a numeric N -> N); a
//     "#{window-size}" other than "latest", or either readback erroring or
//     answering an unrecognised value, suppresses the chain rather than
//     risking a wrong-height string. Unlike the remain-on-exit/mouse pins
//     beside them, both pins and both readbacks here are NON-FATAL: those
//     two are correctness dependencies, these two are geometry-quality
//     options whose absence degrades to a working session, and psmux's
//     support for them is unverified anywhere in this repo (Shared Decision
//     geometry-tmux-failures-are-non-fatal-everywhere).
//   - window-resized is the only usable resize event source (windowsize.go,
//     watchdog.go): on a client resize the hooks fire client-resized ->
//     window-layout-changed -> window-resized; client-resized reports the
//     STALE pre-resize window size, so it cannot plan a correct layout, and
//     window-layout-changed is self-triggering, so reed's own select-layout
//     would re-enter the watcher in an infinite loop. window-resized fires
//     exactly once per settled size, after the window already has the new
//     geometry, on both growth and shrink.
//   - SIGWINCH is not a substitute (reedcli/header.go's blocking tail): with
//     the header pinned to one row, growing the window delivers SIGWINCH
//     every time — and that growth IS the layout bug — but SHRINKING
//     delivers nothing while the strand budgets below are silently violated
//     (at 30 rows the bottom strand had been squeezed from 15 rows to 2). A
//     watcher that self-heals only on growth is worse than none, because the
//     operator learns to trust it.
//   - select-layout does not fire window-resized (apply.go, reapply.go):
//     verified in all four cases — attached, detached, re-applying an
//     identical layout, and the documented detached over-budget apply that
//     genuinely grew the window 40 -> 60 rows — zero fires each time. So
//     window-resized tracks CLIENT-DRIVEN size changes, not layout-driven
//     ones, which is exactly the property that makes it usable where
//     window-layout-changed is not. The box-equality guard inside
//     reapplyLayout is kept anyway: the probe settles tmux 3.6 but not
//     psmux, and a silent infinite loop inside the session keepalive is the
//     worst available failure mode.
//   - The plain set-hook form replaces; -a accumulates (windowsize.go): four
//     identical plain installs yield exactly one fire per resize; three
//     further -a appends yield four. installResizePinsLocked's rebuild always
//     starts from its own unconditional clear and establishes entry [0] with a
//     plain set-hook, using -a only for the entries behind it. Since the rebuild
//     runs on every AttachArgv pre-flight as well as on every apply, an
//     append-only pattern would accumulate N run-shell spawns and N redundant
//     resize-panes per resize after N attaches — the plain-first/-a-after
//     pattern keeps it idempotent instead.
//   - The hook readback is show-options, not show-hooks (reapply.go): in
//     tmux 3.6 hooks are options, and show-hooks prints nothing for a
//     window-scoped hook that demonstrably fires — a show-hooks-based probe
//     would report "no hook" every time and pin every watcher into poll
//     mode.
//   - show-options -v prints an ARRAY option one entry per line
//     (windowsize.go, reapply.go): verified live on tmux 3.6, -v prints
//     every entry of window-resized in index order with the
//     "window-resized[N]" prefix that plain show-options carries suppressed.
//     So the probe's match is exact PER ENTRY, never against the whole
//     answer: reed's own array normally holds a resize-pane pin per
//     fixed-height pane ahead of the touch, and an equality test against the
//     whole multi-line answer therefore reports "absent" on every healthy
//     session with anything pinned — which is exactly what it did for as
//     long as the touch entry had no install site at all. Exactness stays
//     per entry all the same, never a substring search over the raw answer,
//     which would accept a foreign entry that merely embeds reed's command
//     string. Entry POSITION is deliberately outside the match: reed writes
//     the touch last, but the question the probe answers is only "will a
//     resize touch THIS worktree's signal file".
//   - run-shell without -b blocks the tmux server (watchdog.go).
//   - liveBoxLocked never reports failure through its box (windowsize.go,
//     reapply.go): a degraded query returns the configured
//     cfg.Width/cfg.Height pair, which is a perfectly plausible-looking box,
//     so any caller comparing boxes across calls must consume the method's
//     second return value — otherwise a fallback that happens to equal the
//     last applied box skips forever and one that differs re-applies
//     forever.
//   - The header pane's stdout/stderr is its screen (reedcli/header.go): the
//     --blocking tail rebinds the logger's stderr sink to a discarding
//     writer before entering the loop; the durable sink is untouched.
//   - testing.Testing() gates the header launch line (headerpane.go,
//     lifecycle.go): no Go test can exercise a header-hosted watch loop by
//     booting a header pane, which is why the tier-2 proof runs the loop
//     in-process against a real session instead.
//
// requiredSubcommands (probe.go) still does not grow for the live-geometry
// rule, the attach chain, or the two option pins: display-message,
// select-layout, set-option, and list-panes were already spent by the
// engine before this task.
// set-hook and resize-pane are not the same story: both are new to
// internal/, so the wire contract this package assumes genuinely widens,
// and that widening costing nothing at the probe is now a deliberate
// trade, not a free consequence — the non-fatal degrade the two Optional
// verbs above are wired for (Shared Decision
// hook-failure-is-non-fatal-everywhere) is what pays for it.
package reedengine
