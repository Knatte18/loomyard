// refs_test.go is the untagged, spawn-free counterpart to
// refs_integration_test.go: it exercises References's error-mapping paths
// that do not require a real language server. exec.LookPath failing for a
// nonexistent binary happens before any subprocess is spawned, so this test
// needs no //go:build integration tag and no installed language server.

package scoutengine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCollectInFileMatches exercises the pure, transport-free exact-name
// recursion helper directly — no fake LSP server, no client — over a small
// hand-built hierarchical symbol tree: a top-level match, a match nested
// under a child (proving recursion descends), a name with zero matches, and
// two same-name matches under two different top-level symbols (e.g. an
// "Open" method on two distinct types), proving every match is collected
// rather than the search stopping at the first hit.
func TestCollectInFileMatches(t *testing.T) {
	tree := []lspDocumentSymbol{
		{
			Name: "Reader",
			Children: []lspDocumentSymbol{
				{Name: "Open", SelectionRange: lspRange{Start: lspPosition{Line: 1, Character: 1}}},
				{Name: "Close", SelectionRange: lspRange{Start: lspPosition{Line: 2, Character: 1}}},
			},
		},
		{
			Name: "Writer",
			Children: []lspDocumentSymbol{
				{Name: "Open", SelectionRange: lspRange{Start: lspPosition{Line: 10, Character: 1}}},
			},
		},
		{Name: "TopLevelFunc", SelectionRange: lspRange{Start: lspPosition{Line: 20, Character: 1}}},
	}

	tests := []struct {
		name      string
		query     string
		wantLines []int
	}{
		{name: "TopLevelMatch", query: "TopLevelFunc", wantLines: []int{20}},
		{name: "NestedChildMatch", query: "Close", wantLines: []int{2}},
		{name: "NoMatch", query: "NoSuchSymbol", wantLines: nil},
		{name: "TwoSameNameMatchesAcrossTypes", query: "Open", wantLines: []int{1, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := collectInFileMatches(tree, tt.query)
			if len(matches) != len(tt.wantLines) {
				t.Fatalf("collectInFileMatches(tree, %q) returned %d matches; want %d", tt.query, len(matches), len(tt.wantLines))
			}
			for i, line := range tt.wantLines {
				if got := matches[i].SelectionRange.Start.Line; got != line {
					t.Errorf("collectInFileMatches(tree, %q)[%d].SelectionRange.Start.Line = %d; want %d", tt.query, i, got, line)
				}
			}
		})
	}
}

// TestResolvePosition_InFileSingleMatchReturnsSelectionRangeStart drives
// resolvePosition's InFile branch end to end over the fake transport:
// exactly one documentSymbol match returns that match's
// SelectionRange.Start as the position and "file://"+File as the URI, with
// no round trip through toLSPPosition.
func TestResolvePosition_InFileSingleMatchReturnsSelectionRangeStart(t *testing.T) {
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
			"capabilities": map[string]any{"documentSymbolProvider": true},
		}) {
			return
		}
		server.readMessage(t) // initialized notification

		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "textDocument/documentSymbol" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "textDocument/documentSymbol")
			return
		}
		server.respond(t, req.ID, []map[string]any{
			{
				"name": "Open",
				"kind": 6,
				"range": map[string]any{
					"start": map[string]any{"line": 6, "character": 0},
					"end":   map[string]any{"line": 8, "character": 1},
				},
				"selectionRange": map[string]any{
					"start": map[string]any{"line": 6, "character": 15},
					"end":   map[string]any{"line": 6, "character": 19},
				},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}

	opts := Options{
		TargetDir: "/tmp/example",
		Query:     Query{InFile: &InFileQuery{File: "/tmp/example/foo.go", Name: "Open"}},
		Timeout:   5 * time.Second,
	}
	fileURI, pos, err := resolvePosition(ctx, client, opts, "go", Entry{Command: []string{"gopls"}})
	<-done
	if err != nil {
		t.Fatalf("resolvePosition() returned unexpected error: %v", err)
	}
	if fileURI != "file:///tmp/example/foo.go" {
		t.Errorf("resolvePosition() fileURI = %q; want %q", fileURI, "file:///tmp/example/foo.go")
	}
	wantPos := lspPosition{Line: 6, Character: 15}
	if pos != wantPos {
		t.Errorf("resolvePosition() pos = %+v; want %+v (the match's SelectionRange.Start)", pos, wantPos)
	}
}

// TestResolvePosition_InFileZeroMatchesYieldsErrSymbolNotFound asserts a
// documentSymbol result with no exact-name match maps to
// ErrSymbolNotFoundSentinel.
func TestResolvePosition_InFileZeroMatchesYieldsErrSymbolNotFound(t *testing.T) {
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
			"capabilities": map[string]any{"documentSymbolProvider": true},
		}) {
			return
		}
		server.readMessage(t) // initialized notification

		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "textDocument/documentSymbol" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "textDocument/documentSymbol")
			return
		}
		server.respond(t, req.ID, []map[string]any{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.initialize(ctx, "file:///tmp/example"); err != nil {
		t.Fatalf("initialize() returned unexpected error: %v", err)
	}

	opts := Options{
		TargetDir: "/tmp/example",
		Query:     Query{InFile: &InFileQuery{File: "/tmp/example/foo.go", Name: "NoSuchSymbol"}},
		Timeout:   5 * time.Second,
	}
	_, _, err := resolvePosition(ctx, client, opts, "go", Entry{Command: []string{"gopls"}})
	<-done
	if !errors.Is(err, ErrSymbolNotFoundSentinel) {
		t.Errorf("resolvePosition() err = %v; want errors.Is(err, ErrSymbolNotFoundSentinel)", err)
	}
}

// TestResolvePosition_InFileMultipleMatchesYieldsErrAmbiguousSymbol asserts
// a documentSymbol result with two exact-name matches (e.g. a same-named
// method on two distinct types in the same file) maps to
// ErrAmbiguousSymbolSentinel, with Candidates formatted as file:line:col
// strings.
func TestResolvePosition_InFileMultipleMatchesYieldsErrAmbiguousSymbol(t *testing.T) {
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
			"capabilities": map[string]any{"documentSymbolProvider": true},
		}) {
			return
		}
		server.readMessage(t) // initialized notification

		req, ok := server.readMessage(t)
		if !ok {
			return
		}
		if req.Method != "textDocument/documentSymbol" {
			t.Errorf("fakeServer: got request method %q; want %q", req.Method, "textDocument/documentSymbol")
			return
		}
		server.respond(t, req.ID, []map[string]any{
			{
				"name": "Reader",
				"kind": 23,
				"range": map[string]any{
					"start": map[string]any{"line": 0, "character": 0},
					"end":   map[string]any{"line": 10, "character": 1},
				},
				"selectionRange": map[string]any{
					"start": map[string]any{"line": 0, "character": 5},
					"end":   map[string]any{"line": 0, "character": 11},
				},
				"children": []map[string]any{
					{
						"name": "Open",
						"kind": 6,
						"range": map[string]any{
							"start": map[string]any{"line": 2, "character": 0},
							"end":   map[string]any{"line": 4, "character": 1},
						},
						"selectionRange": map[string]any{
							"start": map[string]any{"line": 2, "character": 15},
							"end":   map[string]any{"line": 2, "character": 19},
						},
					},
				},
			},
			{
				"name": "Writer",
				"kind": 23,
				"range": map[string]any{
					"start": map[string]any{"line": 12, "character": 0},
					"end":   map[string]any{"line": 20, "character": 1},
				},
				"selectionRange": map[string]any{
					"start": map[string]any{"line": 12, "character": 5},
					"end":   map[string]any{"line": 12, "character": 11},
				},
				"children": []map[string]any{
					{
						"name": "Open",
						"kind": 6,
						"range": map[string]any{
							"start": map[string]any{"line": 14, "character": 0},
							"end":   map[string]any{"line": 16, "character": 1},
						},
						"selectionRange": map[string]any{
							"start": map[string]any{"line": 14, "character": 15},
							"end":   map[string]any{"line": 14, "character": 19},
						},
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

	opts := Options{
		TargetDir: "/tmp/example",
		Query:     Query{InFile: &InFileQuery{File: "/tmp/example/foo.go", Name: "Open"}},
		Timeout:   5 * time.Second,
	}
	_, _, err := resolvePosition(ctx, client, opts, "go", Entry{Command: []string{"gopls"}})
	<-done
	var ambiguous *ErrAmbiguousSymbol
	if !errors.As(err, &ambiguous) {
		t.Fatalf("resolvePosition() err = %v; want *ErrAmbiguousSymbol", err)
	}
	if !errors.Is(err, ErrAmbiguousSymbolSentinel) {
		t.Errorf("resolvePosition() err = %v; want errors.Is(err, ErrAmbiguousSymbolSentinel)", err)
	}
	want := []string{"/tmp/example/foo.go:3:16", "/tmp/example/foo.go:15:16"}
	if len(ambiguous.Candidates) != len(want) {
		t.Fatalf("resolvePosition() ambiguous.Candidates = %v; want %v", ambiguous.Candidates, want)
	}
	for i := range want {
		if ambiguous.Candidates[i] != want[i] {
			t.Errorf("resolvePosition() ambiguous.Candidates[%d] = %q; want %q", i, ambiguous.Candidates[i], want[i])
		}
	}
}

// TestResolvePosition_InFileUnsupportedDocumentSymbolNeverSendsRequest
// asserts that when the server's initialize response omits
// documentSymbolProvider, resolvePosition's InFile branch returns
// ErrResolverUnsupported and never issues a textDocument/documentSymbol
// request at all — mirroring symbol_test.go's
// TestSymbolFromClient_UnsupportedWorkspaceSymbolNeverSendsRequest precedent
// for the workspace/symbol path.
func TestResolvePosition_InFileUnsupportedDocumentSymbolNeverSendsRequest(t *testing.T) {
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
		// Deliberately omit documentSymbolProvider so
		// client.supportsDocumentSymbol() reports false.
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

	if client.supportsDocumentSymbol() {
		t.Fatal("supportsDocumentSymbol() = true; want false (server omitted documentSymbolProvider)")
	}

	// Guard the documentSymbol phase: this goroutine blocks on the first
	// byte of any further message. If resolvePosition's InFile branch
	// wrongly calls client.documentSymbol anyway, that byte arrives and
	// fails the test; otherwise the transport close below unblocks the read
	// with a transport-closed error, the quiet path this goroutine takes
	// when nothing was ever sent.
	unexpectedRequest := make(chan struct{})
	go func() {
		defer close(unexpectedRequest)
		if _, err := server.r.ReadByte(); err == nil {
			t.Error("fakeServer: received an unexpected byte after an unsupported-capability resolvePosition InFile call; want no request sent at all")
		}
	}()

	opts := Options{
		TargetDir: "/tmp/example",
		Query:     Query{InFile: &InFileQuery{File: "/tmp/example/foo.go", Name: "Open"}},
		Timeout:   5 * time.Second,
	}
	_, _, err := resolvePosition(ctx, client, opts, "go", Entry{Command: []string{"gopls"}})
	if !errors.Is(err, ErrResolverUnsupportedSentinel) {
		t.Errorf("resolvePosition() err = %v; want errors.Is(err, ErrResolverUnsupportedSentinel)", err)
	}

	clientTransport.Close()
	serverTransport.Close()
	<-unexpectedRequest
}

// TestReferences_HasNativeDaemonRoutesThroughEnsureServer proves that a
// registry entry with HasNativeDaemon: true takes the ensureServer path,
// not the legacy newLSPClient(entry.Command) path — without spawning a
// real gopls. It swaps installGoToolchain for a fake that always fails
// with a distinct, recognizable error, then asserts References's returned
// error wraps that exact sentinel: only reachable if the call chain was
// References -> lookup -> acquireConnection -> ensureServer ->
// resolveGoToolchain -> the fake installer. ensureServer resolves the
// toolchain directly and returns on failure before ever attempting
// ensureSupervised or ensureNative, so this fake-install failure never
// reaches either strategy. Had References instead taken the legacy path,
// it would fail with ErrServerNotFoundSentinel from a literal, unresolved
// "gopls" lookup on $PATH — a categorically different error this assertion
// distinguishes from. This is not a proof that a real gopls connection
// works end to end — that is ensureserver_integration_test.go (batch 5).
func TestReferences_HasNativeDaemonRoutesThroughEnsureServer(t *testing.T) {
	withTempUserCacheDir(t)

	errFakeInstallRefused := errors.New("fake install refused")
	withFakeInstaller(t, func(ctx context.Context, version, destDir string) error {
		return errFakeInstallRefused
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := References(ctx, Options{
		Registry: Registry{
			"go": {
				Command:         []string{"gopls"},
				PinnedVersion:   "v0.0.0-test",
				HasNativeDaemon: true,
			},
		},
		TargetDir: t.TempDir(),
		Lang:      "go",
		Query:     Query{Symbol: "X"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, errFakeInstallRefused) {
		t.Errorf("References() with HasNativeDaemon: true err = %v; want errors.Is(err, errFakeInstallRefused) (proving the ensureServer -> resolveGoToolchain path was taken, not the legacy newLSPClient path)", err)
	}
}

// TestReferences_NonExistentServerBinaryYieldsErrServerNotFound points a
// synthetic registry entry's Command at a binary that cannot exist on
// $PATH and asserts References maps the resulting exec.LookPath failure to
// ErrServerNotFoundSentinel, mirroring the equivalent
// //go:build integration subtest in refs_integration_test.go but without
// any dependency on gopls being installed.
func TestReferences_NonExistentServerBinaryYieldsErrServerNotFound(t *testing.T) {
	dir := t.TempDir()
	reg := Registry{
		"go": {
			Markers:     []string{"go.mod"},
			Match:       "any",
			Command:     []string{"lyx-scout-nonexistent-binary-xyz"},
			InstallHint: "this binary is intentionally fake for the test",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := References(ctx, Options{
		Registry:  reg,
		TargetDir: dir,
		Lang:      "go",
		Query:     Query{Symbol: "Resolve"},
		Timeout:   5 * time.Second,
	})
	if !errors.Is(err, ErrServerNotFoundSentinel) {
		t.Errorf("References() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
	}
}
