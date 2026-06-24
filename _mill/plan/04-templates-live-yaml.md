# Batch: templates-live-yaml

```yaml
task: "Extract yamlengine and migrate config via lyx update"
batch: templates-live-yaml
number: 4
cards: 3
verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/
depends-on: [1]
```

## Batch Scope

Convert the three module templates from commented-out Go string-builders into live,
embedded YAML (`//go:embed template.yaml`), using the `${env:...}` grammar with the
default living in the value. After this batch a template, resolved against an empty
env via `yamlengine.Resolve`, yields the module's defaults — making the template the
single source of truth (the `DefaultConfig()` removal happens in batch 5). The
`ConfigTemplate() string` signature is preserved so existing callers
(`board/init.go`, `configcli`) keep compiling unchanged. This batch also relaxes
`board/init_test.go`'s "fully commented" assertions so the `board` package stays
green; `init` itself is refactored in batch 6.

## Cards

### Card 5: board template → live embedded YAML

- **Context:**
  - `go.mod`
  - `internal/yamlengine/resolve.go`
  - `internal/board/init.go`
- **Edits:**
  - `internal/board/template.go`
  - `internal/board/template_test.go`
  - `internal/board/init_test.go`
- **Creates:**
  - `internal/board/template.yaml`
- **Deletes:** none
- **Requirements:**
  - Create `internal/board/template.yaml` with live YAML (the comment after each value is a trailing `#` line-comment):
    - `path: ${env:LYX_BOARD_PATH:-../_board}` (comment: board dir; tasks.json + rendered output; relative to cwd or absolute)
    - `home: ${env:LYX_HOME:-Home.md}` (comment: home page file name; relative to board dir)
    - `sidebar: ${env:LYX_SIDEBAR:-_Sidebar.md}` (comment: sidebar file name; relative to board dir)
    - `proposal_prefix: ${env:LYX_PROPOSAL_PREFIX:-proposal-}` (comment: prefix for proposal files)
  - Rewrite `internal/board/template.go`: drop the `strings.Builder`; add `import _ "embed"`, a `//go:embed template.yaml` directive over `var configTemplate string`, and keep `func ConfigTemplate() string { return configTemplate }` (signature unchanged). Keep the file's package godoc accurate.
  - Rewrite `internal/board/template_test.go`: assert `ConfigTemplate()` is valid YAML (parses via `yaml.Unmarshal` into a `map[string]any` without error), contains the four expected keys, and that `yamlengine.Resolve([]byte(ConfigTemplate()), nil)` (empty env) unmarshals to the default values `path: ../_board`, `home: Home.md`, `sidebar: _Sidebar.md`, `proposal_prefix: proposal-`.
  - In `internal/board/init_test.go`: REMOVE the two loops that assert every non-blank line of `board.yaml`/`worktree.yaml` starts with `#` (the template is no longer commented). KEEP the byte-equality assertions (`content == board.ConfigTemplate()`, `worktreeContent == worktree.ConfigTemplate()`) and all other assertions unchanged.
  - **Commit:** `refactor(board): make config template live embedded YAML`

### Card 6: worktree template → live embedded YAML

- **Context:**
  - `go.mod`
  - `internal/yamlengine/resolve.go`
- **Edits:**
  - `internal/worktree/template.go`
  - `internal/worktree/template_test.go`
- **Creates:**
  - `internal/worktree/template.yaml`
- **Deletes:** none
- **Requirements:**
  - Create `internal/worktree/template.yaml` with live YAML: `branch_prefix: ${env:LYX_BRANCH_PREFIX:-}` and a trailing comment (prefix prepended to the slug to form the branch name, e.g. `"hanf/"`; empty = branch == slug). The empty default `${env:...:-}` resolves to the empty string.
  - Rewrite `internal/worktree/template.go` to embed `template.yaml` via `//go:embed`, keeping `func ConfigTemplate() string` returning the embedded content (signature unchanged).
  - Rewrite `internal/worktree/template_test.go`: assert valid YAML, the `branch_prefix` key is present, and `yamlengine.Resolve([]byte(ConfigTemplate()), nil)` yields `branch_prefix: ""` (empty string, key present).
  - **Commit:** `refactor(worktree): make config template live embedded YAML`

### Card 7: weft template → live embedded YAML (literal pathspec)

- **Context:**
  - `go.mod`
  - `internal/yamlengine/resolve.go`
- **Edits:**
  - `internal/weft/template.go`
  - `internal/weft/template_test.go`
- **Creates:**
  - `internal/weft/template.yaml`
- **Deletes:** none
- **Requirements:**
  - Create `internal/weft/template.yaml` with a plain LITERAL value (no env marker — weft never had one): `pathspec: "_lyx"` with a trailing comment (directory path(s) relative to worktree root, whitespace-separated; `_lyx` is the default). This is the template's literal-value example.
  - Rewrite `internal/weft/template.go` to embed `template.yaml` via `//go:embed`, keeping `func ConfigTemplate() string` returning the embedded content (signature unchanged).
  - Rewrite `internal/weft/template_test.go`: assert valid YAML, `pathspec` key present, and `yamlengine.Resolve([]byte(ConfigTemplate()), nil)` yields `pathspec: _lyx` (literal passes through unchanged with an empty env).
  - **Commit:** `refactor(weft): make config template live embedded YAML`

## Batch Tests

`verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/` runs each
module's package tests. The template tests assert the new YAML resolves to the prior
defaults via `yamlengine.Resolve` (depends-on batch 1). The board package run also
exercises the relaxed `init_test.go`; if any other test in these three packages
asserts the old commented format, update it minimally to keep the packages green.
