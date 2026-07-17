# Batch: lsp-client-and-refs

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: lsp-client-and-refs
number: 2
cards: 5
verify: go test ./internal/codeintelengine/...
depends-on: [1]
```

## Batch Scope

Delivers the generalized stdio LSP client (adapted from the recovered `gopls.go` harness) and the
`References(...)` orchestration that ties detection → registry → server-launch → LSP query together.
This is the subprocess-bearing half of the engine. The external interface batch 3 consumes:
`References(ctx, Options) ([]Reference, error)` plus the `Reference`, `Options`, and `Query` types.

Recover the reference implementation with
`git show 3b4dcf86:tools/codeintel-poc/gopls.go` — its `lspClient`, `writeMessage`/`readMessage`,
`call`/`notify`, `references`, `toLSPPosition`/`utf16Length`, and `close` are ~90% reusable. The two
Go couplings to strip: the hardcoded `exec.LookPath("gopls")` (→ registry `Command`) and the
`loadPackages`+`resolveSymbol` symbol→position step (→ `workspace/symbol`, or a caller-supplied
`file:line:col`).

Batch-local decisions: (a) the client is generalized to take a launch `command []string` rather than
assuming `gopls`; (b) every request carries a `context.Context`; a deadline expiry cancels the
in-flight request and **hard-kills** the subprocess; (c) the client captures the server's
`initialize` capabilities so `refs.go` can tell whether `workspace/symbol` is supported. Live-server
tests are `//go:build integration`-tagged; the in-memory framing test is untagged.

## Cards

### Card 8: LSP position conversion helpers

- **Context:**
  - `internal/codeintelengine/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/position.go`
  - `internal/codeintelengine/position_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port `toLSPPosition(pos)` and `utf16Length(s)` from the recovered `gopls.go`, but
  decouple from `go/token`: define `type Position struct { File string; Line int; Character int }`
  (1-based line, 1-based byte column, as parsed from a `file:line:col` CLI arg) and a converter
  `toLSPPosition(p Position) (lspPosition, error)` that re-reads the source line and computes the
  0-based UTF-16 code-unit character offset (the byte-column vs UTF-16 mismatch on non-ASCII lines is
  the subtlety #008 already solved — keep it). Keep `utf16Length` verbatim (stdlib `unicode/utf16`).
  Add a formatter turning an `lspLocation` back into a `file:line:col` string (1-based, for display).
  `position_test.go` (untagged): assert UTF-16 conversion on an ASCII line and on a line with a
  multi-byte rune before the target column; assert the round-trip formatter. Import stdlib only.
- **Commit:** `feat(codeintelengine): language-agnostic LSP position conversion`

### Card 9: generalized stdio LSP client

- **Context:**
  - `internal/codeintelengine/position.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/lspclient.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port the recovered `gopls.go` client, generalized. Define the wire types
  (`lspPosition`, `lspRange`, `lspLocation`, `lspError`, `lspMessage`, plus a
  `symbolInformation`/`workspaceSymbol` result shape with a `Location`). Define
  `type lspClient struct` holding the `*exec.Cmd`, stdin writer, buffered stdout reader, and next-id
  counter. `func newLSPClient(command []string) (*lspClient, error)` runs `command[0]` with
  `command[1:]` args (replacing the hardcoded `gopls` lookup); resolve `command[0]` via
  `exec.LookPath` and, when absent, return `ErrServerNotFound` carrying the language + install hint
  (the caller supplies these). Keep `writeMessage`/`readMessage` (Content-Length CRLF framing),
  `call`/`notify`, and the server-initiated-request handling (answer `client/registerCapability` /
  `workspace/configuration` with an empty result so the server never blocks) verbatim. Thread a
  `context.Context` through `call`: select on `ctx.Done()` while awaiting a response, returning
  `ErrServerTimeout` (naming the current phase) on deadline. Add `initialize(ctx, rootURI)` that
  sends the `initialize` request and **retains the returned `capabilities`** (at least
  `workspaceSymbolProvider` presence) on the client, then sends `initialized`. Add
  `references(ctx, fileURI, pos)` (with `includeDeclaration: true`) and
  `workspaceSymbol(ctx, query)` returning candidate locations. Add `close()` (graceful
  `shutdown`/`exit`, best-effort, for the normal end of a run) and `kill()` (`cmd.Process.Kill()` +
  `Wait`, for the timeout path). Add a `supportsWorkspaceSymbol() bool` accessor. **Factor the I/O
  over an injectable transport for testability:** the `lspClient` holds a writer + buffered reader
  (and, when spawned, the `*exec.Cmd`); provide the production constructor
  `newLSPClient(command []string)` (spawns the subprocess, wires its stdio) **and** an unexported seam
  constructor `newLSPClientFromRW(rwc io.ReadWriteCloser) *lspClient` that builds a client over a
  caller-supplied transport with **no** subprocess — Card 10 drives this seam. `kill()` guards on a
  nil `*exec.Cmd` (no-op / closes the transport) so the pipe-backed client is safe to tear down.
  Import stdlib only (`bufio`, `bytes`, `context`, `encoding/json`, `fmt`, `io`, `os`, `os/exec`,
  `sort`, `strconv`, `strings`, `time`). No `internal/output`.
- **Commit:** `feat(codeintelengine): generalized stdio LSP client`

### Card 10: LSP client framing unit test (fake server)

- **Context:**
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/position.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/lspclient_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Untagged, spawn-free. Exercise the framing/protocol logic **without launching a
  real subprocess** by building the client via Card 9's `newLSPClientFromRW(rwc)` seam over an
  `io.Pipe`-backed transport — a scripted fake server goroutine writes Content-Length-framed
  responses. Assert: the `initialize` handshake round-trips and the capabilities are captured
  (`supportsWorkspaceSymbol` reflects the scripted response); a server-initiated
  `client/registerCapability` request receives an empty-result reply; a `textDocument/references` call
  sends `includeDeclaration: true` and parses the location list; a `context` whose deadline is already
  exceeded yields `ErrServerTimeout` (assert via `errors.Is`). Do NOT use `exec.Command` in this file
  (keep it untagged; a real `os/exec` launch belongs in Card 12's integration test). The transport
  seam already exists from Card 9 — this card writes only the test.
- **Commit:** `test(codeintelengine): LSP framing tests against an in-memory server`

### Card 11: References orchestration + public types

- **Context:**
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/position.go`
  - `internal/codeintelengine/detect.go`
  - `internal/codeintelengine/registry.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/refs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Define public types: `type Reference struct { File string; Line int; Character int }`;
  `type Query struct { Symbol string; Pos *Position }` (exactly one set — `Symbol` for the name form,
  `Pos` for the `file:line:col` form); `type Options struct { Registry Registry; TargetDir string;
  Lang string; Query Query; Timeout time.Duration }`. Add
  `func References(ctx context.Context, opts Options) ([]Reference, error)`: (1) `DetectLanguage(opts.TargetDir,
  opts.Registry, opts.Lang)` → language + Entry (propagate `ErrNoLanguage`); (2) derive a per-call
  deadline `context.WithTimeout(ctx, opts.Timeout)`; (3) `newLSPClient(entry.Command)` (propagate
  `ErrServerNotFound` with the entry's `InstallHint`); (4) `defer` teardown — on a timeout/`ErrServerTimeout`
  path call `kill()`, otherwise `close()`; (5) `initialize(ctx, rootURI)` with `rootURI = "file://" +
  absolute(opts.TargetDir)`; (6) resolve the position: if `Query.Pos != nil` use it directly; else if
  the server `supportsWorkspaceSymbol()` call `workspaceSymbol(ctx, Query.Symbol)` and branch — zero
  candidates → `ErrSymbolNotFound`, multiple → `ErrAmbiguousSymbol` with each candidate's
  `file:line:col`, exactly one → its position; if the server lacks the capability → `ErrResolverUnsupported`;
  (7) `references(ctx, fileURI, lspPos)`; (8) map results to `[]Reference` sorted by file:line:col.
  Wrap the phase name into `ErrServerTimeout` at each LSP step. Import stdlib + the sibling files'
  identifiers only. No `internal/output`, no cobra.
- **Commit:** `feat(codeintelengine): References orchestration across detect, registry, and LSP`

### Card 12: live-server integration test (gopls)

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/registry.go`
  - `internal/lyxtest/hermetic.go`
  - `docs/research/codeintel-spike.md`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/refs_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First line `//go:build integration`. Guard the whole test on
  `exec.LookPath("gopls")` — `t.Skip` with the install hint when absent (so the suite passes on a
  machine without gopls). Point `References` at this repo's own root (a Go target) with
  `Lang: "go"` and `builtins()`, querying a known high-fan-in symbol by `file:line:col` (e.g. a
  `hubgeometry` exported function — resolve its position from source, matching #008's approach), and
  assert the returned reference set is non-empty and contains the declaration site. Add a second case
  asserting `ErrServerNotFound` is returned when the registry `Command` names a non-existent binary.
  Because this test may spawn git indirectly through lyxtest helpers, add a `TestMain` calling
  `lyxtest.HermeticGitEnv()` if any helper is used; if the test only spawns gopls (no git), no
  `TestMain` is required — but confirm against the Hermetic Git Test Environment Invariant. This test
  is `integration`-tagged so it does NOT run under the batch's plain `go test` verify.
- **Commit:** `test(codeintelengine): live gopls integration test for References`

## Batch Tests

`verify: go test ./internal/codeintelengine/...` runs the untagged Cards 8 and 10 tests (position
conversion, LSP framing against the in-memory fake) plus batch 1's tests. Card 12's live-gopls test is
`//go:build integration`-tagged and excluded from the plain verify, keeping tier-1 offline and
spawn-free per the Test Tier Purity Invariant; it is exercised separately with `-tags integration` on
a machine with `gopls` installed (batch 4 installs it).
