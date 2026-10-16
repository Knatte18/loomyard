# PATTERN-guarded-interrupt

An interrupt key reaches an agent pane only through shuttle's two pane guards, and every other path refuses it.

- The guarded interrupt (`Runner.Interrupt`, `Run.Interrupt`) sends the provider's interrupt sequence only after the ready-agent-pane check and a fresh capture the provider reads as a running turn.
  A pane with no running turn gets nothing and `ErrNoRunningTurn`, and a capture that fails sends nothing.
- A caller that retries an interrupt waits at least the provider's settle time (`Runner.InterruptSettle`) after its first key before it reads again, and sends the retry through the guarded interrupt.
  So lyx's own keys land at least the settle apart and never open the rewind picker.
- The guarded picker close is the one exception: before every interrupt and every verified send it sends exactly one key of the interrupt sequence, only into a capture the provider reads as the rewind picker.
  It then re-captures after the settle, never selects an entry, never sends a second key, and fails the operation with `ErrRewindPickerOpen`.
- `Runner.Inject` skips the ready-pane check, so it refuses any input list holding a key of the engine's interrupt sequence (case-insensitive, `C-[` read as `Escape`) or a `Text` with a control character, whatever its `SettleMS` and neighbours, and names `lyx shuttle interrupt`.
  A tmux spelling of the interrupt key beyond those still passes, bounded by Inject having no production caller.
- The orch watcher's hub-pause interrupt reaches the orch pane through the guarded interrupt and keeps the single-writer exemption (`PATTERN-orch-pane-single-writer`).
- `Runner.InterruptRestoring` is a second entry point: its key goes only through the guarded interrupt, and its clear is the engine's restored-prompt clear of the box, never an interrupt key.
- The tests that pin it: `TestRun_Interrupt_PlaysEscape`, `TestRunner_InterruptRestoring`, `TestGuardedPickerClose` and `TestRunner_Inject_RefusesAnyInterruptKey` in `internal/shuttleengine`.
