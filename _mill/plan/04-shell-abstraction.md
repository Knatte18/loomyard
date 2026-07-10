# Batch: shell-abstraction

```yaml
task: "Facilitate Linux support (Win11-side prep)"
batch: "shell-abstraction"
number: 4
cards: 5
verify: GOOS=linux go build ./internal/shell/... ./internal/shuttleengine/... && go test ./internal/shell/... ./internal/shuttleengine/...
depends-on: []
```

## Batch Scope

This batch introduces `internal/shell`, a **provider-invariant** leaf owning pane-shell
*mechanics* — argument quoting, the call operator, and the prompt-file read idiom — with a
`pwsh` implementation and a `posix` implementation selected by `runtime.GOOS`. It then routes
`internal/shuttleengine/claudeengine`'s command construction through it, removing the inline
`pwshSingleQuote` and the hardcoded `& <bin> (Get-Content -Raw <path>)` idiom. Claude *content*
— the `--session-id`, `--settings`, `--resume`, `--model`, `--effort`,
`--dangerously-skip-permissions` flags — stays inside `claudeengine` per the Shuttle
Provider-Seam Invariant; `internal/shell` never sees a Claude flag.

Both shell impls are plain (untagged) Go so **both are compiled and unit-tested on the Windows
host** — the posix impl is not build-tagged (it must be host-testable, and it is only *selected*
at runtime on Linux). The separate git-bash hook-path conversion (`PosixPath` in
`shuttleengine/posix.go`, called from `claudeengine.Prepare`) is a **different axis** (the hook
interpreter, git-bash on Windows) and is deliberately **left untouched** — folding it into the
posix pane-shell impl would make it unreachable on Windows and regress the hook path.

**External interface:** `shell.Shell` interface + `shell.ForGOOS()` / `shell.Pwsh()` /
`shell.Posix()` constructors. This batch shares no file with any other batch (root, parallel).

## Cards

### Card 11: internal/shell package (pwsh + posix impls)

- **Context:**
  - `internal/shuttleengine/claudeengine/command.go`
- **Edits:** none
- **Creates:**
  - `internal/shell/shell.go`
  - `internal/shell/pwsh.go`
  - `internal/shell/posix.go`
  - `internal/shell/shell_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `shell` (import path
  `github.com/Knatte18/loomyard/internal/shell`), stdlib-only. In `shell.go` define
  `type Shell interface { Quote(s string) string; Invoke(bin string) string; ReadFile(path string) string }`,
  a `ForGOOS() Shell` that returns the pwsh impl on `runtime.GOOS == "windows"` and the posix
  impl otherwise, and exported constructors `Pwsh() Shell` and `Posix() Shell` (so both impls are
  directly host-testable regardless of OS). In `pwsh.go`: `Quote` wraps in single quotes doubling
  embedded `'` (the existing `pwshSingleQuote` behavior — `'`→`''`); `Invoke(bin)` returns
  `"& " + Quote(bin)` (pwsh call operator); `ReadFile(path)` returns
  `"(Get-Content -Raw " + Quote(path) + ")"`. In `posix.go`: `Quote` wraps in single quotes
  escaping embedded `'` as `'\''`; `Invoke(bin)` returns `Quote(bin)` (no call operator);
  `ReadFile(path)` returns `"\"$(cat " + Quote(path) + ")\""` — the command-substitution is
  **double-quoted** so the whole prompt file becomes a single argument, reproducing pwsh's
  `Get-Content -Raw` single-argument-prompt semantics (a documented, benign divergence: `$(cat)`
  strips trailing newlines whereas `Get-Content -Raw` is byte-exact — benign for prompt text;
  add a code comment stating this rather than implying byte-equality). In `shell_test.go`
  table-test both impls: `Quote` on plain, spaces, single-quote, and mixed inputs for each
  scheme; `Invoke` and `ReadFile` exact-string outputs for each impl. Include the pwsh-quote
  cases moved from `claudeengine`'s `TestPwshSingleQuote` (`claude`→`'claude'`, `C:\a b\c`
  round-trip, `it's`→`'it''s'`, `'a'b'`→`'''a''b'''`).
- **Commit:** `feat(shell): add provider-invariant pane-shell mechanics (pwsh + posix)`

### Card 12: Route claudeengine command building through shell

- **Context:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/command.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `command.go`, remove `pwshSingleQuote` (`command.go:65-67`) and rebuild
  the two command builders via `internal/shell`. Change `buildLaunchCmd` (`command.go:104-119`)
  and `buildResumeCmd` (`command.go:127-132`) to each take a leading `sh shell.Shell` parameter.
  `buildLaunchCmd` composes `sh.Invoke(bin) + " " + sh.ReadFile(promptPath) + " --session-id " +
  sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath)`, then the same optional
  ` --model ` + `sh.Quote(model)`, ` --effort ` + `sh.Quote(effort)`, and
  ` --dangerously-skip-permissions` (when `!interactive`) tail — every interpolated value goes
  through `sh.Quote`, no raw pwsh idiom remains. `buildResumeCmd` composes
  `sh.Invoke(bin) + " --resume " + sh.Quote(sessionID) + " --settings " + sh.Quote(settingsPath)`.
  Keep both single-line (no `\r`/`\n`) and keep `claudeBinary`/`maxLaunchPromptBytes`/effort
  validation as-is. Add the `internal/shell` import.
- **Commit:** `refactor(claudeengine): build launch/resume commands via internal/shell`

### Card 13: Select shell in Prepare + update command tests

- **Context:**
  - `internal/shell/shell.go`
- **Edits:**
  - `internal/shuttleengine/claudeengine/claudeengine.go`
  - `internal/shuttleengine/claudeengine/command_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `claudeengine.go`'s `Prepare` (`claudeengine.go:63-117`), obtain
  `sh := shell.ForGOOS()` and pass it into the `buildLaunchCmd(...)` and `buildResumeCmd(...)`
  calls that populate the returned `Launch`. Do not touch the `PosixPath` call at
  `claudeengine.go:96-100` — that git-bash hook-path conversion stays exactly as-is (separate
  axis). Add the `internal/shell` import. In `command_test.go`, delete `TestPwshSingleQuote`
  (its assertions now live in `internal/shell`'s `shell_test.go`), and update `TestBuildLaunchCmd`
  and `TestBuildResumeCmd` to pass `shell.Pwsh()` as the new first argument — the existing exact
  pwsh expected strings remain valid under the pwsh impl. Optionally add one `shell.Posix()` row
  to `TestBuildLaunchCmd` asserting the posix form (`'claude' "$(cat '/run/prompt.md')"
  --session-id 'abc-123' ...`) to prove the seam is shell-agnostic.
- **Commit:** `refactor(claudeengine): select shell family in Prepare; move pwsh-quote tests`

### Card 14: Record the shell-mechanics seam invariant

- **Context:**
  - `internal/shell/shell.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a short "Shell Mechanics Seam" invariant to `CONSTRAINTS.md` (same
  bold-heading style as the others): pane-shell command strings — argument quoting, the call
  operator, and the prompt-file read idiom — are built ONLY via `internal/shell`;
  `claudeengine` (and any future provider engine) never emits raw pwsh/posix shell syntax
  directly, and `internal/shell` stays provider-invariant (stdlib-only, no Claude flags, marker
  strings, or hook shapes). State it is a **review obligation** today (a grep-guard test — e.g.
  asserting the `Get-Content -Raw` idiom appears only under `internal/shell` — is a cheap future
  machine-check, deferred per YAGNI). Note it complements the existing Shuttle Provider-Seam
  Invariant.
- **Commit:** `docs(constraints): record the shell-mechanics seam invariant`

### Card 15: Add internal/shell to the module overview

- **Context:**
  - `internal/shell/shell.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per the Documentation Lifecycle (cross-cutting infra ships its doc in the
  same task), add `internal/shell` to `docs/overview.md`: include it in the `internal/`
  directory tree and in the shared-infrastructure / portability-family sentence alongside
  `internal/proc`, `internal/fslink`, and `internal/fsx`, described as the provider-invariant
  pane-shell mechanics leaf (pwsh + posix). Do not restructure the doc; a minimal additive edit.
- **Commit:** `docs(overview): list internal/shell in the module map`

## Batch Tests

`verify` cross-compiles the touched packages for Linux (`GOOS=linux go build ./internal/shell/...
./internal/shuttleengine/...`) then runs `go test ./internal/shell/... ./internal/shuttleengine/...`.
Host tests cover both shell impls (`shell_test.go`: quoting/invoke/read-file for pwsh and posix,
including the migrated pwsh-quote cases), the rerouted claudeengine command builders
(`command_test.go`, exact pwsh strings preserved plus an optional posix row), and the untouched
provider-seam import test (`seam_enforcement_test.go` still passes — `internal/shell` is a leaf
neither side's ban touches). No new failure mode reaches the CLI; the shell impls are pure.
