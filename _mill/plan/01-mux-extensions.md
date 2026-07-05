# Batch: mux-extensions

```yaml
task: 'Build internal/shuttle: one LLM agent via a swappable engine'
batch: mux-extensions
number: 1
cards: 4
verify: go test ./internal/muxengine/...
depends-on: []
```

## Batch Scope

Extend `internal/muxengine` with the three provider-agnostic additions shuttle needs
(`AddSpec.SessionID`, pane text/key transport, pane capture) and remove the dead `claude:`
config key. This is one batch because every card touches the same package and shares its
test fixtures. External interface consumed by batch 4 (run-loop): `AddSpec.SessionID`,
`(*Engine).SendText`, `(*Engine).SendKey`, `(*Engine).CapturePane`. mux stays a dumb
carrier throughout: no new field or op reads caller data semantically.

## Cards

### Card 1: AddSpec.SessionID persisted into Strand

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/doc.go`
- **Edits:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `SessionID string` to `AddSpec` (with a doc comment stating it is
  opaque caller metadata mux never reads — mirroring the `Strand.SessionID` comment in
  `state.go`). In `addStrandLocked`, stamp `SessionID: spec.SessionID` into the appended
  `Strand`. Extend `TestAddStrandLocked_HiddenAdd_GuidUniqueRecordStoredNoLaunch` (or add a
  sibling test) to assert a spec-supplied SessionID round-trips into the stored record and
  survives `SaveState`/`LoadState`.
- **Commit:** `feat(mux): persist caller-supplied SessionID via AddSpec`

### Card 2: remove the dead claude config key

- **Context:**
  - `internal/configsync/configsync.go`
  - `internal/configcli/reconcile_test.go`
- **Edits:**
  - `internal/muxengine/template.yaml`
  - `internal/muxengine/config.go`
  - `internal/muxengine/config_test.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `claude:` line from `template.yaml`, the `Claude string`
  field from `Config` (`config.go`), and the `cfg.Claude` empty-default assertion in
  `TestLoadConfig_TemplateDefaultsResolve` (`config_test.go:53-54`). Nothing else in the
  repo reads `Config.Claude` (verified by grep during planning — only `muxpoccli` uses its
  own separate `ClaudePath`). Confirm `internal/configsync` reconcile drops a stale
  `claude:` key from an existing user `mux.yaml`: if `configsync_test.go` does not already
  cover the removed-key case, add one test exercising reconcile of a user file containing a
  key absent from the template.
- **Commit:** `refactor(mux): drop unused claude config key (moves to shuttle.yaml)`

### Card 3: SendText / SendKey engine ops

- **Context:**
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/io.go`
  - `internal/muxengine/io_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file `io.go` with a file-header comment (repo style). Implement:
  (a) unexported pure helper `resolveLivePaneID(st *MuxState, guid string) (string, error)`
  — errors on unknown guid, `hidden` anchor, or empty `PaneID` (message names the guid);
  (b) `func (e *Engine) SendText(guid, text string, submit bool) error` — `withOpLock` +
  `requireSessionLocked` + `loadOrInitStateLocked` + `resolveLivePaneID`, then
  `e.psmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(text))` and, when
  `submit`, a separate `e.psmux.run("send-keys", "-t", paneID, "Enter")` (the exact
  two-step pattern of `launchStrandLocked`); (c) `func (e *Engine) SendKey(guid, key
  string) error` — same lock/lookup, then `send-keys -t <pane> <key>` WITHOUT `-l` (named
  key: `Enter`, `Escape`). Neither op reconciles, re-renders, or persists — they are pure
  transport. Hermetic tests cover `resolveLivePaneID` for all three error cases plus the
  happy path; no test calls the psmux round trip (same discipline as
  `reconcileApplyPersistLocked`'s note).
- **Commit:** `feat(mux): SendText/SendKey pane-transport engine ops`

### Card 4: CapturePane engine op

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/lifecycle.go`
- **Edits:**
  - `internal/muxengine/io.go`
  - `internal/muxengine/io_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (e *Engine) CapturePane(guid string) (string, error)` to
  `io.go`: `withOpLock` + `requireSessionLocked` + `loadOrInitStateLocked` +
  `resolveLivePaneID`, then return `e.psmux.output("capture-pane", "-p", "-t", paneID)`.
  Read-only discipline like `Status`: no reconcile, no layout apply, no persist — document
  this in the method godoc (a query must not move focus or mutate state). Hermetic tests:
  the error paths via `resolveLivePaneID` are already covered by card 3; add a godoc-level
  compile assertion only if a test is warranted — otherwise extend `io_test.go` with a
  table entry confirming the same resolver is used (no psmux round trip in tests).
- **Commit:** `feat(mux): CapturePane read-only engine op`

## Batch Tests

`verify: go test ./internal/muxengine/...` — runs the package's hermetic suite including the
new `io_test.go`, the SessionID round-trip test, and the config-template tests updated by
card 2 (plus `internal/configsync` is exercised by the top-level `go test ./...` overview
verify). The psmux round trips inside SendText/SendKey/CapturePane are deliberately not
unit-tested (repo discipline: hermetic tests never cross the psmux seam); they are proven
live by batch 6's smoke tests, which drive Interrupt/Send/startup-probe end-to-end.
