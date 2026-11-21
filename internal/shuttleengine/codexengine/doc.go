// Package codexengine implements shuttleengine.Engine for the Codex CLI, driving its interactive TUI in a reed pane.
// Only internal/shuttleengines constructs it; it registers there as codex, with its config module codex.yaml.
//
// Every launch runs under one shared lym Codex home, codex.yaml's home, set as CODEX_HOME on the launch line;
// the operator's own Codex home is never written.
// The lym home links auth.json and sessions to the operator home's,
// so one login and its token refresh serve both and rollouts land where the operator's tools read them.
// Codex keys folder trust on a git root and a `-c` flag cannot set it;
// so Prepare renders the lym home's config.toml from lym's own record of trusted roots on every launch.
//
// The launch line runs `-a never`, so no approval prompt shows, and `-s` with the mode settled from sandbox_mode.
// danger-full-access runs the session with no sandbox, as a Claude seat runs.
// Under auto, the template's value, and under workspace-write, each launch first probes Codex's sandbox.
// A machine's bubblewrap may not be able to create a user namespace.
// Then every shell command of a sandboxed session fails and the session writes no output.
// A sandbox that starts runs workspace-write.
// One that cannot start runs danger-full-access with a warning under auto, and refuses the launch under workspace-write.
// A probe that fails to reach a verdict refuses the launch under both modes.
// The key is a machine key: the hub's machine override sets it for one machine, and a local override may not.
//
// The workspace-write line adds writable roots: the records checkout and the checkout's git dirs.
// It adds codex.yaml's writable_roots that exist, which a local override may not set.
// It also sets `sandbox_workspace_write.network_access`.
// Codex's Linux sandbox refuses every socket connect without it, a Unix socket's included.
// Codex has no setting that opens Unix sockets alone.
// A session could not otherwise reach the creel daemon.
// Writes stay bound to the workspace, the writable roots and the temporary folders.
//
// A `.codex/rules` folder in the worktree refuses the launch, since an allow rule there lifts the sandbox;
// a `.codex/config.toml` there is named in a Warn, since its hooks run unreviewed.
// A spec naming an advisor refuses the launch, since Codex has no advisor tool.
// The resume line starts no Codex, since Codex mints its session id at the first turn.
//
// Hooks given by `-c` write the run's events file, which feeds the session signals.
// Codex fires no hook on its own exit, so an exit record appended by the launch line stands in for it,
// and a run whose Codex exited reads dead as a Claude run does.
// A Stop is the turn end and carries the reply; an interrupt fires no Stop, so the rollout marks the turn interrupted.
// The standing items ride the launch line as the `developer_instructions` setting,
// and a launch whose setting reaches Linux's argument cap is refused before anything is written.
// Codex has no session registry, so liveness reads unproven.
// Codex implements no MessagePricer, so its dollar cost stays unknown.
//
// Every installed row of the shell guard table rides the launch line as a PreToolUse hook matching Bash.
// A hook that fails to run fails open in Codex, so a guard is a guardrail and not a barrier.
// An autonomous launch also denies Codex's question tools, the counterpart of Claude's AskUserQuestion deny.
//
// The batching nudge (BatchingNudger) is a PostToolUse hook.
// It adds one line to the calling thread's context when the thread's recent calls break lym's batching rules.
// It never denies a call, because a block after a tool call would make Codex read the call as failed.
// Its keys and defaults are claude.yaml's, and the read-only lookup check is shared with the Claude engine.
// Each thread's count and cap are keyed by its own rollout, since a child's payload carries its parent's session id.
//
// The edit hook (EditRecorder) records the files an `apply_patch` call changes.
// That lets shuttle's edit-loop watch cover Codex runs as it covers Claude runs.
//
// A skill `<plugin>:<name>` is a file the model reads:
// the load line names its SKILL.md under codex.yaml's skill_roots, and a skill under no root is reported unknown.
//
// A run whose engine is told a hub board directory carries creel's hook delivery and is marked CreelHooks:
// creel hands its mail over at the session's boundaries and wakes it when idle.
// HookOutput hands mail over as a block decision, which Codex shows to the operator as `Blocked by hook`.
// No exit of a Codex hook wakes an idle session, so WakeIdle queues the mail as a turn through `codex queue`.
// A failed queue leaves the mail unread, and creel's bell, typed into the pane, wakes the session instead.
// An engine told no board directory, a standalone run, adds no creel hook.
//
// NewFromConfig takes Seams: the user-setup source, the plugin hook runner and the sandbox probe.
// A nil source adds no user setup, a nil probe runs Codex's own, and a nil runner lists each plugin hook as not run.
// The registry wires the Claude engine's source and hook runner,
// which is the one route from the user's Claude setup to this engine.
// A source error refuses the launch.
//
// Codex reads one instruction file in each directory from the checkout root down to the engine's anchor.
// The developer instructions also name the CLAUDE.md files of the worktree that Codex does not read itself.
//
// Prepare also writes the launch manifest into the run directory through shuttleengine.WriteManifest.
// The manifest is read back from the launch line, so a delivery dropped anywhere in Prepare drops its item.
//
// OpenLaunch opens a named interactive session with no prompt, for `lym shuttle open`.
// Its line runs Codex with no hooks and no first turn, so the session carries no shell guard and no creel hooks.
// It is a pane-mail strand, and its resume line is empty.
//
// The engine has no fork audit and no transcript reading.
package codexengine
