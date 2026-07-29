// ensureserver.go implements the EnsureServer(lang, worktreeRoot) -> LSPConn
// seam from manifest/designs/codeintel-redesign.md: given a registry entry
// whose HasNativeDaemon field is true, it resolves, spawns or dials, and
// hands back an already-initialized, already-probed *lspClient ready for
// immediate use. ensureServer is called only for a registry entry with
// HasNativeDaemon == true — in V1 this means Go only. Every other
// language's caller keeps using newLSPClient/client.initialize directly,
// unchanged, and never calls into this file at all.

package codeintelengine

import (
	"context"
	"time"
)

// connKind reports which EnsureServer strategy produced a given *lspClient,
// so the caller knows the correct teardown rule for the connection it got
// back — the two strategies wrap fundamentally different kinds of process
// lifetime and must not be torn down the same way.
type connKind int

const (
	// connKindNative marks a connection from the native strategy: a
	// disposable local -remote=auto proxy subprocess this call spawned
	// itself. It is safe to close()/kill() — doing so ends only this proxy,
	// never the shared daemon behind it.
	connKindNative connKind = iota
	// connKindSupervised marks a connection from the supervised strategy: a
	// dial into a daemon lyx spawned to outlive this call. Never
	// close()/kill() it — the daemon is meant to keep serving other
	// callers, and the graceful-shutdown handshake close() sends would be a
	// needless (and potentially harmful) RPC round trip against it.
	connKindSupervised
)

// ensureServer resolves entry's language server connection and returns it
// already initialized and probed, alongside the connKind the caller needs
// to pick the right teardown. worktreeRoot is accepted but unused by the
// native branch in this batch (native takes no state file/lock) — it
// exists on the signature now so the supervised strategy (which does need
// it) can be added without changing this signature again.
//
// ensureServer has exactly one live dispatch arm in V1: it always calls
// ensureNative, because no V1 registry entry ever requests the supervised
// strategy (every entry that reaches this function has HasNativeDaemon ==
// true, and in V1 that is Go alone, which uses native). ensureSupervised
// exists and is fully tested, but proven by its own dedicated integration
// test, not through this dispatcher — wiring a second arm here would be
// dead code reachable by no test except one written to force it
// artificially.
func ensureServer(ctx context.Context, lang string, entry Entry, targetDir, worktreeRoot string, timeout time.Duration) (*lspClient, connKind, error) {
	client, err := ensureNative(ctx, lang, entry, targetDir, timeout)
	return client, connKindNative, err
}
