// Package creelcli builds the `lyx creel` group over internal/creelengine: `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon`.
// The design, the protocol and the delivery rules are documented in internal/creelengine; this package wires them to the hub, its worktrees and their agent panes.
//
// Every operator verb resolves the mode from its cwd and refuses a standalone directory, since creel needs a hub.
// The first verb of a hub starts its daemon detached, and `daemon` is what that spawn runs.
// The spawn appends the daemon's stderr to the hub's daemon log, truncating a log over one mebibyte first, and leaves it null when the log cannot be opened.
// The spawn reaps the detached daemon in the background, logs its exit with its pid and status, and reads exit status 0 as a lost start race; the client's connect wait ends on any other exit.
// A client that gives up connecting quotes the started daemon's lines from that log.
// The daemon writes its start header and its pid-tagged warnings to its stderr, which that spawn points at the daemon log, so a start failure reaches the file.
// `read` acknowledges its messages only once its envelope was written, so output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code, on one line that opens with `creel: ` once, whether or not the error already carried the prefix.
// The hub delivery in this package implements the engine's delivery seam over the hub's worktrees, reed's persisted state and shuttle.
package creelcli
