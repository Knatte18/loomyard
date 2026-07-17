// lspclient.go implements lspClient, a generalized stdio LSP client
// speaking exactly the request/notification surface this engine needs
// (initialize, initialized, textDocument/references, workspace/symbol,
// shutdown, exit) — not the full LSP protocol, per the plan's
// references-only Shared Decision. It is ported from the recovered
// tools/codeintel-poc/gopls.go harness (git show 3b4dcf86), generalized to
// launch any command []string rather than a hardcoded "gopls" lookup, and
// with every request-level call threading a context.Context so a caller can
// bound it with a deadline that hard-kills the subprocess on expiry.
//
// The I/O is factored over an injectable transport for testability: the
// production constructor newLSPClient spawns a subprocess and wires its
// stdio, while the unexported newLSPClientFromRW seam builds a client over
// a caller-supplied io.ReadWriteCloser with no subprocess at all — the fake
// in-memory server in lspclient_test.go drives this seam.

package codeintelengine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// lspError is the LSP/JSON-RPC error object shape, present on a response
// message when the server could not fulfil the request.
type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// lspMessage is the generic JSON-RPC-over-LSP envelope this client reads.
// ID is kept as raw JSON (rather than decoded to int) so it can be echoed
// back byte-for-byte when answering a server-initiated request, and so its
// presence/absence (nil vs set) distinguishes a notification from a request
// or response without a second bespoke type.
type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

// symbolInformation is the LSP wire shape for one workspace/symbol result:
// the symbol's display name plus the location of its declaration. It is
// deliberately narrow — the LSP spec's SymbolInformation carries a "kind"
// and other fields this engine never inspects.
type symbolInformation struct {
	Name     string      `json:"name"`
	Location lspLocation `json:"location"`
}

// capabilities is the narrow slice of the server's initialize response this
// client retains: whether workspace/symbol name resolution is supported.
// The LSP spec allows workspaceSymbolProvider to be either a bare bool or an
// options object; UnmarshalJSON below normalizes both to a Supported bool
// so refs.go's supportsWorkspaceSymbol() check never has to care which
// shape a given server sent.
type capabilities struct {
	WorkspaceSymbolProvider capabilityFlag `json:"workspaceSymbolProvider"`
}

// capabilityFlag normalizes an LSP capability field that servers may report
// either as a bare JSON bool or as a non-null options object (both mean
// "supported"; absent or explicit false/null means "not supported").
type capabilityFlag struct {
	Supported bool
}

// UnmarshalJSON accepts `true`/`false` or any JSON object as a capability
// value, per the LSP spec's "boolean | options object" capability shape.
func (f *capabilityFlag) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		f.Supported = false
		return nil
	}
	if trimmed == "true" {
		f.Supported = true
		return nil
	}
	if trimmed == "false" {
		f.Supported = false
		return nil
	}
	// Any other well-formed JSON value (an options object) is present, so
	// the capability is advertised.
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal capability flag: %w", err)
	}
	f.Supported = true
	return nil
}

// lspClient drives one language-server subprocess (or, via
// newLSPClientFromRW, a caller-supplied transport) over the standard
// Content-Length-framed LSP envelope: w is the outbound half, stdout the
// buffered inbound half, closer tears both down together. cmd is nil when
// the client was built over an injected transport with no subprocess —
// kill() and close() guard on that.
type lspClient struct {
	cmd    *exec.Cmd
	w      io.Writer
	stdout *bufio.Reader
	closer io.Closer
	nextID int
	closed bool
	caps   capabilities
}

// newLSPClient resolves command[0] on $PATH and spawns it with command[1:]
// as arguments, wiring its stdin/stdout for LSP framing. newLSPClient knows
// nothing of which language or install hint command belongs to — that is
// registry.Entry data the caller (refs.go) already has — so a LookPath
// failure is returned as a plain wrapped error (errors.Is(err,
// exec.ErrNotFound) still succeeds); the caller is responsible for
// translating that into the language- and install-hint-carrying
// *ErrServerNotFound. The subprocess's stderr is forwarded to this
// process's stderr so the server's own diagnostic logging is visible
// rather than silently discarded or deadlocking on a full pipe.
func newLSPClient(command []string) (*lspClient, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("codeintelengine: empty launch command")
	}

	bin, err := exec.LookPath(command[0])
	if err != nil {
		return nil, fmt.Errorf("codeintelengine: resolve %q on $PATH: %w", command[0], err)
	}

	cmd := exec.Command(bin, command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdin: %w", bin, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdout: %w", bin, err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}

	return &lspClient{
		cmd:    cmd,
		w:      stdin,
		stdout: bufio.NewReader(stdout),
		closer: stdin,
	}, nil
}

// newLSPClientFromRW builds an lspClient over a caller-supplied
// io.ReadWriteCloser transport with no subprocess. This is the seam
// lspclient_test.go's in-memory fake server drives: it lets the framing and
// protocol logic (writeMessage/readMessage, call/notify, the
// server-initiated-request handling, initialize/references/workspaceSymbol)
// be exercised without spawning a real language server.
func newLSPClientFromRW(rwc io.ReadWriteCloser) *lspClient {
	return &lspClient{
		w:      rwc,
		stdout: bufio.NewReader(rwc),
		closer: rwc,
	}
}

// writeMessage marshals v and frames it with the LSP Content-Length header,
// the wire shape every LSP implementation requires regardless of message
// kind (request, response, or notification).
func (c *lspClient) writeMessage(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal lsp message: %w", err)
	}
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write lsp header: %w", err)
	}
	if _, err := c.w.Write(body); err != nil {
		return fmt.Errorf("write lsp body: %w", err)
	}
	return nil
}

// readMessage reads one Content-Length-framed message from the transport.
// Header lines other than Content-Length (e.g. Content-Type) are accepted
// and ignored, per the LSP base protocol.
func (c *lspClient) readMessage() (*lspMessage, error) {
	contentLength := -1
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read lsp header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length header %q: %w", line, err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("lsp message missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.stdout, body); err != nil {
		return nil, fmt.Errorf("read lsp body (%d bytes): %w", contentLength, err)
	}

	var msg lspMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal lsp message: %w", err)
	}
	return &msg, nil
}

// call sends a JSON-RPC request and blocks until either the matching
// response arrives or ctx is done, whichever comes first. A single
// background goroutine drives the blocking reads (readMessage has no
// context awareness of its own) and forwards each message over an
// unbuffered channel; the select below is what actually respects ctx.Done.
// If ctx expires while a read is in flight, that goroutine leaks until the
// transport is closed — the timeout path's caller is expected to call
// kill(), which unblocks it. While waiting, call also answers any
// server-initiated request it encounters (e.g. client/registerCapability or
// workspace/configuration) with an empty success result — this client
// implements no client-side LSP capability of its own, so an honest empty
// response is the correct answer rather than leaving the server blocked
// waiting on a reply that will never come. Notifications (messages with no
// id) are dropped silently. phase names the current request for
// ErrServerTimeout's Phase field if ctx expires first.
func (c *lspClient) call(ctx context.Context, phase, method string, params any) (json.RawMessage, error) {
	// writeMessage below has no context awareness of its own: on a
	// pipe/subprocess-stdin transport a Write can block until something
	// reads it, so a ctx that is already expired before the write is even
	// attempted must be caught here rather than left to hang. Once the
	// write has actually started, the select loop below is what bounds the
	// remaining wait.
	if err := ctx.Err(); err != nil {
		return nil, &ErrServerTimeout{Phase: phase, Timeout: err.Error()}
	}

	c.nextID++
	id := c.nextID
	idBytes := []byte(strconv.Itoa(id))

	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := c.writeMessage(req); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	type readResult struct {
		msg *lspMessage
		err error
	}
	msgs := make(chan readResult)
	go func() {
		for {
			msg, err := c.readMessage()
			select {
			case msgs <- readResult{msg: msg, err: err}:
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, &ErrServerTimeout{Phase: phase, Timeout: ctx.Err().Error()}
		case r := <-msgs:
			if r.err != nil {
				return nil, fmt.Errorf("await response to %s: %w", method, r.err)
			}
			msg := r.msg

			if len(msg.ID) > 0 && bytes.Equal(msg.ID, idBytes) {
				if msg.Error != nil {
					return nil, fmt.Errorf("%s: lsp error %d: %s", method, msg.Error.Code, msg.Error.Message)
				}
				return msg.Result, nil
			}

			if msg.Method != "" && len(msg.ID) > 0 {
				if err := c.writeMessage(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID), "result": nil}); err != nil {
					return nil, fmt.Errorf("answer server request %s: %w", msg.Method, err)
				}
			}
		}
	}
}

// notify sends a JSON-RPC notification (a message with no id, expecting no
// response) — the shape "initialized" and "exit" require per the LSP spec.
func (c *lspClient) notify(method string, params any) error {
	n := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		n["params"] = params
	}
	if err := c.writeMessage(n); err != nil {
		return fmt.Errorf("send %s notification: %w", method, err)
	}
	return nil
}

// initialize sends the "initialize" request rooted at rootURI, retains the
// server's reported capabilities (at least workspaceSymbolProvider
// presence, via supportsWorkspaceSymbol), and then sends the "initialized"
// notification per the LSP handshake.
func (c *lspClient) initialize(ctx context.Context, rootURI string) error {
	raw, err := c.call(ctx, "initialize", "initialize", map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      rootURI,
		"capabilities": map[string]any{},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var result struct {
		Capabilities capabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("unmarshal initialize result: %w", err)
	}
	c.caps = result.Capabilities

	if err := c.notify("initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

// supportsWorkspaceSymbol reports whether the server's initialize response
// advertised workspaceSymbolProvider. It is only meaningful after a
// successful initialize call.
func (c *lspClient) supportsWorkspaceSymbol() bool {
	return c.caps.WorkspaceSymbolProvider.Supported
}

// references issues one textDocument/references request (with
// includeDeclaration: true, so the declaration site is included alongside
// call sites) and returns the raw location list.
func (c *lspClient) references(ctx context.Context, fileURI string, pos lspPosition) ([]lspLocation, error) {
	raw, err := c.call(ctx, "references", "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": fileURI},
		"position":     pos,
		"context":      map[string]any{"includeDeclaration": true},
	})
	if err != nil {
		return nil, err
	}

	var locations []lspLocation
	if err := json.Unmarshal(raw, &locations); err != nil {
		return nil, fmt.Errorf("unmarshal textDocument/references result: %w", err)
	}
	return locations, nil
}

// workspaceSymbol issues one workspace/symbol query and returns the
// server's candidate matches, each carrying the symbol's name and
// declaration location.
func (c *lspClient) workspaceSymbol(ctx context.Context, query string) ([]symbolInformation, error) {
	raw, err := c.call(ctx, "workspace/symbol", "workspace/symbol", map[string]any{
		"query": query,
	})
	if err != nil {
		return nil, err
	}

	var symbols []symbolInformation
	if err := json.Unmarshal(raw, &symbols); err != nil {
		return nil, fmt.Errorf("unmarshal workspace/symbol result: %w", err)
	}
	sort.Slice(symbols, func(i, j int) bool {
		return formatLocation(symbols[i].Location) < formatLocation(symbols[j].Location)
	})
	return symbols, nil
}

// close runs the graceful LSP shutdown handshake (shutdown request, exit
// notification) and waits for the subprocess to exit. It is best-effort and
// idempotent, for the normal end of a run: a failed shutdown RPC is logged
// rather than returned, since by the time close is called the caller has
// already gathered the result it needs and a clean process exit is a
// nice-to-have, not a correctness requirement. When the client was built
// with no subprocess (newLSPClientFromRW), close only closes the
// transport.
func (c *lspClient) close() {
	if c.closed {
		return
	}
	c.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.call(ctx, "shutdown", "shutdown", nil); err != nil {
		fmt.Fprintf(os.Stderr, "codeintelengine: lsp shutdown request: %v\n", err)
	}
	if err := c.notify("exit", nil); err != nil {
		fmt.Fprintf(os.Stderr, "codeintelengine: lsp exit notification: %v\n", err)
	}
	c.closer.Close()
	if c.cmd != nil {
		if err := c.cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "codeintelengine: lsp process exit: %v\n", err)
		}
	}
}

// kill hard-terminates the subprocess (cmd.Process.Kill(), then Wait to
// reap it) rather than attempting the graceful shutdown/exit handshake,
// which could itself re-block on a server that is already unresponsive —
// this is the timeout-path teardown per the plan's deadline-with-hard-kill
// Shared Decision. It guards on a nil *exec.Cmd: a client built over an
// injected transport (newLSPClientFromRW) has no subprocess to kill, so
// kill only closes the transport.
func (c *lspClient) kill() {
	if c.closed {
		return
	}
	c.closed = true

	c.closer.Close()
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	if err := c.cmd.Process.Kill(); err != nil {
		fmt.Fprintf(os.Stderr, "codeintelengine: kill lsp process: %v\n", err)
	}
	if err := c.cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "codeintelengine: lsp process exit after kill: %v\n", err)
	}
}
