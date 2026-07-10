# Batch: mux-contract-and-godoc

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "mux-contract-and-godoc"
number: 3
cards: 2
verify: go vet -tags integration ./internal/muxengine/... && go test ./internal/muxengine/...
depends-on: []
```

## Batch Scope

This batch pins the psmux/tmux contract surface two ways: as **godoc** in
`internal/muxengine/doc.go` (the canonical module-doc home, since the standalone mux module
doc was already deleted per the Documentation Lifecycle) and as a **`//go:build integration`
Go test** that spawns a real server via the *configured* binary and asserts the exact wire
contract. The test is the canary for both version drift and the eventual tmux swap: the same
test runs against psmux on Windows today and tmux on Linux in the follow-up, skipping cleanly
when the binary is absent. It complements — does not replace — the existing agent-driven
`SANDBOX-MUX-SUITE`.

This batch only edits `doc.go` (comment-only) and creates a new test file; it shares no source
file with batches 1/2, so it is a root batch that runs in parallel with them. It documents the
contract surface independently of batch 2's probe (the probe checks a subset at boot; this
batch asserts the full surface).

## Cards

### Card 9: Contract-surface godoc in doc.go

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/parse.go`
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/apply.go`
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a "Multiplexer contract surface" section to the package godoc in
  `doc.go` (keep the existing package-doc prose). Document, as prose the reader can verify
  against a real binary: (1) the six `#{pane_*}` format vars the engine parses —
  `#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}` (the exact
  `list-panes -F` string at `overlay.go:110`, parsed by `parsePaneList` in `parse.go`, with
  `pane_dead == "1"` keying a dead pane); (2) the subcommand set the engine depends on
  (`new-session`, `split-window`, `select-layout`, `select-pane`, `send-keys`, `capture-pane`,
  `list-panes`, `list-sessions`, `display-message`, `set-option -g remain-on-exit`, `kill-pane`,
  `kill-server`); and (3) each load-bearing behavioral assumption with a one-line rationale —
  silent split failure (`spawn.go`), dead-pane adoption via `remain-on-exit` (`spawn.go`),
  the `-l` leading-dash send-keys bug handled by `sendKeysLiteralArg` (`spawn.go`), empty-layout
  session destruction guarded by `anyPlacedStrand` (`apply.go`), and async kill-server /
  probe-always-exits-0 (`lifecycle.go`). This is a comment-only edit; add no code.
- **Commit:** `docs(muxengine): document the psmux/tmux contract surface in godoc`

### Card 10: Integration contract test

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/parse.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/apply.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/contract_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `contract_integration_test.go` with a `//go:build integration` tag,
  package `muxengine`. `TestMultiplexerContract` loads config via the package's `LoadConfig`
  (so it targets the *configured* binary — psmux on Windows, tmux on Linux), and if the binary
  is absent (`exec.LookPath` on `cfg.Psmux` fails), `t.Skip` with a clear message. Otherwise it
  spawns a real server on a scratch `-L` socket and asserts: (a) the exact
  `list-panes -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"`
  output shape and its `parsePaneList` parse; (b) the required subcommand set is exercised
  (`new-session`, `split-window`, `select-layout`, `select-pane`, `send-keys -l`, `capture-pane`,
  `list-panes`, `list-sessions`, `display-message`, `set-option -g remain-on-exit`, `kill-pane`,
  `kill-server`); and (c) each behavioral assumption — `remain-on-exit` keeps a dead pane
  visible with `pane_dead=1`, `send-keys -l` handles a leading-dash literal, and `select-layout`
  succeeds against the live pane set. Always tear the scratch server down (`kill-server`) in a
  `t.Cleanup`. Keep the test hermetic to its own socket so it cannot collide with a real hub
  server.
- **Commit:** `test(muxengine): add //go:build integration multiplexer contract test`

## Batch Tests

`verify` runs `go vet -tags integration ./internal/muxengine/...` — which **compiles** the new
`//go:build integration` contract test (catching any API-drift compile error) without executing
its live-server body — then `go test ./internal/muxengine/...` for the normal (untagged) suite.
The live contract assertions run only under `-tags integration` with a real binary present and
are the canary for the tmux swap; executing them against real tmux is the deferred follow-up.
The godoc edit is compile-trivial. Scope is the single `muxengine` package.
