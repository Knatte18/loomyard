# Batch: board-config-fixes

```yaml
task: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep
batch: board-config-fixes
number: 1
cards: 5
verify: go build ./... && go vet ./...
depends-on: []
```

## Batch Scope

Corrects every doc and doc-comment that still describes the **removed `.mhgo/` config
layer** / "three-layer" / "layered YAML" config model for the board module and the shared
`internal/config` loader. The actual model is two layers: built-in defaults overlaid with
`_mhgo/<module>.yaml`, plus `.env` loading and `$env:NAME` / `$env:NAME ? fallback`
expansion — there is no `.mhgo/` config layer. The fixes mirror the already-corrected
worktree module (see Shared Decision `mirror-worktree-reference-pattern`): board.md is
slimmed to delegate the resolution model + env grammar to `docs/shared-libs/config.md`
rather than re-documenting it. This batch produces no external interface for later batches.
All edits are doc/comment-only (Shared Decision `docs-and-doc-comments-only`); `.go` edits
must not touch line endings (Shared Decision `never-modify-line-endings`).

## Cards

### Card 1: Slim board.md §Configuration to two-layer + delegate

- **Context:**
  - `docs/shared-libs/config.md`
  - `docs/modules/worktree.md`
  - `internal/config/config.go`
  - `internal/board/config.go`
- **Edits:**
  - `docs/modules/board.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/modules/board.md`: (1) delete the blockquote "**Target redesign
  (not yet implemented).** … This section is updated when that milestone lands." (the
  `> ...` block immediately under the `## Configuration` heading). (2) Replace the "###
  Layered model" subsection — which lists three sources including "**`<cwd>/.mhgo/board.yaml`**
  (optional, gitignored) — machine-local overrides" — with a **two-layer** statement:
  built-in defaults overlaid with `_mhgo/board.yaml`; state that there is **no `.mhgo/`
  config layer** and that machine-local variation is expressed via `$env:` references inside
  the tracked YAML. (3) Keep the "### Keys and defaults" table verbatim (its values `path:
  ../_board`, `home: Home.md`, `sidebar: _Sidebar.md`, `proposal_prefix: proposal-` match
  `DefaultConfig()` in `internal/board/config.go`). (4) Remove the "### Environment variable
  expansion" and "### Path resolution" subsections from board.md and replace them with a
  one-line pointer to `[shared-libs/config.md](../shared-libs/config.md)` for the resolution
  model and `$env:` grammar (board.md must not re-document the env grammar — the current text
  also omits the `$env:NAME ? fallback` optional form). (5) In the "### cli.go" subsection,
  change "configuration is loaded from layered YAML files and merged with defaults" to
  describe the module's `_mhgo/board.yaml` merged with built-in defaults; keep the `_mhgo/`
  requirement, the `not initialized here; run "mhgo init"` error sentence, and the
  `--board-path` bypass paragraph unchanged. Preserve the `## init` section's legitimate
  `.mhgo/` gitignore references (Shared Decision `preserve-runtime-state-mhgo-references`).
- **Commit:** `docs(board): drop removed .mhgo/ config layer, delegate config model to shared-libs/config.md`

### Card 2: board cli.go doc-comment — delegate to internal/config

- **Context:**
  - `internal/worktree/cli.go`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/cli.go`, fix the `RunCLI` doc-comment so it no longer
  names config-file layout. (1) In the file/`RunCLI` header (around the "Otherwise, RunCLI
  resolves config via os.Getwd() and LoadConfig" lines) and (2) in the "Configuration
  resolution (cwd-authoritative)" block that currently reads "Configuration is loaded from
  `<cwd>/_mhgo/board.yaml` and `<cwd>/.mhgo/board.yaml` (optional), merged with defaults":
  replace with worktree-mirrored wording — "resolves the board configuration cwd-
  authoritatively via `internal/config`; the module never reads config files or knows their
  on-disk layout itself", as in `internal/worktree/cli.go` lines 1–11. Keep the `--board-path`
  bypass note. Comment-only change; do not alter any code or line endings.
- **Commit:** `docs(board): cli.go doc-comment delegates config to internal/config`

### Card 3: board config.go file-comment — mirror worktree

- **Context:**
  - `internal/worktree/config.go`
- **Edits:**
  - `internal/board/config.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/board/config.go`, replace the file-level comment (lines
  1–5: "config.go — layered configuration system for board modules. … from YAML files
  organized in layers. The system supports environment variable expansion and path
  resolution.") with wording mirroring `internal/worktree/config.go` lines 1–6: state that
  the file defines the board `Config`/`Outputs` types plus `DefaultConfig`/`LoadConfig`, and
  that `LoadConfig` **delegates to `internal/config` for resolution; the module never reads
  config files or knows their layout itself**. Do not change the `LoadConfig` doc-comment's
  factual description of behaviour (it already correctly references `internal/config.Load`).
  Comment-only change; do not alter code or line endings.
- **Commit:** `docs(board): config.go file-comment reflects internal/config delegation`

### Card 4: config.md — remove stale Status block + forward-looking phrasings

- **Context:**
  - `internal/config/config.go`
- **Edits:**
  - `docs/shared-libs/config.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/shared-libs/config.md`: (1) delete the blockquote "**Status:**
  target design. board currently has its own loader with a three-layer model … the `.mhgo/`
  config layer is **removed**." (lines ~6–9) — the config extraction has shipped, so the
  status block is stale. (2) Change the remaining forward-looking phrasings to present tense:
  "Milestone 2 lifts it here and **redesigns it to the model below**" → describe the shipped
  model directly; the parenthetical "*(This is board's existing behaviour, preserved.)*"
  after the `$env:NAME` bullet → drop the "preserved" framing (state it as current
  behaviour); "Not supported in v1: a fallback that itself contains `$env:`…" → "Not
  supported: …". Leave the rest of the file (Layout, Resolution model, env grammar, `.env`
  loading, exported helpers) unchanged — it already describes the correct two-layer model.
  Preserve the legitimate "`.mhgo/` … machine-local RUNTIME state" line in the Layout block
  (Shared Decision `preserve-runtime-state-mhgo-references`).
- **Commit:** `docs(config): remove shipped-milestone Status block and forward-looking phrasings`

### Card 5: internal/config Load doc-comment — single YAML file

- **Context:**
  - `docs/shared-libs/config.md`
- **Edits:**
  - `internal/config/config.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/config/config.go`, the `Load` function doc-comment opens
  "Load loads configuration for a module from defaults and **layered configuration files**."
  Change "layered configuration files" to name the single source: "from defaults and the
  module's `_mhgo/<module>.yaml` file" (it is one YAML layer over defaults, not multiple
  files). Do not change the rest of the doc-comment or any code; comment-only, no line-ending
  changes.
- **Commit:** `docs(config): Load doc-comment names the single _mhgo/<module>.yaml layer`

## Batch Tests

`verify: go build ./... && go vet ./...` — these are doc-comment-only edits to `.go` files
plus Markdown edits; the only automated risk is a malformed comment breaking compilation or
`vet`. `go build ./...` confirms the whole module still compiles; `go vet ./...` confirms no
comment/directive breakage. No behaviour changes, so no test assertions change — the board
config guardrail test `TestLoad_DotMhgoIgnored` (which proves the loader ignores `.mhgo/`)
remains green untouched and is exercised by the full `go test ./...` run at handoff. Markdown
edits (board.md, config.md) are validated by review and the handoff link/grep checks, not by
`go` tooling.
