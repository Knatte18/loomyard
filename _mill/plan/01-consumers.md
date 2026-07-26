# Batch: A -- consumers

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: A -- consumers
number: 1
cards: 8
verify: go test -tags integration ./internal/initengine/... ./internal/loomengine/... ./internal/buildercli/... ./internal/webstercli/... ./internal/perchcli/...
depends-on: []
```

## Batch Scope

Rewire the six production consumers that import `warpengine`/`weftengine` onto
`fabricengine`, and rewrite the two consumer integration tests that reference the old
engines. This batch is independent of B and C (it only swaps in-package imports/calls; the
old modules still exist and compile). No external interface is produced; batch D1 (delete)
depends on this batch having removed every production import of the old engines from these
files. Batch-local decision: the four structurally different `weftengine.Commit` rewrites
follow the shared "never discard `New`'s error" and "signature gotchas" decisions in the
overview.

## Cards

### Card 1: rewire initengine/init.go WireJunctions

- **Context:**
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/initengine/init.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `warpengine.WireJunctions(l, slug)` call with
  `fabricengine.WireJunctions(l, slug)` (identity signature per the Shared Decisions
  "signature gotchas" mapping). Swap the
  `github.com/Knatte18/loomyard/internal/warpengine` import for
  `github.com/Knatte18/loomyard/internal/fabricengine`. Remove the `warpengine` import only
  if no other reference to it remains in this file. In the same commit, sweep this file's
  comment that names the deleted module -- the step comment "Wiring the host _lyx junction
  via warpengine.WireJunctions" -> `fabricengine.WireJunctions` (per the tree-wide
  comment-sweep Shared Decision). Do not change any other call.
- **Commit:** `refactor(initengine): rewire init.go WireJunctions onto fabricengine`

### Card 2: rewire initengine/undo.go (Unwire + CommitWeft/PushWeftAt)

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/initengine/undo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewire four call sites: (1) `warpengine.UnwireJunctions` ->
  `fabricengine.UnwireJunctions` (identity, returns `UnwireResult`); (2)
  `weftengine.EnvSyncOptions()` -> `fabricengine.EnvSyncOptions()` (identity); (3)
  `weftengine.Commit(weftWorktree, ScopedPathspec(...), msg, opts)` -> construct
  `f, err := fabricengine.New(hostPath, weftWorktree)`, check `err` (return/propagate on
  non-nil, never discard), then `f.CommitWeft(fabricengine.ScopedPathspec(...), msg, opts)`
  whose return is `(sha, committed, err)` -- discard `sha` if the caller only needs
  `committed`; (4) `weftengine.Push(weftWorktree, opts)` ->
  `fabricengine.PushWeftAt(weftWorktree, opts)`. Confirm the host worktree root is in scope
  at the `New` call (it is the primary worktree root; use the same value `initengine`
  already resolves for its geometry). Swap the `warpengine`/`weftengine` imports for
  `fabricengine`. Mirror the proven pattern in `internal/fabriccli/weft_verbs.go` (mappings
  per the Shared Decisions "signature gotchas"). In the same commit, sweep this file's
  comments that name the deleted modules -- "(see warpengine.UnwireJunctions)", "via
  warpengine.UnwireJunctions. Any error ...", "commit and push that deletion through
  weftengine.", and "weftengine.Commit must never be called ..." -> the fabricengine
  equivalents (per the tree-wide comment-sweep Shared Decision).
- **Commit:** `refactor(initengine): rewire undo.go onto fabricengine CommitWeft/PushWeftAt`

### Card 3: rewrite initengine/init_test.go onto fabric

- **Context:**
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/template_test.go`
- **Edits:**
  - `internal/initengine/init_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The test imports both old engines, loops over
  `["board","warp","weft"]`, and calls `warpengine.LoadConfig(root,"warp")` /
  `weftengine.LoadConfig(root)`. Rewrite to loop `["board","fabric"]` and call
  `fabricengine.LoadConfig(root)` (one-arg form). Drop the `warpengine`/`weftengine` imports
  in favour of `fabricengine`. Preserve every assertion's intent; `lyx init` now writes only
  `fabricengine.yaml` (plus board), so the per-module config-existence checks target
  `fabric`, not separate `warp`/`weft`.
- **Commit:** `test(initengine): rewrite init_test.go module loop onto fabric`

### Card 4: rewire loomengine/preflight.go (HostClean/PairInSync)

- **Context:**
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/loomengine/preflight.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `warpengine.HostClean(l)` -> `fabricengine.HostClean(l)` and
  `warpengine.PairInSync(l)` -> `fabricengine.PairInSync(l)` (both identity signatures per the
  Shared Decisions "signature gotchas"). Swap the `warpengine` import for `fabricengine`. In
  the same commit, sweep any comment in this file that names the deleted module (repoint to
  `fabricengine` per the tree-wide comment-sweep Shared Decision). No other call change.
- **Commit:** `refactor(loomengine): rewire preflight.go onto fabricengine`

### Card 5: rewrite loomengine/preflight_integration_test.go fixture

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/add.go`
- **Edits:**
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The test builds its fixture via `warpengine.WireJunctions`. Replace with
  `fabricengine.WireJunctions` (identity) and swap the `warpengine` import for
  `fabricengine`. If the fixture also constructs topology via `warpengine.New(cfg)`, replace
  with `fabricengine.NewTopology(cfg)` (`*Topology`, defined in `topology.go`) and adjust the
  receiver-method calls (`Add`/etc., see `add.go`) accordingly. In the same commit, sweep any
  comment in this file that names the deleted module (per the tree-wide comment-sweep Shared
  Decision). Preserve every assertion.
- **Commit:** `test(loomengine): rewrite preflight fixture onto fabricengine`

### Card 6: rewire buildercli/weft.go

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/buildercli/weft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewire the weft-commit path: `ScopedPathspec` / `EnvSyncOptions` ->
  `fabricengine.*` (identity); `weftengine.Commit(...)` -> `f, err :=
  fabricengine.New(hostPath, weftWorktree)` (check `err`) then `f.CommitWeft(...)` (discard
  the extra `sha` return if unused); `weftengine.Push(...)` ->
  `fabricengine.PushWeftAt(...)`. Confirm the host worktree root is in scope for `New`. Swap
  `weftengine` import for `fabricengine`. Follow the `weft_verbs.go` pattern (mappings per the
  Shared Decisions "signature gotchas"). In the same commit, sweep any comment in this file
  that names the deleted module (per the tree-wide comment-sweep Shared Decision).
- **Commit:** `refactor(buildercli): rewire weft.go onto fabricengine CommitWeft/PushWeftAt`

### Card 7: rewire webstercli/weft.go

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/webstercli/weft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same shape as card 6: identity swaps for `ScopedPathspec`/
  `EnvSyncOptions`; `weftengine.Commit(...)` -> `fabricengine.New(host,weft)` (check `err`)
  then `CommitWeft(...)`; `weftengine.Push(...)` -> `fabricengine.PushWeftAt(...)`. Swap the
  `weftengine` import for `fabricengine`. Confirm host worktree root in scope for `New`. In
  the same commit, sweep any comment in this file that names the deleted module (per the
  tree-wide comment-sweep Shared Decision).
- **Commit:** `refactor(webstercli): rewire weft.go onto fabricengine CommitWeft/PushWeftAt`

### Card 8: rewire perchcli/run.go

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/perchcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Same shape as cards 6/7 at the four weft call sites: identity swaps for
  `ScopedPathspec`/`EnvSyncOptions`; `weftengine.Commit(...)` ->
  `fabricengine.New(host,weft)` (check `err`) then `CommitWeft(...)`; `weftengine.Push(...)`
  -> `fabricengine.PushWeftAt(...)`. Swap the `weftengine` import for `fabricengine`. Confirm
  the host worktree root is in scope for `New` (perchcli's standalone run owns the loop
  boundary; use the worktree root it already resolves). In the same commit, sweep any comment
  in this file that names the deleted module (per the tree-wide comment-sweep Shared
  Decision). Leave the `t.Parallel()`/test-concurrency wording untouched -- that is not a
  module reference.
- **Commit:** `refactor(perchcli): rewire run.go onto fabricengine CommitWeft/PushWeftAt`

## Batch Tests

`verify` runs the integration suites for exactly the five touched modules:
`internal/initengine` (init.go, undo.go, init_test.go), `internal/loomengine` (preflight.go,
preflight_integration_test.go), `internal/buildercli`, `internal/webstercli`, and
`internal/perchcli`. These cover the rewired weft-commit/junction paths. The old modules
still exist, so nothing else in the tree is affected by this batch. The `-tags integration`
flag is required because the weft-commit paths spawn git.
