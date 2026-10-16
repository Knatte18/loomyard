// Package reedcli is the reed CLI, the cobra layer over the tmux window manager whose domain lives in `internal/reedengine`'s package doc.
//
// The per-hub watchdog daemon (`lyx reed watchdog`) runs a discovery loop over the hub's tmux socket.
// Its cadence starts at five seconds, doubles up to sixty across cycles that find the session set unchanged, and returns to five seconds when the set changes.
// The loop also wakes on the hub-level discover signal, a file that a cold session boot touches: a consumed signal runs a cycle at once, but never sooner than five seconds after the previous cycle, so a process touching the file in a loop costs one cycle per five seconds.
// When the signal file cannot be watched the loop logs that once and keeps its backed-off cadence.
// Idle exit and orphan reap each count three consecutive cycles, so at the backed-off cadence they take up to three minutes.
// A signal file holding the stop marker is a stop request, written by the `down` that ends the hub's server: a stop-woken cycle that lists no session exits at once, never sooner than one second after the previous cycle.
// After any clean exit the daemon releases its lock and lists the hub's sessions once more, and spawns a successor only when that listing finds a session, so a session that appeared during the exit is not left without a watchdog.
//
// `add` places the strand in a window of its own, or its group's, and its `--anchor` takes `own-window` (the default) or `hidden`; any other value is refused.
//
// `attach` execs tmux over the lyx process on Unix, so no lyx process outlives the handover, and keeps a child process on Windows.
// Nothing runs after it in the process, so a shortcut that runs `reed attach` runs it as its last argv.
//
// `follow` turns follow on for a session told by `--socket`, `--session` and `--tmux`, and selects the session's relevant window from tmux facts alone.
// It reads no working directory, config or state, skips the root pre-run's stencil-seed pass, and prints nothing on success; the status bar's follow button runs it through run-shell.
// A missing flag or a failed tmux call is an error envelope with a non-zero exit.
//
// Each watched session's engine drives the daemon's told tmux binary, whatever that worktree's config names.
//
// On each cycle the daemon stats the `lyx` binary it started from, and when that file has been replaced or rewritten it retires: it ends its watched sessions, releases the hub lock, spawns a successor detached only when that cycle's listing found a session, and exits 0.
// So a deploy never leaves an old daemon repairing names with stale code.
// A failed stat counts as unchanged, so a binary missing mid-build retires nothing, and a deploy that rewrites the file in several steps can chain retirements, one per observed change.
// A daemon started before this check existed carries none and is ended once by hand after the deploy; the next `reed up`, `attach` or `resume` spawns a successor that has it.
// A spawn that loses the lock race exits 0 and logs at Debug, so at the default verbosity it writes no trace file.
//
// The daemon logs to the durable sink in the hub logs directory, and Debug lines reach it at Debug verbosity.
// A Debug trace comes from running `lyx reed watchdog -vv` in the foreground, or from `lyx reed up`, `attach` or `resume` run with `LYX_LOG_LEVEL=debug` while no daemon runs for the hub.
package reedcli
