// Package claudeengine is the Claude adapter behind shuttleengine.Engine.
// All Claude-specific knowledge lives here and nowhere else:
// CLI flags, the settings.json hook schema, TUI startup and trust markers, and pane key choreography.
// shuttleengine and the run loop it drives know only the Engine interface,
// so a second provider engine would be added alongside this package without touching either.
//
// One piece of that knowledge matters at package level, because getting it wrong is silent and total.
// Claude's trust-this-folder gate is a selection list, not a confirmation prompt;
// the caret does not start on the accepting option.
// Claude 2.1.263 puts the caret on "No, exit", so confirming the selection quits claude.
// Every agent lym spawns in a directory claude has not yet accepted therefore dies at startup.
// That includes every freshly created fabric worktree pair.
// TrustDismissSequence reads the caret out of the capture it is given rather than assuming a position,
// and it presses nothing when it cannot find the accepting option.
//
// The mirror of that knowledge matters too, because getting it wrong is equally silent and total.
// Once a run is under way a capture is the agent's own transcript,
// and every phrase that identifies a gate is ordinary English an agent writes:
// ("the files in this folder", "trust this folder", "yes, I accept").
// A gate is therefore never classified from a phrase alone.
// Startup requires positive evidence that a dialog is rendered:
// an accepting-option line, recognized by what it begins with once the caret and list numbering are stripped;
// or claude's own "Enter to confirm" gate footer.
// It locates that option line with the same helper TrustDismissSequence uses,
// so the classifier never names a gate the dismissal would then have to walk blind.
// The evidence must also sit where a rendered gate puts it:
// the option line adjacent to the last caret, the footer just below it.
// A prose list item that begins with an accept phrase ("- Yes, I accept the risk") is itself a matching option line.
// The only caret on a healthy pane is its input-box marker at the bottom, far from any transcript prose above.
//
// A Stop line whose turn ended with background work still running becomes EventWaiting,
// and its Event.Outstanding lists that work as provider-neutral shuttleengine.BackgroundTask values:
// a fork for an Agent or Task subagent, a shell for a backgrounded Bash or a Monitor.
// The Stop payload's background_tasks list is authoritative when present, even empty:
// its running entries are the whole list and the transcript is not read.
// The transcript fallback runs only when the key is absent or not a list.
// It lists the background launches that no later completion notification names:
// a task-notification user message, a queue-operation line or a queued_command attachment.
// A payload can hide a running task: it omits the task, lists it under another status or changes an entry's shape.
// That counts as no outstanding work.
// Each task carries the signal that reported it.
// An Agent or Task launch counts as background in either of two cases:
// its input sets `run_in_background`;
// or its input names the `fork` subagent type.
// Claude Code runs every fork in the background.
//
// The outstanding-fork reading (OutstandingForkReader) reads the session's own transcript, narrowed to forks.
// Claude Code writes a queue-operation line the moment a notification arrives during a tool call,
// so the reading turns empty while a session that waits on its forks is still inside the waiting call,
// and the notifications reach the session together when the call returns.
// A missing or unreadable transcript is an error.
//
// Every Bash command an agent runs gets `/dev/null` as its default stdin through Claude Code's `CLAUDE_ENV_FILE`.
// Prepare writes `bash-env.sh` beside settings.json,
// and both the launch and the resume line lead with `CLAUDE_ENV_FILE` naming its absolute path,
// so an interpreter left reading the tool's open stdin ends at once instead of hanging the session,
// and a resumed session keeps the default.
// The file runs two statements and nothing else:
// `exec </dev/null`, then one that prepends the parent directory of `LYM_BIN` to `PATH`;
// that statement is double-quoted so a path with spaces holds.
// The PATH statement reads `agentname.LymBinEnv` when Claude Code runs the file before each Bash command;
// that key is the one reed's pane prelude exports with the spawning binary's path.
// A resumed session therefore follows the binary its pane's latest prelude exported.
// The Bash tool shadows exactly the executables the pane shell shadows, even after the operator's rc reorders `PATH`.
// On Windows the statement first converts `LYM_BIN` with `cygpath -u` into the scratch variable `__lym_bin_dir`.
// It unsets that variable afterwards.
// It adds no directory when `LYM_BIN` is unset or empty or, on Windows, when `cygpath` is absent.
// It changes only the default stdin and `PATH` of the tool's own shell:
// a heredoc, pipe or redirect attached to a command still supplies that command's stdin;
// a child `bash` the agent starts does not re-run the file;
// and it grants or denies nothing, so the agents' permission rules are untouched.
// A `CLAUDE_ENV_FILE` the operator's own environment exports is overridden for lym-spawned sessions only.
//
// ColorSequence is the one place lym's palette maps to Claude Code's `/color` names:
// every palette color maps to the same-named Claude Code color.
// The sequence is `/color <name>` typed and submitted, with no leading Escape.
// A color outside the palette answers no inputs.
//
// The engine owns a layered config module, `claude.yaml`, declared in config.go:
// `binary`, `deny_ask_user_question`, `prompt_cache_ttl` and `prompt_cache_ttl_roles`;
// `keep_warm_cap` and `keep_warm_cap_roles`;
// and the `batching_nudge` and `session_scope` blocks.
// The retired `deny_agent_tool` key is read out of every layer;
// a local override setting it is refused, with deleting it as the way forward.
// LoadConfig resolves the module through the degrading loader.
// The loader stacks the template, the repo default, the worktree file and the local override;
// a local override may set every key but the binary and the tool denies.
// The engine is told its directories and derives none.
// NewFromConfig takes them.
// Prepare, OpenLaunch, PromptCacheLifetime and KeepWarmCaps each load claude.yaml through them before anything else,
// so an invalid file refuses a run start, a resume and an open.
// The refusal names the file, the key and the way forward, `lym config claude`.
// No other method and neither constructor loads it;
// an engine built by New, with zero directories, refuses those methods.
//
// The launch and resume lines both carry `CLAUDE_CODE_PROMPT_CACHE_TTL`,
// so a session writes its prompt cache at the TTL its role needs rather than at Claude Code's subscription default.
// Prepare resolves the value from `Spec.Role` through `claude.yaml`'s `prompt_cache_ttl_roles` map;
// `prompt_cache_ttl` is the fallback for an empty or unmapped role.
// Prepare validates the whole configured set to `5m` or `1h` (exact, case-sensitive) before any artifact is written.
// It logs the resolved value with the role.
// The value is fixed for the session's life, since a resume line is never rebuilt.
// OpenLaunch resolves the TTL from `OpenSpec.Role` the same way and refuses an invalid set the same way.
// Its two lines open with a statement exporting the variable, since its resume line falls back to its fresh line.
// PromptCacheLifetime answers the same resolved value as a duration,
// so a caller can keep an idle session's cache warm inside it.
// `keep_warm_cap` and the per-role `keep_warm_cap_roles` set the longest single wait an agent of a role blocks on.
// Each is a Go duration of at least 60s, below the role's prompt-cache lifetime;
// it is also at most 55m, the Bash ceiling less five minutes.
// ValidateKeepWarm checks them where a run starts, in Prepare after the TTLs;
// KeepWarmCaps answers a role's cap together with the 55m long cap.
// Forks run at Claude Code's 5-minute subagent lifetime;
// a cap above five minutes therefore leaves that role's forks' caches cold.
// The subagent bucket (`CLAUDE_CODE_SUBAGENT_PROMPT_CACHE_TTL`) is left at Claude Code's default,
// and a `FORCE_PROMPT_CACHING_5M` in Claude Code's own environment still wins.
//
// The launch and resume lines also carry `BASH_MAX_TIMEOUT_MS`, the ceiling on the Bash tool's `timeout` parameter.
// It is set to one hour from `bashMaxTimeout`.
// `BASH_DEFAULT_TIMEOUT_MS` is left at Claude Code's default;
// so only a call with an explicit `timeout` runs past it.
// No call stays in the foreground past the ceiling:
// at the ceiling the Bash tool ends the call or moves it to the background.
//
// The context reading comes from the transcript a Stop payload names, read backwards from its end in doubling chunks.
// The latest main-chain assistant usage entry or compaction boundary is the reading, whichever sits later in the file;
// a boundary reading carries its `postTokens` and timestamp and is marked compacted.
// Every failure degrades to an unknown reading.
//
// The session-usage reading (UsageReader) sums a finished session's tokens.
// It reads the parent transcript and the fork transcripts under its `subagents/` directory.
// Fresh tokens are input plus cache-creation plus output, and cache-read tokens count separately.
// The reading also carries the input, cache-creation, cache-read and output counts apart.
// Every count is summed over the session and its forks, and the reading marks them read.
// A message counts once however many transcript lines carry it.
// The totals include the forks, which are also broken out.
// A fork transcript's entries are all sidechain, and they all count except the parent's spawning message.
// The fork replays that message as its first entry.
// An unreadable transcript makes the whole reading unknown, and a missing `subagents/` directory is zero forks.
// The reading shares one parse of the transcript lines with the transcript reader below.
//
// The transcript reader (TranscriptReader) locates and reads finished sessions with no run and no shuttle config.
// A worktree's project directory is `<home>/.claude/projects/<dir>`;
// `<dir>` is the worktree path with every non-alphanumeric byte as a hyphen.
// Each `*.jsonl` file directly in it is a session, and the `*.jsonl` files in `<session>/subagents/` are its forks.
// Sessions and forks come back sorted by file name.
// A missing projects root, project directory or `subagents/` directory yields none of them and no error.
// Reading a session parses its own transcript with sidechain lines included.
// It then parses each fork with the message ids the session counted as a skip set.
// A message streamed over several lines carries its usage on its last line only,
// and each earlier line counts as a repeat.
// A fork replays its parent's spawning message,
// so each fork line of a skipped id carries no usage and counts as a repeat;
// its entry is kept for the turn times.
// A message with no id is keyed by its line's uuid, else by its transcript and line number,
// so no two such messages merge.
// A turn ends on one of two lines:
// an assistant line whose stop reason is not `tool_use`;
// or a user line starting `[Request interrupted by user` or `<local-command-stdout>`.
// A turn starts on a user line holding text and no tool result that neither ends a turn nor is a compaction summary.
// The launch line is `launchPointer`'s rendering in a user text, and its prompt path rides on that entry.
// A malformed or half-written line is skipped;
// a vanished transcript fails the read, naming its path;
// a line without a parseable timestamp has a zero time.
//
// The message pricer (MessagePricer) prices one message's neutral usage from its model.
// The price table is Claude's API list price, with its source cited at the table.
//
// The session-signal parse (SessionSignalParser) is in signals.go.
// It reads the same events file as ParseEvents but yields a separate stream.
// Each hook line maps to one signal:
// a `Stop` is a turn end with its outstanding tasks read exactly as ParseEvents reads them;
// a `StopFailure` is an API-error turn end, marked ModelRejected when its `error` is `model_not_found`;
// a `UserPromptSubmit` is a turn start whose Instruction is the trimmed `prompt`.
// The Instruction is empty for a slash command and for a prompt opening `<task-notification>`.
// Claude Code submits that prompt itself when background work ends;
// a `PreToolUse` for `AskUserQuestion` is an ask, and so is a `PermissionRequest`, by its hook name alone;
// a `Notification` is read by the fixed table `notificationSignalKinds`.
// In that table `permission_prompt`, `elicitation_dialog` and `elicitation_url_dialog` are asks;
// `idle_prompt` is an idle notice.
// `agent_needs_input`, `elicitation_response` and any other type are skipped;
// a `SessionEnd` is a session end with its reason;
// a `SessionStart` is a session start carrying the payload's `source` mapped to a provider-neutral one:
// `startup`, `resume`, `clear` and `compact` to their constants, and any other or missing string to the empty source.
// The asks that are not events, the ones the dialog notice reads, are the `PermissionRequest` hook.
// They are also the `permission_prompt`, `elicitation_dialog` and `elicitation_url_dialog` Notifications.
// A fork's notifications are included.
// The question tool's ask is an event.
// The session fold reads a `resume` or `clear` start as an idle-fresh session,
// and a `startup` start too unless a turn is running;
// a `compact` start moves nothing.
// A `SessionEnd` reason of `logout`, `prompt_input_exit` or `other` ends the process;
// `clear`, `resume` and an unknown reason do not.
// A recording hook writes a stamp line `{"lym_stamp":"<hook>","lym_at":"<RFC 3339 UTC>"}` before its payload,
// and the parser gives a payload the time of the nearest preceding untaken stamp naming its hook.
// A payload with no such stamp, or whose stamp's time is empty or malformed, reads a zero time.
// The count the parser reports as consumed stops before the trailing run of stamp lines with no payload after them,
// so an incremental reader sees each stamp with its payload at its next read.
// A transcript-fallback turn end's tasks are read from the transcript at parse time,
// so the same line parsed again later can read fewer.
//
// The events file holds a stamp line and then the hook's payload line for each recording hook.
// The recording hooks are these:
// Stop, UserPromptSubmit, StopFailure and Notification;
// SessionEnd, SessionStart and PermissionRequest.
// An interactive run also records AskUserQuestion.
// The hook command is plain POSIX `sh`, `date`, `cat` and `printf`:
// one `printf` writes the stamp;
// `;` joins it to the unchanged payload append, so a failed stamp never blocks the payload;
// and every hook but Stop ends in `; true`, so it exits 0 whatever the append does.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
// Every `SessionStart` source is recorded once.
// A run without a context-after-compaction command records them in one entry with no matcher;
// a run with one records `compact` in its own entry and the other sources in an entry matching `startup|resume|clear`.
// No recording hook prints to standard output,
// so the `PermissionRequest` recording answers no request and leaves Claude's own dialog to decide it.
// The one exception is the `compact` `SessionStart` entry's second command, which Spec.ContextAfterCompaction sets.
// It runs verbatim after the recording command, and its output joins the session's context after every compaction.
// SessionStartContext renders the additional-context JSON such a command prints.
//
// Every run's PostToolUse, PreCompact and PostCompact hooks carry one entry with no matcher.
// Its command is `touch <stamp>; true`, and the stamp is shuttleengine.ActivityStampFileName beside the events file:
// a tool call, a fork's included, and a compaction move the run's activity stamp without growing the events file.
// No SubagentStart or SubagentStop hook is installed:
// the PostToolUse stamp already moves for every tool call a fork makes,
// and an unmatched start would read a crashed fork active without bound.
// The settings leave Claude Code's `autoContinueAtUsageLimit` unset,
// so a session the usage limit stopped waits for the orch's wake and revive rather than racing it.
// They set Claude Code's `attribution` to `{"commit": "", "pr": ""}`,
// so a commit or pull request the session makes carries no credit of its own,
// and a landing's credit block stays the one credit.
// An engine told a hub board directory also installs a second PostToolUse entry with no matcher, creel's mail hook;
// an engine told none, a standalone run, installs the stamp alone.
// Its command is plain POSIX `sh`.
// It forms the strand's flag name from `$LYM_STRAND_NAME` by parameter expansion alone;
// every `:` becomes `.`, as agentname.FileName does.
// It exits 0 at once unless that flag stands in creel's flag directory of the board directory,
// so a tool call with no mail starts no process.
// When the flag stands it changes to `$CLAUDE_PROJECT_DIR` and runs `lym creel hook-read claude`.
// HookOutput prints this JSON to the hook's standard output:
// `{"systemMessage":<echo and mail>,"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":<mail>}}`.
// It hands the strand's mail to the session after the tool call as additional context;
// it shows the mail to the operator as the system message.
// On a Stop event, HookOutput prints:
// `{"systemMessage":<echo>,"hookSpecificOutput":{"hookEventName":"Stop","additionalContext":<mail>}}`.
// The system message is absent with no echo.
// With an echo and no mail, either event prints only `{"systemMessage":<echo>}`.
// No output carries `decision` or `reason`.
// A blocking hook would reach the model twice and the operator once.
// The model would read `Stop hook feedback:` and `Stop hook blocking error from command: "<command>": <reason>`.
// The operator would read `Stop hook error:`, naming the hook's command.
// With the mail in the additional context alone the model reads it once, as `Stop hook additional context:`;
// the turn continues through it.
// HookInput parses a hook's payload:
// `PostToolUse` and `Stop` map to the tool-boundary and stop events;
// a non-empty `agent_id` marks a subagent;
// and `stop_hook_active` is carried.
// RewakeOutput returns the text plus a newline and exit code 2, the `asyncRewake` wake.
// Claude Code runs an event's matching hooks in parallel,
// so a separate creel Stop hook could not keep the recording from logging a turn end its mail then continues,
// and every Stop that handed out mail would read as a turn end in ParseEvents.
// An engine told a board directory therefore replaces the plain Stop recording with one merged command,
// and it adds the idle waiter as a second Stop entry;
// a standalone run keeps the plain recording.
// The merged command reads the hook input once.
// Only when the strand's flag stands does it pipe the input to `lym creel hook-read claude` from `$CLAUDE_PROJECT_DIR`.
// The output goes straight to the hook's standard output,
// so the mail is acknowledged only after the hook output reached Claude Code.
// When hook-read exits 0, mail was handed out, and the command exits 0 having appended nothing to the events file;
// otherwise, or with the flag absent, it appends the Stop stamp and the payload as the recording command does,
// and it exits with that append's status.
// An exit other than 0 records the turn end and leaves whatever hook-read printed on the hook's standard output.
// The echo of mail an idle ring already delivered relies on that:
// hook-read prints it as a system message and exits 1, so the operator sees it and the turn ends.
// Bound: a merged command killed at its timeout after the write never reaches its record step,
// so the turn ends with no Stop record when Claude Code drops the killed hook's output.
// The waiter entry runs `lym creel hook-wait claude` from `$CLAUDE_PROJECT_DIR` whether or not a flag stands.
// It sets `asyncRewake` true and a timeout of 86400 seconds.
// Claude Code therefore runs it in the background and wakes the idle session when it exits 2.
// It also sets `rewakeMessage` to `[creel] Mail that arrived while you were idle:` and `rewakeSummary` to `creel mail`,
// so the model reads the wake under creel's label rather than Claude Code's default naming the hook's command,
// and the terminal draws the wake as one `creel mail` row and never its body, which the operator gets from the echo.
// A running session keeps the labels its launch wrote until it is relaunched.
// Prepare marks the launch CreelHooks exactly when it passes the board directory's flag directory,
// so a strand is marked only when its settings carry the PostToolUse, Stop and waiter hooks.
// On Windows the flag directory takes the same git-bash POSIX conversion as the events path.
//
// The batching nudge (BatchingNudger) is a PostToolUse hook that adds one line to the calling chain's context.
// It does so when the chain's recent calls break lym's batching rules, and it never denies a call.
// `claude.yaml`'s `batching_nudge` block holds `enabled`, the breach switches and the thresholds.
// The breach switches are `lookup_chain`, `read_slice` and `edit_chain`.
// The thresholds are `context_tokens`, `lookup_run`, `edit_run`, `slice_lines` and `max_nudges`.
// ValidateBatchingNudge refuses a negative `context_tokens`, or any other threshold below 1, while the hook is on.
// It runs in Prepare and on a config write.
// With `enabled` on, Prepare copies the block into `batching-nudge.json` beside the events file,
// and the settings have a PostToolUse entry matching `Read|Grep|Glob|Bash|Edit|Write`.
// The entry runs `lym shuttle hook-batching claude <state file>; true`,
// so no other tool call starts a process,
// and a running session keeps the values its Prepare wrote.
// On Windows the state file takes the git-bash POSIX path of the events file's directory;
// git-bash converts it back for the native lym.
// The judgment reads the call's own input and the tail of the calling chain's transcript, `batchingTailBytes` long.
// Claude Code writes a message's lines to the transcript only after its calls start,
// so a fast call such as a Read or an Edit runs the hook before its own message is in the transcript.
// The judgment therefore rereads the tail until a message holds the call.
// It rereads every `batchingTranscriptTick` for at most `batchingTranscriptAttempts` ticks.
// A call still missing then is left silent, as is the early call of a parallel message still being written.
// It waits only for a chain with a state file and under its nudge cap.
// It also waits only while the tail's first reading is at or above `context_tokens`.
// While the call's message is not in the transcript yet, that reading is the previous message's,
// so the one call whose message first reaches `context_tokens` is left silent.
// It groups the tail's assistant lines by message id, so the parallel calls of one message count as one message.
// A main-session call reads the session's transcript and its main-chain lines.
// A call carrying an agent id, from a subagent or fork, reads the sidechain lines of that agent's own transcript:
// `subagents/agent-<id>.jsonl` beside the session's.
// Such a call counts its nudges apart from the session's, under the same block.
// The judgment is silent in these cases:
// a Bash call whose command is no read-only lookup;
// a call whose context, by the tail's latest usage reading, is below `context_tokens`;
// a chain already nudged `max_nudges` times;
// and a message it already nudged.
// The judgment holds the state file's lock, so the parallel calls of one message get at most one nudge.
// A single-lookup chain is a run of at least `lookup_run` consecutive messages that each hold exactly one lookup call.
// The current message counts as far as the transcript shows it.
// A lookup call is a Read, Grep or Glob call, or a Bash call whose command is a read-only lookup.
// A read-only lookup is a pipe or chain of these commands:
// `grep`, `rg`, `find`, `ls`, `cat`, `head`, `tail`, `sed -n`, `wc` and git reads.
// It has no write, substitution or redirect into a file, and no option that writes a file or runs a program.
// A Bash call with any other command breaks the chain.
// An edit-only chain is a run of at least `edit_run` consecutive messages that hold only Edit or Write calls.
// A read slice is a Read with a `limit`, or an `offset` past the first line, of one of these files:
// a file under `slice_lines` lines;
// or a file that the latest earlier message reading it in the tail read no earlier than the file's last modification.
// The line names one breach and quotes the rule it breaks.
// A single-lookup chain takes precedence over a read slice, and a read slice over an edit-only chain.
// Bound: the hook is a heuristic, since some lookups must run in sequence, so it fires at times on a right call;
// a later call of the current message may not be in the transcript yet when an early call's hook runs,
// so a batched message after a chain can read as one more single lookup;
// an earlier read before the tail window is not seen, and the session's own edit counts as a change.
//
// Prepare declares the launch as recording session starts,
// so the startup waits for the `startup` line beside the ready pane.
// These accepted races follow from the append order:
// hooks that fire at once can interleave their stamps ahead of both payloads, which the pairing by hook name absorbs;
// and a reader can see a payload without its newline, or a stamp without its payload;
// the parser leaves either unconsumed until the line completes.
// The stamp's time is taken when the hook starts,
// so it can precede the payload's append by the length of the hook's own run.
//
// The activity reading (ActivityReader) takes the session's last activity, its forks included.
// It also walks the main transcript backwards for the newest main-chain assistant entry:
// one that ends a turn or carries Claude Code's `isApiErrorMessage` marker.
// The last activity is the later of the main transcript's last write and the newest fork reading.
// The fork readings come from the `subagents/` directory beside the main transcript.
// A fork's reading is its last write time.
// It is the reading clock's now instead when both hold:
// the fork has a tool call with no result;
// and it was last written no more than `bashMaxTimeout` ago.
// The pending-call reading looks only at the fork's newest tool-calling message, read backward from the end:
// a message is every assistant entry sharing one message id,
// and it is pending when any of its calls has no result after it;
// an unanswered call in an older message does not count.
// A held foreground call writes nothing,
// so the pending-call reading keeps a Master waiting on a working fork from reading quiet;
// a fork stopped mid-call reads active for at most that ceiling past its last write, which a live call never outlasts.
// The API-error part reads the main transcript only.
// A session whose newest such entry carries the marker stands on an API error,
// and the entry's final text is the error's text;
// a sidechain entry never counts, and a later normal turn end clears the error.
// The same entry gives the activity reading's usage limit when it carries the usage-limit classification below.
// The usage limit carries the reset time its text states:
// the first instant after the entry's timestamp that matches a `resets 2pm (Europe/Oslo)` clause.
// The clause takes optional minutes and an optional month and day, and the instant is in the clause's IANA zone.
// The classification alone decides that the limit stopped the turn;
// a text with no readable clause, or one naming a day its month lacks, gives a limit with no reset time.
// Every failure degrades to the zero reading.
//
// The usage-limit reading (UsageLimitReader) reads Claude Code's own classification:
// the `error` field `rate_limit` on a synthetic `isApiErrorMessage` entry.
// It never reads the entry's text or the agent's words.
// A turn end is usage-limited when the main transcript's newest main-chain turn end is such an entry.
// It is also usage-limited when a fork transcript under `subagents/` has such an entry as its newest assistant entry,
// and it was last written after the main chain's previous turn end.
// The fork half catches an agent whose forks hit the limit and that then ends its turn itself;
// the time bound keeps a limit an earlier turn already met from counting again.
// A session with no previous turn end counts every fork, and every failure reads as no limit.
//
// The resume check refuses a session whose registry entry names a live pid.
// It does not refuse when the live process's start time differs from the entry's `procStart`;
// that difference proves the pid was reused.
// An unreadable start time, or an entry without `procStart`, still refuses and says the pid could not be proven reused.
//
// The session prober (SessionProber) reads process liveness from the same registry.
// A session is alive when an entry names it and the live pid's start time equals `procStart`.
// It is dead only for an entry whose pid is gone.
// It is unproven in these cases:
// no entry names it;
// the registry or an entry is unreadable;
// the start time is unreadable;
// or the pid was reused.
// A session Claude Code exited normally has no entry left, so it reads unproven.
// The resume check and the prober share one reader of registry entries.
// The prober also reads the interrupt marker.
// The marker is the user's `[Request interrupted by user` entry;
// it is read from the transcript a turn start's `transcript_path` names.
// It counts only as the newest main-chain entry, and its own timestamp is the interrupt's time.
// Any later main-chain entry, a missing path or an unreadable transcript reads as no interrupt.
// The prober also reads a turn's progress from the same transcript (TurnProgressReader).
// The newest main-chain user or assistant entry decides:
// a user entry other than the interrupt marker reads in flight, and the marker reads unknown.
// An assistant entry is read by its message's stop reason, not by its last content block:
// `tool_use` reads in flight, and `end_turn` and `stop_sequence` read over;
// an absent, null or other reason reads unknown.
// A last line without its newline is still being written and is not read.
// A missing path, an unreadable transcript and a transcript with no such entry read unknown.
//
// The engine takes text as one bracketed paste (TextPaster).
// Shuttle pastes a send through reed's paste-submit.
// With `PasteSubmits` true, the Enter follows the paste end in the same tmux invocation.
// `PasteSubmits` is true on the assumption that an Enter right after a bracketed paste's end submits it in Claude Code;
// a live check pins it.
// If the check fails, the one constant flips and shuttle sends the first Enter itself, guarded.
// No sequence opens with an Escape.
// Prompt suggestion is off in every shuttle-launched session, so an Escape would guard nothing.
// It could complete a double-Esc or interrupt a turn.
// ComposeSend is the text step and the Enter, and TypeSequence is the text step alone.
// The session commands and the color command are their text and an Enter with no pause between them.
// ClearInputSequence is one `C-u`, which deletes from the caret to the start of the line.
// It empties a one-line draft without interrupting a running turn.
// It serves sent text and a lone stray-character candidate alike, since both are one line with the caret at its end.
// It also empties whitespace the box may hold before a session command is pasted:
// a box of whitespace reads as empty, and Claude Code takes a command behind it as a prompt.
// RestoredPromptClearSequence clears the prompt an interrupt restored into the box.
// It is sized from the box capture: n `C-u` with one `BSpace` between each pair, for n box lines.
// One `C-u` deletes only the caret's row, and a `BSpace` at a row's start joins it to the row above.
// OwnPastePlaceholder recognises the form Claude Code collapses a pasted draft into:
// `[Pasted text #<n>]` or `[Pasted text #<n> +<m> lines]`.
// A box reading cannot otherwise match that form against the pasted text.
// It answers true only for a text of at least `PasteCollapseMinChars` characters.
// When the placeholder shows `m`, the text's newline count must equal it.
// The box reading takes a box holding only a lone `%`, the shell's end-of-line mark, as empty.
// PlainBoxCapture takes the placeholder reading too, and turns a styled capture into a plain one.
// It blanks a box whose every cell but a lone unstyled `%` is drawn faint.
// Claude draws its placeholder and prompt suggestion that way.
// The collapsed-paste placeholder is the exception and is kept.
// Unstyled text, a box mixing unstyled and faint text, and a release that restyles the placeholder read as a draft,
// so the reading fails closed.
// StrayInput returns the character of a stray-character candidate.
// A candidate is a box holding exactly one character, with no running turn on screen.
// That character is neither a letter, a digit nor whitespace.
// The lone `%` is never a candidate.
// IdleSession, StrayInput and RunningTurn share one running-turn reading:
// the "esc to interrupt" hint matched over the whole capture.
// RewindPicker reads the rewind picker: the picker's subtitle is on screen and no input box is.
// A transcript quoting the subtitle above a live box is therefore not the picker.
// DismissiblePicker reads the mid-session pickers in its table, such as the autocompact picker.
// A picker reads as shown when every phrase of it is on screen ("We recommend compacting earlier", "esc not now"),
// and no input box is.
// It answers one Escape, which closes the picker and keeps the current setting.
// InterruptSettle is 2 seconds, on the assumption that Claude Code's double-Esc window is shorter.
// Two keys lym sends therefore never read as a double Esc.
//
// The engine realizes the clear sequence (`/clear`) and the compact sequence (`/compact`).
// Skill loading is in skillload.go.
// A spec that names skills carries the skill-load message on the launch line, so the session's first turn is the load;
// the prompt pointer comes back as Launch.PromptLine for shuttle to send after the skills.
//
// The engine realizes a whole skill list as one typed message with no leading slash, SkillLoadMessage.
// The message asks the model to load each skill through the Skill tool in one turn, in list order.
// ClassifySkillLoad checks that turn from the transcript a Stop payload names, read backwards from its end:
// the latest main-chain user entry equal to the message starts the turn,
// and a skill is loaded when its Skill call is answered by a successful result;
// it is unknown when that result is an error or the latest skill listing does not name it;
// it is missing otherwise.
// Each of these degrades to an unverified report:
// a missing transcript_path, an unreadable file, a transcript with no matching message, or one with no skill listing.
//
// The engine announces each standing tool deny to the session and tells it to trust and answer creel mail.
// It does so through the run directory's append-system-prompt.txt.
// Both the launch line and the resume line name it with --append-system-prompt-file.
// The file holds the deny notice and the creel trust sentence, space-separated.
// The notice is left out when no deny is installed.
// The standing passages heddle's layers hold for the claude engine follow it, each after a blank line and in order.
// They are read through the process-wide passage reader that shuttleengine installs a seam for.
// The one-line rule applies to lym's own sentences alone, since passages may span lines,
// and the file reaches Claude Code through --append-system-prompt-file and is echoed nowhere.
// A resume through the launch's resume line reuses the file its launch wrote, so its passages stay;
// a Prepare that runs again, as an adopted session's does, reads the passages afresh.
// The text goes in a file because its sentence holds a double quote.
// An argument passed through pwsh's Legacy mode (an npm claude.cmd shim) would drop that quote.
// A session opened with `lym shuttle open` in a hub carries its own such file.
// It lies in the session directory Runner.Open names.
// That file holds the creel trust sentence alone; a session opened outside a hub carries none.
// Both the hooks and the notice are built from one deny list in settings.go, so the two cannot drift:
// each row names its tool matcher, hook command, notice sentence and the run inputs that install it.
// The webster fork guard has no notice sentence and is not announced.
// No row denies the Agent tool: a subagent or fork runs inside the session and shows in its own pane,
// and its transcript lies under the session's subagents directory.
// The Bash denies below are not declared here:
// they render the rows of `internal/shellguard`'s table as PreToolUse(Bash) hooks.
// Every row follows the Claude rows, in table order.
// The python row denies a Bash command that runs `python`, `python3` or `python3.<minor>` in command position.
// It applies in every run mode;
// `shuttle.yaml`'s `deny_python` switches it off for a Python target repo.
// Its notice names a version check as refused, so a session does not probe for Python,
// and its steer names jq, grep and awk for parsing and the edit rule.
// Both give the reason and the way forward for work that needs Python itself.
// `deny_python: false` removes the hook and the notice sentence together.
// The publish row denies a Bash `git push`, with any flag words before `push`,
// and every `gh` call in command position except the reads, comments and title and body edits the table lists;
// it is installed only for a run whose spec sets `DenyPublish`.
// The shell guard table's `deny_publish` switch names that setting.
// It is announced like every other row.
// The command-shaped rows read the command as the shell would.
// A command word in quoted text, a heredoc body or another payload field such as the description does not trip a row.
// For every such row, these are not read and pass:
// a command substitution inside a double-quoted string;
// a command after a `<` before a quote or a backslash;
// any line after a heredoc operator's line.
// It is a guardrail, not a barrier: its hook greps the payload;
// so python behind `bash -c`, `eval`, a launcher or a command substitution inside a double-quoted string passes,
// and a separator inside a quote the scan misreads is falsely denied.
//
// The raw go row keeps a session from reaching the hub's gate slots around `lym gate test`.
// Every run installs it, the orch included.
// It denies a Bash `go test`, `go build` or `go vet` whose arguments include a pattern holding `...` or the word `all`;
// a `-C <dir>` before the subcommand changes nothing;
// a `go test` whose `-tags` value names `tmux` or `llm`;
// and any command setting or reading the slot-inheritance, strand-name or prebuilt-binary variable.
// Each is spelled into the pattern from its constant.
// It is denied when the variable is named outside quotes or expanded inside a double-quoted string.
// `go` must stand in command position directly before the subcommand;
// so `go test ./internal/x` and `lym gate test ./...` pass.
// The steer names a package-scoped `go build` or `go vet` as the compile check.
// It names each only while the hub's `refuse_raw` does not refuse it.
// It then states the gate rule:
// `lym gate test` runs in the foreground with the Bash tool's `timeout` at the ceiling;
// every other check runs in the foreground, and no `cd` goes into the session's own working directory.
// It is a guardrail, not a barrier: its hook sees static text only, so these pass:
// a run hidden behind a script or `bash -c`;
// a `cd` plus a relative pattern it does not cover;
// a shell variable or a command substitution inside a double-quoted string;
// any raw package-scoped `go test`.
// A pattern word inside a quoted argument of the go command itself, such as a `-run` filter, is falsely denied;
// a spelling in a commit message, a `grep` or a heredoc body is not.
//
// The refuse-raw row denies a Bash command starting with a prefix the hub lists in `gate.yaml`'s `refuse_raw`,
// so every test run in an agent session goes through `lym gate test`.
// `Prepare` loads `gate.yaml` over the engine's dirs, the hub file inside a hub and the worktree's own file outside one;
// it also fills `shuttleengine.Config.RefuseRaw` on its copy of the config before it builds anything;
// the settings and the notice then render `standingDeniesFor(cfg.RefuseRaw)`:
// the Claude denies followed by every shell guard row.
// An absent file leaves the list empty and renders no row;
// any other load failure refuses the spawn with the loader's way forward before an artifact is written.
// Settings render at spawn, so a running session keeps the list it started with.
// It is a guardrail, not a barrier: its hook sees static text only, so these pass:
// a run behind a script or `bash -c`;
// a flag between the prefix's words such as `go -C dir test`;
// an assignment prefix such as `GOFLAGS=x go test`.
//
// The shell-edit row denies a Bash `sed -i` or `perl -i` in command position;
// it also denies a `>`, `>>` or `tee` into a file with a source extension, a heredoc's included.
// Every run installs it, the orch included.
// It keeps a session on the Edit tool, which takes only a file read with Read.
// Its notice is the session's one statement of the edit rule, which the python row's steer carries too.
// It is a guardrail, not a barrier: these pass:
// an edit behind `bash -c`, a script or `awk`;
// a `sed -i` after `do` or `then`, behind `find -exec` or behind `xargs` with a flag;
// a redirect into a quoted target or a file of another extension;
// anything after a heredoc operator's line.
//
// The computed-rm row denies a Bash `rm` in command position when the same payload also holds either of these:
// a command substitution (`$(` or a backtick);
// a shell variable expansion (`$` followed by a letter, a digit, `_`, `{`, `@` or `*`).
// Every run installs it, the orch included.
// It is a deny and not a prompt because Claude Code raises a dialog for an `rm` on a computed path.
// The dialog is about a statically-unresolvable target.
// That dialog shows even in bypass mode and stalls an unwatched run.
// The steer names the literal path as the way forward.
// It is a guardrail, not a barrier: its hook sees static text only, so these pass:
// a removal behind `find -delete`, `xargs rm` with no `$`, `bash -c` or a script;
// a removal through `$$`, `$?`, `$#` or `$!`.
// These are falsely denied:
// an `rm` of a literal path in a command that uses `$` elsewhere;
// a `$` in another payload field such as the description beside an `rm` in command position.
// An `rm` spelled inside quotes, a `grep` alternation or a heredoc body is not denied.
//
// The headless claude row denies a Bash launch of `claude` or `claude.exe` in command position.
// The launch is denied when its arguments include `--print`;
// a single-dash flag word holding `p`, such as `-p` or `-pc`, also denies it.
// Every run installs it, the orch included.
// A headless session runs in no pane, so its work is hidden from the session that launched it;
// the steer and notice name a subagent through the Agent tool as the way to hand work off.
// It is a guardrail, not a barrier: these pass:
// a launch behind `bash -c`, a script, an assignment prefix or `env` with an assignment.
// A claude subcommand taking a single-dash flag word holding `p` of its own is falsely denied;
// so is a command launched through one.
//
// The open launch (Claude.OpenLaunch) starts a named interactive session with no prompt.
// Its line carries the session id and the strand-name reference, and adds `--model` and `--effort` only when set.
// It exports `CLAUDE_CODE_PROMPT_CACHE_TTL` at the role's TTL ahead of both lines.
// An open naming skills puts their load message after the binary on the fresh line as the session's first turn;
// the resume line carries none.
// An engine told no board directory passes no `--settings`, so the session takes the worktree's own settings.
// An engine told one writes these files into the directory Runner.Open names for the session:
// a settings.json holding creel's hook delivery, the run document's empty `attribution` and the session scope below;
// the settings.json holds nothing else;
// the append-system-prompt file holding the creel trust sentence.
// Both lines name the files, and the launch is marked CreelHooks.
// The settings' PostToolUse mail hook and idle waiter are a run's.
// Its Stop delivery is the merged command without the recording, since an opened session has no events file:
// it exits 0 whatever the delivery printed, so an echo reaches the operator.
// Claude Code stacks `--settings` over the worktree's own settings, so every other setting stays the worktree's.
// It carries `--dangerously-skip-permissions` on the fresh and the resume line alike.
// Only a permission mode of `prompt` leaves it out.
// An unknown mode refuses the open.
// Its resume line falls back to a fresh start under the same id.
// Claude Code keeps no transcript for a session that never took a turn, so `--resume` then fails.
//
// The session scope (scope.go) decides what a session loads besides lym's own text.
// It reads `claude.yaml`'s `session_scope` block.
// With `enabled` on, Prepare writes it into every run's settings;
// OpenLaunch writes it into an opened session's settings in a hub, as the documents' last keys;
// Claude Code's `--settings` layer outranks the user's and the repository's settings,
// so the scope holds whatever those say.
// The plugins file is Claude Code's `plugins/installed_plugins.json`, under `CLAUDE_CONFIG_DIR` or else `~/.claude`.
// Each plugin it names is on in `enabledPlugins` exactly when the session preloads a skill of that plugin.
// Every other plugin is off.
// `disableClaudeAiConnectors`, `disableBundledSkills` and `disableWorkflows` are true,
// and `skillOverrides` turns off each skill of `skills_off` the session does not preload.
// No session lym launches calls the `Workflow` tool, so the scope drops it.
// `disableBundledSkills` alone would move the workflow authoring reference into that tool's schema.
// A plugin that is on lists all of its skills:
// Claude Code reads `skillOverrides` for a plugin's skill only through its claude.ai skill sync;
// a `Skill(...)` deny rule refuses a call but leaves the listing;
// the session skills allowlist reaches only an SDK session.
// A session whose strand role is the orch's alone keeps its auto-memory and the `ListAgents` tool;
// every other session, run or opened, has `autoMemoryEnabled` false and `ListAgents` in its `permissions.deny`.
// A config directory or plugins file the engine cannot read is logged and names no plugin,
// so the session's other settings decide its plugins.
// An opened session outside a hub writes no settings and gets no scope.
// Bound: the settings and the behavior above are Claude Code 2.1.319's, as real launches showed them.
//
// A replay corpus of real turn ends lives under testdata/corpus, one directory per case:
// transcript.jsonl holds the source transcript's lines the parsers read, trimmed to the fields they read;
// events.jsonl is the run's events file, or one rebuilt from the transcript's turn ends when the run directory is gone;
// and case.yaml names the source run, the date, the Claude Code version and what went wrong;
// it also says whether the events are rebuilt, whether the run is gated and which skills Start loads;
// and it gives the reading each turn end must get: done, held, waiting with its outstanding tasks, or waiting_held.
// A waiting_held turn end is a waiting one that the wait loop judges as a plain turn end with an output missing.
// It is held at once.
// corpus_test.go replays every case directory through ParseEvents and shuttle's wait loop under an injected clock,
// so a new case joins without a code change.
// A failing llm-tier live scenario keeps its events file and transcript in a directory it prints;
// so promoting it to a case is a copy, a trim and a case.yaml.
package claudeengine
