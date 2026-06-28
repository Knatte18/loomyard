# Discussion: Rename internal/config to internal/configengine

```yaml
task: Rename internal/config to internal/configengine
slug: config-engine-rename
status: discussing
parent: main
```

## Problem

`internal/config` is the **config engine**: strict YAML load (read → validate against
template → resolve env → bytes) plus interactive edit (scaffold / editor / validate /
abort). But the bare name `config` is both vague for an engine and the most overloaded
word in the config family — it sits alongside `configcli`, `configreg`, and `configsync`,
so "config" the package and "config" the concept are constantly ambiguous.

Renaming it to `configengine` makes the role explicit, matches the existing `yamlengine`
naming convention (role suffix), and keeps the shared `config*` prefix with its siblings.
This is a **behaviour-preserving** rename — the exported API and all runtime behaviour
are unchanged; only the package qualifier moves (`config.Load` → `configengine.Load`).
It is the low-risk precursor to the larger `internal/module/` folder restructure (a
separate task), which is why we do it first.

**Why now:** it unblocks the `internal/module/` reorg and removes a standing source of
naming confusion in the config family before that larger move lands.

## Scope

**In:**

- Rename the directory `internal/config/` → `internal/configengine/` using `git mv`
  (preserve blame/history).
- Change package clauses:
  - `config.go`: `package config` → `package configengine`
  - `edit.go`: `package config` → `package configengine`
  - `config_test.go`: `package config_test` → `package configengine_test`
  - `edit_test.go`: `package config_test` → `package configengine_test`
- Update file-header doc comments in `config.go` / `edit.go` to say `configengine`.
- Keep the filenames `config.go` / `edit.go` / `config_test.go` / `edit_test.go`
  unchanged (responsibility-named, matching the `yamlengine` convention of
  `reconcile.go` / `resolve.go` — see Decisions).
- Update every importer's import path (`internal/config` → `internal/configengine`)
  and `config.` qualifier → `configengine.`:
  - `internal/board/config.go` (import + qualifiers)
  - `internal/warp/config.go` (import + qualifiers)
  - `internal/weft/config.go` (import + qualifiers)
  - `internal/configcli/configcli.go` (import + qualifiers)
  - `internal/configcli/menu.go` (import + qualifiers)
  - `internal/configcli/configcli_test.go` (import + qualifiers)
  - `internal/config/config_test.go` and `internal/config/edit_test.go` — external test
    package (`package config_test`) that imports the package under test; update both the
    package clause (above) and the import path / qualifiers.
- `internal/configcli/configcli_integration_test.go`: **comment-only** update — the file
  imports `configreg`, not `internal/config`; its single reference is the comment
  `config.Edit→FindBaseDir` (line ~42). No import change here; update the comment to
  `configengine.Edit→FindBaseDir`.
- Docs:
  - `git mv docs/shared-libs/config.md` → `docs/shared-libs/configengine.md`; update its
    heading and body references from `internal/config` → `internal/configengine`.
  - `docs/shared-libs/README.md` (line ~21): update the bullet link/target
    `config.md` → `configengine.md` and `internal/config` → `internal/configengine`.
  - `docs/shared-libs/paths.md` (line ~129): `internal/config.FindBaseDir` →
    `internal/configengine.FindBaseDir`.
  - `docs/overview.md` (lines ~172, ~237): `internal/config` → `internal/configengine`
    in the source-tree listing and the shared-infra list.
  - `docs/roadmap.md` (lines ~31, ~65, ~78): update the literal package-name token
    `internal/config` → `internal/configengine` in place (name-accuracy fix only — see
    Decisions; no milestone note is appended).
- **Full stale-reference sweep** (beyond the original brief's list):
  - `internal/paths/paths.go` (line ~128): comment `config.Load` → `configengine.Load`.
  - `docs/benchmarks/test-suite-timing.md` (lines ~65, ~335): the table row labels
    `config` → `configengine`.
- Record the **package-naming convention** in `CONSTRAINTS.md` as a new
  `### Package naming` subsection **under the existing `## CLI / Cobra Invariant`**
  (see Decisions for the exact convention text), in the **same commit** as the rename.

**Out:**

- The `internal/module/` folder restructure (separate, larger task).
- Any rename of `configcli` / `initcli` — kept as deliberate, principled exceptions.
- Any change to the engine's exported API, signatures, or behaviour
  (`Load`, `Edit`, `FindBaseDir`, `EditorFunc`, `ErrAborted`, `DefaultEditor` all
  stay exactly as-is).
- Any CLI change. The engine has no `Command()`; `lyx config` is owned by `configcli`
  and is untouched.
- The `lyx update` command-verb question (separate discussion).

## Decisions

### use-git-mv-to-preserve-history

- Decision: Perform the directory rename and the `config.md` → `configengine.md` doc
  rename with `git mv`, not delete-and-recreate.
- Rationale: Keeps `git blame` / history attached to the moved files; this is a pure
  rename so history continuity is free and valuable.
- Rejected: delete+recreate (relies on best-effort rename detection; can fragment blame).

### keep-responsibility-named-files

- Decision: Keep filenames `config.go` / `edit.go` (and their `_test.go` variants); do
  **not** rename to `configengine.go`.
- Rationale: The sibling engine `internal/yamlengine` names files by responsibility
  (`reconcile.go`, `resolve.go`), not after the package. `config.go` (the Load path) and
  `edit.go` (the interactive edit path) already follow that responsibility-named pattern,
  and every consumer package keeps its `config.go` filename too. Renaming files adds
  churn without aligning to any convention.
- Rejected: rename `config.go` → `configengine.go` (no convention supports it; pure noise
  in the diff).

### full-stale-reference-sweep

- Decision: Leave **zero** stale `config`-as-package-name references in the tree. In
  addition to the brief's explicit list, fix the `config.Load` comment in
  `internal/paths/paths.go` and the `config` row labels in
  `docs/benchmarks/test-suite-timing.md`.
- Rationale: The rename is behaviour-preserving and mechanical; a clean sweep is cheap
  and prevents the renamed package from being referred to by a name that no longer
  exists. Half-renames are a future-reader trap.
- Rejected: brief-scope-only (leaves paths.go and the benchmark doc saying `config`);
  paths.go-only (still leaves the benchmark labels stale).

### update-roadmap-name-token-in-place

- Decision: Update the three historical `internal/config` mentions in `docs/roadmap.md`
  (lines ~31, ~65, ~78) to `internal/configengine` **in place**, as a name-accuracy fix.
- Rationale: CLAUDE.md's roadmap discipline forbids *appending rename notes / milestone
  entries* to the roadmap — it does not forbid keeping an existing token factually
  correct. These are literal package-name references describing already-done foundation
  work; after the rename the old name is simply wrong. We change only the token, add no
  prose, mark nothing new as done.
- Rejected: leave roadmap untouched (keeps a now-incorrect package name in a living doc).

### record-naming-convention-under-cli-cobra-invariant

- Decision: Add a `### Package naming` subsection inside the existing
  `## CLI / Cobra Invariant` in `CONSTRAINTS.md` (not a new top-level invariant).
- Rationale: The convention is about how command-owning packages are named, which is part
  of the CLI/Cobra surface; co-locating it with the command-seam rules keeps related
  guidance together. The convention text: a command-owning package takes the command's
  bare name (`internal/warp` ⟷ `lyx warp`); a `cli` suffix is used **only** when the bare
  name is unavailable — taken by a sibling (`config` → the engine) or reserved/special in
  Go (`init` → `func init()`). Thus `configcli` and `initcli` are principled exceptions,
  not inconsistency.
- Rejected: standalone top-level `## Package Naming Invariant` (over-weights a convention
  that naturally belongs with the CLI rules).

## Technical context

- **The engine** (`internal/config`, → `internal/configengine`): two production files.
  - `config.go` — `package config`; exports `Load`, `FindBaseDir`, plus the config-load
    machinery. Imports `internal/envsource`, `internal/paths`, `internal/yamlengine`.
  - `edit.go` — `package config`; exports `Edit`, `EditorFunc`, `ErrAborted`,
    `DefaultEditor`. None of these exported symbols change.
- **Importers** (exact, verified by grep):
  - Production: `internal/board/config.go:15`, `internal/warp/config.go:14`,
    `internal/weft/config.go:14`, `internal/configcli/configcli.go:19`,
    `internal/configcli/menu.go:15` — each has the import line
    `"github.com/Knatte18/loomyard/internal/config"` and uses `config.` qualifiers.
  - Tests: `internal/configcli/configcli_test.go:19` (import + qualifiers);
    `internal/config/config_test.go` and `internal/config/edit_test.go` (external
    `package config_test`, import the package under test).
  - Comment-only: `internal/configcli/configcli_integration_test.go` (`package configcli`,
    imports `configreg`; the only `config.` token is a comment).
- **Module path / import prefix:** `github.com/Knatte18/loomyard`. The new import path is
  `github.com/Knatte18/loomyard/internal/configengine`.
- **No `Command()` on the engine** — confirmed; the CLI/Cobra registration guards
  (`cmd/lyx/registration_test.go`, `helptree_test.go`, `longlist_test.go`) are unaffected
  because the engine is not a CLI module. `lyx config` lives in `internal/configcli`.
- **Path Invariant is untouched** — the rename does not introduce or move any
  cwd/geometry or `_lyx`/config-path resolution; all such calls already route through
  `internal/paths` and stay exactly as written.
- Doc surface verified by grep across `docs/`: `config.md`, `README.md`, `paths.md`,
  `overview.md`, `roadmap.md`, `benchmarks/test-suite-timing.md` (see Scope for exact
  lines).

## Constraints

From `CONSTRAINTS.md` (hub root):

- **Path Invariant** — unaffected; the rename touches no cwd/geometry or config-path
  resolution. Do not introduce string-literal `_lyx`/config paths while editing.
- **lyxtest Leaf Invariant** — unaffected; no change to `internal/lyxtest` imports.
- **CLI / Cobra Invariant** — unaffected at the seam (the engine is not a CLI module);
  this task **adds** to it the new `### Package naming` subsection (same commit).
- **Documentation Lifecycle / Task-completion rule (CLAUDE.md):** all doc updates listed
  in Scope land in the **same commit** as the rename. A commit shipping the rename
  without the docs is incomplete.
- **Roadmap discipline (CLAUDE.md):** only the existing name token is corrected in
  `docs/roadmap.md`; no milestone note is appended.

## Testing

- **No new tests.** The rename is mechanical and behaviour-preserving; the existing
  suites are the guardrail.
- **Guardrail:** `go build ./...` and `go test ./...` must both be green after the
  rename. Green here is the proof of behaviour preservation — the existing
  `config` / `configcli` / `board` / `warp` / `weft` suites all exercise the engine
  through its (unchanged) API.
- **Verification of completeness:** after the edit, a tree-wide grep for the old package
  import path `internal/config"` and the bare `config.` qualifier (excluding the
  legitimately-unrelated `configcli` / `configreg` / `configsync` / `configengine`
  tokens) must return nothing referring to the renamed engine. The CLI drift/registration
  guards (`cmd/lyx/drift_test.go`, `registration_test.go`, `helptree_test.go`,
  `longlist_test.go`) must remain green.

## Q&A log

- **Q:** How to handle stale refs outside the brief's scope list (paths.go:128 comment,
  benchmark timing-table labels)? **A:** Full sweep — fix both; leave zero stale
  references.
- **Q:** What to do with the three historical `internal/config` mentions in
  `docs/roadmap.md` given the "roadmap = planned milestones only" discipline?
  **A:** Update the name token in place (name-accuracy fix, no milestone note).
- **Q:** Where in `CONSTRAINTS.md` should the package-naming convention be recorded?
  **A:** New `### Package naming` subsection under the existing CLI/Cobra Invariant.
- **Q:** Preserve git history for the rename? **A:** Yes — `git mv` for the directory and
  for `config.md` → `configengine.md`.
- **Q:** Rename the engine's filenames to `configengine.go`? **A:** No — keep
  `config.go` / `edit.go`; `yamlengine` names files by responsibility, not package, and
  `config.go`/`edit.go` already follow that.
