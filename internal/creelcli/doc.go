// Package creelcli builds the `lym creel` group over internal/creelengine:
// `post`, `inject`, `interrupt`, `forward`, `read`, `inbox`, `log`, `hold`, `release` and `hubs`.
// The hidden verbs are `daemon`, `hook-read` and `hook-wait`.
// The design, the protocol and the delivery rules are documented in internal/creelengine;
// this package wires them to the hub, its worktrees and their agent panes.
//
// Every operator verb resolves the mode from its cwd and refuses a standalone directory, since creel needs a hub.
// Every creel command skips the root pre-run's stencil-seed pass, so a verb's stderr carries only creel's own lines.
// The first verb of a hub starts its daemon detached, and `daemon` is what that spawn runs.
//
// `read` prints plain text by default and one JSON envelope line with `--envelope`, for programs.
// The flag is not named `--json`, which would shadow the root's global `--json` help flag.
// `read` acknowledges its messages only once its output was written; output that cannot be written leaves them unread.
// A refusal whose JSON envelope cannot be written to stdout writes the error to stderr with the same exit code.
// A success envelope that cannot be written exits 1.
// Every refusal is one `creel: ` line on stderr.
//
// `post` stores the message even when the receiver is not live, and prints one `creel: warning: ` line.
// A `post --wait-reply` whose message creel recorded unanswered prints its envelope with `outcome: unanswered`.
// It then exits 1 with a note whose way forward is `lym creel log --id <id>`.
// `post --interrupt` stops the receiver's running turn before posting; with `--reply` it is refused before any call.
// A receiver a pause covers is claimed into the pause and interrupted;
// an unreadable pause record and another sender's hold refuse it with nothing interrupted or posted.
// `post --expect-reply` and `--wait-reply` mark the post as expecting a reply; with `--reply` they are refused.
// A receiver of `post` may be a strand of another hub by its full name.
// `post --interrupt`, `inject`, `interrupt`, `hold` and `release` refuse such a name, naming `lym creel post`.
// `forward` hands a message from another hub only to a strand of this hub.
// `post --parent <text>` takes no receiver and posts to the caller's parent orch.
// The orch is resolved at post time from the caller's worktree;
// it is refused with a receiver argument or with `--reply`, and when the resolved orch records no strand.
// `inbox` prints the caller's unread counts as `inbox.senders`, a list of `{from, unread}`, and no other key.
//
// `hook-read <engine>` is what a strand's tool-boundary and Stop hooks run.
// It claims the caller's mail through the daemon and prints it as the engine's hook output for the input's event.
// The mail goes to the model as additional context; it is acknowledged only once its output was written.
// A call from a subagent or fork claims and prints nothing, so the mail stays for the main thread's next boundary.
// Stdout stays empty on every refusal, because the provider parses the hook's stdout as its JSON.
// After a tool call it exits 0.
// On a Stop it exits 0 exactly when it wrote mail, and 1 otherwise, an echo alone included.
//
// `hook-wait <engine>` is the idle waiter a strand's hook holds open; the hook entry's timeout bounds it.
// It writes the claimed mail's lines to stderr as the engine's wake output and exits with the engine's wake code.
// It exits 0 when no lines were written, including a superseded wait, an unreachable daemon and any refusal.
// Any other end after the lines were written exits with the wake code, a refused or lost confirmation included.
// A write of the lines that fails leaves the mail unread and the ring unanswered.
//
// `hubs` prints `{"hubs":[{shortname, hub_path, repository, daemon_answers, trusted}]}`.
// The list comes from the machine record in the per-user state directory.
// It starts no daemon and writes no record.
// A hub with no GitHub repository carries `no_repository` in its place and is never trusted.
// The `trusted_hubs` list of creel.yaml is read at each call, so a changed list applies with no daemon restart.
// A corrupt record, an invalid creel.yaml and an unreadable per-user state directory each refuse with a way forward.
//
// GoClient is the one Go-side caller of creel, so no other module builds a creel request by hand.
// NewProgramClient signs as `<shortname>:<PROGRAM>` and claims no strand.
// NewStrandClient claims a strand and no program; the daemon's identity rules still decide who the caller is.
// A strand claim from a process in no strand pane is refused with `creelengine.ErrStrandClaimOutsidePane`.
// ReadWait acknowledges the mail only when the handler returns nil.
// A connection lost before or after the handler ran sends the read again,
// so the handler can run a second time for mail it already saw.
//
// The hub delivery implements the engine's delivery seam.
// Shuttle's busy and not-confirmed answers map onto the engine's ErrBusy and ErrNotLanded.
// ErrBusy means nothing was typed, so a requeue types the line again.
// Bound: an inject left beside a draft is not retyped; a foreign-text or recordless one is lost rather than typed twice.
// The strand listing skips a worktree that no longer resolves, so its strands get no delivery until it does.
package creelcli
