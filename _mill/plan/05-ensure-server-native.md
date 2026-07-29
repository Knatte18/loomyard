# Batch: ensure-server-native

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: ensure-server-native
number: 5
cards: 5
verify: go test -count=1 ./internal/codeintelengine/...
depends-on: [2, 4]
```

## Batch Scope

Builds the `native` strategy — the only `EnsureServer` path any V1
registry entry actually reaches (Go, `HasNativeDaemon: true`) — and the
`ensureServer` dispatcher itself. Creates `ensureserver.go`, the file
batch 6 extends with the `supervised` strategy's own code (not its own
dispatch arm — see the overview's "`EnsureServer` has exactly one live
dispatch arm in V1" Shared Decision).

The external interface batch 7 consumes:
`ensureServer(ctx context.Context, lang string, entry Entry, targetDir,
worktreeRoot string, timeout time.Duration) (*lspClient, connKind,
error)`. `worktreeRoot` is accepted but unused by the native branch in
this batch (native takes no state file/lock) — it exists on the
signature now so batch 6 does not need to change it.

Batch-local decision: the initialize-then-probe-then-kill-on-failure
sequence is factored into a helper, `finalizeConnection`, shared by
`native` (this batch) and `supervised` (batch 6) — both strategies do
exactly this once they have *a* connection, regardless of how they got
it (fresh spawn for native, fresh spawn or reused dial for supervised).
Factoring it out here is what makes it independently unit-testable
against the existing `lspclient_test.go` fake-transport harness, without
needing a real subprocess — `ensureNative`'s own spawn-and-resolve glue
is tested only at the integration level (card 21), where a real `gopls`
is available to validate the toolchain-resolution and argv-assembly
wiring end to end.

## Cards

### Card 17: `connKind` type and the `ensureServer` dispatcher

- **Context:**
  - `internal/codeintelengine/registry.go`
- **Creates:**
  - `internal/codeintelengine/ensureserver.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the file's header doc comment first: state that
  this file implements the `EnsureServer(lang, worktreeRoot) -> LSPConn`
  seam from `manifest/designs/codeintel-redesign.md`, that
  `ensureServer` is called only for a registry entry with
  `HasNativeDaemon == true`, and that in V1 this means Go only — every
  other language's caller keeps using `newLSPClient`/`client.initialize`
  directly, unchanged, and never calls into this file at all. Define
  `type connKind int` with `const ( connKindNative connKind = iota;
  connKindSupervised )`, each with a one-line doc comment stating the
  teardown rule the overview's Shared Decision assigns it (native: safe
  to `close()`/`kill()`, it is a disposable local proxy; supervised:
  never `close()`/`kill()` it, it is a dial into a daemon meant to
  outlive this call). Define
  `func ensureServer(ctx context.Context, lang string, entry Entry,
  targetDir, worktreeRoot string, timeout time.Duration) (*lspClient,
  connKind, error)`: body is `return ensureNative(ctx, lang, entry,
  targetDir, timeout)` returning `connKindNative` alongside it (i.e. the
  function wraps `ensureNative`'s two-value return into the three-value
  shape). Document above the function, prominently, exactly what the
  overview's "`EnsureServer` has exactly one live dispatch arm in V1"
  Shared Decision says: this has one branch today because no V1 registry
  entry ever requests `supervised`; `ensureSupervised` (batch 6) is
  proven by its own dedicated integration test, not through this
  dispatcher.
- **Commit:** `feat(codeintelengine): add ensureServer dispatcher and connKind`

### Card 18: `finalizeConnection` — shared initialize+probe+kill-on-failure sequence

- **Context:**
  - `internal/codeintelengine/probe.go`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func finalizeConnection(ctx context.Context, client *lspClient,
  rootURI string, timeout time.Duration) error`: run
  `client.initialize` under its own `context.WithTimeout(ctx, timeout)`
  (mirroring `References`'s existing per-phase timeout pattern in
  `refs.go`); on error, call `client.kill()` and return the error
  unchanged. On success, call `probe(ctx, client, timeout)`; on error,
  call `client.kill()` and return the error unchanged (wrapped with
  `fmt.Errorf("codeintelengine: probe failed: %w", err)` so a probe
  failure is distinguishable in logs from an initialize failure without
  needing `errors.Is` gymnastics — both still satisfy
  `errors.Is(err, ErrServerTimeoutSentinel)` when the underlying cause is
  a timeout, since `fmt.Errorf`'s `%w` preserves the chain). Return `nil`
  on success. This function performs **no restart/retry of its own** — a
  probe or initialize failure is reported once and torn down once; per
  `native-lifecycle-and-probe-failure`, `native`'s caller (`ensureNative`,
  card 19) never attempts a second spawn after this returns an error,
  since lyx has no supervisory authority over the shared daemon under
  `native`. (`supervised`'s restart-on-staleness, batch 6, is a
  *different* mechanism — it decides to restart *before* ever calling
  `finalizeConnection`, based on the state file's staleness check, not
  in response to a `finalizeConnection` failure.)
- **Commit:** `feat(codeintelengine): add finalizeConnection shared handshake+probe helper`

### Card 19: `ensureNative` — toolchain resolve, argv build, spawn, finalize

- **Context:**
  - `internal/codeintelengine/toolchain.go`
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/errors.go`
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func ensureNative(ctx context.Context, lang string, entry Entry,
  targetDir string, timeout time.Duration) (*lspClient, error)`: (1)
  `binPath, err := resolveGoToolchain(ctx, entry.PinnedVersion)`; wrap
  and return on error. (2) build
  `argv := append([]string{binPath}, entry.Command[1:]...)` then
  `argv = append(argv, "-remote=auto")` — `entry.Command[0]` (the literal
  string `"gopls"`) is **never** used; only `entry.Command[1:]` (empty
  for Go's current registry entry, but preserved for forward
  compatibility with a future entry that carries extra fixed args) is
  kept, per `toolchain-manager-authority`'s exact argv-composition
  decision. (3) `client, err := newLSPClient(argv)`; on
  `errors.Is(err, exec.ErrNotFound)` return
  `&ErrServerNotFound{Language: lang, InstallHint: entry.InstallHint}`
  (mirroring `References`'s existing translation of the same error
  today), any other spawn error wrapped and returned. (4) build
  `rootURI` from `targetDir` exactly as `References` does today
  (`filepath.Abs` then `"file://" + absTargetDir`) — do not
  duplicate that logic inline; extract it as a small unexported
  `rootURIFor(targetDir string) (string, error)` helper in this same
  file and have `References` (batch 7) switch to calling it too, so
  there is exactly one implementation of "path to rootURI" in the
  package. (5) call `finalizeConnection(ctx, client, rootURI, timeout)`;
  on error, return it (the client was already `kill()`'d by
  `finalizeConnection` itself — do not call `kill()` a second time,
  `lspClient.kill()` is idempotent via its `closed` guard but a second
  call is still pointless noise). (6) return the client, `nil`.
- **Commit:** `feat(codeintelengine): implement ensureNative`

### Card 20: Unit tests for `finalizeConnection`

- **Context:**
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/errors.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/ensureserver_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Untagged, offline — build every case on
  `newLSPClientFromRW` + `newPipeTransportPair`/`fakeServer` (this
  package's existing fake-transport harness from `lspclient_test.go`,
  reusable with no import since it's the same package). Cover: (1) a
  fake server that answers `initialize` successfully and then answers
  `workspace/symbol` with `[]` — `finalizeConnection` returns `nil`. (2)
  a fake server that answers `initialize` with an LSP error response —
  `finalizeConnection` returns a non-nil error and, whitebox-asserted via
  the unexported `client.closed` field (same-package test), the client
  is closed. (3) a fake server that answers `initialize` successfully but
  never answers the follow-up `workspace/symbol` probe request —
  `finalizeConnection` returns an error satisfying
  `errors.Is(err, ErrServerTimeoutSentinel)` once a short test timeout
  (e.g. 200ms passed as the `timeout` argument) expires, and
  `client.closed` is `true`. Name this file `ensureserver_test.go` since
  it is the natural home for every future `ensureServer`-adjacent unit
  test batch 6 adds (the dispatcher itself needs no dedicated test today
  — it has one unconditional branch, nothing to assert beyond what
  `ensureNative`'s own tests already cover transitively via the
  integration test, card 21).
- **Commit:** `test(codeintelengine): cover finalizeConnection success, initialize-failure, and probe-timeout paths`

### Card 21: Integration test — native toolchain resolve + wire-compat regression

- **Context:**
  - `internal/codeintelengine/refs_integration_test.go`
  - `internal/codeintelengine/toolchain_integration_test.go`
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/ensureserver_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`-tagged, mirroring
  `refs_integration_test.go`'s `t.Skip(builtins()["go"].InstallHint)`
  gate on `exec.LookPath("gopls")` — even though `ensureNative` itself
  ignores `$PATH` and resolves its own toolchain-managed binary, the
  skip gate here is about whether this **machine** can plausibly run a
  real `gopls` at all (network + `go install` capability), which
  `exec.LookPath` is a reasonable, cheap proxy for reusing rather than
  inventing a second capability probe. Two sub-tests. **(1)
  `ensureNative` end-to-end**: call `ensureNative(ctx, "go",
  builtins()["go"], repoRoot(t), 30*time.Second)` (reusing
  `refs_integration_test.go`'s `repoRoot` helper) and assert it returns a
  non-nil, non-`closed` client with no error — this is the first real
  proof the toolchain-resolve → argv-build → spawn → initialize → probe
  chain works end to end against a real, network-installed `gopls`, not
  just its individually-mocked pieces. **(2) shared-daemon regression**:
  port the exact empirical procedure `_mill/discussion.md`'s
  `native-strategy-wire-compatibility` decision already ran manually —
  spawn two independent `ensureNative` calls a moment apart (each in its
  own goroutine, `-remote=auto` under the hood), both rooted at
  `repoRoot(t)`. Give both a concrete, deterministic pass condition
  (portable — no `pgrep`): each connection issues `workspace/symbol` for
  the literal query `"Resolve"` (the same well-known, unique,
  package-level `hubgeometry.Resolve` symbol `refs_integration_test.go`'s
  own `findFuncPosition` helper already locates for its own test); assert
  both connections return **at least one** candidate, and that the
  first candidate's `formatLocation` (or equivalent file:line:col string)
  is **identical** across both connections — two connections attached to
  the same shared daemon's index resolve the same query to the same
  location; two independent, unconnected `gopls` instances would still
  each resolve it correctly in isolation (so a merely-nonempty result on
  each is not, by itself, discriminating), but only a genuinely shared
  index guarantees this exact byte-for-byte match on every run, which is
  what makes this assertion a real regression pin rather than a vague
  smoke check. The test's own doc comment should state plainly that this
  codifies the discussion's manual verification as automated regression
  coverage, not first-time proof (per this
  decision's own "Rejected: none... the shipped code needs its own
  automated proof, not a one-off manual check").
- **Commit:** `test(codeintelengine): add ensureNative integration coverage`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...` (no
`-tags integration`, so card 21 is excluded from this gate and runs
separately alongside the package's other integration tests). Cards 17–19
introduce no behavior any existing test exercises (nothing calls
`ensureServer`/`ensureNative` yet outside this batch's own new tests —
that wiring is batch 7), so no pre-existing test in this package should
change.
</content>
