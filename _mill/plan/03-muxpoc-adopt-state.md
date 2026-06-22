# Batch: muxpoc-adopt-state

```yaml
task: "Adopt internal/state in board and muxpoc"
batch: "muxpoc-adopt-state"
number: 3
cards: 3
verify: go test ./internal/muxpoc/...
depends-on: [1]
```

## Batch Scope

Route muxpoc's state persistence through `internal/state` and stop swallowing corruption.
`SaveState` and `LoadState` adopt `state.WriteJSON` / `state.ReadJSON`, which moves the
lock file from `.lyx/muxpoc-state.lock` to `.lyx/muxpoc-state.json.lock` (Save and Load
move together so the fence stays intact; no external referrers to the old name exist).
`LoadState`'s vestigial `warn string` return is removed — it was non-empty only for the
corruption case, which is now an error — so the signature becomes
`(*MuxpocState, error)`, and all six non-test callers plus the smoke test are updated.
Independent of the board batch (disjoint files); both depend on batch 1.

## Cards

### Card 4: Adopt state in SaveState and LoadState; drop the warn channel

- **Context:**
  - `internal/state/state.go`
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/muxpoc/state.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Compute `statePath := filepath.Join(cwd, stateRelPath)` in both functions. Use
    `statePath + ".lock"` as the lock path in both (Save and Load must match).
  - `SaveState`: keep the `if s == nil { return fmt.Errorf("cannot save nil state") }`
    guard. Replace the manual `MkdirAll` + `flock.AcquireWriteLock(lockPath)` + manual
    `json.MarshalIndent` + `fsx.AtomicWrite(cwd, stateRelPath, ...)` body with
    `return state.WriteJSON(statePath, statePath+".lock", s)`.
  - `LoadState`: change the signature from
    `func LoadState(cwd string) (*MuxpocState, string, error)` to
    `func LoadState(cwd string) (*MuxpocState, error)`. Implement as
    `v, found, err := state.ReadJSON[MuxpocState](statePath, statePath+".lock")`; on
    `err != nil` return `nil, err` (a corrupt file now surfaces as an error rather than a
    warning string); on `!found` return `nil, nil`; otherwise return `&v, nil`.
  - Remove the now-unused `lockRelPath` constant. Remove imports that are no longer
    referenced (`encoding/json`, `github.com/Knatte18/loomyard/internal/fsx`, the `flock`
    alias) and add the `internal/state` import. Keep `os` (still used by `DeleteState`),
    `crypto/rand`, `regexp`, `strings`, `fmt`, `path/filepath`. Verify each remaining
    import is still referenced in the final file.
  - Leave `DeleteState`, `newSessionID`, `sanitizeEnv`, `strippedEnvKeys`, `socketName`,
    `stateRelPath`, and the `Pane` / `MuxpocState` structs unchanged.
- **Commit:** `refactor(muxpoc): persist state via internal/state, drop warn return`

### Card 5: Update LoadState callers for the new signature

- **Context:**
  - `internal/muxpoc/state.go`
- **Edits:**
  - `internal/muxpoc/attach.go`
  - `internal/muxpoc/daemon.go`
  - `internal/muxpoc/down.go`
  - `internal/muxpoc/review.go`
  - `internal/muxpoc/status.go`
  - `internal/muxpoc/up.go`
  - `internal/muxpoc/muxpoc_smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - Update every `LoadState(cwd)` call from the three-value form to the two-value form:
    `attach.go`, `down.go`, `review.go`, `status.go` currently use `state, _, err :=` → make
    them `state, err :=`. `muxpoc_smoke_test.go` uses `_, _, err = LoadState(...)` → make it
    `_, err = LoadState(...)`.
  - `daemon.go`: change `state, warn, err := LoadState(cwd)` to `state, err := LoadState(cwd)`
    and delete the `if warn != "" { fmt.Fprintf(os.Stderr, "%s\n", warn) }` block. The
    existing `if err != nil { ...; continue }` branch now also covers the corrupt case
    (log and keep polling). Verify `os` is still imported/used in the file after the change.
  - `up.go`: change `state, warn, err := LoadState(cwd)` to `state, err := LoadState(cwd)`
    and delete the `if warn != "" { fmt.Fprintln(os.Stderr, warn) }` block. The existing
    `if err != nil { return output.Err(...) }` now aborts on a corrupt state file instead
    of starting a fresh session. Verify `os` is still imported/used after the change.
  - Do not change any other behaviour in these files.
- **Commit:** `refactor(muxpoc): update LoadState callers to two-value signature`

### Card 6: Update muxpoc state tests for corruption-as-error

- **Context:**
  - `internal/muxpoc/state.go`
- **Edits:**
  - `internal/muxpoc/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `TestLoadStateCorrupt`: rewrite to call `state, err := LoadState(tmpDir)` and assert
    `err != nil` and `state == nil` (previously it asserted a non-empty warning and a nil
    error).
  - `TestLoadStateMissing`: update to `state, err := LoadState(tmpDir)`; still assert
    `state == nil` and `err == nil`. Remove the warning assertion.
  - `TestSaveLoadRoundtrip`: update to `loaded, err := LoadState(tmpDir)`; remove the
    warning assertion; keep the field-equality checks.
  - Update any other `LoadState` call in the test file to the two-value form.
- **Commit:** `test(muxpoc): assert LoadState errors on corrupt state`

## Batch Tests

`verify: go test ./internal/muxpoc/...` runs the `muxpoc` package tests, including
`state_test.go` (round-trip, missing, corrupt) and the smoke test, all against the new
two-value `LoadState` signature. Scope is the single package tree this batch touches.
