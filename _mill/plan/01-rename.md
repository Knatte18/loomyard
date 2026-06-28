# Batch: rename

```yaml
task: "Rename internal/config to internal/configengine"
batch: "rename"
number: 1
cards: 5
verify: go build ./... && go vet -tags integration ./... && go test ./...
depends-on: []
```

## Batch Scope

This batch performs the complete, behaviour-preserving rename of the config engine
package `internal/config` → `internal/configengine` and updates every reference to it
across code and docs, plus records the package-naming convention in `CONSTRAINTS.md`.
It is a single batch because a Go package rename is atomic: the tree only compiles once
the package clause, its import path, and all importers move together (see
`## Shared Decisions` in `00-overview.md`). The five cards are ordered — Card 1 performs
the directory move and must be applied before Cards 2–3 reference the new
`internal/configengine` package. The batch exposes no new interface; the engine's
exported API (`Load`, `Edit`, `FindBaseDir`, `EditorFunc`, `ErrAborted`, `DefaultEditor`)
is unchanged. There is no CLI change.

## Cards

### Card 1: Rename the engine directory, package clauses, and header comments

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/configengine/config.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit_test.go`
- **Deletes:**
  - `internal/config/config.go`
  - `internal/config/edit.go`
  - `internal/config/config_test.go`
  - `internal/config/edit_test.go`
- **Requirements:** Run `git mv internal/config internal/configengine` to move the
  directory while preserving history (do NOT delete-and-recreate). Then, in the moved
  files: in `config.go` and `edit.go` change the package clause `package config` →
  `package configengine`, and update each file-header doc comment that names `config` (the
  package/actor) to `configengine` (e.g. `config.go`'s opening
  `// config.go implements strict YAML configuration loading…` keeps the filename token
  `config.go` but any reference to the *package* `config` becomes `configengine`). In
  `config_test.go` and `edit_test.go` change the package clause `package config_test` →
  `package configengine_test`, change the import path
  `github.com/Knatte18/loomyard/internal/config` →
  `github.com/Knatte18/loomyard/internal/configengine`, and change every `config.`
  qualifier (e.g. `config.Load`, `config.Edit`, `config.FindBaseDir`, `config.ErrAborted`,
  `config.EditorFunc`, `config.DefaultEditor`) → `configengine.`. Do NOT change any
  exported symbol name, signature, or behaviour. Keep the filenames `config.go` /
  `edit.go` / `config_test.go` / `edit_test.go` unchanged.
- **Commit:** `refactor(configengine): rename internal/config package to configengine`

### Card 2: Update production importers

- **Context:**
  - `_mill/discussion.md`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/board/config.go`
  - `internal/warp/config.go`
  - `internal/weft/config.go`
  - `internal/configcli/configcli.go`
  - `internal/configcli/menu.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In each listed file, change the import
  `github.com/Knatte18/loomyard/internal/config` →
  `github.com/Knatte18/loomyard/internal/configengine` and every `config.` qualifier
  (`config.Load`, `config.Edit`, `config.FindBaseDir`, `config.EditorFunc`,
  `config.ErrAborted`, `config.DefaultEditor`) → `configengine.`. Additionally, in
  `internal/board/config.go`, `internal/warp/config.go`, and `internal/weft/config.go`,
  update the line-4 file-header doc comment `// LoadConfig uses internal/config.Load …` →
  `// LoadConfig uses internal/configengine.Load …`. No behaviour change.
- **Commit:** `refactor(configengine): update production importers to configengine`

### Card 3: Update test importer and comment-only references

- **Context:**
  - `_mill/discussion.md`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/configcli/configcli_test.go`, change the import
  `github.com/Knatte18/loomyard/internal/config` →
  `github.com/Knatte18/loomyard/internal/configengine` and every `config.` qualifier →
  `configengine.`. In `internal/configcli/configcli_integration_test.go` (comment-only —
  it imports `configreg`, not the engine), change the comment reference
  `config.Edit→FindBaseDir` → `configengine.Edit→FindBaseDir`. In
  `internal/warp/worktreelifecycle.go` (comment-only) change the line-7 comment
  `// Configuration is resolved cwd-authoritatively via internal/config;` →
  `… via internal/configengine;`. In `internal/paths/paths.go` (comment-only, no import)
  change the line-70 comment `that authority stays in internal/config` →
  `that authority stays in internal/configengine` and the line-128 comment
  `used by callers like config.Load` → `used by callers like configengine.Load`.
- **Commit:** `refactor(configengine): update test importer and comment refs`

### Card 4: Rename and update the shared-lib docs

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/paths.md`
  - `docs/overview.md`
  - `docs/roadmap.md`
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:**
  - `docs/shared-libs/configengine.md`
- **Deletes:**
  - `docs/shared-libs/config.md`
- **Requirements:** Run `git mv docs/shared-libs/config.md docs/shared-libs/configengine.md`
  (preserve history). In the moved `configengine.md`, change the H1 heading
  `# `internal/config`` → `# `internal/configengine``, change every `internal/config`
  body reference → `internal/configengine`, and change the bare `` `config` `` actor token
  at line ~22 (`` `config` errors with `not initialized…` ``) → bare `` `configengine` ``.
  In `docs/shared-libs/README.md` (line ~21), change the bullet link target `config.md` →
  `configengine.md` and `internal/config` → `internal/configengine`. In
  `docs/shared-libs/paths.md` (line ~129), change `internal/config.FindBaseDir` →
  `internal/configengine.FindBaseDir`. In `docs/overview.md` (lines ~172 and ~237), change
  `internal/config` → `internal/configengine` in the source-tree listing and the
  shared-infra list. In `docs/roadmap.md`, change `internal/config` → `internal/configengine`
  at lines ~65 and ~78, and change the **bare** `` `config` `` token at line ~31 (in the
  list `` `config`/`git`/`lock` ``) → bare `` `configengine` `` to stay consistent with its
  sibling bare tokens. In `docs/benchmarks/test-suite-timing.md` (lines ~65 and ~335),
  change the table row label `config` → `configengine`. Do not append any milestone note
  to `docs/roadmap.md` — these are name-accuracy fixes only.
- **Commit:** `docs(configengine): rename shared-lib doc and update references`

### Card 5: Record the package-naming convention in CONSTRAINTS.md

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a new `### Package naming` subsection **inside** the existing
  `## CLI / Cobra Invariant` section (place it after the `### For New Code` subsection, so
  it sits within the CLI/Cobra invariant and before the `## Documentation Lifecycle`
  section). The subsection states the convention: a command-owning package takes the
  command's bare name (`internal/warp` ⟷ `lyx warp`); a `cli` suffix is used **only** when
  the bare name is unavailable — taken by a sibling (`config` → the engine, now
  `configengine`) or reserved/special in Go (`init` → `func init()`). Therefore
  `configcli` and `initcli` are principled, deliberate exceptions, not inconsistency. Do
  not alter any other invariant text.
- **Commit:** `docs(constraints): record package-naming convention`

## Batch Tests

The batch `verify` is `go build ./... && go vet -tags integration ./... && go test ./...`,
run from the worktree root after all five cards are applied. This is the legitimate
cross-cutting case for a full-tree verify (the rename touches the engine plus six importer
packages and a comment in `internal/paths`), so the unbounded tree-wide test is the
correct scope — a per-package scope could not prove the whole module still compiles after
a package rename. `go build ./...` proves every non-test package compiles against the new
`internal/configengine` import path; `go vet -tags integration ./...` additionally
compiles the integration-tagged file `internal/configcli/configcli_integration_test.go`
(which references the engine only in a comment, so it is unaffected at the type level but
should still be confirmed to compile); `go test ./...` runs the existing suites
(`configengine`, `configcli`, `board`, `warp`, `weft`, `paths`, and the `cmd/lyx`
drift/registration/helptree guards) which are the behaviour-preservation guardrail. No new
tests are added — the rename is mechanical and behaviour-preserving, so green existing
tests are the proof. After `verify` passes, completeness is confirmed by the
word-boundary grep from `## Shared Decisions` returning no stale `config`-package
references.
