// Package reedcli is the reed CLI, the cobra layer over the tmux window manager.
// The domain lives in `internal/reedengine`'s package doc.
//
// The per-hub watchdog daemon (`lym reed watchdog`) runs a discovery loop over the hub's tmux socket.
// Its cadence starts at five seconds and doubles up to sixty across cycles that find the session set unchanged;
// it returns to five seconds when the set changes.
// The loop also wakes on the hub-level discover signal, a file that a cold session boot touches:
// a consumed signal runs a cycle at once, but never sooner than five seconds after the previous cycle,
// so a process touching the file in a loop costs one cycle per five seconds.
// If the signal file cannot be watched, as when the inotify instance table is full, the loop logs that once.
// It then keeps its backed-off cadence and reads the file itself on each timer wake;
// a stop request still lands, at that cadence.
// Idle exit and orphan reap each count three consecutive cycles.
// At the backed-off cadence they take up to three minutes.
// A signal file holding the stop marker is a stop request, written by the `down` that ends the hub's server:
// a stop-woken cycle that lists no session exits at once, never sooner than one second after the previous cycle.
// After any clean exit the daemon releases its lock and lists the hub's sessions once more.
// It spawns a successor only when that listing finds a session,
// so a session that appeared during the exit is not left without a watchdog.
// After `down` ends the hub's server it polls the daemon's lock for up to ten seconds,
// so the daemon has left its loop when the verb returns;
// a lock still held at the bound logs one Warn and `down` still succeeds.
//
// In a task pair, `down` first calls `pairteardown.Teardown.EndBeforeSessionDown`.
// That ends the pair's registered processes.
// It then kills the session.
// The envelope carries `ended_processes`, the `{pid, role}` of each process ended, empty when none.
// The pre-step reads local state only and makes no network call,
// so it never waits at the network gate and runs no origin reconcile.
// It proceeds when a pause covers the pair, its board entry reads done, or the run is not running;
// it also proceeds when it finished its Finalize with no registered batten alive.
// It then waits for the run lock before it ends the processes.
// A run that is running and unpaused is refused, with the envelope's error giving the way forward:
// `lym tender pause <slug>`, then `lym reed down` again.
// The session stays live in each refusal.
// A refusal of the run-lock wait, or survivors of the end, are the envelope's error the same way.
// The prime is no pair: its `down` ends no registered process.
//
// `add` places the strand in a window of its own, or its group's.
// Its `--anchor` takes `own-window` (the default) or `hidden`; any other value is refused.
//
// `attach` execs tmux over the lym process on Unix, so no lym process outlives the handover;
// on Windows it keeps a child process.
// Nothing runs after it in the process, so a shortcut that runs `reed attach` runs it as its last argv.
//
// `window <worktree-dir> [--title <title>]` opens one desktop terminal window through `internal/termwindow`.
// Its shell runs `lym reed attach` in that directory, and the verb returns once the launcher process has started.
// With no `--title` it titles the window with the console title of the worktree holding the directory.
// That title is read from the hub's board.
// It reads no acting location, config or engine; a directory that does not exist is refused before any launcher runs;
// a machine with no launcher reports why on the envelope.
//
// `follow` turns follow on for a session told by `--socket`, `--session` and `--tmux`.
// It selects the session's relevant window from tmux facts alone.
// It reads no working directory, config or state, and it skips the root pre-run's stencil-seed pass.
// It prints nothing on success; the status bar's follow button runs it through run-shell.
// A missing flag or a failed tmux call is an error envelope with a non-zero exit.
//
// `button` runs the status-bar button that `--range` names, for the client `--client`.
// It acts in the session told by `--session`, on the server told by `--socket` and `--tmux`.
// It reads no working directory, config or state.
// It skips the root pre-run's stencil-seed pass and the reed pre-run's location and config resolution.
// It prints nothing; the click binding and a button's ask and confirm prompts run it through run-shell.
// It runs the entry in the anchor recorded now for the worktree holding the session's rendered run directory.
// The anchor is read through `lymcwd.ResolveWorktree` with no cwd gate,
// so an anchor marker changed after the render needs no reattach.
// A missing required flag, or `--confirmed` or `--answered` without `--hash`, is an error envelope with a non-zero exit;
// every other outcome, a failed button included, exits 0 and shows its message on the clicking client.
// `--confirmed`, `--answered`, `--hash`, `--strand` and `--answer` are passed by the prompts' own lines;
// they are not meant for a hand call.
//
// Each watched session's engine drives the daemon's told tmux binary, whatever that worktree's config names.
//
// On each cycle the daemon stats the `lym` binary it started from.
// When that file has been replaced or rewritten it retires:
// it ends its watched sessions and releases the hub lock;
// it spawns a successor detached only when that cycle's listing found a session, and it exits 0.
// So a deploy never leaves an old daemon repairing names with stale code.
// A failed stat counts as unchanged, so a binary missing mid-build retires nothing,
// and a deploy that rewrites the file in several steps can chain retirements, one per observed change.
// A spawn that loses the lock race exits 0 and logs at Debug, so at the default verbosity it writes no trace file.
//
// The daemon logs to the durable sink in the hub logs directory, and Debug lines reach it at Debug verbosity.
// A Debug trace comes from running `lym reed watchdog -vv` in the foreground.
// It also comes from `lym reed up`, `attach` or `resume` run with `LYM_LOG_LEVEL=debug`.
// That holds only while no daemon runs for the hub.
package reedcli
