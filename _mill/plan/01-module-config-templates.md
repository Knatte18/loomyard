# Batch: module-config-templates

```yaml
task: 'weft producers: _lyx/config, lyx config, codeguide'
batch: module-config-templates
number: 1
cards: 4
verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/
depends-on: []
```

## Batch Scope

Give each config module a `ConfigTemplate() string` that returns its fully-commented default
YAML. Relocate the existing `board` and `worktree` generators out of `internal/board/init.go`
into their owning modules (behaviour-preserving — `lyx init` must scaffold byte-identical
content), and author a fresh `weft` generator (none existed). This batch is the foundation the
`lyx-config-command` batch consumes: its module registry maps each module to its
`ConfigTemplate`. Batch-local decision: `board.ConfigTemplate`/`worktree.ConfigTemplate` are
**exact** ports of the current commented strings — no wording changes — so the regression guard
holds; `weft.ConfigTemplate` is new and only needs to parse and carry the `pathspec` default.

## Cards

### Card 1: Relocate board template to `board.ConfigTemplate`

- **Context:**
  - `internal/board/config.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:**
  - `internal/board/template.go`
- **Deletes:** none
- **Requirements:** Move the body of `generateCommentedBoardYAML()` from
  `internal/board/init.go` into a new `internal/board/template.go` as an exported
  `func ConfigTemplate() string` returning the **identical** 4-line commented string (the
  `# path:`, `# home:`, `# sidebar:`, `# proposal_prefix:` lines, byte-for-byte). In
  `init.go`, delete `generateCommentedBoardYAML` and change its call site (currently
  `content := generateCommentedBoardYAML()`) to `content := ConfigTemplate()`. No behaviour
  change to `RunInit`.
- **Commit:** `refactor(board): relocate board.yaml template to ConfigTemplate`

### Card 2: Relocate worktree template to `worktree.ConfigTemplate`

- **Context:**
  - `internal/worktree/config.go`
- **Edits:**
  - `internal/board/init.go`
- **Creates:**
  - `internal/worktree/template.go`
- **Deletes:** none
- **Requirements:** Move the body of `generateCommentedWorktreeYAML()` from
  `internal/board/init.go` into a new `internal/worktree/template.go` as an exported
  `func ConfigTemplate() string` returning the **identical** single commented `# branch_prefix:`
  line (byte-for-byte). In `init.go`, delete `generateCommentedWorktreeYAML`, add the import
  `github.com/Knatte18/loomyard/internal/worktree`, and change its call site (currently
  `content := generateCommentedWorktreeYAML()`) to `content := worktree.ConfigTemplate()`. The
  `board → worktree` import is acyclic (verified: `worktree` does not import `board`).
- **Commit:** `refactor(worktree): relocate worktree.yaml template to ConfigTemplate`

### Card 3: Author fresh `weft.ConfigTemplate`

- **Context:**
  - `internal/weft/config.go`
- **Edits:** none
- **Creates:**
  - `internal/weft/template.go`
- **Deletes:** none
- **Requirements:** Add a new `internal/weft/template.go` with an exported
  `func ConfigTemplate() string` returning a fully-commented YAML template for weft config whose
  single key is `pathspec` with the default value from `DefaultConfig().Pathspec` (`_lyx`).
  Follow the comment style of the board/worktree templates (a leading `#`, the key, and a short
  inline description). The commented `pathspec` line MUST parse as valid YAML when uncommented,
  yielding the value `_lyx` (verified by the card-4(b) `yaml.Unmarshal` assertion — no
  `config.Load` fixture needed).
- **Commit:** `feat(weft): add commented ConfigTemplate for weft.yaml`

### Card 4: Tests — template parse + init regression

- **Context:**
  - `internal/board/init.go`
  - `internal/board/template.go`
  - `internal/worktree/template.go`
  - `internal/weft/template.go`
  - `internal/board/init_test.go`
- **Edits:**
  - `internal/board/init_test.go`
- **Creates:**
  - `internal/board/template_test.go`
  - `internal/worktree/template_test.go`
  - `internal/weft/template_test.go`
- **Deletes:** none
- **Requirements:** (a) In `internal/board/template_test.go` and
  `internal/worktree/template_test.go`, assert `ConfigTemplate()` returns the exact expected
  commented string (the byte-for-byte regression baseline that proves the relocation preserved
  content). (b) In `internal/weft/template_test.go`, assert `weft.ConfigTemplate()` is non-empty
  and that uncommenting its `pathspec` line parses as YAML yielding `_lyx`. (c) In
  `internal/board/init_test.go`, confirm the existing assertions that `lyx init` writes
  `_lyx/config/board.yaml` and `_lyx/config/worktree.yaml` still pass with the relocated
  generators and that the written bytes equal `board.ConfigTemplate()` / `worktree.ConfigTemplate()`
  respectively.
- **Commit:** `test: cover relocated and new ConfigTemplate generators`

## Batch Tests

`verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/` runs the new
template tests plus the existing `init_test.go` regression and the modules' config tests. All
are pure-logic (no git fixtures), so no `integration` tag is needed. The board/worktree
identical-content assertions are the guard that the relocation is behaviour-preserving; the weft
test only checks parse + default since that generator is net-new.
