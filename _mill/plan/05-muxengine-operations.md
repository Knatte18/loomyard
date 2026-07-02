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

Composes the batch-4 carrier primitives into the engine's public operations: reconcile-
against-live-panes, `ApplyLayout` (render -> `select-layout`/`select-pane`), strand mutation
(`AddStrand`/`UpdateStrand`/`RemoveStrand`), and the lifecycle ops (`Up`/`Resume`/`Down`/
`Status`). This is the seam shuttle/loom and `muxcli` (batch 6) call. External interface: an
`Engine` type (holding resolved `Config`, `Layout` geometry, and a `PsmuxCmd`) with exported
methods `AddStrand`, `UpdateStrand`, `RemoveStrand`, `Up`, `Resume`, `Down`, `Status`, plus
`Socket()`/`SessionName()` accessors, each returning `(resultStruct, error)` — no cobra, no
io.Writer, no exit codes (engine-purity litmus).

**Card order matters:** the low-level composable helpers come first so later cards never
forward-reference an undefined symbol at per-card build time — 16 (Engine+lock) -> 17
(reconcile) -> 18 (apply) -> 19 (strand mutation, which *composes* reconcile+apply) -> 20
(lifecycle, which composes all of the above).

Batch-local decisions (from the discussion, load-bearing):
- **Single-layer lock:** each public engine op acquires `.lyx/mux.lock` **once at entry**
  (via `internal/lock.AcquireWriteLock` on `layout.DotLyxDir()/mux.lock`) and holds it for
  the whole `read->mutate->persist->render->apply` cycle, composing from unexported
  **unlocked** helpers (`reconcileLocked`, `applyLayoutLocked`, `addStrandLocked`, ...).
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

### Card 16: Engine type, single mux operation lock, session/socket accessors

- **Context:**
  - `internal/lock/lock.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/server.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/lock.go`
  - `internal/muxengine/lock_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/lock.go`, define the engine value: `type Engine
  struct { cfg Config; layout *hubgeometry.Layout; psmux PsmuxCmd }` and a constructor `func
  New(cfg Config, layout *hubgeometry.Layout) *Engine` (builds the `PsmuxCmd` from
  `cfg.Psmux` + `socketName(layout.Hub)`). Add exported accessors so `muxcli` never needs the
  unexported naming helpers or the raw `layout`: `func (e *Engine) Socket() string { return
  socketName(e.layout.Hub) }` and `func (e *Engine) SessionName() string { return
  SessionName(e.layout.WorktreeRoot) }`. Add the lock helper `func (e *Engine) withOpLock(fn
  func() error) error` that acquires `lock.AcquireWriteLock(filepath.Join(
  e.layout.DotLyxDir(), "mux.lock"))` **once**, `defer`s `Release()`, and runs `fn` — this
  is the ONLY acquisition point; every public op wraps its body in `withOpLock` and calls
  unexported `*Locked` helpers that assume the lock is held. Document the non-reentrancy
  rule (gofrs/flock across handles) and the outer(mux.lock)->inner(state's mux.json.lock)
  ordering in a comment. In `lock_test.go`: assert the lock path is under `.lyx/`
  (per-worktree); assert two `withOpLock` calls serialize (second blocks until first
  releases) using a fixture `.lyx` dir; assert a released lock can be re-acquired (no stale
  lock — handle auto-released on `Release`); assert `Socket()`/`SessionName()` return the
  expected `lyx-<hub-basename>-<hash>` / worktree-slug strings.
- **Commit:** `feat(muxengine): Engine type, single-layer mux op lock, socket/session accessors`

### Card 17: reconcile against live panes

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
  (pure planning + kill I/O; assumes the op lock held). The precise rule (GAP B — the two
  clauses must not contradict): to keep the layout string consistent with psmux's live pane
  set, **kill each `pane_dead=1` pane** (via `e.psmux.run("kill-pane", "-t", id)`) **before**
  re-applying the layout — **except** a **sole-remaining** dead pane (killing it would end the
  session). Then, for each strand: **(a)** if its `PaneID` is absent from `live` **or** its
  pane was just killed, **clear the `PaneID` and set `Live = false`, but keep the record**;
  **(b)** if its pane is still present in `live` — including the **sole-remaining dead pane
  that was deliberately NOT killed** — **keep its `PaneID` and set `Live = true`** (do NOT
  clear the binding: the sole-dead pane is still a present window pane, so render must include
  it, else the layout string would enumerate 0 panes while psmux still has 1 — the exact GAP2
  mismatch). So the sole-dead exception is expressed as "not killed ⇒ still present ⇒ Live",
  never as a cleared-binding-yet-rendered contradiction. Return the killed ids. Provide a pure
  inner helper `func planReconcile(strands []Strand, live []LivePane) (clearedGUIDs []string,
  deadToKill []string, solePane string)` so the decision logic is unit-testable without psmux;
  `solePane` (when non-empty) is the strand/pane kept `Live` without clearing. In
  `reconcile_test.go`, table-test `planReconcile` with saved tables + fake `list-panes`
  results incl. `pane_dead=1` rows: a gone/killed strand's record survives with cleared
  `PaneID` and `Live=false`; present (alive) strands keep ids and `Live=true`; a
  **sole-remaining dead pane keeps its `PaneID` and stays `Live=true`** (not scheduled for
  kill, binding not cleared); non-sole dead panes are scheduled for kill; only `RemoveStrand`
  (card 19) ever deletes a record.
- **Commit:** `feat(muxengine): reconcile clears dead pane bindings, keeps records, kills dead-except-sole`

### Card 18: ApplyLayout — render to select-layout/select-pane (+ debounce)

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
  set (the ids present in `live`, i.e. `list-panes` — alive or dead-but-`remain`), map **all**
  strands via `toRenderStrands(st.Strands, liveIDs)` (render, not the engine, drops
  not-live/pane-less strands — GAP B, card 6), call `render.Rules` with **keyed** struct
  fields (never positional): `render.Rules(strands, render.Box{X: 0, Y: 0, W: cfg.Width, H:
  cfg.Height}, render.Params{TopBandRows: cfg.TopBandRows, CollapsedStripRows:
  cfg.CollapsedStripRows, MinFullRows: cfg.MinFullRows})`, then `e.psmux.run("select-layout",
  "-t", session, layout)` and `e.psmux.run("select-pane", "-t", focus)` (skip when <2 panes,
  mirroring muxpoc's `applyColumnLayout` guard).
  Order inside the caller (reconcile then apply): kill dead -> re-enumerate live -> compute
  layout -> apply. Add a pure planning helper `func (e *Engine) planLayout(st *MuxState,
  live []LivePane) (layout, focus string, err error)` so the "what layout string would be
  applied" is testable without psmux. Debounce is an **in-process driver** concern only:
  document that a burst of in-process mutations coalesces into one `applyLayoutLocked` per
  op (a one-shot CLI verb is a single mutation -> single apply, nothing to debounce). In
  `apply_test.go`, assert `planLayout` produces the expected layout string + focus target
  for a canonical strand table (reuse render golden expectations) with no live psmux.
- **Commit:** `feat(muxengine): ApplyLayout renders and applies select-layout/select-pane`

### Card 19: strand mutation — AddStrand / UpdateStrand / RemoveStrand

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/name.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/server.go`
  - `internal/muxpoccli/up.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/lock.go`
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/apply.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/spawn_test.go`
  - `internal/muxengine/strand.go`
  - `internal/muxengine/strand_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First, in `internal/muxengine/spawn.go`, define the **shared** pane-launch
  helper (GAP A — this is the one place `add`, surface, and resume all realize a strand into a
  live pane; without it `add` would register a record and re-render but never create a pane or
  run its command): `func (e *Engine) launchStrandLocked(st *MuxState, s *Strand, launchCmd
  string) error` (op lock held) — pick the target pane via the adopt-vs-split rule (**adopt**
  the `new-session` pane, captured with `activePaneID(session)`, when no other strand currently
  holds a live pane — the first strand into a fresh session; **otherwise** `split-window -P -F
  "#{pane_id}"`), set `s.PaneID` to the captured id and `s.Live = true`, then `e.psmux.run(
  "send-keys", "-t", s.PaneID, launchCmd, "Enter")`. Expose the adopt-vs-split decision as a
  pure `func planLaunch(st *MuxState) (adopt bool)` (adopt when no strand has a non-empty
  `PaneID`) so it is unit-testable without psmux; test it in `spawn_test.go`. Then, in
  `internal/muxengine/strand.go`, add the mutation ops on `*Engine`, each acquiring
  `withOpLock` and delegating to an unexported `*Locked` helper. `func (e *Engine)
  AddStrand(spec AddSpec) (Strand, error)`: generate `newGUID`; **stamp `Worktree =
  e.layout.WorktreeRoot`** (the engine owns its geometry — the CLI never supplies it);
  **resolve the `Name`** — use `spec.NameOverride` verbatim when set, else
  `FormatStrandName(e.cfg.StrandName, parts)` with `parts` = `{ROLE: spec.Role, ROUND:
  spec.Round, WORKTREE: filepath.Base(e.layout.WorktreeRoot), SHORT_GUID: guid[:8]}`, else the
  bare `guid[:8]` when neither name nor role is given (so the engine, which owns guid
  generation, also owns the guid-dependent name — the CLI can't compute `<SHORT_GUID>` before
  the guid exists); validate `spec.Parent` names an existing strand (reject unknown); reject a
  parent link that would form a **cycle**; build the `Strand` (opaque `Cmd`/`ResumeCmd`,
  `Display` from `spec`); **if `anchor == hidden`, register the record with no pane and do NOT
  call `launchStrandLocked` (defer to surface); otherwise call `e.launchStrandLocked(st,
  &strand, strand.Cmd)` to actually create the pane and run `cmd`**; then reconcile + apply the
  layout. Define `type AddSpec struct { Role, Round, NameOverride, Parent, Cmd, ResumeCmd
  string; Display render.Display }` (`Role`/`Round`/`NameOverride` are formatting-only inputs
  consumed here, never persisted as fields). `func (e *Engine) UpdateStrand(guid string,
  display render.Display) (Strand, error)`: mutate by guid; **reject** a `visible -> hidden`
  transition (error `cannot hide a live strand in v1`); on `hidden -> visible` (surface),
  **call `e.launchStrandLocked(st, &strand, strand.Cmd)`** to create the pane + run `cmd`, then
  reconcile + apply. `func (e *Engine) RemoveStrand(guid string, recursive bool) (Removed,
  error)`: a **non-leaf** with `recursive == false` returns an error `strand has children, use
  --recursive`; otherwise **cascade** — remove the strand and its whole descendant subtree;
  return `type Removed struct { Strands []struct{ GUID, Name string } }` listing every removed
  strand. In `strand_test.go` (drive the ops against a fixture `.lyx`, spawn behind the
  planning seam / `//go:build smoke` for the live `send-keys`): assert guid is generated +
  unique; unknown/cyclic parent rejected; hidden-add stores a record with empty `PaneID` and
  unrun `cmd` (no `launchStrandLocked` call); a non-hidden add / a `hidden->visible` update
  both invoke the launch path; `UpdateStrand` visible->hidden rejected; `RemoveStrand` on a
  non-leaf without `recursive` errors, and with `recursive` cascades and lists every removed
  strand.
- **Commit:** `feat(muxengine): strand mutation + shared launchStrandLocked pane spawn (add/surface)`

### Card 20: lifecycle — Up / Resume / Down / Status

- **Context:**
  - `internal/muxpoccli/up.go`
  - `internal/muxpoccli/down.go`
  - `internal/muxpoccli/status.go`
  - `internal/muxengine/overlay.go`
  - `internal/muxengine/env.go`
  - `internal/muxengine/server.go`
  - `internal/muxengine/state.go`
  - `internal/muxengine/reconcile.go`
  - `internal/muxengine/apply.go`
  - `internal/muxengine/spawn.go`
  - `internal/proc/proc_windows.go`
  - `internal/muxengine/lock.go`
- **Edits:** none
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
  hidden**, call the **shared** `e.launchStrandLocked(st, &strand, resumeOrCmd)` (card 19's
  GAP-A helper — same adopt-vs-split + `send-keys` path `add`/surface use) where `resumeOrCmd =
  strand.ResumeCmd` (or `strand.Cmd` when `ResumeCmd` is empty); **skip hidden strands**; leave
  already-live strands untouched; reconcile; apply; re-persist pane ids. `func (e *Engine) Down() (DownResult, error)`: `kill-server` + delete
  `.lyx/mux.json` (idempotent, ignore not-exist). `func (e *Engine) Status() (StatusResult,
  error)`: reconcile against live panes and report **this session only** (tracked strands +
  live/dead) — no stray-server enumeration (deferred). Returns a **non-nil error when the
  server/session is absent** (e.g. `no mux session; run "lyx mux up"`) so `attach`'s pre-flight
  (card 27) can surface it on the envelope. In `lifecycle_test.go`, drive the
  planning seams that do not need a live psmux (assert `Up` plans no strand launch; `Resume`
  plans a replay for each not-live non-hidden strand and skips hidden; the three states —
  server dead / server-up-CLI-restarted / single pane died — at the "what commands would
  run" seam). Guard any real-psmux round-trip behind `//go:build smoke`.
- **Commit:** `feat(muxengine): Up/Resume/Down/Status lifecycle ops (up=substrate, resume=replay)`

## Batch Tests

`verify: go test ./internal/muxengine/...` runs the whole engine package: lock serialization
+ per-worktree scope + no-stale-lock + accessor strings, `planReconcile`, `planLayout`,
strand mutation (guid/cascade/hidden/cycle rules), and the lifecycle planning seams. All
live psmux I/O (`new-session`, `send-keys`, `select-layout`) is confined to `//go:build
smoke` tests so the default `verify:` stays hermetic and fast. Concurrency is asserted via
`withOpLock` serialization on a fixture `.lyx` dir (real gofrs/flock, no psmux).
