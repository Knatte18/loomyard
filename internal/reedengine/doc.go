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
// reedengine imports internal/reedengine/render (the pure display-vocabulary leaf: Anchor, Display and ValidateDisplay).
// render never imports reedengine, so the import graph stays acyclic (reedcli -> reedengine -> render).
//
// reedengine is told its geometry as a Geometry value (geometry.go) and
// derives none of it. internal/lyxcwd is consequently absent from this
// package's DIRECT production imports; hubgeom.ReedGeometry is the hub-mode
// teller that builds a Geometry from a resolved *lyxcwd.Location for every
// hub-mode caller. Stated honestly, that is a direct-import fact and not a
// transitive one: internal/lyxcwd is still in `go list -deps
// ./internal/reedengine`, reached through internal/logger, so what the
// absence buys is that reed never RESOLVES its own coordinates — not
// isolation from the package that does.
//
// reedengine is told, as an EnvScrub passed to New, the environment keys its tmux server spawn must not inherit.
// The server is shared by every strand, so the teller passes the keys of every registered engine, and reedengine carries no provider's names itself.
//
// internal/fabricengine became a direct production import when
// spawnwatchdog.go landed (SpawnWatchdog calls fabricengine.HubScratchDir).
// That does not relax the Told-Geometry Invariant: the invariant bars a
// direct internal/lyxcwd import and requires an engine be handed the
// absolute paths it operates on, and the one fabricengine call here takes
// hubPath as a parameter and derives nothing from it — the seam is told its
// geometry exactly as the rest of this package is.
//
// One additional invariant this package enforces: exactly one named tmux
// server per hub. The server name is derived deterministically from the hub
// path (ServerName), so every worktree under the same hub locates and shares
// the same tmux server rather than each spawning its own.
//
// A strand's Retiring flag is reed's own lifecycle bookkeeping, not a caller field:
// `reed remove --detach` sets it through Engine.MarkRetiring before spawning the detached remover,
// so a caller can tell a strand about to remove itself from a live one,
// and the remover no-ops on a guid that is already gone.
// EndSessionByName sits beside ReapSession as the engine-less session end for a pair whose worktree is gone, and kills the server when it ends the last session.
//
// Engine.SetWaitMark marks a strand's pane as waiting: it sets the pane user options @lyx_wait (the label) and @lyx_wait_start (epoch seconds), or unsets both on an empty label.
// The mark is display only,
// so reed stores nothing for it and the options die with the pane;
// the status bar renders it on the strand's own button, as " ⏳<label> <elapsed>m" appended to the strand's name with tmux computing the elapsed minutes at each refresh,
// and no Go decision reads it.
//
// Engine.CapturePaneStyled returns a pane's screen with its SGR attributes beside the plain CapturePane, for a caller that reads cell styles; reed interprets none of it.
//
// Segment colors are read from reed.yaml's segment_colors block, resolved only through Engine.segmentColor, and refused only at boot.
//
// SwitchClient (switch.go) is the engine-less function behind `lyx reed switch`: it addresses a told server by socket path and issues list-sessions, display-message and switch-client on it, and nothing else.
// It is the only reed code that issues switch-client.
//
// FollowSession (follow.go) is the engine-less function behind `lyx reed follow`: it addresses a told session of a told server by socket path and issues list-panes, set-option @lyx_follow on and, when a window is relevant, select-window, and nothing else.
// It reads no working directory, config or state, so it picks the window from tmux facts alone: the highest-index window carrying @lyx_own and not @lyx_overview that holds a pane that is not dead, else the window carrying @lyx_overview, else none.
// openOwnWindowLocked sets the window option @lyx_overview beside @lyx_own on an overview window, so that rule can tell the overview from an agent window; an overview window opened by an older lyx carries no marker and counts as an agent window, which it can win only when no other agent is live, the same outcome as the overview fallback.
// A failed listing, which is how an unknown session answers, returns an error before any write, a failed option write returns before any select, and a failed select returns an error naming the window.
//
// Every strand pane carries the user options @strand (the role segment of its name, or the full name when it does not parse) and @strand_color (the tmux color of its segment, unset when it has none),
// and the strand window carries @lyx_strands.
// Launch sets the pane options and boot marks the strand window; every attach pre-flight marks the window and re-asserts both pane options on every strand bound to a live pane, so a session or strand spawned by an older lyx gets them at its next attach.
// The Selvage pane carries the user option @lyx_console (consolePaneOption), set when it is split and re-asserted by every attach pre-flight.
// The options are display only: the status bar and the key bindings read them, no Go decision does, and Selvage never gets @strand or @strand_color.
// The session option @lyx_follow is the one decision-bearing session option reed reads; @strand, @strand_color, @lyx_unseen and the wait marks stay display only.
// A strand's Segment is an opaque field its spawner names through AddSpec.Segment; reed reads it only to look up the segment's color, which the strand an add or replace returns and Status carry as Color.
// An own window gets a status-bar button on line 1 in its segment's color, read from the window options @lyx_own and @strand_color, labelled with the window name and carrying its panes' wait marks.
// The button is lit while the window is current, and otherwise that color on dark grey, underscored while the window carries @lyx_unseen.
// Outside Windows, the geometry pins also pin the session hook `session-window-changed` with `set-option -w -u @lyx_unseen`, so selecting the window clears the mark; boot and every attach pre-flight run the pin.
// On Windows no hook is pinned and the mark is never set.
// The shell button is a range=user|shell button labelled shell, lit while the shell window is active; line 1 follows window index order, so the overview's button is first, the agents' follow in start order and the shell button comes last.
// An overview window's button is an own-window button with no @strand_color, which renders as the plain window-button style.
// A window carrying @lyx_hidden still gets no button, and no Go decision reads @strand_color or @lyx_unseen.
// Line 2 lists the sessions, then the follow button, a range=user|follow button labelled follow that sits left of the time, lit green while the current session's @lyx_follow is not off and unlit while it is.
//
// The navigation bindings (bindings.go) are root-table bindings on the hub's reed server, issued by pinBindingsLocked at the end of the geometry pins, so boot and every attach pre-flight pin them.
// The pin stamps the server-global option @lyx_bindings_build with the pinning build's VCS commit time and revision label, and unsets it for a binary with no VCS stamp.
// Every op under the blocking op lock reads the stamp before its step and re-pins the bindings when it is absent, unparseable or names another build whose commit time is not after the running one's, so a deployed lyx reaches a running server at its first reed op.
// Bound: an older build never re-pins over a newer one's stamp, so a long-lived process of an older lyx cannot restore stale bindings, and an unstamped binary never re-pins; two builds of one commit time, such as a dirty build and its commit's clean one, can re-pin over each other.
// The name-repair loop never re-pins, and the global status-bar options refresh only at boot and attach.
// The pin starts with `unbind-key -n M-z`, which removes the Alt+z an older build bound, since a `bind-key` re-pin never removes a binding; reed binds no zoom key.
// Alt+Up and Alt+Down step to the previous or next window in index order, in every window and wrapping with tmux's own window wrap: a step that lands on a window carrying @lyx_hidden goes on, forward to the session's first window, the overview, and back to the strands' window, the shell, which is one conditional step since hidden windows sit after the shell (on psmux, where the order is best-effort, a step may land on a hidden window);
// Alt+Left and Alt+Right run `lyx reed switch --prev|--next` in the background through run-shell, naming the absolute path of the `lyx` that pinned them (panebin.go's composeSwitchCommand), never PATH's;
// and a left click on the status bar dispatches on its range: the shell button selects the strands' window, a window button selects that window, a session button switches the client to that session, and the follow button toggles the follow state.
// The status bar draws no button for a strand pane, so a click selects no pane.
// The follow state is written off by the bindings that change window by hand: Alt+Up and Alt+Down start with `set-option @lyx_follow off`, so the write lands also when the step wraps onto the current window, and the shell and window clicks start with the same write, also for a click on the current window.
// Each write is untargeted, so it hits the session of the client that pressed the key or clicked; Alt+Left, Alt+Right and the session click leave the follow state alone.
// The follow button writes off when the state is on and changes no window; from off it writes on and, when the jump line is pinned, runs `lyx reed follow` in the background through run-shell, so the viewer jumps back to the run.
// The jump line (panebin.go's composeFollowCommand) names the absolute path of the `lyx` that pinned it and the clicking client's session as `#{q:session_name}`, and rides the server-global option @lyx_follow_command, which the click runs through `#{E:...}`, keeping the line out of tmux's nested command-string quoting.
// pinBindingsLocked sets that option when the line resolves and unsets it otherwise, so a click from off then only writes follow on and the next agent window takes the viewer.
// A psmux without bind-key or run-shell pins none of these, never writes off, and its sessions always follow.
// Bound: the bindings only ever write off, and the follow button on, to the clicking client's session, and the button runs one `lyx` verb whose effect FollowSession bounds; a window change outside these bindings leaves follow on.
// Bound: these four keys and the left status click are taken from every pane on the server, so a program in a pane, Claude Code included, never receives them; no other key or mouse binding is changed, and tmux's right-click menus keep their defaults.
// The shell branch finds the strands' window through the @lyx_strands marker, expanded at click time to a tmux-issued window id (`@<n>`), never a config value or free text.
// Every binding is non-fatal: a failed `bind-key` is logged and the rest are still issued, and an unresolvable executable, socket path or tmux path logs a named warning and leaves only Alt+Left and Alt+Right unbound.
// The capability probe checks bind-key, run-shell, if-shell and switch-client as optional verbs after the required set: a missing one logs one warning naming it and never fails the probe, so a psmux without them still boots.
//
// Reed keeps no zoom state: a step never zooms or unzooms a pane.
//
// A restarted server's sessions come back in spawn order (revive.go).
// The teller tells Geometry.SpawnOrder, a lazy list of ReviveEntry for the hub's worktrees in spawn order, nil in standalone mode; reedengine sorts nothing and reads no loom or fabric record.
// The revival predicate is one test a worktree evaluates only on its own state under its own op lock: its reed.json records a session, and that session is not live.
// A downed worktree has no such record, so it is never revived, and a first boot has none either.
// The steps that can create the session (Up, EnsureSession, Resume, AddStrandUnless) run through withRevivalFirst.
// At the point where the booter would create its session, with the skip unset, a spawn order told and the predicate holding, the boot returns a sentinel having created nothing.
// The lock is then released, each entry before the booter's own in the list runs its Revive in list order, sequentially and outside the booter's lock,
// and the whole step runs again from its start under the lock with the skip set, so a session another booter created meanwhile is used as found and the trigger is evaluated once.
// No two op locks are ever held at once.
// Engine.Revive runs this worktree's ordinary up (session and Selvage, no strand relaunch) with the skip set when the predicate holds, and reports whether it did.
// Bound: the step runs only for a revival of the booter's own recorded session, never for a first boot; it starts no strand and never recurses, since every nested up carries the skip;
// an entry whose Revive or whose list fails is logged and skipped, and the boot goes on; a later-position session that is still live is not reordered, since session ids cannot change without killing live sessions.
//
// A second package-level invariant: every session also carries exactly one
// additional, permanent pane beyond its strands — Selvage
// (ReedState.SelvagePaneID). It is a first-class construct, deliberately
// never a Strand (Shared Decision header-is-not-a-strand): it is excluded
// from strand accounting, from being the preferred split target, and from
// both halves of reconcile's kill schedule (see selvagepane.go's
// create/heal path and the reapPolicy value
// planReconcile is handed for the three exclusion seams), so that
// removing a session's last strand can never destroy the
// session or corpse its sole pane — Selvage keeps the session (and the
// substrate the next add needs) alive no matter how many strands come and
// go. It boots alongside the session/initial pane on both Up and Resume.
// Selvage is created by a split-window call that carries e.cfg.Shell — the
// operator's configured shell, `bash` on POSIX and `pwsh` on Windows by
// default — as its own trailing shell-command argument, the same way
// new-session already launches the session's first pane: Selvage is an
// ordinary, typeable interactive shell, never a re-exec of lyx and never
// sent a command of reed's own choosing.
// A strand pane keeps its split with no trailing command and starts the shell from the session's own pins:
// the boot and the attach pre-flight set `default-shell` and an equal non-empty `default-command` to the resolved shell on the session, never globally, because one server serves every worktree of the hub.
// A non-empty `default-command` makes tmux start the shell without the login flag, so a strand pane runs the shell's rc files and not the login profile.
// The boot refuses a configured shell that does not resolve, naming the `shell` key;
// the attach pre-flight logs a warning and leaves the session's earlier values.
// Windows keeps its current boot and pins no session shell.
// A Selvage pane whose shell process dies (pane_dead=1) is deliberately kept as an enumerable corpse by reconcile, never killed there, and healed by ensureSelvagePaneLocked on the next Up/Resume: the corpse is killed and a fresh full-width Selvage is split in below the bottom-most live pane, carrying e.cfg.Shell.
//
// Three module-local Selvage rules, kept here rather than in a PATTERN entry
// because they describe this package's own design rather than a
// cross-cutting invariant another module could violate.
// Selvage is always physically bottom-most in the window, because every split site that creates or rebuilds it targets the bottommost live pane, never the topmost.
// Selvage is never a strand — it is excluded
// from ReedState.Strands, from strand accounting, and from every strand-only
// code path, by construction rather than by a runtime check. And Selvage is
// never written to, cleared, or sent keys by reed — it is a real, usable
// terminal an operator can run lyx/reed commands in, and reed's own writes
// stop at booting its shell; the ED2/ED3 screen-clear payload the header
// pane once needed to display identity text has no counterpart here, since
// Selvage never displays anything of reed's choosing. selvagepane.go is now
// the single file these three rules are implemented in, and
// selvagepane_enforcement_test.go is the mechanical check keeping them
// there.
//
// The placement rule: every placed strand gets a window of its own, a group window or the overview, and the shell window holds Selvage alone, which fills it after boot, a heal and the move below.
// The anchors are own-window, overview and hidden (internal/reedengine/render); a hidden strand never owns a pane.
// `lyx reed add` with no `--anchor` places the strand own-window.
// The retired anchors `below-parent` and `top` read as own-window when reed.json is decoded, and a write refuses them before any tmux call, naming the way forward.
// Reed arranges no panes in the shell window: it computes no layout, pins no pane height and keeps no resize hook.
// The only layout reed issues is the even layout of a group window, in the window_split direction.
//
// The move rule: the head of every reconcile (listReconcilePanesLocked, reached from launch, resume and the shared apply tail) lists the shell window's panes and breaks any pane a strand is bound to out into a window of its own.
// That upgrades a session an older lyx left with strands split under a parent.
// The recorded Selvage pane and an unbound pane never move, and with the Selvage pane absent the last bound pane stays, because breaking out a window's only pane would take the window with it.
// The shell window is found by the Selvage pane first, else by the @lyx_strands marker, and with neither the current window is listed and nothing moves.
//
// Every strand pane reed creates resolves `lyx` to the binary that spawned it, because launchStrandLocked prepends that binary's directory to the pane's PATH and exports LYX_BIN to its absolute path as shell statements.
// A fresh shell a strand's command starts re-runs its rc and can reorder PATH, so LYX_BIN names the spawning binary there,
// and a shuttle engine re-asserts LYX_BIN's directory in its agent's tool shell.
// The composed line, prelude and launch command together, is written to the strand's launch script,
// and the typed send-keys line sources it in the pane shell's own scope, so the prelude still runs after the pane shell's profile.
// The shell dialect's discard-line key (`C-u` on POSIX, none on PowerShell) is sent ahead of that line, so input that reached the booting pane first, such as a viewer's keystrokes or a terminal reply after a window switch, never prefixes it.
// The strand records that absolute path as `Strand.LyxBin` at every launch and relaunch, and reed's status reports it.
// The composition lives in panebin.go and is reached from the one chokepoint, so the property holds
// by construction for every present and future AddStrand caller including Resume's replay. The
// mechanism is a typed shell statement rather than `split-window -e` or `set-environment`, so it
// prepends onto the pane's live PATH instead of replacing it with a value computed from the lyx
// process's own environment, it survives the pane shell's profile (which has already run by the time
// anything is typed), and it needs no multiplexer capability. Selvage's pane and the new-session
// first pane are out of scope.
//
// A strand's full name is formed once, in AddStrand under the state lock, from the prefix the told geometry carries and the strand's role (the grammar is PATTERN-agent-name).
// A role already held in the worktree is numbered `-N`, and an explicit name that is held refuses.
// A role-less add takes the default role `strand`, and an empty told shortname refuses before anything boots, naming `lyx fabric shortname <shortname>`.
// The name is a birth attribute and the only lookup key: it is stored in the strand record and every by-name lookup resolves through it, matching the full name, its role segment, or a legacy exact name.
// An add can also name a strand whose presence in this worktree's state skips it (AddStrandUnless), matched by the same rule as an explicit name;
// a live, dormant or hidden match all skip,
// and a skipped add launches, revives, removes and changes nothing.
// The retired `strand_name` config key is ignored on load.
//
// A strand may carry a done-when list (Strand.DoneWhen, set by its spawner through AddSpec.DoneWhen), the one place reed learns a strand finished.
// Reed only stats the paths: Resume drops from state, instead of relaunching, a non-live strand whose list is non-empty and whose every path exists, and reports the count on ResumeResult.Dropped.
// While a live pause entry covers the pair (Geometry.PauseRecordPath and NameSlug both set), Resume relaunches nothing and names the strands it would have relaunched on ResumeResult.Held; the prime is never held.
// A live strand, a strand with no list and a strand with a path missing are untouched or relaunched as before.
//
// The name is mirrored outward, never read back as truth.
// launchStrandLocked exports it to the strand's process as LYX_STRAND_NAME, with LYX_PARENT when a parent is told, ahead of the launch command.
// It also sets the pane title to the full name after `set-option -p allow-set-title off`, so the program in the pane cannot overwrite it.
// A provider session name is the other mirror, passed by the launch line as `--name`.
// The daemon's file-event waits (watchloop.go) go through a FileWatchOpener, whose production value OpenFileWatch wraps internal/fswatch.
// PasteSubmit is the pane-write transport op: one tmux invocation fills a fresh named buffer (`lyx-inject-` plus 8 hex characters) with the text, pastes it as a bracketed paste, deletes the buffer, and, when asked, sends Enter behind it.
// The buffer is loaded with `load-buffer -` from the tmux client's standard input in the same invocation.
// The paste and the Enter reach the pane in one command list, so nothing the operator types lands between them, and the operator's automatic paste-buffer stack is never touched.
// A text of any length pastes whole, since tmux's command size limit applies to the command list and not to a buffer read from standard input.
// A failed invocation stops before the paste's own delete, so a best-effort `delete-buffer` of its own removes the buffer.
// OperatorActive is the operator-activity query: it lists the session's attached clients with their active pane and last-activity epoch second, and answers whether any client is on the strand's pane with activity within the quiet window, plus that window.
// The window is the reed.yaml key `operator_quiet_s` (seconds, default 5; a zero or negative value takes the default, as does a materialized reed.yaml lacking the key until `lyx config reconcile` writes it).
// No attached client, or clients on other panes only, answer not active, and a failed listing is returned as the error for the caller to decide on.
// The hub watchdog daemon retires and respawns itself when the `lyx` binary it started from is replaced (reedcli's watchdog verb owns the check and calls SpawnWatchdog for the successor), and a daemon started before that check existed is ended once by hand after the deploy.
// The hub watchdog daemon runs a name-repair tick (namerepair.go), gated by the reed.yaml key `watchdog`, and runs no resize self-heal.
// The tick resets a drifted pane title itself and repairs a drifted provider session name through the SessionNamer seam, which the provider package implements and `internal/shuttleengines` fills.
// A strand carries an opaque provider tag its spawner sets (Strand.Provider, JSON `provider`, omitted when empty); reed stores it and never interprets it.
// The watchdog is handed a map from tag to SessionNamer and repairs each strand's session name through the namer for its tag, so a strand whose tag has no namer gets its pane title repaired only.
// The session-name repair types the rename only into an idle pane with no active operator, and does it with one paste-submit under the op lock it already holds; an active operator defers the repair to the next tick, and an unreadable client list does not hold it.
// The name-repair tick backs off: its wait starts at ten seconds, doubles through NextWakeCadence up to a minute across passes that repair nothing, and returns to ten seconds after a pass that repaired a title or a session name.
// NextWakeCadence is the one backoff rule the daemon's loops share.
// A failure that can repeat on every tick of the name-repair tick logs once per episode through logger.Episode: Warn at the first failure, Debug while it goes on, Info at recovery with the count of failed ticks.
// The name-repair tick guards the session-name repair's operator-activity read, pane capture and rename typing the same way, per strand where it names one.
// The tmux reads and writes the tick reaches through its callees share one engine-wide episode set, one episode per read: the live pane generation read and each pane title repair.
// One-shot callers of the same helpers share those episodes, so a failure inside a tick's episode logs at Debug and a success ends the episode with the Info line.
// The name-repair loop holds its own episode for a pass that fails as a whole.
// The two in-memory pane-binding clears of a state load are no failure and have no episode: a state load never saves, so the same clear repeats on every tick, and it warns once per recorded and live generation pair or cleared strand set and logs at Debug while it repeats.
// The discover signal is the hub-level file named DiscoverSignalFileName, at the path Geometry.DiscoverSignalPath tells.
// A cold session boot touches it, right after the new session exists and on the boot path alone, so every path that boots a session raises it and an attach to a live session touches nothing; a failed touch is logged at Warn and never fails the boot.
// The daemon's discovery loop wakes on it; standalone mode tells no path and touches nothing.
// When `Down` ends the server, because no session is left or the session listing errored, it writes DiscoverStopMarker into the file, atomically, before kill-server, to ask the daemon to stop;
// a cold boot's touch truncates the file back to an ordinary discover, so a racing `up` wins, and `DownResult.EndedServer` reports the teardown.
// `lyx reed list` reports the hub-wide directory of names across worktrees, with each row's live state.
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
// Operator config: a server reed starts reads no `~/.tmux.conf`.
// The server spawn (serverSpawnArgv) and the capability probe's two invocations carry `-f /dev/null` on every platform but Windows;
// the probe needs it because `list-commands` can start a transient server on reed's socket, and a config that creates a session would keep that server alive for the boot's `new-session` to join.
// A running server ignores the flag, and every other engine call keeps its argv.
// The skip covers the operator's whole `~/.tmux.conf` on reed's per-hub servers only, every option, key binding and hook in it, and leaves the operator's own servers alone.
// It takes effect at a hub server's next start, so a server an older lyx started keeps the operator's config until it exits.
// The hermetic default stands unless the `reed.yaml` key `tmux_conf` (variable `LYX_REED_TMUX_CONF`) names a tmux config file; empty, it sources nothing.
// A non-empty value is refused on a POSIX boot unless it names an absolute path to an existing regular file free of whitespace and the characters `* ? [ \ ' " ; # { } $`, the error naming the key, the variable and the path.
// The file is sourced once per server start, by the boot that started the server.
// Every POSIX boot reads the server's identity (`#{pid} #{start_time}`) before its spawn and again once its session is up, and the boot is the candidate for the source when the two differ.
// A candidate claims the server with the server option `@lyx_tmux_conf_claimed` in an `if-shell` check that waits on nothing, so exactly one of two concurrent candidates sources; the winner sets the option to `sourced`, or to `empty` when no file is named.
// The claim and the source run before reed's own option pins, so reed's values win over the file's.
// What the file cannot pass is the session and server lifetime: the spawn pins the new session `destroy-unattached off` and tags it `@lyx_reed_session`, and the guard ends by pinning `destroy-unattached` globally and `exit-unattached` on the server `off`.
// The guard invocation's exit status is only a warning, written to the booting command's stderr and logged with the path and tmux's own message, because one status carries the claim, the source and both pins.
// The decisions are separate read-back round trips: the boot fails when its session is gone, naming the key, the variable, the path and the command to remove from the file (an untargeted `set destroy-unattached on`, a `kill-session`), or when either pin does not read `off`.
// Nothing re-sources the file into a running server, so a changed value takes effect at the server's next start, and a starter whose boot ends between spawn and guard leaves the server unclaimed and file-less until then.
// Only sessions that count as reed's are seen by reed.
// On a server whose claim is `sourced`, a session counts when it carries `@lyx_reed_session`; on any other server, every session counts, so Windows and servers started before the claim existed behave as before.
// Every session read applies the rule: `list-sessions -f` through listSessionsArgv in Go (the watchdog's discovery, the pair teardown, the last-session check that ends the server, the session-less holder and zombie checks, the recorded-session classification and the next/previous session switch), and the same condition in the status bar's session row.
// Teardown therefore ends the server, and any session the file created with it, when no reed session remains, and so does a boot refused because the file ended its session.
// The bound: such a session is hidden from reed and lives until the server ends.
// Windows parses the key and does none of it: psmux reads its own config at start, and the filter flag is never passed to its `list-sessions`.
//
// Pane enumeration: listPanes (overlay.go) always runs
//
//	list-panes -t <strand window> -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid} #{pane_title}"
//
// and parsePaneList (parse.go) parses each output line's first six whitespace-separated fields positionally, in that exact order, into a LivePane;
// everything after the sixth field is LivePane.Title, so a title holding spaces survives.
// The title is a display mirror of the strand's full name, never a lookup key.
// #{pane_dead} is reported as the string "1" or "0";
// parsePaneList keys a dead pane on the literal value "1", never a numeric
// or boolean comparison.
//
// Strand window: the strands live in one window of the session, and every op that enumerates, lays out or measures them addresses that window by its id ("@<n>"), never by the session's current window.
// A second window in the session is therefore never read as the strands' window.
// The one seam, windowtarget.go's TmuxCmd.strandWindowTarget, lists the session's own panes across all its windows (`list-panes -s -t '=<session>' -F "#{pane_id} #{window_id}"`).
// It resolves the window of the recorded Selvage pane (alive, or a dead corpse), else of any present recorded strand pane left in the shell window by an older lyx, else the current-window target "=<session>:", which only the seam keeps.
// A session reap (Down, and the engine-less ReapSession) ends every window, so it lists the panes of all of them (`list-panes -s`) and reads no reed state.
// A recorded pane id counts as present only when that session-scoped listing carries it, since pane ids are server-wide and a stale id can name another session's pane.
// With a present Selvage or strand pane a batten window, current or not, is never read as the strands' window and its pane is never reaped as untracked.
// A session's first up has neither, so it falls back to the current window, and the Selvage pane created there defines the strand window from then on.
// An own-window strand lives outside the strands' window: its pane is never read as that window, and its liveness is read through the session-wide listing (ownwindow.go's windowPane) instead.
// Reconcile ends only a window carrying @lyx_own with no pane bound to a strand; a window without the marker is never ended.
// The pane-generation read (generation.go) stays session-scoped and outside the seam: it reads session fields only, so it keeps the "=<session>:" form, which expands them, where the bare "=<session>" form expands every session field to empty (verified live, tmux 3.6).
//
// Window order (windoworder.go): a session's windows read, in index order, the overview window, the agents' own windows in start order, then the window named shell, then the hidden windows.
// Every window is created through newWindowLocked (spawn.go) as `new-window -d` plus placement arguments, listed from `list-windows` in index order under the op lock:
// an own window opens `-a -t <previous window>` for a relaunch, which keeps its window's place and ends the old one behind it, else `-b -t <shell window>`, else `-a -t <highest-index non-hidden window>`;
// the overview window opens `-b -t <lowest-index window>`, hidden windows included; a hidden window opens `-a -t <highest-index window>`; with nothing to place against the plain form `-t '=<session>:'` is used.
// A placed new-window the multiplexer refuses on Windows (psmux) logs at Debug and is retried once in the plain form, so the order is best-effort there and nothing fails.
// The shell window is the window of the recorded console pane (Selvage), named shell with automatic-rename off, which is how a fresh boot names its one window.
// A session booted by an older lyx is brought into that order by ensureShellWindowLocked, which runs wherever a window opens, once the boot marks the strands' window and at the head of every attach pre-flight, also when no client size is known:
// it renames the shell window when its name is wrong and moves it behind the last non-hidden window, and only that window; a strand pane an older lyx left in the shell window is broken out by the reconcile head's move rule.
// It does nothing, logged at Debug, when no console pane is recorded or the window listing does not carry its window, and a check never fails its caller.
// A failed rename or move warns once per failure episode, which the session option @lyx_shell_check_failed keeps open, logs at Debug while the episode stays open and always on Windows, and an Info line closes the episode when a later check succeeds.
//
// Detached windows: OpenWindow (window.go) opens a detached, named window in the worktree's reed session running a `lyx` command, for a caller that wants a long-running command to live in the session without a status-bar button.
// One multiplexer invocation runs `new-window -d -a -t <highest-index window> -P -F '#{window_id}' -n <temporary> <command>`, or in the plain form `new-window -d -t '=<session>:' -P -F '#{window_id}' -n <temporary> <command>` when no window is listed, chained with `set-option -w -t '=<session>:=<temporary>' remain-on-exit off`, `set-option -w -t '=<session>:=<temporary>' @lyx_hidden 1` and a final `rename-window -t '=<session>:=<temporary>' <name>`.
// The window is detached and never selected, so it takes no focus.
// The options are chained by exact name because one chained after a detached new-window without a target lands on the current window, which can be any window since attach and the focus rules select agent and overview windows, and whose remain-on-exit must stay on.
// The name is a per-call temporary one no other window holds and the rename comes last, because an own window may share the requested name and the options would otherwise land on it.
// The status bar skips a window carrying @lyx_hidden, and window placement and the shell check read the flag to find the last non-hidden window; no focus rule reads it.
// The op reads remain-on-exit back from the new window by the printed id, and a window that does not read off is killed by that id with ErrWindowReadBack.
// A window of the same name whose pane is live is returned as existing and nothing is started; one whose pane is dead is killed and replaced.
// A window carrying @lyx_own is skipped by that lookup, so OpenWindow never adopts, kills or replaces an own window of the same name.
// The command line is composed in panebin.go from the pane-binary prelude, so `lyx` resolves to the spawning binary.
// Bound: the op only chooses where a command runs; the window lives as long as the reed session, and the session's other windows keep remain-on-exit on.
//
// Own windows: an own-window strand gets a detached window named by its role, opened through the launch chokepoint (spawn.go's openOwnWindowLocked) by `new-window -d`, and the window is then selected only under the focus rule below.
// The window carries @lyx_own, @strand_color and @lyx_unseen, each set by window id; the last is left unset on Windows, and an overview window carries neither @strand_color nor @lyx_unseen, nor does a new agent window the focus rule selects.
// Its pane runs the launch tail every strand pane runs.
// Its dead pane stays until the strand is removed or relaunched; a relaunch kills the old pane once the new one exists, so a shared window never keeps a dead pane beside the relaunched one.
// Resume reopens the strand detached.
// UpdateStrand refuses a move into or out of the anchor, since the pane would sit where the new display does not expect it.
// A strand declaring a window group (Display.Window, sized by Display.WindowSize) shares one window with its group while the declared size is at most window_share_max; every other own-window strand gets a window to itself.
// The group's first strand decides, and ReedState.WindowGroups records the choice and the size until the group's last strand goes, so a later window_share_max edit never splits a group across shared and separate windows.
// A strand declaring another size than the record's is refused with both sizes named.
// A strand arriving once the group's recorded size is already held, a re-stepped seat added before its predecessor is removed, gets a window of its own and leaves the shared window as it is.
// A pane joins the shared window by a detached split in the window_split direction, so an operator viewing the window keeps the active pane, and the window is then laid out evenly in that direction.
// Removing an own-window strand moves a viewer on its window to the relevant window before the window ends, never the shell, and never moves a viewer on another window.
// A shared window a strand leaves is laid out again.
// An invalid window_split or window_share_max never refuses a removal: the layout is skipped and the key is reported in Removed.ConfigWarning.
//
// Focus rules: an agent window is a window holding a pane bound to an own-window strand; the overview, the shell and hidden windows are not agent windows.
// The follow state is the session option @lyx_follow (followSessionOption): boot sets it on off Windows, and it is off only while it reads off, so an unset option, as on Windows or in a session booted by an older lyx, reads on.
// followingLocked reads it, and a failed read counts as off for that one decision and is logged at Warn, so a broken read never moves a viewer.
// It is the one decision-bearing session option reed reads.
// An automatic window switch happens only at three points, each deciding from reed state, window ids, strand roles, pane liveness and the follow state alone, never from @lyx_unseen, @lyx_hidden or a wait mark.
// relevantWindow is the target of two of them: among the listed windows not about to end, the highest-index agent window holding a pane that is not dead, else the overview strand's window, else none.
// Attach (attachWindowSelectionLocked, resolved by AttachArgv right after the shell check) appends `; select-window -t <relevant window>` to every argv it returns, and appends nothing when a listing fails or no window is relevant.
// Beside another attached client, or when the client listing fails, it appends the selection only while follow reads on, reading the follow state only then and never writing it, so with follow on an attach moves every client of the session to the relevant window, including one on a window it reached outside reed's bindings, and a failed follow read counts as off.
// A new agent window is selected by openOwnWindowLocked, for an own-window strand and never the overview, when newAgentWindowSelected holds: always for a Focus strand, never for one whose display sets only KeepViewer, otherwise exactly when the follow state reads on; it then carries no @lyx_unseen.
// With follow on, a new agent window takes the viewer wherever it is, the shell included; with follow off, it leaves the viewer where it is.
// Opening a window never changes the follow state.
// FollowRun is the one engine operation that turns follow on, beside the engine-less FollowSession: under the operation lock it sets the option on and selects the relevant window, the newest live agent window, else the overview, else nothing, for whichever client views the session.
// It persists no state, a failed option write is logged and the select still runs, and AddStrand, ReplaceStrand, Resume and Up never call it.
// Every client of a session shares its current window, so the one select-window moves each attached client.
// A viewed window ending, by RemoveStrand, by ReplaceStrand and by reconcile ending an orphan own window (selectRelevantWindowIfCurrentLocked), moves a viewer on it to the relevant window among those left, never the shell, and with none left to tmux; this move neither reads nor changes the follow state.
// A viewer elsewhere is never moved, and a strand leaving a shared window that stays open ends no window and switches nothing.
// An automatic switch races an operator's own window change by at most one switch.
//
// Overview anchor: the overview anchor places one strand per session, alone in a window of its own named overview that opens at the lowest index and never joins a group.
// Everything that reads a window of its own, reconcile, presentPanes, the strands' window resolver and the relaunch's old-pane kill, treats an overview pane as an own-window one (isOwnWindow); only the placement, the name, the unseen mark, the one-per-session refusal and the focus rules ask isOverview.
// The window carries @lyx_own like an own window and sets neither @lyx_unseen nor @strand_color, so its button takes the plain window-button style.
// An overview display takes no window group, and UpdateStrand refuses a move between the overview and own-window anchors.
// AddStrand refuses a second overview strand while another strand, live or dormant, holds the anchor, naming that strand and lyx reed remove.
// ReplaceStrand removes the old strand before its add, so replacing the overview strand itself is never refused; replacing another strand with an overview strand is refused only while a third strand holds the anchor.
// A replacement overview strand that ends the window the operator is viewing selects its own new window once the launch succeeded; a replacement whose old strand shared a window that stays open ends no window and switches nothing.
//
// Session targeting: every -t argument that names a SESSION is passed in
// an exact-match form — "=<name>" for session targets (has-session,
// kill-session, attach-session) and "=<name>:" for window/pane targets
// (list-panes, select-layout, display-message, plus the set-option pins'
// window-targeted -t), since the window/pane target parser rejects the bare "=<name>"
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
// has-session, split-window, new-window, select-layout, select-pane, send-keys,
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
// set-hook is different: it is deliberately absent from requiredSubcommands, because gating the capability probe on it would take every reed verb down on a psmux lacking it, over the quality-only unseen-clear hook and the window-resized unpin that already degrade silently.
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
//     splits and ensureSelvagePaneLocked's Selvage rebuild both run the
//     shared validateSplitCreatedNewPane guard.
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
//     a reed session is either Selvage or a bound strand's pane, and the
//     untracked reap enforces that rule as `anyBoundPresent || policy.authorizesReap()`,
//     where the Selvage anchor requires ALIVENESS rather than mere
//     presence — launchStrandLocked makes the gate fire from AddStrand and
//     UpdateStrand, neither of which calls ensureSelvagePaneLocked to heal a
//     Selvage corpse first, so a dead Selvage id must never itself authorize
//     sparing the untracked set. The reap runs before pane allocation at
//     one chokepoint inside launchStrandLocked, so the property holds by
//     construction on every realization path rather than requiring two
//     call sites to stay in sync. Two consequences follow: an `up` against
//     a session with zero tracked strands ends up Selvage-only and
//     full-height; and RemoveStrand's reconcileApplyPersistLocked tail
//     inherits the gate, so removing the last strand reaps any untracked
//     alive pane in the same verb.
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
//     strand returns 0, then a subsequent add finds the session already up
//     through ensureSessionLocked's own liveness probe and boots nothing —
//     which can only hold if the session survived the prior remove — and
//     yields a live second strand). has-session and
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
//     process's whole life, so a long-lived process such as the daemon
//     holds a frozen WorktreeRoot that a `mv` of the
//     worktree makes stale. Every operation therefore re-checks that told
//     worktree root's liveness at the op-lock chokepoint, and refuses
//     rather than creating substrate under a path that is no longer a
//     worktree. One user-visible consequence is a deliberate behaviour
//     change in standalone mode: a `--target-dir` naming a directory that
//     does not exist is now refused at the first engine op, instead of
//     proceeding and deriving a state directory for it.
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
//   - Duplicate-pane-cell session destruction (reconcile.go's clearConflictingPaneBindings):
//     select-layout does not REJECT a layout string that
//     names one pane TWICE — it accepts it (exit 0), assigns cells
//     positionally, and destroys every pane the resulting short cell list no
//     longer covers (verified live, tmux 3.6: one `up` reduced a two-pane
//     session to one and reported ok:true). Nothing reed constructs can
//     produce such a table — every split result passes
//     validateSplitCreatedNewPane — so the
//     source is always a CORRUPT persisted table: a restored backup, a hand-edited
//     or partially-restored reed.json (R5 review finding R5-F3). The engine
//     clears such bindings at the single load chokepoint.
//   - Async kill-server / probe-always-exits-0 (lifecycle.go): kill-server
//     returns before the server process (and its "__warm__" helper) have
//     actually released the -L socket, and no probe — list-sessions,
//     kill-server itself — can distinguish "no server" from "server dying
//     asynchronously", since both exit 0 either way — so Down/reap logic
//     waits on the underlying OS process actually exiting rather than
//     trusting any CLI exit code as a death signal.
//   - Socket-file removal (socketfile.go, lifecycle.go, overlay.go):
//     a killed tmux server can leave its `-L` socket file behind.
//     ensureServerGoneLocked removes it once no tmux process remains on the socket, on both the graceful and the force-reap path,
//     and Down and every engine-bound teardown reach it through that one method.
//     EndSessionByName has no engine, so after it kills the server of the last session it retries the removal for a bounded wait while the server stops answering.
//     The path is resolved as tmux does (`$TMUX_TMPDIR`, else `/tmp`, then `tmux-<uid>/<key>`) and removed only when it is a socket, nothing answers a connection on it and a second Lstat reports the same file.
//     The bound: only the file for the key just killed, only inside that teardown, never a glob or sweep of the directory.
//     A server restarting on the same key between the check and the removal is the accepted residual race.
//     A removal failure is logged at Debug and never fails the teardown.
//     It is a no-op on Windows, where psmux keeps no socket file.
//     The boot path also measures the socket path, with the base's symlinks resolved, before any tmux round trip, and refuses one over the OS limit (107 bytes on linux, 103 elsewhere) naming `TMUX_TMPDIR` as the way forward.
//     The bound: the check runs on the boot path only, so a verb run later under a longer `TMUX_TMPDIR` than the server booted with still gets tmux's own error.
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
//   - The dot-fill artifact's mechanism (windowsize.go, attach.go): the dots are painted by tmux
//     itself, in the region of an attached client's terminal that the current window geometry does
//     not cover or whose paint has gone stale relative to a just-changed window size. They are
//     never content reed writes and are in no pane's grid — which is also why capture-pane against
//     the reed session is structurally blind to them. Both reported triggers are one mechanism, a transient or standing mismatch between a
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
//     remove the uncovered subset, and reed ships no repaint entry: removing it would require changing
//     window-size, which is refused for stronger reasons (see the next bullet).
//   - Why window-size latest is not changed, made configurable, or conditioned on the client count
//     (windowsize.go): with two clients of different sizes attached, tmux must pick one window size,
//     so some client is always mismatched under every available policy; latest gives the artifact to
//     the client the operator is not currently using, which is the best of the three.
//     Rejected: largest shows tmux padding to every operator with a second client open; smallest turns an intermittent artifact into a
//     standing one; manual abandons client-following.
//   - The orch pane under the same policy:
//     a smaller client attached to the orch session can shrink the window below the rows the input box needs,
//     and the orch watcher's idle probe then reads no box and records the pane as too short (internal/orchengine's State.Stuck), holding every cycle.
//     The policy is kept rather than changed or conditioned for the orch pane, since no policy avoids a mismatch with two clients of different sizes.
//     The window returns to the larger client's size at that client's next keystroke, which clears the hold at the next passing probe.
//   - The attach-time multi-client warning (attach.go's warnMismatchedClientsLocked): AttachArgv's
//     pre-flight lists the session's attached clients and emits one logger.Warn per client whose
//     size differs from the size this attach was told, naming that client and both sizes; zero
//     differing clients produce zero lines. It never blocks and never reaches the JSON envelope;
//     it reports whether another client is attached, a failed client listing counting as one, and
//     another attached client, or a failed listing, drops the attach window selection from the argv. This is the primary deliverable for the cross-client trigger rather
//     than a consolation prize, because that trigger's dots are the uncovered subset and cannot be
//     repainted away — the residual's real operator cost is bewilderment, and the warning turns a
//     mysterious artifact into a logged, searchable fact at the moment the operator creates the
//     condition. The target-form rule: list-clients -t takes a SESSION
//     target, so warnMismatchedClientsLocked uses the bare "=<name>" form.
//   - list-clients stays out of requiredSubcommands (probe.go): it is
//     geometry-quality only, exactly like the status/window-size pins the Shared Decision
//     geometry-tmux-failures-are-non-fatal-everywhere already exempts from fatality. A failing or
//     unsupported list-clients logs one warning and emits no multi-client warning
//     (warnMismatchedClientsLocked). Requiring it would make a multiplexer that runs reed perfectly well
//     today fail at boot over a cosmetic feature.
//   - The attach argv (attach.go): AttachArgv's argv is the bare "attach-session" argv, with a "; select-window -t <relevant window>" tail when the window selection applies, the separator a literal one-character ";" argv element — never "\;", since exec.Command passes argv directly to the child and no shell ever sees it to unescape.
//     attach-session is first in the chain, so a failing select-window still leaves the operator attached.
//   - The geometry option pins (windowsize.go): pinGeometryOptionsLocked pins the two-line status bar, whose format strings bar.go declares: "status-format[0]" (the window and shell buttons) and "status-format[1]" (the session buttons and the time), both with -g since tmux's status formats are one array; "status-style" (-g) with the dark grey statusBarStyle, so the gaps between the buttons are not tmux's default green; "status" "2" and "status-position" "bottom" on the session; and "window-size" "latest" (-w) on the strand window.
//     The window-targeted pin is targeted at the strand window (-t <strand window>, per the Strand window and Session targeting grammar above), both at boot and again in AttachArgv's pre-flight.
//     The bar reads strands only through the @strand and @strand_color pane options and the @lyx_strands window option, never through the current window.
//     The same function clears the `window-resized` hook an older binary pinned on the shell window, with `set-hook -u` in the window scope and the session scope, everywhere but Windows, so boot and attach both clear it.
//     Unlike the remain-on-exit/mouse pins beside
//     them, every pin here is NON-FATAL: those two are correctness dependencies,
//     these are geometry-quality options whose absence degrades to a working session, and psmux's
//     support for them is unverified anywhere in this repo (Shared Decision
//     geometry-tmux-failures-are-non-fatal-everywhere).
//   - The watchdog daemon's own stderr is watched by no one
//     (reedcli/watchdog.go): the daemon points the logger's durable sink at
//     fabricengine.HubLogsDir(hub) FIRST, before it takes its lock — the
//     ordering is what keeps its diagnostics reachable at all, since nothing
//     reads a detached process's stdio.
//
// requiredSubcommands (probe.go) does not grow for the option pins or the window-resized unpin: display-message, set-option and list-panes are already required, and set-hook is absent from the set on purpose, its failure being logged and never fatal.
package reedengine
