# Discussion: Extract internal/fsx and build internal/state

```yaml
task: Extract internal/fsx and build internal/state
slug: extract-internal-fsx
status: discussing
parent: main
```

## Problem

`AtomicWrite` and `PathGuard` live in `internal/board/git.go` but contain no
board-specific logic — they are generic filesystem-safety primitives (atomic
temp-file+rename writes, and rejection of empty/absolute/`..` relative paths).
Two forces make their current home wrong:

1. `internal/muxpoc/state.go:108` already reaches across a module boundary into
   `board.AtomicWrite` — a cross-module dependency flagged in `docs/shared-libs/state.md`
   as needing a real home.
2. Milestone 3's `internal/state` (locked typed JSON I/O for machine-local runtime
   state) needs atomic writes too. Giving `internal/state` a dependency on
   `internal/board` would invert the layering — board is a domain module, not shared
   infrastructure.

**Why now:** the task lands both halves together because they are tightly coupled
and individually small — extracting `internal/fsx` is the prerequisite that lets
`internal/state` be built on shared plumbing instead of on board. `internal/state`
is built **test-first** here (its named consumer `mux`, milestone 5, does not exist
yet), so this task locks the state API early so mux can simply call it.

## Scope

**In:**

- Create `internal/fsx` package: `AtomicWriteBytes` (general primitive), `PathGuard`
  (standalone validator), and `AtomicWrite` (guarded convenience composed over the two).
- Move `TestPathGuard` and `TestAtomicWrite` from `internal/board/git_test.go` into a
  new `internal/fsx` test file; rewrite the moved `AtomicWrite` tests to also cover
  `AtomicWriteBytes`.
- Update `internal/board` to import `fsx` instead of owning these functions
  (behaviour-preserving): `git.go` loses `PathGuard`/`AtomicWrite`/`BoardPathError`;
  `store.go` and `render.go` switch to the appropriate fsx call.
- Update `internal/muxpoc/state.go` to call `fsx.AtomicWrite` directly (minimal swap),
  removing its reach into `board`.
- Create `internal/state` package: generic locked typed JSON read/write
  (`ReadJSON[T]`, `WriteJSON[T]`) built on `fsx.AtomicWriteBytes` + `internal/lock`,
  with deep test-first coverage.
- Docs: create `docs/shared-libs/fsx.md`; rewrite `docs/shared-libs/state.md` for the
  revised (non-shared-document) design; update `docs/roadmap.md` milestone 3 scope;
  add fsx + flip state status in `docs/shared-libs/README.md`.

**Out:**

- Refactoring `internal/muxpoc` onto `internal/state`. muxpoc keeps its own
  `LoadState`/`SaveState`; only the `board.AtomicWrite` call is swapped for
  `fsx.AtomicWrite`. (muxpoc's corrupt-file "warn string, no error" semantics differ
  from the generic primitive; migrating it is mux-milestone work.)
- Building the `mux` module or any live consumer of `internal/state`. `internal/state`
  ships test-only this task.
- `internal/paths` geometry. fsx takes paths as arguments and computes no cwd/root
  geometry, so it does not interact with the path invariant.
- Locking changes, JSON-schema/registry design, junction handling — all out (see the
  "What fsx/state does NOT do" notes under Decisions).

## Decisions

### fsx-api-shape

- Decision: `internal/fsx` exposes three functions:
  - `AtomicWriteBytes(absPath string, data []byte) error` — the **general primitive**:
    `MkdirAll(filepath.Dir(absPath))`, write to a temp file in that dir, `Rename` onto
    `absPath`. No path guard (the path is trusted, code-constructed geometry).
  - `PathGuard(relPath string) error` — standalone validator: rejects empty, absolute
    (Unix `/…`, Windows `C:\…` and `X:` drive form), and any `..` component. Logic moved
    verbatim from `board/git.go:33-59`.
  - `AtomicWrite(dir, relPath, content string) error` — **guarded convenience** composed
    over the two: `PathGuard(relPath)` then `AtomicWriteBytes(filepath.Join(dir, relPath), []byte(content))`.
- Rationale: the writer primitive carries no security policy; callers opt into the guard
  where the relative path is untrusted. The convenience wrapper keeps existing call sites
  (board render, muxpoc) one-line and behaviour-identical. `AtomicWriteBytes` lets
  `internal/state` write a known absolute path with raw bytes — no `[]byte`→`string`
  round-trip, no no-op guard on a path the program built itself.
- Rejected: keeping only the guarded `AtomicWrite(dir, relPath, content)` and forcing
  `internal/state` to split its known full path into `dir`+`base` and re-convert bytes to
  string — runs a guaranteed-pass guard and an avoidable string conversion, and would
  reject any absolute path it was handed.

### error-type

- Decision: rename `BoardPathError` (string-backed) to `fsx.PathError` (same `string`
  underlying type and `Error()` method), moved into `fsx`. board's push/path error types
  that remain git-related (`BoardPushError`) stay in `internal/board`.
- Rationale: `BoardPathError` is referenced only inside `board/git.go` (returned by
  `PathGuard`); no external type assertion exists, and board's tests assert `err != nil`
  not the concrete type, so the rename is safe.
- Rejected: a more generic `fsx.GuardError` name (bikeshed, same mechanics); keeping the
  `Board`-prefixed name in a non-board package (misleading).

### board-migration

- Decision: full migration, no compatibility shims. Per call site:
  - `internal/board/store.go:113` (writes the fixed `tasks.json` under the swap lock;
    `content` is already `[]byte`) → `fsx.AtomicWriteBytes(filepath.Join(boardPath, relPath), content)`.
    Drops the pointless guard on a fixed path and the `[]byte`→`string` round-trip. The
    swap-lock acquire/release in store.go is unchanged and stays in board.
  - `internal/board/render.go:27` (writes rendered filenames derived from task data) →
    `fsx.AtomicWrite(boardPath, relPath, content)`. Keeps the guard, which has real value
    on dynamic relPaths.
  - `git.go` deletes the moved `PathGuard`, `AtomicWrite`, and `BoardPathError`; keeps
    `Pull`, `CommitPush`, `BoardPushError`, and adds the `internal/fsx` import where used.
- Rationale: matches "import fsx instead of owning these functions"; gives board the
  general writer where the path is trusted and the guarded writer where it isn't; leaves
  no dead re-exports.
- Rejected: thin `board.AtomicWrite`/`board.PathGuard` wrappers delegating to fsx — leaves
  an ambiguous second home and dead surface.

### muxpoc-change

- Decision: minimal swap. `internal/muxpoc/state.go:108` changes `board.AtomicWrite(cwd, stateRelPath, string(content))`
  to `fsx.AtomicWrite(cwd, stateRelPath, string(content))` and swaps the `board` import for
  `fsx` (the `flock`/`internal/lock` import and all `LoadState`/`SaveState`/`DeleteState`
  logic are untouched).
- Rationale: behaviour-preserving and removes the cross-module reach into board — the only
  thing this task owes muxpoc. The signature of `fsx.AtomicWrite` is identical to
  `board.AtomicWrite`, so the swap is one line plus an import.
- Rejected: refactoring muxpoc onto `internal/state` (scope creep onto a behaviour-preserving
  change; muxpoc's corrupt-file semantics differ — deferred to the mux milestone).

### state-api-shape

- Decision: generics. `internal/state` exposes:
  - `func WriteJSON[T any](path string, v T) error`
  - `func ReadJSON[T any](path string) (value T, found bool, err error)`
- Rationale: type-safe call sites with no caller-side assertions (Go 1.26); the `found`
  return directly serves the dominant "load existing state or start fresh" pattern.
- Rejected: the `encoding/json` unmarshal-into-pointer idiom
  (`WriteJSON(path, v any)` / `ReadJSON(path, out any) error`) — more conventional but no
  compile-time type tie and pushes a zero-value-vs-found check onto callers.

### state-locking

- Decision: `internal/state` derives the lock path internally as `<path> + ".lock"` (suffix
  append, not extension replace) — a sibling of the data file in the same directory. Both
  `ReadJSON` and `WriteJSON` `MkdirAll(filepath.Dir(path))` first (so the lock file can be
  created), then take the lock via `internal/lock`: `WriteJSON` an exclusive write lock,
  `ReadJSON` a shared read lock; both release via `defer`.
- Rationale: caller passes only the data path (simplest API); the lock lives beside the file
  it protects (operator-legible, gitignored under `.lyx/`); suffix-append (`mux-state.json.lock`)
  can never collide with a real data file.
- Rejected: an explicit `lockPath` caller argument (more verbose at every site); extension-replace
  (`mux-state.lock`) which can collide with a sibling file named `mux-state.*`.

### state-read-semantics

- Decision: `ReadJSON` returns `found=false, err=nil` when the data file does not exist;
  a present-but-unparseable file returns a non-nil `err` (corruption is never silently
  swallowed). Writes use `json.MarshalIndent(v, "", "  ")` (2-space indent, human-readable).
- Rationale: matches the `muxpoc.LoadState` "absent → start fresh" pattern without making
  every caller write `errors.Is(..., fs.ErrNotExist)`, while still surfacing real corruption.
  Indented output keeps `.lyx/*.json` runtime files legible.
- Rejected: absent-as-error wrapping `fs.ErrNotExist` (most idiomatic but pushes the
  not-found branch to every caller); silently returning the zero value on a missing OR
  corrupt file (hides corruption).
- Accepted consequence: a `ReadJSON` on a non-existent file still `MkdirAll`s the parent dir
  and creates the `.lock` file (a file lock needs a lock file). This mirrors
  `muxpoc.LoadState` today and is acceptable for the gitignored `.lyx/` runtime dir.

### docs

- Decision: create `docs/shared-libs/fsx.md`; rewrite `docs/shared-libs/state.md` for the
  revised design (drop the "single shared document / worktree+mux share `local-state.json`"
  assumption — `internal/state` is a generic locked-JSON-I/O primitive, the worktree module
  stays stateless, and mux owns `mux-state.json`); narrow `docs/roadmap.md` milestone 3 to
  the revised scope (no registry, no fixed schema — just locked typed JSON I/O); add the
  fsx entry and flip the state entry from **(planned)** in `docs/shared-libs/README.md`.
- Rationale: every shared lib has its own doc; the existing `state.md`/`roadmap.md` describe
  a design this task explicitly supersedes.
- Rejected: folding fsx into `state.md` (breaks the one-doc-per-lib pattern).

## Technical context

- **Source of the primitives:** `internal/board/git.go` — `PathGuard` (lines 33-59),
  `AtomicWrite` (lines 61-96), `BoardPathError` (lines 25-30). The `defer os.Remove(tmpPath)`
  cleanup and the rename-is-the-atomic-swap comment carry over to `AtomicWriteBytes`.
- **board call sites:** `internal/board/render.go:27` (`RenderToDisk`, dynamic relPaths from
  `Render`), `internal/board/store.go:113` (`tasks.json` write, already under the swap lock at
  store.go:106-111, `content` already `[]byte`).
- **muxpoc call site:** `internal/muxpoc/state.go:108`, inside `SaveState`, which already holds
  an exclusive write lock from `internal/lock` and marshals with `json.MarshalIndent`. Only the
  write call + import change.
- **Locking primitive:** `internal/lock` provides `AcquireWriteLock`/`AcquireReadLock` (both
  block until available; cross-process via `gofrs/flock`) and `(*FileLock).Release()`. This is
  what `internal/state` composes for its read/write locks.
- **Existing pattern to mirror:** `internal/muxpoc/state.go` (`LoadState`/`SaveState`) is almost
  exactly what `internal/state.ReadJSON`/`WriteJSON` generalize — same MkdirAll-then-lock-then-
  atomic-write shape. Use it as the reference implementation; do not import from muxpoc.
- **fsx has zero internal dependencies.** `internal/state` depends only on `internal/fsx` +
  `internal/lock` (both already exist).
- **Module path / Go version:** `github.com/Knatte18/loomyard`, `go 1.26` (generics available).
- **Runtime dir:** machine-local state lives under the gitignored `.lyx/` dir (per
  `docs/shared-libs/state.md`); state files and their `.lock` siblings both sit there.

## Constraints

From `CONSTRAINTS.md` (Path Invariant):

- All cwd/worktree-root geometry MUST go through `internal/paths.Getwd()` / `internal/paths.Resolve()`.
  Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go`.
- **Compliance:** `internal/fsx` and `internal/state` take fully-formed paths as arguments and
  compute no cwd/root geometry — they call neither banned primitive, so they satisfy the invariant
  without depending on `internal/paths`. Callers (mux, later) resolve geometry via `internal/paths`
  and hand the resulting absolute path in.

From `docs/shared-libs/README.md` ("the line we hold"): a shared lib does one mechanical thing and
carries no domain logic, plus its own deep tests. fsx (atomic write + path guard) and state (locked
typed JSON I/O) both stay mechanical — no task/worktree/pane knowledge.

## Testing

Behaviour-preserving refactor; the guardrail is that board's existing suite plus the moved tests all
stay green, and `internal/state` is built test-first.

- **`internal/fsx` (move + extend):** new `fsx_test.go` in `package fsx_test`.
  - `TestPathGuard` moved verbatim from `board/git_test.go:17-41` (retarget to `fsx.PathGuard`):
    empty, Unix-absolute, Windows-absolute, `..` mid-path, `..` at start, valid relative/nested.
  - `TestAtomicWrite` moved from `board/git_test.go:43-97` (retarget to `fsx.AtomicWrite`): writes
    content, creates parent dirs, leaves no `.tmp-` file.
  - **New** `AtomicWriteBytes` coverage: writes raw bytes to an absolute path, creates parent dirs,
    overwrites an existing file atomically, leaves no temp file. (TDD candidate — it's the new primitive.)
- **`internal/board` (regression):** `board/git_test.go` keeps `TestPull` and `TestCommitPush`
  (lines 99-343) unchanged. The store/render write paths are covered by board's existing
  `store_test.go`/`render_test.go`; confirm they pass after the swap. No new board tests required.
- **`internal/muxpoc` (regression):** existing `state_test.go` / `muxpoc_smoke_test.go` must stay
  green after the one-line swap. No new tests.
- **`internal/state` (test-first, new — primary TDD target):** `state_test.go` in `package state_test`.
  Scenarios to cover:
  - Round-trip: `WriteJSON` a typed struct, `ReadJSON` it back, `found=true`, equal value.
  - Missing file: `ReadJSON` on a never-written path → `found=false`, `err=nil`, and the parent dir +
    `.lock` file get created.
  - Corrupt file: write garbage bytes to the data path, `ReadJSON` → non-nil `err` (not swallowed).
  - Atomicity / no temp leak: after `WriteJSON`, the dir contains only the data file + `.lock` (no
    `.tmp-` remnant).
  - Lock-file location: the lock is `<path>.lock` beside the data file.
  - Overwrite: second `WriteJSON` replaces content; `ReadJSON` returns the new value.
  - (If cheap) generic instantiation with two distinct struct types in the same test file, proving
    the API is type-parametric.

  Do not prescribe exact assertion shapes — that is mill-plan's job.

## Q&A log

- **Q:** Build `internal/state` now (test-first, no live consumer) or fsx-only this task? **A:** Both — they belong tightly together and are individually small.
- **Q:** muxpoc change scope — minimal swap or refactor onto `internal/state`? **A:** Minimal swap (`board.AtomicWrite` → `fsx.AtomicWrite`).
- **Q:** board migration — full migration or compatibility shims? **A:** Full migration, no shims.
- **Q:** fsx error type for `PathGuard`? **A:** Rename `BoardPathError` → `fsx.PathError`.
- **Q:** docs — separate `fsx.md` or fold into `state.md`? **A:** Separate `fsx.md`, plus rewrite state.md/roadmap.md/README.
- **Q:** Add an absolute-path write variant (`AtomicWriteBytes`)? **A:** Yes — and consequently board is rewritten to use the general primitive where the path is trusted (store.go → `AtomicWriteBytes`), keeping the guarded `AtomicWrite` only where relPaths are dynamic (render.go).
- **Q:** `internal/state` API — generics or pointer-out? **A:** Generics (`WriteJSON[T]`, `ReadJSON[T] (T, bool, error)`).
- **Q:** Lock file location? **A:** `<path>.lock` (suffix append) beside the data file — lock lives in the same dir as the file it locks.
- **Q:** `ReadJSON` on missing file? **A:** `found=false, err=nil`; corrupt file is a real error.
