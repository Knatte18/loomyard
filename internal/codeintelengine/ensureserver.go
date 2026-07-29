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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
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
	// connKindLegacy marks the legacy path's kind — never produced by
	// ensureServer itself (that function is only ever called when
	// entry.HasNativeDaemon is true); acquireConnection returns this
	// directly for the false case without calling ensureServer.
	connKindLegacy
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

// spawnRacePollInterval is the sleep duration used at two distinct points in
// ensureSupervised's retry loop: while waiting for someone else's spawn lock
// to free up, and while waiting for the winner of a just-released lock to
// finish binding its listen socket. Both are the same bounded interval
// deliberately — see ensureSupervised's own doc comment for why the second
// wait matters just as much as the first.
const spawnRacePollInterval = 100 * time.Millisecond

// ensureSupervised implements the supervised EnsureServer strategy: dial (or
// spawn, race-fenced by a worktree-scoped advisory lock) a lyx-owned,
// detached daemon process running command with "serve
// -listen=unix;<socketPath>" appended, recorded in a per-language state
// file so every reconnecting caller — this worktree's own future lyx
// invocations — finds and reuses the same daemon rather than spawning a new
// one. Unlike ensureNative, this function takes a raw command []string, not
// an Entry: it does no toolchain resolution of its own, and is the
// low-level strategy proven directly against a plain "gopls" by its own
// dedicated integration test — no V1 registry entry requests it yet (see
// the plan's "EnsureServer has exactly one live dispatch arm" Shared
// Decision).
//
// The whole call is bounded by deadline := time.Now().Add(timeout): a
// caller that keeps losing the spawn race, or keeps finding a healthy state
// it can never actually dial, gets ErrServerSpawnTimeout rather than
// blocking indefinitely — the "bounded retry, not indefinite blocking" the
// concurrency-locking decision requires.
//
// The daemon this function spawns is never killed by its caller: its
// process is intentionally detached (proc.DetachBreakaway) and outlives
// this call. The only things that ever terminate it are its own idle
// timeout (gopls's own -listen.timeout, left unconfigured here, defaulting
// per gopls itself) or a future restart's stale-socket cleanup finding it
// already dead.
//
// Known limitation: daemonStale only checks PID liveness and protocol
// version, not whether the daemon actually answers a dial. A process that
// is alive but hung, or that never finished binding its listen socket, is
// never classified stale, so every caller's dial-then-finalize keeps
// failing against a state that keeps reading "healthy," and no caller ever
// restarts it. The bounded retry below still returns ErrServerSpawnTimeout
// per call rather than hanging, but a fresh call later hits the identical
// wedged daemon and times out again, indefinitely, until the process dies
// on its own or an operator intervenes. This is accepted as a known gap
// rather than fixed with a dial-failure-triggers-restart heuristic, because
// that heuristic risks misclassifying a daemon that is merely slow to bind
// on first spawn (the exact case the dial retry below already exists to
// tolerate) as wedged, and getting that distinction right needs empirical
// grounding this task has no reason to invest in — supervised has no live
// V1 dispatch path, so this limitation affects no production caller yet.
func ensureSupervised(ctx context.Context, command []string, lang, targetDir, worktreeRoot string, timeout time.Duration) (*lspClient, error) {
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	statePath := layout.CodeintelDaemonStateFile(lang)
	lockPath := layout.CodeintelDaemonLock(lang)
	// The daemon's socket path is a deterministic function of
	// (worktreeRoot, lang), not randomly chosen at spawn time — this keeps
	// the state file's address field stable across restarts, since there is
	// nothing to "choose" or persist separately from recomputing it.
	socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")

	rootURI, err := rootURIFor(targetDir)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)

	for {
		// Step 1: the common-case reconnect-to-a-healthy-daemon path. This
		// never touches the lock at all.
		state, found, err := readDaemonState(statePath)
		if err != nil {
			// A genuine OS-level failure (permissions, corrupt file, disk
			// full) — distinct from "no state file yet" (found == false,
			// err == nil) — aborts the whole call rather than being folded
			// into the retry logic below.
			return nil, fmt.Errorf("codeintelengine: ensureSupervised read daemon state for %q: %w", lang, err)
		}
		if found && !daemonStale(state) {
			if network, address, ok := strings.Cut(state.Address, ";"); ok {
				if client, dialErr := newLSPClientDial(ctx, network, address); dialErr == nil {
					if finalizeErr := finalizeConnection(ctx, client, rootURI, timeout); finalizeErr == nil {
						return client, nil
					}
					// Initialize/probe failed against a state that read as
					// healthy; fall through to the lock/spawn path below
					// rather than retrying the same dead end.
				}
			}
		}

		// Step 2: the dial/finalize above failed, or the state was
		// absent/stale — try to become the spawner. gofrs/flock opens the
		// lock file with O_CREATE but never creates missing parent
		// directories, so a worktree's very first supervised call (before
		// .lyx/codeintel/<lang>/ exists at all) must create it here first,
		// matching this package's own MkdirAll-before-lock precedent
		// (goToolchainInstallLock in toolchain.go).
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			return nil, fmt.Errorf("codeintelengine: ensureSupervised create spawn lock dir %s: %w", filepath.Dir(lockPath), err)
		}
		fileLock, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("codeintelengine: ensureSupervised acquire spawn lock for %q: %w", lang, err)
		}
		if !acquired {
			// Someone else is spawning or restarting; re-check state after a
			// short bounded interval rather than blocking on the lock, so
			// this call keeps re-evaluating whether the state has become
			// healthy meanwhile.
			if time.Now().After(deadline) {
				return nil, &ErrServerSpawnTimeout{Lang: lang}
			}
			time.Sleep(spawnRacePollInterval)
			continue
		}

		// Step 3: double-check under the lock — another process may have
		// spawned a fresh daemon while this call was waiting for the lock.
		state, found, err = readDaemonState(statePath)
		if err != nil {
			fileLock.Release()
			return nil, fmt.Errorf("codeintelengine: ensureSupervised re-read daemon state for %q: %w", lang, err)
		}
		if found && !daemonStale(state) {
			fileLock.Release()
			// The winner writes the state file and releases the lock
			// *before* its own dial-retry loop (step 6 below) confirms the
			// daemon has actually finished binding its listen socket. A
			// loser reaching here immediately after the winner's release
			// would otherwise land back on step 1 dialing a socket that
			// isn't bound yet, repeatedly, with no backoff anywhere in this
			// specific path — a tight spin for the duration of the bind
			// window. This sleep is what prevents that.
			time.Sleep(spawnRacePollInterval)
			continue
		}

		// Step 4: this call is the winner. Stale-socket cleanup runs
		// unconditionally before every spawn, first-ever or restart — a
		// nonexistent-path removal is a harmless no-op, and a leftover
		// socket file from a dead daemon would otherwise make the fresh
		// spawn's bind fail with EADDRINUSE.
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			fileLock.Release()
			return nil, fmt.Errorf("codeintelengine: ensureSupervised remove stale socket %s: %w", socketPath, err)
		}

		argv := append(append([]string{}, command...), "serve", fmt.Sprintf("-listen=unix;%s", socketPath))
		cmd := exec.Command(argv[0], argv[1:]...)
		// Detach (and, on Windows, break away from any Job Object lyx itself
		// runs in) so the daemon survives this process's exit — it is meant
		// to outlive this one call by design.
		proc.DetachBreakaway(cmd)
		// Surface the daemon's own diagnostics rather than discarding them,
		// matching newLSPClient's existing convention.
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			// A spawn that fails to even start is not a race-losable
			// condition worth looping on.
			fileLock.Release()
			return nil, fmt.Errorf("codeintelengine: ensureSupervised spawn daemon for %q: %w", lang, err)
		}

		// Step 5: write the state file *before* releasing the lock, so a
		// losing caller that acquires the lock immediately after release
		// always sees a state file, never a window where the lock is free
		// but no state exists yet.
		if err := writeDaemonState(statePath, daemonState{
			PID:             cmd.Process.Pid,
			Address:         "unix;" + socketPath,
			ProtocolVersion: supervisedProtocolVersion,
			StartedAt:       time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			fileLock.Release()
			return nil, fmt.Errorf("codeintelengine: ensureSupervised write daemon state for %q: %w", lang, err)
		}
		fileLock.Release()

		// Step 6: dial the daemon just spawned, with a short bounded retry
		// of its own — the process needs a moment to bind the listen socket
		// after cmd.Start() returns.
		var client *lspClient
		var dialErr error
		for attempt := 0; attempt < 10; attempt++ {
			client, dialErr = newLSPClientDial(ctx, "unix", socketPath)
			if dialErr == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if dialErr != nil {
			return nil, fmt.Errorf("codeintelengine: ensureSupervised dial newly spawned daemon for %q: %w", lang, dialErr)
		}

		// Step 7.
		if err := finalizeConnection(ctx, client, rootURI, timeout); err != nil {
			return nil, err
		}
		return client, nil
	}
}
