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
// Both lines carry `CLAUDE_CODE_PROMPT_CACHE_TTL`, so a session writes its prompt cache at the TTL its role needs rather than at Claude Code's subscription default.
// Prepare resolves the value from `Spec.Role` through `shuttle.yaml`'s `claude_prompt_cache_ttl_roles` map, with `claude_prompt_cache_ttl` as the fallback for an empty or unmapped role,
// and validates the whole configured set to `5m` or `1h` (exact, case-sensitive) before any artifact is written.
// It logs the resolved value with the role.
// The value is fixed for the session's life, since a resume line is never rebuilt.
// The subagent bucket (`CLAUDE_CODE_SUBAGENT_PROMPT_CACHE_TTL`) is left at Claude Code's default,
// and a `FORCE_PROMPT_CACHING_5M` in Claude Code's own environment still wins.
//
// The context reading comes from the transcript a Stop payload names, read backwards from its end in doubling chunks.
// The latest main-chain assistant usage entry or compaction boundary is the reading, whichever sits later in the file;
// a boundary reading carries its `postTokens` and timestamp and is marked compacted.
// Every failure degrades to an unknown reading.
//
// The resume check refuses a session whose registry entry names a live pid, unless the live process's start time differs from the entry's `procStart`, which proves the pid was reused.
// An unreadable start time, or an entry without `procStart`, still refuses and says the pid could not be proven reused.
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
// The notice is built from the same inputs as the PreToolUse hooks, so the two cannot drift.
// The webster fork guard is not announced.
//
// A replay corpus of real turn ends lives under testdata/corpus, one directory per case:
// transcript.jsonl holds the source transcript's lines the parsers read, trimmed to the fields they read;
// events.jsonl is the run's events file, or one rebuilt from the transcript's turn ends when the run directory is gone;
// and case.yaml names the source run, the date, the Claude Code version, what went wrong, whether the events are rebuilt, whether the run is gated, the skills Start loads,
// and the reading each turn end must get: done, held, or waiting with its outstanding tasks.
// corpus_test.go replays every case directory through ParseEvents and shuttle's wait loop under an injected clock, so a new case joins without a code change.
// A failing llm-tier live scenario keeps its events file and transcript in a directory it prints, so promoting it to a case is a copy, a trim and a case.yaml.
package claudeengine
