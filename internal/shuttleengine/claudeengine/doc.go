// Package claudeengine is the Claude adapter behind shuttleengine.Engine.
// All Claude-specific knowledge lives here and nowhere else:
// CLI flags, the settings.json hook schema, TUI startup and trust markers, and pane key choreography.
// shuttleengine and the run loop it drives know only the Engine interface,
// so a second provider engine would be added alongside this package without touching either.
//
// Claude's trust-this-folder gate is a selection list, not a confirmation prompt;
// the caret does not start on the accepting option.
// Confirming the selection as it stands quits claude, so every agent spawned in an unaccepted directory dies at startup.
// That includes every freshly created fabric worktree pair.
// TrustDismissSequence reads the caret out of the capture it is given rather than assuming a position,
// and it presses nothing when it cannot find the accepting option.
//
// The mirror hazard is equally silent.
// Once a run is under way a capture is the agent's own transcript,
// and every phrase that identifies a gate is ordinary English an agent writes.
// A gate is therefore never classified from a phrase alone.
// Startup requires positive evidence that a dialog is rendered:
// an accepting-option line adjacent to the last caret, or claude's own "Enter to confirm" footer just below it.
// The classifier locates the option line with the helper TrustDismissSequence uses,
// so it never names a gate the dismissal would then have to walk blind.
//
// A Stop line whose turn ended with background work still running becomes EventWaiting,
// and its Event.Outstanding lists that work as provider-neutral shuttleengine.BackgroundTask values.
// The Stop payload's background_tasks list is authoritative when present, even empty;
// the transcript fallback runs only when the key is absent or not a list.
// A payload that hides a running task counts as no outstanding work.
// The outstanding-fork reading (OutstandingForkReader) reads the session's own transcript, narrowed to forks.
// Claude Code writes a queue-operation line the moment a notification arrives during a tool call,
// so the reading turns empty while a session that waits on its forks is still inside the waiting call.
//
// Every Bash command an agent runs gets `/dev/null` as its default stdin through Claude Code's `CLAUDE_ENV_FILE`.
// Prepare writes `bash-env.sh` beside settings.json,
// and both the launch and the resume line lead with `CLAUDE_ENV_FILE` naming its absolute path,
// so an interpreter left reading the tool's open stdin ends at once instead of hanging the session.
// The file also prepends the parent directory of `agentname.LymBinEnv` to `PATH`,
// so the Bash tool shadows the executables the pane shell shadows;
// a resumed session follows its pane's latest binary.
// It changes only the default stdin and `PATH` of the tool's own shell and grants or denies nothing.
// A `CLAUDE_ENV_FILE` the operator's own environment exports is overridden for lym-spawned sessions only.
//
// ColorSequence maps every lym palette color to the same-named Claude Code `/color` name.
//
// The engine owns a layered config module, `claude.yaml`, declared in config.go.
// The `accounts` list and the `account_pins` block are machine-layer keys:
// `lym config claude --layer machine` writes them, and LoadConfig refuses a non-empty one in a tracked layer.
// A local override may set every key but the binary and the tool denies.
// The engine is told its directories and derives none.
// Prepare, OpenLaunch, PromptCacheLifetime and KeepWarmCaps each load claude.yaml through them before anything else,
// so an invalid file refuses a run start, a resume and an open, naming the file, the key and `lym config claude`.
// No other method and neither constructor loads it;
// an engine built by New, with zero directories, refuses those methods.
// NewFromConfig keeps the zero Worktree, which means no worktree is told.
//
// The told worktree's account directory holds the account file `accounts.json`.
// It also holds the usage records `usage/<account>.json`.
// Both are caches a fresh machine starts without:
// a missing, empty or undecodable file reads as the zero record.
// Each is rewritten under its own lock with the keys it does not declare kept.
// A worktree runs under the hub pin; else, for the prime and a beam, the beams pin; else its own entry.
// A beams pin whose account a usage limit blocks yields to the worktree's entry when it has one.
// An entry naming an unlisted or retired account counts as none.
// PickAccount implements shuttleengine.AccountKeeper and SwitchAccount implements shuttleengine.AccountSwitcher;
// both change nothing under a hub pin and with fewer than two accounts of positive weight.
// A pick takes, among logged-in accounts of positive weight with headroom, the lowest count over weight.
// SwitchAccount records a usage limit against the account the run launched under.
// It reads that account from the run directory's `account.json`.
// It moves the worktree's entry to the best other logged-in account.
// SetAccount, ShowAccounts and RecordStatusLine are the verbs of `lym claude`.
// RecordStatusLine never writes or clears the blocked-until time.
// Every launch the engine builds resolves the told worktree's account first.
// A launch whose config or env module does not load is refused; a bad account list never degrades to none.
// An account off Claude Code's default directory is exported as `CLAUDE_CONFIG_DIR`.
// The export is outermost on the launch and resume lines.
// An account on the default directory unsets the variable instead, since setting it there moves `.claude.json`.
// A strand relaunched from its stored line keeps the account it launched under.
// A session on a listed account gets the lym status line, which replaces the operator's own in the sessions it covers.
// ScrubbedEnv names `CLAUDE_CONFIG_DIR`,
// so a server spawned from a pane on one account never hands it to a pane with no export.
//
// The launch and resume lines both carry `CLAUDE_CODE_PROMPT_CACHE_TTL`,
// so a main conversation writes its prompt cache at the TTL its role needs, not at Claude Code's subscription default.
// Prepare and OpenLaunch resolve it from the spec's role through `prompt_cache_ttl_roles`, else `prompt_cache_ttl`.
// They refuse a configured value that is not `5m` or `1h` before any artifact is written.
// The value is fixed for the session's life, since a resume line is never rebuilt.
// `keep_warm_cap` and `keep_warm_cap_roles` set the longest single wait an agent of a role blocks on.
// Both lines also carry `BASH_MAX_TIMEOUT_MS` at one hour, so no Bash call stays in the foreground past that ceiling.
// The launch, resume and open lines carry `CLAUDE_CODE_SUBAGENT_PROMPT_CACHE_TTL` at one hour.
// Claude Code moves a background subagent's foreground Bash call to the background after 180 seconds.
// A subagent whose prompt cache is one hour is exempt.
// On the 5-minute default, a fork's capped `lym gate test` call would leave the foreground before its keep-warm cap.
// The hour lets a fork's cache outlive any cap, at one-hour cache-write prices for subagents, forks and helper requests.
//
// The context reading comes from the transcript a Stop payload names:
// the latest main-chain assistant usage entry or compaction boundary.
// Every failure degrades to an unknown reading.
//
// The session-usage reading (UsageReader) sums a finished session's tokens, forks included and also broken out.
// A message counts once however many transcript lines carry it.
// An unreadable transcript makes the whole reading unknown, and a missing `subagents/` directory is zero forks.
//
// The transcript reader (TranscriptReader) locates and reads finished sessions with no run and no shuttle config.
// A worktree's project directory is `projects/<dir>` under every known config directory;
// `<dir>` is the worktree path with every non-alphanumeric byte as a hyphen.
// The known directories are each listed account's directory, retired accounts included.
// They also include the env module's `CLAUDE_CONFIG_DIR` and `~/.claude`.
// A session id found in several directories is one session, its most recently written copy.
// A missing projects root, project directory or `subagents/` directory yields none of them and no error.
// A malformed or half-written line is skipped;
// a vanished transcript fails the read, naming its path.
//
// The message pricer (MessagePricer) prices one message's neutral usage from its model.
//
// The session-signal parse (SessionSignalParser) is in signals.go.
// It reads the same events file as ParseEvents but yields a separate stream of provider-neutral signals.
// The stream holds one signal per hook line.
// A recording hook writes a stamp line `{"lym_stamp":"<hook>","lym_at":"<RFC 3339 UTC>"}` before its payload,
// and the parser gives a payload the time of the nearest preceding untaken stamp naming its hook;
// a payload with no usable stamp reads a zero time.
// The count the parser reports as consumed stops before the trailing run of stamp lines with no payload after them,
// so an incremental reader sees each stamp with its payload at its next read.
// A transcript-fallback turn end's tasks are read from the transcript at parse time,
// so the same line parsed again later can read fewer.
//
// The events file holds a stamp line and then the hook's payload line for each recording hook.
// The hook command is plain POSIX `sh`, `date`, `cat` and `printf`.
// A failed stamp never blocks the payload append, and every hook but Stop exits 0 whatever the append does.
// No recording hook prints to standard output,
// so the `PermissionRequest` recording answers no request and leaves Claude's own dialog to decide it.
// The one exception is the `compact` `SessionStart` entry's second command, which Spec.ContextAfterCompaction sets.
// Hooks that fire at once can interleave their stamps ahead of both payloads, which the pairing by hook name absorbs.
//
// Every run's PostToolUse, PreCompact and PostCompact hooks touch shuttleengine.ActivityStampFileName.
// The file sits beside the events file.
// A tool call, a fork's included, and a compaction move the run's activity stamp without growing the events file.
// No SubagentStart or SubagentStop hook is installed:
// an unmatched start would read a crashed fork active without bound.
// The settings leave Claude Code's `autoContinueAtUsageLimit` unset,
// so a session the usage limit stopped waits for the orch's wake and revive rather than racing it.
// They hold only what differs per session or role.
// What every session of a worktree shares comes from the worktree's `.claude/settings.local.json`.
// That file includes the empty `attribution` and `promptSuggestionEnabled` false, which IdleSession needs.
// heddle renders that file at the session's anchor, and a session started where none is rendered goes without them.
//
// An engine told a hub board directory also installs creel's mail hook as a second PostToolUse entry,
// and merges creel's delivery into the Stop recording; a standalone run installs the plain recording alone.
// HookOutput hands the strand's mail to the session as additional context.
// It also shows the mail to the operator as the system message.
// No output carries `decision` or `reason`:
// a blocking hook would reach the model twice and the operator once, while additional context reaches it once.
// RewakeOutput returns the text plus a newline and exit code 2, the `asyncRewake` wake.
// Claude Code runs an event's matching hooks in parallel,
// so a separate creel Stop hook could not keep the recording from logging a turn end its mail then continues.
// The merged Stop command therefore records only when no mail was handed out,
// and an exit other than 0 records the turn end.
// The idle waiter runs as a second Stop entry with `asyncRewake`, waking the idle session when it exits 2.
// Prepare marks the launch CreelHooks exactly when it passes the board directory's flag directory.
// A spec's turn hook command (Spec.TurnHookCommand) joins the UserPromptSubmit and Stop commands.
// It runs with its output discarded, and in the merged Stop command it runs before creel's delivery.
// Bound: a merged command killed at its timeout after the write never reaches its record step,
// so the turn ends with no Stop record when Claude Code drops the killed hook's output.
//
// The edit hook (EditRecorder) is a PostToolUse entry matching `Edit|Write`.
// It is carried for a run that is not interactive while shuttle.yaml's `edit_loop_edits` is above 0.
// RecordEdit appends to shuttleengine.EditRecordsFileName, never to the events file.
// Each record carries the file's content hash.
// With it, shuttle's edit-loop watch can tell an edit that returns the file to earlier content.
//
// The batching nudge (BatchingNudger) is a PostToolUse hook.
// It adds one line to the calling chain's context when the chain's recent calls break lym's batching rules;
// it never denies a call.
// `claude.yaml`'s `batching_nudge` block holds `enabled`, the breach switches and the thresholds.
// ValidateBatchingNudge refuses invalid thresholds while the hook is on.
// Prepare copies the block into `batching-nudge.json` beside the events file,
// so a running session keeps the values its Prepare wrote.
// Claude Code writes a message's lines to the transcript only after its calls start,
// so the judgment rereads the tail until a message holds the call, and a call still missing is left silent.
// A call carrying an agent id reads and counts against that agent's own sidechain transcript.
// Bound: the hook is a heuristic, since some lookups must run in sequence, so it fires at times on a right call.
//
// The activity reading (ActivityReader) takes the session's last activity, its forks included.
// A fork with a tool call that has no result, last written within `bashMaxTimeout`, reads as active now.
// A held foreground call writes nothing, so a Master waiting on a working fork would otherwise read quiet.
// It also reports an API error: the newest main-chain assistant entry that carries `isApiErrorMessage`.
// A sidechain entry never counts, and a later normal turn end clears the error.
// The usage limit carries the reset time the entry's text states, if it has a readable `resets 2pm (Europe/Oslo)` clause.
// The classification alone decides that the limit stopped the turn.
// Every failure degrades to the zero reading.
//
// The usage-limit reading (UsageLimitReader) reads Claude Code's own classification,
// the `error` field `rate_limit` on a synthetic `isApiErrorMessage` entry,
// and never the entry's text or the agent's words.
// A turn end is usage-limited when the main chain's newest turn end is such an entry.
// It is also usage-limited when a fork's newest assistant entry is one written after the main chain's previous turn end.
// Every failure reads as no limit.
//
// CarryTranscript runs ahead of the resume check.
// It copies the session's newest transcript into the directory of the worktree's resolved account, leaving the source,
// so a resume finds its transcript after the worktree's account changed.
// The resume check refuses a session whose registry entry names a live pid, with a `shuttleengine.SessionHeldError`.
// The refusal is lifted when the process's start time differs from the entry's `procStart`.
// That difference proves the pid was reused.
// A session with no transcript in the resolved directory's project for the pane cwd is refused.
// The refusal wraps `shuttleengine.ErrNoTranscript`.
// SessionHolder names the live pid holding a session by the same rule.
//
// The session prober (SessionProber) reads process liveness from the registries of every known config directory.
// It treats the registries as one set.
// A session is alive when an entry names it and the live pid's start time equals `procStart`.
// It is dead only for an entry whose pid is gone and no registry or entry is unreadable;
// every other case, including a session Claude Code exited normally, reads unproven.
// The prober also reads the interrupt marker, the user's `[Request interrupted by user` entry.
// The marker is read from the transcript a turn start's `transcript_path` names.
// It counts only as the newest main-chain entry; a missing path or unreadable transcript reads as no interrupt.
// A turn's progress (TurnProgressReader) is read from the newest main-chain user or assistant entry:
// in flight, over or unknown, with every missing or ambiguous case reading unknown.
//
// The engine takes text as one bracketed paste (TextPaster).
// `PasteSubmits` is true on the assumption that an Enter right after a bracketed paste's end submits it in Claude Code;
// a live check pins it, and if it fails the one constant flips and shuttle sends the first Enter itself.
// No key sequence opens with an Escape, which could complete a double-Esc or interrupt a turn.
// InterruptSettle is 2 seconds, on the assumption that Claude Code's double-Esc window is shorter.
// ClearInputSequence is one `C-u`, which empties a one-line draft without interrupting a running turn.
// RestoredPromptClearSequence clears the prompt an interrupt restored into the box.
// It sends one `C-u` per box line with a `BSpace` between each pair, since `C-u` deletes only the caret's row.
// OwnPastePlaceholder recognises the `[Pasted text #<n>]` form Claude Code collapses a pasted draft into.
// PlainBoxCapture blanks a box whose every cell but a lone unstyled `%` is drawn faint.
// That is how Claude draws its placeholder and prompt suggestion.
// Anything else reads as a draft, so the reading fails closed.
// IdleSession, StrayInput and RunningTurn share one running-turn reading:
// the "esc to interrupt" hint matched over the whole capture.
// RewindPicker and DismissiblePicker read a picker as shown only when its phrases are on screen and no input box is,
// so a transcript quoting a picker's text above a live box is not a picker.
//
// A spec that names skills carries the skill-load message on the launch line, so the session's first turn is the load;
// the prompt pointer comes back as Launch.PromptLine for shuttle to send after the skills.
// ClassifySkillLoad checks that turn from the transcript a Stop payload names.
// It degrades to an unverified report whenever the transcript cannot settle it.
//
// The engine announces each standing tool deny to the session and tells it to trust and answer creel mail.
// It does so through the run directory's append-system-prompt.txt, which the launch and the resume line both name.
// The file holds the deny notice and the creel trust sentence.
// It also holds the subagent passage from the shuttle config's agents.yaml.
// Finally, it holds the standing passages heddle's layers hold for the claude engine.
// The text goes in a file because its sentence holds a double quote.
// An argument passed through pwsh's Legacy mode (an npm claude.cmd shim) would drop that quote.
// A resume reuses the file its launch wrote; a Prepare that runs again reads the passages afresh.
// The settings set `CLAUDE_CODE_SUBAGENT_EFFORT` to the role's effort on a model the efforts table does not name.
// A spec that names a remote-control session adds `--remote-control` with that name to the launch and resume lines.
// A session's remote control reads as connected once its transcript holds a `bridge-session` record.
// Claude Code writes that record when the connection is made.
// A connection Claude Code refuses, such as one an organization's policy disables, writes none.
//
// Both the deny hooks and the notice are built from one deny list in settings.go, so the two cannot drift.
// The Claude rows come first; the Bash denies render the rows of `internal/shellguard`'s table as PreToolUse(Bash) hooks,
// and that package documents what each row matches, lets through and falsely denies.
// No row denies the Agent tool: a subagent or fork runs inside the session and shows in its own pane.
// Every row is a guardrail over static text and not a barrier.
// The python row is installed in every run, and its hook asks `shuttle hook-python-allowed` on each matching call,
// so a change of `shuttle.yaml`'s `deny_python` reaches running sessions with no restart;
// a lym that is missing or fails keeps the deny.
// The publish row is installed only for a run whose spec sets `DenyPublish`.
// The tmux and script row is installed in every run whose spec does not set `AllowRawTmux`, interactive runs included.
// The refuse-raw row denies a Bash command starting with a prefix the hub lists in `gate.yaml`'s `refuse_raw`,
// so every test run in an agent session goes through `lym gate test`.
// Prepare reads `gate.yaml`'s `refuse_raw` over the engine's dirs through `gateslot.LoadRefuseRaw`.
// It fills `shuttleengine.Config.RefuseRaw` on its copy of the config.
// It renders `standingDeniesFor(cfg.RefuseRaw, lymPath)`.
// An absent file leaves the list empty and renders no row.
// A failure in a key the launch does not read is logged as a warning and the launch goes on.
// A file that does not merge refuses the spawn with the loader's way forward before an artifact is written.
// So does an invalid `refuse_raw`.
// Settings render at spawn, so a running session keeps the list it started with.
//
// The open launch (Claude.OpenLaunch) starts a named interactive session with no prompt.
// An engine told no board directory passes no `--settings`, so the session takes the worktree's own settings.
// An engine told one writes a settings.json holding creel's hook delivery and the session scope.
// It also writes an append-system-prompt file holding the creel trust sentence alone.
// Both go into the directory Runner.Open names.
// Its Stop delivery is the merged command without the recording, since an opened session has no events file.
// Claude Code stacks `--settings` over the worktree's own settings, so every other setting stays the worktree's.
// It carries `--dangerously-skip-permissions` on the fresh and the resume line alike;
// only a permission mode of `prompt` leaves it out, and an unknown mode refuses the open.
// Its resume line falls back to a fresh start under the same id:
// Claude Code keeps no transcript for a session that never took a turn.
//
// The session scope (scope.go) decides what a session loads besides lym's own text.
// It reads `claude.yaml`'s `session_scope` block.
// With `enabled` on, Prepare writes it into every run's settings and OpenLaunch into an opened session's in a hub;
// Claude Code's `--settings` layer outranks the user's and the repository's settings, so the scope holds.
// A plugin is on exactly when the session preloads a skill of it; a plugin that is on lists all of its skills.
// That is because Claude Code honours `skillOverrides` for a plugin's skill only through its claude.ai skill sync.
// A session whose strand role is the orch's alone keeps its auto-memory.
// The scope denies no tool, since every agent keeps the same tool set.
// A config directory or plugins file the engine cannot read is logged and names no plugin.
// An opened session outside a hub writes no settings and gets no scope.
//
// A replay corpus of real turn ends lives under testdata/corpus, one directory per case:
// transcript.jsonl, events.jsonl and case.yaml, which gives the reading each turn end must get.
// corpus_test.go replays every case directory through ParseEvents and shuttle's wait loop under an injected clock,
// so a new case joins without a code change.
//
// UserSetupSource resolves what a Claude session loads from the user's Claude setup.
// It serves another provider's session through shuttleengine's seam.
// It returns the user's global CLAUDE.md and each CLAUDE.md in a directory above the worktree root.
// The files come outermost first, with `@` imports expanded.
// It also returns a listing of the skills the session's scope turns on.
// A CLAUDE.md inside the root is left to the provider, which reads the worktree's own files.
// An unreadable global file is the one error; any other unreadable part is left out with a warning.
// The source also runs the `SessionStart` command hooks of each plugin the scope turns on, as Claude Code runs them.
// A hook that errors or times out is left out and listed as omitted, and a nil runner lists every hook as not run.
// RunPluginHook is the runner, bounded by a timeout, and registers no run process record.
// Bound: a hook left by a Prepare killed mid-run runs to its own end with no owner to end it.
//
// Prepare writes the launch manifest into the run directory once the launch line is built.
// Each item is a candidate whose delivery puts a text or hook command into an artifact:
// the settings document, the appended system prompt, the launch line or the anchor's marked CLAUDE.local.md.
// The manifest keeps a candidate only when the artifact as written holds it, so a dropped delivery drops its item.
// The instruction files, listed skills and session-start hooks the provider loads itself are listed as native.
package claudeengine
