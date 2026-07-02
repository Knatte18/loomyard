# Batch: docs

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'docs'
number: 8
cards: 4
verify: null
depends-on: [7]
```

## Batch Scope

Completes the Documentation Lifecycle for the landed mux module (same task/PR as the
behavior — every prior batch merges together): reconcile `docs/modules/mux.md` to the
as-built dumb-carrier design (its stale decision-3 predates the shuttle split), update
`docs/overview.md`'s module table (mux 🚧 -> ✅, muxpoc parked), mark the mux milestone
✅ Done in `docs/roadmap.md`, and register the `attach` JSON-envelope exception in
`CONSTRAINTS.md` under the CLI/Cobra Invariant. Pure markdown — no runnable Go surface, so
`verify: null`.

## Cards

### Card 33: reconcile docs/modules/mux.md to as-built design

- **Context:**
  - `_mill/discussion.md`
  - `internal/muxengine/doc.go`
- **Edits:**
  - `docs/modules/mux.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `docs/modules/mux.md` so it matches the as-built design and
  supersedes the stale decision-3 (which had mux constructing the Claude `--session-id` /
  command — the dumb-carrier split moved launch/resume construction to shuttle). Document the
  final contract: the domain-free strand record (no `type`; GUID durable key; `sessionId`
  demoted to opaque metadata), one named psmux server per hub
  (`lyx-<hub-basename>-<short-hash>`) with a per-worktree session, the `internal/muxengine` +
  `internal/muxengine/render` + `internal/muxcli` split, the render policy/mechanics
  separation, the on-demand daemonless re-render + single mux operation lock, native resume
  via the stored opaque `resumeCmd`, and the deferred items (daemon, pane-died auto-trigger,
  own-window anchor, mplex/columns, session portability). Explicitly note that the old
  decision-3 text is superseded.
- **Commit:** `docs(mux): reconcile module doc to as-built dumb-carrier design`

### Card 34: docs/overview.md module table

- **Context:**
  - `docs/overview.md`
  - `docs/modules/mux.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `docs/overview.md`'s module table / execution stack: mark `mux`
  as delivered (🚧 -> ✅) with a link to `docs/modules/mux.md`, and note `muxpoc` is parked
  (kept on disk as reference, unwired from the CLI). If the execution-stack diagram names the
  layers (`proc -> mux -> shuttle -> review -> loom`), confirm mux is shown as built. Do not
  alter unrelated rows.
- **Commit:** `docs(overview): mark mux delivered, muxpoc parked`

### Card 35: docs/roadmap.md milestone

- **Context:**
  - `docs/roadmap.md`
  - `docs/modules/mux.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/roadmap.md`, mark the mux milestone **✅ Done** (it is a planned
  milestone — the orchestration-spine prerequisite) with a link to `docs/modules/mux.md`. Do
  not add roadmap notes for anything other than this completed milestone.
- **Commit:** `docs(roadmap): mark mux milestone done`

### Card 36: CONSTRAINTS.md — register attach envelope exception

- **Context:**
  - `CONSTRAINTS.md`
  - `docs/modules/mux.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`, under the **CLI / Cobra Invariant**, register the
  `attach` exception: `lyx mux attach`'s terminal-handover tail emits no JSON envelope (only
  its pre-flight failures do), following the interactive-`ide` precedent. Keep it a short
  index entry in the file's established style (the fuller rationale lives in
  `docs/modules/mux.md`). Do not add a new top-level invariant — this is a scoped note on the
  existing CLI/Cobra Invariant.
- **Commit:** `docs(constraints): register attach JSON-envelope exception`

## Batch Tests

`verify: null` — this batch edits only markdown (`docs/modules/mux.md`, `docs/overview.md`,
`docs/roadmap.md`, `CONSTRAINTS.md`); there is no runnable Go surface. The module-wide
overview `verify: go build ./...` still runs at the batch boundary and confirms the tree
compiles (a no-op for doc-only edits). Documentation Lifecycle is satisfied at task
granularity — all eight batches merge as one unit, so behavior and docs land together.
