# PATTERN-pane-text-injection

Text lyx types into an agent pane never merges with, submits or wipes an operator's draft, and never closes a menu the operator has open.
The bounded exception is a lone stray character in an idle box: the guard clears it once per send attempt, behind a full guard reading, and a character that survives is submitted at the head of the message.

- Text reaches a pane only through reed's paste-submit: one tmux invocation that pastes the text and, when the provider's Enter submits a paste, sends the Enter in the same command list.
  No caller reintroduces `send-keys -l` followed by a blind Enter, whose gap between the two is the window an operator's keystrokes merge into.
- Shuttle's pre-write guard stands in front of every such write.
  One fresh reading takes three facts: the session is ready, no operator was active in the pane within `operator_quiet_s`, and the input box is empty and readable.
  After a clear of a lone stray character, the reading taken after the clear is the write's guard reading, with the same three facts.
  A write the guard holds fails wrapping `ErrSessionBusy` and writes nothing, and the caller retries later.
  The provider's box reading defines empty, and Claude's reads a lone `%`, the shell's end-of-line mark left on screen, as empty, so an operator draft of exactly `%` typed outside `operator_quiet_s` reads as empty and a write can merge with it.
  Claude's box reading goes through a styled capture and takes a box holding only placeholder-styled text as empty.
  That holds only when every non-blank cell, once the caret, the side bars and a lone `%` are stripped, is drawn in the placeholder style, so a typed draft, drawn unstyled, still reads as a draft, and a restyled placeholder reads as a draft, failing closed.
  Shuttle adds the residue reading, taken only after the clear, beside the provider's box reading: a box that still holds exactly the cleared character reads as empty, on the hook reading and the pane reading alike.
- The watchdog's `/rename` name repair has no shuttle run; it stands behind reed's own activity check and goes out as the same paste-submit.
- An Enter of lyx's own goes only into a box holding exactly the sent text, or the provider's placeholder for exactly that text.
  A box holding anything else, the sent text beside a draft included, gets no key.
- A send that may have left its text in the box records `{text, events offset}` in the run directory's `pending-send.json`.
  The next send to the strand reads it: the same text with a turn start past the offset went out already and types nothing; otherwise the send types as usual.
- A send to a strand with no run record takes the pane-only readiness reading, keeps no pending-send record and never clears its unlanded text, so the next send holds on the non-empty box.
- A second bounded clear sits beside the stray-character one: `Runner.InterruptRestoring` empties the prompt Claude restores into the box after lyx's own interrupt key.
  It fires only on text that appeared in a box read empty before the key, at most once per call and never without a passing guard reading: the pane ready, no running turn, a readable box and no active operator.
  Its keys are the provider's restored-prompt clear, no `Escape`, `C-c` or Enter, at most two per line the read showed, and the text is the stopped turn's own prompt, already in the transcript.
  A guard reading that fails sends nothing, and the text stays as a draft the next send holds on.
- Explicit key deliveries stay outside the rule: `Runner.Interrupt`, `Runner.InterruptRestoring`, `Runner.Inject`, `InterruptSequence`, the trust-prompt dismissal, the clear sent into a box holding exactly one stray-character candidate the guard reading found and the restored-prompt clear.
  Each names its keys itself and is not text for a draft box.
  `Runner.Interrupt` and `Runner.Inject` stay outside this rule as explicit key deliveries, but their interrupt key follows the guarded interrupt below.

## Guarded interrupt

An interrupt key reaches an agent pane only through shuttle's two pane guards, and every other path refuses it.

- The guarded interrupt (`Runner.Interrupt`, `Run.Interrupt`) sends the provider's interrupt sequence only after the ready-agent-pane check and a fresh capture the provider reads as a running turn.
  A pane with no running turn gets nothing and `ErrNoRunningTurn`, and a capture that fails sends nothing.
- A caller that retries an interrupt waits at least the provider's settle time (`Runner.InterruptSettle`) after its first key before it reads again, and sends the retry through the guarded interrupt.
  So lyx's own keys land at least the settle apart and never open the rewind picker.
- The guarded picker close is the one exception: before every interrupt and every verified send it sends exactly one key of the interrupt sequence, only into a capture the provider reads as the rewind picker.
  It then re-captures after the settle, never selects an entry, never sends a second key, and fails the operation with `ErrRewindPickerOpen`.
- `Runner.Inject` skips the ready-pane check, so it refuses any input list holding a key of the engine's interrupt sequence (case-insensitive, `C-[` read as `Escape`) or a `Text` with a control character, whatever its `SettleMS` and neighbours, and names `lyx shuttle interrupt`.
  A tmux spelling of the interrupt key beyond those still passes, bounded by Inject having no production caller.
- The orch watcher's hub-pause interrupt is the one key the watcher still sends: it reaches the orch pane through the guarded interrupt, under the exemption the orch pane section below keeps for it.
- `Runner.InterruptRestoring` is a second entry point: its key goes only through the guarded interrupt, and its clear is the engine's restored-prompt clear of the box, never an interrupt key.
- The tests that pin it: `TestRun_Interrupt_PlaysEscape`, `TestRunner_InterruptRestoring`, `TestGuardedPickerClose` and `TestRunner_Inject_RefusesAnyInterruptKey` in `internal/shuttleengine`.

## A pause holds prompts

Typing a line into a session a pause covers: shuttle's send refuses with `ErrPaused` by the target's own slug, read after the idle wait, and a conditioned send's caller reads coverage itself; of the production runners that type a line, only the orch runner is told no pause geometry and reads uncovered, so `BeamUp`'s start lines are never held; the caller holds it until the resume or claims the session.

- The refusal is a `*PausedError` wrapping `ErrPaused`, a sentinel of its own that never wraps `ErrSessionBusy`; it carries the judged slug and the covering entry, so a caller waits on the slug the refusal judged.
- A refused send types nothing, records nothing and never waits.
  Keys stay outside it: an interrupt, an inject and the pause's own Esc are key deliveries under the rule above.
- The tender and creel runners are told the hub's pause geometry with an empty slug, so each send judges its target by the target's own strand name; a prime name reads the empty slug, which only a hub entry covers.

### What each caller does with `ErrPaused`

- The gate loop holds its boundary and its re-prompt.
- The run start and the deferred prompt wait out the pause and resend.
- The seat notice sender retries.
- Creel holds the inject and its queued commands.
- Tender resume claims the session into the refusing pause.
- Tender revive's conditioned send reports the refusal as a send error.
- The start's segment-color line is logged and dropped.
- `lyx loom resume` and `lyx shuttle send` refuse with the pause's way forward.

### Enforcement

Review discipline and the shuttle send tests; no scan enforces it.

## The orch pane

Only creel types into the orch session from Go, through its `Delivery` seam.

- The orch watcher keeps only a hub pause's guarded interrupt.
- Reed's `/rename` repair keeps its own guard.
- Every other module posts a notice or injects text through creel.

## The creel store

Only the creel daemon in `internal/creelengine` writes the creel journal, and that engine reaches an agent's pane only through its `Delivery` seam.

### Journal

- The journal writer is unexported in `internal/creelengine`, and the daemon's `Run` is its only caller.
- Checked by review.

### Panes

- Every keystroke goes through the `Delivery` seam, which `internal/creelcli` implements over shuttle.
- `internal/creelengine` imports neither `internal/reedengine` nor `internal/shuttleengine` or any package under it, checked by `TestCreelStore_DeliverySeamImports`.
- `internal/creelengine` names no tmux command, checked by review like the journal writer.

### Layering

Creel decides what is delivered and when, shuttle knows the provider, and reed does the keystrokes.
Reed never imports creel.
