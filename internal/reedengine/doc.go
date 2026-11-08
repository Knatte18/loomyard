// doc.go carries the package-level godoc comment for reedengine.
// It holds no code, only the package's role and contract.

// Package reedengine is the domain kernel for lym's tmux window manager.
// It holds the tmux subprocess overlay, strand bookkeeping, persisted state and config.
// Its operations layer holds the lifecycle verbs that compose them.
// It is the "dumb carrier" for its caller's strand data:
// it stores every field a caller writes into a strand and reads none of them semantically.
// A strand has no domain `type` field:
// `cmd` and `resumeCmd` are opaque strings that reedengine never parses or branches on.
// `--role` and `--round` are formatting-only inputs.
// They are consumed once at add-time to fill the strand-name template and are never persisted or read back.
//
// reedengine imports internal/reedengine/render (the pure display-vocabulary leaf: Anchor, Display and ValidateDisplay).
// render never imports reedengine, so the import graph stays acyclic (reedcli -> reedengine -> render).
//
// reedengine is told its geometry as a Geometry value (geometry.go) and derives none of it.
// internal/lymcwd is absent from its direct production imports.
// hubgeom.ReedGeometry builds a Geometry from a resolved *lymcwd.Location for every hub-mode caller.
// internal/lymcwd stays in `go list -deps ./internal/reedengine` through internal/logger,
// so the absence means that reed never resolves its own coordinates.
//
// reedengine is told, as an EnvScrub passed to New, the environment keys its tmux server spawn must not inherit.
// The server is shared by every strand, so the teller passes the keys of every registered engine.
// reedengine carries no provider's names itself.
//
// internal/fabricengine is a direct production import because SpawnWatchdog calls fabricengine.HubScratchDir.
// The Told-Geometry Invariant bars a direct internal/lymcwd import.
// It requires that an engine be handed the absolute paths it operates on.
// The one fabricengine call takes hubPath as a parameter and derives nothing from it,
// so the seam is told its geometry as the rest of this package is.
//
// One more invariant this package enforces: exactly one named tmux server per hub.
// The server name is derived from the hub path (ServerName),
// so every worktree under the same hub shares that one tmux server.
//
// A strand's Retiring flag is reed's own lifecycle bookkeeping, not a caller field:
// `reed remove --detach` sets it through Engine.MarkRetiring before spawning the detached remover,
// so a caller can tell a strand about to remove itself from a live one,
// and the remover no-ops on a guid that is already gone.
// EndSessionByName is the engine-less session end for a pair whose worktree is gone.
// It kills the server when it ends the last session.
//
// Engine.SetWaitMark marks a strand's pane as waiting:
// it sets the pane user options @lym_wait (the label) and @lym_wait_start (epoch seconds);
// an empty label unsets both.
// The mark is display only:
// reed stores nothing for it and the options die with the pane, and no Go decision reads it (PATTERN-reed-panes).
// The status bar renders it on the strand's own button, as " ⏳<label> <elapsed>m" appended to the strand's name;
// tmux computes the elapsed minutes at each refresh.
// The options are set only through SetWaitMark.
// The landing wait-mark callback marks the verify and the parent lock wait with the label `parent lock <branch>`.
//
// Engine.SetCheckMark marks the session with a check's progress.
// It sets @lym_check (the text) and @lym_check_state (running, green or failed) as session user options.
// It also sets @lym_check_start, the epoch seconds the step in flight started, or leaves it unset for none.
// An empty text unsets all of them.
// Like the wait mark it is display only and dies with the session;
// line 2 of the status bar renders the current session's mark right-aligned before the clock, styled by its state;
// tmux computes the seconds since the start at each refresh.
// The adopt check sets it on the prime's session.
//
// Engine.CapturePaneStyled returns a pane's screen with its SGR attributes, for a caller that reads cell styles.
// Reed interprets none of it.
//
// Segment colors are read from reed.yaml's segment_colors block and resolved only through Engine.segmentColor;
// an invalid one is refused only at boot.
// The buttons block of reed.yaml is carried as an uninterpreted node and decoded only at render.
// The geometry tells the button context through Geometry.Buttons, nil in standalone mode.
// buttons.go is the one decoder of button entries and applies the validity rules:
// a non-empty label and run;
// no '#', '%' or control character in a label, confirm text or ask text;
// no ',' in an ask text;
// ask_default and {answer} only with an ask;
// a window of agents or none;
// and {strand} only with window: agents.
//
// The action row (buttonrow.go) is rendered by pinGeometryOptionsLocked at boot and at every attach pre-flight.
// The render is one tmux invocation of set-option commands on the session.
// composeButtonRowLocked takes reed's entries for the session's place first and the machine's after them.
// It numbers the valid ones in that order and draws entry N as the user range lymbtnN.
// @lym_buttons_plain holds the entries without window: agents, and @lym_buttons_agent holds every entry;
// a window whose active pane carries @strand draws the second.
// @lym_button_spec holds the JSON buttonSpec a click reads.
// It carries the configured shell and its command flag, and the run directory.
// Per entry it carries the label, the run, confirm and ask text (static placeholders filled) and the ask_default.
// It also records whether {strand} is left for the click.
// An entry whose filled ask text holds a ',' is left out of the spec,
// and a click whose ask text holds one after the {strand} fill raises no prompt.
// @lym_button_dir holds the run directory as one word quoted in reed's shell dialect.
// The run directory is the beam's anchor for a task, else the prime's anchor when the beam is not on this machine;
// for the prime and a beam it is the session's own anchor.
// A standalone geometry, a failed context, an unresolvable shell and a failed beam lookup render empty options;
// line 3 then shows follow alone.
//
// SwitchClient (switch.go) is the engine-less function behind `lym reed switch`.
// It addresses a told server by socket path and issues only list-sessions, display-message and switch-client on it.
// It is the only reed code that issues switch-client.
//
// FollowSession (follow.go) is the engine-less function behind `lym reed follow`.
// It addresses a told session of a told server by socket path.
// It issues list-panes, set-option @lym_follow on and, when a window is relevant, select-window, and nothing else.
// It reads no working directory, config or state, so it picks the window from tmux facts alone:
// the highest-index window carrying @lym_own and not @lym_overview that holds a pane that is not dead;
// else the lowest-index window carrying @lym_overview; else none.
// openOwnWindowLocked sets the window option @lym_overview beside @lym_own on an overview window.
// That marker lets the rule tell the overview from an agent window.
// An overview window opened by an older lym carries no marker and counts as an agent window;
// it can win only when no other agent is live, the same outcome as the overview fallback.
// A failed listing, which is how an unknown session answers, returns an error before any write;
// a failed option write returns before any select;
// and a failed select returns an error naming the window.
//
// RunButton (buttonclick.go) is the engine-less function behind `lym reed button`.
// It addresses a told session of a told server by socket path.
// It reads the session's @lym_button_spec and runs the entry the clicked range `lymbtnN` names.
// It reads no config and no reed state, and it returns nothing:
// a failure is shown on the clicking client with display-message and is logged.
// The checks come in this order:
// a spec that is missing, empty or not JSON;
// a range with no entry;
// a `{strand}` entry whose name cannot be read;
// an answered `ask` entry whose answer cannot be read;
// a run directory that is gone;
// a run directory whose worktree's anchor does not resolve;
// an `ask` entry whose ask text holds a ',' after the `{strand}` fill.
// A `{strand}` entry reads the clicking client's active pane for @lym_strand_name.
// The entry runs in the current anchor of the worktree holding the rendered run directory.
// The anchor is read at the click through the anchor reader RunButton is told:
// the render is frozen until the next attach while an anchor marker can change after it,
// and a lym verb refuses anywhere but its worktree's anchor.
// An entry with an ask text raises `command-prompt` on the clicking client.
// Its template stores the answer, escaped by tmux's `%%%`, in the session option @lym_button_answer.
// The template then runs a line (composeButtonReentryLine) that opens in that anchor.
// The line calls the verb again with `--answered`, the hash of the entry's run text and the strand name.
// The answered call reads the option and takes the entry's ask_default for an empty answer.
// It fills `{answer}` with the answer, shell-quoted in the run text.
// An Escape at the prompt runs no template, so the click ends there.
// An entry with a confirm text then raises `confirm-before`.
// Its line calls the verb again with `--confirmed`, the hash, the strand name and the answer.
// Both lines discard the output of the directory change and of the verb and end with status zero.
// An answered or confirmed call runs the entry only when its hash still matches, so no run text crosses tmux's parsing.
// @lym_button_answer is decision-bearing: it is what the answered call fills `{answer}` with.
// The entry runs as `<shell> <flag> <run>` in that anchor (runButtonScript) and waits with no timeout.
// It logs its spawn and end at Info with its exit code and combined output.
// It sets no caller identity and is registered in no process table.
// A non-zero exit or a failed start shows `lym: <label> failed (exit <code>): <first output line>`.
// The verb runs only what the session's spec holds at the clicked index;
// anything that can run it against the socket can run an entry as itself;
// the verb adds nothing over running the command directly.
//
// Every strand pane carries these user options:
// @strand, the role segment of its name, or the full name when it does not parse;
// @strand_color, the tmux color of its segment, unset when it has none;
// and @lym_strand_name (strandNamePaneOption), the strand's full name, unset when the name is empty.
// The strand window carries @lym_strands.
// Launch sets the pane options and boot marks the strand window.
// Every attach pre-flight marks the window and re-asserts the pane options on every strand bound to a live pane,
// so a session or strand spawned by an older lym gets them at its next attach.
// The Selvage pane carries the user option @lym_console (consolePaneOption).
// It is set when the pane is split and re-asserted by every attach pre-flight.
// The options are display only but @lym_strand_name:
// the status bar and the key bindings read them and no Go decision does.
// Selvage never gets @strand or @strand_color.
// @lym_strand_name is the one pane option a Go decision reads: a button click fills {strand} from it.
// The session options @lym_follow, @lym_button_spec, @lym_button_dir and @lym_button_answer are decision-bearing;
// @strand, @strand_color, @lym_unseen, the wait marks and the two action row options stay display only.
// A strand's Segment is an opaque field its spawner names through AddSpec.Segment.
// Reed reads it only to look up the segment's color.
// The strand that an add or replace returns, and Status, carry that color as Color.
// An own window gets a status-bar button on line 1 in its segment's color.
// The button reads the window options @lym_own and @strand_color.
// It is labelled with the window name and carries its panes' wait marks.
// The button is lit while the window is current.
// Otherwise it shows that color on dark grey, underscored while the window carries @lym_unseen.
// Outside Windows, the geometry pins also pin the session hook `session-window-changed`.
// The hook runs `set-option -w -u @lym_unseen`.
// Selecting the window thus clears the mark.
// Boot and every attach pre-flight run the pin.
// On Windows no hook is pinned and the mark is never set.
// The shell button is a range=user|shell button labelled shell, lit while the shell window is active.
// Line 1 follows window index order:
// the overview windows' buttons come first, the agents' follow in start order, and the shell button comes last.
// An overview window's button is an own-window button with no @strand_color.
// It renders as the plain window-button style.
// A window carrying @lym_hidden gets no button, and no Go decision reads @strand_color or @lym_unseen.
// Line 2 lists the sessions, then, right-aligned, the check mark and the time.
// Line 3 starts with the follow button, then the slack and away toggles, then the action row.
// The follow button is a range=user|follow button labelled follow.
// It is lit green while the current session's @lym_follow is not off and unlit while it is.
// The toggles are range=user|slack and range=user|away buttons, each lit green while its state is on.
// They read the machine's Slack state, which is no session's, from a `#()` job:
// the line `lym slack bar` that the server-global option @lym_slack_state_command holds, pinned with the bindings.
// slack is lit while the state line holds SlackToggleLit, and away while it holds AwayToggleLit.
// tmux runs the job again at each redraw a second or more after its last run.
// The geometry pins set the redraw interval (status-interval) to five seconds.
// So a change made by any route, a Slack text or a hook included, shows within that interval.
// A toggle's click redraws its client's bar at once.
//
// The navigation bindings (bindings.go) are root-table bindings on the hub's reed server.
// pinBindingsLocked issues them at the end of the geometry pins, so boot and every attach pre-flight pin them.
// The pin stamps the server-global option @lym_bindings_build with the build's VCS commit time and revision label.
// A binary with no VCS stamp unsets it instead.
// Every op under the blocking op lock reads the stamp before its step.
// The op re-pins the bindings when the stamp is absent or unparseable.
// It also re-pins when the stamp names another build whose commit time is not after the running one's.
// So a deployed lym reaches a running server at its first reed op.
// Bound: an older build never re-pins over a newer one's stamp.
// A long-lived process of an older lym therefore cannot restore stale bindings, and an unstamped binary never re-pins.
// Two builds of one commit time, such as a dirty build and its commit's clean one, can re-pin over each other.
// The name-repair loop never re-pins.
// The global status-bar options refresh at boot, at attach and with the bindings' stale-build re-pin.
// That re-pin pins the status formats and the bar style before the bindings.
// Bound: between an older binary's boot or attach and the newest binary's next op, follow can show on lines 2 and 3;
// a session an older binary boots or attaches keeps status 2, without line 3, until the newest binary next attaches it.
// The re-pin restores only the global options.
// The pin starts with `unbind-key -n M-z`, which removes the Alt+z an older build bound.
// A `bind-key` re-pin never removes a binding, and reed binds no zoom key.
// Alt+Up and Alt+Down step to the previous or next window in index order, in every window.
// The step wraps with tmux's own window wrap.
// A step that lands on a window carrying @lym_hidden goes on:
// forward to the session's first window, the lowest-index overview, and back to the strands' window, the shell.
// That is one conditional step, since hidden windows sit after the shell.
// On psmux, where the order is best-effort, a step may land on a hidden window.
// Alt+Left and Alt+Right run `lym reed switch --prev|--next` in the background through run-shell.
// The line names the absolute path of the `lym` that pinned them (panebin.go's composeSwitchCommand), never PATH's.
// A left click on the status bar dispatches on its range:
// the shell button selects the strands' window;
// a window button selects that window;
// a session button switches the client to that session;
// the follow button toggles the follow state;
// a button range named by buttonRangePrefix runs `lym reed button`;
// and a Slack toggle runs `lym slack toggle slack|away` from the session's run directory.
// That line (panebin.go's composeSlackToggleCommand) rides the server-global option @lym_slack_toggle_command.
// It is pinned and unset together with the Slack state line, apart from the other lines.
// It carries the socket, tmux and client the verb redraws through ShowToggleOutcome.
// Neither Slack line is pinned when the resolved binary is a running test binary.
// tmux runs the state job at every redraw.
// A test binary given `slack bar` runs its whole suite, whose sessions would pin the same job again.
// The button line (panebin.go's composeButtonCommand) rides the server-global option @lym_button_command.
// The option is set when the line resolves and unset otherwise,
// and the one global binding resolves it through `#{E:...}` with the session's own options.
// It changes into the session's run directory, read from @lym_button_dir.
// That is needed because tmux's run-shell format-expands its command but not its start directory.
// It then runs the verb, discarding the output of both and forcing a zero status.
// So view mode stays shut on every path, a gone run directory included.
// Bound: a click runs only the line the binding names, which runs only the spec entry at the clicked index;
// a click whose pinned binary is gone runs nothing, logs nothing and shows no message;
// that lasts until a reed op of another build not older than the pinned one re-pins the bindings.
// A run directory removed after the render leaves the verb in the server's directory.
// There it shows the gone-directory message.
// The status bar draws no button for a strand pane, so a click selects no pane.
// The bindings that change window by hand write the follow state off.
// Alt+Up and Alt+Down start with `set-option @lym_follow off`.
// The write also lands when the step wraps onto the current window.
// The shell and window clicks start with the same write, also for a click on the current window.
// Each write is untargeted, so it hits the session of the client that pressed the key or clicked;
// Alt+Left, Alt+Right and the session click leave the follow state alone.
// The follow button writes off when the state is on and changes no window;
// from off it writes on and, when the jump line is pinned, runs `lym reed follow` in the background through run-shell,
// so the viewer jumps back to the run.
// The jump line (panebin.go's composeFollowCommand) names the absolute path of the `lym` that pinned it.
// It names the clicking client's session as `#{q:session_name}`.
// The line rides the server-global option @lym_follow_command, which the click runs through `#{E:...}`.
// That keeps the line out of tmux's nested command-string quoting.
// pinBindingsLocked sets that option when the line resolves and unsets it otherwise,
// so a click from off then only writes follow on and the next agent window takes the viewer.
// A psmux without bind-key or run-shell pins none of these, never writes off, and its sessions always follow.
// Bound: the bindings only ever write off, and the follow button on, to the clicking client's session,
// and the button runs one `lym` verb whose effect FollowSession bounds;
// a window change outside these bindings leaves follow on.
// Bound: these four keys and the left status click are taken from every pane on the server.
// A program in a pane, Claude Code included, never receives them;
// no other key or mouse binding is changed, and tmux's right-click menus keep their defaults.
// The shell branch finds the strands' window through the @lym_strands marker.
// The marker is expanded at click time to a tmux-issued window id (`@<n>`), never a config value or free text.
// Every binding is non-fatal:
// a failed `bind-key` is logged and the rest are still issued;
// an unresolvable executable, socket path or tmux path logs a named warning.
// That case leaves only Alt+Left and Alt+Right unbound.
// The capability probe checks bind-key, run-shell, if-shell and switch-client as optional verbs after the required set:
// a missing one logs one warning naming it and never fails the probe, so a psmux without them still boots.
//
// Reed keeps no zoom state: a step never zooms or unzooms a pane.
//
// A restarted server's sessions come back in spawn order (revive.go).
// The teller tells Geometry.SpawnOrder, a lazy list of ReviveEntry for the hub's worktrees in spawn order.
// It is nil in standalone mode.
// reedengine sorts nothing and reads no loom or fabric record.
// The revival predicate is one test a worktree evaluates only on its own state under its own op lock:
// its reed.json records a session, and that session is not live.
// A downed worktree has no such record, so it is never revived, and a first boot has none either.
// The steps that can create the session (Up, EnsureSession, Resume, AddStrandUnless) run through withRevivalFirst.
// The boot returns a sentinel and creates nothing when all of these hold where it would create its session:
// the skip is unset, a spawn order is told, and the predicate holds.
// The lock is then released.
// Each entry before the booter's own runs its Revive in list order, sequentially and outside the booter's lock.
// The whole step then runs again from its start under the lock with the skip set,
// so a session another booter created meanwhile is used as found and the trigger is evaluated once.
// No two op locks are ever held at once.
// Engine.Revive runs this worktree's ordinary up with the skip set when the predicate holds.
// That up creates the session and Selvage and relaunches no strand.
// Revive reports whether it ran the up.
// Bound: the step runs only for a revival of the booter's own recorded session, never for a first boot;
// it starts no strand and never recurses, since every nested up carries the skip;
// an entry whose Revive or whose list fails is logged and skipped, and the boot goes on;
// a live session in a later position is not reordered, since session ids cannot change without killing live sessions.
//
// A second package-level invariant: every session carries exactly one permanent pane beyond its strands.
// That pane is Selvage (ReedState.SelvagePaneID).
// Selvage is a first-class construct and never a Strand (Shared Decision header-is-not-a-strand).
// It is excluded from strand accounting.
// It is never the preferred split target, and it is outside both halves of reconcile's kill schedule.
// The exclusion seams are selvagepane.go's create/heal path and the reapPolicy value planReconcile is handed.
// Removing a session's last strand therefore never destroys the session or corpses its sole pane:
// Selvage keeps the session, and the substrate the next add needs, alive however many strands come and go.
// It boots alongside the session's initial pane on both Up and Resume.
// Selvage is created by a split-window call that carries e.cfg.Shell as its own trailing shell-command argument.
// new-session launches the session's first pane the same way.
// e.cfg.Shell is the operator's configured shell, `bash` on POSIX and `pwsh` on Windows by default.
// Selvage is an ordinary, typeable interactive shell.
// It is never a re-exec of lym and never sent a command of reed's own choosing.
// A strand pane keeps its split with no trailing command and starts the shell from the session's own pins:
// the boot and the attach pre-flight set `default-shell` and `default-command` on the session.
// Both are set to the resolved shell.
// They never set them globally, because one server serves every worktree of the hub.
// A non-empty `default-command` makes tmux start the shell without the login flag,
// so a strand pane runs the shell's rc files and not the login profile.
// The boot refuses a configured shell that does not resolve, naming the `shell` key;
// the attach pre-flight logs a warning and leaves the session's earlier values.
// Windows pins no session shell.
// A Selvage pane whose shell process dies (pane_dead=1) is kept by reconcile as an enumerable corpse.
// Reconcile never kills it.
// ensureSelvagePaneLocked heals it on the next Up or Resume:
// the corpse is killed and a fresh full-width Selvage is split in below the bottom-most live pane, carrying e.cfg.Shell.
//
// The module-local Selvage rules below live here and not in a PATTERN entry.
// They describe this package's own design, not a cross-cutting invariant another module could violate.
// Selvage is always physically bottom-most in the window.
// Every split site that creates or rebuilds it targets the bottommost live pane, never the topmost.
// Selvage is never a strand:
// it is excluded from ReedState.Strands, from strand accounting, and from every strand-only code path.
// The exclusion holds by construction and not by a runtime check.
// Selvage is never written to, cleared, or sent keys by reed:
// it is a real, usable terminal an operator can run lym and reed commands in.
// Reed's own writes stop at booting its shell.
// selvagepane.go implements all of them.
// selvagepane_enforcement_test.go is the mechanical check keeping them there.
//
// The placement rule: every placed strand gets a window of its own, a group window or the overview.
// The shell window holds Selvage alone, which fills it after boot, a heal and the move below.
// The anchors are own-window, overview and hidden (internal/reedengine/render); a hidden strand never owns a pane.
// `lym reed add` with no `--anchor` places the strand own-window.
// The retired anchors `below-parent` and `top` read as own-window when reed.json is decoded,
// and a write refuses them before any tmux call, naming the way forward.
// Reed arranges no panes in the shell window: it computes no layout, pins no pane height and keeps no resize hook.
// The only layout reed issues is the even layout of a group window, in the window_split direction.
// The only resize hook reed keeps is a group window's own, which issues that layout again.
//
// The move rule: the head of every reconcile (listReconcilePanesLocked) lists the shell window's panes.
// It breaks any pane a strand is bound to out into a window of its own.
// That upgrades a session an older lym left with strands split under a parent.
// The recorded Selvage pane and an unbound pane never move.
// Without a Selvage pane the last bound pane stays: breaking out a window's only pane would take the window with it.
// The shell window is found by the Selvage pane first, else by the @lym_strands marker.
// With neither, the current window is listed and nothing moves.
//
// Every strand pane reed creates resolves `lym` to the binary that spawned it.
// launchStrandLocked does it with shell statements:
// it prepends that binary's directory to the pane's PATH and exports LYM_BIN as the binary's absolute path.
// A fresh shell a strand's command starts re-runs its rc and can reorder PATH.
// LYM_BIN names the spawning binary there.
// A shuttle engine re-asserts LYM_BIN's directory in its agent's tool shell.
// After the prelude the line exports `proc.BuildServerOffEnv`,
// so no .NET build an agent runs leaves MSBuild reuse nodes or the MSBuild server holding threads after it exits;
// these exports stay when the prelude is dropped.
// The launch command is preceded by the exports of the variables the geometry's `PaneEnv` tells, in name order.
// They are read afresh at every launch and relaunch,
// so an agent running a test by hand sees the values lym's own runs get.
// A variable named by `proc.BuildServerOffEnv` is the exception.
// The line has already exported it, and the hub's value never undoes that.
// A failed read logs a named logger.Warn and exports none, never failing the launch.
// The composed line, prelude and launch command together, is written to the strand's launch script.
// The typed send-keys line sources that script in the pane shell's own scope,
// so the prelude still runs after the pane shell's profile.
// The shell dialect's discard-line key (`C-u` on POSIX, none on PowerShell) is sent ahead of that line.
// Input that reached the booting pane first therefore never prefixes it.
// Examples are a viewer's keystrokes or a terminal reply after a window switch.
// The strand records that absolute path as `Strand.LymBin` at every launch and relaunch, and reed's status reports it.
// The composition lives in panebin.go and is reached from the one chokepoint,
// so the property holds by construction for every AddStrand caller, Resume's replay included.
// The mechanism is a typed shell statement, not `split-window -e` or `set-environment`.
// It prepends onto the pane's live PATH, not a value computed from the lym process's own environment.
// It survives the pane shell's profile, which has already run by the time anything is typed.
// It needs no multiplexer capability.
// Selvage's pane and the new-session first pane are out of scope.
// The detached `lym` window OpenWindow opens composes its command in panebin.go with the same prelude.
// Every key binding that runs `lym` names the pinning binary's absolute path.
// The path comes from the same executable-path lookup, an engine field New sets to selfexec.LymBinary.
// Every pin passes that lookup through selfexec.ResolveLymBinary.
// SpawnWatchdog calls selfexec.LymBinary directly, since it takes no injectable lookup.
// No prelude, binding, button or Slack line therefore names the running test binary.
// That holds even when a test hands the engine os.Executable.
// Each `#` in it is doubled for run-shell's format expansion.
// The detached `lym loom run` and watchdog daemon spawns are outside the rule:
// both are spawned from the executable path and never resolve `lym` from PATH.
// Every shell token is emitted through internal/shell, in the shell.ForGOOS() dialect the launch command uses.
// A failed launch-script write degrades to typing the composed line.
// An unresolvable executable degrades to a pane with no prelude and no such binding.
// Each logs a named logger.Warn and never fails the launch.
// panebin_enforcement_test.go fails on a split-window or new-window call outside the chokepoint.
// Its named allowlist is the only exception.
//
// A strand's full name is formed once, in AddStrand under the state lock.
// It is built from the prefix the told geometry carries and the strand's role (the grammar is internal/agentname's).
// A role already held in the worktree is numbered `-N`, and an explicit name that is held refuses.
// Before naming, an ordinary add (not the `--if-absent` or unless-name paths) reclaims the name it asks for.
// Reclaim removes the strand holding that exact name, whether the un-numbered role name or the explicit one.
// It uses the ordinary removal and only when all of these hold:
// its engine is dead by the liveness predicate;
// it is not hidden;
// it has no children;
// and the pair is not paused.
// A live holder, a holder inside the launch grace, and a numbered form (`fix-2` for a role add of `fix`) keep the name,
// and the ordinary numbering or refusal applies.
// Reclaim kills the removed holder's pane only when this session lists it,
// so a pane placed in another session is never killed;
// the removal reaches reed.json with the add's own save,
// so a launch failure after it leaves the old record and only its dead pane gone.
// Bound: a holder misread as dead under the liveness predicate's residual cases is killed with its engine;
// creel addresses receivers by strand name, so mail queued for the dead holder goes to the new strand of that name.
// A role-less add takes the default role `strand`,
// and an empty told shortname refuses before anything boots, naming `lym fabric shortname <shortname>`.
// The name is a birth attribute and the only lookup key:
// it is stored in the strand record, and every by-name lookup resolves through it.
// A lookup matches the full name, its role segment, or a legacy exact name.
// An add can also name a strand whose presence in this worktree's state skips it (AddStrandUnless).
// The name is matched by the same rule as an explicit name;
// a live, dormant or hidden match all skip,
// and a skipped add launches, revives, removes and changes nothing.
// The retired `strand_name` config key is ignored on load.
//
// A strand may carry a done-when list (Strand.DoneWhen, set by its spawner through AddSpec.DoneWhen).
// It is the one place reed learns a strand finished.
// Reed only stats the paths:
// Resume does not relaunch a non-live strand whose list is non-empty and whose every path exists.
// It drops that strand from state and reports the count on ResumeResult.Dropped.
// A live pause entry covers a pair when Geometry.PauseRecordPath and NameSlug are both set.
// While one covers it, Resume relaunches nothing and reports the strands it held back on ResumeResult.Held;
// the prime is never held.
// Resume leaves a live strand alone and relaunches a non-live strand that has no list or a missing path.
//
// The name is mirrored outward, never read back as truth.
// launchStrandLocked exports it to the strand's process as LYM_STRAND_NAME ahead of the launch command.
// It exports LYM_PARENT the same way when a parent is told.
// It sets the pane title to the full name after `set-option -p allow-set-title off`.
// That keeps the program in the pane from overwriting it.
// On tmux below 3.5 the option is missing and the program can overwrite the title.
// The tmux floor probe's version line decides: a parsed release below 3.5 skips the `set-option`,
// and the skip is noted once per tmux path per process at Info.
// On an unrecognised version line the `set-option` runs, and a failure Warns and the launch goes on.
// A provider session name is the other mirror, passed by the launch line as `--name`.
// The daemon's file-event waits (watchloop.go) go through a FileWatchOpener.
// Its production value, OpenFileWatch, wraps internal/fswatch.
// PasteSubmit is the pane-write transport op.
// One tmux invocation fills a fresh named buffer (`lym-inject-` plus 8 hex characters) with the text.
// The same invocation pastes it as a bracketed paste, deletes the buffer, and, when asked, sends Enter behind it.
// The buffer is loaded with `load-buffer -` from the tmux client's standard input in the same invocation.
// The paste and the Enter reach the pane in one command list, so nothing the operator types lands between them,
// and the operator's automatic paste-buffer stack is never touched.
// A text of any length pastes whole:
// tmux's command size limit applies to the command list, not to a buffer read from standard input.
// A failed invocation stops before its own `delete-buffer`,
// so a separate best-effort `delete-buffer` removes the buffer.
// OperatorActive is the operator-activity query.
// It lists the session's attached clients with their active pane and last-activity epoch second,
// and it answers whether any client is on the strand's pane with activity within the quiet window, plus that window.
// The window is the reed.yaml key `operator_quiet_s` (seconds, default 5).
// A zero or negative value takes the default.
// So does a materialized reed.yaml that lacks the key, until `lym config reconcile` writes it.
// No attached client, or clients on other panes only, answer not active.
// A failed listing is returned as the error, for the caller to decide on.
// The hub watchdog daemon retires and respawns itself when the `lym` binary it started from is replaced.
// The check belongs to reedcli's watchdog verb, which calls SpawnWatchdog for the successor.
// The hub watchdog daemon runs a name-repair tick (namerepair.go), gated by the reed.yaml key `watchdog`.
// It runs no resize self-heal.
// The tick resets a drifted pane title itself.
// It repairs a drifted provider session name through the SessionNamer seam.
// The provider package implements that seam and `internal/shuttleengines` fills it.
// A strand carries an opaque provider tag its spawner sets (Strand.Provider, JSON `provider`, omitted when empty);
// reed stores it and never interprets it.
// The watchdog takes a map from tag to SessionNamer and repairs each strand's session name through its tag's namer,
// so a strand whose tag has no namer gets its pane title repaired only.
// A strand carries an opaque `CreelHooks` mark its spawner sets.
// The field is Strand.CreelHooks, JSON `creelHooks`, omitted when false.
// Reed stores it, keeps it through a resume and never interprets it.
// The session-name repair types the rename only into an idle pane with no active operator,
// and it does so with one paste-submit under the op lock it already holds.
// An active operator defers the repair to the next tick, and an unreadable client list does not hold it.
// The name-repair tick backs off: its wait starts at ten seconds,
// and it doubles through NextWakeCadence up to a minute across passes that repair nothing.
// The wait returns to ten seconds after a pass that repaired a title or a session name.
// NextWakeCadence is the one backoff rule the daemon's loops share.
// A failure that can repeat on every tick logs once per episode through logger.Episode:
// Warn at the first failure, Debug while it goes on, Info at recovery with the count of failed ticks.
// The tick guards the session-name repair's operator-activity read, pane capture and rename typing the same way.
// Each such episode is per strand where the failure names one.
// The tmux reads and writes the tick reaches through its callees share one engine-wide episode set.
// It holds one episode per read: the live pane generation read and each pane title repair.
// One-shot callers of the same helpers share those episodes,
// so a failure inside a tick's episode logs at Debug and a success ends the episode with the Info line.
// The name-repair loop holds its own episode for a pass that fails as a whole.
// The two in-memory pane-binding clears of a state load are no failure and have no episode:
// a state load never saves, so the same clear repeats on every tick.
// Each warns once per recorded and live generation pair or cleared strand set, and logs at Debug while it repeats.
// The discover signal is the hub-level file DiscoverSignalFileName, at the path Geometry.DiscoverSignalPath tells.
// A cold session boot touches it right after the new session exists, on the boot path alone,
// so every path that boots a session raises it and an attach to a live session touches nothing.
// A failed touch is logged at Warn and never fails the boot.
// The daemon's discovery loop wakes on it; standalone mode tells no path and touches nothing.
// `Down` ends the server when no session is left or the session listing errored.
// It then writes DiscoverStopMarker into the file, atomically, before kill-server, to ask the daemon to stop;
// a cold boot's touch truncates the file back to an ordinary discover, so a racing `up` wins,
// and `DownResult.EndedServer` reports the teardown.
// `lym reed list` reports the hub-wide directory of names across worktrees, with each row's live state.
//
// # Multiplexer contract surface
//
// This package assumes its configured binary honors the tmux-derived wire contract documented here.
// The binary is psmux on Windows and tmux elsewhere.
// contract_integration_test.go's TestMultiplexerContract exercises this surface against a running binary.
// It catches version drift in the installed binary.
// It runs unmodified against whichever binary LoadConfig resolves.
//
// Tmux floor: off Windows only, every tmux command and the lifecycle server spawn first check the tmux version.
// The tmux binary must be at or above lym's floor (`toolfloor.TmuxFloor`).
// The check runs `<tmux> -V` once per resolved tmux path per process and caches the parsed answer,
// so a below-floor tmux makes every command return the `*toolfloor.FloorError`.
// The error names the tool, the version found and the floor.
// A probe that could not run is returned wrapped and not cached, so the next command probes again;
// an unparseable version line is logged once with Warn and admitted.
// `CheckTmuxFloor` exposes the same check to a tmux spawn outside the runner, such as `lym reed attach`'s handover.
// A command carrying a test hook never reads or fills the process cache.
// Bound: a long-lived process, the watchdog daemon included, keeps its cached answer across a tmux upgrade.
// The capability probe's own native floor stays beneath this one, and on Windows psmux keeps that floor alone.
//
// Operator config: a server reed starts reads no `~/.tmux.conf`.
// The server spawn (serverSpawnArgv) and the capability probe's two invocations carry `-f /dev/null`.
// That holds on every platform but Windows;
// the probe needs it because `list-commands` starts a transient server on the probe's private socket,
// and a config that creates a session would keep that server alive.
// A running server ignores the flag, and every other engine call keeps its argv.
// The skip covers the operator's whole `~/.tmux.conf`: every option, key binding and hook.
// It applies on reed's per-hub servers only.
// The operator's own servers are left alone.
// It takes effect at a hub server's next start,
// so a server started without it keeps the operator's config until it exits.
// The hermetic default stands unless the `reed.yaml` key `tmux_conf` names a tmux config file.
// The key's variable is `LYM_REED_TMUX_CONF`, and an empty value sources nothing.
// A non-empty value is refused on a POSIX boot unless it is an absolute path to an existing regular file.
// The path must be free of whitespace and the characters `* ? [ \ ' " ; # { } $`.
// The error names the key, the variable and the path.
// The file is sourced once per server start, by the boot that started the server.
// A POSIX boot reads the server's identity (`#{pid} #{start_time}`) twice: before its spawn and once its session is up.
// The boot is the candidate for the source when the two reads differ.
// A candidate claims the server with the server option `@lym_tmux_conf_claimed`.
// The claim is an `if-shell` check that waits on nothing, so exactly one of two concurrent candidates sources;
// the winner sets the option to `sourced`, or to `empty` when no file is named.
// The claim and the source run before reed's own option pins, so reed's values win over the file's.
// What the file cannot pass is the session and server lifetime:
// the spawn pins the new session `destroy-unattached off` and tags it `@lym_reed_session`,
// and the guard ends by pinning `destroy-unattached` globally and `exit-unattached` on the server `off`.
// The guard invocation's exit status is only a warning, since one status carries the claim, the source and both pins.
// The warning is written to the booting command's stderr and logged with the path and tmux's own message.
// The decisions are separate read-back round trips.
// The boot fails when its session is gone, or when either pin does not read `off`.
// The error names the key, the variable, the path and the command to remove from the file:
// an untargeted `set destroy-unattached on` or a `kill-session`.
// Nothing re-sources the file into a running server, so a changed value takes effect at the server's next start.
// A starter whose boot ends between spawn and guard leaves the server unclaimed and file-less until then.
// Only sessions that count as reed's are seen by reed.
// On a server whose claim is `sourced`, a session counts when it carries `@lym_reed_session`;
// on any other server, every session counts, so Windows and servers without a claim keep every session.
// Every session read applies the rule: Go reads use `list-sessions -f` through listSessionsArgv,
// and the status bar's session row uses the same condition.
// Teardown therefore ends the server, and any session the file created with it, when no reed session remains.
// So does a boot refused because the file ended its session.
// The bound: such a session is hidden from reed and lives until the server ends.
// Windows parses the key and does none of it:
// psmux reads its own config at start, and the filter flag is never passed to its `list-sessions`.
//
// Every tmux call goes through TmuxCmd's run, runStdin or output (overlay.go).
// A tmux the host could not start for EAGAIN is retried there, four tries in all.
// The wait is 250 ms before the first retry and doubles each time, and each retry is logged at Warn.
// A fork or exec failure means tmux never ran, so a retry repeats no side effect;
// a tmux that ran and exited nonzero, and every other start failure, returns at once.
// A transient process-table shortage therefore never reads as a dead pane or a failed command.
// Each EAGAIN failure is noted in proc's spawn-refusal record.
// The step's transient classifier reads that record to judge an agent that died soon after.
//
// Pane enumeration: listPanes (overlay.go) always runs
//
//	list-panes -t <strand window> -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid} #{pane_title}\t#{pane_current_command}\t#{pane_start_command}"
//
// and parsePaneList (parse.go) parses each output line's first six space-separated fields positionally into a LivePane;
// everything after the sixth field is one tail field.
// splitPaneTail splits it on the tab paneTailSeparator into the title and the two commands:
// LivePane.Title, LivePane.CurrentCommand and LivePane.StartCommand.
// All three can hold spaces, and a tail with no separator parses as the title alone, with both commands empty.
// The session-wide window listing (windowPaneFormat) ends in the same tail, so both listings carry both commands.
// The title is a display mirror of the strand's full name, never a lookup key.
// #{pane_dead} is reported as the string "1" or "0";
// parsePaneList keys a dead pane on the literal value "1", never a numeric or boolean comparison.
//
// Strand liveness: a strand reads live (StrandStatus.Live) only when three rules hold.
// Its pane is listed in this session, is not dead, and does not show only the shell it was started with.
// engineAliveIDs (apply.go) is the one predicate.
// A pane shows only its shell when its start command and foreground command are both known and name the same shell,
// and its first process is at least launchGrace old.
// shellBaseName compares the executable names, so the login shell "-bash" matches the pin "/bin/bash".
// The age is read from /proc through paneProcessAge, only for a pane whose commands match.
// A pane reads live when its start command is empty, as for a pane spawned with no default-command pin.
// It also reads live when its foreground command is empty or its age cannot be read.
// Every pane on Windows reads live, where a PowerShell cmdlet runs inside the shell.
// Status, Directory, Resume and the --if-absent path of AddStrand take the predicate;
// Resume therefore relaunches a shell-only strand and kills its old own-window pane.
// StrandStatus.Present, OwnWindow and PanePID stay pane-level.
// The pane-level checks (the Selvage pane's health, pane options and title repair) keep aliveIDSet.
// It reads only the first two rules, since the Selvage is a shell by design.
// Two residuals remain.
// An engine that never takes the foreground reads dead: a wrapper binary runs it under a shell that does not exec it.
// A shell-only pane younger than the grace, or whose age is unreadable, reads live.
//
// Strand window: the strands live in one window of the session,
// and every op that enumerates, lays out or measures them addresses that window by its id ("@<n>").
// No op addresses it by the session's current window.
// A second window in the session is therefore never read as the strands' window.
// The one seam is windowtarget.go's TmuxCmd.strandWindowTarget.
// It lists the session's panes across all its windows (`list-panes -s -t '=<session>' -F "#{pane_id} #{window_id}"`).
// It resolves the window of the recorded Selvage pane, alive or a dead corpse.
// Failing that, it resolves the window of any present recorded strand pane left in the shell window by an older lym.
// Failing both, it uses the current-window target "=<session>:", which only the seam keeps.
// A session reap (Down, and the engine-less ReapSession) ends every window,
// so it lists the panes of all of them (`list-panes -s`) and reads no reed state.
// A recorded pane id counts as present only when that session-scoped listing carries it.
// Pane ids are server-wide, and a stale id can name another session's pane.
// With a present Selvage or strand pane, a batten window, current or not, is never read as the strands' window,
// and its pane is never reaped as untracked.
// A session's first up has neither, so it falls back to the current window,
// and the Selvage pane created there defines the strand window from then on.
// An own-window strand lives outside the strands' window: its pane is never read as that window,
// and its liveness is read through the session-wide listing (ownwindow.go's windowPane) instead.
// Reconcile ends only a window carrying @lym_own with no pane bound to a strand;
// a window without the marker is never ended.
// Pane placement: a bound pane of an own-window strand that the session listing lacks is looked up once.
// The lookup spans the whole server (`list-panes -a`, strandsElsewhereLocked) and runs only when such a pane exists.
// Found in another session, the strand reads dead and StrandStatus.Elsewhere names the placement as session:window.
// Reconcile keeps its binding instead of clearing it;
// nothing in another session is ever killed, since the kill sets read the session's own panes alone.
// The binding ends when the pane leaves the server, which a later lookup finds nowhere, or when a removal drops it.
// A failed lookup logs one episode and reads every such pane as found nowhere, so the binding is cleared;
// a pane in another tmux server is not listed and is cleared the same way.
// Bound: a pane id reused after a server restart can name a sibling worktree's pane until the binding goes.
// The pane-generation read (generation.go) is session-scoped and outside the seam: it reads session fields only,
// so it keeps the "=<session>:" form, which expands them.
// The bare "=<session>" form expands every session field to empty (verified live, tmux 3.6).
//
// Window order (windoworder.go): a session's windows sit in this index order.
// Overview windows come first, in ascending order key.
// The agents' own windows follow in start order.
// Then comes the window named shell, then the hidden windows.
// Every window is created through newWindowLocked (spawn.go) as `new-window -d` plus placement arguments.
// The placement is chosen from `list-windows` in index order under the op lock.
// An own window opens `-a -t <previous window>` for a relaunch:
// it keeps its window's place and the old one ends behind it.
// Otherwise it opens `-b -t <shell window>`, else `-a -t <highest-index non-hidden window>`.
// An overview window opens `-a -t <highest-index overview window whose order key is not higher than its own>`.
// Failing that, it opens `-b -t <lowest-index window>`, hidden windows included.
// A hidden window opens `-a -t <highest-index window>`.
// With nothing to place against, the plain form `-t '=<session>:'` is used.
// A placed new-window the multiplexer refuses on Windows (psmux) logs at Debug and is retried once in the plain form,
// so the order is best-effort there and nothing fails.
// Every window reed opens logs one Info line naming its window id and name, and its pane id where the open reports one:
// the boot's first window, an own window, a strand pane broken out of the shell window, and a detached window.
// An own window, a break-out and a detached window are each opened with a name.
// The boot's first window starts named after its shell and the shell-window step renames it shell,
// so a window named after its shell outside that boot came from no reed step.
// The session's default-command is the configured shell, so a window the operator opens by hand runs that shell.
// Reed ends only a window carrying @lym_own and reuses only one carrying @lym_hidden; a window opened by hand stays.
// The shell window is the window of the recorded console pane (Selvage), named shell with automatic-rename off.
// A fresh boot's one window ends up named that way.
// ensureShellWindowLocked brings a session booted by an older lym into that order.
// It runs wherever a window opens, once the boot marks the strands' window and at the head of every attach pre-flight.
// It runs also when no client size is known.
// It renames the shell window when its name is wrong and moves it behind the last non-hidden window.
// It touches only that window;
// a strand pane an older lym left in the shell window is broken out by the reconcile head's move rule.
// It does nothing, logged at Debug, when no console pane is recorded or the window listing does not carry its window.
// A check never fails its caller.
// A failed rename or move warns once per failure episode; the session option @lym_shell_check_failed keeps it open.
// It logs at Debug while the episode stays open and always on Windows.
// An Info line closes the episode when a later check succeeds.
//
// Detached windows: OpenWindow (window.go) opens a detached, named window running a `lym` command.
// The window lives in the worktree's reed session.
// It serves a caller that wants a long-running command to live in the session without a status-bar button.
// One multiplexer invocation chains the commands that open the window.
// It starts with `new-window -d -a -t <highest-index window> -P -F '#{window_id}' -n <temporary> <command>`.
// When no window is listed, it starts with the plain form instead:
// `new-window -d -t '=<session>:' -P -F '#{window_id}' -n <temporary> <command>`.
// It then runs `set-option -w -t '=<session>:=<temporary>' remain-on-exit off`.
// It next runs `set-option -w -t '=<session>:=<temporary>' @lym_hidden 1`.
// It ends with `rename-window -t '=<session>:=<temporary>' <name>`.
// The window is detached and never selected, so it takes no focus.
// OpenWindowIn is OpenWindow with a working directory:
// a non-empty directory adds `-c <dir>` before the command, and OpenWindow passes an empty one.
// The options are chained by exact name.
// An option chained after a detached new-window without a target lands on the current window.
// The current window can be any window, because attach and the focus rules select agent and overview windows.
// Its remain-on-exit must stay on.
// The name is a per-call temporary one that no other window holds, and the rename comes last.
// An own window may share the requested name, and the options would otherwise land on it.
// The status bar skips a window carrying @lym_hidden.
// Window placement and the shell check read the flag to find the last non-hidden window; no focus rule reads it.
// The op reads remain-on-exit back from the new window by the printed id,
// and a window that does not read off is killed by that id with ErrWindowReadBack.
// The read-back also asks for the answering window's id.
// Tmux answers for a window that has already closed, as when its command exited at once.
// The answer is exit 0, an empty id and the global value;
// a failed read or a reply from any other window counts as the window being gone, never as a mismatch.
// A hidden window of the same name whose pane is live is returned as existing and nothing is started;
// one whose pane is dead is killed and replaced.
// That lookup reads only windows carrying @lym_hidden.
// OpenWindow therefore never adopts, kills or replaces an own window or a window the operator opened by hand.
// The command line is composed in panebin.go from the pane-binary prelude, so `lym` resolves to the spawning binary.
// Bound: the op only chooses where a command runs;
// the window lives as long as the reed session, and the session's other windows keep remain-on-exit on.
//
// Own windows: an own-window strand gets a detached window named by its role.
// spawn.go's openOwnWindowLocked, the launch chokepoint, opens it by `new-window -d`.
// The window is then selected only under the focus rule below.
// The window carries @lym_own, @strand_color and @lym_unseen, each set by window id;
// the last is left unset on Windows.
// An overview window carries neither @strand_color nor @lym_unseen.
// Nor does a new agent window the focus rule selects.
// Its pane runs the launch tail every strand pane runs.
// Its dead pane stays until the strand is removed or relaunched;
// a relaunch kills the old pane once the new one exists,
// so a shared window never keeps a dead pane beside the relaunched one.
// Resume reopens the strand detached.
// UpdateStrand refuses a move into or out of the anchor.
// The pane would sit where the new display does not expect it.
// A strand declaring a window group (Display.Window, sized by Display.WindowSize) shares one window with its group.
// That holds while the declared size is at most window_share_max;
// every other own-window strand gets a window to itself.
// The group's first strand decides.
// ReedState.WindowGroups records the choice and the size until the group's last strand goes,
// so a later window_share_max edit never splits a group across shared and separate windows.
// A strand declaring another size than the record's is refused with both sizes named.
// A strand arriving once the group's recorded size is already held gets a window of its own.
// An example is a re-stepped seat added before its predecessor is removed.
// The shared window stays as it is.
// A pane joins the shared window by a detached split in the window_split direction,
// so an operator viewing the window keeps the active pane, and the window is then laid out evenly in that direction.
// Every such layout also pins the window's window-resized hook to issue the same layout after each resize.
// tmux's own resize of a layout can leave one pane at its old size while the other takes the whole change,
// and the hook gives every pane of the window an equal share again.
// The hook skips a zoomed window, so a resize never unzooms it.
// psmux lacks set-hook, so on Windows no hook is pinned.
// Removing an own-window strand moves a viewer on its window to the relevant window before the window ends.
// The move never goes to the shell, and it never moves a viewer on another window.
// A shared window a strand leaves is laid out again, and its hook is pinned again.
// An invalid window_split or window_share_max never refuses a removal:
// the layout is skipped and the key is reported in Removed.ConfigWarning.
//
// Focus rules: an agent window is a window holding a pane bound to an own-window strand;
// the overview, the shell and hidden windows are not agent windows.
// The follow state is the session option @lym_follow (followSessionOption): boot sets it on off Windows,
// and it is off only while it reads off.
// An unset option reads on, as on Windows or in a session booted by an older lym.
// followingLocked reads it, and a failed read counts as off for that one decision and is logged at Warn,
// so a broken read never moves a viewer.
// Only @lym_follow, @lym_button_spec, @lym_button_dir and @lym_button_answer are decision-bearing session options.
// An automatic window switch happens only at three points.
// Each decides from reed state, window ids, strand roles, pane liveness and the follow state alone.
// None reads @lym_unseen, @lym_hidden or a wait mark.
// relevantWindow is the target of two of them.
// Among the listed windows not about to end, it picks the highest-index agent window holding a pane that is not dead.
// Failing that, it picks the lowest-index overview window, and failing that none.
// Attach (attachWindowSelectionLocked, called by AttachArgv right after the shell check) selects the relevant window.
// It appends `; select-window -t <relevant window>` to every argv it returns,
// and it appends nothing when a listing fails or no window is relevant.
// With another client attached, or when the client listing fails, it appends the selection only while follow reads on.
// It reads the follow state only then and never writes it, and a failed follow read counts as off.
// So with follow on, an attach moves every client of the session to the relevant window.
// That includes a client on a window it reached outside reed's bindings.
// A new agent window is selected by openOwnWindowLocked when newAgentWindowSelected holds.
// Only an own-window strand's window is selected this way, never an overview.
// newAgentWindowSelected holds always for a Focus strand and never for one whose display sets only KeepViewer.
// For any other strand it holds exactly when the follow state reads on.
// A selected window carries no @lym_unseen.
// With follow on, a new agent window takes the viewer wherever it is, the shell included;
// with follow off, it leaves the viewer where it is.
// Opening a window never changes the follow state.
// FollowRun is the one engine operation that turns follow on, beside the engine-less FollowSession.
// Under the operation lock it sets the option on and selects the relevant window for whichever client views the session.
// That window is the newest live agent window, else the lowest-index overview window, else nothing.
// It persists no state.
// A failed option write is logged and the select still runs.
// AddStrand, ReplaceStrand, Resume and Up never call it.
// Every client of a session shares its current window, so the one select-window moves each attached client.
// A viewed window ending moves a viewer on it to the relevant window among those left, never the shell.
// That covers RemoveStrand, ReplaceStrand and reconcile ending an orphan own window (selectRelevantWindowIfCurrentLocked).
// With none left, the choice is left to tmux.
// The move neither reads nor changes the follow state.
// A viewer elsewhere is never moved,
// and a strand leaving a shared window that stays open ends no window and switches nothing.
// An automatic switch races an operator's own window change by at most one switch.
//
// Overview anchor: the overview anchor places any number of strands, each alone in a window of its own.
// The windows sit at the session's lowest indexes in ascending order key.
// The key is render.Display.Order, and an absent key reads as zero.
// An overview strand never joins a group.
// A new, relaunched or replacement overview window opens after the last overview window whose key is at most its own.
// Failing that, it opens before the lowest-index window.
// So equal keys sit in the order they last opened.
// The window takes the display's Title as its name, overview when the display carries none;
// the orch's runs strand names it after its beam.
// Everything that reads a window of its own treats an overview pane as an own-window one (isOwnWindow):
// reconcile, presentPanes, the strands' window resolver and the relaunch's old-pane kill.
// Only the placement, the name, the unseen mark and the focus rules ask isOverview.
// The window carries @lym_own like an own window and sets neither @lym_unseen nor @strand_color,
// so its button takes the plain window-button style.
// An overview display takes no window group.
// UpdateStrand refuses a move between the overview and own-window anchors.
// UpdateStrand also refuses a change of an overview strand's order.
// ReplaceStrand removes the old strand before its add,
// so the replacement takes its place among the overview windows by its own key.
// A replacement overview strand can end the window the operator is viewing.
// Once the launch succeeds, it selects its own new window.
// A replacement whose old strand shared a window that stays open ends no window and switches nothing.
//
// Session targeting: every -t argument that names a session is passed in an exact-match form.
// Session targets (has-session, kill-session, attach-session) use "=<name>".
// Window and pane targets use "=<name>:", because the window/pane target parser rejects the bare "=<name>" form.
// Those targets are list-panes, select-layout, display-message and the set-option pins' window-targeted -t.
// The exact form is load-bearing: tmux falls back to prefix matching a bare -t name when no exact match exists.
// On the shared per-hub server, a bare name from one worktree can silently address a sibling worktree's session.
// The sibling is one whose session name the bare name prefixes.
// With tmux 3.6 and only "repo2" present, `has-session -t repo` exits 0 and `kill-session -t repo` kills repo2.
// contract_integration_test.go's TestExactSessionTargetsNeverPrefixMatchSiblings pins both grammars.
// Pane-id (-t %N) targets are already exact and stay bare.
//
// Subcommand set: the engine's correctness depends on tmux's own documented semantics for each of these verbs.
// Session and layout verbs: new-session, has-session, split-window, new-window, select-layout, select-pane.
// Pane and server teardown verbs: kill-pane, kill-session, kill-server.
// Input and output verbs: send-keys, capture-pane, list-panes, list-sessions, display-message.
// Option verbs: set-option -g remain-on-exit, set-option -g mouse.
// The engine may also pass tmux's -v/-vv verbose-logging flags on the server-spawning invocation.
// They are opt-in via debug_log, and the configured binary must accept them.
// The capability probe (probe.go) also issues -V and list-commands, which differ in a way that matters:
// -V is answered client-side and contacts no server.
// list-commands is answered by a server and starts one if none is running.
// Both go through TmuxCmd with an explicit -L socket.
// The probe therefore never starts a server on the operator's global default socket.
// Off Windows the socket is a fresh private key per probe, never the hub's.
// The transient server `list-commands` starts exits as soon as it has answered.
// On the hub's socket, a sibling worktree's concurrent boot could connect to it mid-exit.
// That boot would fail with `server exited unexpectedly`.
// The probe removes the private key's socket file afterwards.
// Every verb named above is required:
// requiredSubcommands (probe.go) fails the capability probe on a binary missing any of them.
// set-hook is absent from requiredSubcommands.
// Gating the capability probe on it would take every reed verb down on a psmux lacking it.
// The hooks it serves are the unseen-clear hook, the window-resized unpin and a group window's even-layout hook.
// All three are quality-only and degrade silently.
//
// Load-bearing behavioral assumptions, each with the rationale that makes it load-bearing:
//
//   - Silent split failure (spawn.go): split-window against a pane too small to split exits 0 and creates no new pane.
//     It prints an existing pane's id on stdout rather than erroring.
//     psmux behaves this way, and native tmux errors loud with "no space for new pane".
//     Every split site must therefore check that the returned pane id was absent from the pre-split live set.
//     A pane id already in that set is not a new pane.
//     Both launchStrandLocked and ensureSelvagePaneLocked run the shared validateSplitCreatedNewPane guard.
//   - Dead panes under remain-on-exit (spawn.go): "set-option -g remain-on-exit on" is set at boot.
//     A pane whose command exits then stays enumerable (pane_dead=1) instead of vanishing, while the session survives.
//     That is the only signal that lets reconcile tell "the strand's process died" from "the pane is simply gone".
//     Such a corpse is never reused as a strand's pane, since a strand's pane is always a fresh split:
//     send-keys into a dead pane exits 0 while running nothing.
//     The strand's command would be silently swallowed, a failure a fresh split cannot reproduce.
//     This behavior holds for a non-last pane on any backend, and for psmux even on the true last pane.
//     It does not hold for tmux's true last pane (see the last-pane bullet below).
//   - The untracked-pane reap gate (spawn.go, reconcile.go):
//     every pane in a reed session is either Selvage or a bound strand's pane.
//     The untracked reap enforces that rule as `anyBoundPresent || policy.authorizesReap()`.
//     The Selvage anchor requires aliveness rather than mere presence.
//     launchStrandLocked fires the gate from AddStrand and UpdateStrand.
//     Neither heals a Selvage corpse first through ensureSelvagePaneLocked.
//     A dead Selvage id must therefore never itself authorize sparing the untracked set.
//     The reap runs before pane allocation at one chokepoint inside launchStrandLocked.
//     The property therefore holds by construction on every realization path, with no two call sites to keep in sync.
//     Two consequences follow.
//     An `up` against a session with zero tracked strands ends up Selvage-only and full-height.
//     RemoveStrand's reconcileApplyPersistLocked tail inherits the gate.
//     Removing the last strand therefore reaps any untracked alive pane in the same verb.
//   - Last-pane fate depends on the binary (strand.go's kill-pane loop, RemoveStrand):
//     killing a session's true last pane behaves oppositely depending on the configured multiplexer.
//     On tmux (the PATH-resolved POSIX default per template_posix.go) it destroys the session outright,
//     and the server exits if that was its only session.
//     This produces "exit status 1: no server running".
//     On psmux (the Windows default) it corpses the pane (pane_dead=1, exit 0) and the session survives.
//     TestSmokeRemoveLastStrandThenAddRunsTheNewCommand in internal/reedcli/smoke_lifecycle_windows_test.go verifies this.
//     The test removes the sole strand, which returns 0.
//     A subsequent add then finds the session already up through ensureSessionLocked's liveness probe and boots nothing,
//     which only holds if the session survived the remove, and it yields a live second strand.
//     has-session and list-panes exit 1 for "no server running", which hasSession (overlay.go) maps to (false, nil).
//     list-sessions and kill-server (see the async kill-server bullet) exit 0 regardless of server state,
//     so they cannot distinguish "no server" from "server dying asynchronously".
//     RemoveStrand's post-kill re-probe therefore uses hasSession, never list-sessions.
//     That exit 1 lets the re-probe classify the emptied session on tmux.
//     removalEmptiedSession (strand.go) then treats the resulting applyErr as an expected success.
//     It does so only when the session is confirmed gone.
//   - Told-geometry lifetime and the vanished worktree root (server.go, lock.go):
//     a told Geometry is resolved once per process and pinned for that process's whole life.
//     A long-lived process such as the daemon therefore holds a frozen WorktreeRoot.
//     A `mv` of the worktree makes it stale.
//     Every operation re-checks the told worktree root's liveness at the op-lock chokepoint,
//     through validateToldWorktreeRootLive in withOpLock and withTryOpLock.
//     It refuses rather than creating substrate under a path that is no longer a worktree.
//     In standalone mode, a `--target-dir` naming a directory that does not exist is refused at the first engine op.
//   - Silent session-name rewriting (server.go's validateToldTmuxIdentity):
//     tmux does not reject a session name containing '.' or ':'.
//     It rewrites each to '_', creates the session under the rewritten name, and exits 0 (verified live, tmux 3.6).
//     The same silence covers two further classes.
//     A backslash is doubled: vis(3) doubles '\' unless VIS_NOSLASH is passed, and tmux does not pass it.
//     So "bs\slash" becomes "bs\\slash".
//     Every ASCII control character, DEL and invalid-UTF-8 byte is vis-encoded into a multi-character escape, with exit 0.
//     TAB becomes the two literal characters `\t` and 0xFF becomes `\377` (verified live, tmux 3.6).
//     Valid multi-byte UTF-8 passes through verbatim.
//     Those three classes are the whole rewrite surface.
//     An exhaustive sweep on tmux 3.6 sent every printable ASCII byte through new-session.
//     Each was then looked up with an exact-match has-session.
//     It found exactly '.', ':' and '\'.
//     Every session target this package issues is the exact-match "=<name>" form above.
//     The rewritten session is therefore unreachable by the name that created it:
//     the boot loop polls a target that can never match.
//     The session is left squatting on the shared per-hub server, where no reed verb can address or tear it down.
//     The told identity is therefore validated at withOpLock, the one chokepoint every public op passes.
//     A name tmux would rewrite is refused rather than sanitized:
//     substituting would map sibling worktrees "svc.v2" and "svc_v2" onto one session.
//     Each would then adopt the other's panes.
//     contract_integration_test.go's TestSessionNameRewriteIsSilentAndExactTargetsMissIt pins the wire behavior.
//     The socket key is the mirror-image case.
//     Its readable half carries no identity.
//     ServerName substitutes separators out at the derivation instead of refusing.
//   - The -l leading-dash send-keys bug (spawn.go):
//     send-keys -l parses a '-'-prefixed literal argument as flags and silently drops it.
//     A "--" separator does not stop this parsing.
//     sendKeysLiteralArg therefore prefixes a single space onto any opaque cmd/resumeCmd beginning with '-'.
//     The space is added before the argument reaches send-keys.
//   - Pane ids are server-global and not stable across a server rebirth (generation.go):
//     tmux hands out %0, %1, … per server, not per session.
//     The counter restarts at %0 when the server dies.
//     A persisted pane id therefore identifies a pane only within one server generation.
//     A reed.json that outlives its generation names panes that exist and belong to something else.
//     list-panes cannot tell the two apart, so reconcile cannot either: "present" is the only question it can ask.
//     reed.json therefore records the generation its bindings were minted in.
//     The generation is session_id, server pid and session_created.
//     Together they are unique, and each is immutable over a session's life.
//     The engine discards every binding whose generation is not the live one.
//     This generalizes Up/Resume's `booted` clear to the tables that arrive between boots.
//     One disagreement is refused rather than cleared:
//     a recorded session still running on this socket under a name that is not this worktree's.
//     That happens with a renamed worktree or a copied .lym.
//     Carrying on there would launch a second copy of every strand and leave the first unreachable.
//   - display-message does not exit 1 for an absent session (generation.go).
//     has-session and list-panes exit 1 for a `-t` target that names no session.
//     display-message exits 0 and expands every session-scoped format to empty.
//     The server-global #{pid} still fills.
//     With tmux 3.6, `-t '=nosuch:' '#{session_id}|#{pid}|#{session_created}'` answers "|2912080|" with exit 0.
//     It does not fall back to a current session when several exist.
//     The generation probe's absent case therefore surfaces as parsePaneGeneration's empty-field rejection.
//     It does not surface as a tmux error.
//     An error from that probe means the round trip could not be run, never that the session is gone.
//     The still-running-orphan refusal answers existence with list-sessions for this reason,
//     and it refuses a listed session it cannot identify rather than failing open on it.
//   - Duplicate-pane-cell session destruction (reconcile.go's clearConflictingPaneBindings):
//     select-layout does not reject a layout string that names one pane twice.
//     It accepts it (exit 0) and assigns cells positionally.
//     It then destroys every pane the resulting short cell list no longer covers.
//     With tmux 3.6, one `up` reduced a two-pane session to one and reported ok:true.
//     Nothing reed constructs can produce such a table, since every split result passes validateSplitCreatedNewPane.
//     The source is therefore always a corrupt persisted table:
//     a restored backup, or a hand-edited or partially-restored reed.json.
//     The engine clears such bindings at the single load chokepoint.
//   - Async kill-server and probes that always exit 0 (lifecycle.go):
//     kill-server returns before the server process (and its "__warm__" helper) has released the -L socket.
//     No probe can distinguish "no server" from "server dying asynchronously", since both exit 0 either way.
//     That covers list-sessions and kill-server itself.
//     Down/reap logic therefore waits on the underlying OS process exiting.
//     It does not trust any CLI exit code as a death signal.
//   - Socket-file removal (socketfile.go, lifecycle.go, overlay.go):
//     a killed tmux server can leave its `-L` socket file behind.
//     ensureServerGoneLocked removes it once no tmux process remains on the socket.
//     It does so on both the graceful and the force-reap path.
//     Down and every engine-bound teardown reach it through that one method.
//     EndSessionByName has no engine.
//     After it kills the last session's server, it retries the removal for a bounded wait.
//     The wait covers the time the server takes to stop answering.
//     The path is resolved as tmux does (`$TMUX_TMPDIR`, else `/tmp`, then `tmux-<uid>/<key>`).
//     It is removed only when it is a socket and nothing answers a connection on it.
//     A second Lstat must also report the same file.
//     The removal is bounded to the file for the key just killed, only inside that teardown.
//     It never globs or sweeps the directory.
//     A server restarting on the same key between the check and the removal is the accepted residual race.
//     A removal failure is logged at Debug and never fails the teardown.
//     It is a no-op on Windows, where psmux keeps no socket file.
//     The boot path also measures the socket path, with the base's symlinks resolved, before any tmux round trip.
//     It refuses a path over the OS limit (107 bytes on linux, 103 elsewhere), naming `TMUX_TMPDIR` as the way forward.
//     The check runs on the boot path only.
//     A verb run later under a longer `TMUX_TMPDIR` than the server booted with still gets tmux's own error.
//   - Mouse boot pin (lifecycle.go):
//     on a fresh boot the engine pins "-g mouse" to the configured mouse value (default "on"), alongside remain-on-exit.
//     Like remain-on-exit and debug_log, it is applied only on the boot that spawns the session.
//     Toggling mouse in config or LYM_REED_MOUSE on an already-running hub has no effect until the reed server restarts.
//     An already-materialized reed.yaml keeps whatever value it holds.
//     Reconcile is key-based and never rewrites a value.
//     "on" is the default because "off" is not neutral.
//     With mouse off, tmux never claims the wheel.
//     The terminal's own alternate-screen wheel translation then sends arrow keys to the pane's foreground process.
//     In a reed session that process is a live agent, so scrolling scrolls nothing and types into the agent instead.
//     An operator is at the pane by design under an interactive producer.
//     A scroll gesture there edits the answer they were trying to re-read.
//     The cost of "on" is that native terminal text selection needs the terminal's shift-bypass.
//     Hold Shift while dragging.
//     tmux copy-mode is the in-band alternative.
//     "on" also enables click-to-switch-pane.
//   - The dot-fill artifact's mechanism (windowsize.go, attach.go): the dots are painted by tmux itself.
//     They fill a region of an attached client's terminal in one of two cases:
//     the window geometry does not cover it, or its paint is stale after a window size change.
//     They are never content reed writes and are in no pane's grid.
//     capture-pane against the reed session is therefore structurally blind to them.
//     Both triggers are one mechanism:
//     a transient or standing mismatch between a client's terminal size and the window's size.
//     A resize changes the client's size, and window-size latest moves the window to follow it.
//     Delivering any input to a second client (a keystroke, a mouse report, a focus report) makes it most-recently-used.
//     window-size latest keys on that, so the window resizes to it.
//     Every other client is left mismatched with no resize from the operator's point of view.
//     Mouse-tracking escape sequences do not leak through the shared session:
//     mouse bytes are consumed by tmux as client input and never reach another client's screen.
//     The mouse is only how the second client announced itself as most-recently-used.
//   - The stale-paint and uncovered subsets, and which trigger each is (windowsize.go):
//     the stale-paint subset is a client fully covered by the new window geometry that shows leftover paint.
//     A forced redraw removes it; this is the resize trigger.
//     The uncovered subset is a client whose terminal is genuinely larger than the window.
//     tmux has real estate with nothing behind it and legitimately pads it.
//     This is the cross-client trigger.
//     A VS Code integrated terminal is smaller than a standalone Konsole window.
//     No repaint mechanism can remove the uncovered subset, and reed ships no repaint entry.
//     Removing it would require changing window-size, which is refused for stronger reasons (see the next bullet).
//   - Why window-size latest is not changed, made configurable, or conditioned on the client count (windowsize.go):
//     with two clients of different sizes attached, tmux must pick one window size.
//     Some client is therefore mismatched under every available policy.
//     latest gives the artifact to the client the operator is not currently using, which is the best of the three.
//     Rejected: largest shows tmux padding to every operator with a second client open;
//     smallest turns an intermittent artifact into a standing one;
//     manual abandons client-following.
//   - The orch pane under the same policy:
//     a smaller client attached to the orch session can shrink the window below the rows the input box needs.
//     The orch watcher's idle probe then reads no box.
//     It records the pane as too short (internal/orchengine's State.Stuck), holding every cycle.
//     The policy is kept for the orch pane too, since no policy avoids a mismatch with two clients of different sizes.
//     The window returns to the larger client's size at that client's next keystroke.
//     The hold then clears at the next passing probe.
//   - The attach-time multi-client warning (attach.go's warnMismatchedClientsLocked):
//     AttachArgv's pre-flight lists the session's attached clients.
//     It emits one logger.Warn per client whose size differs from the size this attach was told.
//     Each warning names that client and both sizes; zero differing clients produce zero lines.
//     It never blocks and never reaches the JSON envelope.
//     It reports whether another client is attached, a failed client listing counting as one.
//     Another attached client, or a failed listing, drops the attach window selection from the argv.
//     This is the primary deliverable for the cross-client trigger.
//     Its dots are the uncovered subset, which cannot be repainted away.
//     The residual's operator cost is bewilderment.
//     The warning turns a mysterious artifact into a logged, searchable fact when the operator creates the condition.
//     The target-form rule: list-clients -t takes a session target.
//     warnMismatchedClientsLocked therefore uses the bare "=<name>" form.
//   - list-clients is not in requiredSubcommands (probe.go): it serves geometry quality only.
//     The Shared Decision geometry-tmux-failures-are-non-fatal-everywhere covers it, as it does the window-size pin.
//     A failing or unsupported list-clients logs one warning.
//     warnMismatchedClientsLocked then emits no multi-client warning.
//     Requiring it would make a multiplexer that runs reed well fail at boot over a cosmetic feature.
//   - The attach argv (attach.go):
//     AttachArgv returns the bare "attach-session" argv.
//     When the window selection applies, the argv gains a "; select-window -t <relevant window>" tail.
//     The separator is a literal one-character ";" argv element, never "\;".
//     exec.Command passes argv directly to the child, so no shell ever sees it to unescape.
//     attach-session is first in the chain, so a failing select-window still leaves the operator attached.
//   - The geometry option pins (windowsize.go):
//     pinGeometryOptionsLocked pins the three-line status bar, whose format strings bar.go declares.
//     All three status formats are set with -g, since tmux's status formats are one array.
//     "status-format[0]" holds the window and shell buttons.
//     "status-format[1]" holds the session buttons and the time.
//     "status-format[2]" holds the follow button and the action row.
//     "status-style" (-g) is the dark grey statusBarStyle, so the gaps between the buttons are not tmux's default green.
//     "status" "3" and "status-position" "bottom" are set on the session.
//     "window-size" "latest" (-w) is set on the strand window.
//     The window-targeted pin uses -t <strand window>, per the Strand window and Session targeting grammar above.
//     It runs at boot and again in AttachArgv's pre-flight.
//     The bar reads strands only through the @strand and @strand_color pane options and the @lym_strands window option.
//     It never reads them through the current window.
//     Line 2 is two single-format `#{S:}` loops, since tmux 3.4's loop takes one format.
//     The first draws the session carrying the session user option @lym_prime_session.
//     The second draws every other reed session, in the loop's own order.
//     That order is session name on tmux 3.4 and session id on newer tmux.
//     The prime session is the hub geometry with an empty name slug.
//     The mark is `set-option -t <strand window> @lym_prime_session 1`.
//     It is set after the status pins, at boot and in the attach pre-flight.
//     The marking is non-fatal like the pins.
//     A pair's session and a standalone session are never marked, and no Go decision reads the mark.
//     A prime session booted without the mark draws in the second loop.
//     That lasts until a `lym reed attach` to it reaches the geometry pins.
//     An attach whose pre-flight degrades to the bare argv leaves it unmarked.
//     The same function clears a stale `window-resized` hook on the shell window.
//     It runs `set-hook -u` in the window and session scopes everywhere but Windows, so boot and attach both clear it.
//     Every pin here is non-fatal, unlike the remain-on-exit and mouse pins beside them.
//     Those two are correctness dependencies.
//     These are geometry-quality options whose absence degrades to a working session.
//     psmux's support for them is unverified anywhere in this repo.
//     Shared Decision geometry-tmux-failures-are-non-fatal-everywhere covers them.
//   - The watchdog daemon's own stderr is watched by no one (reedcli/watchdog.go):
//     the daemon points the logger's durable sink at fabricengine.HubLogsDir(hub) first, before it takes its lock.
//     That ordering keeps its diagnostics reachable, since nothing reads a detached process's stdio.
//
// requiredSubcommands (probe.go) needs nothing more for the option pins or the window-resized unpin:
// display-message, set-option and list-panes are already required.
package reedengine
