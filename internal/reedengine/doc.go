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
// every strand-accounting, adoption, split-target, and reconcile path (see
// ensureHeaderPaneLocked in lifecycle.go, planPaneTarget in spawn.go, and
// planReconcile's exemptPaneIDs in reconcile.go for the three exclusion
// seams), so that removing a session's last strand can never destroy the
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
//   - Dead-pane adoption via remain-on-exit (spawn.go): with
//     "set-option -g remain-on-exit on" set at boot, a pane whose command
//     exits stays enumerable (pane_dead=1) instead of vanishing WHILE THE
//     SESSION ITSELF SURVIVES — the only signal that lets reconcile
//     distinguish "the strand's process died" from "the pane is simply
//     gone" — and planPaneTarget must never adopt such a corpse, since
//     send-keys into a dead pane exits 0 while running nothing, silently
//     swallowing the strand's command. This corpse-and-session-survives
//     behavior is scoped to a non-last pane (any backend) and to psmux even
//     for the true last pane; it does NOT hold for tmux's true last pane —
//     see the next bullet.
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
//     produce such a table — planPaneTarget never adopts or splits the header
//     and validateSplitCreatedNewPane guarantees a fresh id — so the source
//     is always a CORRUPT persisted table: a restored backup, a hand-edited
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
//   - The chained attach (attach.go): AttachArgv's argv is
//     "attach-session … ; select-layout -t '=<session>:' <layout>", with the
//     separator a literal one-character ";" argv element — never "\;",
//     since exec.Command passes argv directly to the child and no shell ever
//     sees it to unescape. The chained select-layout runs only after the
//     client has attached and tmux has already resized the window to it, so
//     the layout string lands verbatim with no rescale. attach-session is
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
//
// requiredSubcommands (probe.go) did not grow for any of this: display-message,
// select-layout, set-option, and list-panes were already spent by the engine
// before this task, so the live-geometry rule, the attach chain, and the two
// option pins add no capability-probe change and no new psmux risk.
package reedengine
