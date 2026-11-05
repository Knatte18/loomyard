// Package codexengine implements shuttleengine.Engine for the Codex CLI, driving its interactive TUI in a reed pane.
// Only internal/shuttleengines constructs it; it registers there as codex, with its config module codex.yaml.
//
// Every launch runs under one shared lym Codex home, codex.yaml's home, set as CODEX_HOME on the launch line; the operator's own Codex home is never written.
// The lym home links auth.json and sessions to the operator home's, so one login and its token refresh serve both and rollouts land where the operator's tools read them.
// lym keeps the home's trusted roots in a record of its own and renders config.toml whole from it on every launch, under a file lock: Codex keys folder trust on a git root, and a `-c` flag cannot set it.
// Prepare trusts the worktree root and a linked worktree's main checkout root, read from the `.git` entries, and drops recorded roots whose folder is gone, so no teardown step is needed.
// A login made in the lym home leaves a regular auth.json, which Prepare names in a Warn rather than replaces.
// After startup the engine reads config.toml back and warns about a root Codex's own write dropped; the trust prompt's accept is the backstop.
//
// The launch line runs `-a never -s workspace-write`, so no approval prompt shows, and adds writable roots, each symlink-resolved as the trusted roots are.
// The records checkout it is told through its Dirs, kept as told with a Warn when it cannot be resolved, lets the session write under the anchor's `.lym` as a Claude seat does; outside a hub it is told none and adds none.
// The checkout's git dir and the common git dir it links to, read from the `.git` entries, let the session commit: a linked worktree's git dirs lie under the main checkout's `.git`, outside the workspace.
// codex.yaml's writable_roots that exist, the Go build cache in the template, let the session's tools fill a cache the sandbox would leave read-only; a local override may not set the key.
// The line also sets `sandbox_workspace_write.network_access`, the one opening the sandbox gets beyond writes:
// Codex's Linux sandbox refuses every socket connect without it, a Unix socket's included, so a session could not run `lym creel read` or `post --reply` against the creel daemon.
// Codex has no setting that opens Unix sockets alone; its network proxy forwards a Unix socket only for HTTP requests, which creel's protocol is not.
// So the session's shell commands have the network a Claude seat's have, and writes stay bound to the workspace, the writable roots and the temporary folders.
// The Bolt checkout's `.lym` stays read-only, so a client in the session that finds no daemon cannot start one; the daemon that delivers the session's mail is already up.
// A `.codex/rules` folder in the worktree refuses the launch, since an allow rule there lifts the sandbox; a `.codex/config.toml` there is named in a Warn, since its hooks run unreviewed.
// Both are read on disk, tracked by git or not, since Codex reads them either way.
// The first turn is the positional prompt pointer, and after Codex exits the line appends an exit record to the run's events file and exits the pane's shell, so a Codex exit reads as a dead pane;
// the resume line starts no Codex, since Codex mints its session id at the first turn.
//
// Hooks given by `-c` append a stamp line and the payload of SessionStart, UserPromptSubmit, Stop and PermissionRequest to the run's events file, and PostToolUse touches the activity stamp.
// The prompt file opens with the creel trust sentence, after the deny notice, since Codex reads a hook's block reason as a failed tool call unless its instructions say `[creel]` lines are mail.
// A Stop is the turn end and carries the reply; an interrupt fires no Stop, so a newest turn_aborted in the rollout the payloads name marks the turn interrupted.
// The rollout's token_count events give the context size, from the newest event's latest call, and the session usage (UsageReader), from the newest event's running total.
// The session usage splits input net of its cached part, cache-read, cache-write and output, with a total that names no cache-write reading it as zero; the reasoning tokens are already in the output.
// Codex implements no MessagePricer, so its dollar cost stays unknown.
// Codex keeps no session registry, so liveness reads unproven, and the session state follows the hook signals, the exit record and the interrupt marker alone.
// Codex fires no hook on its own exit, so the exit record stands in: it reads as a process-ending session end with reason `codex-exit`, and a run whose Codex exited reads dead as a Claude run does.
//
// The pane readings follow the spike's table: the `›` composer, the `Working (… esc to interrupt)` busy line and the folder-trust prompt, whose accept presses nothing unless the option sits by the caret.
// A send pastes the text and leaves the Enter to shuttle's guarded send, since Codex drops an Enter typed inside a burst of characters.
//
// Every installed row of the shell guard table rides the launch line as a PreToolUse hook matching Bash, which prints Codex's deny decision with the row's Codex steer,
// and the prompt file opens with the rows' Codex notices; a hook that fails to run fails open in Codex, so a guard is a guardrail and not a barrier.
//
// A skill `<plugin>:<name>` is a file the model reads: the load line names the newest version's skills/<name>/SKILL.md under codex.yaml's skill_roots by absolute path,
// and a tool call of the load turn naming that tail in the rollout reads the skill loaded; a skill under no root is reported unknown.
//
// A run whose engine is told a hub board directory carries creel's hook delivery and is marked CreelHooks, so creel hands its mail over at the session's boundaries and wakes it when idle.
// A second PostToolUse entry, after the activity hook, forms the strand's flag name from `$LYM_STRAND_NAME` and, only when that flag stands in the hub's creel flag directory, runs `lym creel hook-read codex`;
// with no flag it prints the empty hook output and starts no process.
// The Stop recording becomes one merged command: when the flag stands it pipes the payload to `lym creel hook-read codex` and exits 0 when that handed out mail, recording no turn end, since the block continues the turn;
// otherwise it appends the Stop record as before.
// HookOutput hands mail over as `{"decision":"block","reason":<mail>}`, which Codex takes at the next step after a tool call and at once after a turn end, and shows the operator as `Blocked by hook`.
// It drops the echo of mail an idle wake handed over, since the operator already saw that wake as a user line and Codex would show the echo beside the reason.
// A block after a tool call makes Codex read that call as failed, so the session may run it once more.
// The Stop setting also carries the idle waiter as a second, async entry with a timeout of 86400 seconds: `lym creel hook-wait codex`, with CODEX_HOME and LYM_CODEX_BINARY set on its command.
// No exit of a Codex hook wakes an idle session, so when creel rings the waiter it calls WakeIdle, which runs `codex queue --thread <session_id> --message <mail>` with the session id of the Stop payload, before creel marks the mail read;
// the queued line starts a turn the operator sees as a user message.
// A failed queue leaves the mail unread, and creel's bell, typed into the pane, wakes the session instead.
// An engine told no board directory, a standalone run, keeps the plain Stop recording and adds no creel hook.
//
// OpenLaunch opens a named interactive session with no prompt, for `lym shuttle open`: it validates the model choice and readies the lym home as Prepare does,
// and its line runs Codex with no hooks and no first turn, so the session carries no shell guard and waits at its composer.
// The line keeps the network access, so an opened session reaches creel too, and appends no exit record, since an opened session has no events file.
// An opened session carries no creel hooks either, since it has no prompt file to hold the creel trust sentence, so it is a pane-mail strand.
// Its resume line is empty, so a revive starts no Codex and Runner.Open's chain ends the strand.
//
// The engine has no fork audit and no transcript reading.
package codexengine
