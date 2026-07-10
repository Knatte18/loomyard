// doc.go carries the package-level godoc comment for muxengine. It holds no
// code — its only job is documenting the package's role and contract in one
// place a reader finds first.

// Package muxengine is the domain kernel for lyx's psmux window manager: the
// psmux subprocess overlay, strand bookkeeping, persisted state, config, and
// (in the operations layer) the lifecycle verbs that compose them. It is the
// "dumb carrier" for its caller's strand data — muxengine stores every field
// a caller writes into a strand and reads none of them semantically. There is
// deliberately no domain `type` field on a strand: `cmd`/`resumeCmd` are
// opaque strings muxengine never parses or branches on, and `--role`/`--round`
// are formatting-only inputs consumed once, at add-time, to fill the
// strand-name template — they are never persisted or read back.
//
// muxengine imports internal/muxengine/render (the pure display-vocabulary
// leaf) and maps its own persisted records down to render.Strand when
// computing a layout; render never imports muxengine, so the import graph
// stays acyclic (muxcli -> muxengine -> render).
//
// One additional invariant this package enforces: exactly one named psmux
// server per hub. The server name is derived deterministically from the hub
// path (ServerName), so every worktree under the same hub locates and shares
// the same psmux server rather than each spawning its own.
//
// # Multiplexer contract surface
//
// This package assumes its configured binary (psmux on Windows today, tmux
// on Linux in the deferred follow-up) honors the tmux-derived wire contract
// documented here. contract_integration_test.go's TestMultiplexerContract
// exercises this surface against a real, running instance of that binary —
// the canary for both version drift in the on-box binary and the eventual
// tmux swap, since the same test runs unmodified against whichever binary
// LoadConfig resolves.
//
// Pane enumeration: listPanes (overlay.go) always runs
//
//	list-panes -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"
//
// and parsePaneList (parse.go) parses each output line's six
// whitespace-separated fields positionally, in that exact order, into a
// LivePane. #{pane_dead} is reported as the string "1" or "0";
// parsePaneList keys a dead pane on the literal value "1", never a numeric
// or boolean comparison.
//
// Subcommand set: the engine's correctness depends on new-session,
// has-session, split-window, select-layout, select-pane, send-keys,
// capture-pane, list-panes, list-sessions, display-message,
// set-option -g remain-on-exit, kill-pane, kill-session, and kill-server all
// behaving per tmux's own documented semantics for each.
//
// Load-bearing behavioral assumptions, each with the rationale that makes it
// load-bearing:
//
//   - Silent split failure (spawn.go): split-window against a pane too
//     small to split exits 0, creates no new pane, and prints an EXISTING
//     pane's id on stdout rather than erroring — so launchStrandLocked must
//     verify a split's returned pane id was absent from the pre-split live
//     set before trusting it as genuinely new.
//   - Dead-pane adoption via remain-on-exit (spawn.go): with
//     "set-option -g remain-on-exit on" set at boot, a pane whose command
//     exits stays enumerable (pane_dead=1) instead of vanishing — the only
//     signal that lets reconcile distinguish "the strand's process died"
//     from "the pane is simply gone" — and planPaneTarget must never adopt
//     such a corpse, since send-keys into a dead pane exits 0 while running
//     nothing, silently swallowing the strand's command.
//   - The -l leading-dash send-keys bug (spawn.go): send-keys -l parses a
//     '-'-prefixed literal argument as flags and silently drops it (a "--"
//     separator does not stop this parsing), so sendKeysLiteralArg prefixes
//     a single space onto any opaque cmd/resumeCmd beginning with '-' before
//     it is ever handed to send-keys.
//   - Empty-layout session destruction (apply.go): select-layout accepts a
//     layout string that enumerates zero panes (exit 0) and answers it by
//     destroying every pane in the session, wedging it into a zero-pane
//     zombie that no later add can host a strand in — anyPlacedStrand
//     refuses to call select-layout at all when no strand would place a
//     present pane.
//   - Async kill-server / probe-always-exits-0 (lifecycle.go): kill-server
//     returns before the server process (and its "__warm__" helper) have
//     actually released the -L socket, and no probe — list-sessions,
//     kill-server itself — can distinguish "no server" from "server dying
//     asynchronously", since both exit 0 either way — so Down/reap logic
//     waits on the underlying OS process actually exiting rather than
//     trusting any CLI exit code as a death signal.
package muxengine
