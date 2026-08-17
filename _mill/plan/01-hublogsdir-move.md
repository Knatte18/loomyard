# Batch: hublogsdir-move

```yaml
task: "shuttleengine + reedengine + tokenvocab told-geometry"
batch: "hublogsdir-move"
number: 1
cards: 3
verify: go test ./internal/fabricengine/... ./internal/reedengine/... ./cmd/lyx/... && go vet -tags smoke ./internal/reedcli/...
depends-on: []
```

## Batch Scope

This batch moves `HubLogsDir` out of `internal/reedengine` and into `internal/fabricengine`, where its base `HubScratchDir` already lives.
It is one batch because the move is a self-contained precondition for the rest of the task: `reedengine.Geometry.LogsDir` (batch 3) must be told a value that some other package computes, and `internal/fabricengine` must leave `reedengine`'s production import set for the Treadle Runner-Seam Invariant reword to become true.
It is a root batch, parallel-safe with batch 2 — the two share no file.

The external interface batch 3 consumes is `fabricengine.HubLogsDir(hubPath string) string`, which `internal/hubgeom` calls to populate `Geometry.LogsDir`.

Batch-local decision beyond `## Shared Decisions`: `internal/reedengine/lifecycle.go` still reads `e.layout.HubPath` after this batch — the `*lyxcwd.Location` field does not leave the `Engine` here, only the derivation moves.
`internal/fabricengine` therefore stays in reed's import set until batch 3 replaces the call with `e.geom.LogsDir`.

## Cards

### Card 1: Add `fabricengine.HubLogsDir`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/reedengine/lifecycle.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/hubscratch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `HubLogsDir(hubPath string) string` to `internal/fabricengine/junctionnames.go`, placed immediately after the existing `HubScratchDir`, returning `filepath.Join(HubScratchDir(hubPath), "logs")`.
  Its doc comment must state that it is the hub-level directory where the shared per-hub reed server writes its runtime log, that it is hub-anchored so one server per hub resolves to one deterministic place, and that it lives beside `HubScratchDir` so the derivation is named once rather than re-joined at each caller.
  Carry over the substance of the doc comment currently on `reedengine.HubLogsDir` in `internal/reedengine/lifecycle.go` rather than writing a fresh one.
  In `internal/fabricengine/hubscratch_test.go`, add a new test asserting `fabricengine.HubLogsDir(hub) == filepath.Join(fabricengine.HubScratchDir(hub), "logs")` for a synthetic hub path.
  Do not delete `reedengine.HubLogsDir` in this card; card 2 removes it together with every caller.
- **Commit:** `feat(fabricengine): add HubLogsDir beside HubScratchDir`

### Card 2: Delete `reedengine.HubLogsDir` and retarget every caller

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/hubforge/hub.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/fabricengine/hubscratch_test.go`
  - `internal/reedcli/smoke_debuglog_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the exported `HubLogsDir(l *lyxcwd.Location) string` function from `internal/reedengine/lifecycle.go` and retarget every caller in the same commit, so the tree compiles at this card's boundary.
  In `internal/reedengine/lifecycle.go`, replace the `logsDir := HubLogsDir(e.layout)` assignment in the server-boot path with `logsDir := fabricengine.HubLogsDir(e.layout.HubPath)`, and drop the now-unused `github.com/Knatte18/loomyard/internal/lyxcwd` import from that file — the file's remaining `e.layout` reads go through the `Engine` field, not the package.
  Keep the `github.com/Knatte18/loomyard/internal/fabricengine` import; it is now the source of `HubLogsDir`.
  Every surrounding `logger.Warn` / `logger.Info` call in that boot path stays exactly as it is.
  In `internal/fabricengine/hubscratch_test.go`, rewrite `TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` to call `fabricengine.HubLogsDir(hubPath)` instead of `reedengine.HubLogsDir(l)`, drop the now-unused `l := &lyxcwd.Location{HubPath: hubPath}` local, and drop the `reedengine` and `lyxcwd` imports if nothing else in the file still needs them.
  Rename the test to `TestHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` and update the file's header comment and the test's own doc comment, which both attribute the function to reed.
  If dropping `reedengine` leaves the file with no reason to be an external test package, still leave it as `package fabricengine_test` — the header comment explains that choice and changing it is out of scope.
  In `internal/reedcli/smoke_debuglog_test.go`, replace both `reedengine.HubLogsDir(h.Location)` calls with `fabricengine.HubLogsDir(h.Location.HubPath)` and fix the import block accordingly.
  In `cmd/lyx/constructoranchoring_test.go`, replace both `assertPath(t, "reedengine.HubLogsDir", reedengine.HubLogsDir(l), ...)` rows with `assertPath(t, "fabricengine.HubLogsDir", fabricengine.HubLogsDir(l.HubPath), ...)`, keeping the expected value `filepath.Join(hub, "_board", ".lyx", "logs")` byte-identical in both rows.
  Update the file header comment and the section comment above the second row, which both name `reedengine.HubLogsDir`.
  Fix that file's import block: add `internal/fabricengine`, and drop `internal/reedengine` only if no other row in the file still calls into it.
- **Commit:** `refactor(reedengine): move HubLogsDir to fabricengine`

### Card 3: Correct the design doc's Location-consumption row for `HubLogsDir`

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `manifest/designs/producers-standalone.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Location-consumption table, the `internal/reedengine` row attributes `HubLogsDir` to `lifecycle.go`.
  Edit that row's `Sites` cell so it no longer names `HubLogsDir` as a `reedengine` site, and note in the same cell that the hub-logs derivation now lives in `fabricengine.HubLogsDir(hubPath string)`.
  Keep the cell on one physical line, per this repo's markdown convention that table cells stay on one line.
  The row's `What it reads from the Location` cell keeps `HubPath` — `lock.go`'s socket-name derivation still reads it at this point in the task.
  Change nothing else in this file; the T6 and T7 pointer edits are card 10's, and T3's own Files/Verify lines are deliberately left as they are.
- **Commit:** `docs(producers-standalone): correct HubLogsDir attribution in the Location table`

## Batch Tests

`verify:` runs the untagged suites of `internal/fabricengine`, `internal/reedengine` and `cmd/lyx`, then type-checks the `//go:build smoke` tier of `internal/reedcli` with `go vet -tags smoke`.

- `internal/fabricengine/hubscratch_test.go` — the new `HubLogsDir(hub) == Join(HubScratchDir(hub), "logs")` assertion from card 1, and the retargeted `MkdirAll` idempotency test from card 2.
- `cmd/lyx/constructoranchoring_test.go` — both `HubLogsDir` rows must keep asserting the identical expected path in the unanchored and the subpath-anchored fixtures.
  That is the whole point of the rows: the value does not move, only the package that computes it.
  The subpath-anchored row also keeps proving the value ignores `AnchorRel` entirely.
- `internal/reedengine/...` — no reed test asserts `HubLogsDir` directly, so this is a regression gate on the boot path compiling and the package's untagged suite staying green after the import change.
- `internal/reedcli/smoke_debuglog_test.go` is `//go:build smoke`, so it is compile-checked rather than run, per the shared decision on tagged tiers.
  The only breakage card 2 can introduce there is a call-site signature mismatch, which `go vet -tags smoke` catches.
