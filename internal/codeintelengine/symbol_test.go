// symbol_test.go is the untagged, spawn-free counterpart to any future
// symbol_integration_test.go, mirroring definition_test.go's style for the
// legacy-path regression proof, and TestLSPClient_InitializeCapturesCapabilities's
// fake-server shape for symbolFromClient's own logic: Symbol's interesting
// behavior lives entirely in how symbolFromClient interprets
// workspace/symbol's result, and connection acquisition is already covered
// generically elsewhere, so these tests call symbolFromClient directly
// against a hand-built client rather than driving Symbol's full
// DetectLanguage/acquireConnection machinery, which would require a real
// spawn.

package codeintelengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSymbol_NonExistentServerBinaryYieldsErrServerNotFound points a
// synthetic registry entry's Command at a binary that cannot exist on
// $PATH and asserts Symbol maps the resulting exec.LookPath failure to
// ErrServerNotFoundSentinel — the same legacy-path regression proof
// definition_test.go's equivalent runs for Definition. This one does
// exercise the full Symbol function, since it never reaches a connection
// at all.
func TestSymbol_NonExistentServerBinaryYieldsErrServerNotFound(t *testing.T) {
	dir := t.TempDir()
	reg := Registry{
		"go": {
			Markers:     []string{"go.mod"},
			Match:       "any",
			Command:     []string{"lyx-codeintel-nonexistent-binary-xyz"},
			InstallHint: "this binary is intentionally fake for the test",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Symbol(ctx, Options{
		Registry:  reg,
		TargetDir: dir,
		Lang:      "go",
		Query:     Query{Symbol: "Resolve"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, ErrServerNotFoundSentinel) {
		t.Errorf("Symbol() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
	}
}

// TestSymbolFromClient_TwoCandidatesReturnsBothMatchesNotAmbiguous asserts
// that symbolFromClient returns every candidate workspace/symbol reports
// rather than collapsing to ErrAmbiguousSymbol — a bug that accidentally
// reintroduces resolvePosition-style collapsing is exactly the regression
// this test exists to catch.
func TestSymbolFromClient_TwoCandidatesReturnsBothMatchesNotAmbiguous(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		initReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if initReq.Method != "initialize" {
			t.Errorf("fakeServer: got request method %q; want %q", initReq.Method, "initialize")
			return
		}
		if !server.respond(t, initReq.ID, map[string]any{
			"capabilities": map[string]any{"workspaceSymbolProvider": true},
		}) {
			return
		}
		server.readMessage(t) // initialized notification

		symReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if symReq.Method != "workspace/symbol" {
			t.Errorf("fakeServer: got request method %q; want %q", symReq.Method, "workspace/symbol")
			return
		}
		// aaa.go/bbb.go are chosen so that workspaceSymbol's own
		// location-string sort leaves them in this same order, keeping the
		// assertions below straightforward to read.
		server.respond(t, symReq.ID, []map[string]any{
			{
				"name": "Foo",
				"kind": 12,
				"location": map[string]any{
					"uri": "file:///tmp/example/aaa.go",
					"range": map[string]any{
						"start": map[string]any{"line": 4, "character": 6},
						"end":   map[string]any{"line": 4, "character": 9},
					},
				},
			},
			{
				"name": "Bar",
				"kind": 6,
				"location": map[string]any{
					"uri": "file:///tmp/example/bbb.go",
					"range": map[string]any{
						"start": map[string]any{"line": 10, "character": 2},
						"end":   map[string]any{"line": 10, "character": 5},
					},
				},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}

	matches, err := symbolFromClient(ctx, client, "go", Entry{Command: []string{"gopls"}}, "Foo", 5*time.Second)
	<-done
	if err != nil {
		t.Fatalf("symbolFromClient() returned unexpected error: %v", err)
	}
	if errors.Is(err, ErrAmbiguousSymbolSentinel) {
		t.Error("symbolFromClient() err unexpectedly matches ErrAmbiguousSymbolSentinel; Symbol must never collapse multiple candidates to ambiguous")
	}

	if len(matches) != 2 {
		t.Fatalf("symbolFromClient() returned %d matches; want 2", len(matches))
	}
	want := []SymbolMatch{
		{Name: "Foo", Kind: 12, File: "/tmp/example/aaa.go", Line: 5, Character: 7},
		{Name: "Bar", Kind: 6, File: "/tmp/example/bbb.go", Line: 11, Character: 3},
	}
	for i := range want {
		if matches[i] != want[i] {
			t.Errorf("symbolFromClient()[%d] = %+v; want %+v", i, matches[i], want[i])
		}
	}
}

// TestSymbolFromClient_ZeroCandidatesReturnsErrSymbolNotFound asserts that
// a workspace/symbol response with zero candidates maps to
// ErrSymbolNotFoundSentinel.
func TestSymbolFromClient_ZeroCandidatesReturnsErrSymbolNotFound(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	done := make(chan struct{})
	go func() {
		defer close(done)
		initReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if !server.respond(t, initReq.ID, map[string]any{
			"capabilities": map[string]any{"workspaceSymbolProvider": true},
		}) {
			return
		}
		server.readMessage(t) // initialized notification

		symReq, ok := server.readMessage(t)
		if !ok {
			return
		}
		if symReq.Method != "workspace/symbol" {
			t.Errorf("fakeServer: got request method %q; want %q", symReq.Method, "workspace/symbol")
			return
		}
		server.respond(t, symReq.ID, []map[string]any{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}

	_, err := symbolFromClient(ctx, client, "go", Entry{Command: []string{"gopls"}}, "NoSuchSymbol", 5*time.Second)
	<-done
	if !errors.Is(err, ErrSymbolNotFoundSentinel) {
		t.Errorf("symbolFromClient() err = %v; want errors.Is(err, ErrSymbolNotFoundSentinel)", err)
	}
}

// TestSymbolFromClient_UnsupportedWorkspaceSymbolNeverSendsRequest asserts
// that when the server's initialize response omits workspaceSymbolProvider,
// symbolFromClient returns ErrResolverUnsupported and — critically — never
// sends a workspace/symbol request at all. The fake server's own
// goroutine, still reading past the initialize handshake, is the witness:
// it fails the test via t.Errorf if it ever receives a further byte, and
// exits quietly when the transport closes with nothing written, which is
// the expected outcome once symbolFromClient returns without ever calling
// client.workspaceSymbol.
func TestSymbolFromClient_UnsupportedWorkspaceSymbolNeverSendsRequest(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()

	client := newLSPClientFromRW(clientTransport)
	server := newFakeServer(serverTransport)

	initDone := make(chan struct{})
	go func() {
		defer close(initDone)
		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "initialize" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "initialize")
			return
		}
		// Deliberately omit workspaceSymbolProvider so
		// client.supportsWorkspaceSymbol() reports false.
		if !server.respond(t, req.ID, map[string]any{"capabilities": map[string]any{}}) {
			return
		}
		server.readMessage(t) // initialized notification
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}
	<-initDone

	if client.supportsWorkspaceSymbol() {
		t.Fatal("supportsWorkspaceSymbol() = true; want false (server omitted workspaceSymbolProvider)")
	}

	// Guard the workspace/symbol phase: this goroutine blocks on the first
	// byte of any further message. If symbolFromClient wrongly calls
	// client.workspaceSymbol anyway, that byte arrives and fails the test;
	// otherwise the transport close below unblocks the read with a
	// transport-closed error, the quiet path this goroutine takes when
	// nothing was ever sent.
	unexpectedRequest := make(chan struct{})
	go func() {
		defer close(unexpectedRequest)
		if _, err := server.r.ReadByte(); err == nil {
			t.Error("fakeServer: received an unexpected byte after an unsupported-capability symbolFromClient call; want no request sent at all")
		}
	}()

	_, err := symbolFromClient(ctx, client, "go", Entry{Command: []string{"gopls"}}, "Resolve", 5*time.Second)
	if !errors.Is(err, ErrResolverUnsupportedSentinel) {
		t.Errorf("symbolFromClient() err = %v; want errors.Is(err, ErrResolverUnsupportedSentinel)", err)
	}

	clientTransport.Close()
	serverTransport.Close()
	<-unexpectedRequest
}
