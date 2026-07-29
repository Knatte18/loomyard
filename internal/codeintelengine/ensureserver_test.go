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
