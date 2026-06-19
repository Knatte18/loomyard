# Batch: ide-watcher-exclude

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
batch: ide-watcher-exclude
number: 4
cards: 1
verify: go test ./internal/ide/
depends-on: []
```

## Batch Scope

A single, independent change: the ide module owns all `.vscode/settings.json` writes, so the `files.watcherExclude` entry that prevents VS Code's file watcher from locking the `_lyx` junction (the millhouse issue #498 hazard) is seeded in the ide module's default settings block — never by `worktree add`. This batch has no dependency on the geometry batch (it touches only static settings JSON) and runs in parallel with everything else. Batch-local decision: only `**/_lyx/**` is added now; `**/_codeguide/**` is deferred to task 008 (per the discussion's codeguide-geometry-only scope).

## Cards

### Card 20: seed files.watcherExclude in writeVSCodeConfig

- **Context:**
  - `internal/ide/spawn.go`
  - `internal/gitignore/gitignore.go`
- **Edits:**
  - `internal/ide/vscode.go`
  - `internal/ide/vscode_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `writeVSCodeConfig` (`internal/ide/vscode.go`), add a `"files.watcherExclude"` key to the `settings` map literal written when `settings.json` is absent, with value `map[string]any{"**/_lyx/**": true}`. Do not add a `_codeguide` entry (deferred to task 008). Leave the existing `workbench.colorCustomizations`, `window.title`, and other keys unchanged, and keep the "write only if absent" behavior intact. In `internal/ide/vscode_test.go`, extend the settings-writing test to assert the generated `settings.json` parses and its `files.watcherExclude` map contains the `**/_lyx/**` key set to `true`.
- **Commit:** `feat(ide): seed _lyx watcherExclude in vscode settings`

## Batch Tests

`verify: go test ./internal/ide/` runs the `internal/ide` package, covering the extended `vscode_test.go` assertion plus the existing ide tests. The change is a pure additive key in a generated JSON file; the test verifies the key is present and correctly typed.
