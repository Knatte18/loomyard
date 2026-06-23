# Batch: config-edit-machinery

```yaml
task: 'weft producers: _lyx/config, lyx config, codeguide'
batch: config-edit-machinery
number: 3
cards: 2
verify: go test ./internal/config/
depends-on: []
```

## Batch Scope

Add the centralized, **weft-agnostic** config-edit machinery to `internal/config`: resolve a
module's YAML path, scaffold it from a passed-in template if missing, open it in an injectable
editor, validate (re-parse YAML), with an explicit re-edit loop and a deterministic abort
contract. This is the load/edit core the `lyx-config-command` batch composes with `weft sync`.
It deliberately does NOT import `internal/weft` (that would be circular — `weft` imports
`config`). The editor is an injected `EditorFunc` so tests drive it without a real interactive
editor. This batch is independent of the others (it takes the template as a string parameter, so
it does not depend on the `module-config-templates` batch). External interface consumed by
Batch 4: `config.Edit`, `config.EditorFunc`, `config.DefaultEditor`, and the sentinel
`config.ErrAborted`.

## Cards

### Card 8: Add `config.Edit` + injectable editor

- **Context:**
  - `internal/config/config.go`
- **Edits:** none
- **Creates:**
  - `internal/config/edit.go`
- **Deletes:** none
- **Requirements:** In a new `internal/config/edit.go` add:
  - `type EditorFunc func(path string) error` — opens an editor on `path`; a non-nil return
    signals editor failure / user-abort.
  - `var ErrAborted = errors.New("config edit aborted")` — sentinel returned when the edit is
    aborted (file left untouched by `Edit` beyond any scaffold already written; caller skips sync).
  - `func DefaultEditor(path string) error` — resolves the editor command from `$VISUAL` then
    `$EDITOR`, falling back to `notepad` on Windows (`runtime.GOOS == "windows"`) and `vi`
    elsewhere; runs it via `os/exec` with `Stdin/Stdout/Stderr` wired to the process std streams;
    returns a non-nil error if the editor exits non-zero.
  - `func Edit(baseDir, module, template string, edit EditorFunc) error` with this flow: call
    `FindBaseDir(baseDir)` (propagate the not-initialized error); compute
    `path = filepath.Join(baseDir, "_lyx", "config", module+".yaml")`; if `path` does not exist,
    write `template` to it (scaffold, `0o644`, creating `_lyx/config/` if needed); then loop:
    record the file bytes, call `edit(path)` — if it returns an error, return `ErrAborted`
    (wrapping the editor error); re-read the bytes and `yaml.Unmarshal` into a
    `map[string]any` to validate syntax; on success return `nil`; on parse failure, if the bytes
    are unchanged from the pre-`edit` snapshot return `ErrAborted` (operator saved without
    fixing), otherwise print the parse error to `os.Stderr` and loop to re-open the editor.
  - Validation is syntactic only (the file must parse as YAML); do not enforce known keys.
- **Commit:** `feat(config): add Edit machinery with injectable editor and abort contract`

### Card 9: Tests — Edit scaffold/validate/abort

- **Context:**
  - `internal/config/edit.go`
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- **Edits:** none
- **Creates:**
  - `internal/config/edit_test.go`
- **Deletes:** none
- **Requirements:** In `internal/config/edit_test.go`, using a temp dir with `_lyx/` created and
  a fake `EditorFunc`, cover: (a) scaffold-when-missing — `Edit` writes `template` to
  `_lyx/config/<module>.yaml` before the editor runs (assert the fake editor sees the template
  bytes); (b) edit of an existing file — fake editor rewrites valid YAML, `Edit` returns nil and
  the file holds the new bytes; (c) re-edit loop — first fake-editor pass writes invalid YAML
  (changed), second pass writes valid YAML; assert the editor is invoked twice and `Edit` returns
  nil; (d) abort on unchanged-after-failure — fake editor writes invalid YAML then leaves it
  unchanged; assert `Edit` returns `ErrAborted`; (e) abort on editor error — fake editor returns
  an error; assert `Edit` returns `ErrAborted`; (f) not-initialized — calling `Edit` with a
  `baseDir` lacking `_lyx/` returns the `FindBaseDir` not-initialized error (not `ErrAborted`).
- **Commit:** `test(config): cover Edit scaffold, re-edit loop, and abort paths`

## Batch Tests

`verify: go test ./internal/config/` runs the new `edit_test.go` alongside the existing
`config_test.go`. All pure-logic (fake editor, temp dirs) — no `integration` tag. The abort
cases (d, e) pin the deterministic abort contract the discussion requires; the not-initialized
case (f) confirms the `FindBaseDir` error is surfaced rather than swallowed.
