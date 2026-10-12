// Package claudeengine is the Claude adapter behind shuttleengine.Engine: all Claude-specific
// knowledge — CLI flags, the settings.json hook schema, TUI startup/trust markers, and pane key
// choreography — lives here and nowhere else.
// shuttleengine and the run loop it drives know only the Engine interface;
// a second provider engine would be added alongside this package without touching either.
//
// One piece of that knowledge is worth stating at package level because getting it wrong is silent
// and total: claude's trust-this-folder gate is a SELECTION LIST, not a confirmation prompt, and
// the caret does not start on the accepting option.
// Claude 2.1.263 puts it on "No, exit", so confirming whatever is selected quits claude and every
// agent lyx spawns in a directory claude has not previously been accepted in dies at startup —
// which is every freshly-created fabric worktree pair.
// TrustDismissSequence therefore reads the caret out of the capture it is given rather than
// assuming a position, and presses nothing at all when it cannot find the accepting option.
//
// The mirror of that knowledge is worth stating too, because getting it wrong is equally silent and
// equally total: a capture is the AGENT'S OWN TRANSCRIPT once a run is under way, and every phrase
// that identifies a gate is ordinary English an agent writes ("the files in this folder", "trust this
// folder", "yes, I accept").
// So a gate is never classified from a phrase alone. Startup requires positive evidence that a
// dialog is rendered — an accepting-option LINE, recognized by what it begins with once the caret and
// list numbering are stripped, or claude's own "Enter to confirm" gate footer — and it locates that
// option line with the same helper TrustDismissSequence uses, so the classifier can never name a gate
// the dismissal would then have to walk blind.
// The evidence must also sit where a rendered gate puts it: the option line adjacent to the last
// caret, the footer just below it — because a prose LIST ITEM that begins with an accept phrase
// ("- Yes, I accept the risk") is itself a matching option line, while the only caret on a healthy
// pane is its input-box marker at the bottom, far from any transcript prose above (crucible round
// fable-high-r7, F1).
//
// A Stop line whose turn ended with background work still running becomes EventWaiting, and its
// Event.Outstanding lists that work as provider-neutral shuttleengine.BackgroundTask values:
// a fork for an Agent or Task subagent, a shell for a backgrounded Bash or a Monitor.
// The Stop payload's background_tasks list is authoritative when present, even empty:
// its running entries are the whole list and the transcript is not read.
// The transcript fallback runs only when the key is absent or not a list.
// It lists the background launches that no later completion notification names:
// a task-notification user message, a queue-operation line or a queued_command attachment.
// A payload that omits a running task, lists it under another status or changes an entry's shape counts as no outstanding work.
// Each task carries the signal that reported it.
//
// Every Bash command an agent runs gets `/dev/null` as its default stdin through Claude Code's `CLAUDE_ENV_FILE`:
// Prepare writes `bash-env.sh` beside settings.json with the content `exec </dev/null`, and both the launch and the resume line lead with `CLAUDE_ENV_FILE` naming its absolute path,
// so an interpreter left reading the tool's open stdin ends at once instead of hanging the session, and a resumed session keeps the default.
// The file runs that one statement and nothing else.
// It changes only the default stdin of the tool's own shell: a heredoc, pipe or redirect attached to a command still supplies that command's stdin,
// a child `bash` the agent starts does not re-run the file, and it grants or denies nothing, so the agents' permission rules are untouched.
// A `CLAUDE_ENV_FILE` the operator's own environment exports is overridden for lyx-spawned sessions only.
//
// ColorSequence is the one place lyx's palette maps to Claude Code's `/color` names: the eight palette colors map to the same-named Claude Code colors,
// and the sequence is `/color <name>` typed and submitted, with no leading Escape, the way ModelSwitchSequence submits `/model`.
// A color outside the palette answers no inputs.
//
// Both lines carry `CLAUDE_CODE_PROMPT_CACHE_TTL`, so a session writes its prompt cache at the TTL its role needs rather than at Claude Code's subscription default.
// Prepare resolves the value from `Spec.Role` through `shuttle.yaml`'s `claude_prompt_cache_ttl_roles` map, with `claude_prompt_cache_ttl` as the fallback for an empty or unmapped role,
// and validates the whole configured set to `5m` or `1h` (exact, case-sensitive) before any artifact is written.
// It logs the resolved value with the role.
// The value is fixed for the session's life, since a resume line is never rebuilt.
// PromptCacheLifetime answers the same resolved value as a duration, so a caller can keep an idle session's cache warm inside it.
// The subagent bucket (`CLAUDE_CODE_SUBAGENT_PROMPT_CACHE_TTL`) is left at Claude Code's default,
// and a `FORCE_PROMPT_CACHING_5M` in Claude Code's own environment still wins.
//
// Both lines also carry `BASH_MAX_TIMEOUT_MS`, the ceiling on the Bash tool's `timeout` parameter, set to one hour from `bashMaxTimeout`.
// `BASH_DEFAULT_TIMEOUT_MS` is left at Claude Code's default, so only a call with an explicit `timeout` runs past the default.
// No call stays in the foreground past the ceiling: at the ceiling the Bash tool ends the call or moves it to the background.
//
// The context reading comes from the transcript a Stop payload names, read backwards from its end in doubling chunks.
// The latest main-chain assistant usage entry or compaction boundary is the reading, whichever sits later in the file;
// a boundary reading carries its `postTokens` and timestamp and is marked compacted.
// Every failure degrades to an unknown reading.
//
// The session-usage reading (UsageReader) sums a finished session's tokens from its parent transcript and the fork transcripts under its `subagents/` directory.
// Fresh tokens are input plus cache-creation plus output, and cache-read tokens count separately;
// a message counts once however many transcript lines carry it, and the totals include the forks, which are also broken out.
// A fork transcript's entries are all sidechain and count, except the parent's spawning message it replays as its first entry.
// An unreadable transcript makes the whole reading unknown, and a missing `subagents/` directory is zero forks.
//
// The session-signal parse (SessionSignalParser) reads the same events file as ParseEvents but yields a separate stream, in signals.go, and leaves ParseEvents unchanged.
// A `Stop` line is a turn end with its outstanding tasks read exactly as ParseEvents reads them, a `StopFailure` an API-error turn end, a `UserPromptSubmit` a turn start, a `PreToolUse` for `AskUserQuestion` an ask, a `PermissionRequest` an ask by its hook name alone, a `Notification` by the fixed table `notificationSignalKinds` (`permission_prompt`, `elicitation_dialog` and `elicitation_url_dialog` are asks, `idle_prompt` is an idle notice, and `agent_needs_input`, `elicitation_response` and any other type are skipped), a `SessionEnd` a session end with its reason, and a `SessionStart` a session start carrying the payload's `source` mapped to a provider-neutral one: `startup`, `resume`, `clear` and `compact` to their constants, and any other or missing string to the empty source.
// The asks that are not events, the ones the dialog notice reads, are the `PermissionRequest` hook and the `permission_prompt`, `elicitation_dialog` and `elicitation_url_dialog` Notifications, a fork's included; the question tool's ask is an event.
// The session fold reads a `resume` or `clear` start as an idle-fresh session, and a `startup` start too unless a turn is running; a `compact` start moves nothing.
// A `SessionEnd` reason of `logout`, `prompt_input_exit` or `other` ends the process; `clear`, `resume` and an unknown reason do not.
// A recording hook writes a stamp line `{"lyx_stamp":"<hook>","lyx_at":"<RFC 3339 UTC>"}` before its payload, and the parser gives a payload the time of the nearest preceding untaken stamp naming its hook.
// A payload with no such stamp, or whose stamp's time is empty or malformed, reads a zero time.
// The count the parser reports as consumed stops before the trailing run of stamp lines with no payload after them, so an incremental reader sees each stamp with its payload at its next read.
// A transcript-fallback turn end's tasks are read from the transcript at parse time, so the same line parsed again later can read fewer.
//
// The events file holds, per recording hook (Stop, UserPromptSubmit, StopFailure, Notification, SessionEnd, SessionStart, PermissionRequest and, in an interactive run, the AskUserQuestion record), a stamp line and then the hook's payload line.
// The hook command is plain POSIX `sh`, `date`, `cat` and `printf`: one `printf` writes the stamp, `;` joins it to the unchanged payload append so a failed stamp never blocks the payload, and every hook but Stop ends in `; true` so it exits 0 whatever the append does.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
// Every `SessionStart` source is recorded exactly once.
// A run without a context-after-compaction command records them in one entry with no matcher;
// a run with one records the sources but `compact` in an entry matching `startup|resume|clear`, and `compact` in its own entry.
// No recording hook prints to standard output, so the `PermissionRequest` recording answers no request and leaves Claude's own dialog to decide it.
// The one exception is the second command of the `compact` `SessionStart` entry, which Spec.ContextAfterCompaction sets: it runs after the recording command and verbatim, and its output joins the session's context after every compaction.
// SessionStartContext renders the additional-context JSON such a command prints.
// Prepare declares the launch as recording session starts, so startup waits for the `startup` line beside the ready pane.
// Two accepted races follow from the append order.
// Hooks that fire at once can interleave their stamps ahead of both payloads, which the pairing by hook name absorbs;
// and a reader can see a payload without its newline, or a stamp without its payload, which the parser leaves unconsumed until the line completes.
// The stamp's time is taken when the hook starts, so it can precede the payload's append by the length of the hook's own run.
//
// The activity reading (ActivityReader) takes the session's last activity, its forks included, and walks the main transcript backwards for the newest main-chain assistant entry that ends a turn or carries Claude Code's `isApiErrorMessage` marker.
// The last activity is the later of the main transcript's last write and the newest fork reading under the `subagents/` directory beside it:
// a fork's last write time, or the reading clock's now when the fork holds a tool call with no result and was last written no more than `bashMaxTimeout` ago.
// The pending-call reading looks at the fork's newest tool-calling message only, read backward from the transcript's end:
// a message is every assistant entry sharing one message id, and it is pending when any of its calls has no result after it, so an unanswered call in an older message no longer counts.
// A held foreground call writes nothing, so the pending-call reading keeps a Master waiting on a working fork from reading quiet;
// a fork stopped mid-call reads active for at most that ceiling past its last write, which a live call never outlasts.
// The API-error part reads the main transcript only.
// A session whose newest such entry carries the marker stands on an API error, and the entry's final text is the error's text;
// a sidechain entry never counts, and a later normal turn end clears the error.
// Every failure degrades to the zero reading.
//
// The resume check refuses a session whose registry entry names a live pid, unless the live process's start time differs from the entry's `procStart`, which proves the pid was reused.
// An unreadable start time, or an entry without `procStart`, still refuses and says the pid could not be proven reused.
//
// The session prober (SessionProber) reads process liveness from the same registry.
// A session is alive when an entry names it and the live pid's start time equals `procStart`.
// It is dead only for an entry whose pid is gone.
// It is unproven when no entry names it, the registry or an entry is unreadable, the start time is unreadable or the pid was reused.
// A session Claude Code exited normally has no entry left, so it reads unproven.
// The resume check and the prober share one reader of registry entries.
// The prober also reads the interrupt marker.
// The newest main-chain entry of the transcript a turn start's `transcript_path` names is the user's `[Request interrupted by user` entry, and its own timestamp is the interrupt's time.
// Any later main-chain entry, a missing path or an unreadable transcript reads as no interrupt.
//
// The engine takes text as one bracketed paste (TextPaster): shuttle pastes a send through reed's paste-submit and, with `PasteSubmits` true, the Enter follows the paste end in the same tmux invocation.
// `PasteSubmits` is true on the assumption that an Enter right after a bracketed paste's end submits it in Claude Code; a live check pins it, and if it fails the one constant flips and shuttle sends the first Enter itself, guarded.
// No sequence opens with an Escape: prompt suggestion is off in every shuttle-launched session, so an Escape guarded nothing and could complete a double-Esc or interrupt a turn.
// ComposeSend is the text step and the Enter, TypeSequence the text step alone, and the session commands and the color command are their text and an Enter with no pause between them.
// ClearInputSequence is one `C-u`, which deletes from the caret to the start of the line and so empties a one-line draft without interrupting a running turn.
// It serves sent text and a lone stray-character candidate alike, since both are one line with the caret at its end.
// OwnPastePlaceholder recognises the `[Pasted text #<n>]` or `[Pasted text #<n> +<m> lines]` form Claude Code collapses a pasted draft into, which a box reading cannot otherwise match against the pasted text.
// It answers true only for a text of at least `PasteCollapseMinChars` characters, whose newline count equals `m` when the placeholder shows one.
// The box reading takes a box holding only a lone `%`, the shell's end-of-line mark, as empty.
// StrayInput is the judgment beside that rule: a box holding exactly one character that is neither a letter, a digit nor whitespace, with no running turn on screen, is a stray-character candidate and the character is returned; the lone `%` is never one.
// IdleSession, StrayInput and RunningTurn share one running-turn reading, the "esc to interrupt" hint matched over the whole capture.
// RewindPicker reads the rewind picker: the picker's subtitle is on screen and no input box is, so a transcript quoting the subtitle above a live box is not the picker.
// InterruptSettle is 2 seconds, assuming Claude Code's double-Esc window is shorter, so two keys lyx sends never read as a double Esc.
//
// Beside the clear sequence (`/clear`) and the compact sequence (`/compact`), the engine realizes skill loading, in skillload.go.
// A spec that names skills starts on an empty input box: the launch line carries no prompt pointer, and the pointer comes back as Launch.PromptLine for shuttle to send after the skills.
//
// The engine realizes a whole skill list as one typed message with no leading slash, SkillLoadMessage, that asks the model to load each skill through the Skill tool in one turn, in list order.
// ClassifySkillLoad checks that turn from the transcript a Stop payload names, read backwards from its end:
// the latest main-chain user entry equal to the message starts the turn,
// and a skill is loaded when its Skill call is answered by a successful result, unknown when that result is an error or the latest skill listing does not name it, and missing otherwise.
// A missing transcript_path, an unreadable file, a transcript with no matching message or one with no skill listing degrades to an unverified report.
//
// The engine also announces each standing tool deny to the session through --append-system-prompt, on both the launch and the resume line.
// Both the hooks and the notice are built from one deny table in settings.go, so the two cannot drift:
// each row names its tool matcher, hook command, notice sentence and the run inputs that install it.
// The webster fork guard has no notice sentence and is not announced.
// The python row denies a Bash command that runs `python`, `python3` or `python3.<minor>` in command position, in every run mode;
// `shuttle.yaml`'s `claude_deny_python` switches it off for a Python target repo.
// It is a guardrail, not a barrier: its hook greps the payload, so a quoted argument or heredoc body can trip it falsely, and python behind `bash -c`, `eval` or a launcher passes.
//
// The raw go row keeps a session from reaching the hub's gate slots around `lyx gate test`, and every run installs it, the orch included.
// It denies a Bash `go test`, `go build` or `go vet`, with or without `-C <dir>` before the subcommand, whose arguments include a pattern holding `...` or the word `all`;
// a `go test` whose `-tags` value names `tmux` or `llm`;
// and any command naming the slot-inheritance variable or the strand-name variable, each spelled into the pattern from its constant.
// `go` must stand in command position directly before the subcommand, so `go test ./internal/x` and `lyx gate test ./...` pass.
// The steer is the gate rule: it names `lyx gate test` run in the foreground with the Bash tool's `timeout` at the ceiling, every other check in the foreground, and no `cd` into the session's own working directory.
// It is a guardrail, not a barrier: its hook sees static text only, so a run hidden behind a script, `bash -c`, a `cd` plus a relative pattern it does not cover, or a shell variable passes, as does any raw package-scoped `go test`,
// and a matching spelling inside a quoted argument, a commit message or a `grep` is falsely denied.
//
// The in-place edit row denies a Bash `sed` or `perl` whose words include an in-place flag, and every run installs it, the orch included, with no config key.
// For `sed` that is `-i`, `-i<suffix>`, a short-flag cluster holding `i`, or any prefix of `--in-place` from `--i` up;
// for `perl`, a short-flag cluster holding `i`.
// Reading with `sed -n` passes, and the steer points at the edit directive's rule.
// It is a guardrail, not a barrier: its hook sees static text only, so `sed` writing by redirection, a `w` command inside a script, `awk`, `find -exec sed -i`, anything behind `bash -c`, `eval` or a variable, and anything after a prefix the pattern does not list pass,
// and a matching spelling inside a quoted argument, a `grep` alternation or another payload field, or a `perl` read whose cluster holds an `i`, is falsely denied.
//
// The computed-rm row denies a Bash `rm` in command position when the same payload also holds a command substitution (`$(` or a backtick) or a shell variable expansion (`$` followed by a letter, a digit, `_`, `{`, `@` or `*`), and every run installs it, the orch included.
// It is a deny and not a prompt because Claude Code raises a statically-unresolvable-target dialog for an `rm` on a computed path, which shows even in bypass mode and stalls an unwatched run.
// The steer names the literal path as the way forward.
// It is a guardrail, not a barrier: its hook sees static text only, so a removal behind `find -delete`, `xargs rm` with no `$`, `bash -c` or a script passes, as does one through `$$`, `$?`, `$#` or `$!`,
// and an `rm` of a literal path in a command that uses `$` elsewhere, a `grep` alternation naming `rm`, or a matching spelling in another payload field such as the description is falsely denied.
//
// A replay corpus of real turn ends lives under testdata/corpus, one directory per case:
// transcript.jsonl holds the source transcript's lines the parsers read, trimmed to the fields they read;
// events.jsonl is the run's events file, or one rebuilt from the transcript's turn ends when the run directory is gone;
// and case.yaml names the source run, the date, the Claude Code version, what went wrong, whether the events are rebuilt, whether the run is gated, the skills Start loads,
// and the reading each turn end must get: done, held, or waiting with its outstanding tasks.
// corpus_test.go replays every case directory through ParseEvents and shuttle's wait loop under an injected clock, so a new case joins without a code change.
// A failing llm-tier live scenario keeps its events file and transcript in a directory it prints, so promoting it to a case is a copy, a trim and a case.yaml.
package claudeengine
