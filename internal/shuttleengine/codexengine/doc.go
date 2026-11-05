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
// The launch line runs `-a never -s workspace-write`, so no approval prompt shows, and adds the records checkout it is told through its Dirs as a writable root, symlink-resolved as the trusted roots are and kept as told, with a Warn, when it cannot be resolved,
// so the session writes under the anchor's `.lym` as a Claude seat does; outside a hub it is told none and adds none.
// A `.codex/rules` folder in the worktree refuses the launch, since an allow rule there lifts the sandbox; a `.codex/config.toml` there is named in a Warn, since its hooks run unreviewed.
// Both are read on disk, tracked by git or not, since Codex reads them either way.
// The first turn is the positional prompt pointer, and the pane's shell exits after Codex, so a Codex exit reads as a dead pane;
// the resume line starts no Codex, since Codex mints its session id at the first turn.
//
// Hooks given by `-c` append a stamp line and the payload of SessionStart, UserPromptSubmit, Stop and PermissionRequest to the run's events file, and PostToolUse touches the activity stamp.
// A Stop is the turn end and carries the reply; an interrupt fires no Stop, so a newest turn_aborted in the rollout the payloads name marks the turn interrupted.
// The rollout's newest token_count gives the context size.
// Codex keeps no session registry, so liveness reads unproven, and the session state follows the hook signals and the interrupt marker alone.
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
// OpenLaunch opens a named interactive session with no prompt, for `lym shuttle open`: it validates the model choice and readies the lym home as Prepare does,
// and its line runs Codex with no hooks and no first turn, so the session carries no shell guard and waits at its composer.
// Its resume line is empty, so a revive starts no Codex and Runner.Open's chain ends the strand.
//
// The engine has no fork audit, no transcript reading and no creel hooks.
package codexengine
