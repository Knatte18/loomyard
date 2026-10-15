# PATTERN-creel-store

Only the creel daemon in `internal/creelengine` writes the creel journal, and that engine reaches an agent's pane only through its `Delivery` seam.

## Journal

- The journal writer is unexported in `internal/creelengine`, and the daemon's `Run` is its only caller.
- Checked by review.

## Panes

- Every keystroke goes through the `Delivery` seam, which `internal/creelcli` implements over shuttle.
- `internal/creelengine` imports neither `internal/reedengine` nor `internal/shuttleengine` or any package under it, checked by `TestCreelStore_DeliverySeamImports`.
- `internal/creelengine` names no tmux command, checked by review like the journal writer.

## Layering

Creel decides what is delivered and when, shuttle knows the provider, and reed does the keystrokes.
Reed never imports creel.
