# PATTERN-reed-panes

An agent is spawned as an interactive tmux session through reed, never headless;
text reaches its pane only through guarded paths;
the wait mark is display only.

## Interactive spawn

Rule: Spawning an agent:
an interactive tmux session driven through reed, never headless `claude` with `-p` or `--print`. (test)

Every agent lym spawns runs as an interactive session in a tmux pane, placed by reed and driven by shuttle.
None runs as a headless `claude -p` or `claude --print` process.
Interactive sessions keep subscription coverage, while headless use is moving to API billing;
the reason is economic rather than technical.

- The scan in `cmd/lym/interactivespawn_test.go` parses every production Go file of the module, `tools/` included.
  It reports these shapes:
  a call that takes a `-p`, `--print` or `--print=…` string literal and another argument naming claude.
  That argument is a string literal whose base name is `claude` or `claude.exe`.
  It may also be an identifier, selector or call whose name holds `claude` in any case;
  a string literal whose whitespace tokens, quote characters trimmed, name claude and then carry a print flag;
  and any string literal under `internal/shuttleengine/claudeengine` carrying a print-flag token.
  That engine builds claude's launch lines from flag fragments.
- Comments are never read, so documentation naming the forbidden form passes.
- The scan's allowlist is keyed by file and admits every finding in that file.
  Each entry carries a reason naming why the flag there launches no agent.
- Bound: the scan sees static shapes only,
  so a flag built at runtime, or a launch line assembled from fragments outside the claude engine's directory, passes.
- Bound: test files are not scanned, so a test may hold the forbidden text as data.
- An agent session's own shell is guarded too:
  `internal/shellguard`'s headless-claude row refuses a Bash launch of claude with a print flag in every run,
  and its steer names a subagent through the Agent tool as the way to hand work off.
  A subagent runs inside the session and shows in its pane.

## Pane text injection

Rule: Typing into an agent pane:
reed's guarded paste-submit;
an interrupt key only via shuttle's guards, the guarded picker close and dismissal included;
a covered session held; the orch pane and creel journal only via creel. (test)

Text lym types into an agent pane never merges with, submits or wipes an operator's draft.
Its picker dismissal holds while an operator was active in the pane within `operator_quiet_s`.
That protects an operator's open menu.
The bounded exception is a lone stray character in an idle box:
the guard clears it once per send attempt, behind a full guard reading.
A character that survives is submitted at the head of the message.

- Text reaches a pane only through reed's paste-submit:
  one tmux invocation pastes the text.
  When the provider's Enter submits a paste, the same command list sends the Enter.
  No caller reintroduces `send-keys -l` followed by a blind Enter.
  The gap between the two is the window an operator's keystrokes merge into.
- Shuttle's pre-write guard stands in front of every such write.
  One fresh reading takes these facts:
  the session is ready;
  no operator was active in the pane within `operator_quiet_s`;
  and the input box is empty and readable.
  After a clear of a lone stray character, the reading taken after the clear is the write's guard reading.
  It takes the same facts.
  A write the guard holds fails wrapping `ErrSessionBusy` and writes nothing, and the caller retries later.
  The provider's box reading defines empty.
  Claude's reads a lone `%`, the shell's end-of-line mark left on screen, as empty.
  So an operator draft of exactly `%` typed outside `operator_quiet_s` reads as empty, and a write can merge with it.
  Claude's box reading goes through a styled capture and takes a box holding only placeholder-styled text as empty.
  That holds only when every non-blank cell is drawn in the placeholder style.
  The caret, the side bars and a lone `%` are stripped first.
  A typed draft, drawn unstyled, still reads as a draft, and so does a restyled placeholder, which fails closed.
  Shuttle adds the residue reading, taken only after the clear, beside the provider's box reading:
  a box that still holds exactly the cleared character reads as empty, on the hook reading and the pane reading alike.
- The watchdog's `/rename` name repair has no shuttle run;
  it stands behind reed's own activity check and goes out as the same paste-submit.
- lym sends its own Enter only into a box that holds exactly the sent text or the provider's placeholder for it.
  A box holding anything else, the sent text beside a draft included, gets no key.
- A send that may leave its text in the box records `{text, events offset}` in the run directory's `pending-send.json`.
  The next send to the strand reads it:
  the same text with a turn start past the offset went out already and types nothing;
  otherwise the send types as usual.
- A send to a strand with no run record takes the pane-only readiness reading and keeps no pending-send record.
  It never clears its unlanded text, so the next send holds on the non-empty box.
- `Runner.InterruptRestoring` is a second bounded clear.
  It empties the prompt Claude restores into the box after lym's own interrupt key.
  It fires only on text that appeared in a box read empty before the key, at most once per call.
  It never fires without a passing guard reading: the pane ready, no running turn, a readable box and no active operator.
  Its keys are the provider's restored-prompt clear, no `Escape`, `C-c` or Enter, at most two per line the read showed.
  The text is the stopped turn's own prompt, already in the transcript.
  A guard reading that fails sends nothing, and the text stays as a draft the next send holds on.
- Explicit key deliveries stay outside the rule:
  `Runner.Interrupt`, `Runner.InterruptRestoring`, `Runner.Inject` and `InterruptSequence`;
  the trust-prompt dismissal;
  the clear sent into a box holding exactly one stray-character candidate the guard reading found;
  the restored-prompt clear; and the guarded picker dismissal.
  Each names its keys itself and is not text for a draft box.
  `Runner.Interrupt` and `Runner.Inject` stay outside this rule as explicit key deliveries,
  but their interrupt key follows the guarded interrupt below.

### Guarded interrupt

An interrupt key reaches an agent pane only through shuttle's guarded interrupt and its two guarded picker paths below.
Every other path refuses it.

- The guarded interrupt (`Runner.Interrupt`, `Run.Interrupt`) sends the provider's interrupt sequence.
  It sends only after the ready-agent-pane check and a fresh capture the provider reads as a running turn.
  A pane with no running turn gets nothing and `ErrNoRunningTurn`, and a capture that fails sends nothing.
- A retrying caller waits at least the provider's settle time (`Runner.InterruptSettle`) after its first key.
  It reads again only after the wait, and sends the retry through the guarded interrupt.
  So lym's own keys land at least the settle apart and never open the rewind picker.
- The guarded picker close is one of two exceptions.
  Before every interrupt and every verified send it sends exactly one key of the interrupt sequence.
  It sends it only into a capture the provider reads as the rewind picker.
  It then re-captures after the settle and never selects an entry or sends a second key.
  It fails the operation with `ErrRewindPickerOpen`.
- The guarded picker dismissal is the other exception, in a verified send's idle wait:
  the engine names the keys; for Claude's autocompact picker they are one `Escape` that keeps the current setting.
  It fires only into a capture the provider reads as that known picker with no input box.
  It also needs the provider's running-turn check to read no running turn.
  It fires only when no operator was active in the pane within `operator_quiet_s`.
  It plays at most once per send attempt, then waits the provider's interrupt settle and reads the pane again.
  It never selects an entry.
- `Runner.Inject` skips the ready-pane check.
  It therefore refuses any input list holding a key of the engine's interrupt sequence.
  The key match is case-insensitive and reads `C-[` as `Escape`.
  It also refuses a `Text` with a control character.
  Both refusals hold whatever the entry's `SettleMS` and neighbours, and name `lym creel interrupt`.
  A tmux spelling of the interrupt key beyond those still passes, bounded by Inject having no production caller.
- The orch watcher's hub-pause interrupt is the one key the watcher sends:
  it reaches the orch pane through the guarded interrupt, under the exemption the orch pane section below keeps for it.
- `Runner.InterruptRestoring` is a second entry point:
  its key goes only through the guarded interrupt;
  its clear is the engine's restored-prompt clear of the box, never an interrupt key.
- The tests that pin it live in `internal/shuttleengine`:
  `TestRun_Interrupt_PlaysEscape`, `TestRunner_InterruptRestoring`, `TestGuardedPickerClose`;
  `TestGate_RepromptOverPicker`, `TestDismissPicker_Guards` and `TestRunner_Inject_RefusesAnyInterruptKey`.

### A pause holds prompts

Typing a line into a session a pause covers:
shuttle's send refuses with `ErrPaused` by the target's own slug, read after the idle wait.
A conditioned send's caller reads coverage itself.
Of the production runners that type a line, only the orch runner is told no pause geometry and reads uncovered.
So `BeamUp`'s start lines are never held.
The caller holds the line until the resume or claims the session.

- The refusal is a `*PausedError` wrapping `ErrPaused`, a sentinel of its own that never wraps `ErrSessionBusy`;
  it carries the judged slug and the covering entry, so a caller waits on the slug the refusal judged.
- A refused send types nothing, records nothing and never waits.
  Keys stay outside it: an interrupt, an inject and the pause's own Esc are key deliveries under the rule above.
- The tender and creel runners are told the hub's pause geometry with an empty slug,
  so each send judges its target by the target's own strand name;
  a prime name reads the empty slug, which only a hub entry covers.

#### What each caller does with `ErrPaused`

- The gate loop holds its boundary and its re-prompt.
- The run start and the deferred prompt wait out the pause and resend.
- The seat notice sender retries.
- Creel holds the inject and its queued commands.
- Tender resume claims the session into the refusing pause.
- Tender revive's conditioned send reports the refusal as a send error.
- The start's segment-color line is logged and dropped.
- `lym loom resume` refuses with the pause's way forward.

#### Enforcement

Review discipline and the shuttle runner's send tests; no scan enforces it.

### An API error holds the prompt

Rule: A start's own prompt line is not typed behind a skill-load turn the API failed:
the load runs again after a back-off until a load turn ends. (test)

A start's prompt line is not typed behind a skill-load turn the provider's API failed:
shuttle runs the load again after a back-off until a load turn ends, and only then types the prompt.

- A skipped skill stays what the skill's own state says: not installed, or still missing after the retry.
  An API error says nothing about the skill.
- The typed load line is the probe of the API, and goes through the same verified send, so a pause holds it too.
- Bound: an attach delivers a pending prompt without judging the API.

#### Enforcement

`TestStartup_LoadTurnAPIError` in `internal/shuttleengine`.

### The orch pane

Only creel types into the orch session from Go, through its `Delivery` seam.

- The orch watcher keeps only a hub pause's guarded interrupt.
- Reed's `/rename` repair keeps its own guard.
- Every other module posts a notice or injects text through creel.

### The creel store

Only the creel daemon in `internal/creelengine` writes the creel journal.
That engine reaches an agent's pane only through its `Delivery` seam.

#### Journal

- The journal writer is unexported in `internal/creelengine`, and the daemon's `Run` is its only caller.
- Checked by review.

#### Panes

- Every keystroke goes through the `Delivery` seam, which `internal/creelcli` implements over shuttle.
- `internal/creelengine` imports neither `internal/reedengine` nor `internal/shuttleengine` or any package under it.
  `TestCreelStore_DeliverySeamImports` checks it.
- `internal/creelengine` names no tmux command, checked by review like the journal writer.

#### Layering

Creel decides what is delivered and when, shuttle knows the provider, and reed does the keystrokes.
Reed never imports creel.

## Wait mark display only

Rule: Marking a pane waiting: only reed's `SetWaitMark`; no Go decision reads `@lym_wait` or `@lym_wait_start`.

Held by review alone; no test enforces it.
