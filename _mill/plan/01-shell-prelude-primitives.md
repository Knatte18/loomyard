# Batch: shell-prelude-primitives

```yaml
task: "Spawned agent panes resolve the spawning lyx binary"
batch: "shell-prelude-primitives"
number: 1
cards: 2
verify: go test ./internal/shell/
depends-on: []
```

## Batch Scope

This batch extends the `internal/shell` seam with the three generic, provider-agnostic statement builders the reed prelude is composed from: a session-scoped env export, a live-`PATH` prepend, and a single-line statement joiner.
It is one batch because all three are pure string transforms over the same two dialect implementations, tested the same way, and because nothing outside `internal/shell` changes here — batch 2 is the sole consumer and depends on this batch's interface being final.
The external interface batch 2 consumes is exactly `Shell.ExportEnv`, `Shell.PrependPathEntry` and `Shell.Chain`.
No batch-local decision differs from the overview's Shared Decisions;
`chain-separator-is-semicolon` is the one that binds this batch's implementation most directly.

## Cards

### Card 1: Add ExportEnv, PrependPathEntry and Chain to the Shell seam

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add three methods to the `Shell` interface in `internal/shell/shell.go`, declared after the existing `Touch` method, each with a doc comment on the interface method:
  - `ExportEnv(key, value string) string` — returns a standalone statement that exports `key` with `value` into the shell session. Session-scoped in both dialects. The doc comment states that this differs deliberately from `WithEnv`, which is command-scoped on POSIX and therefore emits nothing usable for a pane whose command is empty.
  - `PrependPathEntry(dir string) string` — returns a standalone statement that prepends `dir` to the shell's own live `PATH`. The doc comment states that the statement must reference the live `PATH` variable rather than baking in a value computed by the calling Go process, and that it must not leave a trailing empty `PATH` entry when `PATH` is unset or empty.
  - `Chain(parts ...string) string` — joins statements into one single-line string safe to hand to a `send-keys` literal payload. The doc comment states the separator is `;` rather than `&&`, so a rejected earlier statement cannot suppress a later one, and that empty parts are dropped so a trailing separator is never emitted.

  Add an unexported package-level helper `chainStatements(parts ...string) string` in `internal/shell/shell.go` that drops empty strings from `parts` and joins the remainder with `"; "`.
  Both dialect implementations' `Chain` methods delegate to it, so the joining rule is declared once rather than duplicated per dialect.
  `internal/shell/shell.go` needs a `strings` import for this helper.

  Implement all three on `posixShell` in `internal/shell/posix.go`:
  - `ExportEnv` returns `"export " + key + "=" + p.Quote(value)`.
  - `PrependPathEntry` returns `export PATH=` followed by `p.Quote(dir)` followed by the literal `"${PATH:+:$PATH}"` (a double-quoted parameter expansion, concatenated directly onto the quoted directory with no separator of its own). The `${PATH:+…}` form expands to nothing when `PATH` is unset or empty, which is what keeps a trailing `:` — read by POSIX shells as the current directory — from ever being produced.
  - `Chain` returns `chainStatements(parts...)`.

  Implement all three on `pwshShell` in `internal/shell/pwsh.go`:
  - `ExportEnv` returns `"$env:" + key + " = " + p.Quote(value)`.
  - `PrependPathEntry` returns `"$env:PATH = "` followed by `p.Quote(dir)` followed by the literal ` + $(if ($env:PATH) { [IO.Path]::PathSeparator + $env:PATH })`. The `if` has no `else`, so the subexpression yields nothing when `$env:PATH` is empty or unset and the assignment is the directory alone.
  - `Chain` returns `chainStatements(parts...)`.

  Update `internal/shell/shell.go`'s file header comment so it names the three added mechanics alongside the quoting, call-operator and prompt-file-read mechanics it already lists.
  Do not change `Quote`, `Invoke`, `ReadFile`, `WithEnv`, `Touch`, `ForGOOS`, `Pwsh` or `Posix` in any way — `WithEnv`'s documented POSIX-vs-pwsh scope asymmetry in particular stays exactly as it is.
  The package keeps importing the standard library only.
- **Commit:** `feat(shell): add ExportEnv, PrependPathEntry and Chain to the Shell seam`

### Card 2: Cover the three new methods in both dialects

- **Context:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
- **Edits:**
  - `internal/shell/shell_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add table-driven tests following the file's existing per-dialect, per-method shape (`TestPosixShell_WithEnv` / `TestPwshShell_WithEnv` are the closest model).
  Both dialects are directly constructible via `Posix()` and `Pwsh()`, so every case runs on whichever host CI happens to be — no case may branch on `runtime.GOOS`.

  `TestPosixShell_ExportEnv` and `TestPwshShell_ExportEnv` each cover: an ordinary key and value; a value containing a space and a path separator; a value containing the dialect's own quote character, asserting the dialect's escape (`'\''` for POSIX, `''` for pwsh) is applied.
  Each asserts the exact emitted string, and asserts the emitted statement carries no trailing command fragment — the statement stands alone.

  `TestPosixShell_PrependPathEntry` and `TestPwshShell_PrependPathEntry` each cover: an ordinary directory; a directory containing a space; a directory containing the dialect's quote character.
  Beyond the exact-string assertion, each asserts two properties by substring so a future re-spelling that loses them fails loudly: the emitted statement references the live `PATH` variable (`$PATH` for POSIX, `$env:PATH` for pwsh) rather than only the quoted directory, and it carries the unset-`PATH` guard (`${PATH:+` for POSIX, `if ($env:PATH)` for pwsh).

  `TestShellChain` covers both dialects in one test, since `chainStatements` is shared: zero parts yields the empty string; one part yields that part unchanged; several parts are joined with `"; "`; a mix of empty and non-empty parts drops the empty ones and emits no leading, doubled or trailing separator.
  Assert in every case that the result contains no newline character, since `send-keys` submits one line at a time.
  Assert `Posix().Chain(...)` and `Pwsh().Chain(...)` agree for the same input.

  Leave the existing compile-time `var _ Shell = Posix()` / `var _ Shell = Pwsh()` assertions in place — they are what keeps the interface-completeness check current as the interface grows, so no new completeness test is needed.
  Update the file's header comment to name the three added methods in its list of what it table-tests.
- **Commit:** `test(shell): cover ExportEnv, PrependPathEntry and Chain in both dialects`

## Batch Tests

`verify: go test ./internal/shell/` runs `internal/shell/shell_test.go`, the package's only test file, which is where card 2's cases land.
The scope is exactly this batch's `Edits:` set: `shell.go`, `posix.go` and `pwsh.go` are the package's only production files, and no package outside `internal/shell` changes in this batch, so a wider scope would test nothing this batch can break.
The package is pure string transforms with no build tag and no spawn, so the run is sub-second and both dialects are exercised on whatever host runs it.
