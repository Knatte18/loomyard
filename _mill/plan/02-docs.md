# Batch: docs

```yaml
task: "Rename internal/paths to internal/hubgeometry"
batch: "docs"
number: 2
cards: 3
verify: null
depends-on: [1]
```

## Rename mechanic

This batch renames one documentation file. For the `Moves:` pair the implementer MUST:

1. Run `git mv docs/shared-libs/paths.md docs/shared-libs/hubgeometry.md` FIRST, before
   editing its contents.
2. Make ONLY surgical edits — update the title, the `internal/paths` / `package paths`
   references, and the `paths.` qualifiers inside the moved file. Do NOT rewrite it.
3. Use a `Creates:` entry only for genuinely new files — there are none here.
4. Never write the relocated file from scratch and delete the original.

## Batch Scope

This batch makes the documentation and project-instruction prose match the renamed
package. It renames the invariant ("Path Invariant" → "Hub Geometry Invariant") in
`CONSTRAINTS.md`, renames the shared-lib doc `paths.md` → `hubgeometry.md`, and sweeps
every remaining `internal/paths` / "Path Invariant" reference across the docs tree and
`CLAUDE.md` to the new names. It depends on batch 1 only for ordering, so the docs
describe the already-renamed package. It contains no runnable Go surface, so `verify` is
`null`. The one structural subtlety is the heading-anchor: renaming the `## Path
Invariants` heading in `docs/overview.md` changes its GitHub auto-anchor slug, which a
live cross-doc link in `docs/modules/loom.md` depends on — that link is updated in the
same card so it does not silently 404.

## Cards

### Card 9: Rename the invariant in CONSTRAINTS.md

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`:
  - Rename the section heading `## Path Invariant` → `## Hub Geometry Invariant`, and
    reword every reference to the invariant by that name (e.g. in the `## lyxtest Leaf
    Invariant` section's "importing only the standard library and `internal/paths`") to
    "Hub Geometry Invariant" / `internal/hubgeometry` as appropriate.
  - Replace every `internal/paths` → `internal/hubgeometry` and every `paths.`
    package-qualifier in prose → `hubgeometry.` (e.g. `paths.Getwd()`, `paths.Resolve()`,
    `paths.BoardDir(...)`, `paths.LyxDirName`, `paths.ConfigDir(base)`,
    `paths.ConfigFile(base, module)`, `paths.WeftSiblingPath(...)`, `paths.HubPath(...)`,
    `paths.WeftHostSlug(...)`).
  - Do not change the geometry-literal token list, the enforced rules, the allowlist
    semantics, or any constant value — only the package name and the invariant's name.
- **Commit:** `docs(constraints): rename Path Invariant to Hub Geometry Invariant`

### Card 10: Sweep docs tree and rename the shared-lib doc

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/envsource.md`
  - `docs/modules/loom.md`
  - `docs/modules/mux.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `docs/shared-libs/paths.md` -> `docs/shared-libs/hubgeometry.md`
- **Requirements:**
  - After `git mv paths.md → hubgeometry.md`: update its H1 title, every `internal/paths`
    → `internal/hubgeometry`, `package paths` → `package hubgeometry`, and `paths.`
    qualifier → `hubgeometry.` inside the moved file.
  - In `docs/overview.md`: rename the heading `## Path Invariants` → `## Hub Geometry
    Invariants`, and replace **every** `internal/paths` occurrence in the file with
    `internal/hubgeometry` — not only in the renamed heading's section but also the
    directory-tree entry (`├── internal/paths/ …`, ~line 177) and the shared-modules list
    (~line 242). Also update the prose "sole owner of cwd and worktree-root geometry math"
    sentence and the `Getwd()`/`Resolve()`/`enforcement_test.go` references, and update any
    link to the renamed `shared-libs/paths.md`. Backstop with the comprehensive-sweep grep.
  - In `docs/modules/loom.md`: replace **every** `internal/paths` occurrence with
    `internal/hubgeometry` (both the line-60 "cwd/Hub/Prime via `internal/paths`" reference
    and the line-256 reference), and update the cross-doc anchor link at ~line 256
    `[launcher geometry](../overview.md#path-invariants)` →
    `(../overview.md#hub-geometry-invariants)` to match the renamed heading's new slug.
  - In `docs/shared-libs/README.md`: update the `paths.md` / `internal/paths` entry to
    `hubgeometry.md` / `internal/hubgeometry`.
  - In `docs/shared-libs/envsource.md`: update the dependency-direction line (~line 5)
    "`internal/envsource` imports `internal/paths`" → `internal/hubgeometry`.
  - In `docs/modules/mux.md`, `docs/benchmarks/test-suite-timing.md`, and
    `docs/roadmap.md`: replace every `internal/paths` reference with `internal/hubgeometry`
    (in `roadmap.md` this is an in-place correctness fix to a stale module reference, not
    a new roadmap note — do not add roadmap commentary).
- **Commit:** `docs: rename paths shared-lib doc to hubgeometry and sweep references`

### Card 11: Update CLAUDE.md project instructions

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CLAUDE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CLAUDE.md`, update the invariant name and module reference in the
  "Current invariants include: the **Path Invariant** (`internal/paths` owns all
  cwd/geometry and `_lyx`/config paths)" sentence → "the **Hub Geometry Invariant**
  (`internal/hubgeometry` owns …)", and replace any other `internal/paths` / "Path
  Invariant" mention in the file with the new names. Prose only; no behavioural claim
  changes besides the rename.
- **Commit:** `docs(claude): rename Path Invariant to Hub Geometry Invariant`

## Batch Tests

`verify: null` — this is a pure documentation/prose batch with no runnable Go surface.
The Go build and test guards already ran at the end of batch 1; nothing in this batch
affects compilation. Correctness here is a review concern: the reviewer confirms no
`internal/paths` / "Path Invariant" / `#path-invariants` references dangle (per the
discussion's no-dangling-reference greps) and that the `loom.md` anchor link resolves to
the renamed heading.
