// lspclient_test.go exercises lspClient's framing/protocol logic without
// launching a real subprocess: it builds the client over the
// newLSPClientFromRW(rwc) seam with an io.Pipe-backed transport, driven by
// a scripted fake-server goroutine that reads Content-Length-framed
// requests and writes back Content-Length-framed responses. Untagged and
// spawn-free — no exec.Command anywhere in this file; a real os/exec launch
// belongs in refs_integration_test.go's //go:build integration test.
//
// The fake-server helpers report failures via t.Errorf and an "ok" return
// rather than t.Fatalf: testing.T's FailNow (which Fatalf calls) must only
// be invoked from the goroutine running the test function itself, never
// from a helper goroutine such as the fake server below. Each test's
// goroutine body checks "ok" and returns early on a scripting failure; the
// client-side call's own context timeout (5s in every test here) is what
// bounds the test's total runtime if the fake server bails out early
// without ever responding.

package codeintelengine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pipeTransport wires a client-side io.ReadWriteCloser to a server-side
// io.ReadWriteCloser over two io.Pipes: everything the client writes is
// what the server reads, and vice versa.
type pipeTransport struct {
	io.Reader
	io.Writer
	closers []io.Closer
}

func (p pipeTransport) Close() error {
	var err error
	for _, c := range p.closers {
		if cerr := c.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}

// newPipeTransportPair returns two linked transports: client (for
// newLSPClientFromRW) and server (for the fake-server goroutine to drive).
func newPipeTransportPair() (client, server pipeTransport) {
	clientReadServerWrite, serverWriteClientRead := io.Pipe()
	serverReadClientWrite, clientWriteServerRead := io.Pipe()

	client = pipeTransport{
		Reader:  clientReadServerWrite,
		Writer:  clientWriteServerRead,
		closers: []io.Closer{clientReadServerWrite, clientWriteServerRead},
	}
	server = pipeTransport{
		Reader:  serverReadClientWrite,
		Writer:  serverWriteClientRead,
		closers: []io.Closer{serverReadClientWrite, serverWriteClientRead},
	}
	return client, server
}

// fakeServer reads Content-Length-framed JSON-RPC messages from r and
// writes framed responses to w, matching the same wire shape lspClient's
// writeMessage/readMessage speak. It lets a test script canned responses
// (and server-initiated requests) keyed by the incoming request's method.
type fakeServer struct {
	r *bufio.Reader
	w io.Writer
}

func newFakeServer(rw io.ReadWriter) *fakeServer {
	return &fakeServer{r: bufio.NewReader(rw), w: rw}
}

// fakeServerMessage mirrors lspMessage's wire shape for the test's own
// send/receive helpers, independent of the production type so this file
// tests the wire format itself rather than reusing the type under test.
type fakeServerMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// readMessage reads one Content-Length-framed message. On any framing
// failure it records the failure via t.Errorf and returns ok=false; it
// never calls t.Fatalf, since this runs on the fake-server goroutine, not
// the test's own goroutine.
func (s *fakeServer) readMessage(t *testing.T) (msg fakeServerMessage, ok bool) {
	t.Helper()
	contentLength := -1
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			t.Errorf("fakeServer: read header: %v", err)
			return fakeServerMessage{}, false
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, cut := strings.Cut(line, ":"); cut && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				t.Errorf("fakeServer: parse Content-Length: %v", err)
				return fakeServerMessage{}, false
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		t.Errorf("fakeServer: message missing Content-Length header")
		return fakeServerMessage{}, false
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.r, body); err != nil {
		t.Errorf("fakeServer: read body: %v", err)
		return fakeServerMessage{}, false
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Errorf("fakeServer: unmarshal body: %v", err)
		return fakeServerMessage{}, false
	}
	return msg, true
}

// writeMessage frames and writes v. Like readMessage, it reports failure via
// t.Errorf and a bool rather than t.Fatalf.
func (s *fakeServer) writeMessage(t *testing.T, v any) bool {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Errorf("fakeServer: marshal: %v", err)
		return false
	}
	if _, err := fmt.Fprintf(s.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		t.Errorf("fakeServer: write header: %v", err)
		return false
	}
	if _, err := s.w.Write(body); err != nil {
		t.Errorf("fakeServer: write body: %v", err)
		return false
	}
	return true
}

// respond writes a success response for the request identified by id.
func (s *fakeServer) respond(t *testing.T, id json.RawMessage, result any) bool {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Errorf("fakeServer: marshal result: %v", err)
		return false
	}
	return s.writeMessage(t, fakeServerMessage{JSONRPC: "2.0", ID: id, Result: raw})
}

// request writes a server-initiated request with the given id and method.
func (s *fakeServer) request(t *testing.T, id int, method string) bool {
	t.Helper()
	return s.writeMessage(t, fakeServerMessage{JSONRPC: "2.0", ID: json.RawMessage(strconv.Itoa(id)), Method: method})
}

// TestLSPClient_InitializeCapturesCapabilities drives the initialize
// handshake against a fake server that advertises workspaceSymbolProvider,
// and asserts the client's supportsWorkspaceSymbol() reflects it.
func TestLSPClient_InitializeCapturesCapabilities(t *testing.T) {
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
		if !server.respond(t, req.ID, map[string]any{
			"capabilities": map[string]any{
				"workspaceSymbolProvider": true,
			},
		}) {
			return
		}
		// initialized is a notification (no id); read and discard it so the
		// pipe doesn't leave an unread message for the next test step.
		server.readMessage(t)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}
	<-done

	if !client.supportsWorkspaceSymbol() {
		t.Error("supportsWorkspaceSymbol() = false; want true (server advertised workspaceSymbolProvider)")
	}
}

// TestLSPClient_AnswersServerInitiatedRequest asserts that while call() is
// awaiting its own response, a server-initiated request (e.g.
// client/registerCapability) received in the meantime is answered with an
// empty success result rather than left unanswered.
func TestLSPClient_AnswersServerInitiatedRequest(t *testing.T) {
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

		// Before answering initialize, issue a server-initiated request the
		// client must answer inline while it's still awaiting its own
		// response.
		if !server.request(t, 999, "client/registerCapability") {
			return
		}
		reply, ok := server.readMessage(t)
		if !ok {
			return
		}
		if string(reply.ID) != "999" {
			t.Errorf("fakeServer: client/registerCapability reply id = %s; want 999", reply.ID)
		}
		if string(reply.Result) != "null" {
			t.Errorf("fakeServer: client/registerCapability reply result = %s; want an empty/null result", reply.Result)
		}

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
	<-done
}

// TestLSPClient_ReferencesSendsIncludeDeclarationAndParsesResult asserts
// that references() sends includeDeclaration: true in its request context
// and correctly parses a multi-location response.
func TestLSPClient_ReferencesSendsIncludeDeclarationAndParsesResult(t *testing.T) {
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
		if req.Method != "textDocument/references" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "textDocument/references")
			return
		}

		var params struct {
			Context struct {
				IncludeDeclaration bool `json:"includeDeclaration"`
			} `json:"context"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("fakeServer: unmarshal references params: %v", err)
			return
		}
		if !params.Context.IncludeDeclaration {
			t.Error("fakeServer: textDocument/references params.context.includeDeclaration = false; want true")
		}

		server.respond(t, req.ID, []map[string]any{
			{
				"uri": "file:///tmp/example/foo.go",
				"range": map[string]any{
					"start": map[string]any{"line": 4, "character": 6},
					"end":   map[string]any{"line": 4, "character": 9},
				},
			},
			{
				"uri": "file:///tmp/example/bar.go",
				"range": map[string]any{
					"start": map[string]any{"line": 10, "character": 2},
					"end":   map[string]any{"line": 10, "character": 5},
				},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	locations, err := client.references(ctx, "file:///tmp/example/foo.go", lspPosition{Line: 4, Character: 6})
	if err != nil {
		t.Fatalf("references() returned unexpected error: %v", err)
	}
	<-done

	if len(locations) != 2 {
		t.Fatalf("references() returned %d locations; want 2", len(locations))
	}
	if got, want := formatLocation(locations[0]), "/tmp/example/foo.go:5:7"; got != want {
		t.Errorf("references()[0] = %q; want %q", got, want)
	}
	if got, want := formatLocation(locations[1]), "/tmp/example/bar.go:11:3"; got != want {
		t.Errorf("references()[1] = %q; want %q", got, want)
	}
}

// TestLSPClient_CallReturnsErrServerTimeoutOnExpiredContext asserts that a
// context whose deadline has already passed causes call() (exercised here
// via references()) to return ErrServerTimeout without ever blocking on a
// server response, and that errors.Is matches it.
func TestLSPClient_CallReturnsErrServerTimeoutOnExpiredContext(t *testing.T) {
	clientTransport, serverTransport := newPipeTransportPair()
	defer clientTransport.Close()
	defer serverTransport.Close()

	client := newLSPClientFromRW(clientTransport)
	// No fake-server goroutine reads or responds: the point of this test is
	// that call() never waits for one because ctx is already expired.

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := client.references(ctx, "file:///tmp/example/foo.go", lspPosition{Line: 0, Character: 0})
	if err == nil {
		t.Fatal("references() with an expired context returned nil error; want ErrServerTimeout")
	}
	if !errors.Is(err, ErrServerTimeoutSentinel) {
		t.Errorf("references() with an expired context err = %v; want errors.Is(err, ErrServerTimeoutSentinel)", err)
	}
}
