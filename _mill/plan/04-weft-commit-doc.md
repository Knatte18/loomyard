# Batch: weft-commit-doc

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "weft-commit-doc"
number: 4
cards: 2
verify: go test ./internal/weft/...
depends-on: [2]
```

## Batch Scope

Documents `weft commit`'s fixed commit message (W4/W9) and clears the two stale
`warp status` doc-comment references in weft test files (W7 fallout). Depends on batch 2
because it co-edits `internal/weft/cli.go` (which batch 2 gave a group `RunE` + PreRunE
guard) and `internal/weft/cli_test.go`. Batch-local decision: document-only — no
`-m/--message` flag is added; the message stays the fixed `commitMessage` const `"weft sync"`.

## Cards

### Card 15: weft commit Long documenting the fixed message (W4/W9)

- **Context:**
  - `internal/weft/weft.go`
  - `internal/weft/sync.go`
- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a `Long` to the `commitCmd` (`weft commit`) in `weft.Command()`
  stating that the commit message is the fixed string `"weft sync"` (the `commitMessage`
  const in `weft.go`), that staging is scoped to the configured pathspec, and pointing to
  `lyx weft push` (commit + push) and `lyx weft sync` (async commit + push). Do NOT claim the
  message is auto-generated from changed files (the reverted draft's wording was wrong). Do
  NOT add a `-m`/`--message` flag. Keep the existing `Short` and `RunE` unchanged.
- **Commit:** `docs(weft): document the fixed commit message in weft commit help`

### Card 16: weft commit help test + stale comment fixes

- **Context:**
  - `internal/weft/cli.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/weft/cli_test.go`
  - `internal/weft/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `cli_test.go`: add a test asserting `weft commit --help` output contains the documented
    fixed-message wording (substring such as `weft sync`) and does NOT contain `--message`
    or `-m`. (Drive via `weft.RunCLI([]string{"commit", "--help"})`.)
  - `cli_test.go:121` and `status_test.go:45`: update the doc comments that say
    `warp status` to `warp pairs` (comment text only — no code or assertion change). These
    are forward-looking doc fixes for the batch-3 rename; updating the comment does not depend
    on the rename existing.
- **Commit:** `test(weft): cover commit help; fix stale warp status comments`

## Batch Tests

`verify: go test ./internal/weft/...` runs the weft package: the new `weft commit --help`
content assertion and the existing weft suite (unchanged behavior — the comment edits are
non-functional). The weft group `RunE`/guard behavior from batch 2 remains covered by batch
2's tests and is re-run here as a regression check.
