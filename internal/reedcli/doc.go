// Package reedcli is the reed CLI, the cobra layer over the tmux window manager.
// The domain lives in `internal/reedengine`'s package doc.
//
// The per-hub watchdog daemon (`lym reed watchdog`) discovers the hub's tmux sessions and repairs their names.
// It retires when its own `lym` binary is replaced, so a deploy never leaves an old daemon running stale code.
// Every clean exit spawns a successor only when a session is still listed, so none is left without a watchdog.
//
// In a task pair, `down` first calls `pairteardown.Teardown.EndBeforeSessionDown`, then kills the session.
// The pre-step reads local state only and never waits at the network gate.
// A run that is running and unpaused is refused.
// The way forward is `lym tender pause <slug>`, then `lym reed down` again.
// The session stays live in each refusal.
// The envelope carries `ended_processes`, the `{pid, role}` of each process ended.
// `DownWorktree` runs the same `down` for a worktree its caller names, with no envelope.
//
// `attach` and `up` boot a session.
// So the pre-run refuses them in fabric's own checkouts through `fabricengine.RequireDrivableWorktree`.
// Every other verb still runs there.
// `attach` execs tmux over the lym process on Unix, so a shortcut that runs it runs it as its last argv.
//
// `window`, `follow` and `button` are run by launchers and tmux bindings, not by hand.
// `follow` and `button` read no working directory, config or state and skip the root pre-run's stencil-seed pass.
// A failed `button` exits 0 and shows its message on the clicking client.
package reedcli
