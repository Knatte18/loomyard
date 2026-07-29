# Batch: lspclient-dial-transport

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: lspclient-dial-transport
number: 3
cards: 2
verify: go test -count=1 ./internal/codeintelengine/...
depends-on: []
```

## Batch Scope

Adds a second `lspClient` transport mode — dialing an existing Unix socket
or TCP address — alongside the package's existing spawn-and-use-stdio-pipes
mode. This is `supervised`-only machinery (batch 6 dials the daemon it
spawned via `gopls serve -listen=unix;<path>`); `native` never dials a
recorded address and is unaffected by this batch.

The external interface batch 6 consumes:
`newLSPClientDial(ctx context.Context, network, address string)
(*lspClient, error)`.

Batch-local decision: the dial constructor is a thin wrapper around
`net.Dialer.DialContext` + the **existing** `newLSPClientFromRW` seam —
`net.Conn` already satisfies `io.ReadWriteCloser`, so no new framing,
readLoop, or protocol code is needed here at all. This keeps the diff to
one small function and reuses every piece of `newLSPClientFromRW`'s
already-tested behavior (the persistent `readLoop` goroutine, the
server-initiated-request auto-answer logic) unchanged.

## Cards

### Card 10: `newLSPClientDial` — dial an existing Unix socket or TCP address

- **Context:**
  - `internal/codeintelengine/refs.go`
- **Edits:**
  - `internal/codeintelengine/lspclient.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func newLSPClientDial(ctx context.Context, network, address string)
  (*lspClient, error)`, placed immediately after `newLSPClientFromRW`
  (same section of the file, since this is its production dial-based
  sibling). Body: `var d net.Dialer; conn, err := d.DialContext(ctx,
  network, address); if err != nil { return nil, fmt.Errorf("codeintelengine:
  dial lsp server at %s %s: %w", network, address, err) }; return
  newLSPClientFromRW(conn), nil`. Add `"net"` to the file's import block.
  `network`/`address` are passed through verbatim to `net.Dialer.DialContext`
  — this function has no opinion on Unix-vs-TCP; the caller (batch 6)
  decides based on the daemon's recorded state-file address. Document on
  the function that, unlike `newLSPClient`, the returned client's `cmd`
  field is `nil` (no subprocess was spawned — this is a network dial to
  an already-running, externally-owned process), so `close()`'s
  `if c.cmd != nil { c.cmd.Wait() }` branch is already a no-op for a
  dialed client — this is a note for the reader, not a code change,
  since `close()` (lspclient.go) already guards on `c.cmd == nil`
  correctly.
- **Commit:** `feat(codeintelengine): add newLSPClientDial for the supervised dial-transport mode`

### Card 11: Test the dial-transport mode over a real Unix-domain listener

- **Context:**
  - `internal/codeintelengine/lspclient.go`
- **Edits:**
  - `internal/codeintelengine/lspclient_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new test,
  `TestLSPClient_DialTransport_InitializeOverUnixSocket`, that spins up a
  real `net.Listen("unix", filepath.Join(t.TempDir(), "test.sock"))`
  (Unix-only; skip cleanly via `if runtime.GOOS == "windows" {
  t.Skip("unix sockets not exercised on windows here; TCP is covered by
  the address-form flexibility of newLSPClientDial itself") }` — Windows
  supports AF_UNIX too, but this repo's CI convention elsewhere skips
  platform-specific socket tests rather than adding a second listener
  code path; state this explicitly as a scoping choice, not an oversight),
  accepts one connection in a goroutine, wraps the server side with the
  existing `newFakeServer` helper (from this file), and drives the exact
  same initialize-handshake script `TestLSPClient_InitializeCapturesCapabilities`
  already uses — but through `newLSPClientDial(ctx, "unix", socketPath)`
  instead of `newLSPClientFromRW(pipeTransport{...})`. Assert
  `initialize()` succeeds and `supportsWorkspaceSymbol()` reflects the
  fake server's advertised capability, proving the dial path produces a
  client that behaves identically to the pipe-backed one for the same
  wire traffic — this is the point of the test: dial-transport is not a
  new protocol implementation, only a new way to obtain the
  `io.ReadWriteCloser` `newLSPClientFromRW` already knows how to drive.
- **Commit:** `test(codeintelengine): cover newLSPClientDial over a real unix socket`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...`. No
integration tag needed — a `net.Listen("unix", ...)` test fixture is a
local, hermetic listener, not an external process spawn, so it stays in
Tier 1 per the Test Tier Purity Invariant (which bans `exec.Command`/
`gitexec.RunGit`/fixture-tree copies in untagged files, not local
socket listeners).
</content>
