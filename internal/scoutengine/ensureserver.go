// ensureserver.go implements the EnsureServer(lang, anchorRoot) -> LSPConn seam from
// manifest/designs/scout-redesign.md: given a registry entry whose HasNativeDaemon field is true,
// it resolves, spawns or dials, and hands back an already-initialized, already-probed *lspClient
// ready for immediate use.
// ensureServer is called only for a registry entry with HasNativeDaemon == true — in V1 this means
// Go only.
// Every other language's caller keeps using newLSPClient/client.initialize directly, unchanged, and
// never calls into this file at all.

package scoutengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
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

// ensureServer resolves, spawns or dials, and initializes a language server connection,
// returning it ready for use alongside the connKind needed for correct teardown.
func ensureServer(ctx context.Context, lang string, entry Entry, targetDir string, anchorRoot string, timeout time.Duration) (*lspClient, connKind, error) {
	binPath, err := resolveGoToolchain(ctx, entry.PinnedVersion)
	if err != nil {
		return nil, connKindNative, fmt.Errorf("scoutengine: resolve go toolchain for %q: %w", lang, err)
	}

	// binPath (the toolchain-resolved binary), not entry.Command[0] (the
	// literal string "gopls"), is what gets launched — entry.Command[1:] is
	// any fixed extra args the registry entry carries, the same
	// binPath-plus-extraArgs composition nativeArgv applies for the native
	// strategy's own argv.
	command := append([]string{binPath}, entry.Command[1:]...)
	client, err := ensureSupervised(ctx, command, lang, targetDir, anchorRoot, timeout)
	if err == nil {
		return client, connKindSupervised, nil
	}

	client, err = ensureNative(ctx, lang, entry, targetDir, timeout)
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
		return fmt.Errorf("scoutengine: probe failed: %w", err)
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
		return "", fmt.Errorf("scoutengine: resolve absolute path for %s: %w", targetDir, err)
	}
	return "file://" + absTargetDir, nil
}

// daemonIdleTimeout overrides gopls's own idle-timeout default (1 minute,
// via -remote.listen.timeout for the native strategy's shared daemon, or
// -listen.timeout for the supervised strategy's directly-spawned daemon) for
// both EnsureServer strategies. The 1-minute default is tuned for a human's
// edit-pause-edit rhythm, not an agent's — a coding agent's own reasoning
// time between scout calls routinely exceeds a minute (a benchmark run
// against this exact package measured 30-160s of agent think time per call,
// dwarfing gopls's own ~0.3-1.6s query latency), so every lookup in a
// realistic agent investigation pays a fresh cold-start rather than reusing
// the daemon a prior call already warmed. Ten minutes is sized to survive
// that gap without leaving an idle daemon running indefinitely after the
// investigation ends.
const daemonIdleTimeout = 10 * time.Minute

// nativeArgv builds the gopls invocation for the native strategy: the
// toolchain-resolved binary, any fixed extra args the registry entry
// carries, and the -remote=auto proxy flags (including the idle-timeout
// override above). Split out from ensureNative so the argv shape itself is
// unit-testable without spawning a process.
func nativeArgv(binPath string, extraArgs []string) []string {
	// binPath (the toolchain-resolved binary), not entry.Command[0] (the
	// literal string "gopls"), is what gets launched — extraArgs is
	// entry.Command[1:] (empty for Go's current registry entry, but
	// preserved for forward compatibility with a future entry that carries
	// extra fixed args), per toolchain-manager-authority's exact
	// argv-composition decision.
	argv := append([]string{binPath}, extraArgs...)
	return append(argv, "-remote=auto", fmt.Sprintf("-remote.listen.timeout=%s", daemonIdleTimeout))
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
		return nil, fmt.Errorf("scoutengine: resolve go toolchain for %q: %w", lang, err)
	}

	argv := nativeArgv(binPath, entry.Command[1:])

	client, err := newLSPClient(argv)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ErrServerNotFound{Language: lang, InstallHint: entry.InstallHint}
		}
		return nil, fmt.Errorf("scoutengine: start language server for %q: %w", lang, err)
	}
	client.lang = lang

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

// supervisedArgv builds the daemon invocation for the supervised strategy:
// command as given (no toolchain resolution — the caller already resolved
// or chose it), followed by "serve", the unix-socket -listen flag, and the
// same daemonIdleTimeout override nativeArgv applies, expressed as gopls's
// serve-mode -listen.timeout flag. Split out from ensureSupervised so the
// argv shape itself is unit-testable without spawning a process, mirroring
// nativeArgv.
func supervisedArgv(command []string, socketPath string) []string {
	argv := append(append([]string{}, command...), "serve")
	argv = append(argv, fmt.Sprintf("-listen=unix;%s", socketPath))
	return append(argv, fmt.Sprintf("-listen.timeout=%s", daemonIdleTimeout))
}

// spawnRacePollInterval is the sleep duration used at two distinct points in
// ensureSupervised's retry loop: while waiting for someone else's spawn lock
// to free up, and while waiting for the winner of a just-released lock to
// finish binding its listen socket. Both are the same bounded interval
// deliberately — see ensureSupervised's own doc comment for why the second
// wait matters just as much as the first.
const spawnRacePollInterval = 100 * time.Millisecond

// reconnectUnderLock implements the "re-dial-under-lock, reuse-or-restart"
// decision at the heart of ensureSupervised's wedged-daemon escalation: once
// step 1's initial dial-or-finalize attempt has failed against a state that
// read as healthy, this is what decides whether the daemon is merely
// unreachable for a moment (another caller already respawned, or the same
// daemon recovered on its own) or genuinely wedged. It is a pure helper —
// dial and finalize are injected function values, and it performs no
// proc.KillPID, spawn, or state I/O of its own — so this decision is
// unit-testable with fakes and no real process, kept separate from
// ensureSupervised, which alone owns the escalation's side effects.
//
// It runs the same bounded 10x50ms dial retry the spawner's own step 6 uses
// against address, and on a successful dial runs finalize. A successful
// fresh dial+finalize returns (client, true, nil): the daemon was not
// actually wedged, and the caller must not kill it. A failed retry or a
// failed finalize returns (nil, false, nil): the daemon is genuinely
// wedged, and the caller must proc.KillPID + respawn. This function does
// not itself enforce ensureSupervised's overall deadline — its own retry is
// already inherently bounded (~500ms worst case), and the caller applies
// its own deadline check immediately after this function returns, exactly
// as it already does around the spawner's step 6 dial retry.
func reconnectUnderLock(ctx context.Context, network, address string, dial func(ctx context.Context, network, address string) (*lspClient, error), finalize func(ctx context.Context, client *lspClient) error) (client *lspClient, healthy bool, err error) {
	var dialErr error
	for attempt := 0; attempt < 10; attempt++ {
		client, dialErr = dial(ctx, network, address)
		if dialErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if dialErr != nil {
		return nil, false, nil
	}
	if finalizeErr := finalize(ctx, client); finalizeErr != nil {
		return nil, false, nil
	}
	return client, true, nil
}

// ensureSupervised implements the supervised EnsureServer strategy: dial (or
// spawn, race-fenced by a worktree-scoped advisory lock) a lyx-owned,
// detached daemon process running command with "serve
// -listen=unix;<socketPath>" appended, recorded in a per-language state
// file so every reconnecting caller — this worktree's own future lyx
// invocations — finds and reuses the same daemon rather than spawning a new
// one. Unlike ensureNative, this function takes a raw command []string, not
// an Entry: it does no toolchain resolution of its own — that is
// ensureServer's job, which resolves the toolchain and dispatches Go's
// registry entry here as its live V1 strategy, with ensureNative as its
// fallback.
//
// The whole call is bounded by deadline := time.Now().Add(timeout): a
// caller that keeps losing the spawn race, or keeps finding a healthy state
// it can never actually dial, gets ErrServerSpawnTimeout rather than
// blocking indefinitely — the "bounded retry, not indefinite blocking" the
// concurrency-locking decision requires.
//
// The daemon this function spawns is intentionally detached
// (proc.DetachBreakaway) and outlives a normal call — this function does
// not kill a daemon just because the call that reused or spawned it is
// ending. A wedged daemon is the one exception: this function's own
// wedged-daemon escalation (below) force-kills it via proc.KillPID once a
// fresh re-dial under the spawn lock confirms it no longer answers at all,
// then respawns in its place. Otherwise the daemon ends on its own: its own
// idle timeout — set explicitly via supervisedArgv's
// -listen.timeout=<daemonIdleTimeout> flag, not left to gopls's own
// default — or a future restart's stale-socket cleanup finding it already
// dead.
//
// daemonStale itself still only checks PID liveness and protocol version,
// not whether the daemon actually answers a dial — a process that is alive
// but hung, or that never finished binding its listen socket, is never
// classified stale on its own. What used to be this function's known gap —
// every caller's dial-then-finalize failing forever against a state that
// keeps reading "healthy," with no caller ever restarting it — is closed by
// the wedged-daemon escalation above: a dial-or-finalize failure against a
// non-stale state re-dials under the spawn lock and, only if that fresh
// attempt also fails, force-kills and respawns. Now that Go's registry
// entry dispatches here as its live V1 strategy (via ensureServer), that
// escalation is what keeps a single wedged daemon from stranding every
// caller in this worktree indefinitely.
func ensureSupervised(ctx context.Context, command []string, lang, targetDir string, anchorRoot string, timeout time.Duration) (*lspClient, error) {
	statePath := DaemonStateFile(anchorRoot, lang)
	lockPath := DaemonLock(anchorRoot, lang)
	// The daemon's socket path is a deterministic function of
	// (anchorRoot, lang), not randomly chosen at spawn time — this keeps
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
			return nil, fmt.Errorf("scoutengine: ensureSupervised read daemon state for %q: %w", lang, err)
		}
		// escalating distinguishes two very different reasons for reaching
		// the lock below: a non-stale state whose dial-or-finalize just
		// failed (the wedged-daemon scenario this function's own doc
		// comment names, handled by the escalation branch after the lock is
		// acquired) versus an absent/stale state (the ordinary spawn-race
		// path, step 3 below). Conflating the two would make the ordinary
		// path's ordinary re-check-and-spin (step 3) the thing that handles
		// a wedged daemon too, which just spins step1->step3 forever
		// against the same non-stale-but-undialable state.
		escalating := false
		if found && !daemonStale(state) {
			if network, address, ok := strings.Cut(state.Address, ";"); ok {
				if client, dialErr := newLSPClientDial(ctx, network, address); dialErr == nil {
					client.lang = lang
					if finalizeErr := finalizeConnection(ctx, client, rootURI, timeout); finalizeErr == nil {
						return client, nil
					}
					// Initialize/probe failed against a state that read as
					// healthy; escalate under the lock below rather than
					// retrying the same dead end.
					escalating = true
				} else {
					escalating = true
				}
			}
		}

		// Step 2: the dial/finalize above failed against a non-stale state
		// (escalating), or the state was absent/stale — either way, try to
		// become the lock holder. gofrs/flock opens the lock file with
		// O_CREATE but never creates missing parent directories, so a
		// worktree's very first supervised call (before
		// .lyx/scout/<lang>/ exists at all) must create it here first,
		// matching this package's own MkdirAll-before-lock precedent
		// (goToolchainInstallLock in toolchain.go).
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			return nil, fmt.Errorf("scoutengine: ensureSupervised create spawn lock dir %s: %w", filepath.Dir(lockPath), err)
		}
		fileLock, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("scoutengine: ensureSupervised acquire spawn lock for %q: %w", lang, err)
		}
		if !acquired {
			// Someone else is spawning, restarting, or running this same
			// escalation; re-check state after a short bounded interval
			// rather than blocking on the lock, so this call keeps
			// re-evaluating whether the state has become healthy meanwhile.
			if time.Now().After(deadline) {
				return nil, &ErrServerSpawnTimeout{Lang: lang}
			}
			time.Sleep(spawnRacePollInterval)
			continue
		}

		if escalating {
			// Wedged-daemon escalation: re-read the state under the lock —
			// another caller may have already respawned, or the same
			// daemon may have recovered, while this call waited for the
			// lock — and re-dial whatever address is currently recorded,
			// with the same bounded retry the spawner's own step 6 uses,
			// before concluding the daemon is genuinely wedged rather than
			// merely racing another caller.
			escalationState, escalationFound, err := readDaemonState(statePath)
			if err != nil {
				fileLock.Release()
				return nil, fmt.Errorf("scoutengine: ensureSupervised re-read daemon state during wedged-daemon escalation for %q: %w", lang, err)
			}

			var reconnected *lspClient
			healthy := false
			if escalationFound {
				if network, address, ok := strings.Cut(escalationState.Address, ";"); ok {
					reconnected, healthy, err = reconnectUnderLock(ctx, network, address, newLSPClientDial, func(ctx context.Context, client *lspClient) error {
						return finalizeConnection(ctx, client, rootURI, timeout)
					})
					if err != nil {
						fileLock.Release()
						return nil, fmt.Errorf("scoutengine: ensureSupervised re-dial under lock for %q: %w", lang, err)
					}
				}
			}
			if healthy {
				// The fresh under-lock dial+finalize succeeded: another
				// caller already respawned, or this same daemon recovered,
				// between the failed dial in step 1 and acquiring the lock
				// here. Reuse the connection — the daemon was never
				// actually wedged, so killing it would be wrong.
				fileLock.Release()
				return reconnected, nil
			}

			// Same deadline guard step 2's !acquired branch above already
			// applies: reconnectUnderLock's own retry is inherently bounded
			// (~500ms worst case), but a caller whose deadline expires
			// during that retry must still get ErrServerSpawnTimeout here,
			// before ever attempting proc.KillPID/respawn below — otherwise
			// a caller racing a deadline this short could fall through to
			// respawn and fail at cmd.Start() with a spawn error instead of
			// the documented timeout.
			if time.Now().After(deadline) {
				fileLock.Release()
				return nil, &ErrServerSpawnTimeout{Lang: lang}
			}

			// The fresh under-lock dial+finalize also failed (or there was
			// no recorded state left to re-dial at all): the daemon is
			// genuinely wedged, not merely racing another caller or slow to
			// bind. proc.KillPID is best-effort — if the daemon already
			// died on its own between the failed re-dial and this call,
			// KillPID returns a non-nil error (e.g. ESRCH/ErrProcessDone);
			// that is exactly the outcome the respawn below already handles
			// correctly, so it is logged, not returned or treated as this
			// call's own failure.
			if escalationFound {
				if err := proc.KillPID(escalationState.PID); err != nil {
					logger.Warn("scoutengine: kill wedged supervised daemon", "pid", escalationState.PID, "lang", lang, "err", err)
				}
			}
			// Fall through to step 4 below to respawn — the lock acquired
			// above is still held, so the respawn proceeds directly without
			// releasing and re-acquiring it.
		} else {
			// Step 3: double-check under the lock — another process may
			// have spawned a fresh daemon while this call was waiting for
			// the lock.
			state, found, err = readDaemonState(statePath)
			if err != nil {
				fileLock.Release()
				return nil, fmt.Errorf("scoutengine: ensureSupervised re-read daemon state for %q: %w", lang, err)
			}
			if found && !daemonStale(state) {
				fileLock.Release()
				// Same deadline guard as step 2's !acquired branch above:
				// without it, an uncontended lock combined with a state
				// that always reads healthy but never actually dials would
				// spin step1->step3 forever with no bound (this exact
				// scenario is now instead caught by step 1's escalating
				// branch above, but the guard stays here too since step 3
				// is reachable whenever step 1 found an absent/stale state
				// that a racing caller then filled in), contradicting the
				// "whole call is bounded by deadline" guarantee documented
				// above.
				if time.Now().After(deadline) {
					return nil, &ErrServerSpawnTimeout{Lang: lang}
				}
				// The winner writes the state file and releases the lock
				// *before* its own dial-retry loop (step 6 below) confirms
				// the daemon has actually finished binding its listen
				// socket. A loser reaching here immediately after the
				// winner's release would otherwise land back on step 1
				// dialing a socket that isn't bound yet, repeatedly, with
				// no backoff anywhere in this specific path — a tight spin
				// for the duration of the bind window. This sleep is what
				// prevents that.
				time.Sleep(spawnRacePollInterval)
				continue
			}
		}

		// Step 4: this call is the winner. Stale-socket cleanup runs
		// unconditionally before every spawn, first-ever or restart — a
		// nonexistent-path removal is a harmless no-op, and a leftover
		// socket file from a dead daemon would otherwise make the fresh
		// spawn's bind fail with EADDRINUSE.
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			fileLock.Release()
			return nil, fmt.Errorf("scoutengine: ensureSupervised remove stale socket %s: %w", socketPath, err)
		}

		argv := supervisedArgv(command, socketPath)
		cmd := exec.Command(argv[0], argv[1:]...)
		// Detach (and, on Windows, break away from any Job Object lyx itself
		// runs in) so the daemon survives this process's exit — it is meant
		// to outlive this one call by design.
		proc.DetachBreakaway(cmd)
		// Surface the daemon's own diagnostics to a log file beside the
		// state file — never to this process's own os.Stderr. os.Stderr
		// would wire the detached daemon's stderr fd to whatever
		// pipe/terminal this one call happened to inherit, which can close
		// (or itself die, taking a pipe reader with it — e.g. a caller
		// piped through "| head" and the reader exits) long before the
		// daemon is meant to. The daemon's next write to that now-reader-
		// less pipe then raises SIGPIPE and kills it, silently defeating
		// the entire point of DetachBreakaway above: the daemon dies with
		// (or because of) a caller it was explicitly detached from. A
		// stable log file the daemon's own duplicated fd owns
		// independently is what actually decouples its lifetime from the
		// caller's.
		logPath := filepath.Join(filepath.Dir(statePath), "daemon.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fileLock.Release()
			return nil, fmt.Errorf("scoutengine: ensureSupervised open daemon log %s: %w", logPath, err)
		}
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			logFile.Close()
			// A spawn that fails to even start is not a race-losable
			// condition worth looping on.
			fileLock.Release()
			return nil, fmt.Errorf("scoutengine: ensureSupervised spawn daemon for %q: %w", lang, err)
		}
		// cmd.Start() gave the child its own duplicated copy of logFile's
		// fd; this process's handle must not be held open for the rest of
		// this call's (or the daemon's) lifetime.
		logFile.Close()
		// CONSTRAINTS.md's Live-Substrate Spawn Observability entry requires
		// a spawn-side log line for a live-substrate spawn point; cmd.Process
		// is guaranteed non-nil here since cmd.Start() has already succeeded
		// (the line 548 error path above returns before ever reaching this
		// point).
		logger.Info("scoutengine: spawned supervised daemon", "lang", lang, "pid", cmd.Process.Pid, "socket", socketPath)

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
			return nil, fmt.Errorf("scoutengine: ensureSupervised write daemon state for %q: %w", lang, err)
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
			return nil, fmt.Errorf("scoutengine: ensureSupervised dial newly spawned daemon for %q: %w", lang, dialErr)
		}
		client.lang = lang

		// Step 7.
		if err := finalizeConnection(ctx, client, rootURI, timeout); err != nil {
			return nil, err
		}
		return client, nil
	}
}
