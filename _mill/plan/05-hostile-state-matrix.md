# Batch: hostile-state-matrix

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'hostile-state-matrix'
number: 5
cards: 2
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource && go test ./internal/lyxcwd/ -run TestEnforcement
depends-on: [3]
```

## Batch Scope

This batch delivers the ten named hostile states and the suite proving each one actually establishes what it claims.
It is one batch because the states are one table with one shape, and because the self-assertion suite is the single most important guard in the whole harness: a `dirtyWarpTracked` state that silently failed to dirty anything would make every cell using it vacuous while reading like coverage.

The external interface batch 6 consumes is the `State` type and the exported state table.

Batch-local decision: each state's `Apply` asserts its own establishment inline, via `tb.Fatalf`, **in addition to** card 15's suite.
The inline check is what protects a cell running months from now;
the suite is what protects the state builders during development.
Neither substitutes for the other.

## Cards

### Card 14: the ten hostile states

- **Context:**
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
  - `internal/fslink/fslink_windows.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/dirtiness.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/states.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line.
  Define `State` as `struct { Name string; Apply func(tb testing.TB, h *Hub, target StateTarget) }`, where `StateTarget` carries the paths the cell resolved for the verb under test — at minimum the warp checkout to dirty, the weft checkout to dirty, and the fabric-owned path (or wired junction path) a structural state should plant at.
  This is what makes the **dirty-what-per-cell rule** expressible: a dirtiness state dirties **the checkout the verb under test actually acts on**, and a structural state plants at **the path the verb under test actually acts on**, both resolved per cell rather than fixed by the state.
  A tranche-1 hub has at least four checkouts (prime warp, prime weft sibling, the board, and each added pair's warp worktree plus weft sibling) and the gate probes a different one per verb, so a state that dirties a checkout the verb never touches asserts nothing.
  The ten states, each with a doc comment naming the campaign defect it was born from:
  `clean` (no-op, the tautology-guard baseline);
  `dirtyWarpTracked` (modify a tracked file in the target warp checkout — R2);
  `dirtyWarpUntracked` (write a new untracked file into the target warp checkout — R3);
  `dirtyWeftTracked` (modify a tracked file in the target weft checkout);
  `dirtyWeftUntracked` (write a new untracked file into the target weft checkout — the state that makes the dirtiness-scope parameter observable at all, since `Checkout` probes `worktreeDirty(scopeTracked, …)` at `checkout.go:42` and must proceed while `Remove`'s `refuseDirtyWeftWorktree` probes `scopeAll` at `remove.go:79`/`:144` and must refuse);
  `bothDirty` (the warp and weft dirtiness applied together);
  `trackedSymlinkAtWiredPath` (replace a wired junction path with a link the operator owns — R1);
  `foreignDirAtFabricOwnedPath` (an ordinary operator directory parked at a fabric-owned path — R4);
  `unrelatedGitCloneAtWeftNamedPath` (a complete, independently `git init`-ed repository with a committed file and a clean tree parked at a weft-named path — R4's second shape);
  `staleWiredJunction` (a wired junction whose raw target no longer resolves — R1's repair-side counterpart).
  Every dirtiness state plants a **named sentinel** — a file whose name identifies the state and the cell — so a failure message reads "the operator's uncommitted file is gone" rather than "a path list changed".
  Every state's `Apply` ends by asserting its own establishment: a dirtiness state asserts `GitStatusPorcelain` on the target checkout is non-empty and names the sentinel;
  a link state asserts `fslink.IsLink` is true and `fslink.RawTarget` is the planted value;
  a foreign-directory state asserts the planted content is on disk.
  Every link is created through `fslink.CreateDirLink` and inspected through `fslink.IsLink`/`fslink.RawTarget` — never `os.Symlink`, `os.Readlink`, or `os.Lstat` mode bits.
  `trackedSymlinkAtWiredPath` models a git-tracked symlink, which on Windows needs `core.symlinks=true` plus Developer Mode;
  building it through `fslink.CreateDirLink` materialises it as a junction there, and the gate's `ownedWiredJunction` check compares `fslink.RawTarget`, so the assertion keeps its shape even though the on-disk artifact differs.
  Record that divergence in a comment here and confirm `doc.go` already states it — do not hide it behind a `runtime.GOOS` skip.
  Tearing down and replanting these shapes needs `os.Remove`/`fslink.Remove`;
  that is legal in this package because batch 1 excluded the directory from the destructive-bypass guard, and the exclusion is what makes the state builders reachable by `fabricengine_test` consumers instead of stranded inside a test binary.
  Export the ten states as an ordered slice so the driver iterates a stable order and failure names are reproducible.
- **Commit:** `fabrictest: add the ten named hostile states`

### Card 15: state self-assertion suite

- **Context:**
  - `internal/fabricengine/fabrictest/states.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_linux.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/states_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, `package fabrictest`, one `t.Run` per state, each `t.Parallel()` on its own hub from `NewHub`, run for both anchors.
  For each state: build a hub, resolve a `StateTarget` against it, apply the state, and assert **directly** that the claimed condition now holds — not by driving a verb, which would confound the state's establishment with the verb's behaviour.
  Dirtiness states: `GitStatusPorcelain` on the target checkout is non-empty and contains the sentinel, and the tracked/untracked distinction is right (a tracked-dirty state shows a modification, an untracked-dirty state shows a `??` entry, and each must **not** show the other's shape — otherwise `dirtyWeftUntracked` could silently satisfy a `scopeTracked` probe and the whole `Proceeds` derivation would collapse).
  `trackedSymlinkAtWiredPath`: the path is a link, its raw one-hop target is the planted value, and it is git-tracked in the warp checkout.
  `staleWiredJunction`: the path is a link and its raw target does not resolve on disk.
  `foreignDirAtFabricOwnedPath`: the planted directory and its content file exist and the path is not a git checkout.
  `unrelatedGitCloneAtWeftNamedPath`: the planted path is a real repository with at least one commit and a clean working tree — the clean tree matters, because it means the dirty gate has nothing to say even in principle and only the ownership gate stands between `prune --apply` and the whole clone.
  `clean`: asserts the hub is genuinely clean — every checkout's `GitStatusPorcelain` is empty — which is what makes the clean-state effect assertions in batch 6 meaningful.
  Add one manifest-interaction assertion per structural state: applying the state and re-capturing produces a diff against the pre-state manifest, so a state that silently no-ops cannot pass this suite by accident.
- **Commit:** `fabrictest: prove every hostile state establishes what it claims`

## Batch Tests

`verify:` runs card 15's suite via `go test -tags integration ./internal/fabricengine/fabrictest/`, which is the batch's substantive gate.
`go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource` is required here specifically: `states.go` is the first non-test file in the package to carry `os.Remove(` and `fslink.Remove(` — two of the eight banned bypass tokens — so if batch 1's directory exclusion were wrong or incomplete, this is the batch where it fails, and it must fail here rather than at whichever later batch happens to run `cmd/lyx`.
`go test ./internal/lyxcwd/ -run TestEnforcement` covers the geometry and vocabulary rules across the new file, which names warp, weft and every fabric-owned path.
