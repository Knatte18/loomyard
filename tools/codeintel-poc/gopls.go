// gopls.go implements the harness's two gopls-comparison arms: "gopls-refs"
// (a held-open LSP subprocess, measuring warm-up separately from
// steady-state) and "gopls-cli-refs" (a fresh `gopls references` process per
// query, the cold-per-call baseline). Both speak to the `gopls` binary found
// on $PATH; neither implements more of the LSP than textDocument/references
// needs, per this batch's Batch-local decision.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// goplsInstallHint is the error text every gopls-dependent mode reports when
// the binary is missing from $PATH, naming the exact install command per
// Shared Decision network-prerequisites so an operator hitting this cold
// knows the one command that unblocks the arm.
const goplsInstallHint = "gopls not found on $PATH: install it with `go install golang.org/x/tools/gopls@latest`"

// lspPosition is the LSP wire shape for a text position: zero-based line and
// a UTF-16 code-unit offset into that line (not a byte or rune offset — LSP
// mandates UTF-16 regardless of the server's internal encoding).
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is the LSP wire shape for a half-open [Start, End) text range.
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

// lspLocation is the LSP wire shape for one reference result: a document URI
// plus the range within it.
type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// lspError is the LSP/JSON-RPC error object shape, present on a response
// message when the server could not fulfil the request.
type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// lspMessage is the generic JSON-RPC-over-LSP envelope this harness reads.
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

// lspClient drives one held-open gopls subprocess over the standard
// Content-Length-framed LSP envelope on stdin/stdout. It supports exactly
// the request/notification shapes this harness needs (initialize,
// initialized, textDocument/references, shutdown, exit) — not the full LSP
// surface, per this batch's Batch-local decision.
type lspClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
	closed bool
}

// newLSPClient spawns bin (the resolved gopls path) as a subprocess and
// wires up its stdin/stdout for LSP framing. The subprocess's stderr is
// forwarded to this process's stderr so gopls's own diagnostic logging is
// visible rather than silently discarded or deadlocking on a full pipe.
func newLSPClient(bin string) (*lspClient, error) {
	cmd := exec.Command(bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open gopls stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open gopls stdout: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gopls: %w", err)
	}

	return &lspClient{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}, nil
}

// writeMessage marshals v and frames it with the LSP Content-Length header,
// the wire shape every LSP implementation (gopls included) requires
// regardless of message kind (request, response, or notification).
func (c *lspClient) writeMessage(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal lsp message: %w", err)
	}
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write lsp header: %w", err)
	}
	if _, err := c.stdin.Write(body); err != nil {
		return fmt.Errorf("write lsp body: %w", err)
	}
	return nil
}

// readMessage reads one Content-Length-framed message from gopls's stdout.
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

// call sends a JSON-RPC request and blocks until the matching response
// arrives, returning its raw result. While waiting, it also answers any
// server-initiated request it encounters (e.g. gopls's
// client/registerCapability or workspace/configuration) with an empty
// success result — this harness implements no client-side LSP capability of
// its own, so an honest empty response is the correct answer rather than
// leaving gopls blocked waiting on a reply we will never send. Notifications
// (messages with no id) are dropped silently.
func (c *lspClient) call(method string, params any) (json.RawMessage, error) {
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

	for {
		msg, err := c.readMessage()
		if err != nil {
			return nil, fmt.Errorf("await response to %s: %w", method, err)
		}

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

// references issues one textDocument/references request and returns the
// result formatted as sorted "uri:line:character" strings (1-based for
// display, matching how the in-process arms format token.Position).
func (c *lspClient) references(fileURI string, pos lspPosition) ([]string, error) {
	raw, err := c.call("textDocument/references", map[string]any{
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

	out := make([]string, len(locations))
	for i, loc := range locations {
		out[i] = fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
	}
	sort.Strings(out)
	return out, nil
}

// close runs the LSP shutdown handshake (shutdown request, exit
// notification) and waits for the subprocess to exit. It is best-effort and
// idempotent: a failed shutdown RPC is logged rather than returned, since by
// the time close is called the harness has already gathered the
// measurements it needs and a clean process exit is a nice-to-have, not a
// correctness requirement.
func (c *lspClient) close() {
	if c.closed {
		return
	}
	c.closed = true

	if _, err := c.call("shutdown", nil); err != nil {
		fmt.Fprintf(os.Stderr, "codeintel-poc: gopls shutdown request: %v\n", err)
	}
	if err := c.notify("exit", nil); err != nil {
		fmt.Fprintf(os.Stderr, "codeintel-poc: gopls exit notification: %v\n", err)
	}
	c.stdin.Close()
	if err := c.cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "codeintel-poc: gopls process exit: %v\n", err)
	}
}

// toLSPPosition converts a go/token.Position (1-based line, 1-based byte
// column) to the LSP wire position (0-based line, 0-based UTF-16 code-unit
// offset). The conversion re-reads the source line because token.Position's
// byte column and LSP's UTF-16 offset only coincide for pure-ASCII lines;
// any multi-byte rune before the target column would otherwise misalign the
// position gopls is asked to query.
func toLSPPosition(pos token.Position) (lspPosition, error) {
	data, err := os.ReadFile(pos.Filename)
	if err != nil {
		return lspPosition{}, fmt.Errorf("read %s: %w", pos.Filename, err)
	}

	lines := strings.Split(string(data), "\n")
	if pos.Line < 1 || pos.Line > len(lines) {
		return lspPosition{}, fmt.Errorf("line %d out of range in %s (%d lines)", pos.Line, pos.Filename, len(lines))
	}

	line := lines[pos.Line-1]
	byteCol := pos.Column - 1
	if byteCol < 0 {
		byteCol = 0
	}
	if byteCol > len(line) {
		byteCol = len(line)
	}

	return lspPosition{Line: pos.Line - 1, Character: utf16Length(line[:byteCol])}, nil
}

// utf16Length returns the number of UTF-16 code units s would occupy on the
// wire, the unit LSP positions are always expressed in regardless of the
// server's internal string representation.
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if units := utf16.RuneLen(r); units > 0 {
			n += units
		} else {
			// RuneLen reports -1 for a rune it cannot encode (e.g. an
			// unpaired surrogate); count it as one unit rather than drop it,
			// so the running offset never falls behind the true position.
			n++
		}
	}
	return n
}

// runGoplsRefs implements the "gopls-refs" mode: spawn gopls once, hold it
// open across cfg.n textDocument/references queries for the resolved
// symbol, and report the first query's latency (spawn + initialize + first
// query, since gopls loads the package graph lazily on first request) as
// warm-up separately from the remaining queries' steady-state latency.
func runGoplsRefs(cfg config) error {
	bin, err := exec.LookPath("gopls")
	if err != nil {
		return fmt.Errorf("%s", goplsInstallHint)
	}

	pkgs, _, err := loadPackages(cfg.dir)
	if err != nil {
		return err
	}
	obj, err := resolveSymbol(pkgs, cfg.symbol)
	if err != nil {
		return err
	}
	pos := pkgs[0].Fset.Position(obj.Pos())

	lspPos, err := toLSPPosition(pos)
	if err != nil {
		return fmt.Errorf("convert %q's position for gopls: %w", cfg.symbol, err)
	}
	fileURI := "file://" + pos.Filename

	absDir, err := filepath.Abs(cfg.dir)
	if err != nil {
		return fmt.Errorf("resolve module root %q: %w", cfg.dir, err)
	}
	rootURI := "file://" + absDir

	repeats := cfg.n
	if repeats < 1 {
		repeats = 1
	}

	start := time.Now()
	client, err := newLSPClient(bin)
	if err != nil {
		return err
	}
	defer client.close()

	if _, err := client.call("initialize", map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      rootURI,
		"capabilities": map[string]any{},
	}); err != nil {
		return fmt.Errorf("gopls initialize: %w", err)
	}
	if err := client.notify("initialized", map[string]any{}); err != nil {
		return fmt.Errorf("gopls initialized notification: %w", err)
	}

	positions, err := client.references(fileURI, lspPos)
	warmUp := time.Since(start)
	if err != nil {
		return fmt.Errorf("gopls references (warm-up query): %w", err)
	}

	durations := make([]time.Duration, 0, repeats-1)
	for i := 1; i < repeats; i++ {
		queryStart := time.Now()
		result, err := client.references(fileURI, lspPos)
		durations = append(durations, time.Since(queryStart))
		if err != nil {
			return fmt.Errorf("gopls references (steady-state query %d): %w", i, err)
		}
		positions = result
	}

	printTimedReport(cfg, "gopls-refs", warmUp, durations, len(positions), positions)
	return nil
}

// runGoplsCLIRefs implements the "gopls-cli-refs" mode: shell out to
// `gopls references <file>:<line>:<col>` once per query, each invocation a
// fresh process, to capture the cold-per-call penalty a caller pays when it
// cannot amortize gopls's package-graph load across repeated queries (the
// comparison point against "gopls-refs"'s held-open steady-state).
func runGoplsCLIRefs(cfg config) error {
	bin, err := exec.LookPath("gopls")
	if err != nil {
		return fmt.Errorf("%s", goplsInstallHint)
	}

	pkgs, _, err := loadPackages(cfg.dir)
	if err != nil {
		return err
	}
	obj, err := resolveSymbol(pkgs, cfg.symbol)
	if err != nil {
		return err
	}
	pos := pkgs[0].Fset.Position(obj.Pos())
	posArg := fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)

	repeats := cfg.n
	if repeats < 1 {
		repeats = 1
	}

	var positions []string
	var warmUp time.Duration
	durations := make([]time.Duration, 0, repeats-1)
	for i := 0; i < repeats; i++ {
		start := time.Now()
		out, err := exec.Command(bin, "references", posArg).CombinedOutput()
		elapsed := time.Since(start)
		if err != nil {
			return fmt.Errorf("gopls references %s (query %d): %w\n%s", posArg, i, err, out)
		}

		positions = parseGoplsCLIReferences(out)
		if i == 0 {
			warmUp = elapsed
		} else {
			durations = append(durations, elapsed)
		}
	}

	printTimedReport(cfg, "gopls-cli-refs", warmUp, durations, len(positions), positions)
	return nil
}

// parseGoplsCLIReferences splits the `gopls references` CLI's output into
// one trimmed, non-empty line per reference — the CLI prints one
// "file:line:col" location per line with no other structured shape to
// parse.
func parseGoplsCLIReferences(out []byte) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
