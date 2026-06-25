# Batch: templates-home

```yaml
task: "Move config templates home by removing the lyxtest->configreg edge"
batch: templates-home
number: 2
cards: 4
verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...
depends-on: [1]
```

## Batch Scope

This batch reverts commit `c374501`: it moves each module's YAML template back into its own feature
package (`internal/<feature>/template.yaml`, embedded via `//go:embed`), re-points
`configreg.Modules()` at the feature `ConfigTemplate` functions, and deletes the synthetic
`internal/configtmpl` leaf. It is safe to re-introduce the `configreg → feature` import only because
batch 1 already removed the `lyxtest → configreg` edge, so no test-build cycle can form. The batch is
one unit because the three feature reverts and the `configreg`/`configtmpl` change are a single
logical revert and must land together for the build to be consistent. Depends on batch 1.

Batch-local decisions: restore the exact pre-`c374501` file shape — the embedded var is named
`configTemplate` (unexported), the file is `template.yaml` (not `<feature>.yaml`), and the YAML
content is byte-identical to the current `configtmpl/<feature>.yaml`.

## Cards

### Card 7: Restore the board template to internal/board

- **Context:**
  - `internal/configtmpl/board.yaml`
  - `internal/configtmpl/configtmpl.go`
- **Edits:**
  - `internal/board/template.go`
- **Creates:**
  - `internal/board/template.yaml`
- **Deletes:** none
- **Requirements:** Create `internal/board/template.yaml` with content byte-identical to
  `internal/configtmpl/board.yaml`. Rewrite `internal/board/template.go` to the pre-`c374501` form:
  package `board`, `import _ "embed"`, a `//go:embed template.yaml` directive on an unexported
  `var configTemplate string`, and `func ConfigTemplate() string { return configTemplate }`. Remove
  the `internal/configtmpl` import and the delegation to `configtmpl.Board()`. Keep the existing
  doc-comment intent (board uses `${env:VAR:-default}` syntax).
- **Commit:** `refactor(board): embed own template.yaml, drop configtmpl delegation`

### Card 8: Restore the worktree template to internal/worktree

- **Context:**
  - `internal/configtmpl/worktree.yaml`
  - `internal/configtmpl/configtmpl.go`
- **Edits:**
  - `internal/worktree/template.go`
- **Creates:**
  - `internal/worktree/template.yaml`
- **Deletes:** none
- **Requirements:** Create `internal/worktree/template.yaml` with content byte-identical to
  `internal/configtmpl/worktree.yaml`. Rewrite `internal/worktree/template.go` to the pre-`c374501`
  form: package `worktree`, `import _ "embed"`, `//go:embed template.yaml` on `var configTemplate
  string`, and `func ConfigTemplate() string { return configTemplate }`. Remove the `configtmpl`
  import and the `configtmpl.Worktree()` delegation.
- **Commit:** `refactor(worktree): embed own template.yaml, drop configtmpl delegation`

### Card 9: Restore the weft template to internal/weft

- **Context:**
  - `internal/configtmpl/weft.yaml`
  - `internal/configtmpl/configtmpl.go`
- **Edits:**
  - `internal/weft/template.go`
- **Creates:**
  - `internal/weft/template.yaml`
- **Deletes:** none
- **Requirements:** Create `internal/weft/template.yaml` with content byte-identical to
  `internal/configtmpl/weft.yaml`. Rewrite `internal/weft/template.go` to the pre-`c374501` form:
  package `weft`, `import _ "embed"`, `//go:embed template.yaml` on `var configTemplate string`, and
  `func ConfigTemplate() string { return configTemplate }`. Remove the `configtmpl` import and the
  `configtmpl.Weft()` delegation. (weft templates use literal values, no `${env:...}` markers.)
- **Commit:** `refactor(weft): embed own template.yaml, drop configtmpl delegation`

### Card 10: Re-point configreg at features and delete configtmpl

- **Context:**
  - `internal/board/template.go`
  - `internal/worktree/template.go`
  - `internal/weft/template.go`
- **Edits:**
  - `internal/configreg/configreg.go`
- **Creates:** none
- **Deletes:**
  - `internal/configtmpl/configtmpl.go`
  - `internal/configtmpl/board.yaml`
  - `internal/configtmpl/worktree.yaml`
  - `internal/configtmpl/weft.yaml`
- **Requirements:** Rewrite `configreg.go` to the pre-`c374501` form: import
  `internal/board`, `internal/worktree`, `internal/weft` (drop the `internal/configtmpl` import), and
  have `Modules()` return `{"board", board.ConfigTemplate}`, `{"worktree", worktree.ConfigTemplate}`,
  `{"weft", weft.ConfigTemplate}`. Leave `Module`, `Template(name)`, and `Names()` unchanged. Delete
  the entire `internal/configtmpl/` directory (the `.go` file and all three `.yaml` files). After
  this card, `grep -rn "configtmpl" --include=*.go .` must be empty.
- **Commit:** `refactor(configreg): reference feature ConfigTemplate, delete configtmpl`

## Batch Tests

`verify: go build ./... && go vet -tags integration ./... && go test -tags integration ./...`.

`go build ./...` confirms the production revert compiles (the feature `template.go` embeds and the
`configreg` re-point). `go vet -tags integration ./...` re-confirms no import cycle now that
`configreg → feature` is restored (it is safe because batch 1 cut `lyxtest → configreg`). The full
`go test -tags integration ./...` is justified because `configreg` and the feature templates feed
many packages (`board`, `worktree`, `weft`, `configcli`, `configsync`, and every fixture-seeding
test from batch 1). Key tests: each feature's `template_test.go` (`TestConfigTemplate_*`),
`configreg_test.go` (`TestNames`, `TestTemplate_Found` comparing `weft.ConfigTemplate()`), and the
batch-1 seeding tests must all stay green.
