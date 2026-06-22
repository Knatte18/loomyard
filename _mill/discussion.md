# Discussion: Extract internal/vscode; keep ide IDE-generic

```yaml
task: Extract internal/vscode; keep ide IDE-generic
slug: extract-internal-vscode
status: discussing
parent: main
```

## Problem

`internal/ide` currently conflates two responsibilities: the IDE-generic
orchestration (CLI dispatch, the spawn flow, the interactive worktree menu) and
the VS-Code-specific details (the `.vscode/` settings/tasks JSON schema, the
title-bar color palette and collision scan, the `cmd /c code` launch command).
The VS-Code-specific files carry zero IDE-generic logic.

Extracting the VS Code specifics into a new `internal/vscode` package lets `ide`
stay a thin dispatcher/menu/spawn layer and signals clearly that those files are
"VS Code config, not IDE generic." This is a **behavior-preserving** refactor —
no new functionality, no behavior change. The existing `ide` tests are the
guardrail. It pairs with the broader cross-OS work (launch differs per OS), but
this task is purely the physical extraction.

## Scope

**In:**

- Create new package `internal/vscode` (module path
  `github.com/Knatte18/loomyard/internal/vscode`).
- Move `writeVSCodeConfig` (from `internal/ide/vscode.go`) →
  `internal/vscode`, exported as `WriteConfig`.
- Move `pickColor` + `palette` + `mainColor` (from `internal/ide/color.go`) →
  `internal/vscode`. `pickColor` exported as `PickColor`; `palette` and
  `mainColor` stay **unexported**. (`palette` is referenced by both `PickColor`
  and `color_test.go`; `mainColor` is consumed **only** by `color_test.go`
  (`:36`, `:83`) — `PickColor` skips green by starting its index loop at
  `palette[1]` and never references `mainColor` — so `mainColor` survives the
  move purely as test-visible state and must live in a non-`_test.go` file the
  white-box tests can reach, e.g. `color.go`.)
- Move `launchCode` (from `launch_windows.go` / `launch_other.go`, with their
  `//go:build` tags) → `internal/vscode`, exported as `Launch`.
- Move `ErrIDEUnsupported` → `internal/vscode`, **renamed** to `ErrUnsupported`
  (message updated to `"vscode launch unsupported on this platform"`).
- Migrate the two white-box tests that exercise the moved symbols
  (`color_test.go`, `vscode_test.go`) into `internal/vscode`, updating call
  sites to the exported names.
- Rewire `internal/ide/spawn.go`: `Spawn` calls `vscode.WriteConfig` and
  `vscode.PickColor`; the `codeLauncher` seam becomes `var codeLauncher =
  vscode.Launch`.
- Update the `ide` package doc comment (top of `cli.go`) to describe the
  generic spawn/menu/dispatch responsibility, with VS Code details delegated to
  `internal/vscode`.

**Out:**

- **No backend interface / pluggability abstraction.** `ide` imports
  `internal/vscode` directly and calls its functions. "IDE-generic" here means
  free of VS Code *details* (file formats, palette, launch command), not
  runtime-swappable backends. (YAGNI — one backend exists.)
- **No move of the `codeLauncher` seam.** It stays in `internal/ide/spawn.go` so
  the three test files that stub it (`spawn_test.go`, `cli_test.go`,
  `menu_test.go`) need no change.
- **No change to** `cli.go` dispatch logic, `spawn.go` flow shape, or `menu.go`
  picker behavior — only the call targets and the package doc comment change.
- **No behavior change**: same files written, same palette, same launch command,
  same gitignore registration, same error semantics (only the error *symbol
  name* changes).
- No change to `cmd/lyx/main.go` — it imports `ide` and calls `ide.RunCLI`,
  which is untouched.
- No new tests for `Launch` (it has none today; covered only via the seam).

## Decisions

### backend-coupling-direct-import

- Decision: `ide` imports `internal/vscode` directly and calls
  `vscode.WriteConfig` / `vscode.PickColor` / `vscode.Launch`. No `Backend`
  interface is introduced.
- Rationale: There is exactly one IDE backend (VS Code). An interface would add
  indirection and code for a swappability that nothing needs. "Keep ide
  IDE-generic" is satisfied by factoring out the *details*, not by abstracting
  the *backend*.
- Rejected: Declaring a `Backend` interface in `ide` that `vscode` implements —
  speculative generality (YAGNI), more surface area, no current consumer.

### launcher-seam-stays-in-ide

- Decision: The injectable `var codeLauncher = launchCode` seam in
  `spawn.go` is retained in `internal/ide` and re-pointed to `vscode.Launch`
  (`var codeLauncher = vscode.Launch`).
- Rationale: The seam is stubbed by three white-box test files (`spawn_test.go`,
  `cli_test.go`, `menu_test.go`). Keeping it in `ide` means those three files
  need zero change; only the two tests that directly exercise moved symbols
  migrate. Moving the seam into `vscode` would force reworking all three
  stubbing files across a package boundary for no benefit.
- Rejected: Moving the seam into `vscode` — more test churn, no clear benefit.

### exported-api-naming

- Decision: Export the moved functions as `vscode.WriteConfig`,
  `vscode.PickColor`, `vscode.Launch`.
- Rationale: Idiomatic Go — package-qualified call sites read cleanly and avoid
  stutter (`vscode.WriteConfig`, not `vscode.WriteVSCodeConfig`).
- Rejected: Keeping `WriteVSCodeConfig` / `LaunchCode` — stutters against the
  package name.

### error-rename-errunsupported

- Decision: `ErrIDEUnsupported` becomes `vscode.ErrUnsupported`, returned by
  `vscode.Launch` on non-Windows. Message updated to `"vscode launch
  unsupported on this platform"`.
- Rationale: It lives in the `vscode` package now; `vscode.ErrUnsupported` reads
  correctly at any consumer and drops the now-misleading "IDE" prefix. No
  consumer outside the package references the symbol (verified: only
  `launch_other.go` returns it; `cli.go`/`spawn.go`/`menu.go` do not branch on
  it), so the rename is contained.
- Rejected: Keeping `ErrIDEUnsupported` — name says "IDE" while living in
  `vscode`; mildly inconsistent.

## Technical context

Package under change: `internal/ide`. New package: `internal/vscode`.

**Current `internal/ide` files and disposition:**

| File | Symbols | Disposition |
|---|---|---|
| `cli.go` | `RunCLI` (dispatch) | stays; package doc comment rewritten |
| `spawn.go` | `Spawn` flow, `codeLauncher` seam | stays; call targets rewired to `vscode.*` |
| `menu.go` | `Menu` picker | stays (calls `Spawn`) |
| `vscode.go` | `writeVSCodeConfig` | move → `internal/vscode` as `WriteConfig` |
| `color.go` | `pickColor`, `palette`, `mainColor`, `ErrIDEUnsupported` | move → `internal/vscode`; export `PickColor`, rename err to `ErrUnsupported` |
| `launch_windows.go` / `launch_other.go` | `launchCode` (build-tagged) | move → `internal/vscode` as `Launch`, tags preserved |

**Suggested new `internal/vscode` file layout** (mill-plan may adjust):

- `config.go` — `WriteConfig` (+ package doc comment `// Package vscode ...`).
- `color.go` — `PickColor`, `palette`, `mainColor`.
- `launch_windows.go` / `launch_other.go` — `Launch` (build tags preserved).
- `ErrUnsupported` must be defined in a **build-tag-neutral** file (both
  build variants return/reference it) — e.g. in `color.go` (as today) or
  `config.go`. Do **not** put it inside a `//go:build`-tagged file.
- Tests: `config_test.go` (from `vscode_test.go`), `color_test.go` (from
  `color_test.go`), updated to call exported names; remain white-box
  (`package vscode`) so they keep access to `palette` / `mainColor`.

**Signatures (exported forms):**

- `func WriteConfig(worktreeDir, relpath, slug, color string) error`
  (was `writeVSCodeConfig`). Internally calls `gitignore.Ensure(dir,
  ".vscode/")` — that dependency moves with it.
- `func PickColor(l *paths.Layout) string` (was `pickColor`). Takes a
  `*paths.Layout`; reads sibling `.vscode/settings.json` files.
- `func Launch(worktreeDir string) error` (was `launchCode`). Windows execs
  `cmd /c code <dir>`; non-Windows returns `ErrUnsupported`.

**Rewired `ide.Spawn`** (shape unchanged):

```go
color := vscode.PickColor(l)
if err := vscode.WriteConfig(worktreeDir, l.RelPath, slug, color); err != nil { ... }
openDir := filepath.Join(worktreeDir, l.RelPath)
if err := codeLauncher(openDir); err != nil { ... }   // codeLauncher = vscode.Launch
```

**Dependencies of the moved code:** `internal/gitignore` (stable),
`internal/paths` (stable), stdlib (`encoding/json`, `os`, `path/filepath`,
`strings`, `errors`, `os/exec`, `syscall`). No new external deps.

**Blast radius (verified by grep):**

- Only `cmd/lyx/main.go` imports `internal/ide` (via `ide.RunCLI`); it is
  **not** touched.
- No symbol being moved is referenced anywhere outside `internal/ide`.
- `ErrIDEUnsupported` is referenced only at its definition (`color.go`) and its
  single return site (`launch_other.go`).

## Constraints

From `CONSTRAINTS.md` (Path Invariant):

- All cwd / worktree-root queries must go through `internal/paths.Getwd()` and
  `internal/paths.Resolve()`. Raw `os.Getwd` and raw `git rev-parse
  --show-toplevel` are banned outside `internal/paths` and `cmd/lyx/main.go`.
- **Compliance:** None of the moved code calls `os.Getwd` or shells out to git
  — `PickColor` takes an already-resolved `*paths.Layout`; `WriteConfig` and
  `Launch` take plain string paths. The new `internal/vscode` package therefore
  introduces no path-invariant violations. `internal/paths/enforcement_test.go`
  scans the whole tree at `go test` time and must stay green.

From project `CLAUDE.md` (fslink): not relevant — this task adds no filesystem
links. (`.vscode/` directory creation uses plain `os.MkdirAll`, unchanged.)

Other:

- Preserve the `//go:build integration` tag on the unchanged `cli_test.go`
  **and** `menu_test.go` (both carry it — `cli_test.go:1`, `menu_test.go:1`),
  and the `//go:build windows` / `//go:build !windows` tags on the launch files.
- Go module path prefix is `github.com/Knatte18/loomyard`.

## Testing

Behavior-preserving refactor → the existing tests are the guardrail; no new
test scenarios are required, only relocation and call-site updates.

**Migrates to `internal/vscode` (white-box, `package vscode`):**

- `color_test.go` → exercises `PickColor` / `palette` / `mainColor`:
  - `TestPickColorNeverReturnsGreen`
  - `TestPickColorFirstUnusedNonGreen`
  - `TestPickColorWrapAroundAllUsed`
  - `TestPickColorIgnoresUnreadable`
  - Update `pickColor` → `PickColor` at call sites; `palette` / `mainColor`
    references unchanged (still in-package).
- `vscode_test.go` → `config_test.go`, exercises `WriteConfig`:
  - `TestWriteVSCodeConfigCreatesFilesWhenAbsent`
  - `TestWriteVSCodeConfigDoesNotClobber`
  - `TestWriteVSCodeConfigRegistersInGitignore`
  - Update `writeVSCodeConfig` → `WriteConfig` at call sites.

**Stays in `internal/ide` unchanged** (stub `codeLauncher`, never reference
moved symbols by name):

- `spawn_test.go` (`TestSpawnGeneratesConfig`, `TestSpawnDoesNotClobber`,
  `TestSpawnCallsCodeLauncher`, `TestSpawnColorSelection`) — asserts on written
  `settings.json`/`tasks.json` content, which `vscode.WriteConfig` still
  produces identically.
- `cli_test.go` (`//go:build integration`; `TestRunCLISpawnDispatch` et al.).
- `menu_test.go` (all `TestMenu*`).

**Verification gate (must all pass):**

- `go build ./...`
- `go test ./internal/ide/... ./internal/vscode/...`
- `go test ./internal/paths/...` (path-invariant enforcement scan stays green)
- `go vet ./internal/ide/... ./internal/vscode/...`
- `go test -tags integration ./internal/ide/...` — covers **both** the
  integration-tagged `cli_test.go` (CLI dispatch) and `menu_test.go` (picker).

**Integration tests are an OPTIONAL gate step (deliberate operator decision).**
The two files that *stay* in `ide` and exercise the rewired call sites
(`cli.go`/`menu.go` → `Spawn` → `vscode.*`) are both `//go:build integration`,
so the mandatory plain `go test ./internal/ide/...` does **not** run them — it
runs only `spawn_test.go` (which already covers the full `Spawn` flow end-to-end
with the stubbed `codeLauncher`, asserting on the written `settings.json` /
`tasks.json` and color, i.e. the same `vscode.*` call path). The operator chose
to keep `-tags integration` optional rather than mandatory: the rewire changes
only call *targets*, not dispatch/picker logic, and `spawn_test.go` already
exercises the new `vscode.*` path under the mandatory gate. mill-plan/mill-go
should run the integration variant when convenient but must not block on it.

## Q&A log

- **Q:** Should `ide` consume `vscode` via a `Backend` interface or by direct
  import? **A:** Direct import, no interface — "IDE-generic" means free of VS
  Code details, not runtime-swappable (YAGNI; one backend).
- **Q:** Where does the `codeLauncher` test seam live after the move? **A:**
  Stays in `internal/ide/spawn.go` (`= vscode.Launch`), so the three stubbing
  test files need no change.
- **Q:** Exported API naming for the new package? **A:** `vscode.WriteConfig` /
  `vscode.PickColor` / `vscode.Launch` (idiomatic, no stutter).
- **Q:** What about `ErrIDEUnsupported`? **A:** Rename to `vscode.ErrUnsupported`
  (message: "vscode launch unsupported on this platform"); contained — no
  external consumer branches on it.
- **Q:** Are `palette` / `mainColor` exported? **A:** No — keep unexported.
  `palette` is used by `PickColor` + `color_test.go`; `mainColor` is consumed
  **only** by `color_test.go` (`PickColor` skips green by index), so it moves as
  test-visible state in a non-`_test.go` file (e.g. `color.go`).
- **Q:** (review r1 GAP) The mandatory `go test ./internal/ide/...` skips the
  integration-tagged `menu_test.go` + `cli_test.go`, leaving the picker/dispatch
  files unrun by the rewire's mandatory gate — make the integration gate
  mandatory? **A:** No — keep `-tags integration` **optional** (operator
  decision). `spawn_test.go` already exercises the rewired `vscode.*` path under
  the mandatory gate; the rewire changes call targets only. Documented the
  tradeoff explicitly in Testing.
- **Q:** Anything outside `ide` affected? **A:** No — only `cmd/lyx/main.go`
  imports `ide` (via `RunCLI`, untouched); no moved symbol is referenced
  elsewhere.
