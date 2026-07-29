# Batch: wire-ensure-server-into-refs

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: wire-ensure-server-into-refs
number: 7
cards: 2
verify: go test -count=1 ./internal/codeintelengine/...
depends-on: [5, 6]
```

## Batch Scope

Wires the finished `EnsureServer` machinery into `References` — the
`ensure-server-call-site` decision. `References`'s public signature and
behavior for the four non-Go languages are byte-for-byte unchanged
(`TestReferences_NonExistentServerBinaryYieldsErrServerNotFound`, the
existing regression test, is the proof: it exercises a
`HasNativeDaemon: false` entry through the public `References` function
and must keep passing with zero edits). Go now goes through
`ensureServer` instead of a direct `newLSPClient` + `initialize` call.

This is the single point every later batch builds on: this batch factors
`References`'s body into a shared `lookup` pipeline (detect → acquire
connection → resolve position → make one LSP call → convert results)
parameterized by which LSP call to make, specifically so batch 8's
`Definition` can become a second, near-trivial wrapper over the exact
same pipeline instead of duplicating it — `Definition` differs from
`References` only in which LSP method it calls once a position is
resolved, per `_mill/discussion.md`'s `definition-semantics` decision,
so the shared plumbing belongs here, not copy-pasted in batch 8. The CLI
batches (9, 10) never see any of this — they only call
`References`/`Definition`/`Symbol`, unchanged from their perspective.

## Cards

### Card 26: Split `References` into connection-acquisition + teardown + the LSP calls

- **Context:**
  - `internal/codeintelengine/errors.go`
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/refs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `ensureserver.go`, extend card 17's `connKind`
  const block with a third value, `connKindLegacy` — the type as card 17
  left it only has `connKindNative`/`connKindSupervised`, but
  `acquireConnection`/`teardownConnection` (below) need a value for the
  zero-valued/cold-spawn-per-call path, which never goes through
  `ensureServer` at all. Give it its own doc comment: "the legacy path's
  kind — never produced by `ensureServer` itself (that function is only
  ever called when `entry.HasNativeDaemon` is true); `acquireConnection`
  returns this directly for the `false` case without calling
  `ensureServer`." In `refs.go`, add `WorktreeRoot string` to `Options`
  (after `TargetDir`), documented as: "the worktree root `EnsureServer`'s
  `supervised` strategy would anchor its daemon singleton at; unused by
  every strategy actually reachable in V1 (`native` never reads it), but
  threaded through now so a future language selecting `supervised` needs
  no signature change. The CLI layer populates it from a resolved
  `hubgeometry.Layout.WorktreeRoot` when available, empty otherwise." Add
  three new unexported functions. **`acquireConnection(ctx
  context.Context, lang string, entry Entry, opts Options) (*lspClient,
  connKind, error)`**: if `entry.HasNativeDaemon`, return
  `ensureServer(ctx, lang, entry, opts.TargetDir, opts.WorktreeRoot,
  opts.Timeout)` directly (its three-value return already matches this
  function's own signature). Otherwise, run **exactly** today's
  pre-this-task `References` spawn+initialize sequence, moved here
  verbatim in behavior: `newLSPClient(entry.Command)`, translating
  `errors.Is(err, exec.ErrNotFound)` into `*ErrServerNotFound` as today;
  build `rootURI` via `rootURIFor(opts.TargetDir)` (the helper batch 5's
  card 19 extracted — use it here too, do not re-inline
  `filepath.Abs`+concat a second time); run `client.initialize` under its
  own `context.WithTimeout(ctx, opts.Timeout)`; on an initialize error,
  call `client.kill()` when `errors.Is(err, ErrServerTimeoutSentinel)`
  else `client.close()` (mirroring the existing `timedOut`-branching
  teardown logic exactly, just localized to this one failure instead of
  spanning the whole call), and return `connKindLegacy` alongside the
  error; on success return the client, `connKindLegacy`, `nil`.
  **`teardownConnection(client *lspClient, kind connKind, timedOut
  bool)`**: `switch kind { case connKindSupervised:` do nothing at all —
  return immediately, with a comment citing the overview's connKind
  teardown Shared Decision (never run the LSP shutdown handshake or kill
  a daemon-owned dial; the process exit reclaims the fd); `default:`
  (covers both `connKindNative` and `connKindLegacy`, which share
  identical teardown) `if timedOut { client.kill() } else {
  client.close() }` — exactly today's existing logic, now expressed as a
  named function instead of an inline closure. **`lookup(ctx
  context.Context, opts Options, lspCall func(ctx context.Context,
  client *lspClient, fileURI string, pos lspPosition) ([]lspLocation,
  error)) ([]Reference, error)`** — the shared pipeline batch 8's
  `Definition` will also call: `DetectLanguage`; `acquireConnection`
  (return immediately on its error, no deferred teardown needed for the
  same reason as today — `acquireConnection` already tears down any
  partial connection itself); declare `timedOut := false` and `defer
  func() { teardownConnection(client, kind, timedOut) }()` (capturing
  `timedOut` by reference via the closure, exactly as the pre-this-task
  code's own closure did, since `resolvePosition`/`lspCall` below may
  still set it to `true` after the defer is registered); call
  `resolvePosition(ctx, client, opts, lang, entry)`, setting `timedOut =
  true` on an `ErrServerTimeoutSentinel`-matching error and returning it;
  run `lspCall` under its own `context.WithTimeout(ctx, opts.Timeout)`,
  same timeout-detection-and-return pattern; return
  `toSortedReferences(locations), nil` on success. Rewrite `References`
  itself down to a two-line wrapper: `return lookup(ctx, opts, func(ctx
  context.Context, client *lspClient, fileURI string, pos lspPosition)
  ([]lspLocation, error) { return client.references(ctx, fileURI, pos)
  })`. Delete the old inline `client, err := newLSPClient(entry.Command)`
  block, the old inline `absTargetDir`/`rootURI` construction, and the
  old inline `resolvePosition`/`references`/teardown sequence from
  `References` itself — all of it now lives in `acquireConnection` and
  `lookup`. `resolvePosition` and `toSortedReferences` themselves are
  **unchanged** — `lookup` calls them exactly as `References` did before
  this batch.
- **Commit:** `refactor(codeintelengine): extract the lookup pipeline and route it through EnsureServer for daemon-strategy languages`

### Card 27: Test the `HasNativeDaemon` dispatch branch and the legacy-path regression

- **Context:**
  - `internal/codeintelengine/toolchain.go`
  - `internal/codeintelengine/ensureserver.go`
- **Edits:**
  - `internal/codeintelengine/refs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Do not modify
  `TestReferences_NonExistentServerBinaryYieldsErrServerNotFound` — its
  continued, unmodified pass is itself the proof that the
  `HasNativeDaemon: false` (legacy) branch is unaffected by this batch.
  Add a new test,
  `TestReferences_HasNativeDaemonRoutesThroughEnsureServer`, proving the
  **true** branch is actually taken without spawning a real `gopls`: swap
  `userCacheDir` (the seam batch 2's card 8 added to `toolchain.go`) to a
  `t.TempDir()` for the test's duration (`t.Cleanup` restoration) and
  swap `installGoToolchain` to a fake that always returns a distinct,
  recognizable error (e.g. `errFakeInstallRefused :=
  errors.New("fake install refused")`); call `References(ctx, Options{
  Registry: Registry{"go": {Command: []string{"gopls"}, PinnedVersion:
  "v0.0.0-test", HasNativeDaemon: true}}, TargetDir: t.TempDir(), Lang:
  "go", Query: Query{Symbol: "X"}, Timeout: 5*time.Second })` and assert
  the returned error wraps `errFakeInstallRefused` (via
  `errors.Is`) — proving `References` actually called into
  `ensureServer` → `ensureNative` → `resolveGoToolchain` → the fake
  installer, and did **not** take the legacy `newLSPClient(entry.Command)`
  path (which would instead fail with `ErrServerNotFoundSentinel` from a
  literal, unresolved `"gopls"` lookup — a categorically different error
  this assertion distinguishes from). Add a short doc comment on the new
  test naming exactly what it proves and what it deliberately does not
  (it is not a proof that a real `gopls` connection works end to end —
  that is `ensureserver_integration_test.go`, batch 5).
- **Commit:** `test(codeintelengine): cover the HasNativeDaemon dispatch branch in References`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...`. This is
the batch where a regression in the legacy path would be most visible —
`refs_test.go`'s existing test, `load_test.go`, `detect_test.go`,
`registry_test.go`, and `position_test.go` all continue running
unmodified and must all stay green, since none of their behavior is
supposed to change.
</content>
