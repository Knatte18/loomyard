// Package creelcli builds the `lyx creel` group over internal/creelengine: `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and the hidden `daemon`.
// The design, the protocol and the delivery rules are documented in internal/creelengine; this package wires them to the hub, its worktrees and their agent panes.
//
// Every operator verb resolves the mode from its cwd and refuses a standalone directory, since creel needs a hub.
// The first verb of a hub starts its daemon detached, and `daemon` is what that spawn runs.
// The spawn appends the daemon's stderr to the hub's daemon log, truncating a log over one mebibyte first, and leaves it null when the log cannot be opened; a client that gives up connecting quotes the started daemon's lines from that log.
// `read` acknowledges its messages only once its envelope was written, so output that cannot be written leaves them unread.
// The hub delivery in this package implements the engine's delivery seam over the hub's worktrees, reed's persisted state and shuttle.
package creelcli
