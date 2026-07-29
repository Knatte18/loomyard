// ensureserver_test.go covers finalizeConnection, the shared
// initialize+probe+kill-on-failure sequence ensureserver.go defines.
// Untagged and offline: every case here builds a client over
// newLSPClientFromRW + newPipeTransportPair/fakeServer, the same
// fake-transport harness lspclient_test.go already establishes for this
// package, reusable with no import since it's the same package. This is
// also the natural home for every future ensureServer-adjacent unit test
// batch 6 adds — the dispatcher itself needs no dedicated test today, since
// it has one unconditional branch, nothing to assert beyond what
// ensureNative's own tests already cover transitively via the integration
// test.

package codeintelengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestFinalizeConnection_SuccessReturnsNil drives a fake server that answers
// initialize and the follow-up workspace/symbol probe both successfully,
// and asserts finalizeConnection returns nil.
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

// TestFinalizeConnection_InitializeErrorKillsClient drives a fake server
// that answers initialize with an LSP error response, and asserts
// finalizeConnection returns a non-nil error and, whitebox-asserted via the
// unexported client.closed field, that the client was torn down.
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

// TestFinalizeConnection_ProbeTimeoutKillsClient drives a fake server that
// answers initialize successfully but never answers the follow-up
// workspace/symbol probe request, and asserts finalizeConnection returns an
// error satisfying errors.Is(err, ErrServerTimeoutSentinel) once the short
// timeout passed as the timeout argument expires, and that client.closed is
// true.
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

// TestNativeArgv_IncludesExtendedIdleTimeout asserts nativeArgv passes an
// explicit -remote.listen.timeout overriding gopls's own 1-minute default —
// the default is tuned for a human's edit-pause-edit rhythm, not an agent's
// think-time gaps between codeintel calls (see daemonIdleTimeout's own doc
// comment for the benchmark this responds to).
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

// TestNativeArgv_PreservesBinPathAndExtraArgs asserts nativeArgv keeps the
// resolved binary path first and any entry.Command[1:] extra args between it
// and the -remote flags, matching ensureNative's existing argv-composition
// contract (toolchain-manager-authority decision).
func TestNativeArgv_PreservesBinPathAndExtraArgs(t *testing.T) {
	argv := nativeArgv("/path/to/gopls", []string{"-v"})

	if len(argv) < 2 || argv[0] != "/path/to/gopls" || argv[1] != "-v" {
		t.Errorf("nativeArgv() = %v; want binPath first, then extraArgs, then -remote flags", argv)
	}
	if argv[len(argv)-2] != "-remote=auto" {
		t.Errorf("nativeArgv() = %v; want -remote=auto second-to-last", argv)
	}
}

// TestSupervisedArgv_IncludesServeListenAndIdleTimeout asserts
// supervisedArgv's argv shape without spawning anything: the command as
// given, then "serve", the unix-socket -listen flag, and the same
// daemonIdleTimeout override nativeArgv applies, expressed as gopls's
// serve-mode -listen.timeout flag.
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
