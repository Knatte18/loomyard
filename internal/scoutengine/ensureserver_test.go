// ensureserver_test.go covers finalizeConnection, the shared initialize+probe+kill-on-failure
// sequence ensureserver.go defines, plus the argv-shape and wedged-daemon-escalation decision
// helpers (nativeArgv/supervisedArgv, reconnectUnderLock) that are unit-testable without spawning
// anything.
// Untagged and offline: every case here builds a client over newLSPClientFromRW +
// newPipeTransportPair/fakeServer, the same fake-transport harness lspclient_test.go already
// establishes for this package, reusable with no import since it's the same package, or drives a
// pure decision helper with injected fakes.
// This file is not file-allowlisted in cmd/lyx/tierpurity_test.go's allowedSpawners map, so it must
// never spawn a real process — any test needing a real subprocess belongs in supervised_test.go or
// a //go:build integration file instead.

package scoutengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// TestFinalizeConnection_SuccessReturnsNil verifies finalizeConnection succeeds with a responsive
// server.
func TestFinalizeConnection_SuccessReturnsNil(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "initialize" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "initialize")
			return
		}
		if !server.respond(t, req.ID, map[string]any{"capabilities": map[string]any{}}) {
			return
		}
		server.readMessage(t) // initialized notification

		probeReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if probeReq.Method != "workspace/symbol" {
			t.Errorf("fakeServer: got request method %q; want %q", probeReq.Method, "workspace/symbol")
			return
		}
		server.respond(t, probeReq.ID, []map[string]any{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := finalizeConnection(ctx, client, "file:///tmp/example", 5*time.Second); err != nil {
		t.Fatalf("finalizeConnection() returned unexpected error: %v", err)
	}
	<-done
}

// TestFinalizeConnection_InitializeErrorKillsClient drives a fake server that answers initialize
// with an LSP error response,
// and asserts finalizeConnection returns a non-nil error and, whitebox-asserted via the unexported
// client.closed field, that the client was torn down.
func TestFinalizeConnection_InitializeErrorKillsClient(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "initialize" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "initialize")
			return
		}
		server.writeMessage(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			"error":   map[string]any{"code": -32603, "message": "boom"},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := finalizeConnection(ctx, client, "file:///tmp/example", 5*time.Second)
	<-done
	if err == nil {
		t.Fatal("finalizeConnection() returned nil error; want a non-nil error from the failed initialize handshake")
	}
	if !client.closed {
		t.Error("finalizeConnection() left client.closed = false after an initialize failure; want true")
	}
}

// TestFinalizeConnection_ProbeTimeoutKillsClient drives a fake server that answers initialize
// successfully but never answers the follow-up workspace/symbol probe request,
// and asserts finalizeConnection returns an error satisfying errors.Is(err,
// ErrServerTimeoutSentinel) once the short timeout passed as the timeout argument expires, and that
// client.closed is true.
func TestFinalizeConnection_ProbeTimeoutKillsClient(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "initialize" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "initialize")
			return
		}
		if !server.respond(t, req.ID, map[string]any{"capabilities": map[string]any{}}) {
			return
		}
		server.readMessage(t) // initialized notification

		// Read the probe request off the pipe (so the client's blocking
		// pipe write can complete) but never answer it, forcing
		// finalizeConnection's probe phase to hit its own context deadline.
		probeReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if probeReq.Method != "workspace/symbol" {
			t.Errorf("fakeServer: got request method %q; want %q", probeReq.Method, "workspace/symbol")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := finalizeConnection(ctx, client, "file:///tmp/example", 200*time.Millisecond)
	<-done
	if err == nil {
		t.Fatal("finalizeConnection() returned nil error; want a probe-timeout error")
	}
	if !errors.Is(err, ErrServerTimeoutSentinel) {
		t.Errorf("finalizeConnection() err = %v; want errors.Is(err, ErrServerTimeoutSentinel)", err)
	}
	if !client.closed {
		t.Error("finalizeConnection() left client.closed = false after a probe timeout; want true")
	}
}

// TestNativeArgv_IncludesExtendedIdleTimeout asserts nativeArgv passes an explicit
// -remote.listen.timeout overriding gopls's own 1-minute default — the default is tuned for a
// human's edit-pause-edit rhythm, not an agent's think-time gaps between scout calls (see
// daemonIdleTimeout's own doc comment for the benchmark this responds to).
func TestNativeArgv_IncludesExtendedIdleTimeout(t *testing.T) {
	argv := nativeArgv("/path/to/gopls", nil)

	wantTimeoutFlag := fmt.Sprintf("-remote.listen.timeout=%s", daemonIdleTimeout)
	found := false
	for _, arg := range argv {
		if arg == wantTimeoutFlag {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("nativeArgv() = %v; want it to contain %q", argv, wantTimeoutFlag)
	}
	if daemonIdleTimeout <= time.Minute {
		t.Errorf("daemonIdleTimeout = %s; want it longer than gopls's own 1-minute default, or the override is pointless", daemonIdleTimeout)
	}
}

// TestNativeArgv_PreservesBinPathAndExtraArgs asserts nativeArgv keeps the resolved binary path
// first and any entry.Command[1:] extra args between it and the -remote flags, matching
// ensureNative's existing argv-composition contract (toolchain-manager-authority decision).
func TestNativeArgv_PreservesBinPathAndExtraArgs(t *testing.T) {
	argv := nativeArgv("/path/to/gopls", []string{"-v"})

	if len(argv) < 2 || argv[0] != "/path/to/gopls" || argv[1] != "-v" {
		t.Errorf("nativeArgv() = %v; want binPath first, then extraArgs, then -remote flags", argv)
	}
	if argv[len(argv)-2] != "-remote=auto" {
		t.Errorf("nativeArgv() = %v; want -remote=auto second-to-last", argv)
	}
}

// TestSupervisedArgv_IncludesServeListenAndIdleTimeout asserts supervisedArgv's argv shape without
// spawning anything: the command as given, then "serve", the unix-socket -listen flag, and the same
// daemonIdleTimeout override nativeArgv applies, expressed as gopls's serve-mode -listen.timeout
// flag.
func TestSupervisedArgv_IncludesServeListenAndIdleTimeout(t *testing.T) {
	argv := supervisedArgv([]string{"/path/to/gopls"}, "/tmp/example/daemon.sock")

	wantServe := "serve"
	wantListenFlag := "-listen=unix;/tmp/example/daemon.sock"
	wantTimeoutFlag := fmt.Sprintf("-listen.timeout=%s", daemonIdleTimeout)

	var hasServe, hasListenFlag, hasTimeoutFlag bool
	for _, arg := range argv {
		switch arg {
		case wantServe:
			hasServe = true
		case wantListenFlag:
			hasListenFlag = true
		case wantTimeoutFlag:
			hasTimeoutFlag = true
		}
	}
	if !hasServe {
		t.Errorf("supervisedArgv() = %v; want it to contain %q", argv, wantServe)
	}
	if !hasListenFlag {
		t.Errorf("supervisedArgv() = %v; want it to contain %q", argv, wantListenFlag)
	}
	if !hasTimeoutFlag {
		t.Errorf("supervisedArgv() = %v; want it to contain %q", argv, wantTimeoutFlag)
	}
}

// TestReconnectUnderLock_ReuseOrRestart drives reconnectUnderLock's pure dial-then-finalize
// decision against injected fakes, covering every case ensureSupervised's wedged-daemon escalation
// depends on to decide whether to reuse a connection or restart the daemon: (a) another caller
// already respawned while this call waited for the lock, (b) the same daemon recovered on its own,
// and (c) the daemon is genuinely wedged (dial keeps failing, or dial succeeds but finalize fails).
// (a) and (b) are indistinguishable from reconnectUnderLock's own perspective — both are "the fresh
// under-lock dial+finalize succeeded" — so they exercise the same code path;
// they are kept as separate named cases here because ensureSupervised's own doc comment and this
// test's card distinguish them as two different real-world causes of the same outcome.
// No real dial, finalize, spawn, or kill happens anywhere in this test — reconnectUnderLock is a
// pure helper, per its own doc comment.
func TestReconnectUnderLock_ReuseOrRestart(t *testing.T) {
	fakeClient := &lspClient{}
	fakeDialErr := errors.New("dial refused")
	fakeFinalizeErr := errors.New("initialize failed")

	tests := []struct {
		name        string
		dial        func(ctx context.Context, network, address string) (*lspClient, error)
		finalize    func(ctx context.Context, client *lspClient) error
		wantClient  *lspClient
		wantHealthy bool
	}{
		{
			name: "AnotherCallerAlreadyRespawned",
			dial: func(ctx context.Context, network, address string) (*lspClient, error) {
				return fakeClient, nil
			},
			finalize:    func(ctx context.Context, client *lspClient) error { return nil },
			wantClient:  fakeClient,
			wantHealthy: true,
		},
		{
			name: "SameDaemonRecovered",
			dial: func(ctx context.Context, network, address string) (*lspClient, error) {
				return fakeClient, nil
			},
			finalize:    func(ctx context.Context, client *lspClient) error { return nil },
			wantClient:  fakeClient,
			wantHealthy: true,
		},
		{
			name: "GenuinelyWedged_DialAlwaysFails",
			dial: func(ctx context.Context, network, address string) (*lspClient, error) {
				return nil, fakeDialErr
			},
			finalize: func(ctx context.Context, client *lspClient) error {
				t.Fatal("finalize called despite every dial attempt failing")
				return nil
			},
			wantClient:  nil,
			wantHealthy: false,
		},
		{
			name: "GenuinelyWedged_DialSucceedsFinalizeFails",
			dial: func(ctx context.Context, network, address string) (*lspClient, error) {
				return fakeClient, nil
			},
			finalize:    func(ctx context.Context, client *lspClient) error { return fakeFinalizeErr },
			wantClient:  nil,
			wantHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, healthy, err := reconnectUnderLock(context.Background(), "unix", "/tmp/example.sock", tt.dial, tt.finalize)
			if err != nil {
				t.Fatalf("reconnectUnderLock() returned unexpected error: %v", err)
			}
			if healthy != tt.wantHealthy {
				t.Errorf("reconnectUnderLock() healthy = %v; want %v", healthy, tt.wantHealthy)
			}
			if client != tt.wantClient {
				t.Errorf("reconnectUnderLock() client = %v; want %v", client, tt.wantClient)
			}
		})
	}
}

// TestEnsureServer_SupervisedFailsForNonToolchainReasonFallsBackToNative drives ensureServer's
// step-3 safety net — "supervised fails for a non-toolchain reason, so native is attempted and
// native's own result is returned" — the one branch no other untagged test reaches: the
// toolchain-failure test (TestReferences_HasNativeDaemonRoutesThroughEnsureServer, refs_test.go)
// short-circuits at step 1 before ensureSupervised is ever attempted, and the integration test
// (batch 5) only proves the fallback's success path, not this one.
// resolveGoToolchain succeeds here — a fake installer writes a non-executable stub binary,
// and resolveGoToolchain's own fast-path/post-install check is a bare os.Stat that never inspects
// the executable bit — but ensureSupervised cannot acquire its spawn lock (pre-held by this test)
// and returns ErrServerSpawnTimeout within a short deadline, a non-toolchain reason that triggers
// the fallback.
// The fallback's ensureNative re-resolves the same stub binPath and fails at exec.LookPath (the
// stub is non-executable) before any cmd.Start, so this test spawns no subprocess and stays
// untagged/process-free.
func TestEnsureServer_SupervisedFailsForNonToolchainReasonFallsBackToNative(t *testing.T) {
	withTempUserCacheDir(t)

	binName := "gopls"
	if runtime.GOOS == "windows" {
		binName = "gopls.exe"
	}
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		// A present-but-non-executable stub: resolveGoToolchain's os.Stat-
		// only checks are satisfied by mere presence, which is all step 1
		// (the toolchain resolve) needs to succeed, while still guaranteeing
		// the fallback's newLSPClient -> exec.LookPath call fails, with no
		// real subprocess ever spawned by either strategy.
		return os.WriteFile(filepath.Join(destDir, binName), nil, 0o644)
	})

	worktreeRoot := t.TempDir()
	lockPath := DaemonLock(worktreeRoot, "go")
	// The lock's parent directory does not otherwise exist under a fresh
	// t.TempDir() worktree root — no daemon state write happens first to
	// create it as a side effect here, unlike supervised_test.go's own
	// retry-exhaustion test — so lock.AcquireWriteLock would otherwise fail
	// opening the lock file with a not-exist error.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%s) failed: %v", filepath.Dir(lockPath), err)
	}
	fileLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.AcquireWriteLock() failed: %v", err)
	}
	t.Cleanup(func() { _ = fileLock.Release() })

	entry := Entry{
		Command:         []string{"gopls"},
		PinnedVersion:   "v0.0.0-test",
		HasNativeDaemon: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// command is never reached by either strategy: ensureSupervised can
	// never win the pre-held lock, so it returns ErrServerSpawnTimeout well
	// within this deadline, and the fallback's ensureNative fails at
	// exec.LookPath before spawning anything either.
	client, kind, err := ensureServer(ctx, "go", entry, t.TempDir(), worktreeRoot, 300*time.Millisecond)
	if client != nil {
		t.Errorf("ensureServer() client = %v; want nil", client)
	}
	if kind != connKindNative {
		t.Errorf("ensureServer() connKind = %v; want connKindNative (proving the fallback fired, not the supervised success path)", kind)
	}
	if err == nil {
		t.Fatal("ensureServer() returned nil error; want native's own LookPath failure")
	}
	if errors.Is(err, ErrServerSpawnTimeoutSentinel) {
		t.Errorf("ensureServer() err = %v; want it to NOT satisfy errors.Is(err, ErrServerSpawnTimeoutSentinel) (native's own result, not supervised's timeout, must be what ensureServer returns)", err)
	}
}
