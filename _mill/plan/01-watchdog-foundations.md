# Batch: watchdog-foundations

```yaml
task: 'reed: watchdog daemon'
batch: 'watchdog-foundations'
number: 1
cards: 5
verify: go test ./internal/shell/... ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch delivers every pure, dependency-free piece the rest of the task composes: the `internal/shell` seam's new `Touch` primitive, reed's new `watchdog` config key in both embedded templates, and a new `internal/reedengine/watchdog.go` holding the config validator, the five loop constants, the signal-file path accessor, the tmux-value quoter, and the `window-resized` hook command builder.
Nothing here calls tmux, reads a lock, or starts a goroutine — it is all functions of their arguments plus one `filepath.Join` off `Engine.stateDir()`.

It is one batch because the hook command string is the single artefact that binds all three pieces together: it needs `shell.Touch` for its shell fragment, `Engine.stateDir()` for its path, and it is the exact string batch 2's availability probe compares a `show-options -v` readback against byte for byte.
Splitting them would leave a builder in one batch and its only consumer's contract in another.

**External interface batch 2 consumes:** `watchdogOption`, `windowResizedHookName`, `(*Engine).resizeSignalPath`, `resizeHookCommand`, and the five `watchdog*` constants.

Batch-local decision: `internal/shell`'s existing four methods all return a *shell* fragment and know nothing of tmux, so `Touch` follows them exactly — it returns only the shell fragment.
The tmux-side wrapping (`run-shell -b <tmux-quoted fragment>`) lives in `reedengine`, because tmux command quoting is not pane-shell mechanics and the Shell Mechanics Seam does not model it.

## Cards

### Card 1: add a Touch primitive to the internal/shell seam

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a fifth method to the `Shell` interface in `internal/shell/shell.go`, declared after `WithEnv`:

  ```go
  	// Touch returns the shell syntax that creates path as an empty file, truncating it if it
  	// already exists.
  	Touch(path string) string
  ```

  Implement it on `posixShell` in `internal/shell/posix.go` as `": > " + p.Quote(path)` — the `:` no-op builtin plus an output redirection, so the fragment spawns no process at all.
  Implement it on `pwshShell` in `internal/shell/pwsh.go` as `"New-Item -ItemType File -Force -Path " + p.Quote(path) + " | Out-Null"` — `-Force` is what makes an existing file be truncated rather than an error, and `| Out-Null` suppresses the `FileInfo` object pwsh would otherwise emit.
  Give each implementation a godoc line in the same voice as its `WithEnv` neighbour, and state in the pwsh one that only the POSIX dialect is executed in practice today (see card 4's note on `run-shell` and GOOS).
  Do not change `Quote`, `Invoke`, `ReadFile`, or `WithEnv`.
  Do not add any import to `internal/shell` — the package is stdlib-only by the Shell Mechanics Seam and both new bodies need nothing beyond `strings`, which both files already import.
- **Commit:** `feat(shell): add a Touch primitive to the pane-shell seam`

### Card 2: tier-1 tests for shell.Touch

- **Context:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
- **Edits:**
  - `internal/shell/shell_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add table-driven tests for `Touch` on both dialects, following the file's existing per-method test shape.
  Cover: a plain absolute path; a path containing a space; a path containing a single quote (the POSIX `'\''` idiom and pwsh's doubled-quote idiom must both be exercised through `Quote`, not re-derived).
  Assert the POSIX form begins with the literal `": > "` and the pwsh form begins with `"New-Item -ItemType File -Force -Path "` and ends with `" | Out-Null"`.
  Add one assertion that `ForGOOS()` satisfies the widened interface — a compile-time `var _ Shell = Posix()` / `var _ Shell = Pwsh()` pair is sufficient if the file does not already carry one.
- **Commit:** `test(shell): cover Touch on both pane-shell dialects`

### Card 3: add the watchdog config key to Config and both embedded templates

- **Context:**
  - `internal/reedengine/mouse.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Watchdog string \`yaml:"watchdog"\`` field to the `Config` struct in `internal/reedengine/config.go`, placed immediately after the existing `Mouse` field and separated from it by a blank line, matching how `Mouse` is separated from `DebugLog`.
  Add a `watchdog:` line to **both** `internal/reedengine/template_posix.yaml` and `internal/reedengine/template_windows.yaml`, immediately after the existing `mouse:` line and before the `header:` block, identical in both files:
  `watchdog: ${env:LYX_REED_WATCHDOG:-on}` followed by a trailing `#` comment.
  The comment must state, in one line in the same voice as `mouse`'s: that it accepts `on`/`off`, that it enables the header pane's resize self-heal watch loop and the session's `window-resized` hook, that an invalid value fails `lyx reed up` loudly, that it takes effect on the next header-pane rebuild only (a server restart, a dead-header heal, or `lyx reed down` + `up`) so flipping it does not stop an already-running watcher, and that an already-materialized `reed.yaml` keeps whatever value it holds since reconcile is key-based and never rewrites a value.
  Config Strictness Invariant: reed is a `LoadOrTemplate` adopter, so the key MUST exist in both templates — a hub whose `reed.yaml` predates this key resolves the template default `on`.
  Do not add a `Watchdog` default anywhere in Go code; the template is the only default.
- **Commit:** `feat(reed): add the watchdog config key to Config and both templates`

### Card 4: add watchdog.go with the validator, constants, signal path, and hook command builder

- **Context:**
  - `internal/reedengine/mouse.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/overlay.go`
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/reedengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/watchdog.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/watchdog.go` in `package reedengine` with a file-header comment stating that this file owns the resize watchdog's pure surface — the config validator, the loop's fixed timings, the signal file's location, and the `window-resized` hook command string — and that everything here is I/O-free apart from `resizeSignalPath`'s `filepath.Join`.

  Declare, with godoc on each:

  - `watchdogOption(raw string) (bool, error)` — the validator. It follows `mouseOption` (`mouse.go:15`) in behaviour exactly: `strings.ToLower(strings.TrimSpace(raw))`, `"on"` yields `(true, nil)`, `"off"` yields `(false, nil)`, and every other value including the empty string yields `(false, error)` whose message names the offending value in the same shape as `mouseOption`'s (`invalid watchdog value %q: want "on" or "off"`).
    Its godoc must state why the return type is `bool` rather than `mouseOption`'s `string`: the `watchdog` value is never handed to tmux, and every consumer wants a boolean enable.
  - The five constants, as a single `const` block with a comment naming them the loop's fixed, non-configurable timings:
    `watchdogDebounceQuiet = 200 * time.Millisecond`, `watchdogSignalTick = 100 * time.Millisecond`, `watchdogPollCycle = 2 * time.Second`, `watchdogRetryBaseDelay = 200 * time.Millisecond`, and `watchdogMaxAttempts = 3`.
  - `resizeSignalFileName = "reed-resize.signal"` — a string constant, with a godoc line stating the file's existence alone is the signal and that the watcher consumes it by removing it, so no timestamp comparison is ever involved.
  - `windowResizedHookName = "window-resized"` — the tmux hook option name, declared once so the install, the unset, and the readback cannot drift.
  - `func (e *Engine) resizeSignalPath() string` — `filepath.Join(e.stateDir(), resizeSignalFileName)`.
    Its godoc must state that `stateDir()` is the only permitted route to this path under the Durable-vs-Ephemeral State Invariant, and that one signal file per worktree is what keeps sibling worktrees on the shared per-hub tmux server from colliding.
  - `func tmuxQuoteValue(s string) string` — wraps `s` in tmux double quotes, backslash-escaping any `\`, `"`, or `$` in `s` first.
    Its godoc must state that tmux parses a hook's value as a tmux command line with its own word splitting, so the shell fragment inside it has to survive as one tmux word, and that `$` is escaped because tmux expands it inside double quotes.
  - `func resizeHookCommand(sh shell.Shell, signalPath string) string` — returns `"run-shell -b " + tmuxQuoteValue(sh.Touch(signalPath))`.
    Its godoc must record two live-verified facts: `run-shell` MUST carry `-b`, because without it the tmux **server** blocks while the command runs; and this exact string round-trips byte-identically through `show-options -v`, which is what makes batch 2's exact-match availability probe viable.
    It must also record that `run-shell` is executed by the tmux server's own shell rather than the pane shell `internal/shell` otherwise models, so `shell.ForGOOS()` is the closest available approximation for dialect selection and only the POSIX dialect is ever executed in practice (Windows runs poll-only, see batch 2).

  Do not add a `Watch` method, a state machine, or any tmux round trip in this file — those are batches 2 and 3.
  Do not import `internal/lyxcwd` (Told-Geometry Invariant).
- **Commit:** `feat(reed): add the watchdog validator, constants, signal path, and hook command`

### Card 5: tier-1 tests for the watchdog foundations

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/mouse_test.go`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/template.go`
  - `internal/reedengine/template_posix.go`
  - `internal/reedengine/template_windows.go`
  - `internal/shell/shell.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/watchdog_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/watchdog_test.go` (untagged — nothing here spawns a process or sleeps, per the Test Tier Purity Invariant) with a file-header comment naming what it pins.

  Cover:

  - `watchdogOption` as a table, mirroring `mouse_test.go`'s own table: `"on"`, `"ON"`, `" on "` yield `(true, nil)`; `"off"`, `"OFF"`, `" off "` yield `(false, nil)`; `""`, `"1"`, `"true"`, `"yes"`, `"onn"` each yield an error whose message contains the offending value.
  - The embedded-template default for **both** GOOS variants: parse each of `template_posix.yaml` and `template_windows.yaml` and assert each declares a `watchdog` key whose resolved default is `on`.
    Use whatever mechanism `config_test.go` already uses to reach the two templates and resolve `${env:...}` defaults; do not invent a second one, and do not read the `.yaml` files by hand-rolled path if the package already exposes them.
  - `resizeHookCommand` for `shell.Posix()`: assert the result starts with the literal `run-shell -b `, that the remainder is double-quoted, and that the whole string equals `run-shell -b ": > '/tmp/wt/.lyx/reed-resize.signal'"` for `signalPath = "/tmp/wt/.lyx/reed-resize.signal"`.
    Assert there is no `-a` anywhere in the string.
  - `resizeHookCommand` for `shell.Pwsh()`: assert the pwsh fragment is present and correctly tmux-quoted for the same path.
  - `resizeHookCommand` for a path containing a space, on both dialects, asserting the path survives as one shell argument.
  - `tmuxQuoteValue` directly: a value containing `"`, a value containing `\`, and a value containing `$` are each backslash-escaped inside the surrounding double quotes.
  - `(*Engine).resizeSignalPath` against a hand-built `Engine` with a known `Geometry.AnchorPath`: assert it equals `<AnchorPath>/.lyx/reed-resize.signal`, and assert it is built from `stateDir()` by asserting `filepath.Dir` of the result equals `e.stateDir()`.
- **Commit:** `test(reed): cover the watchdog validator, constants, and hook command string`

## Batch Tests

`verify: go test ./internal/shell/... ./internal/reedengine/...` runs the two packages this batch touches.
`internal/shell` covers the new `Touch` implementations via `shell_test.go`; `internal/reedengine` covers the new `watchdog_test.go` plus the whole existing untagged reed suite, which must stay green — card 3's `Config` field addition and both template edits are the only changes that could disturb it (`config_test.go` and `template_test.go`-style assertions over the embedded templates are the ones to watch).
The scope is per-batch, not the repo-wide suite: no other package imports `internal/shell`'s new method or reed's new key at this point.
