# Batch: docs

```yaml
task: "Extract internal/fsx and build internal/state"
batch: "docs"
number: 5
cards: 4
verify: null
depends-on: [1, 4]
```

## Batch Scope

Update the shared-libs documentation to reflect the landed code: a new `fsx.md`, a rewritten
`state.md` that drops the superseded "single shared document" design, a narrowed roadmap milestone 3,
and the shared-libs README index. Pure documentation — no runnable surface, so `verify: null`. Depends
on batches 1 and 4 so the writer can read the real `internal/fsx/fsx.go` and `internal/state/state.go`
rather than guessing the API. Touches only `docs/**`, disjoint from every other batch.

## Cards

### Card 10: Write docs/shared-libs/fsx.md

- **Context:**
  - `internal/fsx/fsx.go`
  - `docs/shared-libs/lock.md`
  - `docs/shared-libs/README.md`
- **Edits:** none
- **Creates:**
  - `docs/shared-libs/fsx.md`
- **Deletes:** none
- **Requirements:** New file `docs/shared-libs/fsx.md` documenting `internal/fsx`. Follow the style of
  the sibling shared-libs docs (e.g. `lock.md`): a short prose description, then the public surface.
  Cover the three functions and the error type per `## Shared Decisions → fsx public API`:
  `AtomicWriteBytes` (general trusted-absolute primitive, no guard), `PathGuard` (standalone validator
  for untrusted relative paths), `AtomicWrite` (guarded convenience composed over the two), and
  `PathError`. State that fsx has zero internal dependencies and computes no path geometry (so it does
  not touch `internal/paths`), and that it is the home extracted from `internal/board`.
- **Commit:** `docs(fsx): document the filesystem-safety primitives`

### Card 11: Rewrite docs/shared-libs/state.md

- **Context:**
  - `internal/state/state.go`
  - `internal/fsx/fsx.go`
- **Edits:**
  - `docs/shared-libs/state.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite `docs/shared-libs/state.md` for the revised design. Remove the
  "single typed document, shared by the modules that write to it" / worktree+mux-share-`local-state.json`
  assumption and the old "A note on AtomicWrite / PathGuard" section (those primitives now live in
  `internal/fsx` — link to `fsx.md`). Describe `internal/state` as a generic, schema-less locked typed
  JSON I/O primitive: `WriteJSON[T]` / `ReadJSON[T] (T, bool, error)`, locking on `<path>.lock` beside
  the data file via `internal/lock`, atomic writes via `fsx.AtomicWriteBytes`, missing-file →
  `found=false`, corrupt-file → error. State that callers own *what* the fields mean and the file path
  (e.g. mux will write `.lyx/mux-state.json`); the worktree module stays stateless. Keep the note that
  the `.lyx/` runtime dir is gitignored and machine-local.
- **Commit:** `docs(state): rewrite for generic locked-JSON design`

### Card 12: Narrow roadmap milestone 3

- **Context:**
  - `docs/shared-libs/state.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/roadmap.md`, update milestone 3 (`**\`internal/state\`.**`, lines ~29-34).
  Drop the "mux + worktree share the same state document" framing and the `local-state.json` registry
  wording. Re-scope it to: a generic locked typed JSON I/O primitive (`internal/state`) built on
  `internal/fsx` + `internal/lock`, no fixed schema and no shared registry, still test-first and still
  landing with mux (milestone 5). Mention that the `internal/fsx` extraction (this task) is its
  prerequisite. Do not renumber other milestones.
- **Commit:** `docs(roadmap): narrow milestone 3 to generic state I/O`

### Card 13: Update shared-libs README index

- **Context:**
  - `docs/shared-libs/fsx.md`
  - `docs/shared-libs/state.md`
- **Edits:**
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/README.md` "## Libraries" list, add an `fsx.md` entry
  (`internal/fsx`: atomic file writes + relative-path guard, extracted from board) in alphabetical/
  logical position, and change the `state.md` entry from **(planned)** to its shipped description
  (generic locked typed JSON I/O built on fsx + lock). Keep the existing one-line-per-lib format.
- **Commit:** `docs(shared-libs): index fsx and mark state shipped`

## Batch Tests

`verify: null` — this batch only edits `docs/**` Markdown, which has no runnable surface. Correctness
is confirmed by the plan reviewer reading the four files against the landed `internal/fsx` and
`internal/state` code listed in each card's `Context:`.
