# Batch: muxpoc-swap

```yaml
task: "Extract internal/fsx and build internal/state"
batch: "muxpoc-swap"
number: 3
cards: 1
verify: go test ./internal/muxpoc/...
depends-on: [1]
```

## Batch Scope

Remove `internal/muxpoc`'s cross-module reach into `internal/board` by swapping its single
`board.AtomicWrite` call for `fsx.AtomicWrite` (identical signature). Minimal and
behaviour-preserving: `muxpoc`'s `LoadState`/`SaveState`/`DeleteState` logic and its `internal/lock`
usage are untouched. Independent of batch 2 (disjoint files), so it may run in parallel with it.
Depends on batch 1 for the fsx symbol.

## Cards

### Card 7: Swap muxpoc to fsx.AtomicWrite

- **Context:**
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/muxpoc/state.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/muxpoc/state.go`, `SaveState` calls
  `board.AtomicWrite(cwd, stateRelPath, string(content))` at line 108. Change it to
  `fsx.AtomicWrite(cwd, stateRelPath, string(content))`. In the import block, replace
  `"github.com/Knatte18/loomyard/internal/board"` with `"github.com/Knatte18/loomyard/internal/fsx"`.
  Leave the `flock "github.com/Knatte18/loomyard/internal/lock"` alias and all other imports and logic
  unchanged. Confirm `board` is no longer referenced anywhere else in the file (it is not — line 108
  is the only use).
- **Commit:** `refactor(muxpoc): use fsx.AtomicWrite instead of board`

## Batch Tests

`verify: go test ./internal/muxpoc/...` runs muxpoc's suite (`state_test.go`,
`muxpoc_smoke_test.go`, etc.). The state round-trip and smoke tests confirm the atomic-write swap is
behaviour-preserving. Scope is the single muxpoc package.
