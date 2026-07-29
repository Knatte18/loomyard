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
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
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

// finalizeConnection runs the initialize-then-probe-then-kill-on-failure
// sequence every EnsureServer strategy performs once it has *a* connection,
// regardless of how that connection was obtained (a fresh spawn for
// native, a fresh spawn or a reused dial for supervised). Factoring this
// out is what makes the sequence independently unit-testable against the
// package's existing fake-transport harness, without needing a real
// subprocess.
//
// finalizeConnection performs no restart/retry of its own: an initialize
// or probe failure is reported once and the connection is torn down once
// via client.kill() — never a second spawn attempt. Callers that need
// restart-on-failure semantics (none exist for native; supervised decides
// to restart based on its own state-file staleness check, before ever
// calling finalizeConnection) implement that above this function, not
// inside it.
func finalizeConnection(ctx context.Context, client *lspClient, rootURI string, timeout time.Duration) error {
	initCtx, initCancel := context.WithTimeout(ctx, timeout)
	defer initCancel()
	if err := client.initialize(initCtx, rootURI); err != nil {
		client.kill()
		return err
	}

	// A successful initialize handshake alone is not sufficient proof of
	// health (see probe.go's own doc comment) — probe exercises the
	// connection end-to-end before it is handed back to the caller.
	if err := probe(ctx, client, timeout); err != nil {
		client.kill()
		// Wrap (rather than return verbatim) so a probe failure is
		// distinguishable in logs from an initialize failure without
		// needing errors.Is gymnastics; %w still preserves the chain, so
		// errors.Is(err, ErrServerTimeoutSentinel) keeps matching when the
		// underlying cause is a timeout.
		return fmt.Errorf("codeintelengine: probe failed: %w", err)
	}

	return nil
}

// rootURIFor converts targetDir into the file:// rootURI the LSP
// "initialize" request expects, exactly as References builds it today.
// Factored out here so ensureNative and References (batch 7) share exactly
// one implementation of "path to rootURI" rather than duplicating it.
func rootURIFor(targetDir string) (string, error) {
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("codeintelengine: resolve absolute path for %s: %w", targetDir, err)
	}
	return "file://" + absTargetDir, nil
}

// ensureNative implements the native EnsureServer strategy: resolve the
// toolchain-managed gopls binary for entry.PinnedVersion, spawn it as a
// disposable -remote=auto proxy rooted at targetDir, and finalize the
// connection (initialize + probe) before handing it back. On a
// finalizeConnection failure lyx makes no second spawn attempt — it has no
// supervisory authority over the shared daemon under native, so a single
// reported-and-torn-down failure is the correct behavior, not a retry
// loop.
func ensureNative(ctx context.Context, lang string, entry Entry, targetDir string, timeout time.Duration) (*lspClient, error) {
	binPath, err := resolveGoToolchain(ctx, entry.PinnedVersion)
	if err != nil {
		return nil, fmt.Errorf("codeintelengine: resolve go toolchain for %q: %w", lang, err)
	}

	// entry.Command[0] (the literal string "gopls") is never used here —
	// only entry.Command[1:] (empty for Go's current registry entry, but
	// preserved for forward compatibility with a future entry that carries
	// extra fixed args) is kept, per toolchain-manager-authority's exact
	// argv-composition decision. The toolchain-resolved binPath, not
	// whatever "gopls" resolves to on $PATH, is what gets launched.
	argv := append([]string{binPath}, entry.Command[1:]...)
	argv = append(argv, "-remote=auto")

	client, err := newLSPClient(argv)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ErrServerNotFound{Language: lang, InstallHint: entry.InstallHint}
		}
		return nil, fmt.Errorf("codeintelengine: start language server for %q: %w", lang, err)
	}

	rootURI, err := rootURIFor(targetDir)
	if err != nil {
		client.kill()
		return nil, err
	}

	// finalizeConnection already kills client on its own failure path — a
	// second kill() here would be idempotent but pointless noise.
	if err := finalizeConnection(ctx, client, rootURI, timeout); err != nil {
		return nil, err
	}

	return client, nil
}
