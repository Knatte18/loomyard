# Batch: muxengine-operations

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'muxengine-operations'
number: 5
cards: 5
verify: go test ./internal/muxengine/...
depends-on: [3, 4]
```

## Batch Scope

Composes the batch-4 carrier primitives into the engine's public operations: strand
mutation (`AddStrand`/`UpdateStrand`/`RemoveStrand`), reconcile-against-live-panes, the
single mux operation lock, `ApplyLayout` (render -> `select-layout`/`select-pane`), and the
lifecycle ops (`Up`/`Resume`/`Down`/`Status`). This is the seam shuttle/loom and `muxcli`
(batch 6) call. External interface: an `Engine` type (holding resolved `Config`, `Layout`
geometry, and a `PsmuxCmd`) with exported methods `AddStrand`, `UpdateStrand`, `RemoveStrand`,
`Up`, `Resume`, `Down`, `Status`, each returning `(resultStruct, error)` — no cobra, no
io.Writer, no exit codes (engine-purity litmus).

Batch-local decisions (from the discussion, load-bearing):
- **Single-layer lock:** each public engine op acquires `.lyx/mux.lock` **once at entry**
  (via `internal/lock.AcquireWriteLock` on `layout.DotLyxDir()/mux.lock`) and holds it for
  the whole `read->mutate->persist->render->apply` cycle, composing from unexported
  **unlocked** helpers (`addStrandLocked`, `reconcileLocked`, `applyLayoutLocked`, ...).
  Public ops never call each other while holding the lock. CLI verbs (batch 6) never lock.
- **Reconcile keeps records:** a dead/absent pane -> clear that strand's `PaneID` + mark
  not-live, **keep the record** (only `RemoveStrand` deletes). Kill the physical
  `pane_dead=1` pane before re-applying the layout, **except** a sole-remaining dead pane.
  Order under the lock: kill dead -> re-enumerate live -> compute layout -> apply.
- **`up` never launches a strand command; `resume` is the only replayer.** `resume` skips
  `hidden` strands (`not-live AND anchor != hidden`).
- **Add-time hidden rule:** `anchor: hidden` at add registers a record with **no pane** and
  does **not** run `cmd`; `UpdateStrand` may flip `hidden -> visible` (create pane + run
  `cmd`) but **rejects** `visible -> hidden`.

## Cards

### Card 16: Engine type + single mux operation lock

- **Context:**
  - `internal/lock/lock.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/lock.go`
  - `internal/muxengine/lock_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/lock.go`, define the engine value: `type Engine
  struct { cfg Config; layout *hubgeometry.Layout; psmux PsmuxCmd }` and a constructor `func
  New(cfg Config, layout *hubgeometry.Layout) *Engine` (builds the `PsmuxCmd` from
  `cfg.Psmux` + `socketName(layout.Hub)`). Add the lock helper `func (e *Engine)
  withOpLock(fn func() error) error` that acquires `lock.AcquireWriteLock(filepath.Join(
  e.layout.DotLyxDir(), "mux.lock"))` **once**, `defer`s `Release()`, and runs `fn` — this
  is the ONLY acquisition point; every public op wraps its body in `withOpLock` and calls
  unexported `*Locked` helpers that assume the lock is held. Document the non-reentrancy
  rule (gofrs/flock across handles) and the outer(mux.lock)->inner(state's mux.json.lock)
  ordering in a comment. In `lock_test.go`: assert the lock path is under `.lyx/`
  (per-worktree); assert two `withOpLock` calls serialize (second blocks until first
  releases) using a fixture `.lyx` dir; assert a released lock can be re-acquired (no stale
  lock — handle auto-released on `Release`).
- **Commit:** `feat(muxengine): Engine type and single-layer mux operation lock`

### Card 17: strand mutation — AddStrand / UpdateStrand / RemoveStrand

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/name.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/lock.go`
- **Edits:**
  - `internal/muxengine/lock.go`
- **Creates:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/strand_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/strand.go`, add the mutation ops on `*Engine`,
  each acquiring `withOpLock` and delegating to an unexported `*Locked` helper that also
  runs reconcile+apply (card 18/19 provide those; here call the helpers). `func (e *Engine)
  AddStrand(spec AddSpec) (Strand, error)`: generate `newGUID`; validate `spec.Parent` names
  an existing strand (reject unknown); reject a parent link that would form a **cycle**;
  build the `Strand` (opaque `Cmd`/`ResumeCmd`, `Display` from `spec`); if `anchor ==
  hidden`, register the record with **no pane** and do **not** run `cmd` (defer to surface);
  otherwise the pane creation/launch happens in the apply/lifecycle path. Define `type
  AddSpec struct { Name, Worktree, Parent, Cmd, ResumeCmd string; Display render.Display }`.
  `func (e *Engine) UpdateStrand(guid string, display render.Display) (Strand, error)`:
  mutate by guid; **reject** a `visible -> hidden` transition (error `cannot hide a live
  strand in v1`); allow `hidden -> visible` (mark for surface: create pane + run `cmd` in
  apply). `func (e *Engine) RemoveStrand(guid string) (Removed, error)`: **always cascade**
  — remove the strand and its whole descendant subtree; return `type Removed struct {
  Strands []struct{ GUID, Name string } }` listing every removed strand. In `strand_test.go`
  (drive the unlocked helpers or the public ops against a fixture `.lyx`): assert guid is
  generated + unique; unknown/cyclic parent rejected; hidden-add stores a record with empty
  `PaneID` and unrun `cmd`; `UpdateStrand` visible->hidden rejected, hidden->visible allowed;
  `RemoveStrand` cascades and the result lists every removed strand.
- **Commit:** `feat(muxengine): AddStrand/UpdateStrand/RemoveStrand with cascade and hidden rules`

### Card 18: reconcile against live panes

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/parse.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/reconcile_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/reconcile.go`, add the unexported `func (e
  *Engine) reconcileLocked(st *MuxState, live []LivePane) (killed []string, err error)`
  (pure planning + kill I/O; assumes the op lock held). For each strand: if its `PaneID` is
  absent from `live` or the matching `LivePane.Dead == true`, **clear the `PaneID` and mark
  not-live but keep the record**; live panes keep/re-derive their id. To keep the layout
  string consistent with psmux's live pane set, **kill each `pane_dead=1` pane** (via
  `e.psmux.run("kill-pane", "-t", id)`) **before** re-applying the layout — **except** a
  **sole-remaining** dead pane (killing it would end the session), which is kept and
  rendered until `resume`/`remove`. Return the killed ids. Provide a pure inner helper `func
  planReconcile(strands []Strand, live []LivePane) (clearedGUIDs []string, deadToKill
  []string, solePane string)` so the decision logic is unit-testable without psmux. In
  `reconcile_test.go`, table-test `planReconcile` with saved tables + fake `list-panes`
  results incl. `pane_dead=1` rows: dead strand's record survives with cleared `PaneID`;
  live strands keep ids; dead panes are scheduled for kill except a sole-remaining one; only
  `RemoveStrand` (card 17) ever deletes a record.
- **Commit:** `feat(muxengine): reconcile clears dead pane bindings, keeps records, kills dead-except-sole`

### Card 19: ApplyLayout — render to select-layout/select-pane (+ debounce)

- **Context:**
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/apply.go`
  - `internal/muxengine/apply_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/apply.go`, add the unexported `func (e *Engine)
  applyLayoutLocked(st *MuxState, live []LivePane) error` (op lock held): build the live-id
  set, map `toRenderStrands`, call `render.Rules(strands, render.Box{0,0,cfg.Width,
  cfg.Height}, render.Params{TopBandRows, CollapsedStripRows, MinFullRows})`, then
  `e.psmux.run("select-layout", "-t", session, layout)` and `e.psmux.run("select-pane",
  "-t", focusTarget)` (skip when <2 panes, mirroring muxpoc's `applyColumnLayout` guard).
  Order inside the caller (reconcile then apply): kill dead -> re-enumerate live -> compute
  layout -> apply. Add a pure planning helper `func (e *Engine) planLayout(st *MuxState,
  live []LivePane) (layout, focus string, err error)` so the "what layout string would be
  applied" is testable without psmux. Debounce is an **in-process driver** concern only:
  document that a burst of in-process mutations coalesces into one `applyLayoutLocked` per
  op (a one-shot CLI verb is a single mutation -> single apply, nothing to debounce). In
  `apply_test.go`, assert `planLayout` produces the expected layout string + focus target
  for a canonical strand table (reuse render golden expectations) with no live psmux.
- **Commit:** `feat(muxengine): ApplyLayout renders and applies select-layout/select-pane`

### Card 20: lifecycle — Up / Resume / Down / Status

- **Context:**
  - `internal/muxpoccli/up.go`
  - `internal/muxpoccli/down.go`
  - `internal/muxpoccli/status.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/env.go`
  - `internal/muxengine/naming.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/apply.go`
  - `internal/proc/proc_windows.go`
  - `internal/muxengine/lock.go`
- **Edits:**
  - `internal/muxengine/lock.go`
- **Creates:**
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/lifecycle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/lifecycle.go`, add the public lifecycle ops on
  `*Engine`, each wrapping `withOpLock`. `func (e *Engine) Up() (UpResult, error)`: ensure
  the named server (spawned raw: `exec.Command(cfg.Psmux, "-L", socket, "new-session", "-d",
  "-s", session, "-x", W, "-y", H, cfg.Pwsh)`, `cmd.Env` from `CleanClaudeEnv(os.Environ())`,
  `proc.Detach(cmd)`, `cmd.Start()`, then `set-option -g remain-on-exit on`, poll
  `hasSession`) + this worktree's session **exist**; reconcile; apply the layout from the
  current table; **run no strand command**. `func (e *Engine) Resume() (ResumeResult,
  error)`: boot server+session if absent; for each strand that is **not live AND anchor !=
  hidden**, recreate its pane (`split-window -P -F "#{pane_id}"`, or adopt the new-session
  pane for the first) and `send-keys` its `ResumeCmd` (or `Cmd` if empty) + `"Enter"`;
  **skip hidden strands**; leave already-live strands untouched; reconcile; apply; re-persist
  pane ids. `func (e *Engine) Down() (DownResult, error)`: `kill-server` + delete
  `.lyx/mux.json` (idempotent, ignore not-exist). `func (e *Engine) Status() (StatusResult,
  error)`: reconcile against live panes and report **this session only** (tracked strands +
  live/dead) — no stray-server enumeration (deferred). In `lifecycle_test.go`, drive the
  planning seams that do not need a live psmux (assert `Up` plans no strand launch; `Resume`
  plans a replay for each not-live non-hidden strand and skips hidden; the three states —
  server dead / server-up-CLI-restarted / single pane died — at the "what commands would
  run" seam). Guard any real-psmux round-trip behind `//go:build smoke`.
- **Commit:** `feat(muxengine): Up/Resume/Down/Status lifecycle ops (up=substrate, resume=replay)`

## Batch Tests

`verify: go test ./internal/muxengine/...` runs the whole engine package: lock serialization
+ per-worktree scope + no-stale-lock, strand mutation (guid/cascade/hidden/cycle rules),
`planReconcile`, `planLayout`, and the lifecycle planning seams. All live psmux I/O
(`new-session`, `send-keys`, `select-layout`) is confined to `//go:build smoke` tests so the
default `verify:` stays hermetic and fast. Concurrency is asserted via `withOpLock`
serialization on a fixture `.lyx` dir (real gofrs/flock, no psmux).
