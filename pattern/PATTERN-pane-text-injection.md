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
- Explicit key deliveries stay outside the rule: `Runner.Interrupt`, `Runner.Inject`, `InterruptSequence`, the trust-prompt dismissal and the clear sent into a box holding exactly one stray-character candidate the guard reading found.
  Each names its keys itself and is not text for a draft box.
  `Runner.Interrupt` and `Runner.Inject` stay outside this rule as explicit key deliveries, but their interrupt key follows `PATTERN-guarded-interrupt`.
